package handlers

import (
	"exc6/db"
	"exc6/services/sessions"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var defaultIcons = []string{
	"gradient-blue",
	"gradient-purple",
	"gradient-green",
	"gradient-orange",
	"gradient-cyan",
	"gradient-rose",
	"gradient-indigo",
	"gradient-amber",
	"gradient-teal",
	"solid-signal",
}

func HandleUserRegister(udb *db.UsersDB) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		username := ctx.FormValue("username")
		password := ctx.FormValue("password")
		confirmPassword := ctx.FormValue("confirm_password")

		if password != confirmPassword {
			return ctx.Render("partials/register", fiber.Map{
				"Error": "Passwords do not match!",
			})
		}

		if len(password) < 6 {
			return ctx.Render("partials/register", fiber.Map{
				"Error": "Password must be at least 6 characters!",
			})
		}

		if usr := udb.FindUserByUsername(username); usr != nil {
			return ctx.Render("partials/register", fiber.Map{
				"Error": "Username already exists!",
			})
		}

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return ctx.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}

		randomIcon := defaultIcons[rand.Intn(len(defaultIcons))]

		udb.AddUser(db.User{
			Username: username,
			Password: string(passwordHash),
			Role:     "member",
			Icon:     randomIcon,
		})
		udb.Save()

		return ctx.Render("partials/account-created", nil)
	}
}

func HandleUserLogin(udb *db.UsersDB, smngr *sessions.SessionManager) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		username := ctx.FormValue("username")
		password := ctx.FormValue("password")

		if !udb.ValidateCredentials(username, password) {
			return ctx.Render("partials/login", fiber.Map{
				"Error":    "Invalid credentials!",
				"Username": username,
			})
		}

		user := udb.FindUserByUsername(username)

		sessionID := uuid.NewString()
		newSession := sessions.NewSession(
			sessionID,
			user.UserId,
			username,
			time.Now().Unix(),
			time.Now().Unix(),
		)

		err := smngr.SaveSession(ctx.Context(), newSession)
		if err != nil {
			log.Println("Error saving session:", err)
			return ctx.Status(fiber.StatusInternalServerError).SendString("Session Error")
		}

		ctx.Cookie(&fiber.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Expires:  time.Now().Add(24 * time.Hour),
			HTTPOnly: true,
			SameSite: "Lax",
			Path:     "/",
		})

		ctx.Set("HX-Redirect", "/dashboard")
		return ctx.SendStatus(fiber.StatusOK)
	}
}

func HandleUserProfileUpdate(udb *db.UsersDB, smngr *sessions.SessionManager) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		oldUsername := ctx.Locals("username").(string)
		user := udb.FindUserByUsername(oldUsername)
		if user == nil {
			return ctx.Status(fiber.StatusNotFound).SendString("User not found")
		}

		newUsername := ctx.FormValue("username")
		selectedIcon := ctx.FormValue("icon")

		file, err := ctx.FormFile("custom_icon")
		if err == nil && file != nil {
			if file.Size > 5*1024*1024 {
				return ctx.Render("partials/profile-edit", fiber.Map{
					"Username":   oldUsername,
					"UserId":     user.UserId,
					"Role":       user.Role,
					"Icon":       user.Icon,
					"CustomIcon": user.CustomIcon,
					"Error":      "File size must be less than 5MB",
				})
			}

			contentType := file.Header.Get("Content-Type")
			if !strings.HasPrefix(contentType, "image/") {
				return ctx.Render("partials/profile-edit", fiber.Map{
					"Username":   oldUsername,
					"UserId":     user.UserId,
					"Role":       user.Role,
					"Icon":       user.Icon,
					"CustomIcon": user.CustomIcon,
					"Error":      "Only image files are allowed",
				})
			}

			uploadDir := "./server/uploads/icons"
			os.MkdirAll(uploadDir, 0755)

			filename := fmt.Sprintf("%s_%d%s", user.UserId, time.Now().Unix(), filepath.Ext(file.Filename))
			fullPath := filepath.Join(uploadDir, filename)

			if err := ctx.SaveFile(file, fullPath); err != nil {
				log.Printf("Failed to save file: %v", err)
				return ctx.Render("partials/profile-edit", fiber.Map{
					"Username":   oldUsername,
					"UserId":     user.UserId,
					"Role":       user.Role,
					"Icon":       user.Icon,
					"CustomIcon": user.CustomIcon,
					"Error":      "Failed to upload file",
				})
			}

			if user.CustomIcon != "" {
				oldPath := "." + user.CustomIcon
				os.Remove(oldPath)
			}

			user.CustomIcon = "/uploads/icons/" + filename
			user.Icon = ""
		} else if selectedIcon != "" {
			if user.CustomIcon != "" {
				oldPath := "." + user.CustomIcon
				os.Remove(oldPath)
			}
			user.Icon = selectedIcon
			user.CustomIcon = ""
		}

		if newUsername != "" && newUsername != oldUsername {
			if usr := udb.FindUserByUsername(newUsername); usr != nil {
				return ctx.Render("partials/profile-edit", fiber.Map{
					"Username":   oldUsername,
					"UserId":     user.UserId,
					"Role":       user.Role,
					"Icon":       user.Icon,
					"CustomIcon": user.CustomIcon,
					"Error":      "Username already exists",
				})
			}
			user.Username = newUsername
		}

		if err := udb.Save(); err != nil {
			log.Printf("Failed to save user database: %v", err)
			return ctx.Render("partials/profile-edit", fiber.Map{
				"Username":   user.Username,
				"UserId":     user.UserId,
				"Role":       user.Role,
				"Icon":       user.Icon,
				"CustomIcon": user.CustomIcon,
				"Error":      "Failed to save changes",
			})
		}

		sessionID := ctx.Cookies("session_id")
		if sessionID != "" {
			currentSession, _ := smngr.GetSession(ctx.Context(), sessionID)
			if currentSession != nil {
				currentSession.Username = user.Username
				smngr.SaveSession(ctx.Context(), currentSession)
			}
		}

		ctx.Locals("username", user.Username)

		// Small delay for better UX
		time.Sleep(500 * time.Millisecond)

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

func HandleUserLogout(smngr *sessions.SessionManager) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		sessionID := ctx.Cookies("session_id")

		if sessionID != "" {
			smngr.DeleteSession(ctx.Context(), sessionID)
		}

		ctx.Cookie(&fiber.Cookie{
			Name:     "session_id",
			Value:    "",
			Expires:  time.Now().Add(-1 * time.Hour),
			HTTPOnly: true,
			Path:     "/",
		})

		ctx.Set("HX-Redirect", "/")
		return ctx.SendStatus(fiber.StatusOK)
	}
}
