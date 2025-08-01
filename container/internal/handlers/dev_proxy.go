package handlers

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// IsDevMode checks if we're running in development mode
func IsDevMode() bool {
	return os.Getenv("CATNIP_DEV") == "true"
}

func ViteServerURL() string {
	viteServer := os.Getenv("VITE_DEV_SERVER")
	if viteServer == "" {
		port := os.Getenv("VITE_PORT")
		if port == "" {
			port = "5173"
		}
		viteServer = "http://localhost:" + port
	}
	return viteServer
}

// ProxyToVite proxies requests to Vite dev server
func ProxyToVite(c *fiber.Ctx) error {
	viteServer := ViteServerURL()

	// Build the target URL
	targetURL := viteServer + c.OriginalURL()
	viteServerURL, err := url.Parse(viteServer)
	if err != nil {
		return c.Status(500).SendString("Failed to parse Vite server URL")
	}
	viteHost := viteServerURL.Host

	// Create HTTP client
	client := &http.Client{}

	// Create request to Vite server
	req, err := http.NewRequest(c.Method(), targetURL, strings.NewReader(string(c.Body())))
	if err != nil {
		return c.Status(500).SendString("Failed to create proxy request")
	}

	// Copy headers from original request
	for key, values := range c.GetReqHeaders() {
		if len(values) > 0 {
			req.Header.Set(key, values[0])
		}
	}

	// Set Host header for proper proxying
	req.Header.Set("Host", viteHost)

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return c.Status(500).SendString("Failed to proxy to Vite server")
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Response().Header.Add(key, value)
		}
	}

	// Set status code
	c.Status(resp.StatusCode)

	// Copy response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(500).SendString("Failed to read proxy response")
	}

	return c.Send(body)
}
