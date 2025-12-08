package server

import (
	"context"
	"errors"
	"exc6/config"
	"exc6/db"
	"exc6/server/middleware/limiter"
	"exc6/server/routes"
	"exc6/services/chat"
	"exc6/services/sessions"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	app   *fiber.App
	udb   *db.UsersDB
	rdb   *redis.Client
	csrv  *chat.ChatService
	smngr *sessions.SessionManager
	cfg   *config.Config
}

func NewServer(cfg *config.Config, udb *db.UsersDB, rdb *redis.Client, csrv *chat.ChatService, smngr *sessions.SessionManager) (*Server, error) {
	// Initialize template engine
	engine := html.New(cfg.Server.ViewsDir, ".html")

	// Add template functions
	if err := addTemplateFunctions(engine); err != nil {
		return nil, fmt.Errorf("failed to add template functions: %w", err)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "SecureChat",
		Views:        engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: customErrorHandler,
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
	}))

	srv := &Server{
		app:   app,
		rdb:   rdb,
		udb:   udb,
		csrv:  csrv,
		smngr: smngr,
		cfg:   cfg,
	}

	// Register all routes
	routes.RegisterRoutes(app, udb, csrv, smngr)

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

func addTemplateFunctions(engine *html.Engine) error {
	// Dict function for template maps
	engine.AddFunc("dict", func(values ...any) (map[string]any, error) {
		if len(values)%2 != 0 {
			return nil, errors.New("invalid dict call")
		}
		dict := make(map[string]any, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, errors.New("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	})

	// Time formatting function
	engine.AddFunc("formatTime", func(timestamp int64) string {
		t := time.Unix(timestamp, 0)
		now := time.Now()

		if t.Day() == now.Day() && t.Month() == now.Month() && t.Year() == now.Year() {
			return t.Format("3:04 PM")
		}

		yesterday := now.AddDate(0, 0, -1)
		if t.Day() == yesterday.Day() && t.Month() == yesterday.Month() && t.Year() == yesterday.Year() {
			return "Yesterday"
		}

		return t.Format("Jan 2")
	})

	return nil
}

func setupLogging(app *fiber.App, logFile string) error {
	// Ensure log directory exists
	if err := os.MkdirAll("log", 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open log file
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// Fallback to stdout if file can't be opened
		log.Printf("Warning: could not open log file %s: %v", logFile, err)
		f = os.Stdout
	}

	// Setup logger middleware
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${method} ${path} | ${ip}\n",
		TimeFormat: "2006-01-02 15:04:05",
		TimeZone:   "Local",
		Output:     f,
	}))

	return nil
}

// customErrorHandler provides consistent error handling
func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"

	// Handle Fiber errors
	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
		message = e.Message
	}

	// Log the error
	log.Printf("Error: %v | Path: %s | Method: %s", err, c.Path(), c.Method())

	// Check if HTMX request
	if c.Get("HX-Request") == "true" {
		return c.Status(code).SendString(fmt.Sprintf("<div class='error'>%s</div>", message))
	}

	// Return JSON for API requests
	if c.Path()[:4] == "/api" {
		return c.Status(code).JSON(fiber.Map{
			"error": message,
			"code":  code,
		})
	}

	// Return HTML error page
	return c.Status(code).SendString(message)
}
