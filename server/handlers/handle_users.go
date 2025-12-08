package handlers

import (
	"exc6/apperrors"
	"exc6/db"
	"exc6/services/sessions"
	"log"
	"math/rand"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
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

		// Validate password match
		if password != confirmPassword {
			err := apperrors.NewPasswordMismatch()
			return ctx.Render("partials/register", fiber.Map{
				"Error": err.Message,
			})
		}

		// Validate password strength
		if err := db.ValidatePasswordStrength(password); err != nil {
			appErr := apperrors.FromError(err)
			return ctx.Render("partials/register", fiber.Map{
				"Error": appErr.Message,
			})
		}

		// Check if user exists
		if usr := udb.FindUserByUsername(username); usr != nil {
			err := apperrors.NewUserExists(username)
			return ctx.Render("partials/register", fiber.Map{
				"Error": err.Message,
			})
		}

		// Hash password
		passwordHash, err := db.HashPassword(password)
		if err != nil {
			log.Printf("Password hashing error: %v", err)
			return apperrors.NewInternalError("Failed to create account")
		}

		// Create user
		randomIcon := defaultIcons[rand.Intn(len(defaultIcons))]
		if err := udb.AddUser(db.User{
			Username: username,
			Password: passwordHash,
			Role:     "member",
			Icon:     randomIcon,
		}); err != nil {
			appErr := apperrors.FromError(err)
			return ctx.Render("partials/register", fiber.Map{
				"Error": appErr.Message,
			})
		}

		// Save to database
		if err := udb.Save(); err != nil {
			log.Printf("Database save error: %v", err)
			return apperrors.NewInternalError("Failed to save account")
		}

		return ctx.Render("partials/account-created", nil)
	}
}

func HandleUserLogin(udb *db.UsersDB, smngr *sessions.SessionManager) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		username := ctx.FormValue("username")
		password := ctx.FormValue("password")

		// Validate credentials
		if !udb.ValidateCredentials(username, password) {
			err := apperrors.NewInvalidCredentials()
			return ctx.Render("partials/login", fiber.Map{
				"Error":    err.Message,
				"Username": username,
			})
		}

		// Get user
		user := udb.FindUserByUsername(username)
		if user == nil {
			// This shouldn't happen after ValidateCredentials passes
			return apperrors.NewUserNotFound()
		}

		// Create session
		sessionID := uuid.NewString()
		newSession := sessions.NewSession(
			sessionID,
			user.UserId,
			username,
			time.Now().Unix(),
			time.Now().Unix(),
		)

		// Save session
		if err := smngr.SaveSession(ctx.Context(), newSession); err != nil {
			log.Printf("Session save error: %v", err)
			return apperrors.NewInternalError("Failed to create session")
		}

		// Set secure cookie
		ctx.Cookie(&fiber.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Expires:  time.Now().Add(24 * time.Hour),
			HTTPOnly: true,
			SameSite: "Lax",
			Secure:   false, // TODO: Set to true in production with HTTPS
			Path:     "/",
		})

		// Redirect to dashboard
		ctx.Set("HX-Redirect", "/dashboard")
		return ctx.SendStatus(fiber.StatusOK)
	}
}

func HandleUserLogout(smngr *sessions.SessionManager) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		sessionID := ctx.Cookies("session_id")

		if sessionID != "" {
			if err := smngr.DeleteSession(ctx.Context(), sessionID); err != nil {
				log.Printf("Failed to delete session: %v", err)
				// Continue anyway - clear the cookie
			}
		}

		// Clear cookie
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
