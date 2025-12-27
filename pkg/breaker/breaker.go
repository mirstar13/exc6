package breaker

import (
	"exc6/pkg/logger"
	"time"

	"github.com/sony/gobreaker"
)

// Config allows custom settings for specific breakers
type Config struct {
	Name        string
	MaxRequests uint32
	Interval    time.Duration
	Timeout     time.Duration
}

// New creates a new CircuitBreaker with sensible defaults
func New(cfg Config) *gobreaker.CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip if failure ratio is > 50% and we have at least 5 requests
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.5
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Info("circuit breaker '%s' changed state from %s to %s", name, from.String(), to.String())
		},
	}

	if settings.MaxRequests == 0 {
		settings.MaxRequests = 5 // Half-open max requests
	}
	if settings.Interval == 0 {
		settings.Interval = 60 * time.Second // Clear counts interval
	}
	if settings.Timeout == 0 {
		settings.Timeout = 30 * time.Second // Open state duration
	}

	return gobreaker.NewCircuitBreaker(settings)
}
