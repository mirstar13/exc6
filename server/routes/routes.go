package routes

import (
	"exc6/db"
	"exc6/server/handlers"
	"exc6/server/middleware/auth"
	"exc6/services/chat"
	"exc6/services/sessions"

	"github.com/gofiber/fiber/v2"
)

// ContactInfo holds contact display information
type ContactInfo struct {
	Username   string
	Icon       string
	CustomIcon string
}

var RegisterRoutes = func(app *fiber.App, udb *db.UsersDB, csrv *chat.ChatService, smngr *sessions.SessionManager) {
	app.Get("/", func(ctx *fiber.Ctx) error {
		return ctx.Render("homepage", fiber.Map{
			"Title": "SecureChat - Private Messaging",
		})
	})

	app.Get("/test/sse", func(ctx *fiber.Ctx) error {
		return ctx.Render("test-sse", fiber.Map{})
	})

	app.Get("/login-form", func(c *fiber.Ctx) error {
		if c.Get("HX-Request") == "true" {
			return c.Render("partials/login", fiber.Map{})
		}
		return c.Render("login", fiber.Map{})
	})

	app.Get("/register-form", func(c *fiber.Ctx) error {
		if c.Get("HX-Request") == "true" {
			return c.Render("partials/register", fiber.Map{})
		}
		return c.Render("register", fiber.Map{})
	})

	app.Post("/register", handlers.HandleUserRegister(udb))
	app.Post("/login", handlers.HandleUserLogin(udb, smngr))
	app.Post("/logout", handlers.HandleUserLogout(smngr))

	authed := app.Group("")
	authed.Use(auth.New(auth.Config{Next: authNext, UsersDB: udb, SessionManager: smngr}))

	authed.Get("/profile", func(c *fiber.Ctx) error {
		val := c.Locals("username")
		username, ok := val.(string)

		if !ok || username == "" {
			if c.Get("HX-Request") == "true" {
				c.Set("HX-Redirect", "/")
				return c.SendStatus(fiber.StatusUnauthorized)
			}
			return c.Redirect("/")
		}

		user := udb.FindUserByUsername(username)
		if user == nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}

		if c.Get("HX-Request") == "true" {
			return c.Render("partials/profile-view", user)
		}

		return c.Render("profile", user)
	})

	authed.Get("/dashboard", func(c *fiber.Ctx) error {
		username := c.Locals("username").(string)
		contactUsernames := csrv.GetContacts(username)

		// Build contact info with icons
		contacts := make([]ContactInfo, 0, len(contactUsernames))
		for _, contactUsername := range contactUsernames {
			user := udb.FindUserByUsername(contactUsername)
			if user != nil {
				contacts = append(contacts, ContactInfo{
					Username:   user.Username,
					Icon:       user.Icon,
					CustomIcon: user.CustomIcon,
				})
			}
		}

		return c.Render("dashboard", fiber.Map{
			"Username": username,
			"Contacts": contacts,
		})
	})

	authed.Get("/chat/:contact", handlers.HandleLoadChatWindow(csrv, udb))
	authed.Post("/chat/:contact", handlers.HandleSendMessage(csrv))

	authed.Get("/sse/:contact", handlers.HandleSSE(csrv))

	authed.Get("/profile/edit", func(c *fiber.Ctx) error {
		val := c.Locals("username")
		username, ok := val.(string)
		if !ok || username == "" {
			if c.Get("HX-Request") == "true" {
				c.Set("HX-Redirect", "/")
				return c.SendStatus(fiber.StatusUnauthorized)
			}
			return c.Redirect("/")
		}

		user := udb.FindUserByUsername(username)
		if user == nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}

		if c.Get("HX-Request") == "true" {
			return c.Render("partials/profile-edit", fiber.Map{
				"Username":   user.Username,
				"UserId":     user.UserId,
				"Role":       user.Role,
				"Icon":       user.Icon,
				"CustomIcon": user.CustomIcon,
				"Saved":      false,
			})
		}
		return c.Render("partials/profile-edit", fiber.Map{
			"Username":   user.Username,
			"UserId":     user.UserId,
			"Role":       user.Role,
			"Icon":       user.Icon,
			"CustomIcon": user.CustomIcon,
			"Saved":      false,
		})
	})

	authed.Put("/profile", handlers.HandleUserProfileUpdate(udb, smngr))

	api := authed.Group("/api")
	v1 := api.Group("/v1")

	v1.Get("/status", func(c *fiber.Ctx) error {
		return c.SendString("API v1 is operational")
	})
}

func authNext(c *fiber.Ctx) bool {
	return false
}
