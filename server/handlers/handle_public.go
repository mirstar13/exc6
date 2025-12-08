package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// HandleHomepage renders the landing page
func HandleHomepage() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Render("homepage", fiber.Map{
			"Title": "SecureChat - Private Messaging",
		})
	}
}

// HandleLoginForm renders the login form
// Supports both full page and HTMX partial rendering
func HandleLoginForm() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if isHTMXRequest(c) {
			return c.Render("partials/login", fiber.Map{})
		}
		return c.Render("login", fiber.Map{})
	}
}

// HandleRegisterForm renders the registration form
// Supports both full page and HTMX partial rendering
func HandleRegisterForm() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if isHTMXRequest(c) {
			return c.Render("partials/register", fiber.Map{})
		}
		return c.Render("register", fiber.Map{})
	}
}

// HandleSSETest renders the SSE testing page for development
func HandleSSETest() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Render("test-sse", fiber.Map{})
	}
}
