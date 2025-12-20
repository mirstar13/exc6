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

	engine.AddFunc("iconClass", GetIconClass)

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

func GetIconClass(icon string) string {
	iconClasses := map[string]string{
		"gradient-blue":   "bg-gradient-to-br from-blue-500 to-blue-700",
		"gradient-purple": "bg-gradient-to-br from-purple-500 to-pink-600",
		"gradient-green":  "bg-gradient-to-br from-green-500 to-emerald-600",
		"gradient-orange": "bg-gradient-to-br from-orange-500 to-red-600",
		"gradient-cyan":   "bg-gradient-to-br from-cyan-500 to-blue-600",
		"gradient-rose":   "bg-gradient-to-br from-rose-500 to-pink-600",
		"gradient-indigo": "bg-gradient-to-br from-indigo-500 to-purple-600",
		"gradient-amber":  "bg-gradient-to-br from-amber-500 to-orange-600",
		"gradient-teal":   "bg-gradient-to-br from-teal-500 to-green-600",
		"gradient-slate":  "bg-gradient-to-br from-slate-600 to-gray-700",
		"solid-signal":    "bg-signal-blue",
		"solid-dark":      "bg-signal-surface border border-white/10",
		"solid-red":       "bg-red-600",
		"solid-emerald":   "bg-emerald-600",
		"solid-violet":    "bg-violet-600",
	}

	if class, ok := iconClasses[icon]; ok {
		return class
	}

	// Default fallback
	return "bg-signal-blue"
}
