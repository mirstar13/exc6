package server

import (
	"context"
	"exc6/apperrors"
	"exc6/config"
	"exc6/db"
	"exc6/server/middleware/limiter"
	"exc6/server/routes"
	"exc6/services/chat"
	"exc6/services/sessions"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	app   *fiber.App
	db    *db.Queries
	rdb   *redis.Client
	csrv  *chat.ChatService
	smngr *sessions.SessionManager
	cfg   *config.Config
}

func NewServer(cfg *config.Config, db *db.Queries, rdb *redis.Client, csrv *chat.ChatService, smngr *sessions.SessionManager) (*Server, error) {
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
			// Example: metrics.RecordError(err.Code, err.StatusCode)
		},
	}

	// Create Fiber app with custom error handler
	app := fiber.New(fiber.Config{
		AppName:      "SecureChat",
		ServerHeader: "SecureChatServer",
		Views:        engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: apperrors.Handler(errorConfig),
	})

	// Serve static uploads
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
		app:   app,
		rdb:   rdb,
		db:    db,
		csrv:  csrv,
		smngr: smngr,
		cfg:   cfg,
	}

	// Register all routes
	routes.RegisterRoutes(app, db, csrv, smngr)

	return srv, nil
}

func (s *Server) Start() error {
	addr := s.cfg.ServerAddress()
	log.Printf("Starting server on %s", addr)
	return s.app.Listen(addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	return s.app.ShutdownWithContext(ctx)
}
