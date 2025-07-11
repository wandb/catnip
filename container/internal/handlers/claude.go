package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
	
	// Import models for swagger documentation
	_ "github.com/vanpelt/catnip/internal/models"
)

// ClaudeHandler handles Claude Code session-related API endpoints
type ClaudeHandler struct {
	claudeService *services.ClaudeService
}

// NewClaudeHandler creates a new Claude handler
func NewClaudeHandler(claudeService *services.ClaudeService) *ClaudeHandler {
	return &ClaudeHandler{
		claudeService: claudeService,
	}
}

// GetWorktreeSessionSummary returns Claude session information for a specific worktree
// @Summary Get worktree session summary
// @Description Returns Claude Code session metadata for a specific worktree
// @Tags claude
// @Produce json
// @Param worktree_path query string true "Worktree path"
// @Success 200 {object} models.ClaudeSessionSummary
// @Router /v1/claude/session [get]
func (h *ClaudeHandler) GetWorktreeSessionSummary(c *fiber.Ctx) error {
	worktreePath := c.Query("worktree_path")
	if worktreePath == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "worktree_path query parameter is required",
		})
	}
	
	summary, err := h.claudeService.GetWorktreeSessionSummary(worktreePath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	
	if summary == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No Claude session found for this worktree",
		})
	}
	
	return c.JSON(summary)
}

// GetAllWorktreeSessionSummaries returns Claude session information for all worktrees
// @Summary Get all worktree session summaries
// @Description Returns Claude Code session metadata for all worktrees with Claude data
// @Tags claude
// @Produce json
// @Success 200 {object} map[string]models.ClaudeSessionSummary
// @Router /v1/claude/sessions [get]
func (h *ClaudeHandler) GetAllWorktreeSessionSummaries(c *fiber.Ctx) error {
	summaries, err := h.claudeService.GetAllWorktreeSessionSummaries()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	
	return c.JSON(summaries)
}

// GetSessionByUUID returns complete session data for a specific session UUID
// @Summary Get session by UUID
// @Description Returns complete session data including all messages for a specific session UUID
// @Tags claude
// @Produce json
// @Param uuid path string true "Session UUID"
// @Success 200 {object} models.FullSessionData
// @Router /v1/claude/session/{uuid} [get]
func (h *ClaudeHandler) GetSessionByUUID(c *fiber.Ctx) error {
	sessionUUID := c.Params("uuid")
	if sessionUUID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "session UUID is required",
		})
	}
	
	sessionData, err := h.claudeService.GetSessionByUUID(sessionUUID)
	if err != nil {
		if err.Error() == "session not found: "+sessionUUID {
			return c.Status(404).JSON(fiber.Map{
				"error": "Session not found",
				"uuid": sessionUUID,
			})
		}
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get session data",
			"details": err.Error(),
		})
	}
	
	return c.JSON(sessionData)
}