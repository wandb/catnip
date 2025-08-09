package logger

import (
	"fmt"
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
	// LevelDebug enables debug level logging
	LevelDebug LogLevel = "debug"
	// LevelInfo enables info level logging
	LevelInfo LogLevel = "info"
	// LevelWarn enables warn level logging
	LevelWarn LogLevel = "warn"
	// LevelError enables error level logging
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
		// Use pretty console output for development with custom format to match Fiber logs
		writer = zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: "15:04:05", // Short time format like Fiber
			NoColor:    false,
			FormatMessage: func(i interface{}) string {
				return fmt.Sprintf("| %s", i)
			},
			FormatLevel: func(i interface{}) string {
				var l string
				if ll, ok := i.(string); ok {
					switch ll {
					case "debug":
						l = "DBG"
					case "info":
						l = "INF"
					case "warn":
						l = "WRN"
					case "error":
						l = "ERR"
					case "fatal":
						l = "FTL"
					default:
						l = strings.ToUpper(ll)
					}
				}
				return l
			},
			FormatTimestamp: func(i interface{}) string {
				if ts, ok := i.(string); ok {
					// Parse the timestamp and format it as HH:MM:SS
					if t, err := time.Parse(time.RFC3339, ts); err == nil {
						return fmt.Sprintf("%s |", t.Format("15:04:05"))
					}
				}
				return fmt.Sprintf("%s |", i)
			},
		}
	}

	Logger = zerolog.New(writer).With().Timestamp().Logger()

	// Update the global logger
	log.Logger = Logger
}

// ConfigureForTUI sets up the global logger to write to debug file instead of stderr
// This prevents log output from corrupting the TUI display
func ConfigureForTUI(level LogLevel, isDev bool) {
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

	// Always write to debug file when running TUI to avoid corrupting display
	file, err := os.OpenFile("/tmp/catnip-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		// Fallback to stderr if we can't open the debug file
		Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
		log.Logger = Logger
		return
	}

	var writer io.Writer = file
	if isDev {
		// Use pretty console output for development, but write to file with custom format to match Fiber logs
		writer = zerolog.ConsoleWriter{
			Out:        file,
			TimeFormat: "15:04:05", // Short time format like Fiber
			NoColor:    true,       // Disable color codes in file
			FormatMessage: func(i interface{}) string {
				return fmt.Sprintf("| %s", i)
			},
			FormatLevel: func(i interface{}) string {
				var l string
				if ll, ok := i.(string); ok {
					switch ll {
					case "debug":
						l = "DBG"
					case "info":
						l = "INF"
					case "warn":
						l = "WRN"
					case "error":
						l = "ERR"
					case "fatal":
						l = "FTL"
					default:
						l = strings.ToUpper(ll)
					}
				}
				return l
			},
			FormatTimestamp: func(i interface{}) string {
				if ts, ok := i.(string); ok {
					// Parse the timestamp and format it as HH:MM:SS
					if t, err := time.Parse(time.RFC3339, ts); err == nil {
						return fmt.Sprintf("%s |", t.Format("15:04:05"))
					}
				}
				return fmt.Sprintf("%s |", i)
			},
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

// Fatal logs a message at fatal level and exits
func Fatal(msg string) {
	Logger.Fatal().Msg(msg)
}

// Fatalf logs a formatted message at fatal level and exits
func Fatalf(format string, args ...interface{}) {
	Logger.Fatal().Msgf(format, args...)
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
