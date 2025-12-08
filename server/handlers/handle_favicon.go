package handlers

import "github.com/gofiber/fiber/v2"

func HandleFavicon() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.SendFile("./server/static/favicon.ico")
	}
}
