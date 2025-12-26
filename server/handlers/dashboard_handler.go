package handlers

import (
	"context"
	"exc6/db"
	"exc6/pkg/logger"
	"exc6/services/calls"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/groups"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Reusable function to get notifications
func getNotificationData(ctx context.Context, username string, fsrv *friends.FriendService, cs *chat.ChatService, callSrv *calls.CallService) (fiber.Map, int) {
	// 1. Friend Requests
	requests, err := fsrv.GetFriendRequests(ctx, username)
	if err != nil {
		requests = []friends.FriendInfo{}
	}

	// 2. Unread Messages
	unreadMap, err := cs.GetUnreadMessages(ctx, username)
	if err != nil {
		unreadMap = make(map[string]int)
	}

	// 3. Missed Calls
	missedCalls, err := callSrv.GetMissedCalls(ctx, username)
	if err != nil {
		missedCalls = []*calls.Call{}
	}

	total := len(requests) + len(unreadMap) + len(missedCalls)

	return fiber.Map{
		"Notifications":  requests,
		"UnreadMessages": unreadMap,
		"MissedCalls":    missedCalls,
	}, total
}

func HandleDashboard(fsrv *friends.FriendService, gsrv *groups.GroupService, cs *chat.ChatService, callSrv *calls.CallService, qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Get Friends & Groups
		friendsList, err := fsrv.GetUserFriends(ctx, username)
		if err != nil {
			return err
		}
		groupsList, err := gsrv.GetUserGroups(ctx, username)
		if err != nil {
			groupsList = []groups.GroupInfo{}
		}

		// Get Notifications
		notifData, totalNotifications := getNotificationData(ctx, username, fsrv, cs, callSrv)

		// Get user info
		user, err := qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return err
		}

		iconValue := ""
		if user.Icon.Valid {
			iconValue = user.Icon.String
		}
		customIconValue := ""
		if user.CustomIcon.Valid {
			customIconValue = user.CustomIcon.String
		}

		csrfToken := ""
		if token := c.Locals("csrf_token"); token != nil {
			if tokenStr, ok := token.(string); ok {
				csrfToken = tokenStr
			}
		}

		// Contacts logic
		type ContactData struct {
			Username    string
			Icon        string
			CustomIcon  string
			IsGroup     bool
			GroupID     string
			UnreadCount int
		}
		contacts := make([]ContactData, 0, len(friendsList)+len(groupsList))
		unreadMap := notifData["UnreadMessages"].(map[string]int)

		for _, friend := range friendsList {
			contacts = append(contacts, ContactData{
				Username:    friend.Username,
				Icon:        friend.Icon,
				CustomIcon:  friend.CustomIcon,
				IsGroup:     false,
				UnreadCount: unreadMap[friend.Username],
			})
		}
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
			"PendingRequestCount": totalNotifications,
			"Notifications":       notifData["Notifications"],
			"MissedCalls":         notifData["MissedCalls"],
			"UnreadMessages":      notifData["UnreadMessages"],
			"CSRFToken":           csrfToken,
		})
	}
}

// HandleGetNotifications returns just the notification list HTML
func HandleGetNotifications(fsrv *friends.FriendService, cs *chat.ChatService, callSrv *calls.CallService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		notifData, total := getNotificationData(ctx, username, fsrv, cs, callSrv)

		// Also send the count header so HTMX can update the badge if we wanted to use OOB
		c.Set("X-Notification-Count", fmt.Sprintf("%d", total))

		return c.Render("partials/notifications", notifData)
	}
}

// HandleMarkNotificationsRead clears notifications
func HandleMarkNotificationsRead(cs *chat.ChatService, callSrv *calls.CallService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// 1. Mark all chats as read
		if err := cs.MarkAllRead(ctx, username); err != nil {
			logger.WithError(err).Error("Failed to mark chats read")
		}

		// 2. Mark calls as seen
		if err := callSrv.MarkCallsSeen(ctx, username); err != nil {
			logger.WithError(err).Error("Failed to mark calls seen")
		}

		// Return empty notification list to clear the UI immediately
		return c.Render("partials/notifications", fiber.Map{
			"Notifications":  []friends.FriendInfo{},
			"UnreadMessages": map[string]int{},
			"MissedCalls":    []*calls.Call{},
		})
	}
}
