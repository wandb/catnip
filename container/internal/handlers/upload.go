package handlers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/logger"
)

// UploadHandler handles file upload operations
type UploadHandler struct{}

// NewUploadHandler creates a new upload handler
func NewUploadHandler() *UploadHandler {
	return &UploadHandler{}
}

// UploadResponse represents the response after file upload
// @Description Response containing upload status and file location
type UploadResponse struct {
	// Whether the upload succeeded
	Success bool `json:"success" example:"true"`
	// Path where the uploaded file was saved
	FilePath string `json:"filePath" example:"/tmp/uploads/document.pdf"`
	// Status message or error details
	Message string `json:"message,omitempty" example:"File uploaded successfully"`
}

// UploadFile handles file uploads to /tmp/uploads with conflict resolution
// @Summary Upload a file
// @Description Upload a file to /tmp/uploads directory with automatic conflict resolution
// @Tags upload
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "File to upload"
// @Success 200 {object} UploadResponse
// @Failure 400 {object} UploadResponse
// @Failure 500 {object} UploadResponse
// @Router /v1/upload [post]
func (h *UploadHandler) UploadFile(c *fiber.Ctx) error {
	// Ensure uploads directory exists
	uploadsDir := "/tmp/uploads"
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		logger.Errorf("❌ Failed to create uploads directory: %v", err)
		return c.Status(500).JSON(UploadResponse{
			Success: false,
			Message: "Failed to create uploads directory",
		})
	}

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		logger.Errorf("❌ Failed to get uploaded file: %v", err)
		return c.Status(400).JSON(UploadResponse{
			Success: false,
			Message: "No file provided or invalid file",
		})
	}

	// Generate unique filename with conflict resolution
	originalName := file.Filename
	finalPath := h.resolveConflict(uploadsDir, originalName)

	// Open uploaded file
	src, err := file.Open()
	if err != nil {
		logger.Errorf("❌ Failed to open uploaded file: %v", err)
		return c.Status(500).JSON(UploadResponse{
			Success: false,
			Message: "Failed to open uploaded file",
		})
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(finalPath)
	if err != nil {
		logger.Errorf("❌ Failed to create destination file: %v", err)
		return c.Status(500).JSON(UploadResponse{
			Success: false,
			Message: "Failed to create destination file",
		})
	}
	defer dst.Close()

	// Copy file contents
	if _, err := io.Copy(dst, src); err != nil {
		logger.Errorf("❌ Failed to copy file: %v", err)
		return c.Status(500).JSON(UploadResponse{
			Success: false,
			Message: "Failed to copy file",
		})
	}

	logger.Infof("✅ File uploaded successfully: %s", finalPath)
	return c.JSON(UploadResponse{
		Success:  true,
		FilePath: finalPath,
	})
}

// resolveConflict handles filename conflicts by appending numbers
func (h *UploadHandler) resolveConflict(dir, filename string) string {
	// Split filename into name and extension
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	// Try original filename first
	fullPath := filepath.Join(dir, filename)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fullPath
	}

	// File exists, try numbered versions
	counter := 1
	for {
		numberedName := fmt.Sprintf("%s_%d%s", name, counter, ext)
		fullPath = filepath.Join(dir, numberedName)

		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return fullPath
		}

		counter++

		// Safety check to avoid infinite loops
		if counter > 9999 {
			// Use timestamp as fallback
			numberedName = fmt.Sprintf("%s_%d%s", name, counter, ext)
			return filepath.Join(dir, numberedName)
		}
	}
}
