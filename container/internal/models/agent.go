package models

import (
	"context"
	"io"
	"time"
)

// AgentType represents the type of coding agent
type AgentType string

// Supported agent types
const (
	AgentTypeClaude AgentType = "claude"
	AgentTypeCodex  AgentType = "codex"
)

// Agent represents a coding agent (Claude, Codex, etc.)
type Agent interface {
	// Core agent operations
	GetType() AgentType
	GetName() string

	// Session management
	GetWorktreeSessionSummary(worktreePath string) (*AgentSessionSummary, error)
	GetAllWorktreeSessionSummaries() (map[string]*AgentSessionSummary, error)
	GetFullSessionData(worktreePath string, includeFullData bool) (*AgentFullSessionData, error)
	GetSessionByUUID(sessionUUID string) (*AgentFullSessionData, error)

	// Content retrieval
	GetLatestTodos(worktreePath string) ([]Todo, error)
	GetLatestAssistantMessage(worktreePath string) (string, error)
	GetLatestAssistantMessageOrError(worktreePath string) (content string, isError bool, err error)

	// Completion creation
	CreateCompletion(ctx context.Context, req *AgentCompletionRequest) (*AgentCompletionResponse, error)
	CreateStreamingCompletion(ctx context.Context, req *AgentCompletionRequest, responseWriter io.Writer) error

	// Settings management
	GetSettings() (*AgentSettings, error)
	UpdateSettings(req *AgentSettingsUpdateRequest) (*AgentSettings, error)

	// Activity tracking
	UpdateActivity(worktreePath string)
	GetLastActivity(worktreePath string) time.Time
	IsActiveSession(worktreePath string, within time.Duration) bool

	// Event handling
	HandleEvent(event *AgentEvent) error

	// Lifecycle management
	Start() error
	Stop()
	CleanupWorktreeFiles(worktreePath string) error
}

// AgentMonitor handles file watching and event extraction for an agent
type AgentMonitor interface {
	// Monitoring lifecycle
	Start() error
	Stop()

	// Event handling
	OnWorktreeCreated(worktreeID, worktreePath string)
	OnWorktreeDeleted(worktreeID, worktreePath string)

	// Activity tracking
	GetLastActivityTime(worktreePath string) time.Time
	GetTodos(worktreePath string) ([]Todo, error)
	GetActivityState(worktreePath string) ClaudeActivityState

	// Manual operations
	TriggerBranchRename(workDir string, customBranchName string) error
	RefreshTodoMonitoring()
}

// AgentSessionSummary represents session information for any agent
type AgentSessionSummary struct {
	// Path to the worktree directory
	WorktreePath string `json:"worktreePath" example:"/workspace/my-project"`
	// Agent type
	AgentType AgentType `json:"agentType" example:"claude"`
	// When the current session started
	SessionStartTime *time.Time `json:"sessionStartTime" example:"2024-01-15T14:30:00Z"`
	// When the last session ended (if not active)
	SessionEndTime *time.Time `json:"sessionEndTime" example:"2024-01-15T16:45:30Z"`
	// Number of conversation turns in the session
	TurnCount int `json:"turnCount" example:"15"`
	// Whether this session is currently active
	IsActive bool `json:"isActive" example:"true"`
	// ID of the most recent completed session
	LastSessionId *string `json:"lastSessionId" example:"abc123-def456"`
	// ID of the currently active session
	CurrentSessionId *string `json:"currentSessionId,omitempty" example:"xyz789-ghi012"`
	// List of all available sessions for this worktree
	AllSessions []AgentSessionListEntry `json:"allSessions,omitempty"`
	// Header/title of the session
	Header *string `json:"header,omitempty" example:"Fix bug in user authentication"`

	// Metrics (from completed sessions)
	// Cost in USD of the last completed session (agent-specific)
	LastCost *float64 `json:"lastCost,omitempty" example:"0.25"`
	// Duration in seconds of the last session
	LastDuration *int `json:"lastDuration,omitempty" example:"3600"`
	// Total input tokens used in the last session (agent-specific)
	LastTotalInputTokens *int `json:"lastTotalInputTokens,omitempty" example:"15000"`
	// Total output tokens generated in the last session (agent-specific)
	LastTotalOutputTokens *int `json:"lastTotalOutputTokens,omitempty" example:"8500"`
}

// AgentSessionListEntry represents a single session in a list with basic metadata
type AgentSessionListEntry struct {
	// Unique session identifier
	SessionId string `json:"sessionId" example:"abc123-def456-ghi789"`
	// When the session was last modified
	LastModified time.Time `json:"lastModified" example:"2024-01-15T16:45:30Z"`
	// When the session started (if available)
	StartTime *time.Time `json:"startTime,omitempty" example:"2024-01-15T14:30:00Z"`
	// When the session ended (if available)
	EndTime *time.Time `json:"endTime,omitempty" example:"2024-01-15T16:45:30Z"`
	// Whether this session is currently active
	IsActive bool `json:"isActive" example:"false"`
}

// AgentFullSessionData represents complete session data including all messages
type AgentFullSessionData struct {
	// Basic session information
	SessionInfo *AgentSessionSummary `json:"sessionInfo"`
	// All sessions available for this workspace
	AllSessions []AgentSessionListEntry `json:"allSessions"`
	// Full conversation history (only when full=true)
	Messages []AgentSessionMessage `json:"messages,omitempty"`
	// User prompts/history (agent-specific format)
	UserPrompts []AgentHistoryEntry `json:"userPrompts,omitempty"`
	// Total message count in full data
	MessageCount int `json:"messageCount,omitempty"`
}

// AgentSessionMessage represents a message in any agent's session format
type AgentSessionMessage struct {
	// Agent-specific message data
	AgentType AgentType              `json:"agentType"`
	Timestamp string                 `json:"timestamp"`
	Type      string                 `json:"type"`
	Content   map[string]interface{} `json:"content"` // Raw agent-specific content
}

// AgentHistoryEntry represents a history entry for any agent
type AgentHistoryEntry struct {
	Display string                 `json:"display"`
	Data    map[string]interface{} `json:"data"` // Agent-specific data
}

// AgentCompletionRequest represents a request to create a completion using any agent
type AgentCompletionRequest struct {
	// The prompt/message to send to the agent
	Prompt string `json:"prompt" example:"Help me debug this error"`
	// Whether to stream the response
	Stream bool `json:"stream,omitempty" example:"true"`
	// Optional system prompt override
	SystemPrompt string `json:"system_prompt,omitempty" example:"You are a helpful coding assistant"`
	// Optional model override (agent-specific)
	Model string `json:"model,omitempty" example:"claude-3-5-sonnet-20241022"`
	// Maximum number of turns in the conversation
	MaxTurns int `json:"max_turns,omitempty" example:"10"`
	// Working directory for the command
	WorkingDirectory string `json:"working_directory,omitempty" example:"/workspace/my-project"`
	// Whether to resume the most recent session for this working directory
	Resume bool `json:"resume,omitempty" example:"true"`
	// Whether to suppress events for this automated operation
	SuppressEvents bool `json:"suppress_events,omitempty" example:"true"`
	// Agent-specific options
	AgentOptions map[string]interface{} `json:"agent_options,omitempty"`
}

// AgentCompletionResponse represents a response from any agent
type AgentCompletionResponse struct {
	// The generated response text
	Response string `json:"response" example:"I can help you debug that error..."`
	// Whether this is a streaming chunk or complete response
	IsChunk bool `json:"is_chunk,omitempty" example:"false"`
	// Whether this is the last chunk in a stream
	IsLast bool `json:"is_last,omitempty" example:"true"`
	// Any error that occurred
	Error string `json:"error,omitempty"`
	// Agent that generated this response
	AgentType AgentType `json:"agent_type"`
}

// AgentSettings represents configuration settings for any agent
type AgentSettings struct {
	// Agent type
	AgentType AgentType `json:"agentType"`
	// Whether user is authenticated
	IsAuthenticated bool `json:"isAuthenticated" example:"true"`
	// Version information
	Version string `json:"version,omitempty" example:"1.2.3"`
	// Whether user has completed onboarding
	HasCompletedOnboarding bool `json:"hasCompletedOnboarding" example:"true"`
	// Number of times agent has been started
	NumStartups int `json:"numStartups" example:"15"`
	// Whether notifications are enabled
	NotificationsEnabled bool `json:"notificationsEnabled" example:"true"`
	// Agent-specific settings
	AgentSpecificSettings map[string]interface{} `json:"agentSpecificSettings,omitempty"`
}

// AgentSettingsUpdateRequest represents a request to update agent settings
type AgentSettingsUpdateRequest struct {
	// Whether notifications should be enabled
	NotificationsEnabled *bool `json:"notificationsEnabled,omitempty" example:"true"`
	// Agent-specific settings updates
	AgentSpecificSettings map[string]interface{} `json:"agentSpecificSettings,omitempty"`
}

// AgentEvent represents an event from any agent
type AgentEvent struct {
	// Type of the event (UserPromptSubmit, Stop, etc.)
	EventType string `json:"event_type" example:"UserPromptSubmit"`
	// Working directory where the event occurred
	WorkingDirectory string `json:"working_directory" example:"/workspace/my-project"`
	// Session ID if available
	SessionID string `json:"session_id,omitempty" example:"abc123-def456-ghi789"`
	// Agent type that generated this event
	AgentType AgentType `json:"agent_type"`
	// Additional event-specific data
	Data map[string]interface{} `json:"data,omitempty"`
	// Timestamp of the event
	Timestamp time.Time `json:"timestamp"`
}
