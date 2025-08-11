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
	eventsHandler *EventsHandler
}

// NewClaudeHandler creates a new Claude handler
func NewClaudeHandler(claudeService *services.ClaudeService) *ClaudeHandler {
	return &ClaudeHandler{
		claudeService: claudeService,
	}
}

// WithEvents adds events handler for broadcasting events
func (h *ClaudeHandler) WithEvents(eventsHandler *EventsHandler) *ClaudeHandler {
	h.eventsHandler = eventsHandler
	return h
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

// GetClaudeSettings returns Claude configuration settings from ~/.claude.json
// @Summary Get Claude settings
// @Description Returns Claude Code configuration settings including theme, authentication status, and other metadata
// @Tags claude
// @Produce json
// @Success 200 {object} models.ClaudeSettings
// @Router /v1/claude/settings [get]
func (h *ClaudeHandler) GetClaudeSettings(c *fiber.Ctx) error {
	settings, err := h.claudeService.GetClaudeSettings()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(settings)
}

// UpdateClaudeSettings updates Claude configuration settings in ~/.claude.json
// @Summary Update Claude settings
// @Description Updates Claude Code configuration settings (currently only theme is supported)
// @Tags claude
// @Accept json
// @Produce json
// @Param request body models.ClaudeSettingsUpdateRequest true "Settings update request"
// @Success 200 {object} models.ClaudeSettings
// @Router /v1/claude/settings [put]
func (h *ClaudeHandler) UpdateClaudeSettings(c *fiber.Ctx) error {
	var req models.ClaudeSettingsUpdateRequest

	// Parse the request body
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate theme
	validThemes := []string{"dark", "light", "dark-daltonized", "light-daltonized", "dark-ansi", "light-ansi"}
	valid := false
	for _, theme := range validThemes {
		if req.Theme == theme {
			valid = true
			break
		}
	}
	if !valid {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid theme value. Must be one of: dark, light, dark-daltonized, light-daltonized, dark-ansi, light-ansi",
		})
	}

	// Update settings
	settings, err := h.claudeService.UpdateClaudeSettings(&req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(settings)
}

// HandleClaudeHook handles Claude Code hook notifications
// @Summary Handle Claude hook events
// @Description Receives hook notifications from Claude Code for activity tracking
// @Tags claude
// @Accept json
// @Produce json
// @Param request body models.ClaudeHookEvent true "Claude hook event"
// @Success 200 {object} map[string]string
// @Router /v1/claude/hooks [post]
func (h *ClaudeHandler) HandleClaudeHook(c *fiber.Ctx) error {
	var req models.ClaudeHookEvent

	// Parse the request body
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.EventType == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "event_type is required",
		})
	}

	if req.WorkingDirectory == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "working_directory is required",
		})
	}

	// Handle the hook event
	err := h.claudeService.HandleHookEvent(&req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to handle hook event",
			"details": err.Error(),
		})
	}

	// Handle special events that should broadcast to frontend
	if h.eventsHandler != nil && req.EventType == "Stop" {
		// Get session information for this worktree
		summary, _ := h.claudeService.GetWorktreeSessionSummary(req.WorkingDirectory)
		todos, _ := h.claudeService.GetLatestTodos(req.WorkingDirectory)

		var sessionTitle *string
		var lastTodo *string
		var worktreeID *string

		if summary != nil && summary.Header != nil {
			sessionTitle = summary.Header
		}

		if len(todos) > 0 {
			// Find the last incomplete todo
			for i := len(todos) - 1; i >= 0; i-- {
				if todos[i].Status != "completed" {
					lastTodo = &todos[i].Content
					break
				}
			}
			// If all todos are completed, use the last one
			if lastTodo == nil {
				lastTodo = &todos[len(todos)-1].Content
			}
		}

		// For now, we don't have direct access to git info here,
		// but we could extend this to get branch name
		var branchName *string

		// Emit the session stopped event
		h.eventsHandler.EmitSessionStopped(
			req.WorkingDirectory,
			worktreeID,
			sessionTitle,
			branchName,
			lastTodo,
		)
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Hook event processed successfully",
	})
}
