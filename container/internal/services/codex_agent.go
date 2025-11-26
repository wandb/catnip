package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
)

// CodexAgent implements the Agent interface for OpenAI Codex CLI
type CodexAgent struct {
	codexPath         string
	codexSessionsDir  string
	codexHistoryPath  string
	activityMutex     sync.RWMutex
	lastActivity      map[string]time.Time
	eventHandlers     []func(*models.AgentEvent) error
	eventHandlerMutex sync.RWMutex
}

// CodexSessionMeta represents Codex session metadata
type CodexSessionMeta struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Cwd          string    `json:"cwd"`
	Originator   string    `json:"originator"`
	CLIVersion   string    `json:"cli_version"`
	Instructions string    `json:"instructions"`
	Git          struct {
		CommitHash    string `json:"commit_hash"`
		Branch        string `json:"branch"`
		RepositoryURL string `json:"repository_url"`
	} `json:"git"`
}

// CodexMessage represents a Codex message
type CodexMessage struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type    string `json:"type"`
		Role    string `json:"role,omitempty"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content,omitempty"`
		ID       string            `json:"id,omitempty"`
		CLI      string            `json:"cli_version,omitempty"`
		MetaData *CodexSessionMeta `json:",omitempty"` // For session_meta type
	} `json:"payload"`
}

// CodexHistoryEntry represents an entry in the Codex history file
type CodexHistoryEntry struct {
	SessionID string `json:"session_id"`
	Timestamp int64  `json:"ts"`
	Text      string `json:"text"`
}

// NewCodexAgent creates a new Codex agent
func NewCodexAgent() *CodexAgent {
	homeDir := config.Runtime.HomeDir
	return &CodexAgent{
		codexPath:        "codex", // Assume it's in PATH
		codexSessionsDir: filepath.Join(homeDir, ".codex", "sessions"),
		codexHistoryPath: filepath.Join(homeDir, ".codex", "history.jsonl"),
		lastActivity:     make(map[string]time.Time),
	}
}

// GetType returns the agent type
func (ca *CodexAgent) GetType() models.AgentType {
	return models.AgentTypeCodex
}

// GetName returns the agent name
func (ca *CodexAgent) GetName() string {
	return "OpenAI Codex CLI"
}

// GetWorktreeSessionSummary gets Codex session summary for a worktree
func (ca *CodexAgent) GetWorktreeSessionSummary(worktreePath string) (*models.AgentSessionSummary, error) {
	// Find the most recent session for this worktree
	sessionFile, sessionMeta, err := ca.findLatestSessionForWorktree(worktreePath)
	if err != nil {
		return nil, err
	}

	if sessionFile == "" {
		return nil, nil // No session found
	}

	// Parse session file to get message count and timing
	messageCount, startTime, endTime, err := ca.parseSessionMetrics(sessionFile)
	if err != nil {
		return nil, err
	}

	// Get latest title from session
	title, err := ca.getSessionTitle(sessionFile)
	if err != nil {
		title = ""
	}

	summary := &models.AgentSessionSummary{
		WorktreePath:     worktreePath,
		AgentType:        models.AgentTypeCodex,
		SessionStartTime: startTime,
		SessionEndTime:   endTime,
		TurnCount:        messageCount,
		IsActive:         endTime == nil, // Active if no end time
		CurrentSessionId: &sessionMeta.ID,
		Header:           &title,
	}

	// Get all sessions for this worktree
	allSessions, err := ca.getAllSessionsForWorktree(worktreePath)
	if err == nil {
		summary.AllSessions = allSessions
	}

	return summary, nil
}

// GetAllWorktreeSessionSummaries gets all Codex session summaries
func (ca *CodexAgent) GetAllWorktreeSessionSummaries() (map[string]*models.AgentSessionSummary, error) {
	summaries := make(map[string]*models.AgentSessionSummary)

	// Walk through all session files and group by worktree
	err := filepath.Walk(ca.codexSessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Parse session to get worktree path
		worktreePath, err := ca.getWorktreePathFromSession(path)
		if err != nil {
			return nil // Skip invalid sessions
		}

		// Only include the latest session for each worktree
		if _, exists := summaries[worktreePath]; !exists {
			summary, err := ca.GetWorktreeSessionSummary(worktreePath)
			if err == nil && summary != nil {
				summaries[worktreePath] = summary
			}
		}

		return nil
	})

	return summaries, err
}

// GetFullSessionData gets complete Codex session data
func (ca *CodexAgent) GetFullSessionData(worktreePath string, includeFullData bool) (*models.AgentFullSessionData, error) {
	summary, err := ca.GetWorktreeSessionSummary(worktreePath)
	if err != nil {
		return nil, err
	}

	if summary == nil {
		return nil, nil
	}

	fullData := &models.AgentFullSessionData{
		SessionInfo: summary,
		AllSessions: summary.AllSessions,
	}

	if includeFullData && summary.CurrentSessionId != nil {
		// Get full messages for the session
		sessionFile, err := ca.findSessionFile(*summary.CurrentSessionId)
		if err == nil && sessionFile != "" {
			messages, err := ca.parseSessionMessages(sessionFile)
			if err == nil {
				fullData.Messages = messages
				fullData.MessageCount = len(messages)
			}
		}

		// Get user prompts from history
		userPrompts, err := ca.getUserPrompts(worktreePath)
		if err == nil {
			fullData.UserPrompts = userPrompts
		}
	}

	return fullData, nil
}

// GetSessionByUUID gets Codex session by UUID
func (ca *CodexAgent) GetSessionByUUID(sessionUUID string) (*models.AgentFullSessionData, error) {
	sessionFile, err := ca.findSessionFile(sessionUUID)
	if err != nil {
		return nil, err
	}

	if sessionFile == "" {
		return nil, fmt.Errorf("session not found: %s", sessionUUID)
	}

	// Get worktree path from session
	worktreePath, err := ca.getWorktreePathFromSession(sessionFile)
	if err != nil {
		return nil, err
	}

	return ca.GetFullSessionData(worktreePath, true)
}

// GetLatestTodos gets latest todos from Codex session
func (ca *CodexAgent) GetLatestTodos(worktreePath string) ([]models.Todo, error) {
	sessionFile, _, err := ca.findLatestSessionForWorktree(worktreePath)
	if err != nil || sessionFile == "" {
		return []models.Todo{}, nil
	}

	return ca.extractTodosFromSession(sessionFile)
}

// GetLatestAssistantMessage gets latest assistant message from Codex
func (ca *CodexAgent) GetLatestAssistantMessage(worktreePath string) (string, error) {
	sessionFile, _, err := ca.findLatestSessionForWorktree(worktreePath)
	if err != nil || sessionFile == "" {
		return "", nil
	}

	return ca.getLatestAssistantMessageFromSession(sessionFile)
}

// GetLatestAssistantMessageOrError gets latest assistant message or error from Codex
func (ca *CodexAgent) GetLatestAssistantMessageOrError(worktreePath string) (content string, isError bool, err error) {
	message, err := ca.GetLatestAssistantMessage(worktreePath)
	if err != nil {
		return "", false, err
	}

	// Check if message contains error patterns
	lowerMessage := strings.ToLower(message)
	isError = strings.Contains(lowerMessage, "error") ||
		strings.Contains(lowerMessage, "failed") ||
		strings.Contains(lowerMessage, "unavailable")

	return message, isError, nil
}

// CreateCompletion creates a Codex completion
func (ca *CodexAgent) CreateCompletion(ctx context.Context, req *models.AgentCompletionRequest) (*models.AgentCompletionResponse, error) {
	// Build codex command
	args := []string{}

	// Set working directory
	workingDir := req.WorkingDirectory
	if workingDir == "" {
		workingDir = filepath.Join(config.Runtime.WorkspaceDir, "current")
	} else {
		workingDir = config.Runtime.ResolvePath(workingDir)
	}

	// Add resume flag if requested
	if req.Resume {
		args = append(args, "--resume")
	}

	// Create command
	cmd := exec.CommandContext(ctx, ca.codexPath, args...)
	cmd.Dir = workingDir
	cmd.Env = os.Environ()

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start codex command: %w", err)
	}

	// Send prompt
	go func() {
		defer stdin.Close()
		if _, err := stdin.Write([]byte(req.Prompt)); err != nil {
			logger.Errorf("Failed to write prompt to codex: %v", err)
		}
	}()

	// Read response
	output, err := io.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to read stdout: %w", err)
	}

	// Read stderr
	stderrOutput, _ := io.ReadAll(stderr)

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		return &models.AgentCompletionResponse{
			Error:     string(stderrOutput),
			AgentType: models.AgentTypeCodex,
		}, fmt.Errorf("codex command failed: %s", string(stderrOutput))
	}

	return &models.AgentCompletionResponse{
		Response:  string(output),
		IsChunk:   false,
		IsLast:    true,
		AgentType: models.AgentTypeCodex,
	}, nil
}

// CreateStreamingCompletion creates a streaming Codex completion
func (ca *CodexAgent) CreateStreamingCompletion(ctx context.Context, req *models.AgentCompletionRequest, responseWriter io.Writer) error {
	// For now, use non-streaming and write response at once
	// TODO: Implement true streaming if Codex CLI supports it
	resp, err := ca.CreateCompletion(ctx, req)
	if err != nil {
		return err
	}

	// Write response as a single chunk
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	_, err = responseWriter.Write(append(responseJSON, '\n'))
	return err
}

// GetSettings gets Codex settings (minimal implementation)
func (ca *CodexAgent) GetSettings() (*models.AgentSettings, error) {
	// Codex doesn't have the same settings system as Claude
	// Return basic settings
	return &models.AgentSettings{
		AgentType:              models.AgentTypeCodex,
		IsAuthenticated:        true, // Assume authenticated if codex command works
		Version:                "",   // Could be detected from CLI version
		HasCompletedOnboarding: true,
		NumStartups:            0,
		NotificationsEnabled:   true, // Default to enabled
		AgentSpecificSettings:  map[string]interface{}{},
	}, nil
}

// UpdateSettings updates Codex settings (minimal implementation)
func (ca *CodexAgent) UpdateSettings(req *models.AgentSettingsUpdateRequest) (*models.AgentSettings, error) {
	// Codex doesn't have settings to update, so just return current settings
	return ca.GetSettings()
}

// UpdateActivity updates Codex activity
func (ca *CodexAgent) UpdateActivity(worktreePath string) {
	ca.activityMutex.Lock()
	ca.lastActivity[worktreePath] = time.Now()
	ca.activityMutex.Unlock()
}

// GetLastActivity gets last Codex activity
func (ca *CodexAgent) GetLastActivity(worktreePath string) time.Time {
	ca.activityMutex.RLock()
	defer ca.activityMutex.RUnlock()
	return ca.lastActivity[worktreePath]
}

// IsActiveSession checks if Codex session is active
func (ca *CodexAgent) IsActiveSession(worktreePath string, within time.Duration) bool {
	lastActivity := ca.GetLastActivity(worktreePath)
	if lastActivity.IsZero() {
		return false
	}
	return time.Since(lastActivity) <= within
}

// HandleEvent handles Codex events (no-op since Codex doesn't have hooks)
func (ca *CodexAgent) HandleEvent(event *models.AgentEvent) error {
	// Codex doesn't have native hooks, but we can still handle events
	// that might be generated by our file watchers
	ca.eventHandlerMutex.RLock()
	defer ca.eventHandlerMutex.RUnlock()

	for _, handler := range ca.eventHandlers {
		if err := handler(event); err != nil {
			logger.Warnf("Codex event handler error: %v", err)
		}
	}

	return nil
}

// AddEventHandler adds an event handler for Codex events
func (ca *CodexAgent) AddEventHandler(handler func(*models.AgentEvent) error) {
	ca.eventHandlerMutex.Lock()
	defer ca.eventHandlerMutex.Unlock()
	ca.eventHandlers = append(ca.eventHandlers, handler)
}

// Start starts the Codex agent
func (ca *CodexAgent) Start() error {
	// Ensure directories exist
	if err := os.MkdirAll(ca.codexSessionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create codex sessions directory: %w", err)
	}

	logger.Infof("🚀 Codex agent started")
	return nil
}

// Stop stops the Codex agent
func (ca *CodexAgent) Stop() {
	logger.Infof("🛑 Codex agent stopped")
}

// CleanupWorktreeFiles cleans up Codex files for a worktree
func (ca *CodexAgent) CleanupWorktreeFiles(worktreePath string) error {
	// Remove session files for this worktree
	// This is more complex for Codex since we need to find sessions by worktree path
	var errs []error

	err := filepath.Walk(ca.codexSessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Check if this session belongs to the worktree
		sessionWorktreePath, err := ca.getWorktreePathFromSession(path)
		if err != nil {
			return nil
		}

		if sessionWorktreePath == worktreePath {
			if err := os.Remove(path); err != nil {
				errs = append(errs, err)
			}
		}

		return nil
	})

	if err != nil {
		errs = append(errs, err)
	}

	// Clear activity tracking
	ca.activityMutex.Lock()
	delete(ca.lastActivity, worktreePath)
	ca.activityMutex.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}

	return nil
}

// Helper methods for Codex-specific functionality

// findLatestSessionForWorktree finds the most recent Codex session for a worktree
func (ca *CodexAgent) findLatestSessionForWorktree(worktreePath string) (string, *CodexSessionMeta, error) {
	var latestFile string
	var latestTime time.Time
	var latestMeta *CodexSessionMeta

	err := filepath.Walk(ca.codexSessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Parse session to check if it's for this worktree
		sessionWorktreePath, sessionMeta, err := ca.getWorktreePathAndMetaFromSession(path)
		if err != nil {
			return nil
		}

		if sessionWorktreePath == worktreePath && sessionMeta.Timestamp.After(latestTime) {
			latestFile = path
			latestTime = sessionMeta.Timestamp
			latestMeta = sessionMeta
		}

		return nil
	})

	return latestFile, latestMeta, err
}

// getWorktreePathFromSession extracts the worktree path from a Codex session file
func (ca *CodexAgent) getWorktreePathFromSession(sessionFile string) (string, error) {
	path, _, err := ca.getWorktreePathAndMetaFromSession(sessionFile)
	return path, err
}

// getWorktreePathAndMetaFromSession extracts worktree path and metadata from a session file
func (ca *CodexAgent) getWorktreePathAndMetaFromSession(sessionFile string) (string, *CodexSessionMeta, error) {
	file, err := os.Open(sessionFile)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var msg CodexMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		if msg.Type == "session_meta" && msg.Payload.MetaData != nil {
			return msg.Payload.MetaData.Cwd, msg.Payload.MetaData, nil
		}
	}

	return "", nil, fmt.Errorf("no session metadata found in %s", sessionFile)
}

// Additional helper methods would go here...
// (parseSessionMetrics, getSessionTitle, getAllSessionsForWorktree, etc.)

// parseSessionMetrics parses basic metrics from a Codex session file
func (ca *CodexAgent) parseSessionMetrics(sessionFile string) (messageCount int, startTime *time.Time, endTime *time.Time, err error) {
	file, err := os.Open(sessionFile)
	if err != nil {
		return 0, nil, nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var firstTime, lastTime time.Time

	for scanner.Scan() {
		var msg CodexMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		timestamp, err := time.Parse(time.RFC3339, msg.Timestamp)
		if err != nil {
			continue
		}

		if firstTime.IsZero() {
			firstTime = timestamp
		}
		lastTime = timestamp

		if msg.Type == "response_item" {
			messageCount++
		}
	}

	if !firstTime.IsZero() {
		startTime = &firstTime
	}
	if !lastTime.IsZero() && !lastTime.Equal(firstTime) {
		endTime = &lastTime
	}

	return messageCount, startTime, endTime, nil
}

// Placeholder implementations for other helper methods
func (ca *CodexAgent) getSessionTitle(sessionFile string) (string, error) {
	// TODO: Extract title from session content
	return "Codex Session", nil
}

func (ca *CodexAgent) getAllSessionsForWorktree(worktreePath string) ([]models.AgentSessionListEntry, error) {
	// TODO: Implement session listing
	return []models.AgentSessionListEntry{}, nil
}

func (ca *CodexAgent) findSessionFile(sessionUUID string) (string, error) {
	// TODO: Find session file by UUID
	return "", fmt.Errorf("not implemented")
}

func (ca *CodexAgent) parseSessionMessages(sessionFile string) ([]models.AgentSessionMessage, error) {
	// TODO: Parse all messages from session
	return []models.AgentSessionMessage{}, nil
}

func (ca *CodexAgent) getUserPrompts(worktreePath string) ([]models.AgentHistoryEntry, error) {
	// TODO: Get user prompts from history
	return []models.AgentHistoryEntry{}, nil
}

func (ca *CodexAgent) extractTodosFromSession(sessionFile string) ([]models.Todo, error) {
	// TODO: Extract todos from session content
	return []models.Todo{}, nil
}

func (ca *CodexAgent) getLatestAssistantMessageFromSession(sessionFile string) (string, error) {
	// TODO: Get latest assistant message
	return "", nil
}
