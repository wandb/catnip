package handlers

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/mattn/go-isatty"
)

// Color constants for terminal output
const (
	cBlack   = "\u001b[90m"
	cRed     = "\u001b[91m"
	cGreen   = "\u001b[92m"
	cYellow  = "\u001b[93m"
	cBlue    = "\u001b[94m"
	cMagenta = "\u001b[95m"
	cCyan    = "\u001b[96m"
	cWhite   = "\u001b[97m"
	cReset   = "\u001b[0m"
)

// getStatusColor returns the appropriate color for HTTP status codes
func getStatusColor(status int, enableColors bool) string {
	if !enableColors {
		return ""
	}

	switch {
	case status >= 200 && status < 300:
		return cGreen
	case status >= 300 && status < 400:
		return cBlue
	case status >= 400 && status < 500:
		return cYellow
	default:
		return cRed
	}
}

// getMethodColor returns the appropriate color for HTTP methods
func getMethodColor(method string, enableColors bool) string {
	if !enableColors {
		return ""
	}

	switch method {
	case "GET":
		return cCyan
	case "POST":
		return cGreen
	case "PUT":
		return cYellow
	case "DELETE":
		return cRed
	case "PATCH":
		return cMagenta
	case "HEAD":
		return cBlue
	case "OPTIONS":
		return cWhite
	default:
		return cReset
	}
}

// SamplingLogger creates a custom logger middleware that samples certain endpoints
func SamplingLogger() fiber.Handler {
	// Counter for /v1/ports endpoint
	var portsCounter uint64
	var counterMu sync.Mutex

	// Check if colors should be enabled
	enableColors := isatty.IsTerminal(os.Stdout.Fd()) && os.Getenv("NO_COLOR") != "1" && os.Getenv("TERM") != "dumb"

	// Default logger for most endpoints
	defaultLogger := logger.New(logger.Config{
		Format: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path} | ${error}\n",
	})

	return func(c *fiber.Ctx) error {
		// Check if this is the /v1/ports endpoint
		if c.Path() == "/v1/ports" {
			counterMu.Lock()
			portsCounter++
			currentCount := portsCounter
			counterMu.Unlock()

			// Only log every 10th request
			if currentCount == 10 {
				start := time.Now()
				err := c.Next()
				duration := time.Since(start)

				// Format log to match regular logger exactly
				status := c.Response().StatusCode()
				method := c.Method()

				statusColor := getStatusColor(status, enableColors)
				methodColor := getMethodColor(method, enableColors)
				resetColor := ""
				if enableColors {
					resetColor = cReset
				}

				// Log with exact same format as regular logger
				fmt.Printf("%s | %s%d%s | %13s | %s | %s%s%s | %s | - [sampled: %d calls]\n",
					time.Now().Format("15:04:05"),
					statusColor,
					status,
					resetColor,
					duration,
					c.IP(),
					methodColor,
					method,
					resetColor,
					c.Path(),
					currentCount)

				// Reset the counter after logging
				counterMu.Lock()
				portsCounter = 0
				counterMu.Unlock()

				return err
			}

			// Don't log, just process the request
			return c.Next()
		}

		// Use default logger for all other endpoints
		return defaultLogger(c)
	}
}
