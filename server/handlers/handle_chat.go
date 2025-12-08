package handlers

import (
	"exc6/db"
	"exc6/services/chat"
	"log"

	"github.com/gofiber/fiber/v2"
)

func HandleLoadChatWindow(cs *chat.ChatService, udb *db.UsersDB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		currentUser := c.Locals("username").(string)
		targetUser := c.Params("contact")

		history, err := cs.GetHistory(c.Context(), currentUser, targetUser)
		if err != nil {
			return c.Status(500).SendString("Error fetching chat")
		}

		// Get contact's user info for icon
		contactUser := udb.FindUserByUsername(targetUser)
		contactIcon := ""
		contactCustomIcon := ""
		if contactUser != nil {
			contactIcon = contactUser.Icon
			contactCustomIcon = contactUser.CustomIcon
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

		if content == "" {
			return c.SendStatus(fiber.StatusBadRequest)
		}

		_, err := cs.SendMessage(c.Context(), currentUser, targetUser, content)
		if err != nil {
			log.Printf("Error sending message: %v", err)
			return c.Status(500).SendString("Error sending message")
		}

		// Return 200 OK without HTML - SSE will handle displaying the message
		return c.SendStatus(fiber.StatusOK)
	}
}
