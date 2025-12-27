package limiter

import (
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type TokenBucket struct {
	Capacity     int64         `json:"capacity"`
	Tokens       int64         `json:"tokens"`
	RefillRate   int64         `json:"refill_rate"`
	RefillPeriod time.Duration `json:"refill_period"`
	LastRefill   time.Time     `json:"last_refill"`
	mu           sync.Mutex
}

func NewTokenBucket(capacity, refillRate int64, refillPeriod time.Duration) *TokenBucket {
	return &TokenBucket{
		Capacity:     capacity,
		Tokens:       capacity,
		RefillRate:   refillRate,
		RefillPeriod: refillPeriod,
		LastRefill:   time.Now(),
	}
}

func (tb *TokenBucket) Take(n int64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.Tokens >= n {
		tb.Tokens -= n
		return true
	}
	return false
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.LastRefill)

	if elapsed >= tb.RefillPeriod {
		periods := int64(elapsed / tb.RefillPeriod)
		tokensToAdd := periods * tb.RefillRate

		tb.Tokens += tokensToAdd
		if tb.Tokens > tb.Capacity {
			tb.Tokens = tb.Capacity
		}

		tb.LastRefill = now
	}
}

func New(config ...Config) fiber.Handler {
	cfg := configDefault(config...)

	return func(c *fiber.Ctx) error {
		key := cfg.KeyGenerator(c)

		var bucket *TokenBucket
		var err error

		// Retry loop for handling race conditions during bucket creation
		for attempts := 0; attempts < 3; attempts++ {
			// Get or create bucket for this key
			bucket, err = cfg.Storage.Get(key)
			if err != nil {
				return err
			}

			if bucket != nil {
				// Bucket exists, proceed to token consumption
				break
			}

			// Bucket doesn't exist, create a new one
			newBucket := NewTokenBucket(cfg.Capacity, cfg.RefillRate, cfg.RefillPeriod)

			// Try to atomically set the bucket if it doesn't exist
			// This prevents race conditions where multiple goroutines create buckets
			created, err := cfg.Storage.SetIfNotExists(key, newBucket)
			if err != nil {
				return err
			}

			if created {
				// We successfully created the bucket
				bucket = newBucket
				break
			}

			// Another goroutine created the bucket first, retry to get it
			// The retry will fetch the bucket that was just created by another goroutine
		}

		if bucket == nil {
			// This should never happen after retries, but handle defensively
			return fiber.ErrInternalServerError
		}

		// Try to take a token
		allowed := bucket.Take(1)

		// Save the updated bucket state back to storage
		if err := cfg.Storage.Set(key, bucket); err != nil {
			return err
		}

		if !allowed {
			c.Set(fiber.HeaderRetryAfter, strconv.FormatInt(int64(cfg.RefillPeriod.Seconds()), 10))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Rate limit exceeded. Please try again later.",
			})
		}

		return c.Next()
	}
}
