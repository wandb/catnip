package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	Logger zerolog.Logger
)

type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

func init() {
	// Initialize with a basic console writer
	Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
}

// Configure sets up the global logger with the specified level and output
func Configure(level LogLevel, isDev bool) {
	var zeroLevel zerolog.Level
	switch level {
	case LevelDebug:
		zeroLevel = zerolog.DebugLevel
	case LevelInfo:
		zeroLevel = zerolog.InfoLevel
	case LevelWarn:
		zeroLevel = zerolog.WarnLevel
	case LevelError:
		zeroLevel = zerolog.ErrorLevel
	default:
		zeroLevel = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(zeroLevel)

	var writer io.Writer = os.Stderr
	if isDev {
		// Use pretty console output for development
		writer = zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
	}

	Logger = zerolog.New(writer).With().Timestamp().Logger()

	// Update the global logger
	log.Logger = Logger
}

// GetLogLevelFromEnv determines log level from environment variables
func GetLogLevelFromEnv(isDev bool) LogLevel {
	debug := os.Getenv("DEBUG")

	// In dev mode, default to DEBUG=true unless explicitly set to false
	if isDev {
		if strings.ToLower(debug) == "false" || debug == "0" {
			return LevelInfo
		}
		return LevelDebug
	}

	// In production mode, only enable debug if explicitly requested
	if strings.ToLower(debug) == "true" || debug == "1" {
		return LevelDebug
	}

	return LevelInfo
}

// Debug logs a message at debug level
func Debug(msg string) {
	Logger.Debug().Msg(msg)
}

// Debugf logs a formatted message at debug level
func Debugf(format string, args ...interface{}) {
	Logger.Debug().Msgf(format, args...)
}

// Info logs a message at info level
func Info(msg string) {
	Logger.Info().Msg(msg)
}

// Infof logs a formatted message at info level
func Infof(format string, args ...interface{}) {
	Logger.Info().Msgf(format, args...)
}

// Warn logs a message at warn level
func Warn(msg string) {
	Logger.Warn().Msg(msg)
}

// Warnf logs a formatted message at warn level
func Warnf(format string, args ...interface{}) {
	Logger.Warn().Msgf(format, args...)
}

// Error logs a message at error level
func Error(msg string) {
	Logger.Error().Msg(msg)
}

// Errorf logs a formatted message at error level
func Errorf(format string, args ...interface{}) {
	Logger.Error().Msgf(format, args...)
}

// WithField creates a logger with a field
func WithField(key string, value interface{}) zerolog.Logger {
	return Logger.With().Interface(key, value).Logger()
}

// WithFields creates a logger with multiple fields
func WithFields(fields map[string]interface{}) zerolog.Logger {
	logger := Logger.With()
	for k, v := range fields {
		logger = logger.Interface(k, v)
	}
	return logger.Logger()
}
