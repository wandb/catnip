package handlers

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// SamplingLogger creates a custom logger middleware that samples certain endpoints and filters frontend assets
func SamplingLogger() fiber.Handler {
	// Counter for /v1/ports endpoint
	var portsCounter uint64
	var counterMu sync.Mutex

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

	shouldLogRequest := func(c *fiber.Ctx, path string) bool {
		// Skip /health endpoint unless it's not returning 200
		if path == "/health" && c.Response().StatusCode() == 200 {
			return false
		}

		// Always log API endpoints (except /health which is handled above)
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

		// Filter out development source files (Vite dev server)
		if strings.HasPrefix(path, "/src/") || strings.HasPrefix(path, "/node_modules/") || strings.HasPrefix(path, "/@") {
			return false
		}

		// Filter out frontend assets
		if isAssetRequest(path) {
			return false
		}

		// Log everything else by default (root requests, etc.)
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

				// Log a summary instead of individual request (using direct fmt.Printf for consistency)
				fmt.Printf("%s | \033[32m%d\033[0m | %v | %s | \033[96m%s\033[0m | %s | - [sampled: %d calls]\n",
					time.Now().Format("15:04:05"),
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

		// Capture start time for duration measurement
		start := time.Now()

		// Process the request
		err := c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Check if we should log this request (after we have the response status)
		if !shouldLogRequest(c, path) {
			// Skip logging for filtered requests
			return err
		}

		// Log the request details using Fiber-style format
		errMsg := "-"
		if err != nil {
			errMsg = err.Error()
		}

		// Format the log message directly without log level prefix for request logs
		statusCode := c.Response().StatusCode()
		var statusColor string
		switch {
		case statusCode >= 500:
			statusColor = "\033[31m" // Red for 5xx
		case statusCode >= 400:
			statusColor = "\033[33m" // Yellow for 4xx
		case statusCode >= 300:
			statusColor = "\033[36m" // Cyan for 3xx
		case statusCode >= 200:
			statusColor = "\033[32m" // Green for 2xx
		default:
			statusColor = "\033[35m" // Magenta for 1xx
		}

		// Print directly to match Fiber's format (no log level prefix for request logs)
		fmt.Printf("%s | %s%d\033[0m | %v | %s | \033[96m%s\033[0m | %s | %s\n",
			time.Now().Format("15:04:05"),
			statusColor,
			statusCode,
			duration,
			c.IP(),
			c.Method(),
			path,
			errMsg)

		return err
	}
}
