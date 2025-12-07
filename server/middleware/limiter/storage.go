package limiter

import (
	"context"
	"encoding/json"
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

	// Delete removes a token bucket for the given key
	Delete(key string) error
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

func (s *InMemoryStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.buckets, key)
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

func (s *RedisStorage) Delete(key string) error {
	return s.client.Del(s.ctx, "ratelimit:"+key).Err()
}
