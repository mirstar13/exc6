package handlers

import (
	"exc6/server/middleware/csrf"
	"time"

	"github.com/gofiber/fiber/v2"
)

// CSRFTokenHandler handles CSRF token generation for HTMX requests
type CSRFTokenHandler struct {
	storage    csrf.Storage
	expiration time.Duration
}

// NewCSRFTokenHandler creates a new CSRF token handler
func NewCSRFTokenHandler(storage csrf.Storage, expiration time.Duration) *CSRFTokenHandler {
	return &CSRFTokenHandler{
		storage:    storage,
		expiration: expiration,
	}
}

// HandleGetToken generates and returns a CSRF token
func (h *CSRFTokenHandler) HandleGetToken() fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, err := csrf.GenerateToken(c, h.storage, h.expiration)
		if err != nil {
			return err
		}

		// For HTMX requests, return as meta tag
		if c.Get("HX-Request") == "true" {
			return c.Type("html").SendString(
				`<meta name="csrf-token" content="` + token + `">`,
			)
		}

		// For API requests, return as JSON
		return c.JSON(fiber.Map{
			"csrf_token": token,
		})
	}
}

// HandleRefreshToken refreshes an existing CSRF token
func (h *CSRFTokenHandler) HandleRefreshToken() fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionID := c.Cookies("session_id")
		if sessionID == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "No session found",
			})
		}

		// Delete old token
		h.storage.Delete(sessionID)

		// Generate new token
		token, err := csrf.GenerateToken(c, h.storage, h.expiration)
		if err != nil {
			return err
		}

		return c.JSON(fiber.Map{
			"csrf_token": token,
		})
	}
}

// InjectCSRFToken is middleware to inject CSRF token into templates
func InjectCSRFToken(storage csrf.Storage, expiration time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionID := c.Cookies("session_id")
		if sessionID != "" {
			// Generate token for authenticated users
			token, err := csrf.GenerateToken(c, storage, expiration)
			if err == nil {
				// Store in context for template access
				c.Locals("csrf_token", token)
			}
		}
		return c.Next()
	}
}
