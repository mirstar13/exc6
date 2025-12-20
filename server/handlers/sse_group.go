package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"exc6/db"
	"exc6/pkg/logger"
	"exc6/services/chat"
	"exc6/services/groups"
	"fmt"
	"strings"
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
			logger.WithFields(map[string]interface{}{
				"username": username,
				"group_id": groupID,
				"error":    err.Error(),
			}).Warn("User not authorized for group SSE")
			return c.Status(fiber.StatusForbidden).SendString("Not a member of this group")
		}

		usernameCopy := string([]byte(username))
		groupIDCopy := string([]byte(groupID))

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

			pubsub := csrv.SubscribeToGroup(ctx, groupID)
			defer pubsub.Close()

			ch := pubsub.Channel()

			logger.WithFields(map[string]interface{}{
				"username": username,
				"group_id": groupID,
			}).Info("Group SSE connection established")

			sendSSE(w, "connected", `{"status":"connected"}`)

			if lastMessageID != "" {
				logger.WithFields(map[string]interface{}{
					"username":        username,
					"group_id":        groupID,
					"last_message_id": lastMessageID,
				}).Debug("Sending missed group messages")
				sendMissedGroupMessages(w, csrv, groupID, lastMessageID, username, qdb)
			}

			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case msg, ok := <-ch:
					if !ok {
						logger.WithFields(map[string]interface{}{
							"username": username,
							"group_id": groupID,
						}).Debug("Group SSE channel closed")
						return
					}

					var chatMsg chat.ChatMessage
					if err := json.Unmarshal([]byte(msg.Payload), &chatMsg); err != nil {
						logger.WithError(err).Warn("Failed to unmarshal group message")
						continue
					}

					logger.WithFields(map[string]interface{}{
						"username":   username,
						"group_id":   groupID,
						"from":       chatMsg.FromID,
						"message_id": chatMsg.MessageID,
					}).Debug("Broadcasting group message via SSE")

					html := renderCompactGroupMessageHTML(chatMsg, username, qdb)
					if !sendSSE(w, "message", html) {
						logger.WithFields(map[string]interface{}{
							"username": username,
							"group_id": groupID,
						}).Warn("Failed to send SSE message")
						return
					}

				case <-ticker.C:
					if !sendSSE(w, "ping", `{"time":`+fmt.Sprintf("%d", time.Now().Unix())+`}`) {
						logger.WithFields(map[string]interface{}{
							"username": username,
							"group_id": groupID,
						}).Debug("Group SSE ping failed, closing connection")
						return
					}

				case <-ctx.Done():
					logger.WithFields(map[string]interface{}{
						"username": username,
						"group_id": groupID,
					}).Debug("Group SSE context cancelled")
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
		logger.WithError(err).Warn("Failed to fetch group history for missed messages")
		return
	}

	foundLast := false
	missedCount := 0

	for _, msg := range messages {
		if msg.MessageID == lastMessageID {
			foundLast = true
			continue
		}

		if foundLast {
			html := renderCompactGroupMessageHTML(*msg, currentUser, qdb)
			if sendSSE(w, "message", html) {
				missedCount++
			} else {
				return
			}
		}
	}

	if missedCount > 0 {
		logger.WithFields(map[string]interface{}{
			"group_id":     groupID,
			"missed_count": missedCount,
		}).Debug("Sent missed group messages")
	}
}

// renderCompactGroupMessageHTML generates HTML for messages with proper spacing hints
func renderCompactGroupMessageHTML(msg chat.ChatMessage, currentUser string, qdb *db.Queries) string {
	isMe := msg.FromID == currentUser
	content := escapeHTML(msg.Content)
	timestamp := formatTimestamp(msg.Timestamp)

	var html strings.Builder

	if isMe {
		// My message - right aligned
		html.WriteString(fmt.Sprintf(`<div class="flex w-full justify-end mt-3" data-message-id="%s" data-sender="%s">`,
			msg.MessageID, msg.FromID))
		html.WriteString(`<div class="max-w-[85%] md:max-w-[60%] lg:max-w-[500px] px-4 py-2 text-[15px] leading-relaxed shadow-sm relative bg-signal-blue text-white rounded-2xl rounded-tr-sm" style="word-break: break-word; overflow-wrap: break-word;">`)
		html.WriteString(content)
		html.WriteString(fmt.Sprintf(`<div class="text-[10px] opacity-60 text-right mt-1 select-none text-blue-100">%s</div>`, timestamp))
		html.WriteString(`</div></div>`)
	} else {
		// Get sender icon
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		iconClass := "bg-gradient-to-br from-blue-500 to-blue-700"
		sender, err := qdb.GetUserByUsername(ctx, msg.FromID)
		if err == nil && sender.Icon.Valid {
			iconClass = getIconClass(sender.Icon.String)
		}

		// Other's message - left aligned with avatar and name
		html.WriteString(fmt.Sprintf(`<div class="flex w-full justify-start mt-3" data-message-id="%s" data-sender="%s">`,
			msg.MessageID, msg.FromID))
		html.WriteString(`<div class="flex items-start gap-2 max-w-[85%] md:max-w-[60%] lg:max-w-[500px]">`)

		// Avatar (show for first message in group)
		html.WriteString(fmt.Sprintf(`<div class="w-8 h-8 rounded-full %s flex items-center justify-center text-white font-bold text-xs shrink-0">`, iconClass))
		html.WriteString(string(msg.FromID[0]))
		html.WriteString(`</div>`)

		// Message content with sender name
		html.WriteString(`<div class="flex-1 min-w-0">`)
		html.WriteString(fmt.Sprintf(`<div class="text-xs font-semibold text-signal-blue mb-0.5">%s</div>`, escapeHTML(msg.FromID)))
		html.WriteString(`<div class="px-4 py-2 text-[15px] leading-relaxed shadow-sm relative bg-signal-bubble text-signal-text-main rounded-2xl rounded-tl-sm" style="word-break: break-word; overflow-wrap: break-word;">`)
		html.WriteString(content)
		html.WriteString(fmt.Sprintf(`<div class="text-[10px] opacity-60 text-right mt-1 select-none text-signal-text-sub">%s</div>`, timestamp))
		html.WriteString(`</div></div>`)

		html.WriteString(`</div></div>`)
	}

	return html.String()
}

func getIconClass(icon string) string {
	iconClasses := map[string]string{
		"gradient-blue":   "bg-gradient-to-br from-blue-500 to-blue-700",
		"gradient-purple": "bg-gradient-to-br from-purple-500 to-pink-600",
		"gradient-green":  "bg-gradient-to-br from-green-500 to-emerald-600",
		"gradient-orange": "bg-gradient-to-br from-orange-500 to-red-600",
		"gradient-cyan":   "bg-gradient-to-br from-cyan-500 to-blue-600",
		"gradient-rose":   "bg-gradient-to-br from-rose-500 to-pink-600",
		"gradient-indigo": "bg-gradient-to-br from-indigo-500 to-purple-600",
		"gradient-amber":  "bg-gradient-to-br from-amber-500 to-orange-600",
		"gradient-teal":   "bg-gradient-to-br from-teal-500 to-green-600",
		"gradient-slate":  "bg-gradient-to-br from-slate-600 to-gray-700",
		"solid-signal":    "bg-signal-blue",
		"solid-dark":      "bg-signal-surface border border-white/10",
		"solid-red":       "bg-red-600",
		"solid-emerald":   "bg-emerald-600",
		"solid-violet":    "bg-violet-600",
	}

	if class, ok := iconClasses[icon]; ok {
		return class
	}
	return "bg-gradient-to-br from-blue-500 to-blue-700"
}

func formatTimestamp(timestamp int64) string {
	if timestamp == 0 {
		return "Now"
	}

	t := time.Unix(timestamp, 0)
	now := time.Now()

	// Today - show time
	if t.Day() == now.Day() && t.Month() == now.Month() && t.Year() == now.Year() {
		return t.Format("3:04 PM")
	}

	// Yesterday
	yesterday := now.AddDate(0, 0, -1)
	if t.Day() == yesterday.Day() && t.Month() == yesterday.Month() && t.Year() == yesterday.Year() {
		return "Yesterday"
	}

	// Older - show date
	return t.Format("Jan 2")
}
