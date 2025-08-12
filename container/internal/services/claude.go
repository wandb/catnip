package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
)

// ClaudeService manages Claude Code session metadata
type ClaudeService struct {
	claudeConfigPath  string
	claudeProjectsDir string
	volumeProjectsDir string
	settingsPath      string // Path to volume settings.json
	subprocessWrapper ClaudeSubprocessInterface
	// Activity tracking for PTY sessions
	activityMutex sync.RWMutex
	lastActivity  map[string]time.Time // Map of worktree path to last activity time
	// Hook-based activity tracking
	lastUserPromptSubmit map[string]time.Time // Map of worktree path to last UserPromptSubmit time
	lastPostToolUse      map[string]time.Time // Map of worktree path to last PostToolUse time
	lastStopEvent        map[string]time.Time // Map of worktree path to last Stop event time
	// Event suppression for automated operations
	suppressEventsMutex sync.RWMutex
	suppressEventsUntil map[string]time.Time // Map of worktree path to suppression expiry time
}

// readJSONLines reads a JSONL file line by line, handling arbitrarily large lines
// This is used instead of bufio.Scanner to avoid "token too long" errors with large base64 images
func readJSONLines(filePath string, handler func([]byte) error) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				// Handle last line without newline
			} else if err == io.EOF {
				break // Normal end of file
			} else {
				return fmt.Errorf("error reading file: %w", err)
			}
		}

		// Trim newline character
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r") // Handle Windows line endings

		if len(line) == 0 {
			continue // Skip empty lines
		}

		if err := handler([]byte(line)); err != nil {
			// Handler can return an error to stop processing
			return err
		}

		// If we hit EOF while processing the last line, break
		if err == io.EOF {
			break
		}
	}

	return nil
}

// NewClaudeService creates a new Claude service
func NewClaudeService() *ClaudeService {
	// Use runtime-appropriate directories
	homeDir := config.Runtime.HomeDir
	volumeDir := config.Runtime.VolumeDir
	return &ClaudeService{
		claudeConfigPath:     filepath.Join(homeDir, ".claude.json"),
		claudeProjectsDir:    filepath.Join(homeDir, ".claude", "projects"),
		volumeProjectsDir:    filepath.Join(volumeDir, ".claude", ".claude", "projects"),
		settingsPath:         filepath.Join(volumeDir, "settings.json"),
		subprocessWrapper:    NewClaudeSubprocessWrapper(),
		lastActivity:         make(map[string]time.Time),
		lastUserPromptSubmit: make(map[string]time.Time),
		lastPostToolUse:      make(map[string]time.Time),
		lastStopEvent:        make(map[string]time.Time),
		suppressEventsUntil:  make(map[string]time.Time),
	}
}

// NewClaudeServiceWithWrapper creates a new Claude service with a custom subprocess wrapper (for testing)
func NewClaudeServiceWithWrapper(wrapper ClaudeSubprocessInterface) *ClaudeService {
	// Use runtime-appropriate directories
	homeDir := config.Runtime.HomeDir
	volumeDir := config.Runtime.VolumeDir
	return &ClaudeService{
		claudeConfigPath:     filepath.Join(homeDir, ".claude.json"),
		claudeProjectsDir:    filepath.Join(homeDir, ".claude", "projects"),
		volumeProjectsDir:    filepath.Join(volumeDir, ".claude", ".claude", "projects"),
		settingsPath:         filepath.Join(volumeDir, "settings.json"),
		subprocessWrapper:    wrapper,
		lastActivity:         make(map[string]time.Time),
		lastUserPromptSubmit: make(map[string]time.Time),
		lastPostToolUse:      make(map[string]time.Time),
		lastStopEvent:        make(map[string]time.Time),
		suppressEventsUntil:  make(map[string]time.Time),
	}
}

// findProjectDirectory returns the path to the project directory if it exists in either location
func (s *ClaudeService) findProjectDirectory(projectDirName string) string {
	// Check local directory first
	localDir := filepath.Join(s.claudeProjectsDir, projectDirName)
	if _, err := os.Stat(localDir); err == nil {
		return localDir
	}

	// Check volume directory
	volumeDir := filepath.Join(s.volumeProjectsDir, projectDirName)
	if _, err := os.Stat(volumeDir); err == nil {
		return volumeDir
	}

	return ""
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

	// Check if the project directory exists in either location
	// Claude replaces both "/" and "." with "-"
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	projectDirName = strings.ReplaceAll(projectDirName, ".", "-")
	projectDir := s.findProjectDirectory(projectDirName)
	if projectDir == "" {
		// Project directory doesn't exist in either location, skip this session
		return nil, nil
	}

	summary := &models.ClaudeSessionSummary{
		WorktreePath: worktreePath,
		TurnCount:    len(projectMeta.History),
	}

	// Extract header from the most recent history entry
	if len(projectMeta.History) > 0 {
		// Get the most recent history entry
		latestHistory := projectMeta.History[len(projectMeta.History)-1]
		if latestHistory.Display != "" {
			summary.Header = &latestHistory.Display
		}
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

	// Add list of all sessions for this worktree
	allSessions, err := s.GetAllSessionsForWorkspace(worktreePath)
	if err == nil {
		summary.AllSessions = allSessions
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
	// Claude replaces both "/" and "." with "-"
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	projectDirName = strings.ReplaceAll(projectDirName, ".", "-")
	projectDir := s.findProjectDirectory(projectDirName)

	if projectDir == "" {
		return nil, fmt.Errorf("project directory not found for worktree: %s", worktreePath)
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
	hasTimestamp := false

	// Use a closure to capture the result and exit early
	err := readJSONLines(filePath, func(line []byte) error {
		var lineData map[string]interface{}
		if err := json.Unmarshal(line, &lineData); err != nil {
			return nil // Skip invalid JSON lines
		}

		timestampValue, exists := lineData["timestamp"]
		if !exists {
			return nil
		}

		timestampStr, ok := timestampValue.(string)
		if !ok || timestampStr == "" {
			return nil
		}

		if _, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			hasTimestamp = true
			return fmt.Errorf("found timestamp") // Use error to exit early
		}

		return nil
	})

	// If we got an error because we found a timestamp, return true
	if err != nil && err.Error() == "found timestamp" {
		return true
	}

	return hasTimestamp
}

// readSessionTiming reads the first and last timestamps from a session file
func (s *ClaudeService) readSessionTiming(sessionFilePath string) (*SessionTiming, error) {
	var firstTimestamp, lastTimestamp *time.Time

	err := readJSONLines(sessionFilePath, func(line []byte) error {
		// Parse each line as a map to get timestamp
		var lineData map[string]interface{}
		if err := json.Unmarshal(line, &lineData); err != nil {
			return nil // Skip invalid JSON lines, don't stop processing
		}

		// Get timestamp from the map
		timestampValue, exists := lineData["timestamp"]
		if !exists {
			return nil // Skip lines without timestamps
		}

		// Convert to string and skip null/empty values
		timestampStr, ok := timestampValue.(string)
		if !ok || timestampStr == "" {
			return nil // Skip invalid timestamp values
		}

		// Parse the timestamp
		timestamp, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil // Skip invalid timestamps
		}

		// Set first timestamp if not set
		if firstTimestamp == nil {
			firstTimestamp = &timestamp
		}
		// Always update last timestamp
		lastTimestamp = &timestamp

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read session timing: %w", err)
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

// GetFullSessionData gets complete session data for a workspace including all messages
func (s *ClaudeService) GetFullSessionData(worktreePath string, includeFullData bool) (*models.FullSessionData, error) {
	// Get basic session summary
	sessionSummary, err := s.GetWorktreeSessionSummary(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get session summary: %w", err)
	}

	if sessionSummary == nil {
		return nil, nil // No session data for this workspace
	}

	fullData := &models.FullSessionData{
		SessionInfo: sessionSummary,
	}

	// Get all sessions for this workspace
	allSessions, err := s.GetAllSessionsForWorkspace(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get all sessions: %w", err)
	}
	fullData.AllSessions = allSessions

	// Only include full message data if requested
	if includeFullData {
		// Get messages from current/latest session
		var sessionID string
		if sessionSummary.CurrentSessionId != nil {
			sessionID = *sessionSummary.CurrentSessionId
		} else if sessionSummary.LastSessionId != nil {
			sessionID = *sessionSummary.LastSessionId
		}

		if sessionID != "" {
			messages, err := s.GetSessionMessages(worktreePath, sessionID)
			if err == nil {
				fullData.Messages = messages
				fullData.MessageCount = len(messages)
			}
		}

		// Get user prompts from claude.json
		userPrompts, err := s.GetUserPrompts(worktreePath)
		if err == nil {
			fullData.UserPrompts = userPrompts
		}
	}

	return fullData, nil
}

// GetAllSessionsForWorkspace returns all session IDs for a workspace with metadata
func (s *ClaudeService) GetAllSessionsForWorkspace(worktreePath string) ([]models.SessionListEntry, error) {
	// Convert worktree path to project directory name
	// Claude replaces both "/" and "." with "-"
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	projectDirName = strings.ReplaceAll(projectDirName, ".", "-")
	projectDir := s.findProjectDirectory(projectDirName)

	if projectDir == "" {
		return []models.SessionListEntry{}, nil
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.SessionListEntry{}, nil
		}
		return nil, fmt.Errorf("failed to read project directory: %w", err)
	}

	var sessions []models.SessionListEntry

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")

			// Validate UUID format
			if len(sessionID) != 36 || strings.Count(sessionID, "-") != 4 {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			// Get session timing if available
			sessionFile := filepath.Join(projectDir, entry.Name())
			timing, err := s.readSessionTiming(sessionFile)

			sessionEntry := models.SessionListEntry{
				SessionId:    sessionID,
				LastModified: info.ModTime(),
				IsActive:     false, // Will be updated below
			}

			if err == nil {
				sessionEntry.StartTime = timing.StartTime
				sessionEntry.EndTime = timing.EndTime
			}

			sessions = append(sessions, sessionEntry)
		}
	}

	// Sort by last modified (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified.After(sessions[j].LastModified)
	})

	// Mark the most recent session as active if it doesn't have an end time
	if len(sessions) > 0 && sessions[0].EndTime == nil {
		sessions[0].IsActive = true
	}

	return sessions, nil
}

// GetSessionMessages reads all messages from a specific session file
func (s *ClaudeService) GetSessionMessages(worktreePath, sessionID string) ([]models.ClaudeSessionMessage, error) {
	// Convert worktree path to project directory name
	// Claude replaces both "/" and "." with "-"
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	projectDirName = strings.ReplaceAll(projectDirName, ".", "-")
	projectDir := s.findProjectDirectory(projectDirName)

	if projectDir == "" {
		return nil, fmt.Errorf("project directory not found for worktree: %s", worktreePath)
	}

	sessionFile := filepath.Join(projectDir, sessionID+".jsonl")

	var messages []models.ClaudeSessionMessage

	err := readJSONLines(sessionFile, func(line []byte) error {
		var message models.ClaudeSessionMessage
		if err := json.Unmarshal(line, &message); err != nil {
			return nil // Skip invalid JSON lines, don't stop processing
		}
		messages = append(messages, message)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read session messages: %w", err)
	}

	return messages, nil
}

// GetUserPrompts reads user prompts from claude.json for a specific workspace
func (s *ClaudeService) GetUserPrompts(worktreePath string) ([]models.ClaudeHistoryEntry, error) {
	claudeConfig, err := s.readClaudeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read claude config: %w", err)
	}

	projectMeta, exists := claudeConfig[worktreePath]
	if !exists {
		return []models.ClaudeHistoryEntry{}, nil
	}

	return projectMeta.History, nil
}

// GetSessionByID gets complete session data for a specific session ID
func (s *ClaudeService) GetSessionByID(worktreePath, sessionID string) (*models.FullSessionData, error) {
	// Validate session exists
	sessions, err := s.GetAllSessionsForWorkspace(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}

	var targetSession *models.SessionListEntry
	for _, session := range sessions {
		if session.SessionId == sessionID {
			targetSession = &session
			break
		}
	}

	if targetSession == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// Create session summary for this specific session
	sessionSummary := &models.ClaudeSessionSummary{
		WorktreePath:     worktreePath,
		SessionStartTime: targetSession.StartTime,
		SessionEndTime:   targetSession.EndTime,
		IsActive:         targetSession.IsActive,
		CurrentSessionId: &sessionID,
	}

	// Get messages for this session
	messages, err := s.GetSessionMessages(worktreePath, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session messages: %w", err)
	}

	// Get user prompts
	userPrompts, err := s.GetUserPrompts(worktreePath)
	if err != nil {
		userPrompts = []models.ClaudeHistoryEntry{} // Don't fail if we can't get prompts
	}

	return &models.FullSessionData{
		SessionInfo:  sessionSummary,
		AllSessions:  sessions,
		Messages:     messages,
		UserPrompts:  userPrompts,
		MessageCount: len(messages),
	}, nil
}

// GetSessionByUUID gets complete session data for a specific session UUID
func (s *ClaudeService) GetSessionByUUID(sessionUUID string) (*models.FullSessionData, error) {
	// First, find which worktree this session belongs to
	allSummaries, err := s.GetAllWorktreeSessionSummaries()
	if err != nil {
		return nil, fmt.Errorf("failed to get all summaries: %w", err)
	}

	var targetWorktree string
	for worktreePath, summary := range allSummaries {
		// Check if this session UUID is in the allSessions list
		for _, session := range summary.AllSessions {
			if session.SessionId == sessionUUID {
				targetWorktree = worktreePath
				break
			}
		}
		if targetWorktree != "" {
			break
		}

		// Also check current and last session IDs
		if (summary.CurrentSessionId != nil && *summary.CurrentSessionId == sessionUUID) ||
			(summary.LastSessionId != nil && *summary.LastSessionId == sessionUUID) {
			targetWorktree = worktreePath
			break
		}
	}

	if targetWorktree == "" {
		return nil, fmt.Errorf("session not found: %s", sessionUUID)
	}

	// Get the session data using the existing method
	return s.GetSessionByID(targetWorktree, sessionUUID)
}

// GetLatestTodos gets the most recent Todo structure from the session history
func (s *ClaudeService) GetLatestTodos(worktreePath string) ([]models.Todo, error) {
	// Convert worktree path to project directory name
	// /workspace/vllmulator/midnight -> -workspace-vllmulator-midnight
	// Claude replaces both "/" and "." with "-"
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	projectDirName = strings.ReplaceAll(projectDirName, ".", "-")
	projectDirName = strings.TrimPrefix(projectDirName, "-")
	projectDirName = "-" + projectDirName // Add back the leading dash
	projectDir := s.findProjectDirectory(projectDirName)

	if projectDir == "" {
		return nil, fmt.Errorf("project directory not found for worktree: %s", worktreePath)
	}

	// Find the most recent session file
	sessionFile, err := s.findLatestSessionFile(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find latest session file: %w", err)
	}

	// Look for the most recent TodoWrite tool call in the session
	var latestTodos []models.Todo

	err = readJSONLines(sessionFile, func(line []byte) error {
		var message models.ClaudeSessionMessage
		if err := json.Unmarshal(line, &message); err != nil {
			return nil // Skip invalid JSON lines
		}

		// Check if this is an assistant message that might contain TodoWrite
		if message.Type == "assistant" && message.Message != nil {
			messageData := message.Message
			if content, exists := messageData["content"]; exists {
				if contentArray, ok := content.([]interface{}); ok {
					for _, contentItem := range contentArray {
						if contentMap, ok := contentItem.(map[string]interface{}); ok {
							if contentType, exists := contentMap["type"]; exists && contentType == "tool_use" {
								if name, exists := contentMap["name"]; exists && name == "TodoWrite" {
									if input, exists := contentMap["input"]; exists {
										if inputMap, ok := input.(map[string]interface{}); ok {
											if todos, exists := inputMap["todos"]; exists {
												if todosArray, ok := todos.([]interface{}); ok {
													var parsedTodos []models.Todo
													for _, todoItem := range todosArray {
														if todoMap, ok := todoItem.(map[string]interface{}); ok {
															todo := models.Todo{}
															if id, exists := todoMap["id"]; exists {
																if idStr, ok := id.(string); ok {
																	todo.ID = idStr
																}
															}
															if content, exists := todoMap["content"]; exists {
																if contentStr, ok := content.(string); ok {
																	todo.Content = contentStr
																}
															}
															if status, exists := todoMap["status"]; exists {
																if statusStr, ok := status.(string); ok {
																	todo.Status = statusStr
																}
															}
															if priority, exists := todoMap["priority"]; exists {
																if priorityStr, ok := priority.(string); ok {
																	todo.Priority = priorityStr
																}
															}
															parsedTodos = append(parsedTodos, todo)
														}
													}
													// Update latestTodos with the most recent one found
													latestTodos = parsedTodos
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	return latestTodos, nil
}

// CreateCompletion creates a completion using the claude CLI subprocess
func (s *ClaudeService) CreateCompletion(ctx context.Context, req *models.CreateCompletionRequest) (*models.CreateCompletionResponse, error) {
	// Validate required fields
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Set default working directory if not provided
	workingDir := req.WorkingDirectory
	if workingDir == "" {
		workingDir = filepath.Join(config.Runtime.WorkspaceDir, "current")
	}

	// Default SuppressEvents to true for all internal calls
	// This prevents duplicate notifications during automated tasks like branch renaming
	// Since Go's zero value for bool is false, if SuppressEvents is not set in the request,
	// we'll default it to true to avoid spurious notifications from internal Claude calls
	suppressEvents := true
	// Note: Currently all internal calls (like branch renaming) don't set SuppressEvents,
	// so they'll use the default of true. External API calls can explicitly set it to false
	// if they want notifications.

	// Set up subprocess options
	opts := &ClaudeSubprocessOptions{
		Prompt:           req.Prompt,
		SystemPrompt:     req.SystemPrompt,
		Model:            req.Model,
		MaxTurns:         req.MaxTurns,
		WorkingDirectory: workingDir,
		Resume:           req.Resume,
		SuppressEvents:   suppressEvents,
	}

	// Enable event suppression for automated operations
	if suppressEvents {
		s.SetSuppressEvents(workingDir, true)
		defer func() {
			s.SetSuppressEvents(workingDir, false)
		}()
	}

	// Resume logic is handled by claude CLI's --continue flag

	// Call the subprocess wrapper
	result, err := s.subprocessWrapper.CreateCompletion(ctx, opts)

	// Ensure suppression is cleared even on error
	if req.SuppressEvents {
		s.SetSuppressEvents(workingDir, false)
	}

	return result, err
}

// CreateStreamingCompletion creates a streaming completion using the claude CLI subprocess
func (s *ClaudeService) CreateStreamingCompletion(ctx context.Context, req *models.CreateCompletionRequest, responseWriter io.Writer) error {
	// Validate required fields
	if req.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}

	// Set default working directory if not provided
	workingDir := req.WorkingDirectory
	if workingDir == "" {
		workingDir = filepath.Join(config.Runtime.WorkspaceDir, "current")
	}

	// Default SuppressEvents to true for all internal calls
	// This prevents duplicate notifications during automated tasks like branch renaming
	// Since Go's zero value for bool is false, if SuppressEvents is not set in the request,
	// we'll default it to true to avoid spurious notifications from internal Claude calls
	suppressEvents := true
	// Note: Currently all internal calls (like branch renaming) don't set SuppressEvents,
	// so they'll use the default of true. External API calls can explicitly set it to false
	// if they want notifications.

	// Set up subprocess options for streaming
	opts := &ClaudeSubprocessOptions{
		Prompt:           req.Prompt,
		SystemPrompt:     req.SystemPrompt,
		Model:            req.Model,
		MaxTurns:         req.MaxTurns,
		WorkingDirectory: workingDir,
		Resume:           req.Resume,
		SuppressEvents:   suppressEvents,
	}

	// Enable event suppression for automated operations
	if suppressEvents {
		s.SetSuppressEvents(workingDir, true)
		defer func() {
			s.SetSuppressEvents(workingDir, false)
		}()
	}

	// Resume logic is handled by claude CLI's --continue flag

	// Call the subprocess wrapper for streaming
	err := s.subprocessWrapper.CreateStreamingCompletion(ctx, opts, responseWriter)

	// Ensure suppression is cleared even on error
	if req.SuppressEvents {
		s.SetSuppressEvents(workingDir, false)
	}

	return err
}

// GetClaudeSettings reads Claude configuration settings from ~/.claude.json and volume settings.json
func (s *ClaudeService) GetClaudeSettings() (*models.ClaudeSettings, error) {
	data, err := os.ReadFile(s.claudeConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default settings if file doesn't exist
			return &models.ClaudeSettings{
				Theme:                  "dark", // Default theme
				IsAuthenticated:        false,
				Version:                "",
				HasCompletedOnboarding: false,
				NumStartups:            0,
				NotificationsEnabled:   true, // Default to enabled
			}, nil
		}
		return nil, fmt.Errorf("failed to read claude config file: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse claude config: %w", err)
	}

	settings := &models.ClaudeSettings{
		Theme:                  "dark", // Default theme
		IsAuthenticated:        false,
		Version:                "",
		HasCompletedOnboarding: false,
		NumStartups:            0,
		NotificationsEnabled:   true, // Default to enabled
	}

	// Extract theme (default to "dark" if not set)
	if theme, exists := config["theme"]; exists {
		if themeStr, ok := theme.(string); ok {
			settings.Theme = themeStr
		}
	}

	// Check authentication status based on userID
	if userID, exists := config["userID"]; exists {
		if userIDStr, ok := userID.(string); ok && userIDStr != "" {
			settings.IsAuthenticated = true
		}
	}

	// Extract version from lastReleaseNotesSeen
	if lastRelease, exists := config["lastReleaseNotesSeen"]; exists {
		if lastReleaseStr, ok := lastRelease.(string); ok && lastReleaseStr != "" {
			settings.Version = lastReleaseStr
		}
	}

	// Extract onboarding status
	if onboarding, exists := config["hasCompletedOnboarding"]; exists {
		if onboardingBool, ok := onboarding.(bool); ok {
			settings.HasCompletedOnboarding = onboardingBool
		}
	}

	// Extract startup count
	if startups, exists := config["numStartups"]; exists {
		if startupsFloat, ok := startups.(float64); ok {
			settings.NumStartups = int(startupsFloat)
		}
	}

	// Read notifications setting from volume settings.json
	notificationsEnabled, err := s.getNotificationsEnabled()
	if err == nil {
		settings.NotificationsEnabled = notificationsEnabled
	}

	return settings, nil
}

// UpdateClaudeSettings updates Claude configuration settings in ~/.claude.json and volume settings.json
func (s *ClaudeService) UpdateClaudeSettings(req *models.ClaudeSettingsUpdateRequest) (*models.ClaudeSettings, error) {
	// Handle theme updates (update ~/.claude.json)
	if req.Theme != "" {
		// Read current config
		var config map[string]interface{}

		data, err := os.ReadFile(s.claudeConfigPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Create new config if file doesn't exist
				config = make(map[string]interface{})
			} else {
				return nil, fmt.Errorf("failed to read claude config file: %w", err)
			}
		} else {
			if err := json.Unmarshal(data, &config); err != nil {
				return nil, fmt.Errorf("failed to parse claude config: %w", err)
			}
		}

		// Update theme
		config["theme"] = req.Theme

		// Write back to file with proper formatting
		updatedData, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}

		// Create a temporary file first (atomic write)
		tempFile := s.claudeConfigPath + ".tmp"
		if err := os.WriteFile(tempFile, updatedData, 0644); err != nil {
			return nil, fmt.Errorf("failed to write temp config file: %w", err)
		}

		// Atomically rename temp file to final destination
		if err := os.Rename(tempFile, s.claudeConfigPath); err != nil {
			os.Remove(tempFile) // Clean up temp file on error
			return nil, fmt.Errorf("failed to update config file: %w", err)
		}

		// Set proper ownership for catnip user
		if err := os.Chown(s.claudeConfigPath, 1000, 1000); err != nil {
			// Log but don't fail
			fmt.Printf("Warning: Failed to chown %s: %v\n", s.claudeConfigPath, err)
		}
	}

	// Handle notifications updates (update volume settings.json)
	if req.NotificationsEnabled != nil {
		if err := s.setNotificationsEnabled(*req.NotificationsEnabled); err != nil {
			return nil, fmt.Errorf("failed to update notifications setting: %w", err)
		}
	}

	// Return updated settings
	return s.GetClaudeSettings()
}

// getNotificationsEnabled reads notifications setting from volume settings.json
func (s *ClaudeService) getNotificationsEnabled() (bool, error) {
	data, err := os.ReadFile(s.settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Default to enabled if file doesn't exist
			return true, nil
		}
		return false, fmt.Errorf("failed to read settings file: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, fmt.Errorf("failed to parse settings file: %w", err)
	}

	if notifications, exists := settings["notificationsEnabled"]; exists {
		if notificationsBool, ok := notifications.(bool); ok {
			return notificationsBool, nil
		}
	}

	// Default to enabled if setting doesn't exist
	return true, nil
}

// setNotificationsEnabled writes notifications setting to volume settings.json
func (s *ClaudeService) setNotificationsEnabled(enabled bool) error {
	// Read current settings or create new ones
	var settings map[string]interface{}

	data, err := os.ReadFile(s.settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new settings if file doesn't exist
			settings = make(map[string]interface{})
		} else {
			return fmt.Errorf("failed to read settings file: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("failed to parse settings file: %w", err)
		}
	}

	// Update notifications setting
	settings["notificationsEnabled"] = enabled

	// Write back to file with proper formatting
	updatedData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(s.settingsPath), 0755); err != nil {
		return fmt.Errorf("failed to create settings directory: %w", err)
	}

	// Create a temporary file first (atomic write)
	tempFile := s.settingsPath + ".tmp"
	if err := os.WriteFile(tempFile, updatedData, 0644); err != nil {
		return fmt.Errorf("failed to write temp settings file: %w", err)
	}

	// Atomically rename temp file to final destination
	if err := os.Rename(tempFile, s.settingsPath); err != nil {
		os.Remove(tempFile) // Clean up temp file on error
		return fmt.Errorf("failed to update settings file: %w", err)
	}

	// Set proper ownership for catnip user
	if err := os.Chown(s.settingsPath, 1000, 1000); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: Failed to chown %s: %v\n", s.settingsPath, err)
	}

	return nil
}

// UpdateActivity records activity for a Claude session in a specific worktree
func (s *ClaudeService) UpdateActivity(worktreePath string) {
	s.activityMutex.Lock()
	s.lastActivity[worktreePath] = time.Now()
	s.activityMutex.Unlock()
}

// GetLastActivity returns the last activity time for a worktree, or zero time if no activity
func (s *ClaudeService) GetLastActivity(worktreePath string) time.Time {
	s.activityMutex.RLock()
	defer s.activityMutex.RUnlock()
	return s.lastActivity[worktreePath]
}

// IsActiveSession returns true if the session has been active within the specified duration
func (s *ClaudeService) IsActiveSession(worktreePath string, within time.Duration) bool {
	lastActivity := s.GetLastActivity(worktreePath)

	if lastActivity.IsZero() {
		return false
	}
	return time.Since(lastActivity) <= within
}

// SetSuppressEvents sets event suppression for a worktree with a 30-second timeout (dead man switch)
func (s *ClaudeService) SetSuppressEvents(worktreePath string, suppress bool) {
	s.suppressEventsMutex.Lock()
	defer s.suppressEventsMutex.Unlock()

	if suppress {
		// Set suppression with 30-second timeout (dead man switch) for automated operations like PR creation
		s.suppressEventsUntil[worktreePath] = time.Now().Add(30 * time.Second)
		logger.Debugf("🔕 Event suppression enabled for %s (expires in 30s)", worktreePath)
	} else {
		// Clear suppression
		delete(s.suppressEventsUntil, worktreePath)
		logger.Debugf("🔊 Event suppression disabled for %s", worktreePath)
	}
}

// IsSuppressingEvents checks if events should be suppressed for a worktree (with dead man switch cleanup)
func (s *ClaudeService) IsSuppressingEvents(worktreePath string) bool {
	s.suppressEventsMutex.Lock()
	defer s.suppressEventsMutex.Unlock()

	// Normalize the path to worktree root for consistent suppression checking
	normalizedPath := s.normalizeToWorktreeRoot(worktreePath)

	suppressUntil, exists := s.suppressEventsUntil[normalizedPath]
	if !exists {
		return false
	}

	// Check if suppression has expired (dead man switch)
	if time.Now().After(suppressUntil) {
		// Clean up expired suppression
		delete(s.suppressEventsUntil, normalizedPath)
		logger.Debugf("🔊 Event suppression expired for %s (dead man switch)", normalizedPath)
		return false
	}

	return true
}

// normalizeToWorktreeRoot normalizes a subdirectory path to its worktree root using path prefix matching
func (s *ClaudeService) normalizeToWorktreeRoot(workingDir string) string {
	// If not under /workspace, return as-is
	if !strings.HasPrefix(workingDir, "/workspace/") {
		return workingDir
	}

	// Extract the worktree root pattern: /workspace/{repo}/{worktree}
	// Example: /workspace/catnip/earl/container -> /workspace/catnip/earl
	parts := strings.Split(workingDir, "/")
	if len(parts) >= 4 && parts[0] == "" && parts[1] == "workspace" {
		// Reconstruct the worktree root path: /workspace/{repo}/{worktree}
		worktreeRoot := "/" + strings.Join(parts[1:4], "/")
		return worktreeRoot
	}

	// If pattern doesn't match expected structure, return original
	return workingDir
}

// HandleHookEvent processes Claude Code hook events for activity tracking
func (s *ClaudeService) HandleHookEvent(event *models.ClaudeHookEvent) error {
	// Normalize subdirectory paths to worktree root for consistent activity tracking
	worktreeRoot := s.normalizeToWorktreeRoot(event.WorkingDirectory)

	// Check if events are suppressed for this worktree
	if s.IsSuppressingEvents(worktreeRoot) {
		logger.Debugf("🔕 Suppressing %s hook event for %s", event.EventType, event.WorkingDirectory)
		return nil
	}

	s.activityMutex.Lock()
	defer s.activityMutex.Unlock()

	now := time.Now()

	switch event.EventType {
	case "UserPromptSubmit":
		// Track both general activity and specific prompt submit using worktree root
		s.lastActivity[worktreeRoot] = now
		s.lastUserPromptSubmit[worktreeRoot] = now
		logger.Debugf("🎯 Claude hook: UserPromptSubmit in %s (normalized from %s)", worktreeRoot, event.WorkingDirectory)
		return nil
	case "PostToolUse":
		// Track both general activity and specific tool use (heartbeat) using worktree root
		s.lastActivity[worktreeRoot] = now
		s.lastPostToolUse[worktreeRoot] = now
		logger.Debugf("🔧 Claude hook: PostToolUse in %s (normalized from %s)", worktreeRoot, event.WorkingDirectory)
		return nil
	case "Stop":
		// Track both general activity and specific stop event using worktree root
		s.lastActivity[worktreeRoot] = now
		s.lastStopEvent[worktreeRoot] = now
		logger.Debugf("🛑 Claude hook: Stop in %s (normalized from %s)", worktreeRoot, event.WorkingDirectory)
		return nil
	default:
		// For other events, just update general activity timestamp using worktree root
		s.lastActivity[worktreeRoot] = now
		logger.Debugf("🔍 Claude hook: %s in %s (normalized from %s)", event.EventType, worktreeRoot, event.WorkingDirectory)
		return nil
	}
}

// GetLastUserPromptSubmit returns the last UserPromptSubmit event time for a worktree
func (s *ClaudeService) GetLastUserPromptSubmit(worktreePath string) time.Time {
	s.activityMutex.RLock()
	defer s.activityMutex.RUnlock()
	return s.lastUserPromptSubmit[worktreePath]
}

// GetLastPostToolUse returns the last PostToolUse event time for a worktree
func (s *ClaudeService) GetLastPostToolUse(worktreePath string) time.Time {
	s.activityMutex.RLock()
	defer s.activityMutex.RUnlock()
	return s.lastPostToolUse[worktreePath]
}

// GetLastStopEvent returns the last Stop event time for a worktree
func (s *ClaudeService) GetLastStopEvent(worktreePath string) time.Time {
	s.activityMutex.RLock()
	defer s.activityMutex.RUnlock()
	return s.lastStopEvent[worktreePath]
}
