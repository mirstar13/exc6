package routes

import (
	"exc6/db"
	"exc6/services/chat"
	"exc6/services/sessions"

	"github.com/gofiber/fiber/v2"
)

// RegisterRoutes configures all application routes and middleware
func RegisterRoutes(app *fiber.App, db *db.Queries, csrv *chat.ChatService, smngr *sessions.SessionManager) {
	// Initialize route handlers
	publicRoutes := NewPublicRoutes(db, smngr)
	authRoutes := NewAuthRoutes(db, csrv, smngr)
	apiRoutes := NewAPIRoutes()

	// Register public routes (no auth required)
	publicRoutes.Register(app)

	// Register authenticated routes (auth middleware applied)
	authRoutes.Register(app)

	// Register API routes (versioned, authenticated)
	apiRoutes.Register(app)
}
