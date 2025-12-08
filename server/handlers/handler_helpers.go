package handlers

import (
	"exc6/db"

	"github.com/gofiber/fiber/v2"
)

// isHTMXRequest checks if the request is from HTMX
func isHTMXRequest(c *fiber.Ctx) bool {
	return c.Get("HX-Request") == "true"
}

// getUsernameFromContext safely extracts username from context locals
func getUsernameFromContext(c *fiber.Ctx) (string, error) {
	val := c.Locals("username")
	if val == nil {
		return "", fiber.ErrUnauthorized
	}

	username, ok := val.(string)
	if !ok || username == "" {
		return "", fiber.ErrUnauthorized
	}

	return username, nil
}

// handleUnauthorized redirects to login for unauthorized requests
func handleUnauthorized(c *fiber.Ctx) error {
	if isHTMXRequest(c) {
		c.Set("HX-Redirect", "/")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	return c.Redirect("/")
}

// renderProfileEditError is a helper to render profile edit with error
func renderProfileEditError(ctx *fiber.Ctx, user *db.User, errorMsg string) error {
	return ctx.Render("partials/profile-edit", fiber.Map{
		"Username":   user.Username,
		"UserId":     user.ID,
		"Role":       user.Role,
		"Icon":       user.Icon,
		"CustomIcon": user.CustomIcon,
		"Error":      errorMsg,
	})
}
