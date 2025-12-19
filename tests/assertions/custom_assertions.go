package assertions

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// AssertTimeAlmostEqual checks if two times are within delta
func AssertTimeAlmostEqual(t *testing.T, expected, actual time.Time, delta time.Duration) {
	diff := actual.Sub(expected)
	if diff < 0 {
		diff = -diff
	}

	assert.LessOrEqual(t, diff, delta,
		"Times should be within %v: expected %v, got %v (diff: %v)",
		delta, expected, actual, diff)
}

// AssertJSONEqual compares JSON strings ignoring formatting
func AssertJSONEqual(t *testing.T, expected, actual string) {
	var expectedJSON, actualJSON interface{}

	json.Unmarshal([]byte(expected), &expectedJSON)
	json.Unmarshal([]byte(actual), &actualJSON)

	assert.Equal(t, expectedJSON, actualJSON)
}

// AssertHTMLContains checks if HTML contains text (ignoring tags)
func AssertHTMLContains(t *testing.T, html, text string) {
	// Strip HTML tags
	stripped := stripHTMLTags(html)
	assert.Contains(t, stripped, text)
}

func stripHTMLTags(html string) string {
	// Simple implementation - use a proper HTML parser for production
	result := ""
	inTag := false

	for _, char := range html {
		if char == '<' {
			inTag = true
		} else if char == '>' {
			inTag = false
		} else if !inTag {
			result += string(char)
		}
	}

	return result
}
