package handlers

import (
	"exc6/db"
	"exc6/services/chat"

	"github.com/gofiber/fiber/v2"
)

// HandleProfileView renders the user's profile page
func HandleProfileView(udb *db.UsersDB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		user := udb.FindUserByUsername(username)
		if user == nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}

		// Check if it's an HTMX request for partial rendering
		if isHTMXRequest(c) {
			return c.Render("partials/profile-view", user)
		}

		// Full page render
		return c.Render("profile", user)
	}
}

// HandleProfileEdit renders the profile edit form
func HandleProfileEdit(udb *db.UsersDB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		user := udb.FindUserByUsername(username)
		if user == nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}

		return c.Render("partials/profile-edit", fiber.Map{
			"Username":   user.Username,
			"UserId":     user.UserId,
			"Role":       user.Role,
			"Icon":       user.Icon,
			"CustomIcon": user.CustomIcon,
			"Saved":      false,
		})
	}
}

// HandleDashboard renders the main dashboard with chat list
func HandleDashboard(csrv *chat.ChatService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		contacts := csrv.GetContacts(username)

		return c.Render("dashboard", fiber.Map{
			"Username": username,
			"Contacts": contacts,
		})
	}
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
