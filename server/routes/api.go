package routes

import (
	"github.com/gofiber/fiber/v2"
)

// APIRoutes handles versioned API endpoints
type APIRoutes struct{}

// NewAPIRoutes creates a new API routes handler
func NewAPIRoutes() *APIRoutes {
	return &APIRoutes{}
}

// Register sets up all API routes with versioning
func (ar *APIRoutes) Register(app *fiber.App) {
	// API base group
	api := app.Group("/api")

	// Version 1 endpoints
	ar.registerV1Routes(api)

}

// registerV1Routes sets up API v1 endpoints
func (ar *APIRoutes) registerV1Routes(api fiber.Router) {
	v1 := api.Group("/v1")

	// Health check / status endpoint
	v1.Get("/status", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "operational",
			"version": "1.0.0",
			"service": "SecureChat API",
		})
	})

}
