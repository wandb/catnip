package handlers

import (
	"io/fs"
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
		// File exists, serve it
		return c.Send(data)
	}

	// File doesn't exist, serve index.html for SPA routing
	if data, err := fs.ReadFile(embeddedFS, "index.html"); err == nil {
		return c.Send(data)
	}
	
	return c.Status(404).SendString("Asset not found")
}

