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

		// Get the target contact from URL parameter
		targetContact := c.Params("contact")
		if targetContact == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Contact required")
		}

		// Create copies of these strings to avoid them being corrupted
		// when the Fiber context is reused for other requests
		usernameCopy := string([]byte(username))
		targetContactCopy := string([]byte(targetContact))

		// Set headers for SSE
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			// Use the copied variables, not the originals
			username := usernameCopy
			targetContact := targetContactCopy
			// Create a context that we can cancel
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Subscribe to Redis Pub/Sub
			pubsub := cs.SubscribeToMessages(ctx)
			defer pubsub.Close()

			ch := pubsub.Channel()

			// Send initial connection message
			fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
			if err := w.Flush(); err != nil {
				return
			}

			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case msg, ok := <-ch:
					if !ok {
						return
					}

					// Parse message
					var chatMsg chat.ChatMessage
					if err := json.Unmarshal([]byte(msg.Payload), &chatMsg); err != nil {
						continue
					}

					// Only send messages for THIS specific chat
					isRelevant := (chatMsg.FromID == username && chatMsg.ToID == targetContact) ||
						(chatMsg.FromID == targetContact && chatMsg.ToID == username)

					if isRelevant {
						// Render the message HTML
						html := renderMessageHTML(chatMsg, username)

						// Send as SSE - ensure single line for proper SSE format
						fmt.Fprintf(w, "event: message\ndata: %s\n\n", html)
						if err := w.Flush(); err != nil {
							return
						}
					}

				case <-ticker.C:
					// Send keepalive ping
					fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
					if err := w.Flush(); err != nil {
						return
					}
				}
			}
		})

		return nil
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

	// Escape HTML in content
	content := escapeHTML(msg.Content)

	// Create HTML as a single line to ensure proper SSE format
	html := fmt.Sprintf(`<div class="flex w-full mb-1 group %s" data-message-id="%s"><div class="max-w-[85%%] md:max-w-[60%%] lg:max-w-[500px] px-4 py-2 text-[15px] leading-relaxed shadow-sm relative %s" style="word-break: break-word; overflow-wrap: break-word;">%s<div class="text-[10px] opacity-60 text-right mt-1 select-none %s">Now</div></div></div>`,
		justify, msg.MessageID, bubbleClass, content, timeClass)

	// Remove any newlines that might have been added
	html = strings.ReplaceAll(html, "\n", "")
	html = strings.ReplaceAll(html, "\r", "")

	return html
}

func escapeHTML(s string) string {
	// Simple HTML escaping
	s = replaceAll(s, "&", "&amp;")
	s = replaceAll(s, "<", "&lt;")
	s = replaceAll(s, ">", "&gt;")
	s = replaceAll(s, "\"", "&quot;")
	s = replaceAll(s, "'", "&#39;")
	return s
}

func replaceAll(s, old, new string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		if i <= len(s)-len(old) && s[i:i+len(old)] == old {
			result += new
			i += len(old) - 1
		} else {
			result += string(s[i])
		}
	}
	return result
}
