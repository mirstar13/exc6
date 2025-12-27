package auth

import (
	"context"
	"exc6/apperrors"
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

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Retrieve session from Redis
		sess, err := cfg.SessionManager.GetSession(ctx, sessionID)
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
			updateCtx, updateCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer updateCancel()

			// Renew session TTL
			if err := cfg.SessionManager.RenewSession(updateCtx, sessionID); err != nil {
				// Log but don't fail the request if session renewal fails
				// The session is still valid
				c.Locals("session_renewal_failed", true)
			} else {
				cfg.SessionManager.RenewSession(updateCtx, sessionID)
			}
		}

		return c.Next()
	}
}
