package apperrors

import (
	"errors"
	"fmt"
	"net/http"

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

// AppError represents a structured application error
type AppError struct {
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	StatusCode int                    `json:"-"`
	Internal   error                  `json:"-"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
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

// New creates a new AppError
func New(code ErrorCode, message string, statusCode int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
	}
}

// Pre-defined error constructors for common cases

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

func NewDatabaseError(operation string, err error) *AppError {
	return New(ErrCodeDatabaseError, fmt.Sprintf("Database error during %s", operation), fiber.StatusInternalServerError).
		WithInternal(err)
}

func NewRateLimitError() *AppError {
	return New(ErrCodeRateLimited, "Too many requests. Please try again later.", http.StatusTooManyRequests)
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
