package handlers

import (
	"exc6/db"
	"exc6/services/chat"

	"github.com/gofiber/fiber/v2"
)

func HandleDashboard(csrv *chat.ChatService, udb *db.UsersDB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		contactUsernames := csrv.GetContacts(username)

		// Convert usernames to full user objects for template
		contacts := make([]*db.User, 0, len(contactUsernames))
		for _, contactName := range contactUsernames {
			if user := udb.FindUserByUsername(contactName); user != nil {
				contacts = append(contacts, user)
			}
		}

		currentUserIcon := ""
		currentUserCustomIcon := ""

		user := udb.FindUserByUsername(username)
		if user != nil {
			currentUserIcon = user.Icon
			currentUserCustomIcon = user.CustomIcon
		}

		return c.Render("dashboard", fiber.Map{
			"Username":              username,
			"CurrentUserIcon":       currentUserIcon,
			"CurrentUserCustomIcon": currentUserCustomIcon,
			"Contacts":              contacts,
		})
	}
}

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
