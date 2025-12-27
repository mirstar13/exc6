package server

import (
	"exc6/config"
	"exc6/pkg/logger"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
)

// setupLogging configures the HTTP request logger middleware with rotation
func setupLogging(app *fiber.App, cfg config.LogConfig) error {
	// Create logger with rotation
	httpLogger, err := logger.NewWithConfig(logger.Config{
		Filename:   cfg.Filename,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
		LocalTime:  true,
		Level:      config.ParseLogLevel(cfg.Level),
	})
	if err != nil {
		return fmt.Errorf("failed to create HTTP logger: %w", err)
	}

	// Setup Fiber logger middleware
	app.Use(fiberlogger.New(fiberlogger.Config{
		Format:     "[${time}] WEB: ${status} | ${latency} | ${method} ${path} | ${ip}\n",
		TimeFormat: "2006-01-02 15:04:05.999",
		TimeZone:   "Local",
		Output:     httpLogger.OutputWriter, // Use the rotating writer
	}))

	return nil
}

// setupErrorLogging creates a logger for application errors with rotation
func setupErrorLogging(cfg config.LogConfig) (*logger.Logger, error) {
	// Create error logger with rotation
	errorLogger, err := logger.NewWithConfig(logger.Config{
		Filename:   cfg.Filename,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
		LocalTime:  true,
		Level:      config.ParseLogLevel(cfg.Level),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create error logger: %w", err)
	}

	// Set as default logger for the apperrors package
	// (This assumes apperrors.DefaultHandlerConfig().Logger accepts *logger.Logger)

	return errorLogger, nil
}

func convertLoggerToLog(l *logger.Logger) *log.Logger {
	return log.New(l.OutputWriter, "", 0)
}
