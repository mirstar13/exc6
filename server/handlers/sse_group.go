package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"exc6/db"
	"exc6/services/chat"
	"exc6/services/groups"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

func HandleGroupSSE(csrv *chat.ChatService, gsrv *groups.GroupService, qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		if username == "" {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		groupID := c.Params("groupId")
		if groupID == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Group ID required")
		}

		lastMessageID := c.Query("lastMessageId", "")

		// Verify user is member
		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, err := gsrv.GetGroupInfo(verifyCtx, groupID, username)
		verifyCancel()
		if err != nil {
			return c.Status(fiber.StatusForbidden).SendString("Not a member of this group")
		}

		// Create copies to avoid corruption
		usernameCopy := string([]byte(username))
		groupIDCopy := string([]byte(groupID))

		// Set headers for SSE
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")
		c.Set("Transfer-Encoding", "chunked")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			username := usernameCopy
			groupID := groupIDCopy

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Subscribe to group messages
			pubsub := csrv.SubscribeToGroup(ctx, groupID)
			defer pubsub.Close()

			ch := pubsub.Channel()

			// Send connection established event
			sendSSE(w, "connected", `{"status":"connected"}`)

			// If reconnecting, send any missed messages
			if lastMessageID != "" {
				sendMissedGroupMessages(w, csrv, groupID, lastMessageID, username, qdb)
			}

			// Keep-alive ticker
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

					// Render message with sender info
					html := renderGroupMessageHTML(chatMsg, username, qdb)
					if !sendSSE(w, "message", html) {
						return
					}

				case <-ticker.C:
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

func sendMissedGroupMessages(w *bufio.Writer, cs *chat.ChatService, groupID, lastMessageID, currentUser string, qdb *db.Queries) {
	ctx := context.Background()

	messages, err := cs.GetGroupHistory(ctx, groupID)
	if err != nil {
		return
	}

	foundLast := false
	for _, msg := range messages {
		if msg.MessageID == lastMessageID {
			foundLast = true
			continue
		}

		if foundLast {
			html := renderGroupMessageHTML(*msg, currentUser, qdb)
			if !sendSSE(w, "message", html) {
				return
			}
		}
	}
}

func renderGroupMessageHTML(msg chat.ChatMessage, currentUser string, qdb *db.Queries) string {
	isMe := msg.FromID == currentUser

	// Get sender info
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	sender, err := qdb.GetUserByUsername(ctx, msg.FromID)
	senderIcon := ""
	if err == nil && sender.Icon.Valid {
		senderIcon = sender.Icon.String
	}

	_ = senderIcon // Currently unused, but could be used for displaying sender icons

	// Build HTML
	var html string
	if isMe {
		html = fmt.Sprintf(`<div class="flex w-full mb-1 group justify-end" data-message-id="%s">
			<div class="max-w-[85%%] md:max-w-[60%%] lg:max-w-[500px] px-4 py-2 text-[15px] leading-relaxed shadow-sm relative bg-signal-blue text-white rounded-2xl rounded-tr-sm" style="word-break: break-word; overflow-wrap: break-word;">
				%s
				<div class="text-[10px] opacity-60 text-right mt-1 select-none text-blue-100">Now</div>
			</div>
		</div>`, msg.MessageID, escapeHTML(msg.Content))
	} else {
		// Show sender name for group messages
		html = fmt.Sprintf(`<div class="flex w-full mb-1 group justify-start" data-message-id="%s">
			<div class="max-w-[85%%] md:max-w-[60%%] lg:max-w-[500px] px-4 py-2 text-[15px] leading-relaxed shadow-sm relative bg-signal-bubble text-signal-text-main rounded-2xl rounded-tl-sm" style="word-break: break-word; overflow-wrap: break-word;">
				<div class="text-xs font-semibold text-signal-blue mb-1">%s</div>
				%s
				<div class="text-[10px] opacity-60 text-right mt-1 select-none text-signal-text-sub">Now</div>
			</div>
		</div>`, msg.MessageID, msg.FromID, escapeHTML(msg.Content))
	}

	return html
}
