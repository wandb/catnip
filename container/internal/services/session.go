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
}

// ActiveSessionInfo represents information about an active session in a workspace
type ActiveSessionInfo struct {
	ClaudeSessionUUID string     `json:"claude_session_uuid"`
	StartedAt         time.Time  `json:"started_at"`
	ResumedAt         *time.Time `json:"resumed_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
}

// NewSessionService creates a new session service
func NewSessionService() *SessionService {
	stateDir := "/workspace/.session-state"
	
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

// SaveSessionState persists session state to disk
func (s *SessionService) SaveSessionState(state *SessionState) error {
	if state.ID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}
	
	filePath := filepath.Join(s.stateDir, fmt.Sprintf("%s.json", state.ID))
	
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
	
	filePath := filepath.Join(s.stateDir, fmt.Sprintf("%s.json", sessionID))
	
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
	claudeProjectsDir := filepath.Join(workDir, ".claude", "projects")
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

// findNewestClaudeSessionFile finds the newest JSONL file in .claude/projects directory
func (s *SessionService) findNewestClaudeSessionFile(claudeProjectsDir string) string {
	// Check if .claude/projects directory exists
	if _, err := os.Stat(claudeProjectsDir); os.IsNotExist(err) {
		return ""
	}
	
	files, err := os.ReadDir(claudeProjectsDir)
	if err != nil {
		return ""
	}
	
	var newestFile string
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
		
		// Track the newest file
		if fileInfo.ModTime().After(newestTime) {
			newestTime = fileInfo.ModTime()
			newestFile = sessionID
		}
	}
	
	return newestFile
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
	
	s.activeSessions[workspaceDir] = &ActiveSessionInfo{
		ClaudeSessionUUID: claudeSessionUUID,
		StartedAt:         time.Now(),
	}
	
	return s.saveActiveSessionsState()
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