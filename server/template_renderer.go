package server

import (
	"bytes"
	"strings"

	"github.com/gofiber/template/html/v2"
)

// TemplateRenderer provides utilities for rendering templates to strings
type TemplateRenderer struct {
	engine *html.Engine
}

// NewTemplateRenderer creates a new template renderer
func NewTemplateRenderer(engine *html.Engine) *TemplateRenderer {
	return &TemplateRenderer{engine: engine}
}

// RenderToString renders a template to a string
func (tr *TemplateRenderer) RenderToString(template string, binding interface{}) (string, error) {
	buf := new(bytes.Buffer)
	if err := tr.engine.Render(buf, template, binding); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderToSingleLine renders a template and removes all newlines/carriage returns
// This is useful for SSE (Server-Sent Events) which require single-line data
func (tr *TemplateRenderer) RenderToSingleLine(template string, binding interface{}) (string, error) {
	html, err := tr.RenderToString(template, binding)
	if err != nil {
		return "", err
	}

	// Remove newlines and carriage returns for SSE
	html = strings.ReplaceAll(html, "\n", "")
	html = strings.ReplaceAll(html, "\r", "")
	html = strings.ReplaceAll(html, "\t", "")

	// Collapse multiple spaces into single space
	for strings.Contains(html, "  ") {
		html = strings.ReplaceAll(html, "  ", " ")
	}

	return strings.TrimSpace(html), nil
}
