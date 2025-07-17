package handlers

import (
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/vanpelt/catnip/internal/assets"
)

// HasEmbeddedAssets returns true if frontend assets are embedded
func HasEmbeddedAssets() bool {
	return assets.HasEmbeddedAssets()
}

// ServeEmbeddedAssets serves the embedded frontend assets
func ServeEmbeddedAssets() fiber.Handler {
	// Get the embedded filesystem
	embeddedFS := assets.GetEmbeddedAssets()
	if embeddedFS == nil {
		// No assets embedded, return 404 handler
		return func(c *fiber.Ctx) error {
			return c.Status(404).SendString("Frontend assets not embedded")
		}
	}

	return filesystem.New(filesystem.Config{
		Root:   http.FS(embeddedFS),
		Browse: false,
		Index:  "index.html",
		Next: func(c *fiber.Ctx) bool {
			// Let the SPA handler catch routes that don't exist as files
			return true
		},
	})
}

// ServeEmbeddedSPA handles SPA routing fallback for embedded assets
func ServeEmbeddedSPA(c *fiber.Ctx) error {
	// Get the embedded filesystem
	embeddedFS := assets.GetEmbeddedAssets()
	if embeddedFS == nil {
		return c.Status(404).SendString("Frontend assets not embedded")
	}

	// Try to serve the requested file
	path := strings.TrimPrefix(c.Path(), "/")
	if path == "" {
		path = "index.html"
	}

	// Clean the path to prevent directory traversal
	path = filepath.Clean(path)

	// Check if file exists
	if data, err := fs.ReadFile(embeddedFS, path); err == nil {
		// File exists, serve it with appropriate content type
		contentType := getContentType(path)
		c.Set("Content-Type", contentType)
		return c.Send(data)
	}

	// File doesn't exist, serve index.html for SPA routing
	if data, err := fs.ReadFile(embeddedFS, "index.html"); err == nil {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Send(data)
	}

	return c.Status(404).SendString("Asset not found")
}

// getContentType returns the appropriate content type for a file based on its extension
func getContentType(path string) string {
	ext := filepath.Ext(path)
	contentType := mime.TypeByExtension(ext)

	// Set default content types for common web files if mime doesn't recognize them
	if contentType == "" {
		switch ext {
		case ".js":
			contentType = "application/javascript; charset=utf-8"
		case ".css":
			contentType = "text/css; charset=utf-8"
		case ".html":
			contentType = "text/html; charset=utf-8"
		case ".json":
			contentType = "application/json; charset=utf-8"
		case ".svg":
			contentType = "image/svg+xml"
		case ".ico":
			contentType = "image/x-icon"
		default:
			contentType = "application/octet-stream"
		}
	}

	return contentType
}
