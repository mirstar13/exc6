package csrf

import (
	"exc6/apperrors"
	"sync"
	"time"
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
