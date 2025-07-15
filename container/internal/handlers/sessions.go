package handlers

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// SessionsHandler handles session management API endpoints
type SessionsHandler struct {
	sessionService *services.SessionService
	claudeService  *services.ClaudeService
}

// SessionsResponse represents the response containing all sessions
// @Description Map of workspace paths to session information
type SessionsResponse map[string]ActiveSessionInfo

// ActiveSessionInfo represents information about an active session in a workspace
// @Description Active session information with timing and Claude session details
type ActiveSessionInfo struct {
	// Unique identifier for the Claude session
	ClaudeSessionUUID string `json:"claude_session_uuid" example:"abc123-def456-ghi789"`
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
func NewSessionsHandler(sessionService *services.SessionService, claudeService *services.ClaudeService) *SessionsHandler {
	return &SessionsHandler{
		sessionService: sessionService,
		claudeService:  claudeService,
	}
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
// @Description Returns session information for a specific workspace directory. Use ?full=true for complete session data including messages.
// @Tags sessions
// @Produce json
// @Param workspace path string true "Workspace directory path"
// @Param full query boolean false "Include full session data with messages and user prompts"
// @Success 200 {object} ActiveSessionInfo "Basic session info when full=false"
// @Success 200 {object} github_com_vanpelt_catnip_internal_models.FullSessionData "Full session data when full=true"
// @Router /v1/sessions/workspace/{workspace} [get]
func (h *SessionsHandler) GetSessionByWorkspace(c *fiber.Ctx) error {
	workspace := c.Params("workspace")
	fullParam := c.Query("full", "false")
	includeFull := fullParam == "true"

	if includeFull {
		// Return full session data using Claude service
		fullData, err := h.claudeService.GetFullSessionData(workspace, true)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error":   "Failed to get full session data",
				"details": err.Error(),
			})
		}

		if fullData == nil {
			return c.Status(404).JSON(fiber.Map{
				"error": "Session not found for workspace",
			})
		}

		return c.JSON(fullData)
	} else {
		// Try session service first (for active PTY sessions)
		session, exists := h.sessionService.GetActiveSession(workspace)
		if exists {
			return c.JSON(session)
		}

		// Fallback to Claude service for basic info (without full data)
		fullData, err := h.claudeService.GetFullSessionData(workspace, false)
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

		// Return just the session info part for basic requests
		return c.JSON(fullData.SessionInfo)
	}
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

// UpdateSessionStatusRequest represents the request to update session status
// @Description Request to update session status with todo information
type UpdateSessionStatusRequest struct {
	// Git branch name
	Branch string `json:"branch" example:"feature/new-feature"`
	// Todo content
	Todo string `json:"todo" example:"Implement user authentication"`
	// Status of the todo
	Status string `json:"status" example:"in_progress"`
}

// UpdateSessionStatusResponse represents the response when updating session status
// @Description Response confirming session status update
type UpdateSessionStatusResponse struct {
	// Confirmation message
	Message string `json:"message" example:"Session status updated successfully"`
	// Workspace path
	Workspace string `json:"workspace" example:"/workspace/my-project"`
	// Session ID
	SessionID string `json:"session_id" example:"abc123-def456-ghi789"`
	// Updated status
	Status string `json:"status" example:"in_progress"`
}

// UpdateSessionStatus updates the status of a session with todo information
// @Summary Update session status
// @Description Updates session status with todo and branch information
// @Tags sessions
// @Accept json
// @Produce json
// @Param workspace path string true "Workspace directory path"
// @Param request body UpdateSessionStatusRequest true "Status update request"
// @Success 200 {object} UpdateSessionStatusResponse
// @Router /v1/sessions/workspace/{workspace}/status [post]
func (h *SessionsHandler) UpdateSessionStatus(c *fiber.Ctx) error {
	workspace := c.Params("workspace")

	log.Printf("🔍 [DEBUG] Updating session status for workspace: %s", workspace)

	var req UpdateSessionStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Look up the session ID from the workspace
	session, exists := h.sessionService.GetActiveSession(workspace)
	var sessionId string

	if exists {
		sessionId = session.ClaudeSessionUUID
		log.Printf("🔍 [DEBUG] Found active session ID: %s", sessionId)
	} else {
		// Fallback to Claude service to get session info
		fullData, err := h.claudeService.GetFullSessionData(workspace, false)
		if err != nil || fullData == nil {
			return c.Status(404).JSON(fiber.Map{
				"error":     "Session not found for workspace",
				"workspace": workspace,
			})
		}
		maybeSessionId := fullData.SessionInfo.CurrentSessionId
		if maybeSessionId == nil {
			return c.Status(404).JSON(fiber.Map{
				"error":     "SessionId not found for workspace",
				"workspace": workspace,
			})
		}
		sessionId = *maybeSessionId
		log.Printf("🔍 [DEBUG] Found session ID from Claude service: %s", sessionId)
	}

	log.Printf("🔍 [DEBUG] Updating session status - SessionID: %s, Branch: %s, Todo: %s, Status: %s",
		sessionId, req.Branch, req.Todo, req.Status)

	// For now, just log the status update and return success
	// In the future, this could update Claude session metadata or PTY session info

	return c.JSON(UpdateSessionStatusResponse{
		Message:   "Session status updated successfully",
		Workspace: workspace,
		SessionID: sessionId,
		Status:    req.Status,
	})
}
