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

		// Store user info in context
		c.Locals("username", sess.Username)
		c.Locals("user_id", sess.UserID)

		// Only update session if last activity exceeds the threshold
		// This reduces Redis writes by ~95% for active users
		now := time.Now().Unix()
		timeSinceLastUpdate := now - sess.LastActivity

		if timeSinceLastUpdate >= int64(cfg.UpdateThreshold.Seconds()) {
			// Renew session TTL
			if err := cfg.SessionManager.RenewSession(c.Context(), sessionID); err != nil {
				return apperrors.NewInternalError("Failed to renew session").WithInternal(err)
			}

			// Update last activity timestamp
			cfg.SessionManager.UpdateSessionField(
				c.Context(),
				sessionID,
				"last_activity",
				fmt.Sprintf("%d", now),
			)
		}

		return c.Next()
	}
}
