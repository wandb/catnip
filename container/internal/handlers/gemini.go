package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// GeminiHandler handles Gemini CLI session-related API endpoints.
type GeminiHandler struct {
	geminiService *services.GeminiService
}

// NewGeminiHandler creates a new Gemini handler.
func NewGeminiHandler(geminiService *services.GeminiService) *GeminiHandler {
	return &GeminiHandler{
		geminiService: geminiService,
	}
}

// GetAllWorktreeSessionSummaries returns Gemini session information for all worktrees.
func (h *GeminiHandler) GetAllWorktreeSessionSummaries(c *fiber.Ctx) error {
	summaries, err := h.geminiService.GetAllWorktreeSessionSummaries()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get Gemini session summaries",
		})
	}
	return c.JSON(summaries)
}

// GetSessionByUUID returns a single Gemini session by its UUID.
func (h *GeminiHandler) GetSessionByUUID(c *fiber.Ctx) error {
	sessionUUID := c.Params("uuid")
	sessionData, err := h.geminiService.GetSessionByUUID(sessionUUID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Gemini session not found",
		})
	}
	return c.JSON(sessionData)
}

// GetSessionMessages returns all messages for a given Gemini session.
func (h *GeminiHandler) GetSessionMessages(c *fiber.Ctx) error {
	sessionUUID := c.Params("uuid")
	messages, err := h.geminiService.GetSessionMessages(sessionUUID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Gemini session not found",
		})
	}
	return c.JSON(messages)
}
