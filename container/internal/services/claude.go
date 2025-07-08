package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// ClaudeService manages Claude Code session metadata
type ClaudeService struct {
	claudeConfigPath  string
	claudeProjectsDir string
}

// NewClaudeService creates a new Claude service
func NewClaudeService() *ClaudeService {
	// Use catnip user's home directory explicitly
	homeDir := "/home/catnip"
	return &ClaudeService{
		claudeConfigPath:  filepath.Join(homeDir, ".claude.json"),
		claudeProjectsDir: filepath.Join(homeDir, ".claude", "projects"),
	}
}

// GetWorktreeSessionSummary gets Claude session information for a worktree
func (s *ClaudeService) GetWorktreeSessionSummary(worktreePath string) (*models.ClaudeSessionSummary, error) {
	// Read claude.json
	claudeConfig, err := s.readClaudeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read claude config: %w", err)
	}
	
	// Find project metadata for this worktree
	projectMeta, exists := claudeConfig[worktreePath]
	if !exists {
		// Return nil instead of error for worktrees without Claude sessions
		return nil, nil
	}
	
	summary := &models.ClaudeSessionSummary{
		WorktreePath: worktreePath,
		TurnCount:    len(projectMeta.History),
	}
	
	// Check if this is an active session (no completion metrics)
	summary.IsActive = projectMeta.LastSessionId == nil
	
	if projectMeta.LastSessionId != nil {
		summary.LastSessionId = projectMeta.LastSessionId
		summary.LastCost = projectMeta.LastCost
		summary.LastDuration = projectMeta.LastDuration
		summary.LastTotalInputTokens = projectMeta.LastTotalInputTokens
		summary.LastTotalOutputTokens = projectMeta.LastTotalOutputTokens
	}
	
	// Get session timing from session files (ignore errors)
	sessionTiming, err := s.getSessionTiming(worktreePath)
	if err == nil {
		summary.SessionStartTime = sessionTiming.StartTime
		
		// For completed sessions, always show end time (even if same as start)
		// For active sessions, only show end time if we have distinct timestamps
		if !summary.IsActive {
			// Completed session - show end time even if it's the same as start time
			if sessionTiming.EndTime != nil {
				summary.SessionEndTime = sessionTiming.EndTime
			} else {
				summary.SessionEndTime = sessionTiming.StartTime
			}
		} else {
			// Active session - only show end time if different from start
			summary.SessionEndTime = sessionTiming.EndTime
		}
		
		summary.CurrentSessionId = &sessionTiming.SessionID
	}
	
	return summary, nil
}

// GetAllWorktreeSessionSummaries gets session summaries for all worktrees with Claude data
func (s *ClaudeService) GetAllWorktreeSessionSummaries() (map[string]*models.ClaudeSessionSummary, error) {
	claudeConfig, err := s.readClaudeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read claude config: %w", err)
	}
	
	summaries := make(map[string]*models.ClaudeSessionSummary)
	
	for worktreePath := range claudeConfig {
		summary, err := s.GetWorktreeSessionSummary(worktreePath)
		if err == nil && summary != nil {
			summaries[worktreePath] = summary
		}
	}
	
	return summaries, nil
}

// SessionTiming represents start and end times for a session
type SessionTiming struct {
	StartTime *time.Time
	EndTime   *time.Time
}

// SessionTimingWithID includes session ID along with timing
type SessionTimingWithID struct {
	SessionTiming
	SessionID string
}

// getSessionTiming extracts session start and end times from session files
func (s *ClaudeService) getSessionTiming(worktreePath string) (*SessionTimingWithID, error) {
	// Convert worktree path to project directory name
	// "/workspace/openui/debug-quokka" -> "-workspace-openui-debug-quokka"
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	projectDir := filepath.Join(s.claudeProjectsDir, projectDirName)
	
	// Check if the projects directory exists
	if _, err := os.Stat(s.claudeProjectsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude projects directory does not exist: %s", s.claudeProjectsDir)
	}
	
	// Find the most recent session file
	sessionFile, err := s.findLatestSessionFile(projectDir)
	if err != nil {
		return nil, err
	}
	
	// Extract session ID from filename
	sessionID := strings.TrimSuffix(filepath.Base(sessionFile), ".jsonl")
	
	// Read session file and extract timing
	timing, err := s.readSessionTiming(sessionFile)
	if err != nil {
		return nil, err
	}
	
	return &SessionTimingWithID{
		SessionTiming: *timing,
		SessionID:     sessionID,
	}, nil
}

// findLatestSessionFile finds the most recent session file with content
func (s *ClaudeService) findLatestSessionFile(projectDir string) (string, error) {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("project directory does not exist: %s", projectDir)
		}
		return "", fmt.Errorf("failed to read project directory: %w", err)
	}
	
	var sessionFiles []fs.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			sessionFiles = append(sessionFiles, entry)
		}
	}
	
	if len(sessionFiles) == 0 {
		return "", fmt.Errorf("no session files found in %s", projectDir)
	}
	
	// Sort by modification time (most recent first)
	sort.Slice(sessionFiles, func(i, j int) bool {
		infoI, _ := sessionFiles[i].Info()
		infoJ, _ := sessionFiles[j].Info()
		return infoI.ModTime().After(infoJ.ModTime())
	})
	
	// Files are already sorted by modification time (newest first)
	// Find the first (newest) file that has timestamps
	for _, entry := range sessionFiles {
		filePath := filepath.Join(projectDir, entry.Name())
		if s.fileHasTimestamps(filePath) {
			return filePath, nil
		}
	}
	
	// If no files have timestamps, return the most recent one anyway
	return filepath.Join(projectDir, sessionFiles[0].Name()), nil
}

// fileHasTimestamps checks if a session file contains at least one valid timestamp
func (s *ClaudeService) fileHasTimestamps(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var lineData map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &lineData); err != nil {
			continue
		}
		
		timestampValue, exists := lineData["timestamp"]
		if !exists {
			continue
		}
		
		timestampStr, ok := timestampValue.(string)
		if !ok || timestampStr == "" {
			continue
		}
		
		if _, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			return true // Found at least one valid timestamp
		}
	}
	
	return false
}

// readSessionTiming reads the first and last timestamps from a session file
func (s *ClaudeService) readSessionTiming(sessionFilePath string) (*SessionTiming, error) {
	file, err := os.Open(sessionFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()
	
	var firstTimestamp, lastTimestamp *time.Time
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Parse each line as a map to get timestamp
		var lineData map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &lineData); err != nil {
			continue // Skip invalid JSON lines
		}
		
		// Get timestamp from the map
		timestampValue, exists := lineData["timestamp"]
		if !exists {
			continue
		}
		
		// Convert to string and skip null/empty values
		timestampStr, ok := timestampValue.(string)
		if !ok || timestampStr == "" {
			continue
		}
		
		// Parse the timestamp
		timestamp, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			continue // Skip invalid timestamps
		}
		
		// Set first timestamp if not set
		if firstTimestamp == nil {
			firstTimestamp = &timestamp
		}
		// Always update last timestamp
		lastTimestamp = &timestamp
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading session file: %w", err)
	}
	
	return &SessionTiming{
		StartTime: firstTimestamp,
		EndTime:   lastTimestamp,
	}, nil
}

// readClaudeConfig reads and parses the ~/.claude.json file
func (s *ClaudeService) readClaudeConfig() (map[string]*models.ClaudeProjectMetadata, error) {
	data, err := os.ReadFile(s.claudeConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty map if file doesn't exist
			return make(map[string]*models.ClaudeProjectMetadata), nil
		}
		return nil, fmt.Errorf("failed to read claude config file: %w", err)
	}
	
	var config struct {
		Projects map[string]*models.ClaudeProjectMetadata `json:"projects"`
	}
	
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse claude config: %w", err)
	}
	
	// Handle case where projects is nil
	if config.Projects == nil {
		return make(map[string]*models.ClaudeProjectMetadata), nil
	}
	
	// Set the path for each project
	for path, project := range config.Projects {
		project.Path = path
	}
	
	return config.Projects, nil
}