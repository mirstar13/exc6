package handlers

import (
	"exc6/server/middleware/csrf"
	"time"

	"github.com/gofiber/fiber/v2"
)

// InjectCSRFToken is middleware to inject CSRF token into templates AND locals
func InjectCSRFToken(storage csrf.Storage, expiration time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionID := c.Cookies("session_id")
		if sessionID != "" {
			// Try to get existing token first
			existingToken, err := storage.Get(sessionID)

			var token string
			if err != nil || existingToken == "" {
				// Generate new token if none exists
				newToken, genErr := csrf.GenerateToken(c, storage, expiration)
				if genErr != nil {
					// Don't fail the request, just log and continue
					token = ""
				} else {
					token = newToken
				}
			} else {
				token = existingToken
			}

			// Store in locals for BOTH template access AND programmatic access
			if token != "" {
				c.Locals("csrf_token", token)
			}
		}
		return c.Next()
	}
}
