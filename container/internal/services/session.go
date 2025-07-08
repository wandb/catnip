package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// SessionService manages session state persistence
type SessionService struct {
	stateDir string
}

// NewSessionService creates a new session service
func NewSessionService() *SessionService {
	stateDir := "/workspace/.session-state"
	
	// Ensure state directory exists
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		fmt.Printf("Warning: Failed to create session state directory: %v\n", err)
	}
	
	return &SessionService{
		stateDir: stateDir,
	}
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