package limiter

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// Config defines the configuration for the rate limiter
type Config struct {
	// Next defines a function to skip middleware.
	//
	// Optional. Default: nil
	Next func(c *fiber.Ctx) bool

	// Max number of requests allowed
	//
	// Optional. Default: 100
	Capacity int64

	// Number of tokens to add per refill period
	//
	// Optional. Default: 10
	RefillRate int64

	// How often to refill tokens
	//
	// Optional. Default: 1 second
	RefillPeriod time.Duration

	// KeyGenerator allows you to generate custom keys for rate limiting
	//
	// Optional. Default: uses IP address
	KeyGenerator func(c *fiber.Ctx) string

	// Handler is called when rate limit is exceeded
	LimitReachedHandler fiber.Handler

	// Storage for buckets (can be in-memory, Redis, etc.)
	//
	// Optional. Default: InMemory
	Storage Storage
}

// ConfigDefault provides default configuration
var ConfigDefault = Config{
	Capacity:     100,
	RefillRate:   10,
	RefillPeriod: time.Second,
	KeyGenerator: func(c *fiber.Ctx) string {
		return c.IP()
	},
	LimitReachedHandler: func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"error": "Rate limit exceeded",
		})
	},
	Storage: NewInMemoryStorage(),
}

func configDefault(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	cfg := config[0]

	if cfg.Capacity <= 0 {
		cfg.Capacity = ConfigDefault.Capacity
	}
	if cfg.RefillRate <= 0 {
		cfg.RefillRate = ConfigDefault.RefillRate
	}
	if cfg.RefillPeriod <= 0 {
		cfg.RefillPeriod = ConfigDefault.RefillPeriod
	}
	if cfg.KeyGenerator == nil {
		cfg.KeyGenerator = ConfigDefault.KeyGenerator
	}
	if cfg.LimitReachedHandler == nil {
		cfg.LimitReachedHandler = ConfigDefault.LimitReachedHandler
	}
	if cfg.Storage == nil {
		cfg.Storage = ConfigDefault.Storage
	}

	return cfg
}
