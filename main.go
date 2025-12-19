package main

import (
	"context"
	"database/sql"
	"exc6/config"
	"exc6/db"
	infraredis "exc6/infrastructure/redis"
	"exc6/server"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/groups"
	"exc6/services/sessions"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}

func run() error {
	// Load environment
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	log.Println("✓ Configuration loaded and validated")
	cfg.PrintSummary()

	// Initialize Redis with proper pooling
	rdb, err := infraredis.NewClient(cfg.Redis)
	if err != nil {
		return fmt.Errorf("failed to initialize Redis client: %w", err)
	}
	defer rdb.Close()
	log.Println("✓ Connected to Redis")

	// Open users database
	datb, err := sql.Open("postgres", cfg.Database.ConnectionString)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	dbqueries := db.New(datb)
	log.Println("✓ Loaded users database")

	csrv, err := chat.NewChatService(appCtx, rdb, dbqueries, cfg.Kafka.Address)
	if err != nil {
		return fmt.Errorf("failed to initialize chat service: %w", err)
	}
	defer csrv.Close()
	log.Println("✓ Initialized chat service")

	// Initialize session manager
	smngr := sessions.NewSessionManager(rdb)
	log.Println("✓ Initialized session manager")

	fsrv := friends.NewFriendService(dbqueries)
	log.Println("✓ Initialized friend service")

	gsrv := groups.NewGroupService(dbqueries)
	log.Println("✓ Initialized group service")

	// Create server
	srv, err := server.NewServer(cfg, dbqueries, rdb, csrv, smngr, fsrv, gsrv)
	if err != nil {
		return fmt.Errorf("failed to create server; err: %w", err)
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		log.Printf("Received signal: %v. Shutting down gracefully...", sig)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Cancel application context to stop background services
	appCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	log.Println("✓ Server shutdown complete")
	return nil
}
