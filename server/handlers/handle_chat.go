package handlers

import (
	"context"
	"exc6/apperrors"
	"exc6/db"
	"exc6/pkg/logger"
	"exc6/services/chat"
	"time"

	"github.com/gofiber/fiber/v2"
)

func HandleLoadChatWindow(cs *chat.ChatService, qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		currentUser := c.Locals("username").(string)
		targetUser := c.Params("contact")

		// Validate target user parameter
		if targetUser == "" {
			return apperrors.NewBadRequest("Contact parameter is required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Mark conversation as read
		if err := cs.MarkConversationRead(ctx, currentUser, targetUser); err != nil {
			logger.WithError(err).Warn("Failed to mark conversation as read")
		}

		history, err := cs.GetHistory(ctx, currentUser, targetUser)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"from":  currentUser,
				"to":    targetUser,
				"error": err.Error(),
			}).Error("Failed to fetch chat history")
			return apperrors.NewInternalError("Failed to load chat history").WithInternal(err)
		}

		// Get contact's user info for icon
		contactUser, err := qdb.GetUserByUsername(ctx, targetUser)
		contactIcon := ""
		contactCustomIcon := ""
		if err == nil {
			if contactUser.Icon.Valid {
				contactIcon = contactUser.Icon.String
			}

			if contactUser.CustomIcon.Valid {
				contactCustomIcon = contactUser.CustomIcon.String
			}
		}

		// Get CSRF token from context
		csrfToken := ""
		if token := c.Locals("csrf_token"); token != nil {
			if tokenStr, ok := token.(string); ok {
				csrfToken = tokenStr
			}
		}

		if csrfToken == "" {
			logger.WithFields(map[string]interface{}{
				"from": currentUser,
				"to":   targetUser,
			}).Warn("CSRF token missing in chat window render")
		}

		return c.Render("partials/chat-window", fiber.Map{
			"Me":                currentUser,
			"Other":             targetUser,
			"Messages":          history,
			"ContactIcon":       contactIcon,
			"ContactCustomIcon": contactCustomIcon,
			"CSRFToken":         csrfToken,
		})
	}
}

// HandleSendMessage - don't return HTML, let WebSocket handle message display
func HandleSendMessage(cs *chat.ChatService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		currentUser := c.Locals("username").(string)
		targetUser := c.Params("contact")
		content := c.FormValue("content")

		// Validate inputs
		if content == "" {
			return apperrors.NewBadRequest("Message content cannot be empty")
		}

		if targetUser == "" {
			return apperrors.NewBadRequest("Target user is required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, err := cs.SendMessage(ctx, currentUser, targetUser, content)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"from":  currentUser,
				"to":    targetUser,
				"error": err.Error(),
			}).Error("Failed to send message")
			return apperrors.NewInternalError("Failed to send message").WithInternal(err)
		}

		// Return 200 OK without HTML - WebSocket will handle displaying the message via Redis Pub/Sub
		return c.SendStatus(fiber.StatusOK)
	}
}
