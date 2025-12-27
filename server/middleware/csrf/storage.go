package csrf

import (
	"container/list"
	"context"
	"exc6/apperrors"
	"exc6/pkg/logger"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Storage interface for CSRF token storage
type Storage interface {
	Get(key string) (string, error)
	Set(key string, value string, expiration time.Duration) error
	Delete(key string) error
}

// InMemoryStorage implements in-memory token storage with LRU eviction
type InMemoryStorage struct {
	mu        sync.RWMutex
	tokens    map[string]*list.Element
	evictList *list.List
	capacity  int
}

type tokenEntry struct {
	key        string
	value      string
	expiration time.Time
}

func NewInMemoryStorage() *InMemoryStorage {
	storage := &InMemoryStorage{
		tokens:    make(map[string]*list.Element),
		evictList: list.New(),
		capacity:  10000, // Limit to 10k tokens
	}

	// Cleanup expired tokens every 5 minutes
	go storage.cleanup()

	return storage
}

func (s *InMemoryStorage) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if elem, exists := s.tokens[key]; exists {
		s.evictList.MoveToFront(elem)
		entry := elem.Value.(*tokenEntry)

		if time.Now().After(entry.expiration) {
			return "", apperrors.New(apperrors.ErrCodeSessionExpired, "Token expired", 401)
		}

		return entry.value, nil
	}

	return "", apperrors.New(apperrors.ErrCodeNotFound, "Token not found", 404)
}

func (s *InMemoryStorage) Set(key string, value string, expiration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for existing item
	if elem, exists := s.tokens[key]; exists {
		s.evictList.MoveToFront(elem)
		entry := elem.Value.(*tokenEntry)
		entry.value = value
		entry.expiration = time.Now().Add(expiration)
		return nil
	}

	// Evict if over capacity
	if s.evictList.Len() >= s.capacity {
		s.removeOldest()
	}

	// Add new item
	entry := &tokenEntry{
		key:        key,
		value:      value,
		expiration: time.Now().Add(expiration),
	}
	elem := s.evictList.PushFront(entry)
	s.tokens[key] = elem

	return nil
}

func (s *InMemoryStorage) removeOldest() {
	elem := s.evictList.Back()
	if elem != nil {
		s.evictList.Remove(elem)
		entry := elem.Value.(*tokenEntry)
		delete(s.tokens, entry.key)
	}
}

func (s *InMemoryStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if elem, exists := s.tokens[key]; exists {
		s.evictList.Remove(elem)
		delete(s.tokens, key)
	}
	return nil
}

func (s *InMemoryStorage) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		// Iterate backwards to safely remove while traversing (optional optimization,
		// but simple iteration works too if we restart or carefully manage pointers)
		// For simplicity, we just scan randomly or check the tail (LRU tail might be old but valid,
		// while front might be expired if short TTL).
		// A full scan is safer for strict expiration.
		var next *list.Element
		for elem := s.evictList.Front(); elem != nil; elem = next {
			next = elem.Next()
			entry := elem.Value.(*tokenEntry)
			if now.After(entry.expiration) {
				s.evictList.Remove(elem)
				delete(s.tokens, entry.key)
			}
		}
		s.mu.Unlock()
	}
}

type RedisStorage struct {
	client    *redis.Client
	prefix    string
	ttl       time.Duration
	cacheMu   sync.RWMutex
	cache     map[string]*list.Element
	evictList *list.List
	capacity  int
}

func NewRedisStorage(client *redis.Client, ttl time.Duration) *RedisStorage {
	return &RedisStorage{
		client:    client,
		prefix:    "csrf:",
		ttl:       ttl,
		cache:     make(map[string]*list.Element),
		evictList: list.New(),
		capacity:  5000, // Local cache size
	}
}

func (s *RedisStorage) Get(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Try Redis first
	val, err := s.client.Get(ctx, s.prefix+key).Result()

	// 2. If Redis works, update local cache (Read-Through) and return
	if err == nil {
		s.updateLocalCache(key, val, s.ttl)
		return val, nil
	}

	// 3. If Redis fails (Down or Timeout), check local cache
	if err != redis.Nil {
		logger.WithField("error", err).Warn("CSRF Redis unavailable, checking local cache")

		s.cacheMu.Lock() // Upgraded to Lock for LRU move
		defer s.cacheMu.Unlock()

		if elem, ok := s.cache[key]; ok {
			s.evictList.MoveToFront(elem)
			entry := elem.Value.(*tokenEntry)
			if time.Now().Before(entry.expiration) {
				return entry.value, nil
			}
		}
	}

	// 4. Return original error if not found in either
	if err == redis.Nil {
		return "", apperrors.New(apperrors.ErrCodeNotFound, "CSRF token not found", 404)
	}
	return "", err
}

func (s *RedisStorage) updateLocalCache(key, value string, ttl time.Duration) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if elem, exists := s.cache[key]; exists {
		s.evictList.MoveToFront(elem)
		entry := elem.Value.(*tokenEntry)
		entry.value = value
		entry.expiration = time.Now().Add(ttl)
		return
	}

	if s.evictList.Len() >= s.capacity {
		oldest := s.evictList.Back()
		if oldest != nil {
			s.evictList.Remove(oldest)
			entry := oldest.Value.(*tokenEntry)
			delete(s.cache, entry.key)
		}
	}

	entry := &tokenEntry{
		key:        key,
		value:      value,
		expiration: time.Now().Add(ttl),
	}
	elem := s.evictList.PushFront(entry)
	s.cache[key] = elem
}

func (s *RedisStorage) Set(key string, value string, expiration time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Always save to local cache
	s.updateLocalCache(key, value, expiration)

	// 2. Try saving to Redis
	err := s.client.Set(ctx, s.prefix+key, value, expiration).Err()

	// 3. Log error but DO NOT fail the operation if Redis is down
	if err != nil {
		logger.WithFields(map[string]interface{}{
			"key":   key,
			"error": err,
		}).Error("CSRF Redis write failed, relying on local cache")
		return nil
	}

	return nil
}

func (s *RedisStorage) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Delete from local cache
	s.cacheMu.Lock()
	if elem, exists := s.cache[key]; exists {
		s.evictList.Remove(elem)
		delete(s.cache, key)
	}
	s.cacheMu.Unlock()

	// 2. Delete from Redis
	return s.client.Del(ctx, s.prefix+key).Err()
}
