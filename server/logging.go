package server

import (
	"exc6/apperrors"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

// setupLogging configures the HTTP request logger middleware
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

// setupErrorLogging creates a logger for application errors
func setupErrorLogging(logFile string) (*log.Logger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll("log", 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open log file
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// Fallback to stdout if file can't be opened
		log.Printf("Warning: could not open log file %s: %v", logFile, err)
		f = os.Stdout
	}

	errLogger := apperrors.DefaultHandlerConfig().Logger
	errLogger.SetOutput(f)
	return errLogger, nil
}
