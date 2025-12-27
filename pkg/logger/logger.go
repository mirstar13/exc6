package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
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

// Logger provides structured logging capabilities
type Logger struct {
	logger *log.Logger
	level  Level
	fields map[string]any
}

// New creates a new logger instance
func New(logfile string) *Logger {
	if logfile != "" {
		file, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(fmt.Sprintf("Failed to open log file: %v", err))
		}

		return &Logger{
			logger: log.New(file, "", 0),
			level:  INFO,
			fields: make(map[string]any),
		}
	} else {
		return &Logger{
			logger: log.New(os.Stdout, "", 0),
			level:  INFO,
			fields: make(map[string]any),
		}
	}
}

// SetLevel sets the minimum logging level
func (l *Logger) SetLevel(level Level) {
	l.level = level
}

// SetOutput sets the output destination
func (l *Logger) SetOutput(output *os.File) {
	l.logger.SetOutput(output)
}

// WithField adds a field to the logger
func (l *Logger) WithField(key string, value any) *Logger {
	newLogger := &Logger{
		logger: l.logger,
		level:  l.level,
		fields: make(map[string]any),
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
		logger: l.logger,
		level:  l.level,
		fields: make(map[string]any),
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

// [FIX] Added helper to easily trace request IDs
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

	// Build the log entry
	var fields []string
	if len(l.fields) > 0 {
		for k, v := range l.fields {
			fields = append(fields, fmt.Sprintf("%s=%v", k, v))
		}
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("[%s] %s: %s", timestamp, level.String(), message)

	if len(fields) > 0 {
		logEntry += " | " + strings.Join(fields, " ")
	}

	l.logger.Println(logEntry)
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
var defaultLogger = New("./log/server.log")

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
