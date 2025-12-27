package apperrors

import (
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ErrorCode represents application-specific error codes
type ErrorCode string

const (
	// Authentication & Authorization
	ErrCodeUnauthorized    ErrorCode = "UNAUTHORIZED"
	ErrCodeInvalidCreds    ErrorCode = "INVALID_CREDENTIALS"
	ErrCodeSessionExpired  ErrorCode = "SESSION_EXPIRED"
	ErrCodeSessionNotFound ErrorCode = "SESSION_NOT_FOUND"

	// User Management
	ErrCodeUserNotFound     ErrorCode = "USER_NOT_FOUND"
	ErrCodeUserExists       ErrorCode = "USER_EXISTS"
	ErrCodeInvalidUsername  ErrorCode = "INVALID_USERNAME"
	ErrCodeWeakPassword     ErrorCode = "WEAK_PASSWORD"
	ErrCodePasswordMismatch ErrorCode = "PASSWORD_MISMATCH"

	// File Upload
	ErrCodeInvalidFileType ErrorCode = "INVALID_FILE_TYPE"
	ErrCodeFileTooLarge    ErrorCode = "FILE_TOO_LARGE"
	ErrCodeInvalidFilename ErrorCode = "INVALID_FILENAME"
	ErrCodeUploadFailed    ErrorCode = "UPLOAD_FAILED"

	// Chat & Messaging
	ErrCodeMessageEmpty  ErrorCode = "MESSAGE_EMPTY"
	ErrCodeChatNotFound  ErrorCode = "CHAT_NOT_FOUND"
	ErrCodeMessageFailed ErrorCode = "MESSAGE_SEND_FAILED"

	// Database & Storage
	ErrCodeDatabaseError ErrorCode = "DATABASE_ERROR"
	ErrCodeSaveFailed    ErrorCode = "SAVE_FAILED"
	ErrCodeNotFound      ErrorCode = "NOT_FOUND"

	// Rate Limiting
	ErrCodeRateLimited ErrorCode = "RATE_LIMITED"

	// Validation
	ErrCodeValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrCodeInvalidInput     ErrorCode = "INVALID_INPUT"

	// Internal Errors
	ErrCodeInternal       ErrorCode = "INTERNAL_ERROR"
	ErrCodeServiceUnavail ErrorCode = "SERVICE_UNAVAILABLE"
)

// AppError represents a structured application error with rich context
type AppError struct {
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	StatusCode int                    `json:"-"`
	Internal   error                  `json:"-"`
	Details    map[string]interface{} `json:"details,omitempty"`

	// New fields for better error tracking
	Operation string                 `json:"-"` // What operation failed
	Stack     []string               `json:"-"` // Call stack (optional)
	Timestamp time.Time              `json:"-"`
	Context   map[string]interface{} `json:"-"` // Additional context for logging
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("[%s] %s: %v (operation: %s)",
			e.Code, e.Message, e.Internal, e.Operation)
	}
	return fmt.Sprintf("[%s] %s (operation: %s)",
		e.Code, e.Message, e.Operation)
}

// Unwrap returns the wrapped error
func (e *AppError) Unwrap() error {
	return e.Internal
}

// WithDetails adds contextual details to the error
func (e *AppError) WithDetails(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// WithInternal wraps an internal error
func (e *AppError) WithInternal(err error) *AppError {
	e.Internal = err
	return e
}

// WithOperation sets the operation that failed
func (e *AppError) WithOperation(op string) *AppError {
	e.Operation = op
	return e
}

// WithContext adds logging context (not exposed in API response)
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithTimestamp sets the error timestamp (automatically set on creation)
func (e *AppError) WithTimestamp(t time.Time) *AppError {
	e.Timestamp = t
	return e
}

// LogFields returns fields suitable for structured logging
func (e *AppError) LogFields() map[string]interface{} {
	fields := map[string]interface{}{
		"error_code":  e.Code,
		"message":     e.Message,
		"operation":   e.Operation,
		"status_code": e.StatusCode,
		"timestamp":   e.Timestamp,
	}

	// Add details
	for k, v := range e.Details {
		fields["detail_"+k] = v
	}

	// Add context
	for k, v := range e.Context {
		fields["ctx_"+k] = v
	}

	// Add internal error message
	if e.Internal != nil {
		fields["internal_error"] = e.Internal.Error()
	}

	return fields
}

// New creates a new AppError with timestamp
func New(code ErrorCode, message string, statusCode int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
		Details:    make(map[string]interface{}),
		Context:    make(map[string]interface{}),
		Timestamp:  time.Now(),
	}
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) bool {
	var appErr *AppError
	return errors.As(err, &appErr)
}

// FromError converts a standard error to AppError if possible
func FromError(err error) *AppError {
	if err == nil {
		return nil
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	// Convert known stdlib/library errors
	if errors.Is(err, fiber.ErrUnauthorized) {
		return NewUnauthorized("")
	}
	if errors.Is(err, fiber.ErrNotFound) {
		return New(ErrCodeNotFound, "Resource not found", fiber.StatusNotFound)
	}
	if errors.Is(err, fiber.ErrBadRequest) {
		return NewValidationError("Invalid request")
	}

	// Default to internal error
	return NewInternalError("").WithInternal(err)
}
