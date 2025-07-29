package models

import (
	"time"
)

// ClaudeProjectMetadata represents the metadata for a Claude Code project
type ClaudeProjectMetadata struct {
	Path                                    string               `json:"path"`
	AllowedTools                            []string             `json:"allowedTools"`
	History                                 []ClaudeHistoryEntry `json:"history"`
	McpContextUris                          []string             `json:"mcpContextUris"`
	McpServers                              map[string]any       `json:"mcpServers"`
	EnabledMcpjsonServers                   []string             `json:"enabledMcpjsonServers"`
	DisabledMcpjsonServers                  []string             `json:"disabledMcpjsonServers"`
	HasTrustDialogAccepted                  bool                 `json:"hasTrustDialogAccepted"`
	ProjectOnboardingSeenCount              int                  `json:"projectOnboardingSeenCount"`
	HasClaudeMdExternalIncludesApproved     bool                 `json:"hasClaudeMdExternalIncludesApproved"`
	HasClaudeMdExternalIncludesWarningShown bool                 `json:"hasClaudeMdExternalIncludesWarningShown"`

	// Session metrics (only present for completed sessions)
	LastCost                          *float64 `json:"lastCost,omitempty"`
	LastAPIDuration                   *int     `json:"lastAPIDuration,omitempty"`
	LastDuration                      *int     `json:"lastDuration,omitempty"`
	LastLinesAdded                    *int     `json:"lastLinesAdded,omitempty"`
	LastLinesRemoved                  *int     `json:"lastLinesRemoved,omitempty"`
	LastTotalInputTokens              *int     `json:"lastTotalInputTokens,omitempty"`
	LastTotalOutputTokens             *int     `json:"lastTotalOutputTokens,omitempty"`
	LastTotalCacheCreationInputTokens *int     `json:"lastTotalCacheCreationInputTokens,omitempty"`
	LastTotalCacheReadInputTokens     *int     `json:"lastTotalCacheReadInputTokens,omitempty"`
	LastTotalWebSearchRequests        *int     `json:"lastTotalWebSearchRequests,omitempty"`
	LastSessionId                     *string  `json:"lastSessionId,omitempty"`

	// Computed fields
	SessionStartTime *time.Time `json:"sessionStartTime,omitempty"`
	SessionEndTime   *time.Time `json:"sessionEndTime,omitempty"`
	TurnCount        int        `json:"turnCount"`
	IsActiveSession  bool       `json:"isActiveSession"`
}

// ClaudeHistoryEntry represents an entry in the Claude history
type ClaudeHistoryEntry struct {
	Display        string         `json:"display"`
	PastedContents map[string]any `json:"pastedContents"`
}

// ClaudeSessionMessage represents a message in a Claude session file
type ClaudeSessionMessage struct {
	Cwd         string         `json:"cwd"`
	IsMeta      bool           `json:"isMeta"`
	IsSidechain bool           `json:"isSidechain"`
	Message     map[string]any `json:"message"`
	ParentUuid  string         `json:"parentUuid"`
	SessionId   string         `json:"sessionId"`
	Timestamp   string         `json:"timestamp"`
	Type        string         `json:"type"`
	UserType    string         `json:"userType"`
	Uuid        string         `json:"uuid"`
	Version     string         `json:"version"`
}

// ClaudeSessionSummary represents aggregated session information
// @Description Claude Code session summary with metrics and timing information
type ClaudeSessionSummary struct {
	// Path to the worktree directory
	WorktreePath string `json:"worktreePath" example:"/workspace/my-project"`
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
	AllSessions []SessionListEntry `json:"allSessions,omitempty"`
	// Header/title of the session from the Claude history
	Header *string `json:"header,omitempty" example:"Fix bug in user authentication"`

	// Metrics (from completed sessions)
	// Cost in USD of the last completed session
	LastCost *float64 `json:"lastCost,omitempty" example:"0.25"`
	// Duration in seconds of the last session
	LastDuration *int `json:"lastDuration,omitempty" example:"3600"`
	// Total input tokens used in the last session
	LastTotalInputTokens *int `json:"lastTotalInputTokens,omitempty" example:"15000"`
	// Total output tokens generated in the last session
	LastTotalOutputTokens *int `json:"lastTotalOutputTokens,omitempty" example:"8500"`
}

// SessionListEntry represents a single session in a list with basic metadata
// @Description Session list entry with basic metadata
type SessionListEntry struct {
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

// FullSessionData represents complete session data including all messages
// @Description Complete session data with all messages and metadata
type FullSessionData struct {
	// Basic session information
	SessionInfo *ClaudeSessionSummary `json:"sessionInfo"`
	// All sessions available for this workspace
	AllSessions []SessionListEntry `json:"allSessions"`
	// Full conversation history (only when full=true)
	Messages []ClaudeSessionMessage `json:"messages,omitempty"`
	// User prompts from ~/.claude.json (only when full=true)
	UserPrompts []ClaudeHistoryEntry `json:"userPrompts,omitempty"`
	// Total message count in full data
	MessageCount int `json:"messageCount,omitempty"`
}

// CreateCompletionRequest represents a request to create a completion using claude CLI
// @Description Request payload for Claude Code completion using claude CLI subprocess
type CreateCompletionRequest struct {
	// The prompt/message to send to claude
	Prompt string `json:"prompt" example:"Help me debug this error"`
	// Whether to stream the response
	Stream bool `json:"stream,omitempty" example:"true"`
	// Optional system prompt override
	SystemPrompt string `json:"system_prompt,omitempty" example:"You are a helpful coding assistant"`
	// Optional model override
	Model string `json:"model,omitempty" example:"claude-3-5-sonnet-20241022"`
	// Maximum number of turns in the conversation
	MaxTurns int `json:"max_turns,omitempty" example:"10"`
	// Working directory for the claude command
	WorkingDirectory string `json:"working_directory,omitempty" example:"/workspace/my-project"`
	// Whether to resume the most recent session for this working directory
	Resume bool `json:"resume,omitempty" example:"true"`
}

// CreateCompletionResponse represents a response from claude CLI completion
// @Description Response from Claude Code completion using claude CLI subprocess
type CreateCompletionResponse struct {
	// The generated response text
	Response string `json:"response" example:"I can help you debug that error..."`
	// Whether this is a streaming chunk or complete response
	IsChunk bool `json:"is_chunk,omitempty" example:"false"`
	// Whether this is the last chunk in a stream
	IsLast bool `json:"is_last,omitempty" example:"true"`
	// Any error that occurred
	Error string `json:"error,omitempty"`
}

// Todo represents a single todo item from the TodoWrite tool
// @Description A todo item with status and priority tracking
type Todo struct {
	// Unique identifier for the todo item
	ID string `json:"id" example:"1"`
	// The content/description of the todo
	Content string `json:"content" example:"Fix authentication bug"`
	// Current status of the todo
	Status string `json:"status" example:"in_progress" enums:"pending,in_progress,completed"`
	// Priority level of the todo
	Priority string `json:"priority" example:"high" enums:"high,medium,low"`
}
