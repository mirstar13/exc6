package main

import (
	"context"
	"exc6/config"
	"exc6/db"
	"exc6/server"
	"exc6/services/chat"
	"exc6/services/sessions"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
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

	// Initialize Redis
	rdb := NewRedisClient()
	defer rdb.Close()

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	log.Println("✓ Connected to Redis")

	// Open users database
	udb, err := db.OpenUsersDB()
	if err != nil {
		return fmt.Errorf("failed to open users database: %w", err)
	}
	log.Println("✓ Loaded users database")

	// Initialize chat service
	csrv, err := chat.NewChatService(rdb, udb, os.Getenv("KAFKA_ADDR"))
	if err != nil {
		return fmt.Errorf("failed to initialize chat service: %w", err)
	}
	defer csrv.Close()
	log.Println("✓ Initialized chat service")

	// Initialize session manager
	smngr := sessions.NewSessionManager(rdb)
	log.Println("✓ Initialized session manager")

	// Create server
	srv, err := server.NewServer(&config.Config{
		Server: config.ServerConfig{
			Host:         "0.0.0.0",
			Port:         8080,
			ViewsDir:     "./server/views",
			LogFile:      "log/server.log",
			UploadsDir:   "./server/uploads",
			ReadTimeout:  5 * time.Minute,
			WriteTimeout: 0,
		},
		Redis: config.RedisConfig{
			Address:  os.Getenv("REDIS_ADDR"),
			Username: os.Getenv("REDIS_USERNAME"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       0,
		},
		Kafka: config.KafkaConfig{
			Address: os.Getenv("KAFKA_ADDR"),
			Topic:   "chat-history",
		},
		Session: config.SessionConfig{
			TTL:        24 * time.Hour,
			CookieName: "session_id",
		},
		Upload: config.UploadConfig{
			MaxFileSize: 5 * 1024 * 1024, // 5MB
			AllowedMimeTypes: []string{
				"image/jpeg",
				"image/png",
				"image/gif",
				"image/webp",
			},
			AllowedExtensions: []string{
				".jpg",
				".jpeg",
				".png",
				".gif",
				".webp",
			},
			IconsDir: "./server/uploads/icons",
		},
		RateLimit: config.RateLimitConfig{
			Capacity:     100,
			RefillRate:   20,
			RefillPeriod: time.Second,
		},
	}, udb, rdb, csrv, smngr)
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

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	log.Println("✓ Server shutdown complete")
	return nil
}
