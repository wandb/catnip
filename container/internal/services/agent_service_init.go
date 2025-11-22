package services

import (
	"fmt"

	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
)

// InitializeAgentServices initializes all agent services and returns the agent manager
func InitializeAgentServices(stateManager *WorktreeStateManager, gitService *GitService) (*AgentManager, error) {
	logger.Info("🤖 Initializing multi-agent services")

	// Create agent manager
	agentManager := NewAgentManager(stateManager, gitService)

	// Initialize Claude agent (existing functionality)
	if err := initializeClaudeAgent(agentManager, stateManager, gitService); err != nil {
		return nil, fmt.Errorf("failed to initialize Claude agent: %w", err)
	}

	// Initialize Codex agent (new functionality)
	if err := initializeCodexAgent(agentManager, stateManager, gitService); err != nil {
		logger.Warnf("⚠️ Failed to initialize Codex agent: %v (continuing with Claude only)", err)
		// Don't fail completely if Codex isn't available
	}

	// Start the agent manager
	if err := agentManager.Start(); err != nil {
		return nil, fmt.Errorf("failed to start agent manager: %w", err)
	}

	logger.Infof("✅ Agent services initialized with %d agents", len(agentManager.GetAvailableAgents()))
	return agentManager, nil
}

// initializeClaudeAgent initializes the Claude agent and monitor
func initializeClaudeAgent(agentManager *AgentManager, stateManager *WorktreeStateManager, gitService *GitService) error {
	logger.Info("🤖 Initializing Claude agent")

	// Create Claude service (existing)
	claudeService := NewClaudeService()

	// Create Claude monitor service (existing)
	claudeMonitor := NewClaudeMonitorService(gitService, NewSessionService(), claudeService, stateManager)

	// Create Claude agent adapter
	claudeAgent := NewClaudeAgent(claudeService)

	// Create Claude monitor adapter
	claudeMonitorAdapter := NewClaudeMonitorAdapter(claudeMonitor)

	// Register with agent manager
	if err := agentManager.RegisterAgent(claudeAgent, claudeMonitorAdapter); err != nil {
		return fmt.Errorf("failed to register Claude agent: %w", err)
	}

	logger.Info("✅ Claude agent initialized")
	return nil
}

// initializeCodexAgent initializes the Codex agent and monitor
func initializeCodexAgent(agentManager *AgentManager, stateManager *WorktreeStateManager, gitService *GitService) error {
	logger.Info("🤖 Initializing Codex agent")

	// Check if Codex CLI is available
	if !isCodexAvailable() {
		return fmt.Errorf("codex CLI not found in PATH")
	}

	// Create Codex agent
	codexAgent := NewCodexAgent()

	// Create Codex monitor
	codexMonitor := NewCodexMonitor(codexAgent, stateManager, gitService)

	// Set up event forwarding from monitor to agent
	codexMonitor.AddEventHandler(func(event *models.AgentEvent) error {
		return codexAgent.HandleEvent(event)
	})

	// Register with agent manager
	if err := agentManager.RegisterAgent(codexAgent, codexMonitor); err != nil {
		return fmt.Errorf("failed to register Codex agent: %w", err)
	}

	logger.Info("✅ Codex agent initialized")
	return nil
}

// isCodexAvailable checks if the Codex CLI is available
func isCodexAvailable() bool {
	// Try to run codex --version to check if it's available
	// This is a simple check - could be enhanced
	return true // For now, assume it's available
}

// GetAgentManagerInstance returns a singleton instance of the agent manager
// This is used for backward compatibility where individual services are expected
var globalAgentManager *AgentManager

// InitializeGlobalAgentServices initializes the global agent manager instance
func InitializeGlobalAgentServices(stateManager *WorktreeStateManager, gitService *GitService) error {
	var err error
	globalAgentManager, err = InitializeAgentServices(stateManager, gitService)
	return err
}

// GetGlobalAgentManager returns the global agent manager instance
func GetGlobalAgentManager() *AgentManager {
	return globalAgentManager
}

// GetClaudeServiceFromAgentManager extracts the Claude service from the agent manager for backward compatibility
func GetClaudeServiceFromAgentManager(agentManager *AgentManager) (*ClaudeService, error) {
	claudeAgent, err := agentManager.GetAgent(models.AgentTypeClaude)
	if err != nil {
		return nil, err
	}

	claudeAgentImpl, ok := claudeAgent.(*ClaudeAgent)
	if !ok {
		return nil, fmt.Errorf("claude agent is not of expected type")
	}

	return claudeAgentImpl.service, nil
}

// GetClaudeMonitorFromAgentManager extracts the Claude monitor from the agent manager for backward compatibility
func GetClaudeMonitorFromAgentManager(agentManager *AgentManager) (*ClaudeMonitorService, error) {
	claudeMonitor, err := agentManager.GetMonitor(models.AgentTypeClaude)
	if err != nil {
		return nil, err
	}

	claudeMonitorAdapter, ok := claudeMonitor.(*ClaudeMonitorAdapter)
	if !ok {
		return nil, fmt.Errorf("claude monitor is not of expected type")
	}

	return claudeMonitorAdapter.monitor, nil
}

// ShutdownAgentServices shuts down all agent services
func ShutdownAgentServices(agentManager *AgentManager) {
	logger.Info("🛑 Shutting down agent services")
	if agentManager != nil {
		agentManager.Stop()
	}
}
