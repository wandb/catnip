package handlers

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// SessionsHandler handles session management API endpoints
type SessionsHandler struct {
	sessionService *services.SessionService
	claudeService  *services.ClaudeService
	statusStore    *StatusStore
}

// StatusStore manages status history for workspaces
type StatusStore struct {
	mu       sync.RWMutex
	statuses map[string][]StatusEntry
}

// StatusEntry represents a single status update
type StatusEntry struct {
	// Timestamp when the status was received
	Timestamp time.Time `json:"timestamp"`
	// Git branch at the time of status update
	Branch string `json:"branch"`
	// Workspace name
	Workspace string `json:"workspace"`
	// Tool input data from Claude
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	// Raw input data as received
	RawInput string `json:"raw_input,omitempty"`
}

// StatusUpdateRequest represents the request to update status
type StatusUpdateRequest struct {
	Branch    string          `json:"branch"`
	Workspace string          `json:"workspace"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	Timestamp string          `json:"timestamp"`
}

// StatusHistoryResponse represents the response with status history
type StatusHistoryResponse struct {
	Workspace string        `json:"workspace"`
	Count     int           `json:"count"`
	Statuses  []StatusEntry `json:"statuses"`
}

// NewStatusStore creates a new status store
func NewStatusStore() *StatusStore {
	return &StatusStore{
		statuses: make(map[string][]StatusEntry),
	}
}

// AddStatus adds a new status entry for a workspace
func (s *StatusStore) AddStatus(workspace string, entry StatusEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.statuses[workspace] == nil {
		s.statuses[workspace] = make([]StatusEntry, 0)
	}

	// Add new entry at the beginning (most recent first)
	s.statuses[workspace] = append([]StatusEntry{entry}, s.statuses[workspace]...)

	// Keep only last 1000 entries per workspace to prevent memory issues
	if len(s.statuses[workspace]) > 1000 {
		s.statuses[workspace] = s.statuses[workspace][:1000]
	}
}

// GetStatus retrieves status history for a workspace
func (s *StatusStore) GetStatus(workspace string, count int) []StatusEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.statuses[workspace]
	if entries == nil {
		return []StatusEntry{}
	}

	// Return requested count or all if count is 0
	if count == 0 || count > len(entries) {
		return entries
	}

	return entries[:count]
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
func NewSessionsHandler(sessionService *services.SessionService, claudeService *services.ClaudeService) *SessionsHandler {
	return &SessionsHandler{
		sessionService: sessionService,
		claudeService:  claudeService,
		statusStore:    NewStatusStore(),
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

// UpdateSessionStatus receives and stores status updates from Claude hooks
// @Summary Update session status
// @Description Receives status updates from Claude hooks and stores them for the workspace
// @Tags sessions
// @Accept json
// @Produce json
// @Param workspace path string true "Workspace directory path"
// @Param request body StatusUpdateRequest true "Status update data"
// @Success 200 {object} map[string]string
// @Router /v1/sessions/workspace/{workspace}/status [post]
func (h *SessionsHandler) UpdateSessionStatus(c *fiber.Ctx) error {
	workspace := c.Params("workspace")
	log.Printf("Received status update for workspace: %s", workspace)

	var req StatusUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		log.Printf("Error parsing status update request for workspace %s: %v", workspace, err)
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Parse timestamp or use current time
	var timestamp time.Time
	if req.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
			timestamp = parsed
		} else {
			log.Printf("Invalid timestamp format for workspace %s, using current time: %s", workspace, req.Timestamp)
			timestamp = time.Now()
		}
	} else {
		timestamp = time.Now()
	}

	// Create status entry
	entry := StatusEntry{
		Timestamp: timestamp,
		Branch:    req.Branch,
		Workspace: workspace,
		ToolInput: req.ToolInput,
		RawInput:  string(c.Body()),
	}

	// Store the status entry
	h.statusStore.AddStatus(workspace, entry)
	log.Printf("Stored status update for workspace %s (branch: %s) at %s", workspace, req.Branch, timestamp.Format(time.RFC3339))

	return c.JSON(fiber.Map{
		"message":   "Status updated successfully",
		"workspace": workspace,
		"timestamp": timestamp.Format(time.RFC3339),
	})
}

// GetSessionStatus retrieves status history for a workspace
// @Summary Get session status history
// @Description Returns status history for a workspace with optional count parameter
// @Tags sessions
// @Produce json
// @Param workspace path string true "Workspace directory path"
// @Param count query int false "Number of recent statuses to return (default: all)"
// @Success 200 {object} StatusHistoryResponse
// @Router /v1/sessions/workspace/{workspace}/status [get]
func (h *SessionsHandler) GetSessionStatus(c *fiber.Ctx) error {
	workspace := c.Params("workspace")
	countStr := c.Query("count", "0")

	count, err := strconv.Atoi(countStr)
	if err != nil {
		log.Printf("Invalid count parameter for workspace %s: %s, using default (all)", workspace, countStr)
		count = 0 // Default to all if invalid
	}

	// Get status history
	statuses := h.statusStore.GetStatus(workspace, count)
	log.Printf("Retrieved %d status entries for workspace %s (requested: %d)", len(statuses), workspace, count)

	return c.JSON(StatusHistoryResponse{
		Workspace: workspace,
		Count:     len(statuses),
		Statuses:  statuses,
	})
}
