package auth

import (
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

		sessionID := c.Cookies("session_id")
		if sessionID == "" {
			return redirectToLogin(c)
		}

		sess, err := cfg.SessionManager.GetSession(c.Context(), sessionID)
		if err != nil || sess == nil {
			// Session invalid or not found in Redis (expired/revoked)
			return redirectToLogin(c)
		}

		if err := cfg.SessionManager.RenewSession(c.Context(), sessionID); err != nil {
			return redirectToLogin(c)
		}

		c.Locals("username", sess.Username)
		c.Locals("user_id", sess.UserID)

		cfg.SessionManager.UpdateSessionField(c.Context(), sessionID, "last_activity", fmt.Sprintf("%d", time.Now().Unix()))

		return c.Next()
	}
}

func redirectToLogin(c *fiber.Ctx) error {
	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	return c.Redirect("/")
}
