package routes

import (
	"exc6/db"
	"exc6/server/handlers"
	"exc6/server/middleware/auth"
	"exc6/server/websocket"
	"exc6/services/calls"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/groups"
	"exc6/services/sessions"

	"github.com/gofiber/fiber/v2"
)

// AuthRoutes handles all authenticated routes (requires valid session)
type AuthRoutes struct {
	db             *db.Queries
	csrv           *chat.ChatService
	fsrv           *friends.FriendService
	gsrv           *groups.GroupService
	smngr          *sessions.SessionManager
	wsManager      *websocket.Manager
	callService    *calls.CallService
	csrfMiddleware fiber.Handler
}

// NewAuthRoutes creates a new authenticated routes handler
func NewAuthRoutes(
	db *db.Queries,
	csrv *chat.ChatService,
	fsrv *friends.FriendService,
	gsrv *groups.GroupService,
	smngr *sessions.SessionManager,
	wsManager *websocket.Manager,
	callService *calls.CallService,
	csrfMiddleware fiber.Handler,
) *AuthRoutes {
	return &AuthRoutes{
		db:             db,
		csrv:           csrv,
		fsrv:           fsrv,
		gsrv:           gsrv,
		smngr:          smngr,
		wsManager:      wsManager,
		callService:    callService,
		csrfMiddleware: csrfMiddleware,
	}
}

// Register sets up all authenticated routes with auth middleware
func (ar *AuthRoutes) Register(app *fiber.App) {
	// Create authenticated route group
	authed := app.Group("")

	// 1. First, apply Auth Middleware (loads user into context)
	authed.Use(auth.New(auth.Config{
		DB:             ar.db,
		SessionManager: ar.smngr,
		Next:           nil,
	}))

	// 2. Then, apply CSRF Middleware (validates token)
	// Now when it runs, c.Locals("username") will be populated, fixing "User: <nil>" logs
	authed.Use(ar.csrfMiddleware)

	// Dashboard - main chat interface
	authed.Get("/dashboard", handlers.HandleDashboard(ar.fsrv, ar.gsrv, ar.csrv, ar.callService, ar.db))

	// WebSocket endpoint for real-time chat and calls
	ar.registerWebSocketRoutes(authed)

	// Chat routes (HTTP endpoints for backwards compatibility)
	ar.registerChatRoutes(authed)

	// Voice call routes
	ar.registerCallRoutes(authed)

	// Profile routes
	ar.registerProfileRoutes(authed)

	// Friend management routes
	ar.registerFriendRoutes(authed)

	authed.Get("/notifications", handlers.HandleGetNotifications(ar.fsrv, ar.csrv, ar.callService))
	authed.Post("/notifications/mark-read", handlers.HandleMarkNotificationsRead(ar.csrv, ar.callService))

	authed.Get("/contacts", handlers.HandleGetContacts(ar.fsrv, ar.gsrv, ar.csrv, ar.callService))

	// Group management routes
	RegisterGroupRoutes(authed, ar.db, ar.csrv, ar.gsrv)
}

// registerWebSocketRoutes sets up WebSocket endpoints
func (ar *AuthRoutes) registerWebSocketRoutes(router fiber.Router) {
	// WebSocket upgrade check
	// Updated to pass GroupService and DB Queries
	router.Use("/ws", handlers.HandleWebSocketUpgrade(ar.wsManager, ar.csrv, ar.callService, ar.gsrv, ar.db))

	// WebSocket endpoint
	// Updated to pass GroupService and DB Queries
	router.Get("/ws/chat", handlers.HandleWebSocket(ar.wsManager, ar.csrv, ar.callService, ar.gsrv, ar.db))
}

// registerChatRoutes sets up chat-related endpoints
func (ar *AuthRoutes) registerChatRoutes(router fiber.Router) {
	router.Get("/chat/:contact", handlers.HandleLoadChatWindow(ar.csrv, ar.db))
	router.Post("/chat/:contact", handlers.HandleSendMessage(ar.csrv))
}

// registerCallRoutes sets up voice call endpoints
func (ar *AuthRoutes) registerCallRoutes(router fiber.Router) {
	// Initiate call
	router.Post("/call/initiate/:username", handlers.HandleCallInitiate(ar.callService, ar.wsManager))

	// Answer call
	router.Post("/call/answer/:call_id", handlers.HandleCallAnswer(ar.callService, ar.wsManager))

	// End call
	router.Post("/call/end/:call_id", handlers.HandleCallEnd(ar.callService, ar.wsManager))

	// Reject call
	router.Post("/call/reject/:call_id", handlers.HandleCallReject(ar.callService, ar.wsManager))

	// Call history
	router.Get("/call/history", handlers.HandleCallHistory(ar.callService))
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
	router.Post("/friends/request/:username", handlers.HandleSendFriendRequest(ar.fsrv, ar.wsManager))

	// Accept friend request
	router.Post("/friends/accept/:username", handlers.HandleAcceptFriendRequest(ar.fsrv, ar.wsManager))

	// Reject friend request
	router.Delete("/friends/reject/:username", handlers.HandleRejectFriendRequest(ar.fsrv))

	// Remove friend
	router.Delete("/friends/remove/:username", handlers.HandleRemoveFriend(ar.fsrv))
}
