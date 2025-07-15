package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

// SamplingLogger creates a custom logger middleware that samples certain endpoints
func SamplingLogger() fiber.Handler {
	// Counter for /v1/ports endpoint
	var portsCounter uint64
	var counterMu sync.Mutex

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

				// Log a summary instead of individual request
				log.Printf("%d | %v | %s | %s | %s | - [sampled: %d calls]",
					c.Response().StatusCode(),
					duration,
					c.IP(),
					c.Method(),
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
