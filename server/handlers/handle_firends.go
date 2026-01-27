package handlers

import (
	"context"
	"exc6/apperrors"
	"exc6/server/websocket"
	"exc6/services/friends"
	"html"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HandleFriendsPage renders the friend management page
func HandleFriendsPage(fsrv *friends.FriendService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Get friends list
		friends, err := fsrv.GetUserFriends(ctx, username)
		if err != nil {
			return err
		}

		// Get pending requests
		requests, err := fsrv.GetFriendRequests(ctx, username)
		if err != nil {
			return err
		}

		return c.Render("friends", fiber.Map{
			"Username": username,
			"Friends":  friends,
			"Requests": requests,
		})
	}
}

// HandleSearchUsers searches for users to add as friends
func HandleSearchUsers(fsrv *friends.FriendService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		query := c.Query("q", "")
		if query == "" {
			return c.Render("partials/user-search-results", fiber.Map{
				"Results": []friends.FriendInfo{},
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		results, err := fsrv.SearchUsers(ctx, username, query)
		if err != nil {
			return err
		}

		return c.Render("partials/user-search-results", fiber.Map{
			"Results": results,
		})
	}
}

// HandleSendFriendRequest sends a friend request
func HandleSendFriendRequest(fsrv *friends.FriendService, wsManager *websocket.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		targetUsername := c.Params("username")
		if targetUsername == "" {
			return apperrors.NewBadRequest("Username parameter required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := fsrv.SendFriendRequest(ctx, username, targetUsername); err != nil {
			return err
		}

		// Send real-time notification
		wsManager.SendToUser(targetUsername, &websocket.Message{
			Type:      websocket.MessageTypeNotification,
			From:      username,
			To:        targetUsername,
			Content:   "New friend request",
			Timestamp: time.Now().Unix(),
		})

		// Return success message
		return c.SendString(`
			<div class="bg-green-500/10 border border-green-500/30 text-green-400 p-3 rounded-xl text-sm animate-fade-in">
				Friend request sent to ` + html.EscapeString(targetUsername) + `
			</div>
		`)
	}
}

// HandleAcceptFriendRequest accepts a friend request
func HandleAcceptFriendRequest(fsrv *friends.FriendService, wsManager *websocket.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		requesterUsername := c.Params("username")
		if requesterUsername == "" {
			return apperrors.NewBadRequest("Username parameter required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := fsrv.AcceptFriendRequest(ctx, username, requesterUsername); err != nil {
			return err
		}

		// Send real-time notification to the requester
		wsManager.SendToUser(requesterUsername, &websocket.Message{
			Type:      websocket.MessageTypeNotification,
			From:      username,
			To:        requesterUsername,
			Content:   "Friend request accepted",
			Timestamp: time.Now().Unix(),
		})

		// Reload the friend requests list
		requests, err := fsrv.GetFriendRequests(ctx, username)
		if err != nil {
			return err
		}

		return c.Render("partials/friend-requests", fiber.Map{
			"Requests": requests,
		})
	}
}

// HandleRejectFriendRequest rejects/removes a friend request
func HandleRejectFriendRequest(fsrv *friends.FriendService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		requesterUsername := c.Params("username")
		if requesterUsername == "" {
			return apperrors.NewBadRequest("Username parameter required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := fsrv.RemoveFriend(ctx, username, requesterUsername); err != nil {
			return err
		}

		// Reload the friend requests list
		requests, err := fsrv.GetFriendRequests(ctx, username)
		if err != nil {
			return err
		}

		return c.Render("partials/friend-requests", fiber.Map{
			"Requests": requests,
		})
	}
}

// HandleRemoveFriend removes a friend
func HandleRemoveFriend(fsrv *friends.FriendService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		friendUsername := c.Params("username")
		if friendUsername == "" {
			return apperrors.NewBadRequest("Username parameter required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := fsrv.RemoveFriend(ctx, username, friendUsername); err != nil {
			return err
		}

		// Reload the friends list
		friends, err := fsrv.GetUserFriends(ctx, username)
		if err != nil {
			return err
		}

		return c.Render("partials/friends-list", fiber.Map{
			"Friends": friends,
		})
	}
}
