package services

import (
	"context"
	"io"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// ClaudeAgent implements the Agent interface for Claude Code
type ClaudeAgent struct {
	service *ClaudeService
}

// NewClaudeAgent creates a new Claude agent
func NewClaudeAgent(service *ClaudeService) *ClaudeAgent {
	return &ClaudeAgent{
		service: service,
	}
}

// GetType returns the agent type
func (ca *ClaudeAgent) GetType() models.AgentType {
	return models.AgentTypeClaude
}

// GetName returns the agent name
func (ca *ClaudeAgent) GetName() string {
	return "Claude Code"
}

// GetWorktreeSessionSummary gets Claude session summary and converts to agent format
func (ca *ClaudeAgent) GetWorktreeSessionSummary(worktreePath string) (*models.AgentSessionSummary, error) {
	summary, err := ca.service.GetWorktreeSessionSummary(worktreePath)
	if err != nil {
		return nil, err
	}

	if summary == nil {
		return nil, nil
	}

	// Convert Claude summary to agent summary
	agentSummary := &models.AgentSessionSummary{
		WorktreePath:          summary.WorktreePath,
		AgentType:             models.AgentTypeClaude,
		SessionStartTime:      summary.SessionStartTime,
		SessionEndTime:        summary.SessionEndTime,
		TurnCount:             summary.TurnCount,
		IsActive:              summary.IsActive,
		LastSessionId:         summary.LastSessionId,
		CurrentSessionId:      summary.CurrentSessionId,
		Header:                summary.Header,
		LastCost:              summary.LastCost,
		LastDuration:          summary.LastDuration,
		LastTotalInputTokens:  summary.LastTotalInputTokens,
		LastTotalOutputTokens: summary.LastTotalOutputTokens,
	}

	// Convert session list entries
	if summary.AllSessions != nil {
		agentSummary.AllSessions = make([]models.AgentSessionListEntry, len(summary.AllSessions))
		for i, session := range summary.AllSessions {
			agentSummary.AllSessions[i] = models.AgentSessionListEntry(session)
		}
	}

	return agentSummary, nil
}

// GetAllWorktreeSessionSummaries gets all Claude session summaries
func (ca *ClaudeAgent) GetAllWorktreeSessionSummaries() (map[string]*models.AgentSessionSummary, error) {
	summaries, err := ca.service.GetAllWorktreeSessionSummaries()
	if err != nil {
		return nil, err
	}

	agentSummaries := make(map[string]*models.AgentSessionSummary)
	for path, summary := range summaries {
		if summary != nil {
			agentSummary, err := ca.GetWorktreeSessionSummary(path)
			if err == nil && agentSummary != nil {
				agentSummaries[path] = agentSummary
			}
		}
	}

	return agentSummaries, nil
}

// GetFullSessionData gets complete Claude session data
func (ca *ClaudeAgent) GetFullSessionData(worktreePath string, includeFullData bool) (*models.AgentFullSessionData, error) {
	fullData, err := ca.service.GetFullSessionData(worktreePath, includeFullData)
	if err != nil {
		return nil, err
	}

	if fullData == nil {
		return nil, nil
	}

	// Convert to agent format
	agentFullData := &models.AgentFullSessionData{
		MessageCount: fullData.MessageCount,
	}

	// Convert session info
	if fullData.SessionInfo != nil {
		agentSummary, err := ca.GetWorktreeSessionSummary(worktreePath)
		if err == nil {
			agentFullData.SessionInfo = agentSummary
		}
	}

	// Convert all sessions
	if fullData.AllSessions != nil {
		agentFullData.AllSessions = make([]models.AgentSessionListEntry, len(fullData.AllSessions))
		for i, session := range fullData.AllSessions {
			agentFullData.AllSessions[i] = models.AgentSessionListEntry(session)
		}
	}

	// Convert messages
	if fullData.Messages != nil {
		agentFullData.Messages = make([]models.AgentSessionMessage, len(fullData.Messages))
		for i, msg := range fullData.Messages {
			agentFullData.Messages[i] = models.AgentSessionMessage{
				AgentType: models.AgentTypeClaude,
				Timestamp: msg.Timestamp,
				Type:      msg.Type,
				Content:   map[string]interface{}{"message": msg.Message}, // Wrap Claude-specific data
			}
		}
	}

	// Convert user prompts
	if fullData.UserPrompts != nil {
		agentFullData.UserPrompts = make([]models.AgentHistoryEntry, len(fullData.UserPrompts))
		for i, prompt := range fullData.UserPrompts {
			agentFullData.UserPrompts[i] = models.AgentHistoryEntry{
				Display: prompt.Display,
				Data:    map[string]interface{}{"pastedContents": prompt.PastedContents},
			}
		}
	}

	return agentFullData, nil
}

// GetSessionByUUID gets Claude session by UUID
func (ca *ClaudeAgent) GetSessionByUUID(sessionUUID string) (*models.AgentFullSessionData, error) {
	fullData, err := ca.service.GetSessionByUUID(sessionUUID)
	if err != nil {
		return nil, err
	}

	if fullData == nil {
		return nil, nil
	}

	// Convert to agent format (similar to GetFullSessionData)
	agentFullData := &models.AgentFullSessionData{
		MessageCount: fullData.MessageCount,
	}

	// Convert session info
	if fullData.SessionInfo != nil {
		agentSummary := &models.AgentSessionSummary{
			WorktreePath:     fullData.SessionInfo.WorktreePath,
			AgentType:        models.AgentTypeClaude,
			SessionStartTime: fullData.SessionInfo.SessionStartTime,
			SessionEndTime:   fullData.SessionInfo.SessionEndTime,
			IsActive:         fullData.SessionInfo.IsActive,
			CurrentSessionId: fullData.SessionInfo.CurrentSessionId,
		}
		agentFullData.SessionInfo = agentSummary
	}

	// Convert all sessions and messages (similar to above)
	if fullData.AllSessions != nil {
		agentFullData.AllSessions = make([]models.AgentSessionListEntry, len(fullData.AllSessions))
		for i, session := range fullData.AllSessions {
			agentFullData.AllSessions[i] = models.AgentSessionListEntry(session)
		}
	}

	return agentFullData, nil
}

// GetLatestTodos gets latest todos from Claude
func (ca *ClaudeAgent) GetLatestTodos(worktreePath string) ([]models.Todo, error) {
	return ca.service.GetLatestTodos(worktreePath)
}

// GetLatestAssistantMessage gets latest assistant message from Claude
func (ca *ClaudeAgent) GetLatestAssistantMessage(worktreePath string) (string, error) {
	return ca.service.GetLatestAssistantMessage(worktreePath)
}

// GetLatestAssistantMessageOrError gets latest assistant message or error from Claude
func (ca *ClaudeAgent) GetLatestAssistantMessageOrError(worktreePath string) (content string, isError bool, err error) {
	return ca.service.GetLatestAssistantMessageOrError(worktreePath)
}

// CreateCompletion creates a Claude completion
func (ca *ClaudeAgent) CreateCompletion(ctx context.Context, req *models.AgentCompletionRequest) (*models.AgentCompletionResponse, error) {
	// Convert agent request to Claude request
	claudeReq := &models.CreateCompletionRequest{
		Prompt:           req.Prompt,
		Stream:           req.Stream,
		SystemPrompt:     req.SystemPrompt,
		Model:            req.Model,
		MaxTurns:         req.MaxTurns,
		WorkingDirectory: req.WorkingDirectory,
		Resume:           req.Resume,
		SuppressEvents:   req.SuppressEvents,
	}

	resp, err := ca.service.CreateCompletion(ctx, claudeReq)
	if err != nil {
		return nil, err
	}

	// Convert Claude response to agent response
	return &models.AgentCompletionResponse{
		Response:  resp.Response,
		IsChunk:   resp.IsChunk,
		IsLast:    resp.IsLast,
		Error:     resp.Error,
		AgentType: models.AgentTypeClaude,
	}, nil
}

// CreateStreamingCompletion creates a streaming Claude completion
func (ca *ClaudeAgent) CreateStreamingCompletion(ctx context.Context, req *models.AgentCompletionRequest, responseWriter io.Writer) error {
	// Convert agent request to Claude request
	claudeReq := &models.CreateCompletionRequest{
		Prompt:           req.Prompt,
		Stream:           true, // Force streaming
		SystemPrompt:     req.SystemPrompt,
		Model:            req.Model,
		MaxTurns:         req.MaxTurns,
		WorkingDirectory: req.WorkingDirectory,
		Resume:           req.Resume,
		SuppressEvents:   req.SuppressEvents,
	}

	return ca.service.CreateStreamingCompletion(ctx, claudeReq, responseWriter)
}

// GetSettings gets Claude settings
func (ca *ClaudeAgent) GetSettings() (*models.AgentSettings, error) {
	claudeSettings, err := ca.service.GetClaudeSettings()
	if err != nil {
		return nil, err
	}

	// Convert Claude settings to agent settings
	return &models.AgentSettings{
		AgentType:              models.AgentTypeClaude,
		IsAuthenticated:        claudeSettings.IsAuthenticated,
		Version:                claudeSettings.Version,
		HasCompletedOnboarding: claudeSettings.HasCompletedOnboarding,
		NumStartups:            claudeSettings.NumStartups,
		NotificationsEnabled:   claudeSettings.NotificationsEnabled,
		AgentSpecificSettings: map[string]interface{}{
			"theme": claudeSettings.Theme,
		},
	}, nil
}

// UpdateSettings updates Claude settings
func (ca *ClaudeAgent) UpdateSettings(req *models.AgentSettingsUpdateRequest) (*models.AgentSettings, error) {
	// Convert agent request to Claude request
	claudeReq := &models.ClaudeSettingsUpdateRequest{
		NotificationsEnabled: req.NotificationsEnabled,
	}

	// Handle agent-specific settings
	if req.AgentSpecificSettings != nil {
		if theme, ok := req.AgentSpecificSettings["theme"].(string); ok {
			claudeReq.Theme = theme
		}
	}

	claudeSettings, err := ca.service.UpdateClaudeSettings(claudeReq)
	if err != nil {
		return nil, err
	}

	// Convert back to agent settings
	return &models.AgentSettings{
		AgentType:              models.AgentTypeClaude,
		IsAuthenticated:        claudeSettings.IsAuthenticated,
		Version:                claudeSettings.Version,
		HasCompletedOnboarding: claudeSettings.HasCompletedOnboarding,
		NumStartups:            claudeSettings.NumStartups,
		NotificationsEnabled:   claudeSettings.NotificationsEnabled,
		AgentSpecificSettings: map[string]interface{}{
			"theme": claudeSettings.Theme,
		},
	}, nil
}

// UpdateActivity updates Claude activity
func (ca *ClaudeAgent) UpdateActivity(worktreePath string) {
	ca.service.UpdateActivity(worktreePath)
}

// GetLastActivity gets last Claude activity
func (ca *ClaudeAgent) GetLastActivity(worktreePath string) time.Time {
	return ca.service.GetLastActivity(worktreePath)
}

// IsActiveSession checks if Claude session is active
func (ca *ClaudeAgent) IsActiveSession(worktreePath string, within time.Duration) bool {
	return ca.service.IsActiveSession(worktreePath, within)
}

// HandleEvent handles Claude hook events
func (ca *ClaudeAgent) HandleEvent(event *models.AgentEvent) error {
	// Convert agent event to Claude hook event
	claudeEvent := &models.ClaudeHookEvent{
		EventType:        event.EventType,
		WorkingDirectory: event.WorkingDirectory,
		SessionID:        event.SessionID,
		Data:             event.Data,
	}

	return ca.service.HandleHookEvent(claudeEvent)
}

// Start starts the Claude agent
func (ca *ClaudeAgent) Start() error {
	// Claude service doesn't have a start method, so this is a no-op
	return nil
}

// Stop stops the Claude agent
func (ca *ClaudeAgent) Stop() {
	ca.service.Shutdown()
}

// CleanupWorktreeFiles cleans up Claude files for a worktree
func (ca *ClaudeAgent) CleanupWorktreeFiles(worktreePath string) error {
	return ca.service.CleanupWorktreeClaudeFiles(worktreePath)
}

// IsSuppressingEvents checks if events are suppressed for a worktree
func (ca *ClaudeAgent) IsSuppressingEvents(worktreePath string) bool {
	return ca.service.IsSuppressingEvents(worktreePath)
}

// ClaudeMonitorAdapter adapts ClaudeMonitorService to implement AgentMonitor
type ClaudeMonitorAdapter struct {
	monitor *ClaudeMonitorService
}

// NewClaudeMonitorAdapter creates a new Claude monitor adapter
func NewClaudeMonitorAdapter(monitor *ClaudeMonitorService) *ClaudeMonitorAdapter {
	return &ClaudeMonitorAdapter{
		monitor: monitor,
	}
}

// Start starts the Claude monitor
func (cma *ClaudeMonitorAdapter) Start() error {
	return cma.monitor.Start()
}

// Stop stops the Claude monitor
func (cma *ClaudeMonitorAdapter) Stop() {
	cma.monitor.Stop()
}

// OnWorktreeCreated handles worktree creation
func (cma *ClaudeMonitorAdapter) OnWorktreeCreated(worktreeID, worktreePath string) {
	cma.monitor.OnWorktreeCreated(worktreeID, worktreePath)
}

// OnWorktreeDeleted handles worktree deletion
func (cma *ClaudeMonitorAdapter) OnWorktreeDeleted(worktreeID, worktreePath string) {
	cma.monitor.OnWorktreeDeleted(worktreeID, worktreePath)
}

// GetLastActivityTime gets last activity time
func (cma *ClaudeMonitorAdapter) GetLastActivityTime(worktreePath string) time.Time {
	return cma.monitor.GetLastActivityTime(worktreePath)
}

// GetTodos gets todos for a worktree
func (cma *ClaudeMonitorAdapter) GetTodos(worktreePath string) ([]models.Todo, error) {
	return cma.monitor.GetTodos(worktreePath)
}

// GetActivityState gets Claude activity state
func (cma *ClaudeMonitorAdapter) GetActivityState(worktreePath string) models.ClaudeActivityState {
	return cma.monitor.GetClaudeActivityState(worktreePath)
}

// TriggerBranchRename triggers branch renaming
func (cma *ClaudeMonitorAdapter) TriggerBranchRename(workDir string, customBranchName string) error {
	return cma.monitor.TriggerBranchRename(workDir, customBranchName)
}

// RefreshTodoMonitoring refreshes todo monitoring
func (cma *ClaudeMonitorAdapter) RefreshTodoMonitoring() {
	cma.monitor.RefreshTodoMonitoring()
}
