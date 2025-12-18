package routes

import (
	"exc6/db"
	"exc6/server/handlers"
	"exc6/server/middleware/auth"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/sessions"

	"github.com/gofiber/fiber/v2"
)

// AuthRoutes handles all authenticated routes (requires valid session)
type AuthRoutes struct {
	db    *db.Queries
	csrv  *chat.ChatService
	fsrv  *friends.FriendService
	smngr *sessions.SessionManager
}

// NewAuthRoutes creates a new authenticated routes handler
func NewAuthRoutes(db *db.Queries, csrv *chat.ChatService, fsrv *friends.FriendService, smngr *sessions.SessionManager) *AuthRoutes {
	return &AuthRoutes{
		db:    db,
		csrv:  csrv,
		fsrv:  fsrv,
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
		Next:           nil,
	}))

	// Dashboard - main chat interface (NOW SHOWS ONLY FRIENDS)
	authed.Get("/dashboard", handlers.HandleDashboard(ar.fsrv, ar.db))

	// Chat routes
	ar.registerChatRoutes(authed)

	// Profile routes
	ar.registerProfileRoutes(authed)

	// Friend management routes
	ar.registerFriendRoutes(authed)
}

// registerChatRoutes sets up chat-related endpoints
func (ar *AuthRoutes) registerChatRoutes(router fiber.Router) {
	router.Get("/chat/:contact", handlers.HandleLoadChatWindow(ar.csrv, ar.db))
	router.Post("/chat/:contact", handlers.HandleSendMessage(ar.csrv))
	router.Get("/sse/:contact", handlers.HandleSSE(ar.csrv))
}

// registerProfileRoutes sets up profile management endpoints
func (ar *AuthRoutes) registerProfileRoutes(router fiber.Router) {
	router.Get("/profile", handlers.HandleProfileView(ar.db))
	router.Get("/profile/edit", handlers.HandleProfileEdit(ar.db))
	router.Put("/profile", handlers.HandleUserProfileUpdate(ar.db, ar.smngr))
}

// registerFriendRoutes sets up friend management endpoints
func (ar *AuthRoutes) registerFriendRoutes(router fiber.Router) {
	// Main friends page
	router.Get("/friends", handlers.HandleFriendsPage(ar.fsrv))

	// Search for users
	router.Get("/friends/search", handlers.HandleSearchUsers(ar.fsrv))

	// Send friend request
	router.Post("/friends/request/:username", handlers.HandleSendFriendRequest(ar.fsrv))

	// Accept friend request
	router.Post("/friends/accept/:username", handlers.HandleAcceptFriendRequest(ar.fsrv))

	// Reject friend request
	router.Delete("/friends/reject/:username", handlers.HandleRejectFriendRequest(ar.fsrv))

	// Remove friend
	router.Delete("/friends/remove/:username", handlers.HandleRemoveFriend(ar.fsrv))
}
