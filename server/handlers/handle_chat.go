package handlers

import (
	"exc6/apperrors"
	"exc6/db"
	"exc6/pkg/logger"
	"exc6/services/chat"

	"github.com/gofiber/fiber/v2"
)

func HandleLoadChatWindow(cs *chat.ChatService, db *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		currentUser := c.Locals("username").(string)
		targetUser := c.Params("contact")

		// Validate target user parameter
		if targetUser == "" {
			return apperrors.NewBadRequest("Contact parameter is required")
		}

		history, err := cs.GetHistory(c.Context(), currentUser, targetUser)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"from":   currentUser,
				"to":     targetUser,
				"error":  err.Error(),
			}).Error("Failed to fetch chat history")
			return apperrors.NewInternalError("Failed to load chat history").WithInternal(err)
		}

		// Get contact's user info for icon
		contactUser, err := db.GetUserByUsername(c.Context(), targetUser)
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

		return c.Render("partials/chat-window", fiber.Map{
			"Me":                currentUser,
			"Other":             targetUser,
			"Messages":          history,
			"ContactIcon":       contactIcon,
			"ContactCustomIcon": contactCustomIcon,
		})
	}
}

// HandleSendMessage - don't return HTML, let SSE handle message display
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

		_, err := cs.SendMessage(c.Context(), currentUser, targetUser, content)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"from":  currentUser,
				"to":    targetUser,
				"error": err.Error(),
			}).Error("Failed to send message")
			return apperrors.NewInternalError("Failed to send message").WithInternal(err)
		}

		// Return 200 OK without HTML - SSE will handle displaying the message
		return c.SendStatus(fiber.StatusOK)
	}
}
