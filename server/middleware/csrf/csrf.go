package csrf

import (
	"crypto/rand"
	"encoding/base64"
	"exc6/apperrors"
	"exc6/pkg/logger"
	"time"

	"github.com/gofiber/fiber/v2"
)

// New creates a new CSRF middleware
func New(config ...Config) fiber.Handler {
	cfg := ConfigDefault
	if len(config) > 0 {
		cfg = config[0]
	}

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

	extractor := createExtractor(cfg.KeyLookup)

	return func(c *fiber.Ctx) error {
		if cfg.Next != nil && cfg.Next(c) {
			return c.Next()
		}

		method := c.Method()
		if method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
			return c.Next()
		}

		path := c.Path()
		if path == "/login" || path == "/register" || path == "/login-form" || path == "/register-form" {
			return c.Next()
		}

		sessionID := c.Cookies("session_id")
		if sessionID == "" {
			// No session yet - skip CSRF check
			return c.Next()
		}

		token := extractor(c)

		if token == "" {
			logger.WithFields(map[string]interface{}{
				"method":     method,
				"path":       path,
				"session_id": sessionID,
			}).Warn("CSRF Validation: Token missing from request")

			return cfg.ErrorHandler(c, apperrors.New(
				apperrors.ErrCodeValidationFailed,
				"CSRF token missing",
				fiber.StatusForbidden,
			))
		}

		// Validate token
		storedToken, err := cfg.Storage.Get(sessionID)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"session_id": sessionID,
				"error":      err,
			}).Error("CSRF Validation: Failed to retrieve stored token")

			return cfg.ErrorHandler(c, err)
		}

		if token != storedToken {
			logger.WithFields(map[string]interface{}{
				"session_id": sessionID,
			}).Warn("CSRF Validation: Token mismatch")

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

	token, err := generateRandomToken(32)
	if err != nil {
		logger.WithError(err).Error("CSRF: Failed to generate random token")
		return "", err
	}

	// Store token associated with session
	if err := storage.Set(sessionID, token, expiration); err != nil {
		logger.WithFields(map[string]interface{}{
			"session_id": sessionID,
			"error":      err,
		}).Error("CSRF: Failed to store token")
		return "", err
	}

	c.Cookie(&fiber.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Expires:  time.Now().Add(expiration),
		HTTPOnly: false,
		Secure:   false,
		SameSite: "Strict",
		Path:     "/",
	})

	return token, nil
}

func createExtractor(lookup string) func(*fiber.Ctx) string {
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

func generateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
