package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// SessionsHandler handles session management API endpoints
type SessionsHandler struct {
	sessionService *services.SessionService
	claudeService  *services.ClaudeService
	gitService     *services.GitService
}

// SessionsResponse represents the response containing all sessions
// @Description Map of workspace paths to session information
type SessionsResponse map[string]ActiveSessionInfo

// ActiveSessionInfo represents information about an active session in a workspace
// @Description Active session information with timing and Claude session details
type ActiveSessionInfo struct {
	// Unique identifier for the Claude session
	ClaudeSessionUUID string `json:"claude_session_uuid" example:"abc123-def456-ghi789"`
	// Title of the session
	Title string `json:"title" example:"Updating README.md"`
	// When the session was initially started
	StartedAt time.Time `json:"started_at" example:"2024-01-15T14:30:00Z"`
	// When the session was resumed (if applicable)
	ResumedAt *time.Time `json:"resumed_at,omitempty" example:"2024-01-15T16:00:00Z"`
	// When the session ended (if not active)
	EndedAt *time.Time `json:"ended_at,omitempty" example:"2024-01-15T18:30:00Z"`
}

// DeleteSessionResponse represents the response when deleting a session
// @Description Response confirming session deletion
type DeleteSessionResponse struct {
	// Confirmation message
	Message string `json:"message" example:"Session deleted successfully"`
	// Workspace path that was deleted
	Workspace string `json:"workspace" example:"/workspace/my-project"`
}

// NewSessionsHandler creates a new sessions handler
func NewSessionsHandler(sessionService *services.SessionService, claudeService *services.ClaudeService, gitService *services.GitService) *SessionsHandler {
	return &SessionsHandler{
		sessionService: sessionService,
		claudeService:  claudeService,
		gitService:     gitService,
	}
}

// generateSessionDataETag generates an ETag hash from session data
func generateSessionDataETag(sessionData interface{}) (string, error) {
	// Marshal the session data to JSON for consistent hashing
	data, err := json.Marshal(sessionData)
	if err != nil {
		return "", err
	}

	// Generate SHA-256 hash
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// GetActiveSessions returns all active sessions
// @Summary Get active sessions
// @Description Returns all active sessions (not ended)
// @Tags sessions
// @Produce json
// @Success 200 {object} SessionsResponse
// @Router /v1/sessions/active [get]
func (h *SessionsHandler) GetActiveSessions(c *fiber.Ctx) error {
	sessions := h.sessionService.GetAllActiveSessions()
	return c.JSON(sessions)
}

// GetAllSessions returns all sessions including ended ones
// @Summary Get all sessions
// @Description Returns all sessions including ended ones
// @Tags sessions
// @Produce json
// @Success 200 {object} SessionsResponse
// @Router /v1/sessions [get]
func (h *SessionsHandler) GetAllSessions(c *fiber.Ctx) error {
	sessions := h.sessionService.GetAllActiveSessionsIncludingEnded()
	return c.JSON(sessions)
}

// GetSessionByWorkspace returns session for a specific workspace
// @Summary Get session by workspace
// @Description Returns session information for a specific workspace. Accepts workspace ID (UUID) or path. Use ?full=true for complete session data including messages. Supports conditional requests via If-None-Match header for efficient polling.
// @Tags sessions
// @Produce json
// @Param workspace path string true "Workspace ID (UUID) or directory path"
// @Param full query boolean false "Include full session data with messages and user prompts"
// @Param If-None-Match header string false "ETag from previous request"
// @Success 200 {object} ActiveSessionInfo "Basic session info when full=false"
// @Success 304 "Not Modified - content unchanged"
// @Router /v1/sessions/workspace/{workspace} [get]
func (h *SessionsHandler) GetSessionByWorkspace(c *fiber.Ctx) error {
	// Get workspace from path parameter
	workspace := c.Params("workspace")
	if workspace == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "workspace parameter is required",
		})
	}

	// Try to resolve as a workspace ID first (UUID)
	// If it looks like a UUID (contains hyphen, no slashes), try to resolve it
	worktreePath := workspace
	if h.gitService != nil && !containsSlash(workspace) {
		if worktree, exists := h.gitService.GetWorktree(workspace); exists && worktree != nil {
			worktreePath = worktree.Path
			// Debug: log the resolution
			c.Set("X-Debug-Workspace-ID", workspace)
			c.Set("X-Debug-Worktree-Path", worktreePath)
		}
	}

	fullParam := c.Query("full", "false")
	includeFull := fullParam == "true"

	// Get session data
	fullData, err := h.claudeService.GetFullSessionData(worktreePath, includeFull)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to get session data",
			"details": err.Error(),
		})
	}

	if fullData == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Session not found for workspace",
		})
	}

	// Generate ETag from the session data
	etag, err := generateSessionDataETag(fullData)
	if err != nil {
		// Log error but continue without ETag support
		// Don't fail the request just because ETag generation failed
		return c.JSON(fullData)
	}

	// Check If-None-Match header for conditional request
	clientETag := c.Get("If-None-Match")
	if clientETag != "" && clientETag == etag {
		// Content hasn't changed, return 304 Not Modified
		c.Set("ETag", etag)
		c.Set("Cache-Control", "no-cache") // Must revalidate, but cacheable
		return c.SendStatus(fiber.StatusNotModified)
	}

	// Content has changed or no ETag provided, return full response with ETag
	c.Set("ETag", etag)
	c.Set("Cache-Control", "no-cache") // Must revalidate, but cacheable
	return c.JSON(fullData)
}

// DeleteSession removes a session
// @Summary Delete session
// @Description Removes a session from the active sessions mapping
// @Tags sessions
// @Produce json
// @Param workspace path string true "Workspace directory path"
// @Success 200 {object} DeleteSessionResponse
// @Router /v1/sessions/workspace/{workspace} [delete]
func (h *SessionsHandler) DeleteSession(c *fiber.Ctx) error {
	workspace := c.Params("workspace")

	if err := h.sessionService.RemoveActiveSession(workspace); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":   "Session deleted successfully",
		"workspace": workspace,
	})
}

// GetSessionById returns full session data for a specific session ID
// @Summary Get session by ID
// @Description Returns complete session data for a specific session ID within a workspace
// @Tags sessions
// @Produce json
// @Param workspace path string true "Workspace directory path"
// @Param sessionId path string true "Session ID (UUID)"
// @Success 200 {object} github_com_vanpelt_catnip_internal_models.FullSessionData
// @Router /v1/sessions/workspace/{workspace}/session/{sessionId} [get]
func (h *SessionsHandler) GetSessionById(c *fiber.Ctx) error {
	workspace := c.Params("workspace")
	sessionId := c.Params("sessionId")

	sessionData, err := h.claudeService.GetSessionByID(workspace, sessionId)
	if err != nil {
		if err.Error() == "session not found: "+sessionId {
			return c.Status(404).JSON(fiber.Map{
				"error":     "Session not found",
				"sessionId": sessionId,
			})
		}
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to get session data",
			"details": err.Error(),
		})
	}

	return c.JSON(sessionData)
}

// containsSlash checks if a string contains a forward slash
// Used to distinguish between workspace IDs (UUIDs) and paths
func containsSlash(s string) bool {
	return strings.Contains(s, "/")
}
