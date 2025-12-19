package handlers

import (
	"context"
	"exc6/apperrors"
	"exc6/db"
	"exc6/services/chat"
	"exc6/services/groups"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HandleCreateGroup creates a new group
func HandleCreateGroup(gsrv *groups.GroupService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		name := c.FormValue("name")
		description := c.FormValue("description")
		icon := c.FormValue("icon", "gradient-blue")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		group, err := gsrv.CreateGroup(ctx, username, name, description, icon)
		if err != nil {
			return err
		}

		// Return success message
		return c.Render("partials/group-created", fiber.Map{
			"Group": group,
		})
	}
}

// HandleGetGroups returns user's groups
func HandleGetGroups(gsrv *groups.GroupService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		groups, err := gsrv.GetUserGroups(ctx, username)
		if err != nil {
			return err
		}

		return c.Render("groups", fiber.Map{
			"Username": username,
			"Groups":   groups,
		})
	}
}

// HandleLoadGroupChat loads a group chat window
func HandleLoadGroupChat(csrv *chat.ChatService, gsrv *groups.GroupService, qdb *db.Queries) fiber.Handler {
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
			history = []*chat.ChatMessage{}
		}

		// Get current user info for icon
		user, _ := qdb.GetUserByUsername(ctx, username)
		userIcon := ""
		if user.Icon.Valid {
			userIcon = user.Icon.String
		}

		// Get CSRF token
		csrfToken := ""
		if token := c.Locals("csrf_token"); token != nil {
			if tokenStr, ok := token.(string); ok {
				csrfToken = tokenStr
			}
		}

		return c.Render("partials/group-chat-window", fiber.Map{
			"Username":  username,
			"UserIcon":  userIcon,
			"Group":     groupInfo,
			"Messages":  history,
			"CSRFToken": csrfToken,
		})
	}
}

// HandleSendGroupMessage sends a message to a group
func HandleSendGroupMessage(csrv *chat.ChatService, gsrv *groups.GroupService) fiber.Handler {
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

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// Verify user is member
		_, err = gsrv.GetGroupInfo(ctx, groupID, username)
		if err != nil {
			return err
		}

		// Send message
		_, err = csrv.SendGroupMessage(ctx, username, groupID, content)
		if err != nil {
			return apperrors.NewInternalError("Failed to send message").WithInternal(err)
		}

		return c.SendStatus(fiber.StatusOK)
	}
}

// HandleGroupMembers shows group members
func HandleGroupMembers(gsrv *groups.GroupService) fiber.Handler {
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

		return c.Render("partials/group-members", fiber.Map{
			"Group":   groupInfo,
			"Members": members,
		})
	}
}

// HandleAddGroupMember adds a member to the group
func HandleAddGroupMember(gsrv *groups.GroupService) fiber.Handler {
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

		// Return updated member list
		members, err := gsrv.GetGroupMembers(ctx, groupID, username)
		if err != nil {
			return err
		}

		groupInfo, _ := gsrv.GetGroupInfo(ctx, groupID, username)

		return c.Render("partials/group-members", fiber.Map{
			"Group":   groupInfo,
			"Members": members,
		})
	}
}

// HandleRemoveGroupMember removes a member from the group
func HandleRemoveGroupMember(gsrv *groups.GroupService) fiber.Handler {
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

		// If user removed themselves, redirect to groups
		if targetUsername == username {
			c.Set("HX-Redirect", "/groups")
			return c.SendStatus(fiber.StatusOK)
		}

		// Return updated member list
		members, err := gsrv.GetGroupMembers(ctx, groupID, username)
		if err != nil {
			return err
		}

		groupInfo, _ := gsrv.GetGroupInfo(ctx, groupID, username)

		return c.Render("partials/group-members", fiber.Map{
			"Group":   groupInfo,
			"Members": members,
		})
	}
}

// HandleDeleteGroup deletes a group
func HandleDeleteGroup(gsrv *groups.GroupService) fiber.Handler {
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

		c.Set("HX-Redirect", "/groups")
		return c.SendStatus(fiber.StatusOK)
	}
}
