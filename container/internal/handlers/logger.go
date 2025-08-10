package handlers

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	catniplogger "github.com/vanpelt/catnip/internal/logger"
)

// SamplingLogger creates a custom logger middleware that samples certain endpoints and filters frontend assets
func SamplingLogger() fiber.Handler {
	// Counter for /v1/ports endpoint
	var portsCounter uint64
	var counterMu sync.Mutex

	// Default logger for most endpoints
	defaultLogger := logger.New(logger.Config{
		Format: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path} | ${error}\n",
	})

	// Regex patterns for requests we want to keep logging
	proxyPattern := regexp.MustCompile(`^/\d+/`)           // /1234/anything
	workspacePattern := regexp.MustCompile(`^/workspace/`) // /workspace/anything (index.html fallbacks)

	// Common frontend asset extensions to filter out
	assetExtensions := []string{
		".js", ".css", ".map", ".ico", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp",
		".woff", ".woff2", ".ttf", ".eot", ".mp4", ".webm", ".mp3", ".wav",
	}

	isAssetRequest := func(path string) bool {
		// Check if path ends with any asset extension
		lowerPath := strings.ToLower(path)
		for _, ext := range assetExtensions {
			if strings.HasSuffix(lowerPath, ext) {
				return true
			}
		}
		return false
	}

	shouldLogRequest := func(path string) bool {
		// Always log API endpoints
		if strings.HasPrefix(path, "/v1/") {
			return true
		}

		// Always log git repo requests
		if strings.Contains(path, ".git/") {
			return true
		}

		// Always log proxy requests
		if proxyPattern.MatchString(path) {
			return true
		}

		// Log workspace routes (index.html fallbacks) but not if they're asset requests
		if workspacePattern.MatchString(path) && !isAssetRequest(path) {
			return true
		}

		// Filter out frontend assets
		if isAssetRequest(path) {
			return false
		}

		// Log everything else by default (health checks, root requests, etc.)
		return true
	}

	return func(c *fiber.Ctx) error {
		path := c.Path()

		// Check if this is the /v1/ports endpoint (keep existing sampling behavior)
		if path == "/v1/ports" {
			counterMu.Lock()
			portsCounter++
			currentCount := portsCounter
			counterMu.Unlock()

			// Only log every 10th request
			if currentCount == 10 {
				start := time.Now()
				err := c.Next()
				duration := time.Since(start)

				// Log a summary instead of individual request
				catniplogger.Debugf("%d | %v | %s | %s | %s | - [sampled: %d calls]",
					c.Response().StatusCode(),
					duration,
					c.IP(),
					c.Method(),
					path,
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

		// Check if we should log this request
		if !shouldLogRequest(path) {
			// Skip logging for frontend assets, just process the request
			return c.Next()
		}

		// Use default logger for all other endpoints
		return defaultLogger(c)
	}
}
