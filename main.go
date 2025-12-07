package main

import (
	"exc6/db"
	"exc6/server"
	"exc6/services/chat"
	"exc6/services/sessions"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env")

	rdb := NewRedisClient()

	udb, err := db.OpenUsersDB()
	if err != nil {
		panic(err)
	}

	csrv, err := chat.NewChatService(rdb, udb, os.Getenv("KAFKA_ADDR"))
	if err != nil {
		panic(err)
	}

	smngr := sessions.NewSessionManager(rdb)

	srv := server.NewServer("./server/views", udb, rdb, csrv, smngr, "0.0.0.0:8080")

	srv.Start()
}
