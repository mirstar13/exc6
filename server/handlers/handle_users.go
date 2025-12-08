package handlers

import (
	"database/sql"
	"exc6/apperrors"
	"exc6/db"
	"exc6/services/sessions"
	"exc6/utils"
	"log"
	"math/rand"
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

func HandleUserRegister(qdb *db.Queries) fiber.Handler {
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
		if err := utils.ValidatePasswordStrength(password); err != nil {
			appErr := apperrors.FromError(err)
			return ctx.Render("partials/register", fiber.Map{
				"Error": appErr.Message,
			})
		}

		// Check if user exists
		if _, err := qdb.GetUserByUsername(ctx.Context(), username); err == nil {
			err := apperrors.NewUserExists(username)
			return ctx.Render("partials/register", fiber.Map{
				"Error": err.Message,
			})
		}

		// Hash password
		passwordHash, err := utils.HashPassword(password)
		if err != nil {
			log.Printf("Password hashing error: %v", err)
			return apperrors.NewInternalError("Failed to create account")
		}

		// Create user
		randomIcon := defaultIcons[rand.Intn(len(defaultIcons))]
		if _, err := qdb.CreateUser(ctx.Context(), db.CreateUserParams{
			Username:     username,
			PasswordHash: passwordHash,
			Icon:         sql.NullString{String: randomIcon, Valid: true},
			CustomIcon:   sql.NullString{String: "", Valid: true},
		}); err != nil {
			appErr := apperrors.FromError(err)
			return ctx.Render("partials/register", fiber.Map{
				"Error": appErr.Message,
			})
		}

		return ctx.Render("partials/account-created", nil)
	}
}

func HandleUserLogin(qdb *db.Queries, smngr *sessions.SessionManager) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		username := ctx.FormValue("username")
		password := ctx.FormValue("password")

		user, err := qdb.GetUserByUsername(ctx.Context(), username)
		if err != nil {
			if err == sql.ErrNoRows {
				// User not found
				appErr := apperrors.NewInvalidCredentials()
				return ctx.Render("partials/login", fiber.Map{
					"Error":    appErr.Message,
					"Username": username,
				})
			}
			// Other DB error
			log.Printf("DB error fetching user: %v", err)
			return apperrors.NewInternalError("Failed to process login")
		}

		// Verify password
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			// Invalid password
			appErr := apperrors.NewInvalidCredentials()
			return ctx.Render("partials/login", fiber.Map{
				"Error":    appErr.Message,
				"Username": username,
			})
		}

		// Create session
		sessionID := uuid.NewString()
		newSession := sessions.NewSession(
			sessionID,
			user.ID.String(),
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
			Secure:   true,
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
