package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/vanpelt/catnip/internal/claude/parser"
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
	sessionService    *SessionService // For best session file selection
	parserService     *ParserService  // Centralized session file parser management
	// Process registry for persistent streaming processes
	processRegistry *ClaudeProcessRegistry
	// Activity tracking for PTY sessions
	activityMutex sync.RWMutex
	lastActivity  map[string]time.Time // Map of worktree path to last activity time
	// Hook-based activity tracking
	lastUserPromptSubmit map[string]time.Time // Map of worktree path to last UserPromptSubmit time
	lastPostToolUse      map[string]time.Time // Map of worktree path to last PostToolUse time
	lastStopEvent        map[string]time.Time // Map of worktree path to last Stop event time
	lastSessionStart     map[string]time.Time // Map of worktree path to last SessionStart time
	// Event suppression for automated operations
	suppressEventsMutex sync.RWMutex
	suppressEventsUntil map[string]time.Time // Map of worktree path to suppression expiry time
}

func WorktreePathToProjectDir(worktreePath string) string {
	// Claude replaces both "/" and "." with "-"
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	projectDirName = strings.ReplaceAll(projectDirName, ".", "-")
	projectDirName = strings.TrimPrefix(projectDirName, "-")
	projectDirName = "-" + projectDirName
	return projectDirName
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
		processRegistry:      NewClaudeProcessRegistry(),
		lastActivity:         make(map[string]time.Time),
		lastUserPromptSubmit: make(map[string]time.Time),
		lastPostToolUse:      make(map[string]time.Time),
		lastStopEvent:        make(map[string]time.Time),
		lastSessionStart:     make(map[string]time.Time),
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
		processRegistry:      NewClaudeProcessRegistry(),
		lastActivity:         make(map[string]time.Time),
		lastUserPromptSubmit: make(map[string]time.Time),
		lastPostToolUse:      make(map[string]time.Time),
		lastStopEvent:        make(map[string]time.Time),
		lastSessionStart:     make(map[string]time.Time),
		suppressEventsUntil:  make(map[string]time.Time),
	}
}

// SetSessionService sets the session service for best session file selection
func (s *ClaudeService) SetSessionService(sessionService *SessionService) {
	s.sessionService = sessionService
}

// SetParserService sets the parser service for centralized session file parsing
func (s *ClaudeService) SetParserService(parserService *ParserService) {
	s.parserService = parserService
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

	projectDirName := WorktreePathToProjectDir(worktreePath)
	projectDir := s.findProjectDirectory(projectDirName)
	if projectDir == "" {
		// Project directory doesn't exist in either location, skip this session
		return nil, nil
	}

	summary := &models.ClaudeSessionSummary{
		WorktreePath: worktreePath,
	}

	// Get user prompts and header from parser (reads from history.jsonl or .claude.json)
	userPrompts, err := s.GetUserPrompts(worktreePath)
	if err == nil {
		summary.TurnCount = len(userPrompts)
		// Use the latest user prompt as the header
		if len(userPrompts) > 0 {
			latestPrompt := userPrompts[len(userPrompts)-1].Display
			summary.Header = &latestPrompt
		}
	} else {
		// Fallback to old logic if parser fails
		summary.TurnCount = len(projectMeta.History)
		if len(projectMeta.History) > 0 {
			latestHistory := projectMeta.History[len(projectMeta.History)-1]
			if latestHistory.Display != "" {
				summary.Header = &latestHistory.Display
			}
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
	projectDirName := WorktreePathToProjectDir(worktreePath)
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

// findLatestSessionFile finds the best session file with content
// Uses SessionService's size-based logic to avoid warmup/small sessions
func (s *ClaudeService) findLatestSessionFile(projectDir string) (string, error) {
	// Use SessionService's proven logic that filters by size (>10KB) and prefers largest sessions
	if s.sessionService != nil {
		sessionFile := s.sessionService.FindBestSessionFile(projectDir)
		if sessionFile != "" {
			return sessionFile, nil
		}
	}

	// Fallback to old logic if SessionService not available (shouldn't happen in production)
	logger.Warn("⚠️ SessionService not set in ClaudeService, using fallback session selection")

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

	// Return the most recent file
	return filepath.Join(projectDir, sessionFiles[0].Name()), nil
}

// readSessionTiming reads the first and last timestamps from a session file
func (s *ClaudeService) readSessionTiming(sessionFilePath string) (*SessionTiming, error) {
	// Use parser to get timing from stats
	reader := parser.NewSessionFileReader(sessionFilePath)
	if err := reader.ReadFull(); err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	stats := reader.GetStats()

	var startTime, endTime *time.Time
	if !stats.FirstMessageTime.IsZero() {
		startTime = &stats.FirstMessageTime
	}
	if !stats.LastMessageTime.IsZero() {
		endTime = &stats.LastMessageTime
	}

	return &SessionTiming{
		StartTime: startTime,
		EndTime:   endTime,
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

	// Note: History is no longer loaded here - use parser.HistoryReader via parserService instead
	// This keeps project metadata separate from user prompt history
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
	projectDirName := WorktreePathToProjectDir(worktreePath)
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
	projectDirName := WorktreePathToProjectDir(worktreePath)
	projectDir := s.findProjectDirectory(projectDirName)

	if projectDir == "" {
		return nil, fmt.Errorf("project directory not found for worktree: %s", worktreePath)
	}

	sessionFile := filepath.Join(projectDir, sessionID+".jsonl")

	// Use parser to read and filter messages
	reader := parser.NewSessionFileReader(sessionFile)
	return reader.GetAllMessages(parser.DefaultFilter)
}

// GetUserPrompts reads user prompts for a specific workspace
// Uses the parser service which reads from both ~/.claude/history.jsonl and ~/.claude.json
func (s *ClaudeService) GetUserPrompts(worktreePath string) ([]models.ClaudeHistoryEntry, error) {
	reader, err := s.parserService.GetOrCreateParser(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get parser: %w", err)
	}
	return reader.GetUserPrompts()
}

// GetLatestUserPrompt gets the latest user prompt for a specific workspace
// Uses the parser service which reads from both ~/.claude/history.jsonl and ~/.claude.json
func (s *ClaudeService) GetLatestUserPrompt(worktreePath string) (string, error) {
	reader, err := s.parserService.GetOrCreateParser(worktreePath)
	if err != nil {
		return "", fmt.Errorf("failed to get parser: %w", err)
	}
	return reader.GetLatestUserPrompt()
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
// Now uses the centralized parser service for efficient, cached access
func (s *ClaudeService) GetLatestTodos(worktreePath string) ([]models.Todo, error) {
	if s.parserService == nil {
		return nil, fmt.Errorf("parser service not initialized")
	}

	// Get or create parser for this worktree (handles finding session file)
	reader, err := s.parserService.GetOrCreateParser(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get parser for worktree %s: %w", worktreePath, err)
	}

	// Refresh parser to get latest todos from file
	_, _ = reader.ReadIncremental() // Ignore errors - use cached data if refresh fails

	// Parser maintains cached todos, updated incrementally
	todos := reader.GetTodos()
	return todos, nil
}

// GetLatestAssistantMessage gets the most recent assistant message from the session history
func (s *ClaudeService) GetLatestAssistantMessage(worktreePath string) (string, error) {
	if s.parserService == nil {
		return "", fmt.Errorf("parser service not initialized")
	}

	reader, err := s.parserService.GetOrCreateParser(worktreePath)
	if err != nil {
		return "", fmt.Errorf("failed to get parser for worktree %s: %w", worktreePath, err)
	}

	// Refresh parser to get latest messages from file
	_, _ = reader.ReadIncremental() // Ignore errors - use cached data if refresh fails

	latestMsg := reader.GetLatestMessage()
	if latestMsg == nil {
		return "", nil
	}

	// Only return content for assistant, summary, or error messages (not user messages)
	if latestMsg.Type == "assistant" || latestMsg.Type == "summary" || latestMsg.Type == "error" {
		content := parser.ExtractTextContent(*latestMsg)
		return content, nil
	}

	// Latest message is a user message, return empty
	return "", nil
}

// GetLatestAssistantMessageOrError gets the most recent assistant message OR error from the session history
func (s *ClaudeService) GetLatestAssistantMessageOrError(worktreePath string) (content string, isError bool, err error) {
	if s.parserService == nil {
		return "", false, fmt.Errorf("parser service not initialized")
	}

	reader, err := s.parserService.GetOrCreateParser(worktreePath)
	if err != nil {
		return "", false, fmt.Errorf("failed to get parser for worktree %s: %w", worktreePath, err)
	}

	// Refresh parser to get latest messages from file
	_, _ = reader.ReadIncremental() // Ignore errors - use cached data if refresh fails

	latestMsg := reader.GetLatestMessage()
	if latestMsg == nil {
		return "", false, nil
	}

	// Check if this is an error message
	if latestMsg.Type == "error" && latestMsg.Message != nil {
		if contentVal, exists := latestMsg.Message["content"]; exists {
			if contentStr, ok := contentVal.(string); ok {
				return contentStr, true, nil
			}
		}
	}

	// Check for assistant message with error content blocks
	if latestMsg.Type == "assistant" && latestMsg.Message != nil {
		if contentVal, exists := latestMsg.Message["content"]; exists {
			if contentArray, ok := contentVal.([]interface{}); ok {
				var textContent strings.Builder
				var foundError bool

				for _, contentItem := range contentArray {
					if contentMap, ok := contentItem.(map[string]interface{}); ok {
						if contentType, exists := contentMap["type"]; exists {
							switch contentType {
							case "error":
								if text, exists := contentMap["text"]; exists {
									if textStr, ok := text.(string); ok {
										textContent.WriteString(textStr)
										foundError = true
									}
								}
							case "text":
								if text, exists := contentMap["text"]; exists {
									if textStr, ok := text.(string); ok {
										textContent.WriteString(textStr)
									}
								}
							}
						}
					}
				}

				if textContent.Len() > 0 {
					return textContent.String(), foundError, nil
				}
			}
		}
	}

	// Only return content for assistant, summary, or error messages (not user messages)
	if latestMsg.Type == "assistant" || latestMsg.Type == "summary" || latestMsg.Type == "error" {
		return parser.ExtractTextContent(*latestMsg), false, nil
	}

	// No assistant message found
	return "", false, nil
}

// CreateCompletion creates a completion using the claude CLI subprocess
func (s *ClaudeService) CreateCompletion(ctx context.Context, req *models.CreateCompletionRequest) (*models.CreateCompletionResponse, error) {
	// Validate required fields
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Default fork=true when resuming (unless explicitly set to false)
	// This ensures forked sessions don't pollute original session history
	if req.Resume && req.Fork == nil {
		forkTrue := true
		req.Fork = &forkTrue
		logger.Debug("🔀 Resuming session, defaulting to fork=true")
	}

	// When fork is requested, automatically use haiku model for fast, cheap responses
	// Fork is used for automated operations (PR summaries, branch names) that don't need
	// the full power of larger models
	if req.Fork != nil && *req.Fork && req.Model == "" {
		req.Model = "claude-haiku-4-5"
		logger.Debugf("🔀 Fork requested, auto-selecting haiku model for fast response")
	}

	// Set default working directory if not provided
	workingDir := req.WorkingDirectory
	if workingDir == "" {
		workingDir = filepath.Join(config.Runtime.WorkspaceDir, "current")
	} else {
		// Resolve container paths (like /workspace/...) to actual paths
		workingDir = config.Runtime.ResolvePath(workingDir)
	}

	// Default SuppressEvents to true for all internal calls
	// This prevents duplicate notifications during automated tasks like branch renaming
	// Since Go's zero value for bool is false, if SuppressEvents is not set in the request,
	// we'll default it to true to avoid spurious notifications from internal Claude calls
	suppressEvents := true
	// Note: Currently all internal calls (like branch renaming) don't set SuppressEvents,
	// so they'll use the default of true. External API calls can explicitly set it to false
	// if they want notifications.

	// Get the best session ID if resuming (needed for both resume and fork since fork uses --resume too)
	var sessionID string
	if req.Resume && s.sessionService != nil {
		if existingState, err := s.sessionService.FindSessionByDirectory(workingDir); err == nil && existingState != nil {
			sessionID = existingState.ClaudeSessionID
			if sessionID != "" {
				logger.Infof("🔄 Found best session %s for resume in %s", sessionID, workingDir)
			}
		}
	}

	// Determine fork value (default false if not set)
	fork := req.Fork != nil && *req.Fork

	// Set up subprocess options
	opts := &ClaudeSubprocessOptions{
		Prompt:           req.Prompt,
		SystemPrompt:     req.SystemPrompt,
		Model:            req.Model,
		MaxTurns:         req.MaxTurns,
		WorkingDirectory: workingDir,
		Resume:           req.Resume,
		Fork:             fork,
		SessionID:        sessionID,
		SuppressEvents:   suppressEvents,
	}

	// Enable event suppression for automated operations
	if suppressEvents {
		s.SetSuppressEvents(workingDir, true)
		defer func() {
			s.SetSuppressEvents(workingDir, false)
		}()
	}

	// Resume logic is handled by claude CLI's --resume/--continue flags

	// Call the subprocess wrapper
	result, err := s.subprocessWrapper.CreateCompletion(ctx, opts)

	// Ensure suppression is cleared even on error
	if req.SuppressEvents {
		s.SetSuppressEvents(workingDir, false)
	}

	return result, err
}

// CreateStreamingCompletionPTY creates a PTY-based streaming completion that enables interactive Claude features
func (s *ClaudeService) CreateStreamingCompletionPTY(ctx context.Context, req *models.CreateCompletionRequest, responseWriter io.Writer) error {
	// Validate required fields
	if req.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}

	// Set default working directory if not provided
	workingDir := req.WorkingDirectory
	if workingDir == "" {
		workingDir = filepath.Join(config.Runtime.WorkspaceDir, "current")
	} else {
		// Resolve container paths (like /workspace/...) to actual paths
		workingDir = config.Runtime.ResolvePath(workingDir)
	}

	// Create PTY session manager for this request
	// Note: PTY sessions are for interactive use, so fork is not relevant here
	ptyManager := &ClaudePTYManager{
		workingDir:     workingDir,
		prompt:         req.Prompt,
		systemPrompt:   req.SystemPrompt,
		model:          req.Model,
		maxTurns:       req.MaxTurns,
		resume:         req.Resume,
		suppressEvents: req.SuppressEvents,
		claudeService:  s,
		ctx:            ctx,
		responseWriter: responseWriter,
	}

	return ptyManager.Start()
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
	} else {
		// Resolve container paths (like /workspace/...) to actual paths
		workingDir = config.Runtime.ResolvePath(workingDir)
	}

	// Use the SuppressEvents value from the request
	// For user-initiated streaming requests, this should be false to allow events
	// For internal calls, this should be true to prevent duplicate notifications
	suppressEvents := req.SuppressEvents

	// Determine fork value (default false if not set)
	fork := req.Fork != nil && *req.Fork

	// Get the best session ID if resuming (needed for both resume and fork since fork uses --resume too)
	var sessionID string
	if req.Resume && s.sessionService != nil {
		if existingState, err := s.sessionService.FindSessionByDirectory(workingDir); err == nil && existingState != nil {
			sessionID = existingState.ClaudeSessionID
			if sessionID != "" {
				logger.Infof("🔄 Found best session %s for streaming resume in %s", sessionID, workingDir)
			}
		}
	}

	// Set up subprocess options for streaming
	opts := &ClaudeSubprocessOptions{
		Prompt:           req.Prompt,
		SystemPrompt:     req.SystemPrompt,
		Model:            req.Model,
		MaxTurns:         req.MaxTurns,
		WorkingDirectory: workingDir,
		Resume:           req.Resume,
		Fork:             fork,
		SessionID:        sessionID,
		SuppressEvents:   suppressEvents,
	}

	// Enable event suppression for automated operations
	if suppressEvents {
		s.SetSuppressEvents(workingDir, true)
		defer func() {
			s.SetSuppressEvents(workingDir, false)
		}()
	}

	// Check if we have a real wrapper or a mock - use process registry only for real wrapper
	if wrapper, ok := s.subprocessWrapper.(*ClaudeSubprocessWrapper); ok {
		// Use process registry for persistent streaming with real wrapper
		process, isNew, err := s.processRegistry.GetOrCreateProcess(opts, wrapper)
		if err != nil {
			return fmt.Errorf("failed to get or create persistent process: %w", err)
		}

		// Generate client ID for this connection
		clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())

		// Add this client to the process
		outputCh := process.AddClient(clientID)
		defer process.RemoveClient(clientID)

		// Stream historical events from project.jsonl if this is a reconnection
		if !isNew {
			logger.Infof("🔄 Streaming historical events for reconnection to %s", workingDir)
			if err := s.StreamHistoricalEvents(workingDir, responseWriter); err != nil {
				logger.Warnf("Failed to stream historical events: %v", err)
			}
		}

		// Stream live output from the process
		for {
			select {
			case output, ok := <-outputCh:
				if !ok {
					// Process completed or client was removed
					return nil
				}
				if _, err := responseWriter.Write(output); err != nil {
					return fmt.Errorf("failed to write output: %w", err)
				}
				// Flush if possible (for Server-Sent Events)
				if flusher, ok := responseWriter.(interface{ Flush() }); ok {
					flusher.Flush()
				}
			case <-ctx.Done():
				// Client disconnected, but process continues running
				logger.Infof("📡 Client disconnected from %s, but process continues", workingDir)
				return ctx.Err()
			}
		}
	} else {
		// For mock or other implementations, use direct streaming
		return s.subprocessWrapper.CreateStreamingCompletion(ctx, opts, responseWriter)
	}
}

// StreamHistoricalEvents streams recent events from project.jsonl files for reconnection
func (s *ClaudeService) StreamHistoricalEvents(worktreePath string, responseWriter io.Writer) error {
	projectDirName := WorktreePathToProjectDir(worktreePath)
	projectDir := s.findProjectDirectory(projectDirName)

	if projectDir == "" {
		// No historical events to stream
		return nil
	}

	// Find the most recent session file
	sessionFile, err := s.findLatestSessionFile(projectDir)
	if err != nil {
		return fmt.Errorf("failed to find latest session file: %w", err)
	}

	logger.Debugf("📜 Streaming historical events from %s", sessionFile)

	// Use parser to read and filter messages
	reader := parser.NewSessionFileReader(sessionFile)
	filter := parser.MessageFilter{
		SkipWarmup:    true,
		SkipAutomated: true,
		OnlyType:      "assistant", // Only stream assistant messages for historical context
	}
	messages, err := reader.GetAllMessages(filter)
	if err != nil {
		return fmt.Errorf("failed to read session file: %w", err)
	}

	// Extract text content and convert to response format
	var recentMessages [][]byte
	for _, message := range messages {
		if message.Message != nil {
			if content, exists := message.Message["content"]; exists {
				if contentArray, ok := content.([]interface{}); ok && len(contentArray) > 0 {
					if textBlock, ok := contentArray[0].(map[string]interface{}); ok {
						if text, ok := textBlock["text"].(string); ok {
							response := &models.CreateCompletionResponse{
								Response: text,
								IsChunk:  true,
								IsLast:   false,
							}

							if responseJSON, err := json.Marshal(response); err == nil {
								recentMessages = append(recentMessages, append(responseJSON, '\n'))
							}
						}
					}
				}
			}
		}
	}

	// Send the recent messages (limit to last 3 for performance)
	start := 0
	if len(recentMessages) > 3 {
		start = len(recentMessages) - 3
	}

	for i := start; i < len(recentMessages); i++ {
		if _, err := responseWriter.Write(recentMessages[i]); err != nil {
			return fmt.Errorf("failed to write historical message: %w", err)
		}

		// Flush if possible
		if flusher, ok := responseWriter.(interface{ Flush() }); ok {
			flusher.Flush()
		}
	}

	logger.Debugf("📜 Streamed %d historical messages", len(recentMessages))
	return nil
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

	// Check authentication status based on credentials file existence
	// Don't rely on userID in config - check if credentials actually exist
	credentialsPath := filepath.Join(os.Getenv("HOME"), ".claude", ".credentials.json")
	if _, err := os.Stat(credentialsPath); err == nil {
		settings.IsAuthenticated = true
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
	// If not under workspace directory, return as-is
	workspacePrefix := config.Runtime.WorkspaceDir + "/"
	if !strings.HasPrefix(workingDir, workspacePrefix) {
		return workingDir
	}

	// Extract the worktree root pattern: {workspaceDir}/{repo}/{worktree}
	// Example: /worktrees/catnip/earl/container -> /worktrees/catnip/earl
	parts := strings.Split(workingDir, "/")
	workspaceDirName := filepath.Base(config.Runtime.WorkspaceDir)

	// Find the workspace directory in the path parts
	workspaceDirIndex := -1
	for i, part := range parts {
		if part == workspaceDirName {
			workspaceDirIndex = i
			break
		}
	}

	// If we found the workspace directory and have enough parts for repo/worktree
	if workspaceDirIndex >= 0 && len(parts) >= workspaceDirIndex+3 {
		// Reconstruct the worktree root path: {workspaceDir}/{repo}/{worktree}
		worktreeRoot := "/" + strings.Join(parts[1:workspaceDirIndex+3], "/")
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
	case "SessionStart":
		// Track both general activity and specific session start using worktree root
		s.lastActivity[worktreeRoot] = now
		s.lastSessionStart[worktreeRoot] = now
		logger.Debugf("🚀 Claude hook: SessionStart in %s (normalized from %s)", worktreeRoot, event.WorkingDirectory)
		return nil
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

// GetLastSessionStart returns the last SessionStart event time for a worktree
func (s *ClaudeService) GetLastSessionStart(worktreePath string) time.Time {
	s.activityMutex.RLock()
	defer s.activityMutex.RUnlock()
	return s.lastSessionStart[worktreePath]
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

// CleanupWorktreeClaudeFiles removes all Claude session files for a worktree path
// This should be called when creating a new worktree to prevent stale session data
func (s *ClaudeService) CleanupWorktreeClaudeFiles(worktreePath string) error {
	// Get the project directory name for this worktree
	projectDirName := WorktreePathToProjectDir(worktreePath)

	// Check both local and volume project directories
	localProjectDir := filepath.Join(s.claudeProjectsDir, projectDirName)
	volumeProjectDir := filepath.Join(s.volumeProjectsDir, projectDirName)

	var cleanupErrors []string

	// Clean up local project directory if it exists
	if _, err := os.Stat(localProjectDir); err == nil {
		logger.Infof("🧹 Cleaning up Claude session files in local directory: %s", localProjectDir)
		if err := os.RemoveAll(localProjectDir); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Sprintf("failed to remove local project dir %s: %v", localProjectDir, err))
		} else {
			logger.Debugf("✅ Successfully removed local Claude project directory: %s", localProjectDir)
		}
	}

	// Clean up volume project directory if it exists
	if _, err := os.Stat(volumeProjectDir); err == nil {
		logger.Infof("🧹 Cleaning up Claude session files in volume directory: %s", volumeProjectDir)
		if err := os.RemoveAll(volumeProjectDir); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Sprintf("failed to remove volume project dir %s: %v", volumeProjectDir, err))
		} else {
			logger.Debugf("✅ Successfully removed volume Claude project directory: %s", volumeProjectDir)
		}
	}

	// TODO: CRITICAL BUG FIX - DO NOT MODIFY CLAUDE.JSON DURING OPERATION!
	// The removeClaudeConfigEntry function has a catastrophic bug that destroys user authentication.
	// It only preserves the 'projects' field and nukes all OAuth/auth data when writing back.
	// We should NEVER modify ~/.claude.json during operation - it should be READ-ONLY.
	// Consider using a separate metadata file like ~/.catnip-projects.json for runtime tracking.
	//
	// DISABLED to prevent auth corruption:
	// if err := s.removeClaudeConfigEntry(worktreePath); err != nil {
	// 	cleanupErrors = append(cleanupErrors, fmt.Sprintf("failed to clean claude.json entry: %v", err))
	// }

	// Clear in-memory activity tracking for this worktree
	s.activityMutex.Lock()
	delete(s.lastActivity, worktreePath)
	delete(s.lastUserPromptSubmit, worktreePath)
	delete(s.lastPostToolUse, worktreePath)
	delete(s.lastStopEvent, worktreePath)
	delete(s.lastSessionStart, worktreePath)
	s.activityMutex.Unlock()

	// Clear event suppression for this worktree
	s.suppressEventsMutex.Lock()
	delete(s.suppressEventsUntil, worktreePath)
	s.suppressEventsMutex.Unlock()

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("cleanup completed with errors: %s", strings.Join(cleanupErrors, "; "))
	}

	logger.Debugf("✅ Successfully cleaned up all Claude files for worktree: %s", worktreePath)
	return nil
}

// removeClaudeConfigEntry removes the claude.json entry for a specific worktree path
//
// ⚠️  CRITICAL BUG: This function has a catastrophic bug that destroys user authentication!
// It only preserves the 'projects' field (lines 1503-1507) and overwrites the entire file,
// which NUKES all OAuth account data, custom API keys, and other critical auth information.
// This function should NEVER be called during operation - claude.json should be READ-ONLY.
//
// TODO: Replace with separate metadata file like ~/.catnip-projects.json for runtime tracking.
//
//nolint:unused // TODO: Remove after claude.json management is refactored
func (s *ClaudeService) removeClaudeConfigEntry(worktreePath string) error {
	// Read current config
	claudeConfig, err := s.readClaudeConfig()
	if err != nil {
		return fmt.Errorf("failed to read claude config: %w", err)
	}

	// Check if entry exists
	if _, exists := claudeConfig[worktreePath]; !exists {
		return nil // Nothing to remove
	}

	// Remove the entry
	delete(claudeConfig, worktreePath)

	// Write back the config
	configData := struct {
		Projects map[string]*models.ClaudeProjectMetadata `json:"projects"`
	}{
		Projects: claudeConfig,
	}

	updatedData, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	// Create a temporary file first (atomic write)
	tempFile := s.claudeConfigPath + ".tmp"
	if err := os.WriteFile(tempFile, updatedData, 0644); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	// Atomically rename temp file to final destination
	if err := os.Rename(tempFile, s.claudeConfigPath); err != nil {
		os.Remove(tempFile) // Clean up temp file on error
		return fmt.Errorf("failed to update config file: %w", err)
	}

	// Set proper ownership for catnip user
	if err := os.Chown(s.claudeConfigPath, 1000, 1000); err != nil {
		// Log but don't fail
		logger.Warnf("Warning: Failed to chown %s: %v", s.claudeConfigPath, err)
	}

	logger.Debugf("✅ Removed claude.json entry for worktree: %s", worktreePath)
	return nil
}

// GetProcessRegistry returns the process registry for external access
func (s *ClaudeService) GetProcessRegistry() *ClaudeProcessRegistry {
	return s.processRegistry
}

// Shutdown gracefully shuts down the Claude service
func (s *ClaudeService) Shutdown() {
	logger.Infof("🔚 Shutting down Claude service...")
	if s.processRegistry != nil {
		s.processRegistry.Shutdown()
	}
}

// ClaudePTYManager manages PTY-based Claude sessions for streaming completions
type ClaudePTYManager struct {
	workingDir     string
	prompt         string
	systemPrompt   string
	model          string
	maxTurns       int
	resume         bool
	suppressEvents bool
	claudeService  *ClaudeService
	ctx            context.Context
	responseWriter io.Writer

	// PTY management
	pty          *os.File
	cmd          *exec.Cmd
	sessionReady chan struct{}
	promptSent   chan struct{}

	// JSONL streaming
	sessionID     string
	projectDir    string
	sessionFile   string
	lastStreamPos int64
	streamingDone chan struct{}
}

// Start initiates the PTY-based Claude session and handles streaming
func (m *ClaudePTYManager) Start() error {
	logger.Infof("🚀 Starting PTY-based Claude streaming session in %s", m.workingDir)

	// Initialize channels
	m.sessionReady = make(chan struct{})
	m.promptSent = make(chan struct{})
	m.streamingDone = make(chan struct{})

	// Create PTY and start Claude
	if err := m.createPTY(); err != nil {
		return fmt.Errorf("failed to create PTY: %w", err)
	}
	defer m.cleanup()

	// Start goroutines for PTY management
	errCh := make(chan error, 3)

	// Monitor for SessionStart hook, then send prompt and stream JSONL
	go m.waitForSessionStartAndStream(errCh)

	// Read PTY output (but don't stream it - we'll stream JSONL instead)
	go m.readPTYOutput(errCh)

	// Wait for completion or error
	select {
	case err := <-errCh:
		return err
	case <-m.streamingDone:
		logger.Infof("✅ PTY-based Claude streaming session completed")
		return nil
	case <-m.ctx.Done():
		logger.Infof("📡 PTY-based Claude streaming session cancelled")
		return m.ctx.Err()
	}
}

// createPTY creates the PTY and starts the Claude process
func (m *ClaudePTYManager) createPTY() error {
	// Find Claude executable using the same logic as PTY handler
	claudePath := m.findClaudeExecutable()

	// Build Claude command with optional continue or resume flag
	args := []string{"--dangerously-skip-permissions"}

	if m.resume {
		// Try to find the best session ID for resume
		var sessionID string
		if m.claudeService.sessionService != nil {
			if existingState, err := m.claudeService.sessionService.FindSessionByDirectory(m.workingDir); err == nil && existingState != nil {
				sessionID = existingState.ClaudeSessionID
			}
		}

		if sessionID != "" {
			// Use --resume with specific session ID for precise session selection
			args = append(args, "--resume", sessionID)
			logger.Infof("🔄 Starting Claude Code with --resume %s for PTY streaming", sessionID)
		} else {
			// Fallback to --continue which auto-detects session
			args = append(args, "--continue")
			logger.Infof("🔄 Starting Claude Code with --continue for PTY streaming (no session ID found)")
		}
	} else {
		logger.Debugf("🤖 Starting new Claude Code session for PTY streaming")
	}

	// Create command
	m.cmd = exec.Command(claudePath, args...)
	m.cmd.Dir = m.workingDir
	m.cmd.Env = append(os.Environ(),
		"HOME="+config.Runtime.HomeDir,
		"TERM=xterm-direct",
		"COLORTERM=truecolor",
	)

	// Create PTY
	var err error
	m.pty, err = pty.Start(m.cmd)
	if err != nil {
		return fmt.Errorf("failed to start Claude with PTY: %w", err)
	}

	logger.Infof("✅ PTY-based Claude process started, PID: %d", m.cmd.Process.Pid)
	return nil
}

// findClaudeExecutable finds the Claude executable using the same logic as PTY handler
func (m *ClaudePTYManager) findClaudeExecutable() string {
	// PRIORITY 1: Try Catnip's wrapper script first (for title interception)
	catnipClaudePath := "/opt/catnip/bin/claude"
	if _, err := os.Stat(catnipClaudePath); err == nil {
		return catnipClaudePath
	}

	// PRIORITY 2: Try standard PATH lookup
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}

	// PRIORITY 3: Try ~/.local/bin/claude
	if homeDir, err := os.UserHomeDir(); err == nil {
		userClaudePath := filepath.Join(homeDir, ".local", "bin", "claude")
		if _, err := os.Stat(userClaudePath); err == nil {
			return userClaudePath
		}
	}

	// Fallback
	return "claude"
}

// waitForSessionStartAndStream waits for SessionStart, sends prompt, then streams JSONL
func (m *ClaudePTYManager) waitForSessionStartAndStream(errCh chan<- error) {
	logger.Infof("⏳ Waiting for SessionStart hook for PTY streaming session in %s", m.workingDir)

	// Record the current time so we only accept SessionStart events after PTY creation
	startTime := time.Now()

	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:
			logger.Errorf("❌ Timeout waiting for SessionStart hook after 30 seconds")
			errCh <- fmt.Errorf("timeout waiting for SessionStart hook")
			return
		case <-ticker.C:
			// Check if we've received a SessionStart hook for this working directory AFTER we started the PTY
			lastSessionStart := m.claudeService.GetLastSessionStart(m.workingDir)
			if !lastSessionStart.IsZero() && lastSessionStart.After(startTime) {
				logger.Infof("🚀 SessionStart hook received for PTY streaming at %v (PTY started at %v)", lastSessionStart, startTime)

				// Wait 1 second after SessionStart, then send prompt
				logger.Infof("⏳ Waiting 1 second after SessionStart for safety...")
				time.Sleep(1 * time.Second)

				logger.Infof("📝 Injecting prompt into PTY: %q", m.prompt)

				// First send the prompt text (just like pty.go does)
				if _, err := m.pty.Write([]byte(m.prompt)); err != nil {
					logger.Errorf("❌ Failed to write prompt to PTY: %v", err)
					errCh <- fmt.Errorf("failed to send prompt to PTY: %w", err)
					return
				}

				// Small delay to let the TUI process the prompt text (like pty.go)
				time.Sleep(100 * time.Millisecond)

				// Then send carriage return to submit (exactly like pty.go)
				logger.Infof("↩️ Sending carriage return (\\r) to execute prompt")
				if _, err := m.pty.Write([]byte("\r")); err != nil {
					logger.Errorf("❌ Failed to write carriage return to PTY: %v", err)
					errCh <- fmt.Errorf("failed to send carriage return to PTY: %w", err)
					return
				}

				logger.Infof("✅ Prompt submitted to Claude successfully")

				// Now wait for session file and stream it
				if err := m.waitForSessionFileAndStream(); err != nil {
					errCh <- err
					return
				}

				close(m.streamingDone)
				return
			} else if !lastSessionStart.IsZero() {
				logger.Debugf("🔍 SessionStart hook exists but is too old: %v (need after %v)", lastSessionStart, startTime)
			}
		case <-m.ctx.Done():
			errCh <- m.ctx.Err()
			return
		}
	}
}

// readPTYOutput reads PTY output to prevent buffer filling and logs it for debugging
func (m *ClaudePTYManager) readPTYOutput(errCh chan<- error) {
	logger.Infof("📖 Starting PTY output reader with debugging for %s", m.workingDir)

	// Create debug file to capture PTY output
	debugFile := fmt.Sprintf("/tmp/claude-pty-debug-%d.log", time.Now().Unix())
	f, err := os.Create(debugFile)
	if err != nil {
		logger.Errorf("❌ Failed to create debug file: %v", err)
	} else {
		defer f.Close()
		logger.Infof("📝 Writing PTY output to debug file: %s", debugFile)
	}

	buf := make([]byte, 1024)
	totalBytes := 0

	for {
		select {
		case <-m.ctx.Done():
			logger.Infof("📖 PTY output reader stopping, total bytes read: %d", totalBytes)
			if f != nil {
				_, _ = fmt.Fprintf(f, "\n=== PTY reader stopped, total bytes: %d ===\n", totalBytes)
			}
			return
		default:
		}

		// Read PTY output
		n, err := m.pty.Read(buf)
		if err != nil {
			if err == io.EOF {
				logger.Infof("📖 PTY output reader: Claude process ended (total bytes: %d)", totalBytes)
				if f != nil {
					_, _ = fmt.Fprintf(f, "\n=== Claude process ended, total bytes: %d ===\n", totalBytes)
				}
				return
			}
			logger.Warnf("⚠️ PTY read error after %d bytes: %v", totalBytes, err)
			if f != nil {
				_, _ = fmt.Fprintf(f, "\n=== PTY read error after %d bytes: %v ===\n", totalBytes, err)
			}
			return
		}

		if n > 0 {
			totalBytes += n

			// Log the output for debugging
			output := string(buf[:n])
			logger.Debugf("📖 PTY output (%d bytes): %q", n, output)

			// Write to debug file
			if f != nil {
				_, _ = fmt.Fprintf(f, "=== %d bytes at %s ===\n", n, time.Now().Format("15:04:05.000"))
				_, _ = f.Write(buf[:n])
				_, _ = f.WriteString("\n")
				_ = f.Sync() // Flush immediately
			}
		}
	}
}

// waitForSessionFileAndStream waits for the JSONL session file to appear and streams its content
func (m *ClaudePTYManager) waitForSessionFileAndStream() error {
	logger.Debugf("🔍 Starting JSONL monitoring and streaming")

	// Wait for JSONL session file to appear (Claude creates it after receiving prompt)
	if err := m.waitForSessionFile(); err != nil {
		return err
	}

	// Start streaming JSONL content
	return m.streamJSONLContent()
}

// waitForSessionFile waits for the Claude JSONL session file to appear
func (m *ClaudePTYManager) waitForSessionFile() error {
	logger.Debugf("⏳ Waiting for Claude session file to appear")

	timeout := time.NewTimer(30 * time.Second) // Increased from 15 seconds
	defer timeout.Stop()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Calculate expected project directory
	projectDirName := WorktreePathToProjectDir(m.workingDir)
	m.projectDir = filepath.Join(config.Runtime.HomeDir, ".claude", "projects", projectDirName)

	for {
		select {
		case <-timeout.C:
			return fmt.Errorf("timeout waiting for Claude session file to appear")
		case <-ticker.C:
			// Look for newest JSONL file
			sessionID := m.findNewestSessionFile()
			if sessionID != "" {
				m.sessionID = sessionID
				m.sessionFile = filepath.Join(m.projectDir, sessionID+".jsonl")
				logger.Infof("🎯 Found Claude session file: %s", sessionID)
				return nil
			}
		case <-m.ctx.Done():
			return m.ctx.Err()
		}
	}
}

// findNewestSessionFile finds the newest JSONL session file in the project directory
func (m *ClaudePTYManager) findNewestSessionFile() string {
	entries, err := os.ReadDir(m.projectDir)
	if err != nil {
		return ""
	}

	var newestFile string
	var newestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		// Extract session ID from filename
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")

		// Validate UUID format
		if len(sessionID) != 36 || strings.Count(sessionID, "-") != 4 {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestFile = sessionID
		}
	}

	return newestFile
}

// streamJSONLContent streams the JSONL file content as it's written
func (m *ClaudePTYManager) streamJSONLContent() error {
	logger.Infof("📡 Starting to stream JSONL content from %s", m.sessionFile)

	file, err := os.Open(m.sessionFile)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	// Stream existing content first
	if err := m.streamExistingContent(file); err != nil {
		return err
	}

	// Then monitor for new content
	return m.monitorForNewContent(file)
}

// streamExistingContent streams any content already in the file
func (m *ClaudePTYManager) streamExistingContent(file *os.File) error {
	// Read from current position
	_, err := file.Seek(m.lastStreamPos, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek in session file: %w", err)
	}

	return m.processFileContent(file)
}

// monitorForNewContent monitors the file for new content and streams it
func (m *ClaudePTYManager) monitorForNewContent(file *os.File) error {
	logger.Debugf("👀 Monitoring for new content in session file")

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.NewTimer(5 * time.Minute)
	defer timeout.Stop()

	lastSize := m.lastStreamPos

	for {
		select {
		case <-timeout.C:
			logger.Infof("⏰ JSONL streaming timeout reached")
			return nil
		case <-ticker.C:
			// Check file size
			stat, err := file.Stat()
			if err != nil {
				return fmt.Errorf("failed to stat session file: %w", err)
			}

			if stat.Size() > lastSize {
				// New content available
				if _, err := file.Seek(lastSize, io.SeekStart); err != nil {
					return fmt.Errorf("failed to seek in session file: %w", err)
				}

				if err := m.processFileContent(file); err != nil {
					return err
				}

				lastSize = stat.Size()

				// Check if we've reached completion by looking for "Stop" event
				if m.isSessionComplete() {
					logger.Infof("✅ Claude session completed")
					return nil
				}
			}
		case <-m.ctx.Done():
			return m.ctx.Err()
		}
	}
}

// processFileContent reads JSONL content and converts it to streaming format
func (m *ClaudePTYManager) processFileContent(file *os.File) error {
	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read session file: %w", err)
		}

		// Update position
		m.lastStreamPos += int64(len(line))

		// Parse and convert JSONL message to streaming format
		if err := m.convertAndStreamMessage(line); err != nil {
			logger.Warnf("⚠️ Failed to process message: %v", err)
			continue
		}
	}

	// Update position to current file position
	pos, err := file.Seek(0, io.SeekCurrent)
	if err == nil {
		m.lastStreamPos = pos
	}

	return nil
}

// convertAndStreamMessage converts a JSONL message to streaming format and writes it
func (m *ClaudePTYManager) convertAndStreamMessage(line []byte) error {
	var message models.ClaudeSessionMessage
	if err := json.Unmarshal(line, &message); err != nil {
		return err
	}

	// Only stream assistant messages with text content
	if message.Type == "assistant" && message.Message != nil {
		messageData := message.Message
		if content, exists := messageData["content"]; exists {
			if contentArray, ok := content.([]interface{}); ok && len(contentArray) > 0 {
				if textBlock, ok := contentArray[0].(map[string]interface{}); ok {
					if text, ok := textBlock["text"].(string); ok && text != "" {
						// Create streaming response format
						response := &models.CreateCompletionResponse{
							Response: text,
							IsChunk:  true,
							IsLast:   false,
						}

						responseJSON, err := json.Marshal(response)
						if err != nil {
							return err
						}

						// Write to response
						if _, err := m.responseWriter.Write(append(responseJSON, '\n')); err != nil {
							return err
						}

						// Flush if possible
						if flusher, ok := m.responseWriter.(interface{ Flush() }); ok {
							flusher.Flush()
						}
					}
				}
			}
		}
	}

	return nil
}

// isSessionComplete checks if we've received a Stop hook event recently
func (m *ClaudePTYManager) isSessionComplete() bool {
	lastStop := m.claudeService.GetLastStopEvent(m.workingDir)
	return !lastStop.IsZero() && time.Since(lastStop) < 5*time.Second
}

// cleanup cleans up PTY resources
func (m *ClaudePTYManager) cleanup() {
	logger.Debugf("🧹 Cleaning up PTY-based Claude session")

	if m.pty != nil {
		m.pty.Close()
	}

	if m.cmd != nil && m.cmd.Process != nil {
		// Give Claude a moment to finish gracefully
		time.Sleep(1 * time.Second)

		// Then terminate if still running
		if err := m.cmd.Process.Kill(); err != nil {
			logger.Debugf("⚠️ Process kill error (expected if already exited): %v", err)
		}
	}
}
