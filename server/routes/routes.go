package routes

import (
	"exc6/db"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/sessions"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RegisterRoutes configures all application routes and middleware
func RegisterRoutes(app *fiber.App, db *db.Queries, csrv *chat.ChatService, fsrv *friends.FriendService, smngr *sessions.SessionManager) {
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// Initialize route handlers
	publicRoutes := NewPublicRoutes(db, smngr)
	apiRoutes := NewAPIRoutes()
	authRoutes := NewAuthRoutes(db, csrv, fsrv, smngr)

	// Register public routes (no auth required)
	publicRoutes.Register(app)

	// Register API routes (versioned, authenticated)
	apiRoutes.Register(app)

	// Register authenticated routes (auth middleware applied)
	authRoutes.Register(app)
}
