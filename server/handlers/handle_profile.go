package handlers

import (
	"exc6/db"
	"exc6/services/chat"
	"exc6/services/sessions"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HandleUserProfileUpdate handles profile updates with secure file uploads
func HandleUserProfileUpdate(udb *db.UsersDB, smngr *sessions.SessionManager) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		oldUsername := ctx.Locals("username").(string)
		user := udb.FindUserByUsername(oldUsername)
		if user == nil {
			return ctx.Status(fiber.StatusNotFound).SendString("User not found")
		}

		// Extract form values
		newUsername := ctx.FormValue("username")
		selectedIcon := ctx.FormValue("icon")

		// Handle custom icon upload
		file, err := ctx.FormFile("custom_icon")
		if err == nil && file != nil {
			// Validate the upload
			if err := ValidateImageUpload(file); err != nil {
				return renderProfileEditError(ctx, user, err.Error())
			}

			// Generate secure filename
			filename, err := GenerateSecureFilename(user.UserId, file.Filename)
			if err != nil {
				return renderProfileEditError(ctx, user, "Failed to generate filename")
			}

			// Get safe upload path
			uploadDir := "./server/uploads/icons"
			safePath := GetSafeUploadPath(uploadDir, filename)

			// Ensure directory exists
			if err := os.MkdirAll(uploadDir, 0755); err != nil {
				return renderProfileEditError(ctx, user, "Failed to create upload directory")
			}

			// Save file
			if err := ctx.SaveFile(file, safePath); err != nil {
				return renderProfileEditError(ctx, user, "Failed to upload file")
			}

			// Delete old custom icon if exists
			if user.CustomIcon != "" {
				oldPath := "./server" + user.CustomIcon
				os.Remove(oldPath) // Ignore error if file doesn't exist
			}

			// Update user profile
			user.CustomIcon = "/uploads/icons/" + filename
			user.Icon = "" // Clear default icon when custom is set
		} else if selectedIcon != "" {
			// User selected a default icon
			user.Icon = selectedIcon

			// Delete old custom icon if switching to default
			if user.CustomIcon != "" {
				oldPath := "./server" + user.CustomIcon
				os.Remove(oldPath)
				user.CustomIcon = ""
			}
		}

		// Handle username update
		if newUsername != "" && newUsername != oldUsername {
			// Check if username already exists
			if existingUser := udb.FindUserByUsername(newUsername); existingUser != nil {
				return renderProfileEditError(ctx, user, "Username already exists")
			}
			user.Username = newUsername
		}

		// Save to database
		if err := udb.Save(); err != nil {
			return renderProfileEditError(ctx, user, "Failed to save changes")
		}

		// Update session with new username
		sessionID := ctx.Cookies("session_id")
		if sessionID != "" {
			if currentSession, _ := smngr.GetSession(ctx.Context(), sessionID); currentSession != nil {
				currentSession.Username = user.Username
				smngr.SaveSession(ctx.Context(), currentSession)
			}
		}

		// Update context
		ctx.Locals("username", user.Username)

		// Small delay for UX (shows loading state)
		time.Sleep(500 * time.Millisecond)

		// Render success
		return ctx.Render("partials/profile-edit", fiber.Map{
			"Username":   user.Username,
			"UserId":     user.UserId,
			"Role":       user.Role,
			"Icon":       user.Icon,
			"CustomIcon": user.CustomIcon,
			"Saved":      true,
		})
	}
}

func HandleDashboard(csrv *chat.ChatService, udb *db.UsersDB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		contactUsernames := csrv.GetContacts(username)

		// Convert usernames to full user objects for template
		contacts := make([]*db.User, 0, len(contactUsernames))
		for _, contactName := range contactUsernames {
			if user := udb.FindUserByUsername(contactName); user != nil {
				contacts = append(contacts, user)
			}
		}

		currentUserIcon := ""
		currentUserCustomIcon := ""

		user := udb.FindUserByUsername(username)
		if user != nil {
			currentUserIcon = user.Icon
			currentUserCustomIcon = user.CustomIcon
		}

		return c.Render("dashboard", fiber.Map{
			"Username":              username,
			"CurrentUserIcon":       currentUserIcon,
			"CurrentUserCustomIcon": currentUserCustomIcon,
			"Contacts":              contacts,
		})
	}
}

// HandleProfileView renders the user's profile page
func HandleProfileView(udb *db.UsersDB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		user := udb.FindUserByUsername(username)
		if user == nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}

		// Check if it's an HTMX request for partial rendering
		if isHTMXRequest(c) {
			return c.Render("partials/profile-view", user)
		}

		// Full page render
		return c.Render("profile", user)
	}
}

// HandleProfileEdit renders the profile edit form
func HandleProfileEdit(udb *db.UsersDB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		user := udb.FindUserByUsername(username)
		if user == nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}

		return c.Render("partials/profile-edit", fiber.Map{
			"Username":   user.Username,
			"UserId":     user.UserId,
			"Role":       user.Role,
			"Icon":       user.Icon,
			"CustomIcon": user.CustomIcon,
			"Saved":      false,
		})
	}
}

// getUsernameFromContext safely extracts username from context locals
func getUsernameFromContext(c *fiber.Ctx) (string, error) {
	val := c.Locals("username")
	if val == nil {
		return "", fiber.ErrUnauthorized
	}

	username, ok := val.(string)
	if !ok || username == "" {
		return "", fiber.ErrUnauthorized
	}

	return username, nil
}

// handleUnauthorized redirects to login for unauthorized requests
func handleUnauthorized(c *fiber.Ctx) error {
	if isHTMXRequest(c) {
		c.Set("HX-Redirect", "/")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	return c.Redirect("/")
}

// renderProfileEditError is a helper to render profile edit with error
func renderProfileEditError(ctx *fiber.Ctx, user *db.User, errorMsg string) error {
	return ctx.Render("partials/profile-edit", fiber.Map{
		"Username":   user.Username,
		"UserId":     user.UserId,
		"Role":       user.Role,
		"Icon":       user.Icon,
		"CustomIcon": user.CustomIcon,
		"Error":      errorMsg,
	})
}
