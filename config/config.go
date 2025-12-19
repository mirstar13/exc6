package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Server    ServerConfig
	Redis     RedisConfig
	Kafka     KafkaConfig
	Upload    UploadConfig
	Session   SessionConfig
	RateLimit RateLimitConfig
	Database  DatabaseConfig
	SSE       SSEConfig
}

type ServerConfig struct {
	Host         string
	Port         int
	ViewsDir     string
	StaticDir    string
	ScriptsDir   string
	UploadsDir   string
	LogFile      string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type RedisConfig struct {
	Address  string
	Username string
	Password string
	DB       int
}

type KafkaConfig struct {
	Address string
	Topic   string
}

type UploadConfig struct {
	MaxFileSize       int64
	AllowedMimeTypes  []string
	AllowedExtensions []string
	IconsDir          string
}

type SessionConfig struct {
	TTL             time.Duration
	CookieName      string
	UpdateThreshold time.Duration // Minimum time between session updates
}

type SSEConfig struct {
	KeepAliveInterval time.Duration // Interval for sending keep-alive pings
}

type RateLimitConfig struct {
	Capacity     int64
	RefillRate   int64
	RefillPeriod time.Duration
}

type DatabaseConfig struct {
	ConnectionString string
}

// getProjectRoot finds the project root by looking for go.mod
func getProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if PROJECT_ROOT env var is set (useful for tests)
	if projectRoot := os.Getenv("PROJECT_ROOT"); projectRoot != "" {
		return projectRoot, nil
	}

	// Walk up the directory tree looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root without finding go.mod
			return "", fmt.Errorf("could not find project root (no go.mod found)")
		}
		dir = parent
	}
}

// resolvePath resolves a path relative to the project root if it's not absolute
func resolvePath(path string) (string, error) {
	// If it's already an absolute path, return it
	if filepath.IsAbs(path) {
		return path, nil
	}

	// Get project root
	projectRoot, err := getProjectRoot()
	if err != nil {
		return "", err
	}

	// Join with project root
	return filepath.Join(projectRoot, path), nil
}

func Load() (*Config, error) {
	// Resolve paths relative to project root
	viewsDir, err := resolvePath(getEnv("VIEWS_DIR", "./server/views"))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve views directory: %w", err)
	}

	uploadsDir, err := resolvePath(getEnv("UPLOADS_DIR", "./server/uploads"))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve uploads directory: %w", err)
	}

	logFile, err := resolvePath(getEnv("LOG_FILE", "./log/server.log"))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve log file: %w", err)
	}

	staticDir, err := resolvePath(getEnv("STATIC_DIR", "./static"))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve static directory: %w", err)
	}

	scriptsDir, err := resolvePath(getEnv("SCRIPTS_DIR", "./scripts"))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve scripts directory: %w", err)
	}

	iconsDir, err := resolvePath(getEnv("ICONS_DIR", "./server/uploads/icons"))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve icons directory: %w", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			Port:         getEnvAsInt("SERVER_PORT", 8000),
			ViewsDir:     viewsDir,
			UploadsDir:   uploadsDir,
			StaticDir:    staticDir,
			ScriptsDir:   scriptsDir,
			LogFile:      logFile,
			ReadTimeout:  getEnvAsDuration("READ_TIMEOUT", 5*time.Minute),
			WriteTimeout: 0, // No write timeout by default (needed for SSE)
		},
		Redis: RedisConfig{
			Address:  getEnv("REDIS_ADDR", "localhost:6379"),
			Username: getEnv("REDIS_USERNAME", "default"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
		Kafka: KafkaConfig{
			Address: getEnv("KAFKA_ADDR", "localhost:9092"),
			Topic:   getEnv("KAFKA_TOPIC", "chat-history"),
		},
		Upload: UploadConfig{
			MaxFileSize: getEnvAsInt64("MAX_FILE_SIZE", 5*1024*1024), // 5MB
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
			IconsDir: iconsDir,
		},
		Session: SessionConfig{
			TTL:             getEnvAsDuration("SESSION_TTL", 24*time.Hour),
			CookieName:      getEnv("SESSION_COOKIE_NAME", "session_id"),
			UpdateThreshold: getEnvAsDuration("SESSION_UPDATE_THRESHOLD", 60*time.Second),
		},
		SSE: SSEConfig{
			KeepAliveInterval: getEnvAsDuration("SSE_KEEPALIVE_INTERVAL", 15*time.Second),
		},
		RateLimit: RateLimitConfig{
			Capacity:     getEnvAsInt64("RATE_LIMIT_CAPACITY", 200),
			RefillRate:   getEnvAsInt64("RATE_LIMIT_REFILL", 10),
			RefillPeriod: getEnvAsDuration("RATE_LIMIT_PERIOD", time.Second),
		},
		Database: DatabaseConfig{
			ConnectionString: getEnv("GOOSE_DBSTRING", ""),
		},
	}

	return cfg, cfg.Validate()
}

func (c *Config) Validate() error {
	var errors []string

	// Server validation
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errors = append(errors, fmt.Sprintf("invalid server port: %d (must be 1-65535)", c.Server.Port))
	}
	if c.Server.ViewsDir == "" {
		errors = append(errors, "views directory (VIEWS_DIR) is required")
	}
	if c.Server.UploadsDir == "" {
		errors = append(errors, "uploads directory (UPLOADS_DIR) is required")
	}

	// Redis validation
	if c.Redis.Address == "" {
		errors = append(errors, "redis address (REDIS_ADDR) is required")
	}
	if c.Redis.Username == "" {
		errors = append(errors, "redis username (REDIS_USERNAME) is required")
	}

	// Kafka validation
	if c.Kafka.Address == "" {
		errors = append(errors, "kafka address (KAFKA_ADDR) is required")
	}
	if c.Kafka.Topic == "" {
		errors = append(errors, "kafka topic (KAFKA_TOPIC) is required")
	}

	// Database validation
	if c.Database.ConnectionString == "" {
		errors = append(errors, "database connection string (GOOSE_DBSTRING) is required")
	}

	// Upload validation
	if c.Upload.MaxFileSize <= 0 {
		errors = append(errors, fmt.Sprintf("invalid max file size: %d (must be > 0)", c.Upload.MaxFileSize))
	}
	if len(c.Upload.AllowedMimeTypes) == 0 {
		errors = append(errors, "at least one allowed MIME type is required")
	}
	if c.Upload.IconsDir == "" {
		errors = append(errors, "icons directory (ICONS_DIR) is required")
	}

	// Session validation
	if c.Session.TTL <= 0 {
		errors = append(errors, "session TTL must be > 0")
	}
	if c.Session.CookieName == "" {
		errors = append(errors, "session cookie name (SESSION_COOKIE_NAME) is required")
	}
	if c.Session.UpdateThreshold <= 0 {
		errors = append(errors, "session update threshold must be > 0")
	}

	// SSE validation
	if c.SSE.KeepAliveInterval <= 0 {
		errors = append(errors, "SSE keep-alive interval must be > 0")
	}

	// Rate limit validation
	if c.RateLimit.Capacity <= 0 {
		errors = append(errors, "rate limit capacity must be > 0")
	}
	if c.RateLimit.RefillRate <= 0 {
		errors = append(errors, "rate limit refill rate must be > 0")
	}
	if c.RateLimit.RefillPeriod <= 0 {
		errors = append(errors, "rate limit refill period must be > 0")
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", joinErrors(errors))
	}

	return nil
}

func joinErrors(errors []string) string {
	result := ""
	for i, err := range errors {
		if i > 0 {
			result += "\n  - "
		}
		result += err
	}
	return result
}

func (c *Config) ServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// PrintSummary logs a summary of the loaded configuration
func (c *Config) PrintSummary() {
	fmt.Println("Configuration Summary:")
	fmt.Printf("  Server: %s\n", c.ServerAddress())
	fmt.Printf("  Redis: %s (DB: %d)\n", c.Redis.Address, c.Redis.DB)
	fmt.Printf("  Kafka: %s (Topic: %s)\n", c.Kafka.Address, c.Kafka.Topic)
	fmt.Printf("  Database: %s\n", maskConnectionString(c.Database.ConnectionString))
	fmt.Printf("  Session TTL: %s\n", c.Session.TTL)
	fmt.Printf("  Upload Max Size: %.2f MB\n", float64(c.Upload.MaxFileSize)/(1024*1024))
	fmt.Printf("  Rate Limit: %d requests/%s (capacity: %d)\n",
		c.RateLimit.RefillRate, c.RateLimit.RefillPeriod, c.RateLimit.Capacity)
}

// maskConnectionString masks sensitive parts of the connection string
func maskConnectionString(connStr string) string {
	if len(connStr) < 20 {
		return "***"
	}
	// Show first 20 chars and mask the rest
	return connStr[:20] + "..." + connStr[len(connStr)-10:]
}

// Helper functions to read environment variables with defaults
func getEnv(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	valStr := os.Getenv(key)
	if val, err := strconv.Atoi(valStr); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsInt64(key string, defaultVal int64) int64 {
	valStr := os.Getenv(key)
	if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsDuration(key string, defaultVal time.Duration) time.Duration {
	valStr := os.Getenv(key)
	if val, err := time.ParseDuration(valStr); err == nil {
		return val
	}
	return defaultVal
}
