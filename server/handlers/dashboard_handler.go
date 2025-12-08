package handlers

import (
	"context"
	"exc6/db"
	"exc6/services/chat"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HandleDashboard renders the main dashboard with contacts list
func HandleDashboard(csrv *chat.ChatService, qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		contactUsernames, err := csrv.GetContacts(username)
		if err != nil {
			return err
		}

		// Convert usernames to contact data with proper string values
		type ContactData struct {
			Username   string
			Icon       string
			CustomIcon string
		}

		contacts := make([]ContactData, 0, len(contactUsernames))
		for _, contactName := range contactUsernames {
			if user, err := qdb.GetUserByUsername(ctx, contactName); err == nil {
				iconValue := ""
				if user.Icon.Valid {
					iconValue = user.Icon.String
				}

				customIconValue := ""
				if user.CustomIcon.Valid {
					customIconValue = user.CustomIcon.String
				}

				contacts = append(contacts, ContactData{
					Username:   user.Username,
					Icon:       iconValue,
					CustomIcon: customIconValue,
				})
			}
		}

		user, err := qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return err
		}

		// Extract string values for current user's icon
		iconValue := ""
		if user.Icon.Valid {
			iconValue = user.Icon.String
		}

		customIconValue := ""
		if user.CustomIcon.Valid {
			customIconValue = user.CustomIcon.String
		}

		return c.Render("dashboard", fiber.Map{
			"Username":   username,
			"Icon":       iconValue,       // Pass string, not sql.NullString
			"CustomIcon": customIconValue, // Pass string, not sql.NullString
			"Contacts":   contacts,
		})
	}
}
