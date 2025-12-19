package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
)

type MockRedisClient struct {
	mock.Mock
	data map[string]string
	mu   sync.RWMutex
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		data: make(map[string]string),
	}
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	args := m.Called(ctx, key, value, expiration)

	m.mu.Lock()
	m.data[key] = value.(string)
	m.mu.Unlock()

	return redis.NewStatusResult("OK", args.Error(0))
}

func (m *MockRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	args := m.Called(ctx, key)

	m.mu.RLock()
	val, exists := m.data[key]
	m.mu.RUnlock()

	if !exists {
		return redis.NewStringResult("", redis.Nil)
	}

	return redis.NewStringResult(val, args.Error(0))
}

func (m *MockRedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	args := m.Called(ctx, keys)

	m.mu.Lock()
	for _, key := range keys {
		delete(m.data, key)
	}
	m.mu.Unlock()

	return redis.NewIntResult(int64(len(keys)), args.Error(0))
}
