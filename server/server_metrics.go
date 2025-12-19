package server

import (
	"context"
	"database/sql"
	"exc6/apperrors"
	"exc6/config"
	"exc6/db"
	"exc6/pkg/metrics"
	"exc6/server/handlers"
	"exc6/server/middleware/csrf"
	"exc6/server/middleware/limiter"
	"exc6/server/middleware/security"
	"exc6/server/routes"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/sessions"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/template/html/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/gofiber/adaptor/v2"
)

func NewServerWithMetrics(
	cfg *config.Config,
	dbQueries *db.Queries,
	dbConn *sql.DB,
	rdb *redis.Client,
	csrv *chat.ChatService,
	smngr *sessions.SessionManager,
	fsrv *friends.FriendService,
) (*Server, error) {
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
			metrics.RecordError(string(err.Code), fmt.Sprintf("%d", err.StatusCode))
		},
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "SecureChat",
		ServerHeader: "SecureChat",
		Views:        engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: apperrors.Handler(errorConfig),
	})

	metrics.RegisterCollectors(dbConn, rdb, csrv.GetMetrics)

	go metrics.UpdateSessionCount(context.Background(), rdb, 30*time.Second)

	metrics.SystemInfo.WithLabelValues(
		"1.0.0",                         // version
		runtime.Version(),               // go version
		time.Now().Format(time.RFC3339), // start time
	).Set(1)

	app.Use(metrics.HTTPMetricsMiddleware())

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

	// CSRF injection
	app.Use(handlers.InjectCSRFToken(csrfStorage, 1*time.Hour))

	// CSRF validation
	app.Use(csrf.New(csrf.Config{
		Storage:    csrfStorage,
		KeyLookup:  "header:X-CSRF-Token",
		CookieName: "csrf_token",
		Expiration: 1 * time.Hour,
		Next: func(c *fiber.Ctx) bool {
			path := c.Path()
			return path == "/login" ||
				path == "/register" ||
				path == "/login-form" ||
				path == "/register-form" ||
				path == "/api/v1/status" ||
				path == "/metrics" ||
				c.Method() == "GET" ||
				c.Method() == "HEAD" ||
				c.Method() == "OPTIONS"
		},
	}))

	// Favicon
	app.Use(favicon.New(favicon.Config{
		File: cfg.Server.StaticDir + "/favicon.ico",
		URL:  "/favicon.ico",
	}))

	// Static files
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

	// Rate limiting with metrics
	app.Use(limiter.New(limiter.Config{
		Capacity:     cfg.RateLimit.Capacity,
		RefillRate:   cfg.RateLimit.RefillRate,
		RefillPeriod: cfg.RateLimit.RefillPeriod,
		Next: func(c *fiber.Ctx) bool {
			return c.Path() == "/metrics"
		},
		LimitReachedHandler: func(c *fiber.Ctx) error {
			return apperrors.NewRateLimitError()
		},
	}))

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	srv := &Server{
		App:   app,
		rdb:   rdb,
		db:    dbQueries,
		csrv:  csrv,
		smngr: smngr,
		fsrv:  fsrv,
		cfg:   cfg,
	}

	// Register all routes
	routes.RegisterRoutes(app, dbQueries, csrv, fsrv, smngr)

	return srv, nil
}
