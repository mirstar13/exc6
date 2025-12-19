package handlers

import (
	"context"
	"exc6/db"
	"exc6/services/friends"
	"time"

	"github.com/gofiber/fiber/v2"
)

func HandleDashboard(fsrv *friends.FriendService, qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Get only accepted friends
		friends, err := fsrv.GetUserFriends(ctx, username)
		if err != nil {
			return err
		}

		// Get pending friend requests count for notification badge
		requests, err := fsrv.GetFriendRequests(ctx, username)
		requestCount := 0
		if err == nil {
			requestCount = len(requests)
		}

		// Get current user info for icon
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

		// Get CSRF token from context
		csrfToken := ""
		if token := c.Locals("csrf_token"); token != nil {
			csrfToken = token.(string)
		}

		// Convert FriendInfo to ContactData for template
		type ContactData struct {
			Username   string
			Icon       string
			CustomIcon string
		}

		contacts := make([]ContactData, len(friends))
		for i, friend := range friends {
			contacts[i] = ContactData{
				Username:   friend.Username,
				Icon:       friend.Icon,
				CustomIcon: friend.CustomIcon,
			}
		}

		return c.Render("dashboard", fiber.Map{
			"Username":            username,
			"Icon":                iconValue,
			"CustomIcon":          customIconValue,
			"Contacts":            contacts,
			"PendingRequestCount": requestCount,
			"CSRFToken":           csrfToken,
		})
	}
}
