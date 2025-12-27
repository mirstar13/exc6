package apperrors

import (
	"fmt"
	"html"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// HandlerConfig configures the error handler
type HandlerConfig struct {
	// Logger for error logging
	Logger *log.Logger

	// ShowInternalErrors shows internal error details in responses (dev only)
	ShowInternalErrors bool

	// OnError is called for each error (useful for metrics/monitoring)
	OnError func(c *fiber.Ctx, err *AppError)
}

// DefaultHandlerConfig returns sensible defaults
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		Logger:             log.Default(),
		ShowInternalErrors: false,
		OnError:            nil,
	}
}

// Handler creates a Fiber error handler middleware
func Handler(config HandlerConfig) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		// Convert to AppError
		appErr := FromError(err)

		// Log the error
		if config.Logger != nil {
			logError(config.Logger, c, appErr)
		}

		// Call custom error handler if provided
		if config.OnError != nil {
			config.OnError(c, appErr)
		}

		// Determine response format based on request type
		isHTMX := c.Get("HX-Request") == "true"
		isAPI := strings.HasPrefix(c.Path(), "/api/") ||
			strings.HasPrefix(c.Path(), "/call/") ||
			c.Path() == "/api"

		// Handle HTMX requests
		if isHTMX {
			return handleHTMXError(c, appErr)
		}

		// Handle API requests
		if isAPI {
			return handleAPIError(c, appErr, config.ShowInternalErrors)
		}

		// Handle regular browser requests
		return handleBrowserError(c, appErr)
	}
}

// handleHTMXError returns HTML fragments for HTMX requests
func handleHTMXError(c *fiber.Ctx, err *AppError) error {
	// For authentication errors, redirect to login
	if err.Code == ErrCodeUnauthorized || err.Code == ErrCodeSessionExpired {
		c.Set("HX-Redirect", "/")
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// Return error fragment
	html := renderErrorFragment(err)
	return c.Status(err.StatusCode).Type("html").SendString(html)
}

// handleAPIError returns JSON for API requests
func handleAPIError(c *fiber.Ctx, err *AppError, showInternal bool) error {
	response := fiber.Map{
		"error": fiber.Map{
			"code":    err.Code,
			"message": err.Message,
		},
	}

	// Add details if present
	if len(err.Details) > 0 {
		response["error"].(fiber.Map)["details"] = err.Details
	}

	// Add internal error in dev mode
	if showInternal && err.Internal != nil {
		response["error"].(fiber.Map)["internal"] = err.Internal.Error()
	}

	return c.Status(err.StatusCode).JSON(response)
}

// handleBrowserError returns full HTML pages for browser requests
func handleBrowserError(c *fiber.Ctx, err *AppError) error {
	// For auth errors, redirect to login
	if err.Code == ErrCodeUnauthorized || err.Code == ErrCodeSessionExpired {
		return c.Redirect("/")
	}

	// Render using error.html template
	renderErr := c.Status(err.StatusCode).Render("error", fiber.Map{
		"StatusCode": err.StatusCode,
		"ErrorCode":  err.Code,
		"Message":    err.Message,
		"Title":      getErrorTitle(err.StatusCode),
	})

	// Fallback to plain text if render fails
	if renderErr != nil {
		log.Printf("Error rendering error page: %v", renderErr)
		return c.Status(err.StatusCode).Type("html").SendString(
			fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Error %d</title></head>
<body>
<h1>Error %d</h1>
<p>%s</p>
</body>
</html>`, err.StatusCode, err.StatusCode, html.EscapeString(err.Message)))
	}

	return nil
}

// renderErrorFragment creates an HTML error fragment for HTMX
func renderErrorFragment(err *AppError) string {
	icon := getErrorIcon(err.StatusCode)
	color := getErrorColor(err.StatusCode)

	return fmt.Sprintf(`<div class="error-fragment" data-color="%s">
    %s
    <div>
        <p class="font-semibold mb-0.5">%s</p>
        <p class="error-message">%s</p>
    </div>
</div>
<style>
    .error-fragment {
        padding: 1rem;
        border-radius: 0.75rem;
        margin-bottom: 1rem;
        font-size: 0.875rem;
        display: flex;
        align-items: start;
        gap: 0.75rem;
        animation: shake 0.5s;
    }

    .error-fragment[data-color="red"] {
        background-color: rgba(239, 68, 68, 0.1);
        border: 1px solid rgba(239, 68, 68, 0.3);
        color: #ef4444;
    }

    .error-fragment[data-color="yellow"] {
        background-color: rgba(234, 179, 8, 0.1);
        border: 1px solid rgba(234, 179, 8, 0.3);
        color: #eab308;
    }

    .error-fragment[data-color="orange"] {
        background-color: rgba(249, 115, 22, 0.1);
        border: 1px solid rgba(249, 115, 22, 0.3);
        color: #f97316;
    }

    .error-fragment svg {
        width: 1.25rem;
        height: 1.25rem;
        flex-shrink: 0;
        margin-top: 0.125rem;
    }

    .error-message {
        color: rgba(239, 68, 68, 0.9);
    }
    
    @keyframes shake {
        0%%, 100%% { transform: translateX(0); }
        10%%, 30%%, 50%%, 70%%, 90%% { transform: translateX(-8px); }
        20%%, 40%%, 60%%, 80%% { transform: translateX(8px); }
    }
</style>`,
		color,
		icon,
		html.EscapeString(string(err.Code)),
		html.EscapeString(err.Message))
}

// logError logs the error with rich context
func logError(logger *log.Logger, c *fiber.Ctx, err *AppError) {
	// Build log message with full context
	fields := err.LogFields()

	// Add request context
	fields["method"] = c.Method()
	fields["path"] = c.Path()
	fields["ip"] = c.IP()
	fields["user_agent"] = c.Get("User-Agent")

	if username := c.Locals("username"); username != nil {
		fields["username"] = username
	}

	if requestID := c.Locals("requestid"); requestID != nil {
		fields["request_id"] = requestID
	}

	// Format fields for standard logger
	var fieldPairs []string
	for k, v := range fields {
		fieldPairs = append(fieldPairs, fmt.Sprintf("%s=%v", k, v))
	}

	logLevel := "ERROR"
	if err.StatusCode < 500 {
		logLevel = "WARN"
	}

	logger.Printf("[%s] %s | %s",
		logLevel,
		err.Error(),
		strings.Join(fieldPairs, " | "))
}

// getErrorTitle returns a user-friendly title based on status code
func getErrorTitle(statusCode int) string {
	switch statusCode {
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 429:
		return "Too Many Requests"
	case 500:
		return "Internal Server Error"
	case 503:
		return "Service Unavailable"
	default:
		return "Error"
	}
}

// getErrorIcon returns an SVG icon based on status code
func getErrorIcon(statusCode int) string {
	if statusCode >= 500 {
		return `<svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
		</svg>`
	}
	return `<svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
		<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
	</svg>`
}

// getErrorColor returns appropriate color based on status
func getErrorColor(statusCode int) string {
	switch {
	case statusCode >= 500:
		return "red"
	case statusCode == 429:
		return "yellow"
	case statusCode >= 400:
		return "orange"
	default:
		return "red"
	}
}

// WrapHandler wraps handler functions with automatic error conversion
func WrapHandler(h func(*fiber.Ctx) error) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := h(c)
		if err == nil {
			return nil
		}
		return FromError(err)
	}
}
