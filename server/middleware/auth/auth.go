package auth

import (
	"exc6/apperrors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

func New(config Config) fiber.Handler {
	cfg := configDefault(config)

	return func(c *fiber.Ctx) error {
		if cfg.Next != nil && config.Next(c) {
			return c.Next()
		}

		// Get session ID from cookie
		sessionID := c.Cookies("session_id")
		if sessionID == "" {
			return apperrors.NewUnauthorized("No session found")
		}

		// Retrieve session from Redis
		sess, err := cfg.SessionManager.GetSession(c.Context(), sessionID)
		if err != nil {
			return apperrors.NewInternalError("Failed to retrieve session").WithInternal(err)
		}

		if sess == nil {
			return apperrors.NewSessionExpired()
		}

		// Renew session TTL
		if err := cfg.SessionManager.RenewSession(c.Context(), sessionID); err != nil {
			return apperrors.NewInternalError("Failed to renew session").WithInternal(err)
		}

		// Store user info in context
		c.Locals("username", sess.Username)
		c.Locals("user_id", sess.UserID)

		// Update last activity timestamp
		cfg.SessionManager.UpdateSessionField(
			c.Context(),
			sessionID,
			"last_activity",
			fmt.Sprintf("%d", time.Now().Unix()),
		)

		return c.Next()
	}
}
