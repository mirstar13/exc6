package routes

import (
	"exc6/db"
	"exc6/server/handlers"
	"exc6/server/middleware/limiter"
	"exc6/services/sessions"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// PublicRoutes handles all public-facing routes (no authentication required)
type PublicRoutes struct {
	db    *db.Queries
	smngr *sessions.SessionManager
	rdb   *redis.Client
}

// NewPublicRoutes creates a new public routes handler
func NewPublicRoutes(db *db.Queries, smngr *sessions.SessionManager, rdb *redis.Client) *PublicRoutes {
	return &PublicRoutes{
		db:    db,
		smngr: smngr,
		rdb:   rdb,
	}
}

// Register sets up all public routes
func (pr *PublicRoutes) Register(app *fiber.App) {
	// Landing page
	app.Get("/", handlers.HandleHomepage())

	// Authentication forms (with HTMX support)
	app.Get("/login-form", handlers.HandleLoginForm())
	app.Get("/register-form", handlers.HandleRegisterForm())

	// Strict rate limiting for authentication endpoints
	authLimiter := limiter.New(limiter.Config{
		Capacity:     5,
		RefillRate:   1,
		RefillPeriod: 1 * time.Minute,
		Storage:      limiter.NewRedisStorage(pr.rdb, 5*time.Minute),
		LimitReachedHandler: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Too many attempts. Please try again later.",
			})
		},
	})

	// Authentication actions
	app.Post("/register", authLimiter, handlers.HandleUserRegister(pr.db))
	app.Post("/login", authLimiter, handlers.HandleUserLogin(pr.db, pr.smngr))
	app.Post("/logout", handlers.HandleUserLogout(pr.smngr))
}
