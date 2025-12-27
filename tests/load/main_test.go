package load

import (
	"database/sql"
	"exc6/config"
	"exc6/pkg/logger"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

var testConfig *config.Config

func TestMain(m *testing.M) {
	// 1. Initialize Logger
	testLogger = logger.New("./log/load_test.log")
	testLogger.SetLevel(logger.DEBUG)

	// 2. Load Config & Env Vars
	os.Setenv("GOOSE_DBSTRING", "postgres://postgres:postgres@127.0.0.1:5433/securechat_test?sslmode=disable")
	os.Setenv("REDIS_ADDR", "127.0.0.1:6380")
	os.Setenv("REDIS_PASSWORD", "")
	os.Setenv("KAFKA_ADDR", "127.0.0.1:9093")

	// Increase rate limits for load testing to prevent errors during user creation
	os.Setenv("RATE_LIMIT_CAPACITY", "10000")
	os.Setenv("RATE_LIMIT_REFILL", "1000")

	os.Setenv("LOG_FILE", "./tests/load/log/server.log")

	// 3. Run Migrations
	testLogger.Info("Running global database migrations...")
	if err := runMigrationsWithRetry(os.Getenv("GOOSE_DBSTRING"), 5); err != nil {
		testLogger.WithError(err).Fatal("Global migrations failed")
		os.Exit(1)
	}

	// 4. Run all tests
	exitCode := m.Run()

	// 5. Exit
	os.Exit(exitCode)
}

// Robust migration helper with retry logic
func runMigrationsWithRetry(connStr string, retries int) error {
	var err error
	for i := 0; i < retries; i++ {
		err = runMigrations(connStr)
		if err == nil {
			return nil
		}
		testLogger.WithError(err).Warn("Migration attempt %d failed, retrying in 2s...", i+1)
		time.Sleep(2 * time.Second)
	}
	return err
}

func runMigrations(connStr string) error {
	testLogger.Info("Starting database migration")

	migrateDB, err := sql.Open("postgres", connStr)
	if err != nil {
		testLogger.WithError(err).Error("Failed to open database for migrations")
		return fmt.Errorf("failed to open database for migrations: %w", err)
	}
	defer migrateDB.Close()

	wd, err := os.Getwd()
	if err != nil {
		testLogger.WithError(err).Error("Failed to get working directory")
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	projectRoot := filepath.Join(wd, "..", "..")
	projectRoot, err = filepath.Abs(projectRoot)
	if err != nil {
		testLogger.WithError(err).Error("Failed to get absolute path")
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	migrationsDir := filepath.Join(projectRoot, "sql", "schema")

	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		testLogger.WithField("path", migrationsDir).Error("Migrations directory not found")
		return fmt.Errorf("migrations directory not found at: %s", migrationsDir)
	}

	testLogger.WithField("path", migrationsDir).Info("Running migrations from directory")

	if err := goose.SetDialect("postgres"); err != nil {
		testLogger.WithError(err).Error("Failed to set goose dialect")
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.Up(migrateDB, migrationsDir); err != nil {
		testLogger.WithError(err).Error("Failed to run migrations")
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	testLogger.Info("Migrations completed successfully")
	return nil
}
