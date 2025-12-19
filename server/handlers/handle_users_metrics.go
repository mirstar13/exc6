package handlers

import (
	"context"
	"database/sql"
	"exc6/apperrors"
	"exc6/db"
	"exc6/pkg/logger"
	"exc6/pkg/metrics"
	"exc6/services/sessions"
	"exc6/utils"
	"math/rand"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func HandleUserRegisterWithMetrics(qdb *db.Queries) fiber.Handler {
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

		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Check if user exists
		if _, err := qdb.GetUserByUsername(dbCtx, username); err == nil {
			err := apperrors.NewUserExists(username)
			return ctx.Render("partials/register", fiber.Map{
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
			return ctx.Render("partials/register", fiber.Map{
				"Error": appErr.Message,
			})
		}

		// ✅ Record successful registration
		metrics.IncrementRegistrations()

		return ctx.Render("partials/account-created", nil)
	}
}

func HandleUserLoginWithMetrics(qdb *db.Queries, smngr *sessions.SessionManager) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		username := ctx.FormValue("username")
		password := ctx.FormValue("password")

		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		user, err := qdb.GetUserByUsername(dbCtx, username)
		if err != nil {
			// ✅ Record failed login
			metrics.RecordLoginAttempt(false)

			if err == sql.ErrNoRows {
				appErr := apperrors.NewInvalidCredentials()
				return ctx.Render("partials/login", fiber.Map{
					"Error":    appErr.Message,
					"Username": username,
				})
			}

			logger.WithFields(map[string]any{
				"username": username,
				"error":    err.Error(),
			}).Error("Database error fetching user")
			return apperrors.NewInternalError("Failed to process login")
		}

		// Verify password
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			// ✅ Record failed login
			metrics.RecordLoginAttempt(false)

			appErr := apperrors.NewInvalidCredentials()
			return ctx.Render("partials/login", fiber.Map{
				"Error":    appErr.Message,
				"Username": username,
			})
		}

		// ✅ Record successful login
		metrics.RecordLoginAttempt(true)

		// Create session
		sessionID := uuid.NewString()
		newSession := sessions.NewSession(
			sessionID,
			user.ID.String(),
			username,
			time.Now().Unix(),
			time.Now().Unix(),
		)

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

		// ✅ Record session creation
		metrics.IncrementSessionsCreated()

		// Set secure cookie
		ctx.Cookie(&fiber.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Expires:  time.Now().Add(24 * time.Hour),
			HTTPOnly: true,
			SameSite: "Lax",
			Secure:   false,
			Path:     "/",
		})

		ctx.Set("HX-Redirect", "/dashboard")
		return ctx.SendStatus(fiber.StatusOK)
	}
}
