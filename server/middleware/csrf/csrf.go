package csrf

import (
	"crypto/rand"
	"encoding/base64"
	"exc6/apperrors"
	"time"

	"github.com/gofiber/fiber/v2"
)

// New creates a new CSRF middleware
func New(config ...Config) fiber.Handler {
	cfg := ConfigDefault
	if len(config) > 0 {
		cfg = config[0]
	}

	// Apply defaults
	if cfg.KeyLookup == "" {
		cfg.KeyLookup = ConfigDefault.KeyLookup
	}
	if cfg.CookieName == "" {
		cfg.CookieName = ConfigDefault.CookieName
	}
	if cfg.CookieSameSite == "" {
		cfg.CookieSameSite = ConfigDefault.CookieSameSite
	}
	if cfg.Expiration == 0 {
		cfg.Expiration = ConfigDefault.Expiration
	}
	if cfg.Storage == nil {
		cfg.Storage = ConfigDefault.Storage
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = ConfigDefault.ErrorHandler
	}

	// Parse KeyLookup
	extractor := createExtractor(cfg.KeyLookup)

	return func(c *fiber.Ctx) error {
		// Skip if Next returns true
		if cfg.Next != nil && cfg.Next(c) {
			return c.Next()
		}

		// Get session ID for token association
		sessionID := c.Cookies("session_id")
		if sessionID == "" {
			// No session, skip CSRF check (handled by auth middleware)
			return c.Next()
		}

		// Only check CSRF for state-changing methods
		method := c.Method()
		if method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
			// Safe methods don't need CSRF protection
			return c.Next()
		}

		// Extract token from request
		token := extractor(c)
		if token == "" {
			return cfg.ErrorHandler(c, apperrors.New(
				apperrors.ErrCodeValidationFailed,
				"CSRF token missing",
				fiber.StatusForbidden,
			))
		}

		// Validate token
		storedToken, err := cfg.Storage.Get(sessionID)
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}

		if token != storedToken {
			return cfg.ErrorHandler(c, apperrors.New(
				apperrors.ErrCodeValidationFailed,
				"CSRF token invalid",
				fiber.StatusForbidden,
			))
		}

		// Token valid, continue
		return c.Next()
	}
}

// GenerateToken creates a new CSRF token for a session
func GenerateToken(c *fiber.Ctx, storage Storage, expiration time.Duration) (string, error) {
	sessionID := c.Cookies("session_id")
	if sessionID == "" {
		return "", apperrors.New(
			apperrors.ErrCodeUnauthorized,
			"No session found",
			fiber.StatusUnauthorized,
		)
	}

	// Generate random token
	token, err := generateRandomToken(32)
	if err != nil {
		return "", err
	}

	// Store token associated with session
	if err := storage.Set(sessionID, token, expiration); err != nil {
		return "", err
	}

	// Set cookie with token reference
	c.Cookie(&fiber.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Expires:  time.Now().Add(expiration),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
	})

	return token, nil
}

// createExtractor creates a function to extract the CSRF token based on KeyLookup
func createExtractor(lookup string) func(*fiber.Ctx) string {
	// Parse lookup format: "source:key"
	parts := []rune(lookup)
	var source, key string

	colonIdx := -1
	for i, r := range parts {
		if r == ':' {
			colonIdx = i
			break
		}
	}

	if colonIdx > 0 {
		source = string(parts[:colonIdx])
		key = string(parts[colonIdx+1:])
	} else {
		source = "header"
		key = "X-CSRF-Token"
	}

	switch source {
	case "header":
		return func(c *fiber.Ctx) string {
			return c.Get(key)
		}
	case "form":
		return func(c *fiber.Ctx) string {
			return c.FormValue(key)
		}
	case "query":
		return func(c *fiber.Ctx) string {
			return c.Query(key)
		}
	default:
		return func(c *fiber.Ctx) string {
			return c.Get("X-CSRF-Token")
		}
	}
}

// generateRandomToken creates a cryptographically secure random token
func generateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
