package routes

import (
	"exc6/db"
	"exc6/server/handlers"
	"exc6/server/middleware/auth"
	"exc6/services/chat"
	"exc6/services/sessions"

	"github.com/gofiber/fiber/v2"
)

// AuthRoutes handles all authenticated routes (requires valid session)
type AuthRoutes struct {
	db    *db.Queries
	csrv  *chat.ChatService
	smngr *sessions.SessionManager
}

// NewAuthRoutes creates a new authenticated routes handler
func NewAuthRoutes(db *db.Queries, csrv *chat.ChatService, smngr *sessions.SessionManager) *AuthRoutes {
	return &AuthRoutes{
		db:    db,
		csrv:  csrv,
		smngr: smngr,
	}
}

// Register sets up all authenticated routes with auth middleware
func (ar *AuthRoutes) Register(app *fiber.App) {
	// Create authenticated route group
	authed := app.Group("")
	authed.Use(auth.New(auth.Config{
		DB:             ar.db,
		SessionManager: ar.smngr,
		Next:           nil, // No routes to skip
	}))

	// Dashboard - main chat interface
	authed.Get("/dashboard", handlers.HandleDashboard(ar.csrv, ar.db))

	// Chat routes
	ar.registerChatRoutes(authed)

	// Profile routes
	ar.registerProfileRoutes(authed)
}

// registerChatRoutes sets up chat-related endpoints
func (ar *AuthRoutes) registerChatRoutes(router fiber.Router) {
	// Load chat window with contact
	router.Get("/chat/:contact", handlers.HandleLoadChatWindow(ar.csrv, ar.db))

	// Send message to contact
	router.Post("/chat/:contact", handlers.HandleSendMessage(ar.csrv))

	// Server-Sent Events for real-time messages
	router.Get("/sse/:contact", handlers.HandleSSE(ar.csrv))
}

// registerProfileRoutes sets up profile management endpoints
func (ar *AuthRoutes) registerProfileRoutes(router fiber.Router) {
	// View profile
	router.Get("/profile", handlers.HandleProfileView(ar.db))

	// Edit profile form
	router.Get("/profile/edit", handlers.HandleProfileEdit(ar.db))

	// Update profile
	router.Put("/profile", handlers.HandleUserProfileUpdate(ar.db, ar.smngr))
}
