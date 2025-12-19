package server

import (
	"context"
	"exc6/apperrors"
	"exc6/config"
	"exc6/db"
	"exc6/server/handlers"
	"exc6/server/middleware/csrf"
	"exc6/server/middleware/limiter"
	"exc6/server/middleware/security"
	"exc6/server/routes"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/sessions"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/template/html/v2"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	App   *fiber.App
	db    *db.Queries
	rdb   *redis.Client
	csrv  *chat.ChatService
	smngr *sessions.SessionManager
	fsrv  *friends.FriendService
	cfg   *config.Config
}

func NewServer(cfg *config.Config, db *db.Queries, rdb *redis.Client, csrv *chat.ChatService, smngr *sessions.SessionManager, fsrv *friends.FriendService) (*Server, error) {
	// Initialize template engine
	engine := html.New(cfg.Server.ViewsDir, ".html")

	// Add template functions
	if err := addTemplateFunctions(engine); err != nil {
		return nil, fmt.Errorf("failed to add template functions: %w", err)
	}

	errLogger, err := setupErrorLogging(cfg.Server.LogFile)
	if err != nil {
		return nil, fmt.Errorf("failed to setup error logging: %w", err)
	}

	errorConfig := apperrors.HandlerConfig{
		Logger:             errLogger,
		ShowInternalErrors: os.Getenv("APP_ENV") == "development",
		OnError: func(c *fiber.Ctx, err *apperrors.AppError) {
			// TODO: Add metrics/monitoring here
		},
	}

	// Create Fiber app with custom error handler
	app := fiber.New(fiber.Config{
		AppName:      "SArAChat",
		ServerHeader: "SArAChatServer",
		Views:        engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: apperrors.Handler(errorConfig),
	})

	// Security headers middleware
	app.Use(security.New(security.Config{
		Development: os.Getenv("APP_ENV") == "development",
		AllowedScriptSources: []string{
			"'self'",
			"https://unpkg.com",
			"https://cdn.tailwindcss.com",
		},
	}))

	csrfStorage := csrf.NewRedisStorage(rdb, 1*time.Hour)

	// CRITICAL FIX: Inject CSRF token BEFORE validation
	// This ensures tokens are available when validation happens
	app.Use(handlers.InjectCSRFToken(csrfStorage, 1*time.Hour))

	// CSRF Protection Middleware (validation)
	app.Use(csrf.New(csrf.Config{
		Storage:    csrfStorage,
		KeyLookup:  "header:X-CSRF-Token",
		CookieName: "csrf_token",
		Expiration: 1 * time.Hour,
		Next: func(c *fiber.Ctx) bool {
			path := c.Path()
			// Skip CSRF for public auth endpoints and GET requests
			return path == "/login" ||
				path == "/register" ||
				path == "/login-form" ||
				path == "/register-form" ||
				path == "/api/v1/status" ||
				c.Method() == "GET" ||
				c.Method() == "HEAD" ||
				c.Method() == "OPTIONS"
		},
	}))

	// Setup favicon middleware
	app.Use(favicon.New(favicon.Config{
		File: cfg.Server.StaticDir + "/favicon.ico",
		URL:  "/favicon.ico",
	}))

	// Serve static files
	app.Static("/static", cfg.Server.StaticDir, fiber.Static{
		Compress:      true,
		ByteRange:     false,
		Browse:        false,
		Index:         "",
		CacheDuration: 86400,
		MaxAge:        86400,
	})

	app.Static("/scripts", cfg.Server.ScriptsDir, fiber.Static{
		Compress:  false,
		ByteRange: false,
		Browse:    false,
		Index:     "",
		MaxAge:    86400,
	})

	app.Static("/uploads", cfg.Server.UploadsDir)

	// Setup logging
	if err := setupLogging(app, cfg.Server.LogFile); err != nil {
		return nil, fmt.Errorf("failed to setup logging: %w", err)
	}

	// Setup rate limiting
	app.Use(limiter.New(limiter.Config{
		Capacity:     cfg.RateLimit.Capacity,
		RefillRate:   cfg.RateLimit.RefillRate,
		RefillPeriod: cfg.RateLimit.RefillPeriod,
		LimitReachedHandler: func(c *fiber.Ctx) error {
			return apperrors.NewRateLimitError()
		},
	}))

	srv := &Server{
		App:   app,
		rdb:   rdb,
		db:    db,
		csrv:  csrv,
		smngr: smngr,
		fsrv:  fsrv,
		cfg:   cfg,
	}

	// Register all routes
	routes.RegisterRoutes(app, db, csrv, fsrv, smngr)

	return srv, nil
}

func (s *Server) Start() error {
	addr := s.cfg.ServerAddress()
	log.Printf("Starting server on %s", addr)
	return s.App.Listen(addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	return s.App.ShutdownWithContext(ctx)
}
