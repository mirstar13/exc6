package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"exc6/services/chat"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

func HandleSSE(cs *chat.ChatService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		if username == "" {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		targetContact := c.Params("contact")
		if targetContact == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Contact required")
		}

		// Get last message ID from query param for reconnection
		lastMessageID := c.Query("lastMessageId", "")

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
			// CRITICAL: Remove ALL timeouts for SSE connections
			if conn := c.Context().Conn(); conn != nil {
				// Set to zero value = no deadline
				conn.SetReadDeadline(time.Time{})
				conn.SetWriteDeadline(time.Time{})
			}

			username := usernameCopy
			targetContact := targetContactCopy

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

// sendSSE sends a Server-Sent Event and returns false if it fails
func sendSSE(w *bufio.Writer, event, data string) bool {
	// Format: event: <event>\ndata: <data>\n\n
	_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if err != nil {
		return false
	}

	// Flush immediately - critical for SSE
	err = w.Flush()
	return err == nil
}

// sendMissedMessages sends any messages that were missed during disconnection
func sendMissedMessages(w *bufio.Writer, cs *chat.ChatService, username, targetContact, lastMessageID string) {
	ctx := context.Background()

	// Get recent message history
	messages, err := cs.GetHistory(ctx, username, targetContact)
	if err != nil {
		return
	}

	// Find messages after lastMessageID
	foundLast := false
	missedCount := 0

	for _, msg := range messages {
		if msg.MessageID == lastMessageID {
			foundLast = true
			continue
		}

		// Send all messages after the last one
		if foundLast {
			html := renderMessageHTML(*msg, username)
			if sendSSE(w, "message", html) {
				missedCount++
			} else {
				return
			}
		}
	}

}

func renderMessageHTML(msg chat.ChatMessage, currentUser string) string {
	isMe := msg.FromID == currentUser

	justify := "justify-start"
	bubbleClass := "bg-signal-bubble text-signal-text-main rounded-2xl rounded-tl-sm"
	timeClass := "text-signal-text-sub"

	if isMe {
		justify = "justify-end"
		bubbleClass = "bg-signal-blue text-white rounded-2xl rounded-tr-sm"
		timeClass = "text-blue-100"
	}

	content := escapeHTML(msg.Content)

	// Single-line HTML for SSE (no newlines)
	html := fmt.Sprintf(`<div class="flex w-full mb-1 group %s" data-message-id="%s"><div class="max-w-[85%%] md:max-w-[60%%] lg:max-w-[500px] px-4 py-2 text-[15px] leading-relaxed shadow-sm relative %s" style="word-break: break-word; overflow-wrap: break-word;">%s<div class="text-[10px] opacity-60 text-right mt-1 select-none %s">Now</div></div></div>`,
		justify, msg.MessageID, bubbleClass, content, timeClass)

	html = strings.ReplaceAll(html, "\n", "")
	html = strings.ReplaceAll(html, "\r", "")

	return html
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
