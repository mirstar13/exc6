package security

import (
	"github.com/gofiber/fiber/v2"
)

type Config struct {
	// AllowedScriptSources for CSP script-src directive
	AllowedScriptSources []string

	// AllowedStyleSources for CSP style-src directive
	AllowedStyleSources []string

	// AllowedFontSources for CSP font-src directive
	AllowedFontSources []string

	// Development mode allows 'unsafe-inline' for Tailwind
	Development bool
}

var DefaultConfig = Config{
	AllowedScriptSources: []string{
		"'self'",
		"https://unpkg.com",
		"https://cdn.tailwindcss.com",
	},
	AllowedStyleSources: []string{
		"'self'",
		"https://cdn.tailwindcss.com",
		"https://fonts.googleapis.com",
	},
	AllowedFontSources: []string{
		"'self'",
		"https://fonts.gstatic.com",
		"data:",
	},
	Development: false,
}

// configDefault merges provided config with defaults
func configDefault(config ...Config) Config {
	// Return default if nothing provided
	if len(config) < 1 {
		return DefaultConfig
	}

	cfg := config[0]

	// Merge with defaults if fields are empty
	if len(cfg.AllowedScriptSources) == 0 {
		cfg.AllowedScriptSources = DefaultConfig.AllowedScriptSources
	}
	if len(cfg.AllowedStyleSources) == 0 {
		cfg.AllowedStyleSources = DefaultConfig.AllowedStyleSources
	}
	if len(cfg.AllowedFontSources) == 0 {
		cfg.AllowedFontSources = DefaultConfig.AllowedFontSources
	}

	return cfg
}

// New creates a comprehensive security headers middleware
func New(config ...Config) fiber.Handler {
	cfg := configDefault(config...)

	return func(c *fiber.Ctx) error {
		// Build CSP policy
		csp := buildCSP(cfg)
		c.Set("Content-Security-Policy", csp)

		// Additional security headers
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "geolocation=(), microphone=(self), camera=(self)")

		c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

		return c.Next()
	}
}

func buildCSP(cfg Config) string {
	csp := "default-src 'self'; "

	// Script sources
	csp += "script-src"
	for _, src := range cfg.AllowedScriptSources {
		csp += " " + src
	}
	if cfg.Development {
		csp += " 'unsafe-inline'" // Needed for Tailwind CDN config
	}
	csp += "; "

	// Style sources
	csp += "style-src"
	for _, src := range cfg.AllowedStyleSources {
		csp += " " + src
	}
	csp += " 'unsafe-inline'" // Required for Tailwind and inline styles
	csp += "; "

	// Font sources
	csp += "font-src"
	for _, src := range cfg.AllowedFontSources {
		csp += " " + src
	}
	csp += "; "

	// Image sources (allow profile uploads and data URIs)
	csp += "img-src 'self' data: blob: https:; "

	// Connect sources (for SSE and API calls)
	csp += "connect-src 'self' ws: wss:; "

	// Frame restrictions
	csp += "frame-ancestors 'none'; "

	// Base URI restriction
	csp += "base-uri 'self'; "

	// Form action restriction
	csp += "form-action 'self';"

	return csp
}
