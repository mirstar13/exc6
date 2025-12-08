package handlers

import (
	"context"
	"exc6/db"
	"exc6/services/sessions"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HandleUserProfileUpdate handles profile updates with secure file uploads
func HandleUserProfileUpdate(qdb *db.Queries, smngr *sessions.SessionManager) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		oldUsername := ctx.Locals("username").(string)

		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		user, err := qdb.GetUserByUsername(dbCtx, oldUsername)
		if err != nil {
			return renderProfileEditError(ctx, &db.User{}, "User not found")
		}

		// Extract form values
		newUsername := ctx.FormValue("username")
		selectedIcon := ctx.FormValue("icon")

		// Handle custom icon upload
		file, err := ctx.FormFile("custom_icon")
		if err == nil && file != nil {
			// Validate the upload
			if err := ValidateImageUpload(file); err != nil {
				return renderProfileEditError(ctx, &user, err.Error())
			}

			// Generate secure filename
			filename, err := GenerateSecureFilename(user.ID.String(), file.Filename)
			if err != nil {
				return renderProfileEditError(ctx, &user, "Failed to generate filename")
			}

			// Get safe upload path
			uploadDir := "./server/uploads/icons"
			safePath := GetSafeUploadPath(uploadDir, filename)

			// Ensure directory exists
			if err := os.MkdirAll(uploadDir, 0755); err != nil {
				return renderProfileEditError(ctx, &user, "Failed to create upload directory")
			}

			// Save file
			if err := ctx.SaveFile(file, safePath); err != nil {
				return renderProfileEditError(ctx, &user, "Failed to upload file")
			}

			// Delete old custom icon if exists
			if user.CustomIcon.Valid && user.CustomIcon.String != "" {
				oldPath := "./server" + user.CustomIcon.String
				os.Remove(oldPath)
			}

			// Update user profile
			user.CustomIcon.String = "/uploads/icons/" + filename
			user.Icon.String = "" // Clear default icon when custom is set
		} else if selectedIcon != "" {
			// User selected a default icon
			user.Icon.String = selectedIcon

			// Delete old custom icon if switching to default
			if user.CustomIcon.Valid && user.CustomIcon.String != "" {
				oldPath := "./server" + user.CustomIcon.String
				os.Remove(oldPath)
				user.CustomIcon.String = ""
			}
		}

		// Handle username update
		if newUsername != "" && newUsername != oldUsername {
			// Check if username already exists
			if _, err := qdb.UpdateUser(dbCtx, db.UpdateUserParams{
				ID:         user.ID,
				Username:   newUsername,
				Icon:       user.Icon,
				CustomIcon: user.CustomIcon,
			}); err != nil {
				return renderProfileEditError(ctx, &user, "Username already taken")
			}
		}

		// Update session with new username
		sessionID := ctx.Cookies("session_id")
		if sessionID != "" {
			sessCtx, sessCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer sessCancel()

			if currentSession, _ := smngr.GetSession(sessCtx, sessionID); currentSession != nil {
				currentSession.Username = user.Username
				smngr.SaveSession(sessCtx, currentSession)
			}
		}

		// Update context
		ctx.Locals("username", user.Username)

		// Small delay for UX (shows loading state)
		time.Sleep(500 * time.Millisecond)

		// Render success
		return ctx.Render("partials/profile-edit", fiber.Map{
			"Username":   user.Username,
			"UserId":     user.ID,
			"Role":       user.Role,
			"Icon":       user.Icon,
			"CustomIcon": user.CustomIcon,
			"Saved":      true,
		})
	}
}

// HandleProfileView renders the user's profile page
func HandleProfileView(qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		user, err := qdb.GetUserByUsername(ctx, username)
		if err != nil {
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
func HandleProfileEdit(qdb *db.Queries) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, err := getUsernameFromContext(c)
		if err != nil {
			return handleUnauthorized(c)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		user, err := qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}

		return c.Render("partials/profile-edit", fiber.Map{
			"Username":   user.Username,
			"UserId":     user.ID,
			"Role":       user.Role,
			"Icon":       user.Icon,
			"CustomIcon": user.CustomIcon,
			"Saved":      false,
		})
	}
}
