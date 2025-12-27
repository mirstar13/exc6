package breaker

import (
	"context"
	"database/sql"
	"exc6/pkg/logger"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

// Prometheus Metrics
var (
	breakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "State of the circuit breaker (0: Closed, 1: Half-Open, 2: Open)",
		},
		[]string{"name"},
	)

	breakerRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "circuit_breaker_requests_total",
			Help: "Total number of requests handled by the circuit breaker",
		},
		[]string{"name", "result"}, // result: success, failure, short_circuit
	)

	// Registry to map breaker instances to their names for metric labeling
	breakerRegistry sync.Map
)

func init() {
	// Register metrics with Prometheus
	prometheus.MustRegister(breakerState)
	prometheus.MustRegister(breakerRequests)
}

// Config allows custom settings for specific breakers
type Config struct {
	Name        string
	MaxRequests uint32
	Interval    time.Duration
	Timeout     time.Duration
	Threshold   float64 // Failure ratio threshold (default 0.5)
	MinRequests uint32  // Minimum requests before tripping (default 5)
}

// New creates a new CircuitBreaker with sensible defaults and metric instrumentation
func New(cfg Config) *gobreaker.CircuitBreaker {
	// Set defaults
	if cfg.MaxRequests == 0 {
		cfg.MaxRequests = 5 // Half-open max requests
	}
	if cfg.Interval == 0 {
		cfg.Interval = 60 * time.Second // Clear counts interval
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second // Open state duration
	}
	if cfg.Threshold == 0 {
		cfg.Threshold = 0.5 // 50% failure rate
	}
	if cfg.MinRequests == 0 {
		cfg.MinRequests = 5
	}

	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < cfg.MinRequests {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cfg.Threshold
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Update Prometheus Gauge
			var stateVal float64
			switch to {
			case gobreaker.StateClosed:
				stateVal = 0
			case gobreaker.StateHalfOpen:
				stateVal = 1
			case gobreaker.StateOpen:
				stateVal = 2
			}
			breakerState.WithLabelValues(name).Set(stateVal)

			logger.WithFields(map[string]interface{}{
				"breaker": name,
				"from":    from.String(),
				"to":      to.String(),
			}).Warn("Circuit breaker state changed")
		},
	}

	cb := gobreaker.NewCircuitBreaker(settings)

	// Register instance for lookups in Execute
	breakerRegistry.Store(cb, cfg.Name)

	// Initialize state metric to Closed (0)
	breakerState.WithLabelValues(cfg.Name).Set(0)

	return cb
}

// IsRecoverableError determines if an error should trip the circuit breaker
func IsRecoverableError(err error) bool {
	if err == nil {
		return false
	}

	// Redis errors that should trip the breaker
	if err == redis.Nil {
		return false // Not found is not a service failure
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return true // Timeout/cancellation should trip
	}

	// Database errors that should trip the breaker
	if err == sql.ErrNoRows {
		return false // Not found is not a service failure
	}
	if err == sql.ErrConnDone || err == sql.ErrTxDone {
		return true // Connection issues should trip
	}

	// Default: count as failure
	return true
}

// Execute wraps circuit breaker execution with error classification and metrics
func Execute(cb *gobreaker.CircuitBreaker, fn func() (interface{}, error)) (interface{}, error) {
	// Retrieve name for metrics
	var name string
	if val, ok := breakerRegistry.Load(cb); ok {
		name = val.(string)
	} else {
		name = "unknown"
	}

	result, err := cb.Execute(func() (interface{}, error) {
		res, err := fn()
		// Classify the error
		if err != nil && !IsRecoverableError(err) {
			// Don't count this as a failure
			return res, nil
		}
		return res, err
	})

	// Record Metrics
	if err == nil {
		breakerRequests.WithLabelValues(name, "success").Inc()
	} else {
		if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
			breakerRequests.WithLabelValues(name, "short_circuit").Inc()
		} else {
			breakerRequests.WithLabelValues(name, "failure").Inc()
		}
	}

	return result, err
}

// ExecuteCtx wraps circuit breaker execution with context
func ExecuteCtx(ctx context.Context, cb *gobreaker.CircuitBreaker, fn func() (interface{}, error)) (interface{}, error) {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return Execute(cb, fn)
}
