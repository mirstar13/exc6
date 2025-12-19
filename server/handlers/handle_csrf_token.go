package handlers

import (
	"exc6/apperrors"
	"exc6/pkg/logger"
	"exc6/server/middleware/csrf"
	"time"

	"github.com/gofiber/fiber/v2"
)

// InjectCSRFToken is middleware to inject CSRF token into templates AND locals
func InjectCSRFToken(storage csrf.Storage, expiration time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionID := c.Cookies("session_id")

		// Log every request
		logger.WithFields(map[string]interface{}{
			"method":     c.Method(),
			"path":       c.Path(),
			"session_id": sessionID,
		}).Debug("CSRF Injection: Processing request")

		// Only inject CSRF for authenticated users with sessions
		if sessionID == "" {
			logger.Debug("CSRF Injection: No session ID, skipping")
			return c.Next()
		}

		// Try to get existing token first
		existingToken, err := storage.Get(sessionID)

		if err != nil {
			// Check if it's a "not found" error vs a real error
			if appErr, ok := err.(*apperrors.AppError); ok && appErr.Code == apperrors.ErrCodeNotFound {
				logger.WithField("session_id", sessionID).Debug("CSRF Injection: Token not found, will generate new one")
			} else {
				logger.WithFields(map[string]interface{}{
					"session_id": sessionID,
					"error":      err,
				}).Error("CSRF Injection: Error retrieving token from storage")
			}
		}

		var token string

		// If no token exists or there was an error, generate new one
		if err != nil || existingToken == "" {
			logger.WithField("session_id", sessionID).Info("CSRF Injection: Generating new CSRF token")

			// Generate new token
			newToken, genErr := csrf.GenerateToken(c, storage, expiration)
			if genErr != nil {
				logger.WithFields(map[string]interface{}{
					"session_id": sessionID,
					"error":      genErr,
				}).Error("CSRF Injection: Failed to generate CSRF token")
				// Continue anyway - request should not fail due to CSRF generation issues
				return c.Next()
			}
			token = newToken
			logger.WithFields(map[string]interface{}{
				"session_id":   sessionID,
				"token_length": len(token),
			}).Info("CSRF Injection: New token generated successfully")
		} else {
			token = existingToken
			logger.WithFields(map[string]interface{}{
				"session_id":   sessionID,
				"token_length": len(token),
			}).Debug("CSRF Injection: Using existing token")
		}

		// Store token in locals
		if token != "" {
			c.Locals("csrf_token", token)
			logger.WithFields(map[string]interface{}{
				"session_id":   sessionID,
				"token_length": len(token),
				"path":         c.Path(),
			}).Info("CSRF Injection: Token stored in locals successfully")
		} else {
			logger.WithField("session_id", sessionID).Error("CSRF Injection: Token is empty, cannot store in locals!")
		}

		return c.Next()
	}
}
