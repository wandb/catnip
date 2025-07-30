package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
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
				"uuid":  sessionUUID,
			})
		}
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to get session data",
			"details": err.Error(),
		})
	}

	return c.JSON(sessionData)
}

// CreateCompletion handles requests to create completions using claude CLI subprocess
// @Summary Create Claude messages using CLI
// @Description Creates a completion using the claude CLI tool as a subprocess, supporting both streaming and non-streaming responses, with resume functionality
// @Tags claude
// @Accept json
// @Produce json
// @Param request body github_com_vanpelt_catnip_internal_models.CreateCompletionRequest true "Create completion request"
// @Success 200 {object} github_com_vanpelt_catnip_internal_models.CreateCompletionResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/claude/messages [post]
func (h *ClaudeHandler) CreateCompletion(c *fiber.Ctx) error {
	var req models.CreateCompletionRequest

	// Parse the request body
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.Prompt == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Prompt is required",
		})
	}

	// Create context for the request
	ctx := c.Context()

	// Handle streaming response
	if req.Stream {
		// Set headers for streaming
		c.Set("Content-Type", "application/json")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")

		// Use the streaming method
		return h.claudeService.CreateStreamingCompletion(ctx, &req, c.Response().BodyWriter())
	}

	// Handle non-streaming response
	resp, err := h.claudeService.CreateCompletion(ctx, &req)
	if err != nil {
		// Handle specific error types
		if strings.Contains(err.Error(), "prompt is required") {
			return c.Status(400).JSON(fiber.Map{
				"error": "Prompt is required",
			})
		}

		if strings.Contains(err.Error(), "claude command failed") {
			return c.Status(500).JSON(fiber.Map{
				"error":   "Claude CLI execution failed",
				"details": err.Error(),
			})
		}

		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(resp)
}

// GetWorktreeTodos returns the most recent Todo structure from the session history for a specific worktree
// @Summary Get worktree todos
// @Description Returns the most recent TodoWrite structure from Claude Code session for a specific worktree
// @Tags claude
// @Produce json
// @Param worktree_path query string true "Worktree path"
// @Success 200 {array} models.Todo
// @Router /v1/claude/todos [get]
func (h *ClaudeHandler) GetWorktreeTodos(c *fiber.Ctx) error {
	worktreePath := c.Query("worktree_path")
	if worktreePath == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "worktree_path query parameter is required",
		})
	}

	todos, err := h.claudeService.GetLatestTodos(worktreePath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Return empty array if no todos found instead of null
	if todos == nil {
		todos = []models.Todo{}
	}

	return c.JSON(todos)
}
