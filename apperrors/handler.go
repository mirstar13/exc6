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
		isAPI := strings.HasPrefix(c.Path(), "/api/") || c.Path() == "/api"

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
	return c.Status(err.StatusCode).SendString(html)
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

	// Try to render error page
	renderErr := c.Status(err.StatusCode).Render("error", fiber.Map{
		"Content": renderErrorFragment(err),
	})

	// Fallback to plain text if render fails
	if renderErr != nil {
		fmt.Println("Error rendering error page:", renderErr)
		return c.Status(err.StatusCode).SendString(err.Message)
	}

	return nil
}

// renderErrorFragment creates an HTML error fragment for HTMX
func renderErrorFragment(err *AppError) string {
	_ = getErrorIcon(err.StatusCode)
	color := getErrorColor(err.StatusCode)

	return `<div data-color="` + color + `" class="error-fragment">
        ` + getErrorIcon(err.StatusCode) + `
        <div>
            <p class="font-semibold mb-0.5">` + html.EscapeString(string(err.Code)) + `</p>
            <p class="message">` + html.EscapeString(err.Message) + `</p>
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
		  background-color: rgba(239, 68, 68, 0.1); /* bg-red-500/10 */
		  border: 1px solid rgba(239, 68, 68, 0.3); /* border-red-500/30 */
		  color: rgba(239, 68, 68, 1); /* text-red-400 */
		}

		.error-fragment[data-color="yellow"] {
		  background-color: rgba(234, 179, 8, 0.1); /* bg-yellow-500/10 */
		  border: 1px solid rgba(234, 179, 8, 0.3); /* border-yellow-500/30 */
		  color: rgba(234, 179, 8, 1); /* text-yellow-400 */
		}

		.error-fragment[data-color="orange"] {
		  background-color: rgba(249, 115, 22, 0.1); /* bg-orange-500/10 */
		  border: 1px solid rgba(249, 115, 22, 0.3); /* border-orange-500/30 */
		  color: rgba(249, 115, 22, 1); /* text-orange-400 */
		}

		.error-fragment[data-color="green"] {
		  background-color: rgba(34, 197, 94, 0.1); /* bg-green-500/10 */
		  border: 1px solid rgba(34, 197, 94, 0.3); /* border-green-500/30 */
		  color: rgba(34, 197, 94, 1); /* text-green-400 */
		}

		.message {
		  color: rgba(239, 68, 68, 0.75); /* Adjust text color for message */
		}
		
		@keyframes shake {
			0%, 100% { transform: translateX(0); }
			10%, 30%, 50%, 70%, 90% { transform: translateX(-8px); }
			20%, 40%, 60%, 80% { transform: translateX(8px); }
		}
		.animate-shake { animation: shake 0.6s ease-in-out; }
	</style>`
}

// logError logs the error with context
func logError(logger *log.Logger, c *fiber.Ctx, err *AppError) {
	// Don't log expected errors at error level
	if err.StatusCode < 500 {
		logger.Printf("[WARN] %s %s | %s | Status: %d | User: %v",
			c.Method(), c.Path(), err.Error(), err.StatusCode, c.Locals("username"))
		return
	}

	// Log internal errors with more details
	logger.Printf("[ERROR] %s %s | %s | Status: %d | IP: %s | User: %v",
		c.Method(), c.Path(), err.Error(), err.StatusCode, c.IP(), c.Locals("username"))

	// Log stack trace for internal errors if available
	if err.Internal != nil {
		logger.Printf("[ERROR] Internal error: %+v", err.Internal)
	}
}

// getErrorIcon returns an SVG icon based on status code
func getErrorIcon(statusCode int) string {
	if statusCode >= 500 {
		return `<svg class="w-5 h-5 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
		</svg>`
	}
	return `<svg class="w-5 h-5 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
		<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
	</svg>`
}

// getErrorColor returns appropriate Tailwind color based on status
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

// Helper function to wrap handler functions with automatic error conversion
func WrapHandler(h func(*fiber.Ctx) error) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := h(c)
		if err == nil {
			return nil
		}
		return FromError(err)
	}
}
