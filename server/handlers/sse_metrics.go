package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"exc6/pkg/metrics"
	"exc6/services/chat"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

func HandleSSEWithMetrics(cs *chat.ChatService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		if username == "" {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		targetContact := c.Params("contact")
		if targetContact == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Contact parameter required")
		}

		lastMessageID := c.Query("lastMessageId", "")

		// ✅ Track connection start time
		connectionStart := time.Now()

		// Create copies to avoid corruption
		usernameCopy := string([]byte(username))
		targetContactCopy := string([]byte(targetContact))

		// Set headers for SSE
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")
		c.Set("Transfer-Encoding", "chunked")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			username := usernameCopy
			targetContact := targetContactCopy

			// ✅ Increment active connections
			metrics.IncrementSSEConnections()
			defer func() {
				metrics.DecrementSSEConnections()

				// Record connection duration
				duration := time.Since(connectionStart).Seconds()
				metrics.RecordSSEConnectionDuration(duration)
			}()

			// ✅ Track reconnections
			if lastMessageID != "" {
				metrics.IncrementSSEReconnections()
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Subscribe to Redis Pub/Sub
			pubsub := cs.SubscribeToMessages(ctx)
			defer pubsub.Close()

			ch := pubsub.Channel()

			// Send connection established event
			sendSSE(w, "connected", `{"status":"connected"}`)

			// If reconnecting, send any missed messages from cache
			if lastMessageID != "" {
				sendMissedMessages(w, cs, username, targetContact, lastMessageID)
			}

			// Keep-alive ticker - send every 15 seconds
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case msg, ok := <-ch:
					if !ok {
						return
					}

					var chatMsg chat.ChatMessage
					if err := json.Unmarshal([]byte(msg.Payload), &chatMsg); err != nil {
						continue
					}

					// Filter messages for this conversation
					isRelevant := (chatMsg.FromID == username && chatMsg.ToID == targetContact) ||
						(chatMsg.FromID == targetContact && chatMsg.ToID == username)

					if isRelevant {
						html := renderMessageHTML(chatMsg, username)
						if !sendSSE(w, "message", html) {
							return
						}
					}

				case <-ticker.C:
					// Send keep-alive ping
					if !sendSSE(w, "ping", `{"time":`+fmt.Sprintf("%d", time.Now().Unix())+`}`) {
						return
					}

				case <-ctx.Done():
					return
				}
			}
		})

		return nil
	}
}
