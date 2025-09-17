package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// AgentHandler handles multi-agent API endpoints
type AgentHandler struct {
	agentManager  *services.AgentManager
	gitService    *services.GitService
	eventsHandler *EventsHandler
}

// NewAgentHandler creates a new agent handler
func NewAgentHandler(agentManager *services.AgentManager, gitService *services.GitService) *AgentHandler {
	return &AgentHandler{
		agentManager: agentManager,
		gitService:   gitService,
	}
}

// WithEvents adds events handler for broadcasting events
func (h *AgentHandler) WithEvents(eventsHandler *EventsHandler) *AgentHandler {
	h.eventsHandler = eventsHandler
	return h
}

// GetAvailableAgents returns a list of available agent types
// @Summary Get available agents
// @Description Returns a list of all registered coding agents
// @Tags agents
// @Produce json
// @Success 200 {array} string
// @Router /v1/agents [get]
func (h *AgentHandler) GetAvailableAgents(c *fiber.Ctx) error {
	agents := h.agentManager.GetAvailableAgents()
	return c.JSON(agents)
}

// GetWorktreeSessionSummary returns agent session information for a specific worktree
// @Summary Get worktree session summary
// @Description Returns coding agent session metadata for a specific worktree
// @Tags agents
// @Produce json
// @Param worktree_path query string true "Worktree path"
// @Param agent_type query string false "Agent type (defaults to Claude for backward compatibility)"
// @Success 200 {object} models.AgentSessionSummary
// @Router /v1/agents/session [get]
func (h *AgentHandler) GetWorktreeSessionSummary(c *fiber.Ctx) error {
	worktreePath := c.Query("worktree_path")
	if worktreePath == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "worktree_path query parameter is required",
		})
	}

	summary, err := h.agentManager.GetWorktreeSessionSummary(worktreePath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if summary == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No agent session found for this worktree",
		})
	}

	return c.JSON(summary)
}

// GetAllWorktreeSessionSummaries returns agent session information for all worktrees
// @Summary Get all worktree session summaries
// @Description Returns coding agent session metadata for all worktrees with agent data
// @Tags agents
// @Produce json
// @Success 200 {object} map[string]models.AgentSessionSummary
// @Router /v1/agents/sessions [get]
func (h *AgentHandler) GetAllWorktreeSessionSummaries(c *fiber.Ctx) error {
	summaries, err := h.agentManager.GetAllWorktreeSessionSummaries()
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
// @Tags agents
// @Produce json
// @Param uuid path string true "Session UUID"
// @Success 200 {object} models.AgentFullSessionData
// @Router /v1/agents/session/{uuid} [get]
func (h *AgentHandler) GetSessionByUUID(c *fiber.Ctx) error {
	sessionUUID := c.Params("uuid")
	if sessionUUID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "session UUID is required",
		})
	}

	sessionData, err := h.agentManager.GetSessionByUUID(sessionUUID)
	if err != nil {
		if strings.Contains(err.Error(), "session not found") {
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

// CreateCompletion handles requests to create completions using any agent
// @Summary Create agent messages
// @Description Creates a completion using any registered coding agent, supporting both streaming and non-streaming responses, with resume functionality
// @Tags agents
// @Accept json
// @Produce json
// @Param request body models.AgentCompletionRequest true "Create completion request"
// @Success 200 {object} models.AgentCompletionResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/agents/messages [post]
func (h *AgentHandler) CreateCompletion(c *fiber.Ctx) error {
	var req models.AgentCompletionRequest

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
		return h.agentManager.CreateStreamingCompletion(ctx, &req, c.Response().BodyWriter())
	}

	// Handle non-streaming response
	logger.Infof("🔍 Creating agent completion for prompt: %.100s...", req.Prompt)
	resp, err := h.agentManager.CreateCompletion(ctx, &req)
	if err != nil {
		logger.Errorf("❌ Agent completion failed: %v", err)
		// Handle specific error types
		if strings.Contains(err.Error(), "prompt is required") {
			return c.Status(400).JSON(fiber.Map{
				"error": "Prompt is required",
			})
		}

		if strings.Contains(err.Error(), "command failed") {
			return c.Status(500).JSON(fiber.Map{
				"error":   "Agent CLI execution failed",
				"details": err.Error(),
			})
		}

		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	logger.Infof("✅ Agent completion successful. Response length: %d chars", len(resp.Response))
	logger.Debugf("📝 Agent response content: %s", resp.Response)
	return c.JSON(resp)
}

// GetWorktreeTodos returns the most recent Todo structure from the session history for a specific worktree
// @Summary Get worktree todos
// @Description Returns the most recent TodoWrite structure from any coding agent session for a specific worktree
// @Tags agents
// @Produce json
// @Param worktree_path query string true "Worktree path"
// @Success 200 {array} models.Todo
// @Router /v1/agents/todos [get]
func (h *AgentHandler) GetWorktreeTodos(c *fiber.Ctx) error {
	worktreePath := c.Query("worktree_path")
	if worktreePath == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "worktree_path query parameter is required",
		})
	}

	todos, err := h.agentManager.GetLatestTodos(worktreePath)
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
// @Description Returns the most recent assistant message from any coding agent session for a specific worktree
// @Tags agents
// @Produce json
// @Param worktree_path query string true "Worktree path"
// @Success 200 {object} map[string]interface{}
// @Router /v1/agents/latest-message [get]
func (h *AgentHandler) GetWorktreeLatestAssistantMessage(c *fiber.Ctx) error {
	worktreePath := c.Query("worktree_path")
	if worktreePath == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "worktree_path query parameter is required",
		})
	}

	message, isError, err := h.agentManager.GetLatestAssistantMessageOrError(worktreePath)
	if err != nil {
		if strings.Contains(err.Error(), "project directory not found") {
			return c.Status(404).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"content": message,
		"isError": isError,
	})
}

// GetAgentSettings returns agent configuration settings
// @Summary Get agent settings
// @Description Returns coding agent configuration settings including authentication status and other metadata
// @Tags agents
// @Produce json
// @Param agent_type query string false "Agent type (defaults to Claude)"
// @Success 200 {object} models.AgentSettings
// @Router /v1/agents/settings [get]
func (h *AgentHandler) GetAgentSettings(c *fiber.Ctx) error {
	agentType := models.AgentType(c.Query("agent_type"))
	if agentType == "" {
		agentType = models.AgentTypeClaude // Default to Claude for backward compatibility
	}

	settings, err := h.agentManager.GetSettings(agentType)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(settings)
}

// UpdateAgentSettings updates agent configuration settings
// @Summary Update agent settings
// @Description Updates coding agent configuration settings
// @Tags agents
// @Accept json
// @Produce json
// @Param agent_type query string false "Agent type (defaults to Claude)"
// @Param request body models.AgentSettingsUpdateRequest true "Settings update request"
// @Success 200 {object} models.AgentSettings
// @Router /v1/agents/settings [put]
func (h *AgentHandler) UpdateAgentSettings(c *fiber.Ctx) error {
	agentType := models.AgentType(c.Query("agent_type"))
	if agentType == "" {
		agentType = models.AgentTypeClaude // Default to Claude for backward compatibility
	}

	var req models.AgentSettingsUpdateRequest

	// Parse the request body
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate that at least one field is provided
	if req.NotificationsEnabled == nil && len(req.AgentSpecificSettings) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "At least one setting must be provided",
		})
	}

	// Update settings
	settings, err := h.agentManager.UpdateSettings(agentType, &req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(settings)
}

// HandleAgentEvent handles agent event notifications
// @Summary Handle agent events
// @Description Receives event notifications from coding agents for activity tracking
// @Tags agents
// @Accept json
// @Produce json
// @Param request body models.AgentEvent true "Agent event"
// @Success 200 {object} map[string]string
// @Router /v1/agents/events [post]
func (h *AgentHandler) HandleAgentEvent(c *fiber.Ctx) error {
	var req models.AgentEvent

	// Log the raw request body for debugging
	bodyBytes := c.Body()
	logger.Debugf("🔔 Agent event received - Raw body: %s", string(bodyBytes))

	// Parse the request body
	if err := c.BodyParser(&req); err != nil {
		logger.Debugf("❌ Event parsing error: %v", err)
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Log the parsed event
	logger.Debugf("🔔 Parsed agent event - Type: %s, WorkDir: %s, Agent: %s", req.EventType, req.WorkingDirectory, req.AgentType)

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

	if req.AgentType == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "agent_type is required",
		})
	}

	// Set timestamp if not provided
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}

	// Handle the event
	err := h.agentManager.HandleEvent(&req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to handle agent event",
			"details": err.Error(),
		})
	}

	// Trigger immediate activity state sync for activity-related events
	if req.EventType == "UserPromptSubmit" || req.EventType == "PostToolUse" || req.EventType == "Stop" {
		logger.Debugf("🔄 Triggering immediate activity state sync for %s", req.EventType)
		if stateManager := h.gitService.GetStateManager(); stateManager != nil {
			stateManager.TriggerClaudeActivitySync()
		}
	}

	// Trigger immediate commit sync for Stop events to auto-commit dirty changes
	if req.EventType == "Stop" {
		logger.Debugf("🔄 Triggering immediate commit sync for Stop event in %s", req.WorkingDirectory)
		if commitSyncService := h.gitService.GetCommitSyncService(); commitSyncService != nil {
			commitSyncService.PerformManualSync()
		}
	}

	// Emit agent message on PostToolUse events
	if h.eventsHandler != nil && req.EventType == "PostToolUse" {
		h.handlePostToolUseEvent(&req)
	}

	// Handle special events that should broadcast to frontend
	if h.eventsHandler != nil && req.EventType == "Stop" {
		h.handleStopEvent(&req)
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Agent event processed successfully",
	})
}

// handlePostToolUseEvent handles PostToolUse events
func (h *AgentHandler) handlePostToolUseEvent(event *models.AgentEvent) {
	// Find the workspace directory - handle subdirectories by checking workspace prefix
	workspaceDir := event.WorkingDirectory
	worktrees := h.gitService.ListWorktrees()

	// Check if working directory is a subdirectory of any workspace
	var matchingWorktree *models.Worktree
	for _, wt := range worktrees {
		if strings.HasPrefix(event.WorkingDirectory, wt.Path) {
			// Use the longest matching path (most specific workspace)
			if matchingWorktree == nil || len(wt.Path) > len(matchingWorktree.Path) {
				matchingWorktree = wt
				workspaceDir = wt.Path
			}
		}
	}

	// Get the latest assistant message if we found a matching worktree
	if matchingWorktree != nil {
		if latestMessage, err := h.agentManager.GetLatestAssistantMessage(workspaceDir); err == nil && latestMessage != "" {
			logger.Debugf("📨 Emitting agent message for worktree %s", matchingWorktree.ID)
			h.eventsHandler.EmitClaudeMessage(workspaceDir, matchingWorktree.ID, latestMessage, "assistant")
		} else if err != nil {
			logger.Debugf("📨 Failed to get latest assistant message: %v", err)
		}
	}
}

// handleStopEvent handles Stop events
func (h *AgentHandler) handleStopEvent(event *models.AgentEvent) {
	// Find the workspace directory - handle subdirectories by checking workspace prefix
	workspaceDir := event.WorkingDirectory
	worktrees := h.gitService.ListWorktrees()

	// Check if working directory is a subdirectory of any workspace
	var matchingWorktree *models.Worktree
	for _, wt := range worktrees {
		if strings.HasPrefix(event.WorkingDirectory, wt.Path) {
			// Use the longest matching path (most specific workspace)
			if matchingWorktree == nil || len(wt.Path) > len(matchingWorktree.Path) {
				matchingWorktree = wt
				workspaceDir = wt.Path
			}
		}
	}

	// Check if events are suppressed for this workspace (automated operation)
	// For backward compatibility, check Claude service if it's a Claude event
	var eventsSuppressed bool
	if event.AgentType == models.AgentTypeClaude {
		// Get Claude agent to check suppression
		if claudeAgent, err := h.agentManager.GetAgent(models.AgentTypeClaude); err == nil {
			if claudeAgentImpl, ok := claudeAgent.(*services.ClaudeAgent); ok {
				eventsSuppressed = claudeAgentImpl.IsSuppressingEvents(workspaceDir)
			}
		}
	}

	if eventsSuppressed {
		logger.Debugf("🔔 Skipping stop event - automated operation in progress for %s", workspaceDir)
		return
	}

	// Send stop event for any matching workspace
	if matchingWorktree != nil {
		logger.Debugf("🔔 Emitting session stopped event for %s (branch: %s)", workspaceDir, matchingWorktree.Branch)

		// Get session information for this worktree
		todos, _ := h.agentManager.GetLatestTodos(workspaceDir)

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
		if settings, err := h.agentManager.GetSettings(event.AgentType); err == nil && settings.NotificationsEnabled {
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
			logger.Debugf("🔔 Failed to get agent settings for notification check: %v", err)
		} else {
			logger.Debugf("🔔 Notifications disabled, skipping notification event")
		}
	} else {
		logger.Debugf("🔔 Skipping stop event - no matching workspace found for %s", event.WorkingDirectory)
	}
}
