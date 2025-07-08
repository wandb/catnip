package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// SessionsHandler handles session management API endpoints
type SessionsHandler struct {
	sessionService *services.SessionService
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
// @Success 200 {object} map[string]interface{}
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
// @Success 200 {object} map[string]interface{}
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
// @Success 200 {object} map[string]interface{}
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
// @Success 200 {object} map[string]string
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