package csrf_test

import (
	"exc6/server/middleware/csrf"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestCSRF_UnauthenticatedRequest_ShouldBeBlocked(t *testing.T) {
	app := fiber.New()

	// Use in-memory storage for testing
	storage := csrf.NewInMemoryStorage()

	// Setup CSRF middleware
	app.Use(csrf.New(csrf.Config{
		Storage: storage,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.SendStatus(fiber.StatusForbidden)
		},
	}))

	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	// Create request without session cookie
	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)

	// Expect 403 Forbidden. Currently returns 200 OK (Vulnerable).
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode, "Unauthenticated POST request should be blocked by CSRF middleware")
}

func TestGenerateToken_NoSession_ShouldSucceed(t *testing.T) {
	app := fiber.New()
	storage := csrf.NewInMemoryStorage()

	app.Get("/generate", func(c *fiber.Ctx) error {
		_, err := csrf.GenerateToken(c, storage, 1*time.Hour)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/generate", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)

    // Expect 200 OK. Currently returns 500 (Error: No session found).
	assert.Equal(t, 200, resp.StatusCode, "GenerateToken should succeed even without session")
}
