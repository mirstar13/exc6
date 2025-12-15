package handlers

import (
	"context"
	"exc6/db"
	"exc6/services/chat"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// HealthCheckHandler provides health and readiness checks
type HealthCheckHandler struct {
	rdb  *redis.Client
	qdb  *db.Queries
	csrv *chat.ChatService
}

// NewHealthCheckHandler creates a new health check handler
func NewHealthCheckHandler(rdb *redis.Client, qdb *db.Queries, csrv *chat.ChatService) *HealthCheckHandler {
	return &HealthCheckHandler{
		rdb:  rdb,
		qdb:  qdb,
		csrv: csrv,
	}
}

// HealthCheckResponse represents the health status
type HealthCheckResponse struct {
	Status    string                 `json:"status"`
	Timestamp string                 `json:"timestamp"`
	Version   string                 `json:"version"`
	Uptime    float64                `json:"uptime_seconds"`
	Checks    map[string]CheckStatus `json:"checks"`
	Metrics   map[string]interface{} `json:"metrics,omitempty"`
}

// CheckStatus represents individual component status
type CheckStatus struct {
	Status      string  `json:"status"`
	Message     string  `json:"message,omitempty"`
	Latency     float64 `json:"latency_ms,omitempty"`
	LastChecked string  `json:"last_checked"`
}

var startTime = time.Now()

// HandleHealthCheck performs a basic health check
func (h *HealthCheckHandler) HandleHealthCheck() fiber.Handler {
	return func(c *fiber.Ctx) error {
		response := HealthCheckResponse{
			Status:    "healthy",
			Timestamp: time.Now().Format(time.RFC3339),
			Version:   "1.0.0",
			Uptime:    time.Since(startTime).Seconds(),
			Checks:    make(map[string]CheckStatus),
		}

		// Quick checks (for load balancer health checks)
		response.Checks["server"] = CheckStatus{
			Status:      "up",
			Message:     "Server is running",
			LastChecked: time.Now().Format(time.RFC3339),
		}

		return c.JSON(response)
	}
}

// HandleReadinessCheck performs detailed readiness check
func (h *HealthCheckHandler) HandleReadinessCheck() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		response := HealthCheckResponse{
			Status:    "ready",
			Timestamp: time.Now().Format(time.RFC3339),
			Version:   "1.0.0",
			Uptime:    time.Since(startTime).Seconds(),
			Checks:    make(map[string]CheckStatus),
		}

		overallHealthy := true

		// Check Redis
		redisStatus := h.checkRedis(ctx)
		response.Checks["redis"] = redisStatus
		if redisStatus.Status != "healthy" {
			overallHealthy = false
		}

		// Check PostgreSQL
		pgStatus := h.checkPostgreSQL(ctx)
		response.Checks["postgresql"] = pgStatus
		if pgStatus.Status != "healthy" {
			overallHealthy = false
		}

		// Check Chat Service
		chatStatus := h.checkChatService()
		response.Checks["chat_service"] = chatStatus
		if chatStatus.Status != "healthy" {
			overallHealthy = false
		}

		if !overallHealthy {
			response.Status = "degraded"
			return c.Status(fiber.StatusServiceUnavailable).JSON(response)
		}

		return c.JSON(response)
	}
}

// HandleMetrics returns application metrics
func (h *HealthCheckHandler) HandleMetrics() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		metrics := make(map[string]interface{})

		// Chat service metrics
		if h.csrv != nil {
			metrics["chat"] = h.csrv.GetMetrics()
		}

		// Redis metrics
		if h.rdb != nil {
			info, err := h.rdb.Info(ctx, "stats").Result()
			if err == nil {
				metrics["redis"] = map[string]string{
					"info": info,
				}
			}
		}

		// System metrics
		metrics["uptime_seconds"] = time.Since(startTime).Seconds()
		metrics["timestamp"] = time.Now().Format(time.RFC3339)

		return c.JSON(fiber.Map{
			"status":  "ok",
			"metrics": metrics,
		})
	}
}

// checkRedis verifies Redis connectivity and latency
func (h *HealthCheckHandler) checkRedis(ctx context.Context) CheckStatus {
	start := time.Now()

	err := h.rdb.Ping(ctx).Err()
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CheckStatus{
			Status:      "unhealthy",
			Message:     "Redis connection failed: " + err.Error(),
			Latency:     float64(latency),
			LastChecked: time.Now().Format(time.RFC3339),
		}
	}

	status := "healthy"
	message := "Redis is responding"

	// Warn if latency is high
	if latency > 100 {
		status = "degraded"
		message = "Redis latency is high"
	}

	return CheckStatus{
		Status:      status,
		Message:     message,
		Latency:     float64(latency),
		LastChecked: time.Now().Format(time.RFC3339),
	}
}

// checkPostgreSQL verifies database connectivity
func (h *HealthCheckHandler) checkPostgreSQL(ctx context.Context) CheckStatus {
	start := time.Now()

	// Try a simple query
	_, err := h.qdb.GetAllUsernames(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CheckStatus{
			Status:      "unhealthy",
			Message:     "PostgreSQL query failed: " + err.Error(),
			Latency:     float64(latency),
			LastChecked: time.Now().Format(time.RFC3339),
		}
	}

	status := "healthy"
	message := "PostgreSQL is responding"

	// Warn if latency is high
	if latency > 500 {
		status = "degraded"
		message = "PostgreSQL latency is high"
	}

	return CheckStatus{
		Status:      status,
		Message:     message,
		Latency:     float64(latency),
		LastChecked: time.Now().Format(time.RFC3339),
	}
}

// checkChatService verifies chat service health
func (h *HealthCheckHandler) checkChatService() CheckStatus {
	if h.csrv == nil {
		return CheckStatus{
			Status:      "unhealthy",
			Message:     "Chat service not initialized",
			LastChecked: time.Now().Format(time.RFC3339),
		}
	}

	metrics := h.csrv.GetMetrics()

	// Check if there are too many failed messages
	failureRate := float64(0)
	if metrics["sent"] > 0 {
		failureRate = float64(metrics["failed"]) / float64(metrics["sent"]) * 100
	}

	status := "healthy"
	message := "Chat service is operational"

	if failureRate > 5 {
		status = "degraded"
		message = "High message failure rate"
	} else if failureRate > 20 {
		status = "unhealthy"
		message = "Critical message failure rate"
	}

	return CheckStatus{
		Status:      status,
		Message:     message,
		LastChecked: time.Now().Format(time.RFC3339),
	}
}

// HandleLivenessCheck is a simple liveness probe
func (h *HealthCheckHandler) HandleLivenessCheck() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.SendString("OK")
	}
}
