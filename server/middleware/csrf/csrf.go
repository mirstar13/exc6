package csrf

import (
	"crypto/rand"
	"encoding/base64"
	"exc6/apperrors"
	"exc6/pkg/logger"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetKey retrieves the session ID or creates a persistent client ID for CSRF
func GetKey(c *fiber.Ctx) string {
	// 0. Check locals cache to ensure idempotency within request
	if cachedKey := c.Locals("csrf_key"); cachedKey != nil {
		if keyStr, ok := cachedKey.(string); ok {
			return keyStr
		}
	}

	// 1. Prefer authenticated session
	sessionID := c.Cookies("session_id")
	if sessionID != "" {
		c.Locals("csrf_key", sessionID)
		return sessionID
	}

	// 2. Fallback to client identifier
	clientID := c.Cookies("csrf_client_id")
	if clientID != "" {
		c.Locals("csrf_key", clientID)
		return clientID
	}

	// 3. Generate new identifier
	// Use existing random generator
	newID, _ := generateRandomToken(32)
	isSecure := os.Getenv("APP_ENV") != "development"

	c.Cookie(&fiber.Cookie{
		Name:     "csrf_client_id",
		Value:    newID,
		Expires:  time.Now().Add(24 * 365 * time.Hour), // Long lived
		HTTPOnly: true,
		Secure:   isSecure,
		SameSite: "Strict",
		Path:     "/",
	})

	c.Locals("csrf_key", newID)
	return newID
}

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

		// Get storage key (Session ID or Client ID)
		key := GetKey(c)

		token := extractor(c)

		if token == "" {
			logger.WithFields(map[string]interface{}{
				"method": method,
				"path":   path,
				"key":    key,
			}).Warn("CSRF Validation: Token missing from request")

			return cfg.ErrorHandler(c, apperrors.New(
				apperrors.ErrCodeValidationFailed,
				"CSRF token missing",
				fiber.StatusForbidden,
			))
		}

		// Validate token
		storedToken, err := cfg.Storage.Get(key)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"key":   key,
				"error": err,
			}).Error("CSRF Validation: Failed to retrieve stored token")

			return cfg.ErrorHandler(c, err)
		}

		if token != storedToken {
			logger.WithFields(map[string]interface{}{
				"key": key,
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
	key := GetKey(c)

	token, err := generateRandomToken(32)
	if err != nil {
		logger.WithError(err).Error("CSRF: Failed to generate random token")
		return "", err
	}

	// Store token associated with session/client
	if err := storage.Set(key, token, expiration); err != nil {
		logger.WithFields(map[string]interface{}{
			"key":   key,
			"error": err,
		}).Error("CSRF: Failed to store token")
		return "", err
	}

	isSecure := os.Getenv("APP_ENV") != "development"
	c.Cookie(&fiber.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Expires:  time.Now().Add(expiration),
		HTTPOnly: false,
		Secure:   isSecure,
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
