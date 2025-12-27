package csrf

import (
	"context"
	"exc6/apperrors"
	"exc6/pkg/logger" // Make sure to import your logger
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

// InMemoryStorage implements in-memory token storage
type InMemoryStorage struct {
	mu     sync.RWMutex
	tokens map[string]tokenEntry
}

type tokenEntry struct {
	value      string
	expiration time.Time
}

func NewInMemoryStorage() *InMemoryStorage {
	storage := &InMemoryStorage{
		tokens: make(map[string]tokenEntry),
	}

	// Cleanup expired tokens every 5 minutes
	go storage.cleanup()

	return storage
}

func (s *InMemoryStorage) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.tokens[key]
	if !exists {
		return "", apperrors.New(apperrors.ErrCodeNotFound, "Token not found", 404)
	}

	if time.Now().After(entry.expiration) {
		return "", apperrors.New(apperrors.ErrCodeSessionExpired, "Token expired", 401)
	}

	return entry.value, nil
}

func (s *InMemoryStorage) Set(key string, value string, expiration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[key] = tokenEntry{
		value:      value,
		expiration: time.Now().Add(expiration),
	}

	return nil
}

func (s *InMemoryStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tokens, key)
	return nil
}

func (s *InMemoryStorage) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for key, entry := range s.tokens {
			if now.After(entry.expiration) {
				delete(s.tokens, key)
			}
		}
		s.mu.Unlock()
	}
}

type RedisStorage struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
	// [NEW] Local cache for fallback
	cache sync.Map
}

func NewRedisStorage(client *redis.Client, ttl time.Duration) *RedisStorage {
	return &RedisStorage{
		client: client,
		prefix: "csrf:",
		ttl:    ttl,
	}
}

func (s *RedisStorage) Get(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Try Redis first
	val, err := s.client.Get(ctx, s.prefix+key).Result()

	// 2. If Redis works, update local cache (Read-Through) and return
	if err == nil {
		s.cache.Store(key, tokenEntry{
			value:      val,
			expiration: time.Now().Add(s.ttl), // Refresh TTL estimate
		})
		return val, nil
	}

	// 3. If Redis fails (Down or Timeout), check local cache
	if err != redis.Nil {
		logger.WithField("error", err).Warn("CSRF Redis unavailable, checking local cache")
		if entry, ok := s.cache.Load(key); ok {
			e := entry.(tokenEntry)
			if time.Now().Before(e.expiration) {
				return e.value, nil
			}
		}
	}

	// 4. Return original error if not found in either
	if err == redis.Nil {
		return "", apperrors.New(apperrors.ErrCodeNotFound, "CSRF token not found", 404)
	}
	return "", err
}

func (s *RedisStorage) Set(key string, value string, expiration time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Always save to local cache
	s.cache.Store(key, tokenEntry{
		value:      value,
		expiration: time.Now().Add(expiration),
	})

	// 2. Try saving to Redis
	err := s.client.Set(ctx, s.prefix+key, value, expiration).Err()

	// 3. Log error but DO NOT fail the operation if Redis is down
	// This ensures the user can still get a token and submit forms
	if err != nil {
		logger.WithFields(map[string]interface{}{
			"key":   key,
			"error": err,
		}).Error("CSRF Redis write failed, relying on local cache")
		// Return nil so the application proceeds successfully
		return nil
	}

	return nil
}

func (s *RedisStorage) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Delete from local cache
	s.cache.Delete(key)

	// 2. Delete from Redis
	return s.client.Del(ctx, s.prefix+key).Err()
}
