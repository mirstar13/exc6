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
		key := csrf.GetTokenKey(c)

		// Log every request
		logger.WithFields(map[string]interface{}{
			"method": c.Method(),
			"path":   c.Path(),
			"key":    key,
		}).Debug("CSRF Injection: Processing request")

		var existingToken string
		var err error

		// Try to get existing token first if key exists
		if key != "" {
			existingToken, err = storage.Get(key)
		} else {
			// No key yet, treat as "not found"
			err = apperrors.New(apperrors.ErrCodeNotFound, "No key", 404)
		}

		if err != nil {
			// Check if it's a "not found" error vs a real error
			if appErr, ok := err.(*apperrors.AppError); ok && appErr.Code == apperrors.ErrCodeNotFound {
				logger.WithField("key", key).Debug("CSRF Injection: Token not found, will generate new one")
			} else {
				logger.WithFields(map[string]interface{}{
					"key":   key,
					"error": err,
				}).Error("CSRF Injection: Error retrieving token from storage")
			}
		}

		var token string

		// If no token exists or there was an error, generate new one
		if err != nil || existingToken == "" {
			logger.WithField("key", key).Info("CSRF Injection: Generating new CSRF token")

			// Generate new token (this will handle key generation if needed)
			newToken, newKey, genErr := csrf.GenerateToken(c, storage, expiration)
			if genErr != nil {
				logger.WithFields(map[string]interface{}{
					"key":   key,
					"error": genErr,
				}).Error("CSRF Injection: Failed to generate CSRF token")
				// Continue anyway - request should not fail due to CSRF generation issues
				return c.Next()
			}
			token = newToken
			key = newKey
			logger.WithFields(map[string]interface{}{
				"key":          key,
				"token_length": len(token),
			}).Info("CSRF Injection: New token generated successfully")
		} else {
			token = existingToken
			logger.WithFields(map[string]interface{}{
				"key":          key,
				"token_length": len(token),
			}).Debug("CSRF Injection: Using existing token")
		}

		// Store token in locals
		if token != "" {
			c.Locals("csrf_token", token)
			logger.WithFields(map[string]interface{}{
				"key":          key,
				"token_length": len(token),
				"path":         c.Path(),
			}).Info("CSRF Injection: Token stored in locals successfully")
		} else {
			logger.WithField("key", key).Error("CSRF Injection: Token is empty, cannot store in locals!")
		}

		return c.Next()
	}
}
