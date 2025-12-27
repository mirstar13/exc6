package limiter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Storage is an interface for storing and retrieving token buckets
type Storage interface {
	// Get retrieves a token bucket for the given key
	Get(key string) (*TokenBucket, error)

	// Set stores a token bucket for the given key
	Set(key string, bucket *TokenBucket) error

	// Set stores a token bucket for the given key only if it does not already exist
	SetIfNotExists(key string, bucket *TokenBucket) (bool, error)

	// Delete removes a token bucket for the given key
	Delete(key string) error

	// Reset clears all stored token buckets
	Reset() error
}

type InMemoryStorage struct {
	buckets map[string]*TokenBucket
	mu      sync.RWMutex
}

func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		buckets: make(map[string]*TokenBucket),
	}
}

func (s *InMemoryStorage) Get(key string) (*TokenBucket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bucket, exists := s.buckets[key]
	if !exists {
		return nil, nil
	}
	return bucket, nil
}

func (s *InMemoryStorage) Set(key string, bucket *TokenBucket) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buckets[key] = bucket
	return nil
}

func (s *InMemoryStorage) SetIfNotExists(key string, bucket *TokenBucket) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.buckets[key]; exists {
		return false, nil
	}
	s.buckets[key] = bucket
	return true, nil
}

func (s *InMemoryStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.buckets, key)
	return nil
}

func (s *InMemoryStorage) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buckets = make(map[string]*TokenBucket)
	return nil
}

type RedisStorage struct {
	client *redis.Client
	ctx    context.Context
	ttl    time.Duration
}

func NewRedisStorage(client *redis.Client, ttl time.Duration) *RedisStorage {
	return &RedisStorage{
		client: client,
		ctx:    context.Background(),
		ttl:    ttl,
	}
}

func (s *RedisStorage) Get(key string) (*TokenBucket, error) {
	data, err := s.client.Get(s.ctx, "ratelimit:"+key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var bucket TokenBucket
	if err := json.Unmarshal(data, &bucket); err != nil {
		return nil, err
	}

	bucket.refill()

	return &bucket, nil
}

func (s *RedisStorage) Set(key string, bucket *TokenBucket) error {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	data, err := json.Marshal(bucket)
	if err != nil {
		return err
	}
	return s.client.Set(s.ctx, "ratelimit:"+key, data, s.ttl).Err()
}

// SetIfNotExists atomically sets the bucket only if the key doesn't exist
// Returns true if the bucket was set, false if it already existed
func (s *RedisStorage) SetIfNotExists(key string, bucket *TokenBucket) (bool, error) {
	data, err := json.Marshal(bucket)
	if err != nil {
		return false, fmt.Errorf("failed to marshal bucket: %w", err)
	}

	// Use Redis SETNX (SET if Not eXists) operation
	// Returns true if the key was set, false if it already exists
	result, err := s.client.SetNX(context.Background(), key, data, s.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to set bucket: %w", err)
	}

	return result, nil
}

func (s *RedisStorage) Delete(key string) error {
	return s.client.Del(s.ctx, "ratelimit:"+key).Err()
}

func (s *RedisStorage) Reset() error {
	return s.client.FlushDB(s.ctx).Err()
}
