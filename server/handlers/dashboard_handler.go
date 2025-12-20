package handlers

import (
	"context"
	"exc6/db"
	"exc6/pkg/logger"
	"exc6/services/friends"
	"exc6/services/groups"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

func HandleDashboard(fsrv *friends.FriendService, gsrv *groups.GroupService, qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Get friends
		friendsList, err := fsrv.GetUserFriends(ctx, username)
		if err != nil {
			return err
		}

		// Get groups
		groupsList, err := gsrv.GetUserGroups(ctx, username)
		if err != nil {
			// Log but don't fail - groups are optional
			logger.WithError(err).Warn("Failed to fetch user groups")
			groupsList = []groups.GroupInfo{}
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
			if tokenStr, ok := token.(string); ok {
				csrfToken = tokenStr
				logger.WithFields(map[string]interface{}{
					"username":     username,
					"token_length": len(csrfToken),
				}).Info("Dashboard: CSRF token retrieved from locals")
			} else {
				logger.WithFields(map[string]interface{}{
					"username":   username,
					"token_type": fmt.Sprintf("%T", token),
				}).Error("Dashboard: CSRF token in locals is not a string!")
			}
		} else {
			logger.WithField("username", username).Error("Dashboard: CSRF token is nil in locals!")
		}

		// CRITICAL: Log if token is missing
		if csrfToken == "" {
			logger.WithFields(map[string]interface{}{
				"username":   username,
				"session_id": c.Cookies("session_id"),
			}).Error("Dashboard: CSRF token is EMPTY! Template will not render meta tag!")
		}

		// Convert FriendInfo to ContactData
		type ContactData struct {
			Username   string
			Icon       string
			CustomIcon string
			IsGroup    bool
			GroupID    string
		}

		// Combine friends and groups
		contacts := make([]ContactData, 0, len(friendsList)+len(groupsList))

		// Add friends
		for _, friend := range friendsList {
			contacts = append(contacts, ContactData{
				Username:   friend.Username,
				Icon:       friend.Icon,
				CustomIcon: friend.CustomIcon,
				IsGroup:    false,
			})
		}

		// Add groups
		for _, group := range groupsList {
			contacts = append(contacts, ContactData{
				Username:   group.Name,
				Icon:       group.Icon,
				CustomIcon: group.CustomIcon,
				IsGroup:    true,
				GroupID:    group.ID,
			})
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
