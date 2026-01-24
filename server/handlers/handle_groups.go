package handlers

import (
	"context"
	"exc6/apperrors"
	"exc6/db"
	"exc6/pkg/logger"
	"exc6/server/websocket"
	"exc6/services/chat"
	"exc6/services/groups"
	"html"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HandleGetGroups renders the groups page
func HandleGetGroups(gsrv *groups.GroupService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		groupsList, err := gsrv.GetUserGroups(ctx, username)
		if err != nil {
			return err
		}

		return c.Render("groups", fiber.Map{
			"Username": username,
			"Groups":   groupsList,
		})
	}
}

// HandleSendGroupMessage sends a message to a group
func HandleSendGroupMessage(csrv *chat.ChatService, gsrv *groups.GroupService, wsManager *websocket.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		groupID := c.Params("groupId")
		content := c.FormValue("content")

		if content == "" {
			return apperrors.NewBadRequest("Message content required")
		}

		if groupID == "" {
			return apperrors.NewBadRequest("Group ID required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// Verify user is member
		_, err = gsrv.GetGroupInfo(ctx, groupID, username)
		if err != nil {
			return err
		}

		// Send message (Persist to DB/Redis)
		msg, err := csrv.SendGroupMessage(ctx, username, groupID, content)
		if err != nil {
			logger.WithError(err).Error("Failed to send group message")
			return apperrors.NewInternalError("Failed to send message").WithInternal(err)
		}

		wsMsg := &websocket.Message{
			Type:      websocket.MessageTypeGroupChat,
			ID:        msg.MessageID,
			From:      msg.FromID,
			GroupID:   msg.GroupID,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
		wsManager.BroadcastToGroup(groupID, wsMsg)

		logger.WithFields(map[string]interface{}{
			"username": username,
			"group_id": groupID,
		}).Debug("Group message sent and broadcasted")

		return c.SendStatus(fiber.StatusOK)
	}
}

// HandleLoadGroupChatIntegrated loads a group chat window (integrated with dashboard)
func HandleLoadGroupChatIntegrated(csrv *chat.ChatService, gsrv *groups.GroupService, qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		groupID := c.Params("groupId")
		if groupID == "" {
			return apperrors.NewBadRequest("Group ID required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Verify user is member
		groupInfo, err := gsrv.GetGroupInfo(ctx, groupID, username)
		if err != nil {
			return err
		}

		// Get message history
		history, err := csrv.GetGroupHistory(ctx, groupID)
		if err != nil {
			logger.WithError(err).Warn("Failed to fetch group history")
			history = []*chat.ChatMessage{}
		}

		// Get CSRF token
		csrfToken := ""
		if token := c.Locals("csrf_token"); token != nil {
			if tokenStr, ok := token.(string); ok {
				csrfToken = tokenStr
			}
		}

		logger.WithFields(map[string]interface{}{
			"username":      username,
			"group_id":      groupID,
			"history_count": len(history),
		}).Debug("Loading group chat")

		// Render the integrated group chat partial
		return c.Render("partials/group-chat-window", fiber.Map{
			"Username":  username,
			"Group":     groupInfo,
			"Messages":  history,
			"CSRFToken": csrfToken,
		})
	}
}

// HandleGroupMembersPartial returns the members list partial
func HandleGroupMembersPartial(gsrv *groups.GroupService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		groupID := c.Params("groupId")

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		groupInfo, err := gsrv.GetGroupInfo(ctx, groupID, username)
		if err != nil {
			return err
		}

		members, err := gsrv.GetGroupMembers(ctx, groupID, username)
		if err != nil {
			return err
		}

		return c.Render("partials/group-members-list", fiber.Map{
			"Group":   groupInfo,
			"Members": members,
		})
	}
}

// HandleAddGroupMemberPartial adds a member and returns updated members list
func HandleAddGroupMemberPartial(gsrv *groups.GroupService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		groupID := c.Params("groupId")
		newMemberUsername := c.FormValue("username")

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err = gsrv.AddMember(ctx, groupID, username, newMemberUsername)
		if err != nil {
			return err
		}

		logger.WithFields(map[string]interface{}{
			"username":   username,
			"group_id":   groupID,
			"new_member": newMemberUsername,
		}).Info("Member added to group")

		// Return updated member list
		members, err := gsrv.GetGroupMembers(ctx, groupID, username)
		if err != nil {
			return err
		}

		groupInfo, _ := gsrv.GetGroupInfo(ctx, groupID, username)

		return c.Render("partials/group-members-list", fiber.Map{
			"Group":   groupInfo,
			"Members": members,
		})
	}
}

// HandleRemoveGroupMemberPartial removes a member and returns updated list
func HandleRemoveGroupMemberPartial(gsrv *groups.GroupService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		groupID := c.Params("groupId")
		targetUsername := c.Params("username")

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err = gsrv.RemoveMember(ctx, groupID, username, targetUsername)
		if err != nil {
			return err
		}

		logger.WithFields(map[string]interface{}{
			"username": username,
			"group_id": groupID,
			"removed":  targetUsername,
		}).Info("Member removed from group")

		// If user removed themselves, redirect to dashboard
		if targetUsername == username {
			c.Set("HX-Redirect", "/dashboard")
			return c.SendStatus(fiber.StatusOK)
		}

		// Return updated member list
		members, err := gsrv.GetGroupMembers(ctx, groupID, username)
		if err != nil {
			return err
		}

		groupInfo, _ := gsrv.GetGroupInfo(ctx, groupID, username)

		return c.Render("partials/group-members-list", fiber.Map{
			"Group":   groupInfo,
			"Members": members,
		})
	}
}

// HandleCreateGroupFromDashboard creates a group and returns success message
func HandleCreateGroupFromDashboard(gsrv *groups.GroupService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		name := c.FormValue("name")
		description := c.FormValue("description")
		icon := c.FormValue("icon")
		if icon == "" {
			icon = "gradient-blue"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		group, err := gsrv.CreateGroup(ctx, username, name, description, icon)
		if err != nil {
			return err
		}

		logger.WithFields(map[string]interface{}{
			"username":   username,
			"group_id":   group.ID,
			"group_name": group.Name,
		}).Info("Group created")

		// Return success message that will trigger page reload
		return c.SendString(`
			<div class="bg-green-500/10 border border-green-500/30 text-green-400 p-4 rounded-xl text-center">
				<p class="font-semibold mb-2">Group Created!</p>
				<p class="text-sm">` + html.EscapeString(group.Name) + ` has been created successfully.</p>
			</div>
		`)
	}
}

// HandleDeleteGroupFromChat deletes group and redirects to dashboard
func HandleDeleteGroupFromChat(gsrv *groups.GroupService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		groupID := c.Params("groupId")

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err = gsrv.DeleteGroup(ctx, groupID, username)
		if err != nil {
			return err
		}

		logger.WithFields(map[string]interface{}{
			"username": username,
			"group_id": groupID,
		}).Info("Group deleted")

		c.Set("HX-Redirect", "/dashboard")
		return c.SendStatus(fiber.StatusOK)
	}
}
