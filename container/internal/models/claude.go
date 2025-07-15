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

// CompletionRequest represents a request to the Anthropic API
// @Description Request payload for Claude completion API
type CompletionRequest struct {
	// The message to send to Claude
	Message string `json:"message" example:"Hello, how are you?"`
	// Maximum number of tokens to generate
	MaxTokens int `json:"max_tokens" example:"1024"`
	// Model to use for completion
	Model string `json:"model" example:"claude-3-5-sonnet-20241022"`
	// Optional system prompt
	System string `json:"system,omitempty" example:"You are a helpful assistant."`
	// Optional conversation context
	Context []CompletionMessage `json:"context,omitempty"`
}

// CompletionMessage represents a message in conversation context
// @Description A message in the conversation context
type CompletionMessage struct {
	// Role of the message sender
	Role string `json:"role" example:"user"`
	// Content of the message
	Content string `json:"content" example:"What is the weather like?"`
}

// CompletionResponse represents the response from the Anthropic API
// @Description Response from Claude completion API
type CompletionResponse struct {
	// Generated response text
	Response string `json:"response" example:"I'm doing well, thank you for asking!"`
	// Number of tokens used in the request
	Usage CompletionUsage `json:"usage"`
	// Model used for the completion
	Model string `json:"model" example:"claude-3-5-sonnet-20241022"`
	// Whether the response was truncated
	Truncated bool `json:"truncated"`
}

// CompletionUsage represents token usage information
// @Description Token usage information for completion
type CompletionUsage struct {
	// Tokens used in the input
	InputTokens int `json:"input_tokens" example:"15"`
	// Tokens generated in the output
	OutputTokens int `json:"output_tokens" example:"25"`
	// Total tokens used
	TotalTokens int `json:"total_tokens" example:"40"`
}

// AnthropicAPIMessage represents a message in the Anthropic API format
type AnthropicAPIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicAPIRequest represents the request format for Anthropic API
type AnthropicAPIRequest struct {
	Model     string                `json:"model"`
	MaxTokens int                   `json:"max_tokens"`
	Messages  []AnthropicAPIMessage `json:"messages"`
	System    string                `json:"system,omitempty"`
}

// AnthropicAPIResponse represents the response format from Anthropic API
type AnthropicAPIResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model string `json:"model"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// AnthropicAPIError represents an error response from Anthropic API
type AnthropicAPIError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
