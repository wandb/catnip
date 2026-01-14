package services

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// AgentManager manages multiple coding agents and provides a unified interface
type AgentManager struct {
	agents       map[models.AgentType]models.Agent
	monitors     map[models.AgentType]models.AgentMonitor
	defaultAgent models.AgentType
	agentMutex   sync.RWMutex
	stateManager *WorktreeStateManager
	gitService   *GitService
}

// NewAgentManager creates a new agent manager
func NewAgentManager(stateManager *WorktreeStateManager, gitService *GitService) *AgentManager {
	return &AgentManager{
		agents:       make(map[models.AgentType]models.Agent),
		monitors:     make(map[models.AgentType]models.AgentMonitor),
		defaultAgent: models.AgentTypeClaude, // Default to Claude for backward compatibility
		stateManager: stateManager,
		gitService:   gitService,
	}
}

// RegisterAgent registers an agent with the manager
func (am *AgentManager) RegisterAgent(agent models.Agent, monitor models.AgentMonitor) error {
	am.agentMutex.Lock()
	defer am.agentMutex.Unlock()

	agentType := agent.GetType()
	am.agents[agentType] = agent
	if monitor != nil {
		am.monitors[agentType] = monitor
	}

	return nil
}

// SetDefaultAgent sets the default agent type
func (am *AgentManager) SetDefaultAgent(agentType models.AgentType) error {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	if _, exists := am.agents[agentType]; !exists {
		return fmt.Errorf("agent type %s not registered", agentType)
	}

	am.defaultAgent = agentType
	return nil
}

// GetAgent gets an agent by type, falling back to default
func (am *AgentManager) GetAgent(agentType models.AgentType) (models.Agent, error) {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	if agentType == "" {
		agentType = am.defaultAgent
	}

	agent, exists := am.agents[agentType]
	if !exists {
		return nil, fmt.Errorf("agent type %s not registered", agentType)
	}

	return agent, nil
}

// GetMonitor gets a monitor by agent type
func (am *AgentManager) GetMonitor(agentType models.AgentType) (models.AgentMonitor, error) {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	if agentType == "" {
		agentType = am.defaultAgent
	}

	monitor, exists := am.monitors[agentType]
	if !exists {
		return nil, fmt.Errorf("monitor for agent type %s not registered", agentType)
	}

	return monitor, nil
}

// GetWorktreeAgent determines which agent to use for a given worktree
// This could be enhanced with per-worktree agent configuration
func (am *AgentManager) GetWorktreeAgent(worktreePath string) (models.Agent, error) {
	// For now, use default agent
	// TODO: Add per-worktree agent configuration
	return am.GetAgent(am.defaultAgent)
}

// GetWorktreeMonitor gets the monitor for a given worktree
func (am *AgentManager) GetWorktreeMonitor(worktreePath string) (models.AgentMonitor, error) {
	// For now, use default agent's monitor
	// TODO: Add per-worktree agent configuration
	return am.GetMonitor(am.defaultAgent)
}

// Start starts all registered agents and monitors
func (am *AgentManager) Start() error {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	// Start all agents
	for agentType, agent := range am.agents {
		if err := agent.Start(); err != nil {
			return fmt.Errorf("failed to start agent %s: %w", agentType, err)
		}
	}

	// Start all monitors
	for agentType, monitor := range am.monitors {
		if err := monitor.Start(); err != nil {
			return fmt.Errorf("failed to start monitor for agent %s: %w", agentType, err)
		}
	}

	return nil
}

// Stop stops all agents and monitors
func (am *AgentManager) Stop() {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	// Stop all monitors first
	for _, monitor := range am.monitors {
		monitor.Stop()
	}

	// Stop all agents
	for _, agent := range am.agents {
		agent.Stop()
	}
}

// Unified API methods that delegate to the appropriate agent

// GetWorktreeSessionSummary gets session summary for a worktree using the appropriate agent
func (am *AgentManager) GetWorktreeSessionSummary(worktreePath string) (*models.AgentSessionSummary, error) {
	agent, err := am.GetWorktreeAgent(worktreePath)
	if err != nil {
		return nil, err
	}
	return agent.GetWorktreeSessionSummary(worktreePath)
}

// GetAllWorktreeSessionSummaries gets session summaries for all worktrees
func (am *AgentManager) GetAllWorktreeSessionSummaries() (map[string]*models.AgentSessionSummary, error) {
	// For now, use default agent to get all summaries
	// TODO: Could aggregate from all agents
	agent, err := am.GetAgent(am.defaultAgent)
	if err != nil {
		return nil, err
	}
	return agent.GetAllWorktreeSessionSummaries()
}

// GetFullSessionData gets complete session data for a worktree
func (am *AgentManager) GetFullSessionData(worktreePath string, includeFullData bool) (*models.AgentFullSessionData, error) {
	agent, err := am.GetWorktreeAgent(worktreePath)
	if err != nil {
		return nil, err
	}
	return agent.GetFullSessionData(worktreePath, includeFullData)
}

// GetSessionByUUID gets session data by UUID (searches all agents)
func (am *AgentManager) GetSessionByUUID(sessionUUID string) (*models.AgentFullSessionData, error) {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	// Try each agent until we find the session
	for _, agent := range am.agents {
		data, err := agent.GetSessionByUUID(sessionUUID)
		if err == nil && data != nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("session not found: %s", sessionUUID)
}

// GetLatestTodos gets latest todos for a worktree
func (am *AgentManager) GetLatestTodos(worktreePath string) ([]models.Todo, error) {
	agent, err := am.GetWorktreeAgent(worktreePath)
	if err != nil {
		return nil, err
	}
	return agent.GetLatestTodos(worktreePath)
}

// GetLatestAssistantMessage gets latest assistant message for a worktree
func (am *AgentManager) GetLatestAssistantMessage(worktreePath string) (string, error) {
	agent, err := am.GetWorktreeAgent(worktreePath)
	if err != nil {
		return "", err
	}
	return agent.GetLatestAssistantMessage(worktreePath)
}

// GetLatestAssistantMessageOrError gets latest assistant message or error for a worktree
func (am *AgentManager) GetLatestAssistantMessageOrError(worktreePath string) (content string, isError bool, err error) {
	agent, err := am.GetWorktreeAgent(worktreePath)
	if err != nil {
		return "", false, err
	}
	return agent.GetLatestAssistantMessageOrError(worktreePath)
}

// CreateCompletion creates a completion using the appropriate agent
func (am *AgentManager) CreateCompletion(ctx context.Context, req *models.AgentCompletionRequest) (*models.AgentCompletionResponse, error) {
	var agentType models.AgentType
	if req.AgentOptions != nil {
		if at, ok := req.AgentOptions["agent_type"].(string); ok {
			agentType = models.AgentType(at)
		}
	}

	agent, err := am.GetAgent(agentType)
	if err != nil {
		return nil, err
	}

	return agent.CreateCompletion(ctx, req)
}

// CreateStreamingCompletion creates a streaming completion using the appropriate agent
func (am *AgentManager) CreateStreamingCompletion(ctx context.Context, req *models.AgentCompletionRequest, responseWriter io.Writer) error {
	var agentType models.AgentType
	if req.AgentOptions != nil {
		if at, ok := req.AgentOptions["agent_type"].(string); ok {
			agentType = models.AgentType(at)
		}
	}

	agent, err := am.GetAgent(agentType)
	if err != nil {
		return err
	}

	return agent.CreateStreamingCompletion(ctx, req, responseWriter)
}

// GetSettings gets settings for the default agent (or specific agent)
func (am *AgentManager) GetSettings(agentType models.AgentType) (*models.AgentSettings, error) {
	agent, err := am.GetAgent(agentType)
	if err != nil {
		return nil, err
	}
	return agent.GetSettings()
}

// UpdateSettings updates settings for the default agent (or specific agent)
func (am *AgentManager) UpdateSettings(agentType models.AgentType, req *models.AgentSettingsUpdateRequest) (*models.AgentSettings, error) {
	agent, err := am.GetAgent(agentType)
	if err != nil {
		return nil, err
	}
	return agent.UpdateSettings(req)
}

// HandleEvent handles an event and routes it to the appropriate agent
func (am *AgentManager) HandleEvent(event *models.AgentEvent) error {
	agent, err := am.GetAgent(event.AgentType)
	if err != nil {
		return err
	}
	return agent.HandleEvent(event)
}

// UpdateActivity updates activity for a worktree using the appropriate agent
func (am *AgentManager) UpdateActivity(worktreePath string) {
	agent, err := am.GetWorktreeAgent(worktreePath)
	if err != nil {
		return // Silently ignore errors for activity updates
	}
	agent.UpdateActivity(worktreePath)
}

// GetLastActivity gets last activity time for a worktree
func (am *AgentManager) GetLastActivity(worktreePath string) time.Time {
	agent, err := am.GetWorktreeAgent(worktreePath)
	if err != nil {
		return time.Time{} // Return zero time on error
	}
	return agent.GetLastActivity(worktreePath)
}

// IsActiveSession checks if a session is active within the specified duration
func (am *AgentManager) IsActiveSession(worktreePath string, within time.Duration) bool {
	agent, err := am.GetWorktreeAgent(worktreePath)
	if err != nil {
		return false
	}
	return agent.IsActiveSession(worktreePath, within)
}

// CleanupWorktreeFiles cleans up agent files for a worktree
func (am *AgentManager) CleanupWorktreeFiles(worktreePath string) error {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	// Clean up files for all agents
	var errs []error
	for _, agent := range am.agents {
		if err := agent.CleanupWorktreeFiles(worktreePath); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}

	return nil
}

// OnWorktreeCreated notifies all monitors about a new worktree
func (am *AgentManager) OnWorktreeCreated(worktreeID, worktreePath string) {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	for _, monitor := range am.monitors {
		monitor.OnWorktreeCreated(worktreeID, worktreePath)
	}
}

// OnWorktreeDeleted notifies all monitors about a deleted worktree
func (am *AgentManager) OnWorktreeDeleted(worktreeID, worktreePath string) {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	for _, monitor := range am.monitors {
		monitor.OnWorktreeDeleted(worktreeID, worktreePath)
	}
}

// GetActivityState gets activity state for a worktree
func (am *AgentManager) GetActivityState(worktreePath string) models.ClaudeActivityState {
	monitor, err := am.GetWorktreeMonitor(worktreePath)
	if err != nil {
		return models.ClaudeInactive
	}
	return monitor.GetActivityState(worktreePath)
}

// TriggerBranchRename triggers branch renaming for a worktree
func (am *AgentManager) TriggerBranchRename(workDir string, customBranchName string) error {
	monitor, err := am.GetWorktreeMonitor(workDir)
	if err != nil {
		return err
	}
	return monitor.TriggerBranchRename(workDir, customBranchName)
}

// GetAvailableAgents returns a list of all registered agent types
func (am *AgentManager) GetAvailableAgents() []models.AgentType {
	am.agentMutex.RLock()
	defer am.agentMutex.RUnlock()

	var agents []models.AgentType
	for agentType := range am.agents {
		agents = append(agents, agentType)
	}
	return agents
}
