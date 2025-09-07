package handlers

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// ClaudeHandler handles Claude Code session-related API endpoints
type ClaudeHandler struct {
	claudeService *services.ClaudeService
	gitService    *services.GitService
	eventsHandler *EventsHandler
}

// NewClaudeHandler creates a new Claude handler
func NewClaudeHandler(claudeService *services.ClaudeService, gitService *services.GitService) *ClaudeHandler {
	return &ClaudeHandler{
		claudeService: claudeService,
		gitService:    gitService,
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
	logger.Infof("🔍 Creating Claude completion for prompt: %.100s...", req.Prompt)
	resp, err := h.claudeService.CreateCompletion(ctx, &req)
	if err != nil {
		logger.Errorf("❌ Claude completion failed: %v", err)
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

	logger.Infof("✅ Claude completion successful. Response length: %d chars", len(resp.Response))
	logger.Debugf("📝 Claude response content: %s", resp.Response)
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

// GetWorktreeLatestAssistantMessage returns the most recent assistant message from the session history for a specific worktree
// @Summary Get worktree latest assistant message
// @Description Returns the most recent assistant message from Claude Code session for a specific worktree
// @Tags claude
// @Produce json
// @Param worktree_path query string true "Worktree path"
// @Success 200 {object} map[string]string
// @Router /v1/claude/latest-message [get]
func (h *ClaudeHandler) GetWorktreeLatestAssistantMessage(c *fiber.Ctx) error {
	worktreePath := c.Query("worktree_path")
	if worktreePath == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "worktree_path query parameter is required",
		})
	}

	message, err := h.claudeService.GetLatestAssistantMessage(worktreePath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": message,
	})
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

// UpdateClaudeSettings updates Claude configuration settings in ~/.claude.json and volume settings.json
// @Summary Update Claude settings
// @Description Updates Claude Code configuration settings (theme and notifications)
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

	// Validate theme if provided
	if req.Theme != "" {
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
	}

	// Validate that at least one field is provided
	if req.Theme == "" && req.NotificationsEnabled == nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "At least one setting must be provided (theme or notificationsEnabled)",
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

	// Log the raw request body for debugging
	bodyBytes := c.Body()
	logger.Debugf("🔔 Claude hook received - Raw body: %s", string(bodyBytes))

	// Parse the request body
	if err := c.BodyParser(&req); err != nil {
		logger.Debugf("❌ Hook parsing error: %v", err)
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Log the parsed hook event
	logger.Debugf("🔔 Parsed hook event - Type: %s, WorkDir: %s", req.EventType, req.WorkingDirectory)

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

	// Trigger immediate Claude activity state sync for activity-related events
	if req.EventType == "UserPromptSubmit" || req.EventType == "PostToolUse" || req.EventType == "Stop" {
		logger.Debugf("🔄 Triggering immediate Claude activity state sync for %s", req.EventType)
		if stateManager := h.gitService.GetStateManager(); stateManager != nil {
			stateManager.TriggerClaudeActivitySync()
		}
	}

	// Emit Claude message on PostToolUse events
	if h.eventsHandler != nil && req.EventType == "PostToolUse" {
		// Find the workspace directory - handle subdirectories by checking workspace prefix
		workspaceDir := req.WorkingDirectory
		worktrees := h.gitService.ListWorktrees()

		// Check if working directory is a subdirectory of any workspace
		var matchingWorktree *models.Worktree
		for _, wt := range worktrees {
			if strings.HasPrefix(req.WorkingDirectory, wt.Path) {
				// Use the longest matching path (most specific workspace)
				if matchingWorktree == nil || len(wt.Path) > len(matchingWorktree.Path) {
					matchingWorktree = wt
					workspaceDir = wt.Path
				}
			}
		}

		// Get the latest assistant message if we found a matching worktree
		if matchingWorktree != nil {
			if latestMessage, err := h.claudeService.GetLatestAssistantMessage(workspaceDir); err == nil && latestMessage != "" {
				logger.Debugf("📨 Emitting Claude message for worktree %s", matchingWorktree.ID)
				h.eventsHandler.EmitClaudeMessage(workspaceDir, matchingWorktree.ID, latestMessage, "assistant")
			} else if err != nil {
				logger.Debugf("📨 Failed to get latest assistant message: %v", err)
			}
		}
	}

	// Handle special events that should broadcast to frontend
	logger.Debugf("🔔 Hook processing - EventType: %s, EventsHandler nil: %t", req.EventType, h.eventsHandler == nil)
	if h.eventsHandler != nil && req.EventType == "Stop" {
		// Find the workspace directory - handle subdirectories by checking workspace prefix
		workspaceDir := req.WorkingDirectory
		worktrees := h.gitService.ListWorktrees()

		// Check if working directory is a subdirectory of any workspace
		var matchingWorktree *models.Worktree
		for _, wt := range worktrees {
			if strings.HasPrefix(req.WorkingDirectory, wt.Path) {
				// Use the longest matching path (most specific workspace)
				if matchingWorktree == nil || len(wt.Path) > len(matchingWorktree.Path) {
					matchingWorktree = wt
					workspaceDir = wt.Path
				}
			}
		}

		// Check if events are suppressed for this workspace (automated operation)
		if h.claudeService.IsSuppressingEvents(workspaceDir) {
			logger.Debugf("🔔 Skipping stop event - automated operation in progress for %s", workspaceDir)
			return c.JSON(fiber.Map{
				"status":  "success",
				"message": "Hook event processed successfully (automated - events suppressed)",
			})
		}

		// Send stop event for any matching workspace
		if matchingWorktree != nil {
			logger.Debugf("🔔 Emitting session stopped event for %s (branch: %s)", workspaceDir, matchingWorktree.Branch)

			// Get session information for this worktree
			todos, _ := h.claudeService.GetLatestTodos(workspaceDir)

			// Create title: branch name truncated to 15 chars + " stopped"
			branchName := matchingWorktree.Branch
			if len(branchName) > 15 {
				branchName = branchName[:15]
			}
			title := branchName + " stopped"

			// Create description: last todo truncated to 50 chars or generic message
			var description string
			if len(todos) > 0 {
				// Find the last incomplete todo
				var lastTodo string
				for i := len(todos) - 1; i >= 0; i-- {
					if todos[i].Status != "completed" {
						lastTodo = todos[i].Content
						break
					}
				}
				// If all todos are completed, use the last one
				if lastTodo == "" && len(todos) > 0 {
					lastTodo = todos[len(todos)-1].Content
				}

				if lastTodo != "" {
					if len(lastTodo) > 50 {
						lastTodo = lastTodo[:50] + "..."
					}
					description = lastTodo
				} else {
					description = "Session has completed"
				}
			} else {
				description = "Session has completed"
			}

			// Emit the session stopped event with improved content
			h.eventsHandler.EmitSessionStopped(
				workspaceDir,
				nil, // worktreeID not needed
				&title,
				&matchingWorktree.Branch, // Keep full branch name for context
				&description,
			)

			// Also emit a notification event directly via SSE if notifications are enabled
			if settings, err := h.claudeService.GetClaudeSettings(); err == nil && settings.NotificationsEnabled {
				logger.Debugf("🔔 Emitting notification event: %s", title)

				// Generate workspace URL - remove workspace prefix if present
				workspacePath := strings.TrimPrefix(workspaceDir, config.Runtime.WorkspaceDir)
				workspaceURL := fmt.Sprintf("http://localhost:6369/workspace%s", workspacePath)

				h.eventsHandler.broadcastEvent(AppEvent{
					Type: NotificationEvent,
					Payload: NotificationPayload{
						Title:    title,
						Body:     description,
						Subtitle: "", // Leave empty for consistency with existing notification structure
						URL:      workspaceURL,
					},
				})
			} else if err != nil {
				logger.Debugf("🔔 Failed to get Claude settings for notification check: %v", err)
			} else {
				logger.Debugf("🔔 Notifications disabled, skipping notification event")
			}
		} else {
			logger.Debugf("🔔 Skipping stop event - no matching workspace found for %s", req.WorkingDirectory)
		}
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Hook event processed successfully",
	})
}
