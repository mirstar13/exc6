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
	"exc6/server/websocket"
	"exc6/services/calls"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/groups"
	"exc6/services/sessions"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/requestid"
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
	gsrv  *groups.GroupService
	cfg   *config.Config
}

func NewServer(cfg *config.Config, db *db.Queries, rdb *redis.Client, csrv *chat.ChatService, smngr *sessions.SessionManager, fsrv *friends.FriendService, gsrv *groups.GroupService, websocketManager *websocket.Manager, callsSrv *calls.CallService) (*Server, error) {
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

	app.Use(requestid.New())

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

	// This ensures tokens are available (in Locals and Cookies) when validation happens
	// This remains global so tokens are generated/injected on all requests
	app.Use(handlers.InjectCSRFToken(csrfStorage, 1*time.Hour))

	// Prepare CSRF Protection Middleware (validation) but do not attach globally
	// We will attach it to authenticated routes so it runs AFTER Auth middleware
	csrfMiddleware := csrf.New(csrf.Config{
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
	})

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
		Next: func(c *fiber.Ctx) bool {
			// Skip rate limiting for metrics endpoint
			return c.Path() == "/metrics"
		},
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
		gsrv:  gsrv,
		cfg:   cfg,
	}

	// Register all routes, passing the CSRF middleware
	routes.RegisterRoutes(app, db, csrv, fsrv, gsrv, smngr, *websocketManager, callsSrv, csrfMiddleware)

	return srv, nil
}

func (s *Server) Start() error {
	addr := s.cfg.ServerAddress()

	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")

	if certFile != "" && keyFile != "" {
		log.Printf("Starting HTTPS server on %s", addr)
		return s.App.ListenTLS(addr, certFile, keyFile)
	}

	log.Printf("Starting server on %s", addr)
	return s.App.Listen(addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	return s.App.ShutdownWithContext(ctx)
}
