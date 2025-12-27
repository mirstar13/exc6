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
		// Get CSRF token from context if it exists
		csrfToken := ""
		if token := c.Locals("csrf_token"); token != nil {
			csrfToken = token.(string)
		}

		templateData := fiber.Map{
			"CSRFToken": csrfToken,
		}

		if isHTMXRequest(c) {
			return c.Render("partials/login", templateData)
		}
		return c.Render("login", templateData)
	}
}

// HandleRegisterForm renders the registration form
// Supports both full page and HTMX partial rendering
func HandleRegisterForm() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get CSRF token from context if it exists
		csrfToken := ""
		if token := c.Locals("csrf_token"); token != nil {
			csrfToken = token.(string)
		}

		templateData := fiber.Map{
			"CSRFToken": csrfToken,
		}

		if isHTMXRequest(c) {
			return c.Render("partials/register", templateData)
		}
		return c.Render("register", templateData)
	}
}
