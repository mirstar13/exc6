package handlers

import (
	"exc6/db"
	"exc6/services/chat"

	"github.com/gofiber/fiber/v2"
)

// HandleDashboard renders the main dashboard with contacts list
func HandleDashboard(csrv *chat.ChatService, qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		contactUsernames, err := csrv.GetContacts(username)
		if err != nil {
			return err
		}

		// Convert usernames to full user objects for template
		contacts := make([]*db.User, 0, len(contactUsernames))
		for _, contactName := range contactUsernames {
			if user, err := qdb.GetUserByUsername(c.Context(), contactName); err == nil {
				contacts = append(contacts, &user)
			}
		}

		currentUserIcon := ""
		currentUserCustomIcon := ""

		user, err := qdb.GetUserByUsername(c.Context(), username)
		if err == nil {
			if user.Icon.Valid {
				currentUserIcon = user.Icon.String
			}

			if user.CustomIcon.Valid {
				currentUserCustomIcon = user.CustomIcon.String
			}
		}

		return c.Render("dashboard", fiber.Map{
			"Username":              username,
			"CurrentUserIcon":       currentUserIcon,
			"CurrentUserCustomIcon": currentUserCustomIcon,
			"Contacts":              contacts,
		})
	}
}
