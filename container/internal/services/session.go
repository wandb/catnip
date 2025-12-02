package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/claude/paths"
	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
)

// SessionState represents persistent session state
type SessionState struct {
	ID               string            `json:"id"`
	WorkingDirectory string            `json:"working_directory"`
	Agent            string            `json:"agent"`
	ClaudeSessionID  string            `json:"claude_session_id,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	LastAccess       time.Time         `json:"last_access"`
	Environment      map[string]string `json:"environment,omitempty"`
}

// SessionService manages session state persistence and active sessions tracking
type SessionService struct {
	stateDir       string
	activeSessions map[string]*ActiveSessionInfo // key: workspace directory path
	mu             sync.RWMutex
	eventsHandler  EventsEmitter         // Interface for emitting events
	claudeMonitor  *ClaudeMonitorService // Reference to Claude monitor for activity tracking
}

// ActiveSessionInfo represents information about an active session in a workspace
type ActiveSessionInfo struct {
	ClaudeSessionUUID string              `json:"claude_session_uuid"`
	Title             *models.TitleEntry  `json:"title,omitempty"`
	TitleHistory      []models.TitleEntry `json:"title_history"`
	StartedAt         time.Time           `json:"started_at"`
	ResumedAt         *time.Time          `json:"resumed_at,omitempty"`
	EndedAt           *time.Time          `json:"ended_at,omitempty"`
}

// NewSessionService creates a new session service
func NewSessionService() *SessionService {
	stateDir := filepath.Join(config.Runtime.WorkspaceDir, ".session-state")

	// Ensure state directory exists
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		fmt.Printf("Warning: Failed to create session state directory: %v\n", err)
	}

	service := &SessionService{
		stateDir:       stateDir,
		activeSessions: make(map[string]*ActiveSessionInfo),
	}

	// Load existing active sessions state
	_ = service.loadActiveSessionsState()

	return service
}

// SetEventsHandler sets the events handler for emitting events
func (s *SessionService) SetEventsHandler(handler EventsEmitter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventsHandler = handler
}

// SetClaudeMonitor sets the Claude monitor service for real-time activity tracking
func (s *SessionService) SetClaudeMonitor(monitor *ClaudeMonitorService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claudeMonitor = monitor
}

// SaveSessionState persists session state to disk
func (s *SessionService) SaveSessionState(state *SessionState) error {
	if state.ID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	// Sanitize session ID to replace path separators with underscores
	sanitizedID := strings.ReplaceAll(state.ID, "/", "_")
	sanitizedID = strings.ReplaceAll(sanitizedID, ":", "_")

	filePath := filepath.Join(s.stateDir, fmt.Sprintf("%s.json", sanitizedID))

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for session state: %v", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session state: %v", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write session state file: %v", err)
	}

	return nil
}

// LoadSessionState loads session state from disk
func (s *SessionService) LoadSessionState(sessionID string) (*SessionState, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// Sanitize session ID to match SaveSessionState behavior
	sanitizedID := strings.ReplaceAll(sessionID, "/", "_")
	sanitizedID = strings.ReplaceAll(sanitizedID, ":", "_")

	filePath := filepath.Join(s.stateDir, fmt.Sprintf("%s.json", sanitizedID))

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil // Session state doesn't exist
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session state file: %v", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session state: %v", err)
	}

	return &state, nil
}

// FindSessionByDirectory finds an active Claude session in the given directory
func (s *SessionService) FindSessionByDirectory(workDir string) (*SessionState, error) {
	// First, try to find the newest session file directly from .claude/projects
	// Claude stores projects in ~/.claude/projects/{transformed-path}/
	// where the path is transformed: /workspace/catnip/buddy -> -workspace-catnip-buddy
	homeDir := config.Runtime.HomeDir
	transformedPath := strings.ReplaceAll(workDir, "/", "-")
	transformedPath = strings.TrimPrefix(transformedPath, "-")
	transformedPath = "-" + transformedPath // Add back the leading dash

	claudeProjectsDir := filepath.Join(homeDir, ".claude", "projects", transformedPath)
	if newestSessionID := s.findNewestClaudeSessionFile(claudeProjectsDir); newestSessionID != "" {
		// Create a minimal state for the newest session found
		return &SessionState{
			ID:               "detected",
			WorkingDirectory: workDir,
			Agent:            "claude",
			ClaudeSessionID:  newestSessionID,
			CreatedAt:        time.Now(),
			LastAccess:       time.Now(),
		}, nil
	}

	// Fallback: check our persisted session states
	files, err := os.ReadDir(s.stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read session state directory: %v", err)
	}

	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			sessionID := file.Name()[:len(file.Name())-5] // Remove .json extension

			state, err := s.LoadSessionState(sessionID)
			if err != nil {
				continue // Skip invalid session files
			}

			if state != nil &&
				state.WorkingDirectory == workDir &&
				state.Agent == "claude" &&
				state.ClaudeSessionID != "" {

				// Check if session was accessed recently (within 24 hours)
				if time.Since(state.LastAccess) < 24*time.Hour {
					return state, nil
				}
			}
		}
	}

	return nil, nil // No active Claude session found in this directory
}

// GetClaudeActivityState determines the Claude activity state for a workspace directory
func (s *SessionService) GetClaudeActivityState(workDir string) models.ClaudeActivityState {
	// Use the new ClaudeMonitorService method if available
	s.mu.RLock()
	monitor := s.claudeMonitor
	s.mu.RUnlock()

	if monitor != nil {
		return monitor.GetClaudeActivityState(workDir)
	}

	// Fallback to old method if ClaudeMonitorService is not available
	// Check if PTY session exists for this workspace
	hasPTYSession := s.hasPTYSession(workDir)

	// Get the newest Claude session file and its modification time
	lastActivityTime := s.getLastClaudeActivityTime(workDir)

	// If no Claude session file found, check PTY existence
	if lastActivityTime.IsZero() {
		if hasPTYSession {
			return models.ClaudeRunning // PTY exists but no Claude activity detected
		}
		return models.ClaudeInactive // No PTY session and no Claude activity
	}

	// Check if Claude activity is recent
	timeSinceActivity := time.Since(lastActivityTime)

	// If there's a PTY session (user likely viewing/interacting), be more generous with "active" status
	if hasPTYSession {
		// Within 5 minutes with PTY = active (user is likely still engaged)
		if timeSinceActivity <= 5*time.Minute {
			return models.ClaudeActive // Recent Claude activity with active PTY session
		}
		// PTY exists but older activity = running
		return models.ClaudeRunning // PTY exists but no recent Claude activity
	}

	// No PTY session, use shorter threshold
	if timeSinceActivity <= 2*time.Minute {
		return models.ClaudeActive // Recent Claude activity (but no active session)
	}

	return models.ClaudeInactive // Old activity and no PTY session
}

// getLastClaudeActivityTime returns the last Claude activity time using real-time monitoring when available
func (s *SessionService) getLastClaudeActivityTime(workDir string) time.Time {
	// Try to get real-time activity data from Claude monitor first
	s.mu.RLock()
	monitor := s.claudeMonitor
	s.mu.RUnlock()

	if monitor != nil {
		if activityTime := monitor.GetLastActivityTime(workDir); !activityTime.IsZero() {
			return activityTime
		}
	}

	// Fallback to file modification time method
	homeDir := config.Runtime.HomeDir
	transformedPath := strings.ReplaceAll(workDir, "/", "-")
	transformedPath = strings.TrimPrefix(transformedPath, "-")
	transformedPath = "-" + transformedPath // Add back the leading dash

	claudeProjectsDir := filepath.Join(homeDir, ".claude", "projects", transformedPath)

	// Check if .claude/projects directory exists
	if _, err := os.Stat(claudeProjectsDir); os.IsNotExist(err) {
		return time.Time{} // Zero time if directory doesn't exist
	}

	files, err := os.ReadDir(claudeProjectsDir)
	if err != nil {
		return time.Time{}
	}

	var newestTime time.Time

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		// Extract session ID from filename (remove .jsonl extension)
		sessionID := strings.TrimSuffix(file.Name(), ".jsonl")

		// Validate that it looks like a UUID
		if len(sessionID) != 36 || strings.Count(sessionID, "-") != 4 {
			continue
		}

		// Get file modification time
		filePath := filepath.Join(claudeProjectsDir, file.Name())
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Track the newest modification time
		if fileInfo.ModTime().After(newestTime) {
			newestTime = fileInfo.ModTime()
		}
	}

	return newestTime
}

// hasPTYSession checks if there's an active PTY session for this workspace
func (s *SessionService) hasPTYSession(workDir string) bool {
	// Method 1: Check active sessions tracking first (most reliable)
	if s.IsActiveSessionActive(workDir) {
		return true
	}

	// Method 2: Check for processes with the workspace directory in their command line
	// This is more specific - look for bash processes that might be running in this directory
	cmd := exec.Command("pgrep", "-f", fmt.Sprintf("bash.*%s", workDir))
	output, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		return true
	}

	// Method 3: Check for Claude processes running in this workspace directory
	// Look for claude processes and check their working directory
	cmd = exec.Command("pgrep", "-f", "claude")
	output, err = cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		pids := strings.Fields(strings.TrimSpace(string(output)))

		for _, pid := range pids {
			// First verify the process is still alive and accessible
			if !s.isProcessAlive(pid) {
				continue
			}

			// Read the working directory of this process
			cwdLink := fmt.Sprintf("/proc/%s/cwd", pid)
			actualWorkDir, err := os.Readlink(cwdLink)
			if err != nil {
				// Log permission or access errors for debugging
				fmt.Printf("🔍 Could not read working directory for claude process %s: %v\n", pid, err)
				continue
			}

			if actualWorkDir == workDir {
				// Claude process found and tracked
				return true
			}
		}
	}

	return false
}

// isProcessAlive checks if a process is still alive and accessible
func (s *SessionService) isProcessAlive(pid string) bool {
	// Check if /proc/PID/stat exists and is readable
	statPath := fmt.Sprintf("/proc/%s/stat", pid)
	_, err := os.Stat(statPath)
	if err != nil {
		return false
	}

	// Additional check: try to read the stat file to ensure process isn't zombie
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return false
	}

	// Check if process state is not zombie (Z) or dead (X)
	// Process stat format: pid (comm) state ppid ...
	statStr := string(statData)
	if strings.Contains(statStr, " Z ") || strings.Contains(statStr, " X ") {
		return false
	}

	return true
}

// FindBestSessionFile finds the best JSONL session file in a project directory
// Uses paths.FindBestSessionFile which validates UUIDs, checks conversation content,
// and filters out forked sessions
// Returns the full file path to the best session, or empty string if none found
func (s *SessionService) FindBestSessionFile(projectDir string) string {
	sessionFile, err := paths.FindBestSessionFile(projectDir)
	if err != nil {
		return ""
	}
	return sessionFile
}

// findNewestClaudeSessionFile finds the best JSONL file in .claude/projects directory
// Uses paths.FindBestSessionFile which properly validates UUIDs, checks conversation content,
// and filters out forked sessions
func (s *SessionService) findNewestClaudeSessionFile(claudeProjectsDir string) string {
	sessionFile, err := paths.FindBestSessionFile(claudeProjectsDir)
	if err != nil || sessionFile == "" {
		return ""
	}

	// Extract session ID from the full path (remove directory and .jsonl extension)
	sessionID := strings.TrimSuffix(filepath.Base(sessionFile), ".jsonl")
	return sessionID
}

// DeleteSessionState removes session state from disk
func (s *SessionService) DeleteSessionState(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	filePath := filepath.Join(s.stateDir, fmt.Sprintf("%s.json", sessionID))

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session state file: %v", err)
	}

	return nil
}

// ListActiveSessions returns all active session states
func (s *SessionService) ListActiveSessions() (map[string]*SessionState, error) {
	sessions := make(map[string]*SessionState)

	files, err := os.ReadDir(s.stateDir)
	if err != nil {
		return sessions, fmt.Errorf("failed to read session state directory: %v", err)
	}

	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			sessionID := file.Name()[:len(file.Name())-5] // Remove .json extension

			state, err := s.LoadSessionState(sessionID)
			if err != nil {
				continue // Skip invalid session files
			}

			if state != nil {
				sessions[sessionID] = state
			}
		}
	}

	return sessions, nil
}

// Active Sessions Methods

// StartActiveSession records a new active session for a workspace directory
func (s *SessionService) StartActiveSession(workspaceDir, claudeSessionUUID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if there's already an active session for this workspace
	if existingSession, exists := s.activeSessions[workspaceDir]; exists {
		// If session is not ended, resume it instead of overwriting
		if existingSession.EndedAt == nil {
			now := time.Now()
			existingSession.ResumedAt = &now
			// Update Claude session UUID if provided
			if claudeSessionUUID != "" {
				existingSession.ClaudeSessionUUID = claudeSessionUUID
			}
			return s.saveActiveSessionsState()
		}
		// If session was ended, we can create a new one (fall through)
	}

	// Create new session
	s.activeSessions[workspaceDir] = &ActiveSessionInfo{
		ClaudeSessionUUID: claudeSessionUUID,
		StartedAt:         time.Now(),
	}

	return s.saveActiveSessionsState()
}

// StartOrResumeActiveSession is a convenience method that handles both new and existing sessions
func (s *SessionService) StartOrResumeActiveSession(workspaceDir, claudeSessionUUID string) (*ActiveSessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if there's already an active session for this workspace
	if existingSession, exists := s.activeSessions[workspaceDir]; exists {
		// If session is not ended, resume it
		if existingSession.EndedAt == nil {
			now := time.Now()
			existingSession.ResumedAt = &now
			// Update Claude session UUID if provided
			if claudeSessionUUID != "" {
				existingSession.ClaudeSessionUUID = claudeSessionUUID
			}
			err := s.saveActiveSessionsState()
			return existingSession, err
		}
		// If session was ended, we can create a new one (fall through)
	}

	// Create new session
	newSession := &ActiveSessionInfo{
		ClaudeSessionUUID: claudeSessionUUID,
		StartedAt:         time.Now(),
	}
	s.activeSessions[workspaceDir] = newSession

	err := s.saveActiveSessionsState()
	return newSession, err
}

// ResumeActiveSession marks an existing session as resumed
func (s *SessionService) ResumeActiveSession(workspaceDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, exists := s.activeSessions[workspaceDir]; exists {
		now := time.Now()
		session.ResumedAt = &now
		// Clear ended timestamp if it was set
		session.EndedAt = nil
		return s.saveActiveSessionsState()
	}

	return fmt.Errorf("no active session found for workspace: %s", workspaceDir)
}

// EndActiveSession marks a session as ended
func (s *SessionService) EndActiveSession(workspaceDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, exists := s.activeSessions[workspaceDir]; exists {
		now := time.Now()
		session.EndedAt = &now
		return s.saveActiveSessionsState()
	}

	return fmt.Errorf("no active session found for workspace: %s", workspaceDir)
}

// AddToSessionHistory adds an entry to the session history without updating the current title
func (s *SessionService) AddToSessionHistory(workspaceDir, title, commitHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	logger.Infof("📝 Adding to session history: %s for workspace: %s", title, workspaceDir)

	session, exists := s.activeSessions[workspaceDir]
	if !exists {
		return fmt.Errorf("no active session found for workspace: %s", workspaceDir)
	}

	// Create history entry
	now := time.Now()
	entry := models.TitleEntry{
		Title:      title,
		Timestamp:  now,
		CommitHash: commitHash,
	}

	// Add to history if not already the last entry
	if len(session.TitleHistory) == 0 || session.TitleHistory[len(session.TitleHistory)-1].Title != title {
		session.TitleHistory = append(session.TitleHistory, entry)
		return s.saveActiveSessionsState()
	}

	return nil
}

// UpdateSessionTitle updates the title for an active session and adds it to history
func (s *SessionService) UpdateSessionTitle(workspaceDir, title, commitHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.activeSessions[workspaceDir]
	if !exists {
		return fmt.Errorf("no active session found for workspace: %s", workspaceDir)
	}

	// Only update if the title has changed
	if session.Title == nil || session.Title.Title != title {
		now := time.Now()
		entry := models.TitleEntry{
			Title:      title,
			Timestamp:  now,
			CommitHash: commitHash,
		}
		session.Title = &entry

		// Add to history if not already the last entry
		if len(session.TitleHistory) == 0 || session.TitleHistory[len(session.TitleHistory)-1].Title != title {
			session.TitleHistory = append(session.TitleHistory, entry)
		}

		// Emit event if eventsHandler is available
		if s.eventsHandler != nil {
			// WorktreeID can be derived from workspaceDir if needed, but for now we'll leave it empty
			// since we're matching by workspace path on the frontend
			s.eventsHandler.EmitSessionTitleUpdated(workspaceDir, "", session.Title, session.TitleHistory)
		}

		return s.saveActiveSessionsState()
	}

	return nil
}

// UpdatePreviousTitleCommitHash updates the commit hash for the previous title entry
func (s *SessionService) UpdatePreviousTitleCommitHash(workspaceDir, commitHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.activeSessions[workspaceDir]
	if !exists {
		return fmt.Errorf("no active session found for workspace: %s", workspaceDir)
	}

	// Update the current title's commit hash if it exists
	if session.Title != nil {
		session.Title.CommitHash = commitHash
	}

	// Also update the last entry in history if it exists
	if len(session.TitleHistory) > 0 {
		lastIndex := len(session.TitleHistory) - 1
		session.TitleHistory[lastIndex].CommitHash = commitHash
	}

	return s.saveActiveSessionsState()
}

// GetPreviousTitle returns the previous title from the session, or empty string if none exists
func (s *SessionService) GetPreviousTitle(workspaceDir string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.activeSessions[workspaceDir]
	if !exists {
		return ""
	}

	// Return the current title if it exists
	if session.Title != nil {
		return session.Title.Title
	}

	return ""
}

// GetActiveSession returns the active session info for a workspace directory
func (s *SessionService) GetActiveSession(workspaceDir string) (*ActiveSessionInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.activeSessions[workspaceDir]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent external modification
	sessionCopy := *session
	return &sessionCopy, true
}

// GetAllActiveSessions returns all active sessions (not ended)
func (s *SessionService) GetAllActiveSessions() map[string]*ActiveSessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	activeSessions := make(map[string]*ActiveSessionInfo)
	for workspaceDir, session := range s.activeSessions {
		if session.EndedAt == nil {
			// Return a copy to prevent external modification
			sessionCopy := *session
			activeSessions[workspaceDir] = &sessionCopy
		}
	}

	return activeSessions
}

// GetAllActiveSessionsIncludingEnded returns all sessions (including ended ones)
func (s *SessionService) GetAllActiveSessionsIncludingEnded() map[string]*ActiveSessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allSessions := make(map[string]*ActiveSessionInfo)
	for workspaceDir, session := range s.activeSessions {
		// Return a copy to prevent external modification
		sessionCopy := *session
		allSessions[workspaceDir] = &sessionCopy
	}

	return allSessions
}

// IsActiveSessionActive checks if a session is currently active (not ended)
func (s *SessionService) IsActiveSessionActive(workspaceDir string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.activeSessions[workspaceDir]
	return exists && session.EndedAt == nil
}

// RemoveActiveSession completely removes a session from the mapping
func (s *SessionService) RemoveActiveSession(workspaceDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.activeSessions, workspaceDir)
	return s.saveActiveSessionsState()
}

// saveActiveSessionsState persists the active sessions mapping to disk
func (s *SessionService) saveActiveSessionsState() error {
	data, err := json.MarshalIndent(s.activeSessions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal active sessions: %v", err)
	}

	filePath := filepath.Join(s.stateDir, "active_sessions.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write active sessions file: %v", err)
	}

	return nil
}

// loadActiveSessionsState loads the active sessions mapping from disk
func (s *SessionService) loadActiveSessionsState() error {
	filePath := filepath.Join(s.stateDir, "active_sessions.json")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// No existing state file, start with empty mapping
		return nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read active sessions file: %v", err)
	}

	if err := json.Unmarshal(data, &s.activeSessions); err != nil {
		return fmt.Errorf("failed to unmarshal active sessions: %v", err)
	}

	return nil
}

// CleanupEndedActiveSessions removes sessions that have been ended for more than the specified duration
func (s *SessionService) CleanupEndedActiveSessions(maxAge time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for workspaceDir, session := range s.activeSessions {
		if session.EndedAt != nil && now.Sub(*session.EndedAt) > maxAge {
			delete(s.activeSessions, workspaceDir)
		}
	}

	return s.saveActiveSessionsState()
}

// DetectEndedSessions automatically detects and marks sessions as ended if no PTY is running in their workspace
func (s *SessionService) DetectEndedSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var changed bool

	for workspaceDir, session := range s.activeSessions {
		// Skip sessions that are already ended
		if session.EndedAt != nil {
			continue
		}

		// Check if there's a running PTY process in this workspace
		if !s.hasRunningPTY(workspaceDir, session.ClaudeSessionUUID) {
			// Mark session as ended
			now := time.Now()
			session.EndedAt = &now
			changed = true
			fmt.Printf("🔚 Detected ended session in workspace: %s\n", workspaceDir)
		}
	}

	if changed {
		return s.saveActiveSessionsState()
	}
	return nil
}

// hasRunningPTY checks if there's a running PTY process in the workspace
func (s *SessionService) hasRunningPTY(workspaceDir, claudeSessionUUID string) bool {
	// Check for any bash or claude processes running in this workspace directory
	// This is a simplified check - in a real implementation you might want to
	// track PIDs or use a more sophisticated method

	// Method 1: Check for processes with the workspace directory in their command line
	cmd := exec.Command("pgrep", "-f", workspaceDir)
	output, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		return true
	}

	// Method 2: Check for active claude sessions by looking for recent activity
	// in the .claude/projects directory
	if claudeSessionUUID != "" {
		claudeProjectsDir := filepath.Join(workspaceDir, ".claude", "projects")
		sessionFile := filepath.Join(claudeProjectsDir, claudeSessionUUID+".jsonl")

		if info, err := os.Stat(sessionFile); err == nil {
			// If the file was modified recently (within last 5 minutes), consider it active
			if time.Since(info.ModTime()) < 5*time.Minute {
				return true
			}
		}
	}

	return false
}
