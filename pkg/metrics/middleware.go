package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HTTPMetricsMiddleware tracks HTTP request metrics
func HTTPMetricsMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Increment in-flight requests
		HTTPRequestsInFlight.Inc()
		defer HTTPRequestsInFlight.Dec()

		// Process request
		err := c.Next()

		// Calculate duration
		duration := time.Since(start).Seconds()

		// Get status code
		status := c.Response().StatusCode()
		statusStr := strconv.Itoa(status)

		// Get method and path
		method := c.Method()
		path := sanitizePath(c.Path())

		// Record metrics
		HTTPRequestDuration.WithLabelValues(method, path, statusStr).Observe(duration)
		HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()

		return err
	}
}

// sanitizePath removes dynamic segments to avoid high cardinality
// Example: /chat/john123 -> /chat/:contact
func sanitizePath(path string) string {
	// Common patterns to normalize
	patterns := map[string]string{
		"/chat/":    "/chat/:contact",
		"/friends/": "/friends/:action",
		"/api/v1/":  "/api/v1/:endpoint",
	}

	for prefix, normalized := range patterns {
		if len(path) > len(prefix) && path[:len(prefix)] == prefix {
			return normalized
		}
	}

	// Static paths
	switch path {
	case "/", "/login", "/register", "/dashboard", "/profile", "/friends":
		return path
	case "/login-form", "/register-form", "/profile/edit":
		return path
	default:
		return "/other"
	}
}
