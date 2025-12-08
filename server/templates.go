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
