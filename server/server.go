package server

import (
	"errors"
	"exc6/db"
	"exc6/server/middleware/limiter"
	"exc6/server/routes"
	"exc6/services/chat"
	"exc6/services/sessions"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	app   *fiber.App
	udb   *db.UsersDB
	rdb   *redis.Client
	csrv  *chat.ChatService
	smngr *sessions.SessionManager
}

func NewServer(viewsDir string, udb *db.UsersDB, rdb *redis.Client, csrv *chat.ChatService, smngr *sessions.SessionManager, addr string) *Server {
	engine := html.New(viewsDir, ".html")

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

	app := fiber.New(fiber.Config{
		AppName: "User Management Service",
		Views:   engine,
	})

	app.Static("/uploads", "./server/uploads")

	logFile, err := os.OpenFile("log/server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil
	}

	app.Use(logger.New(logger.Config{
		Format:     "${method} [${time}] ${ip} ${status} - ${path}\n",
		TimeFormat: "02-Jan-2006 15:04:05",
		TimeZone:   "Local",
		Output:     logFile,
	}))

	app.Use(limiter.New(limiter.Config{
		Capacity:   20,
		RefillRate: 5,
	}))

	return &Server{
		app:   app,
		rdb:   rdb,
		udb:   udb,
		csrv:  csrv,
		smngr: smngr,
	}
}

func (s *Server) Start() error {
	routes.RegisterRoutes(s.app, s.udb, s.csrv, s.smngr)
	return s.app.Listen(":8080")
}
