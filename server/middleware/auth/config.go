package auth

import (
	"exc6/db"
	"exc6/services/sessions"

	"github.com/gofiber/fiber/v2"
)

type Config struct {
	// Next defines a function to skip middleware.
	//
	// Optional. Default: nil
	Next func(c *fiber.Ctx) bool

	// Users DB stores the user information.
	//
	// Required. Default: nil
	UsersDB *db.UsersDB

	// Realm is a string to define realm attribute of BasicAuth.
	// the realm identifies the system to authenticate against
	// and can be used by clients to save credentials
	//
	// Optional. Default: "Restricted".
	Realm string

	// Authorizer defines a function you can pass
	// to check the credentials however you want.
	// It will be called with a username and password
	// and is expected to return true or false to indicate
	// that the credentials were approved or not.
	//
	// Optional. Default: nil.
	Authorizer func(string, string) bool

	// SessionManager is the session manager to use for session handling
	//
	// Required. Default: nil
	SessionManager *sessions.SessionManager

	// Unauthorized defines the response body for unauthorized responses.
	// By default it will return with a 401 Unauthorized and the correct WWW-Auth header
	//
	// Optional. Default: nil
	Unauthorized fiber.Handler

	// ContextUser is the key to store the username in Locals
	//
	// Optional. Default: "username"
	ContextUsername any

	// ContextPassword is the key to store the password in Locals
	//
	// Optional. Default: "password"
	ContextPassword any

	// CookieName is the name of the cookie to parse the credentials from
	//
	// Optional. Default: "Authorization"
	CookieName string
}

var ConfigDefault = Config{
	Next:            nil,
	UsersDB:         nil,
	Authorizer:      nil,
	SessionManager:  nil,
	Unauthorized:    nil,
	ContextUsername: "username",
	ContextPassword: "password",
	CookieName:      "Authorization",
}

func configDefault(config ...Config) Config {
	// Return default config if nothing provided
	if len(config) < 1 {
		return ConfigDefault
	}

	// Override default config
	cfg := config[0]

	// Set default values
	if cfg.Next == nil {
		cfg.Next = ConfigDefault.Next
	}
	if cfg.UsersDB == nil {
		cfg.UsersDB = ConfigDefault.UsersDB
	}
	if cfg.Authorizer == nil {
		cfg.Authorizer = func(user, pass string) bool {
			if cfg.UsersDB == nil {
				return false
			}
			return cfg.UsersDB.ValidateCredentials(user, pass)
		}
	}
	if cfg.SessionManager == nil {
		cfg.SessionManager = ConfigDefault.SessionManager
	}
	if cfg.Unauthorized == nil {
		cfg.Unauthorized = func(c *fiber.Ctx) error {
			// If it is an HTMX request, we might want to return a snippet
			if c.Get("HX-Request") == "true" {
				return c.SendString("<div class='error'>Unauthorized. Please log in again.</div>")
			}

			// Otherwise redirect to login or send 401
			return c.Redirect("/login-form")
		}
	}
	if cfg.ContextUsername == nil {
		cfg.ContextUsername = ConfigDefault.ContextUsername
	}
	if cfg.ContextPassword == nil {
		cfg.ContextPassword = ConfigDefault.ContextPassword
	}
	if cfg.CookieName == "" {
		cfg.CookieName = ConfigDefault.CookieName
	}

	return cfg
}
