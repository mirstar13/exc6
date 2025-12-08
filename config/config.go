package config

import (
	"fmt"
	"os"
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
}

type ServerConfig struct {
	Host         string
	Port         int
	ViewsDir     string
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
	TTL        time.Duration
	CookieName string
}

type RateLimitConfig struct {
	Capacity     int64
	RefillRate   int64
	RefillPeriod time.Duration
}

type DatabaseConfig struct {
	Path string
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			Port:         getEnvAsInt("SERVER_PORT", 8080),
			ViewsDir:     getEnv("VIEWS_DIR", "./server/views"),
			UploadsDir:   getEnv("UPLOADS_DIR", "./server/uploads"),
			LogFile:      getEnv("LOG_FILE", "log/server.log"),
			ReadTimeout:  getEnvAsDuration("READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getEnvAsDuration("WRITE_TIMEOUT", 10*time.Second),
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
			IconsDir: getEnv("ICONS_DIR", "./server/uploads/icons"),
		},
		Session: SessionConfig{
			TTL:        getEnvAsDuration("SESSION_TTL", 24*time.Hour),
			CookieName: getEnv("SESSION_COOKIE_NAME", "session_id"),
		},
		RateLimit: RateLimitConfig{
			Capacity:     getEnvAsInt64("RATE_LIMIT_CAPACITY", 20),
			RefillRate:   getEnvAsInt64("RATE_LIMIT_REFILL", 5),
			RefillPeriod: getEnvAsDuration("RATE_LIMIT_PERIOD", time.Second),
		},
		Database: DatabaseConfig{
			Path: getEnv("DATABASE_PATH", "users.json"),
		},
	}

	return cfg, cfg.Validate()
}

func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Redis.Address == "" {
		return fmt.Errorf("redis address is required")
	}
	if c.Kafka.Address == "" {
		return fmt.Errorf("kafka address is required")
	}
	if c.Upload.MaxFileSize <= 0 {
		return fmt.Errorf("invalid max file size: %d", c.Upload.MaxFileSize)
	}
	return nil
}

func (c *Config) ServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
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
