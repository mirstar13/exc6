package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Level represents the logging level
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// String returns the string representation of the level
func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Config configures the logger with rotation settings
type Config struct {
	// Filename is the file to write logs to
	Filename string

	// MaxSize is the maximum size in megabytes before rotation
	MaxSize int // Default: 100 MB

	// MaxBackups is the maximum number of old log files to retain
	MaxBackups int // Default: 3

	// MaxAge is the maximum number of days to retain old log files
	MaxAge int // Default: 28 days

	// Compress determines if rotated logs should be compressed
	Compress bool // Default: true

	// LocalTime determines if rotated log file names use local time
	LocalTime bool // Default: false (UTC)

	// Level is the minimum logging level
	Level Level // Default: INFO

	// Output allows setting custom output writer (for testing)
	Output io.Writer
}

// DefaultConfig returns sensible defaults
func DefaultConfig(filename string) Config {
	return Config{
		Filename:   filename,
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
		LocalTime:  true,
		Level:      INFO,
	}
}

// Logger provides structured logging capabilities with rotation
type Logger struct {
	logger       *log.Logger
	level        Level
	fields       map[string]any
	rotator      *lumberjack.Logger // nil for stdout/custom writers
	OutputWriter io.Writer          // the actual writer (could be stdout, rotator, or custom)
}

// NewWithConfig creates a new logger with rotation configuration
func NewWithConfig(cfg Config) (*Logger, error) {
	var writer io.Writer

	if cfg.Output != nil {
		// Use custom output (useful for testing)
		writer = cfg.Output
	} else if cfg.Filename == "" || cfg.Filename == "-" || cfg.Filename == "stdout" {
		// Log to stdout
		writer = os.Stdout
	} else {
		// Ensure log directory exists
		logDir := filepath.Dir(cfg.Filename)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory %s: %w", logDir, err)
		}

		// Setup rotating file logger
		rotator := &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
			LocalTime:  cfg.LocalTime,
		}

		writer = rotator

		return &Logger{
			logger:       log.New(writer, "", 0),
			level:        cfg.Level,
			fields:       make(map[string]any),
			rotator:      rotator,
			OutputWriter: writer,
		}, nil
	}

	return &Logger{
		logger:       log.New(writer, "", 0),
		level:        cfg.Level,
		fields:       make(map[string]any),
		rotator:      nil,
		OutputWriter: writer,
	}, nil
}

// New creates a new logger with default rotation settings
func New(logfile string) *Logger {
	logger, err := NewWithConfig(DefaultConfig(logfile))
	if err != nil {
		// Fallback to stdout if file creation fails
		fmt.Fprintf(os.Stderr, "Warning: failed to create log file %s: %v. Falling back to stdout.\n", logfile, err)
		logger, _ = NewWithConfig(Config{Output: os.Stdout, Level: INFO})
	}
	return logger
}

// Rotate triggers an immediate log rotation
func (l *Logger) Rotate() error {
	if l.rotator != nil {
		return l.rotator.Rotate()
	}
	return nil
}

// Close closes the log file if using rotation
func (l *Logger) Close() error {
	if l.rotator != nil {
		return l.rotator.Close()
	}
	return nil
}

// SetLevel sets the minimum logging level
func (l *Logger) SetLevel(level Level) {
	l.level = level
}

// SetOutput sets the output destination (mainly for testing)
func (l *Logger) SetOutput(output io.Writer) {
	l.logger.SetOutput(output)
	l.OutputWriter = output
	l.rotator = nil // Disable rotation when custom output is set
}

// WithField adds a field to the logger
func (l *Logger) WithField(key string, value any) *Logger {
	newLogger := &Logger{
		logger:       l.logger,
		level:        l.level,
		fields:       make(map[string]any),
		rotator:      l.rotator,
		OutputWriter: l.OutputWriter,
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value
	return newLogger
}

// WithFields adds multiple fields to the logger
func (l *Logger) WithFields(fields map[string]any) *Logger {
	newLogger := &Logger{
		logger:       l.logger,
		level:        l.level,
		fields:       make(map[string]any),
		rotator:      l.rotator,
		OutputWriter: l.OutputWriter,
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return newLogger
}

func (l *Logger) WithError(err error) *Logger {
	return l.WithField("error", err)
}

func (l *Logger) WithRequestID(requestID string) *Logger {
	return l.WithField("request_id", requestID)
}

// log formats and writes a log message
func (l *Logger) log(level Level, msg string, args ...any) {
	if level < l.level {
		return
	}

	// Format the message
	message := fmt.Sprintf(msg, args...)

	// Build the log entry with structured fields
	var fields []string
	if len(l.fields) > 0 {
		for k, v := range l.fields {
			// Format value based on type for better readability
			var formattedValue string
			switch val := v.(type) {
			case string:
				formattedValue = val
			case error:
				formattedValue = val.Error()
			case fmt.Stringer:
				formattedValue = val.String()
			default:
				formattedValue = fmt.Sprintf("%v", val)
			}
			fields = append(fields, fmt.Sprintf("%s=%s", k, formattedValue))
		}
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logEntry := fmt.Sprintf("[%s] %s: %s", timestamp, level.String(), message)

	if len(fields) > 0 {
		logEntry += " | " + strings.Join(fields, " | ")
	}

	l.logger.Println(logEntry)
}

func (l *Logger) Printf(format string, args ...any) {
	l.log(INFO, format, args...)
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...any) {
	l.log(DEBUG, msg, args...)
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...any) {
	l.log(INFO, msg, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...any) {
	l.log(WARN, msg, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...any) {
	l.log(ERROR, msg, args...)
}

// Fatal logs a fatal error and exits
func (l *Logger) Fatal(msg string, args ...any) {
	l.log(ERROR, msg, args...)
	os.Exit(1)
}

// Global default logger
var defaultLogger *Logger

func init() {
	// Initialize with stdout by default
	defaultLogger, _ = NewWithConfig(Config{Output: os.Stdout, Level: INFO})
}

// SetDefault sets the default global logger
func SetDefault(logger *Logger) {
	defaultLogger = logger
}

// GetDefault returns the default global logger
func GetDefault() *Logger {
	return defaultLogger
}

// Info logs using the default logger
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Warn logs using the default logger
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Error logs using the default logger
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// Debug logs using the default logger
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// WithField returns a logger with a field using the default logger
func WithField(key string, value any) *Logger {
	return defaultLogger.WithField(key, value)
}

// WithFields returns a logger with fields using the default logger
func WithFields(fields map[string]any) *Logger {
	return defaultLogger.WithFields(fields)
}

func WithError(err error) *Logger {
	return defaultLogger.WithError(err)
}

func WithRequestID(requestID string) *Logger {
	return defaultLogger.WithRequestID(requestID)
}
