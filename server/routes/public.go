package routes

import (
	"exc6/db"
	"exc6/server/handlers"
	"exc6/services/sessions"

	"github.com/gofiber/fiber/v2"
)

// PublicRoutes handles all public-facing routes (no authentication required)
type PublicRoutes struct {
	udb   *db.UsersDB
	smngr *sessions.SessionManager
}

// NewPublicRoutes creates a new public routes handler
func NewPublicRoutes(udb *db.UsersDB, smngr *sessions.SessionManager) *PublicRoutes {
	return &PublicRoutes{
		udb:   udb,
		smngr: smngr,
	}
}

// Register sets up all public routes
func (pr *PublicRoutes) Register(app *fiber.App) {
	// Landing page
	app.Get("/", handlers.HandleHomepage())

	// Authentication forms (with HTMX support)
	app.Get("/login-form", handlers.HandleLoginForm())
	app.Get("/register-form", handlers.HandleRegisterForm())

	// Authentication actions
	app.Post("/register", handlers.HandleUserRegister(pr.udb))
	app.Post("/login", handlers.HandleUserLogin(pr.udb, pr.smngr))
	app.Post("/logout", handlers.HandleUserLogout(pr.smngr))

	// Development/testing routes
	app.Get("/test/sse", handlers.HandleSSETest())
}
