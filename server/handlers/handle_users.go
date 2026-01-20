package handlers

import (
	"context"
	"database/sql"
	"exc6/apperrors"
	"exc6/db"
	"exc6/pkg/logger"
	"exc6/services/sessions"
	"exc6/utils"
	"math/rand"
	"os"
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

		// Validate username
		if err := utils.ValidateUsername(username); err != nil {
			return ctx.Status(fiber.StatusBadRequest).Render("partials/register", fiber.Map{
				"Error": err.Message,
			})
		}

		// Validate password match
		if password != confirmPassword {
			return apperrors.NewPasswordMismatch() // Let error handler set status
		}

		// Validate password strength
		if err := utils.ValidatePasswordStrength(password); err != nil {
			appErr := apperrors.FromError(err)
			// FIX: Return proper status code
			return ctx.Status(fiber.StatusBadRequest).Render("partials/register", fiber.Map{
				"Error": appErr.Message,
			})
		}

		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Check if user exists
		if _, err := qdb.GetUserByUsername(dbCtx, username); err == nil {
			err := apperrors.NewUserExists(username)
			// FIX: Return proper status code
			return ctx.Status(fiber.StatusConflict).Render("partials/register", fiber.Map{
				"Error": err.Message,
			})
		}

		// Hash password
		passwordHash, err := utils.HashPassword(password)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Password hashing failed")
			return apperrors.NewInternalError("Failed to create account")
		}

		// Create user
		randomIcon := defaultIcons[rand.Intn(len(defaultIcons))]
		if _, err := qdb.CreateUser(dbCtx, db.CreateUserParams{
			Username:     username,
			PasswordHash: passwordHash,
			Icon:         sql.NullString{String: randomIcon, Valid: true},
			CustomIcon:   sql.NullString{String: "", Valid: true},
		}); err != nil {
			appErr := apperrors.FromError(err)
			return ctx.Status(fiber.StatusInternalServerError).Render("partials/register", fiber.Map{
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

		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		user, err := qdb.GetUserByUsername(dbCtx, username)
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
			logger.WithFields(map[string]any{
				"username": username,
				"error":    err.Error(),
			}).Error("Database error fetching user")
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

		// Save session with background context
		sessCtx, sessCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer sessCancel()

		if err := smngr.SaveSession(sessCtx, newSession); err != nil {
			logger.WithFields(map[string]any{
				"username":   username,
				"session_id": sessionID,
				"error":      err.Error(),
			}).Error("Failed to save session")
			return apperrors.NewInternalError("Failed to create session")
		}

		// Set secure cookie
		isSecure := os.Getenv("APP_ENV") != "development"
		ctx.Cookie(&fiber.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Expires:  time.Now().Add(24 * time.Hour),
			HTTPOnly: true,
			SameSite: "Lax",
			Secure:   isSecure,
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
			sessCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			if err := smngr.DeleteSession(sessCtx, sessionID); err != nil {
				logger.WithFields(map[string]any{
					"session_id": sessionID,
					"error":      err.Error(),
				}).Warn("Failed to delete session during logout")
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
