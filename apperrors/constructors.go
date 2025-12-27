package apperrors

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Database error helpers
func NewDatabaseError(operation string, err error) *AppError {
	return New(ErrCodeDatabaseError, "Database operation failed", fiber.StatusInternalServerError).
		WithOperation(operation).
		WithInternal(err).
		WithContext("subsystem", "database")
}

func NewDatabaseQueryError(query string, params interface{}, err error) *AppError {
	return New(ErrCodeDatabaseError, "Database query failed", fiber.StatusInternalServerError).
		WithOperation("query_execution").
		WithDetails("query", truncateString(query, 100)).
		WithContext("params", params).
		WithInternal(err)
}

// Message delivery errors
func NewMessageDeliveryError(from, to string, reason string, err error) *AppError {
	return New(ErrCodeMessageFailed, "Failed to deliver message", fiber.StatusInternalServerError).
		WithOperation("message_delivery").
		WithDetails("from", from).
		WithDetails("to", to).
		WithDetails("reason", reason).
		WithContext("subsystem", "chat").
		WithInternal(err)
}

func NewGroupMessageDeliveryError(from, groupID string, reason string, err error) *AppError {
	return New(ErrCodeMessageFailed, "Failed to deliver group message", fiber.StatusInternalServerError).
		WithOperation("group_message_delivery").
		WithDetails("from", from).
		WithDetails("group_id", groupID).
		WithDetails("reason", reason).
		WithContext("subsystem", "chat").
		WithInternal(err)
}

// Redis/Cache errors
func NewCacheError(operation string, key string, err error) *AppError {
	return New(ErrCodeInternal, "Cache operation failed", fiber.StatusInternalServerError).
		WithOperation(operation).
		WithDetails("cache_key", key).
		WithContext("subsystem", "redis").
		WithInternal(err)
}

func NewCacheMissError(key string) *AppError {
	return New(ErrCodeNotFound, "Cache entry not found", fiber.StatusNotFound).
		WithOperation("cache_get").
		WithDetails("cache_key", key).
		WithContext("subsystem", "redis")
}

// Session errors
func NewSessionError(operation string, sessionID string, err error) *AppError {
	return New(ErrCodeSessionNotFound, "Session operation failed", fiber.StatusUnauthorized).
		WithOperation(operation).
		WithDetails("session_id", maskSessionID(sessionID)).
		WithContext("subsystem", "sessions").
		WithInternal(err)
}

func NewSessionExpiredError(sessionID string, expiresAt time.Time) *AppError {
	return New(ErrCodeSessionExpired, "Session has expired", fiber.StatusUnauthorized).
		WithOperation("session_validation").
		WithDetails("expired_at", expiresAt).
		WithDetails("session_id", maskSessionID(sessionID)).
		WithContext("subsystem", "sessions")
}

// WebSocket errors
func NewWebSocketError(operation string, username string, err error) *AppError {
	return New(ErrCodeInternal, "WebSocket operation failed", fiber.StatusInternalServerError).
		WithOperation(operation).
		WithDetails("username", username).
		WithContext("subsystem", "websocket").
		WithInternal(err)
}

func NewWebSocketBufferFullError(username string, bufferSize int) *AppError {
	return New(ErrCodeInternal, "WebSocket send buffer full", fiber.StatusServiceUnavailable).
		WithOperation("websocket_send").
		WithDetails("username", username).
		WithDetails("buffer_size", bufferSize).
		WithDetails("suggestion", "Client may be slow or disconnected").
		WithContext("subsystem", "websocket")
}

// Circuit breaker errors
func NewCircuitBreakerError(service string, state string) *AppError {
	return New(ErrCodeServiceUnavail, "Service temporarily unavailable", fiber.StatusServiceUnavailable).
		WithOperation("circuit_breaker_check").
		WithDetails("service", service).
		WithDetails("breaker_state", state).
		WithDetails("retry_after", "30s").
		WithContext("subsystem", "circuit_breaker")
}

// File upload errors
func NewFileUploadError(filename string, reason string, err error) *AppError {
	return New(ErrCodeUploadFailed, "File upload failed", fiber.StatusBadRequest).
		WithOperation("file_upload").
		WithDetails("filename", filename).
		WithDetails("reason", reason).
		WithContext("subsystem", "upload").
		WithInternal(err)
}

func NewFileValidationError(filename string, violations []string) *AppError {
	return New(ErrCodeInvalidFileType, "File validation failed", fiber.StatusBadRequest).
		WithOperation("file_validation").
		WithDetails("filename", filename).
		WithDetails("violations", violations).
		WithContext("subsystem", "upload")
}

// Authentication errors
func NewAuthenticationError(username string, reason string) *AppError {
	return New(ErrCodeInvalidCreds, "Authentication failed", fiber.StatusUnauthorized).
		WithOperation("user_authentication").
		WithDetails("username", username).
		WithDetails("reason", reason).
		WithContext("subsystem", "auth")
}

func NewAuthorizationError(username string, resource string, action string) *AppError {
	return New(ErrCodeUnauthorized, "Not authorized to perform action", fiber.StatusForbidden).
		WithOperation("authorization_check").
		WithDetails("username", username).
		WithDetails("resource", resource).
		WithDetails("action", action).
		WithContext("subsystem", "auth")
}

// Helper functions
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func maskSessionID(sessionID string) string {
	if len(sessionID) < 8 {
		return "***"
	}
	return sessionID[:4] + "****" + sessionID[len(sessionID)-4:]
}

// Old error constructors

func NewUnauthorized(message string) *AppError {
	if message == "" {
		message = "Authentication required"
	}
	return New(ErrCodeUnauthorized, message, fiber.StatusUnauthorized)
}

func NewInvalidCredentials() *AppError {
	return New(ErrCodeInvalidCreds, "Invalid username or password", fiber.StatusUnauthorized)
}

func NewSessionExpired() *AppError {
	return New(ErrCodeSessionExpired, "Your session has expired", fiber.StatusUnauthorized)
}

func NewUserNotFound() *AppError {
	return New(ErrCodeUserNotFound, "User not found", fiber.StatusNotFound)
}

func NewUserExists(username string) *AppError {
	return New(ErrCodeUserExists, "Username already exists", fiber.StatusConflict).
		WithDetails("username", username)
}

func NewWeakPassword(reason string) *AppError {
	return New(ErrCodeWeakPassword, fmt.Sprintf("Password too weak: %s", reason), fiber.StatusBadRequest)
}

func NewPasswordMismatch() *AppError {
	return New(ErrCodePasswordMismatch, "Passwords do not match", fiber.StatusBadRequest)
}

func NewInvalidFileType(allowed []string) *AppError {
	return New(ErrCodeInvalidFileType, "Invalid file type", fiber.StatusBadRequest).
		WithDetails("allowed_types", allowed)
}

func NewFileTooLarge(maxSize int64) *AppError {
	return New(ErrCodeFileTooLarge, "File size exceeds limit", fiber.StatusBadRequest).
		WithDetails("max_size_bytes", maxSize)
}

func NewValidationError(message string) *AppError {
	return New(ErrCodeValidationFailed, message, fiber.StatusBadRequest)
}

func NewBadRequest(message string) *AppError {
	if message == "" {
		message = "Bad request"
	}
	return New(ErrCodeInvalidInput, message, fiber.StatusBadRequest)
}

func NewInternalError(message string) *AppError {
	if message == "" {
		message = "An internal error occurred"
	}
	return New(ErrCodeInternal, message, fiber.StatusInternalServerError)
}

func NewRateLimitError() *AppError {
	return New(ErrCodeRateLimited, "Too many requests. Please try again later.", http.StatusTooManyRequests)
}
