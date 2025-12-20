package server

import (
	"errors"
	"time"

	"github.com/gofiber/template/html/v2"
)

// addTemplateFunctions adds custom functions to the template engine
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

	// Icon style helper - returns inline style for background
	engine.AddFunc("iconStyle", func(icon string) string {
		styles := map[string]string{
			"gradient-blue":   "background: linear-gradient(to bottom right, #3b82f6, #1d4ed8)",
			"gradient-purple": "background: linear-gradient(to bottom right, #a855f7, #ec4899)",
			"gradient-green":  "background: linear-gradient(to bottom right, #22c55e, #059669)",
			"gradient-orange": "background: linear-gradient(to bottom right, #f97316, #dc2626)",
			"gradient-cyan":   "background: linear-gradient(to bottom right, #06b6d4, #2563eb)",
			"gradient-rose":   "background: linear-gradient(to bottom right, #f43f5e, #ec4899)",
			"gradient-indigo": "background: linear-gradient(to bottom right, #6366f1, #9333ea)",
			"gradient-amber":  "background: linear-gradient(to bottom right, #f59e0b, #f97316)",
			"gradient-teal":   "background: linear-gradient(to bottom right, #14b8a6, #059669)",
			"gradient-slate":  "background: linear-gradient(to bottom right, #475569, #374151)",
			"solid-signal":    "background: #2C6BED",
			"solid-dark":      "background: #2C2C2C; border: 1px solid rgba(255,255,255,0.1)",
			"solid-red":       "background: #dc2626",
			"solid-emerald":   "background: #059669",
			"solid-violet":    "background: #7c3aed",
		}

		if style, ok := styles[icon]; ok {
			return style
		}
		return "background: linear-gradient(to bottom right, #3b82f6, #1d4ed8)" // Default blue gradient
	})

	// String truncation helper
	engine.AddFunc("truncate", func(s string, length int) string {
		if len(s) <= length {
			return s
		}
		if length <= 3 {
			return s[:length]
		}
		return s[:length-3] + "..."
	})

	// Check if string is empty or whitespace
	engine.AddFunc("isEmpty", func(s string) bool {
		return len(s) == 0 || len(s) == len(" ")
	})

	// Pluralize helper: pluralize count "item" "items"
	engine.AddFunc("pluralize", func(count int, singular, plural string) string {
		if count == 1 {
			return singular
		}
		return plural
	})

	// Default value helper: default value defaultValue
	engine.AddFunc("default", func(value, defaultValue any) any {
		if value == nil || value == "" {
			return defaultValue
		}
		return value
	})

	return nil
}
