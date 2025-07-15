package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
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
	// Use catnip user's home directory explicitly
	homeDir := "/home/catnip"
	return &ClaudeService{
		claudeConfigPath:  filepath.Join(homeDir, ".claude.json"),
		claudeProjectsDir: filepath.Join(homeDir, ".claude", "projects"),
	}
}

// readStatusFile reads the status from /project/<branch>/status.txt if it exists
func (s *ClaudeService) readStatusFile(worktreePath string) (*string, error) {
	// Get the branch name from the worktree path
	log.Printf("🔍 [DEBUG] Reading status file for worktree: %s", worktreePath)
	branch := s.getBranchFromWorktreePath(worktreePath)
	if branch == "" {
		return nil, nil // No branch found
	}

	// Construct the status file path
	statusFilePath := filepath.Join("/workspace", branch, "status.txt")

	// Check if the file exists
	if _, err := os.Stat(statusFilePath); os.IsNotExist(err) {
		return nil, nil // File doesn't exist, return nil
	}

	// Read the file content
	content, err := os.ReadFile(statusFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read status file: %w", err)
	}

	// Trim whitespace and return
	status := strings.TrimSpace(string(content))
	if status == "" {
		return nil, nil // Empty file
	}

	log.Printf("🔥 [DEBUG] Status file content: %s", status)

	return &status, nil
}

// getBranchFromWorktreePath extracts the branch name from the worktree path
// The branch is always the last segment of the path (e.g., "workspace/catnip/teleport-otter" -> "teleport-otter")
func (s *ClaudeService) getBranchFromWorktreePath(worktreePath string) string {
	if worktreePath == "" {
		return ""
	}

	// Split the path and get the last segment
	parts := strings.Split(filepath.Clean(worktreePath), "/")
	if len(parts) == 0 {
		return ""
	}

	return parts[len(parts)-1]
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

	// Read status from status.txt file (ignore errors)
	status, err := s.readStatusFile(worktreePath)
	if err == nil {
		summary.Status = status
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
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	projectDir := filepath.Join(s.claudeProjectsDir, projectDirName)

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
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	sessionFile := filepath.Join(s.claudeProjectsDir, projectDirName, sessionID+".jsonl")

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

// GetCompletion sends a completion request to the Anthropic API
func (s *ClaudeService) GetCompletion(req *models.CompletionRequest) (*models.CompletionResponse, error) {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is required")
	}

	// Set defaults
	if req.Model == "" {
		req.Model = "claude-3-5-sonnet-20241022"
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}

	// Build messages array
	messages := []models.AnthropicAPIMessage{}

	// Add context messages if provided
	for _, msg := range req.Context {
		messages = append(messages, models.AnthropicAPIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add the main message
	messages = append(messages, models.AnthropicAPIMessage{
		Role:    "user",
		Content: req.Message,
	})

	// Create the API request
	apiReq := models.AnthropicAPIRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Messages:  messages,
		System:    req.System,
	}

	// Marshal the request
	jsonData, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	// Make the request
	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		// Try to parse structured error
		var apiError models.AnthropicAPIError
		if err := json.Unmarshal(body, &apiError); err == nil {
			return nil, fmt.Errorf("API error: %s", apiError.Error.Message)
		}
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var apiResp models.AnthropicAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract the text content
	var responseText string
	if len(apiResp.Content) > 0 && apiResp.Content[0].Type == "text" {
		responseText = apiResp.Content[0].Text
	}

	// Build the response
	return &models.CompletionResponse{
		Response: responseText,
		Model:    apiResp.Model,
		Usage: models.CompletionUsage{
			InputTokens:  apiResp.Usage.InputTokens,
			OutputTokens: apiResp.Usage.OutputTokens,
			TotalTokens:  apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens,
		},
		Truncated: false,
	}, nil
}
