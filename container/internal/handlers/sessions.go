package handlers

import (
	"time"
	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// SessionsHandler handles session management API endpoints
type SessionsHandler struct {
	sessionService *services.SessionService
}

// SessionsResponse represents the response containing all sessions
// @Description Map of workspace paths to session information
type SessionsResponse map[string]ActiveSessionInfo

// ActiveSessionInfo represents information about an active session in a workspace
// @Description Active session information with timing and Claude session details
type ActiveSessionInfo struct {
	ClaudeSessionUUID string     `json:"claude_session_uuid" example:"abc123-def456-ghi789" description:"Unique identifier for the Claude session"`
	StartedAt         time.Time  `json:"started_at" example:"2024-01-15T14:30:00Z" description:"When the session was initially started"`
	ResumedAt         *time.Time `json:"resumed_at,omitempty" example:"2024-01-15T16:00:00Z" description:"When the session was resumed (if applicable)"`
	EndedAt           *time.Time `json:"ended_at,omitempty" example:"2024-01-15T18:30:00Z" description:"When the session ended (if not active)"`
}

// DeleteSessionResponse represents the response when deleting a session
// @Description Response confirming session deletion
type DeleteSessionResponse struct {
	Message   string `json:"message" example:"Session deleted successfully" description:"Confirmation message"`
	Workspace string `json:"workspace" example:"/workspace/my-project" description:"Workspace path that was deleted"`
}

// NewSessionsHandler creates a new sessions handler
func NewSessionsHandler(sessionService *services.SessionService) *SessionsHandler {
	return &SessionsHandler{
		sessionService: sessionService,
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
// @Description Returns session information for a specific workspace directory
// @Tags sessions
// @Produce json
// @Param workspace path string true "Workspace directory path"
// @Success 200 {object} ActiveSessionInfo
// @Router /v1/sessions/workspace/{workspace} [get]
func (h *SessionsHandler) GetSessionByWorkspace(c *fiber.Ctx) error {
	workspace := c.Params("workspace")
	
	session, exists := h.sessionService.GetActiveSession(workspace)
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Session not found for workspace",
		})
	}
	
	return c.JSON(session)
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
		"message": "Session deleted successfully",
		"workspace": workspace,
	})
}