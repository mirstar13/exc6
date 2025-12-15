package csrf

import (
	"exc6/apperrors"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Config defines CSRF middleware configuration
type Config struct {
	// Next defines a function to skip middleware
	Next func(c *fiber.Ctx) bool

	// KeyLookup defines where to look for the CSRF token
	// Format: "<source>:<key>"
	// Possible values: "header:X-CSRF-Token", "form:_csrf", "query:csrf"
	KeyLookup string

	// CookieName is the name of the CSRF cookie
	CookieName string

	// CookieSameSite sets the cookie SameSite attribute
	CookieSameSite string

	// CookieSecure sets the cookie Secure flag
	CookieSecure bool

	// CookieHTTPOnly sets the cookie HTTPOnly flag
	CookieHTTPOnly bool

	// Expiration is the duration for which the CSRF token is valid
	Expiration time.Duration

	// Storage for CSRF tokens (optional, uses in-memory by default)
	Storage Storage

	// ErrorHandler is called when CSRF validation fails
	ErrorHandler fiber.ErrorHandler
}

// ConfigDefault is the default configuration
var ConfigDefault = Config{
	KeyLookup:      "header:X-CSRF-Token",
	CookieName:     "csrf_token",
	CookieSameSite: "Strict",
	CookieSecure:   true,
	CookieHTTPOnly: true,
	Expiration:     1 * time.Hour,
	Storage:        NewInMemoryStorage(),
	ErrorHandler: func(c *fiber.Ctx, err error) error {
		return apperrors.New(
			apperrors.ErrCodeValidationFailed,
			"CSRF token validation failed",
			fiber.StatusForbidden,
		).WithInternal(err)
	},
}
