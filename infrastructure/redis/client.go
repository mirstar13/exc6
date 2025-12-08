package redis

import (
	"context"
	"exc6/config"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewClient creates a new Redis client with proper configuration and connection pooling
func NewClient(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,

		// Connection pool configuration
		PoolSize:     10,              // Maximum number of socket connections
		MinIdleConns: 5,               // Minimum number of idle connections
		MaxIdleConns: 10,              // Maximum number of idle connections
		PoolTimeout:  4 * time.Second, // Time to wait for connection from pool

		// Timeouts
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,

		// Connection age and idle timeout
		ConnMaxIdleTime: 5 * time.Minute,  // Close idle connections after this duration
		ConnMaxLifetime: 30 * time.Minute, // Close connections after this lifetime
	})

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis at %s: %w", cfg.Address, err)
	}

	return client, nil
}
