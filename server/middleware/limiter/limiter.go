package limiter

import (
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

		// Get or create bucket for this key
		bucket, err := cfg.Storage.Get(key)
		if err != nil {
			return err
		}

		if bucket == nil {
			bucket = NewTokenBucket(cfg.Capacity, cfg.RefillRate, cfg.RefillPeriod)
			// Don't save yet, wait until we take a token
		}

		// Try to take a token
		took := bucket.Take(1)

		if err := cfg.Storage.Set(key, bucket); err != nil {
			return err
		}

		if !took {
			return cfg.LimitReachedHandler(c)
		}

		return c.Next()
	}
}
