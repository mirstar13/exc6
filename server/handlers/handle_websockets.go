package handlers

import (
	"context"
	"encoding/json"
	"exc6/apperrors"
	"exc6/db"
	"exc6/pkg/logger"
	_websocket "exc6/server/websocket"
	"exc6/services/calls"
	"exc6/services/chat"
	"exc6/services/groups"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// HandleWebSocketUpgrade upgrades HTTP connection to WebSocket
func HandleWebSocketUpgrade(wsManager *_websocket.Manager, csrv *chat.ChatService, callService *calls.CallService, gsrv *groups.GroupService, qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			// Validate Origin header
			origin := c.Get("Origin")
			if !isAllowedOrigin(origin) {
				return fiber.ErrForbidden
			}
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	}
}

func isAllowedOrigin(origin string) bool {
	// List of allowed origins for WebSocket connections
	allowedOrigins := []string{
		"http://localhost:3000",
		"http://localhost:8080",
		"http://localhost:8000",
		"https://yourdomain.com",
	}

	for _, allowed := range allowedOrigins {
		if origin == allowed {
			return true
		}
	}

	return false
}

// HandleWebSocket handles WebSocket connections for chat and calls
func HandleWebSocket(wsManager *_websocket.Manager, csrv *chat.ChatService, callService *calls.CallService, gsrv *groups.GroupService, qdb *db.Queries) fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		// Get username from locals (set by auth middleware)
		username := conn.Locals("username").(string)

		// Create client
		client := _websocket.NewClient(username, conn, wsManager)

		// Register client
		wsManager.Register <- client

		// Fetch user's groups to filter incoming messages
		ctxGroups, cancelGroups := context.WithTimeout(context.Background(), 5*time.Second)
		userGroups, err := gsrv.GetUserGroups(ctxGroups, username)
		cancelGroups()

		allowedGroups := make(map[string]bool)
		if err == nil {
			for _, g := range userGroups {
				allowedGroups[g.ID] = true
			}
		} else {
			logger.WithError(err).Warn("Failed to fetch user groups for WebSocket")
		}

		// Subscribe to Redis Pub/Sub for chat messages
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pubsub := csrv.SubscribeToMessages(ctx)
		defer pubsub.Close()

		// Start message relay from Redis to WebSocket
		go relayRedisToWebSocket(ctx, client, pubsub, username, allowedGroups, qdb)

		// Start read and write pumps
		go client.WritePump()
		client.ReadPump() // Blocks until connection closes

		logger.WithField("username", username).Info("WebSocket connection closed")
	})
}

// relayRedisToWebSocket relays messages from Redis Pub/Sub to WebSocket
func relayRedisToWebSocket(ctx context.Context, client *_websocket.Client, pubsub *redis.PubSub, username string, allowedGroups map[string]bool, qdb *db.Queries) {
	ch := pubsub.Channel()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}

			var chatMsg chat.ChatMessage
			if err := json.Unmarshal([]byte(msg.Payload), &chatMsg); err != nil {
				logger.WithError(err).Warn("Failed to unmarshal chat message")
				continue
			}

			// Filter messages for this user
			// 1. Direct messages where user is sender or recipient
			// 2. Group messages where user is a member of the group
			isRelevant := (chatMsg.FromID == username || chatMsg.ToID == username) ||
				(chatMsg.IsGroup && allowedGroups[chatMsg.GroupID])

			if !isRelevant {
				continue
			}

			// Convert to WebSocket message
			wsMsg := &_websocket.Message{
				Type:      _websocket.MessageTypeChat,
				ID:        chatMsg.MessageID,
				From:      chatMsg.FromID,
				To:        chatMsg.ToID,
				GroupID:   chatMsg.GroupID,
				Content:   chatMsg.Content,
				Timestamp: chatMsg.Timestamp,
			}

			if chatMsg.IsGroup {
				wsMsg.Type = _websocket.MessageTypeGroupChat

				// Enrich group message with sender info (icon) for the frontend
				if chatMsg.FromID != username {
					fetchCtx, fetchCancel := context.WithTimeout(ctx, 2*time.Second)
					sender, err := qdb.GetUserByUsername(fetchCtx, chatMsg.FromID)
					fetchCancel()

					if err == nil {
						data := map[string]interface{}{
							"icon":        "",
							"custom_icon": "",
						}
						if sender.Icon.Valid {
							data["icon"] = sender.Icon.String
						}
						if sender.CustomIcon.Valid {
							data["custom_icon"] = sender.CustomIcon.String
						}
						wsMsg.Data = data
					}
				}
			}

			// Send to client
			if err := client.SendMessage(wsMsg); err != nil {
				logger.WithError(err).Warn("Failed to send message to WebSocket client")
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// HandleCallInitiate initiates a voice call
func HandleCallInitiate(callService *calls.CallService, wsManager *_websocket.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		caller, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		callee := c.Params("username")
		if callee == "" {
			return apperrors.NewBadRequest("Callee username required")
		}

		if caller == callee {
			return apperrors.NewBadRequest("Cannot call yourself")
		}

		// Check if callee is online
		if !wsManager.IsUserOnline(callee) {
			return apperrors.NewBadRequest("User is offline")
		}

		// Check if either user is already in a call
		if callService.IsUserInCall(caller) {
			return apperrors.NewBadRequest("You are already in a call")
		}
		if callService.IsUserInCall(callee) {
			return apperrors.NewBadRequest("User is already in a call")
		}

		// Initiate call
		call, err := callService.InitiateCall(caller, callee)
		if err != nil {
			return apperrors.NewInternalError("Failed to initiate call").WithInternal(err)
		}

		// Update call state to ringing
		callService.UpdateCallState(call.ID, calls.CallStateRinging)

		return c.JSON(fiber.Map{
			"call_id": call.ID,
			"status":  "ringing",
		})
	}
}

// HandleCallAnswer answers an incoming call
func HandleCallAnswer(callService *calls.CallService, wsManager *_websocket.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		callID := c.Params("call_id")
		if callID == "" {
			return apperrors.NewBadRequest("Call ID required")
		}

		// Answer the call
		if err := callService.AnswerCall(callID, username); err != nil {
			return apperrors.NewBadRequest(err.Error())
		}

		call, _ := callService.GetCall(callID)

		// Notify caller that call was answered
		answerMsg := &_websocket.Message{
			Type: _websocket.MessageTypeCallAnswer,
			ID:   callID,
			From: username,
			To:   call.Caller,
			Data: map[string]interface{}{
				"call_id":  callID,
				"accepted": true,
			},
			Timestamp: time.Now().Unix(),
		}

		wsManager.SendToUser(call.Caller, answerMsg)

		return c.JSON(fiber.Map{
			"call_id": callID,
			"status":  "active",
		})
	}
}

// HandleCallEnd ends an active call
func HandleCallEnd(callService *calls.CallService, wsManager *_websocket.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		callID := c.Params("call_id")
		if callID == "" {
			return apperrors.NewBadRequest("Call ID required")
		}

		call, err := callService.GetCall(callID)
		if err != nil {
			return apperrors.NewBadRequest("Call not found")
		}

		// End the call
		if err := callService.EndCall(callID, username); err != nil {
			return apperrors.NewBadRequest(err.Error())
		}

		// Notify other party
		otherParty := call.Caller
		if otherParty == username {
			otherParty = call.Callee
		}

		endMsg := &_websocket.Message{
			Type: _websocket.MessageTypeCallEnd,
			ID:   callID,
			From: username,
			To:   otherParty,
			Data: map[string]interface{}{
				"call_id":  callID,
				"ended_by": username,
			},
			Timestamp: time.Now().Unix(),
		}

		wsManager.SendToUser(otherParty, endMsg)

		return c.JSON(fiber.Map{
			"call_id": callID,
			"status":  "ended",
		})
	}
}

// HandleCallReject rejects an incoming call
func HandleCallReject(callService *calls.CallService, wsManager *_websocket.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		callID := c.Params("call_id")
		if callID == "" {
			return apperrors.NewBadRequest("Call ID required")
		}

		call, err := callService.GetCall(callID)
		if err != nil {
			return apperrors.NewBadRequest("Call not found")
		}

		if call.Callee != username {
			return apperrors.NewBadRequest("You are not the callee")
		}

		// End the call
		if err := callService.EndCall(callID, username); err != nil {
			return apperrors.NewBadRequest(err.Error())
		}

		// Notify caller
		rejectMsg := &_websocket.Message{
			Type: _websocket.MessageTypeCallEnd,
			ID:   callID,
			From: username,
			To:   call.Caller,
			Data: map[string]interface{}{
				"call_id":  callID,
				"rejected": true,
			},
			Timestamp: time.Now().Unix(),
		}

		wsManager.SendToUser(call.Caller, rejectMsg)

		return c.JSON(fiber.Map{
			"call_id": callID,
			"status":  "rejected",
		})
	}
}

// HandleCallHistory returns call history for a user
func HandleCallHistory(callService *calls.CallService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		limit := c.QueryInt("limit", 20)
		if limit > 100 {
			limit = 100
		}

		history, err := callService.GetCallHistory(username, limit)
		if err != nil {
			return apperrors.NewInternalError("Failed to retrieve call history").WithInternal(err)
		}

		return c.JSON(fiber.Map{
			"calls": history,
		})
	}
}
