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
	Subtype     string         `json:"subtype,omitempty"` // Subtype for system messages (e.g., "compact_boundary")
	Content     string         `json:"content,omitempty"` // Content for system messages
	UserType    string         `json:"userType"`
	Uuid        string         `json:"uuid"`
	Version     string         `json:"version"`
	AgentID     string         `json:"agentId,omitempty"`
	Summary     string         `json:"summary,omitempty"` // Summary text for summary-type messages
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
	// Latest user prompt from history (always populated when available)
	LatestUserPrompt string `json:"latestUserPrompt,omitempty"`
	// Latest assistant message text (always populated when available)
	LatestMessage string `json:"latestMessage,omitempty"`
	// Latest thinking/reasoning content (always populated when available)
	LatestThought string `json:"latestThought,omitempty"`
	// Session statistics (token counts, tool usage, etc.)
	Stats *SessionStats `json:"stats,omitempty"`
	// Current todo items from the session
	Todos []Todo `json:"todos,omitempty"`
	// Latest session title (from PTY escape sequences or session history)
	LatestSessionTitle string `json:"latestSessionTitle,omitempty"`
}

// SessionStats contains aggregated statistics about a Claude session
// @Description Session statistics including token counts and activity metrics
type SessionStats struct {
	// Total number of messages in the session
	TotalMessages int `json:"totalMessages" example:"42"`
	// Number of user messages
	UserMessages int `json:"userMessages" example:"15"`
	// Number of assistant messages
	AssistantMessages int `json:"assistantMessages" example:"27"`
	// Number of human prompts (user messages with text, not tool results)
	HumanPromptCount int `json:"humanPromptCount" example:"10"`
	// Total tool calls made
	ToolCallCount int `json:"toolCallCount" example:"35"`
	// Total input tokens used
	TotalInputTokens int64 `json:"totalInputTokens" example:"150000"`
	// Total output tokens generated
	TotalOutputTokens int64 `json:"totalOutputTokens" example:"85000"`
	// Cache read tokens (context reuse)
	CacheReadTokens int64 `json:"cacheReadTokens" example:"120000"`
	// Cache creation tokens
	CacheCreationTokens int64 `json:"cacheCreationTokens" example:"30000"`
	// Last message's cache_read value (actual context size in tokens)
	LastContextSizeTokens int64 `json:"lastContextSizeTokens" example:"125000"`
	// Number of API calls made
	APICallCount int `json:"apiCallCount" example:"12"`
	// Session duration in seconds (wall-clock time)
	SessionDurationSeconds float64 `json:"sessionDurationSeconds" example:"3600.5"`
	// Active duration in seconds (time Claude was actually working)
	ActiveDurationSeconds float64 `json:"activeDurationSeconds" example:"1800.25"`
	// Number of thinking blocks
	ThinkingBlockCount int `json:"thinkingBlockCount" example:"8"`
	// Number of sub-agents spawned
	SubAgentCount int `json:"subAgentCount" example:"3"`
	// Number of context compactions
	CompactionCount int `json:"compactionCount" example:"1"`
	// Number of images in the conversation
	ImageCount int `json:"imageCount" example:"2"`
	// Tool usage counts by name
	ActiveToolNames map[string]int `json:"activeToolNames,omitempty"`
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
	// Whether to fork the session instead of resuming (doesn't pollute original session history)
	// Defaults to true when resuming unless explicitly set to false
	Fork *bool `json:"fork,omitempty" example:"true"`
	// Whether to suppress stop events for this automated operation
	SuppressEvents bool `json:"suppress_events,omitempty" example:"true"`
	// Whether to disable all tools (Claude will only use context, no tool calls)
	DisableTools bool `json:"disable_tools,omitempty" example:"true"`
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

// ClaudeSettings represents Claude configuration settings
// @Description Claude Code configuration settings from ~/.claude.json
type ClaudeSettings struct {
	// Current theme setting
	Theme string `json:"theme" example:"dark" enums:"dark,light,dark-daltonized,light-daltonized,dark-ansi,light-ansi"`
	// Whether user is authenticated (has userID)
	IsAuthenticated bool `json:"isAuthenticated" example:"true"`
	// Version string derived from lastReleaseNotesSeen
	Version string `json:"version,omitempty" example:"1.2.3"`
	// Whether user has completed onboarding
	HasCompletedOnboarding bool `json:"hasCompletedOnboarding" example:"true"`
	// Number of times Claude has been started
	NumStartups int `json:"numStartups" example:"15"`
	// Whether notifications are enabled
	NotificationsEnabled bool `json:"notificationsEnabled" example:"true"`
}

// ClaudeSettingsUpdateRequest represents a request to update Claude settings
// @Description Request to update Claude Code settings
type ClaudeSettingsUpdateRequest struct {
	// Theme to set (must be one of the valid theme values)
	Theme string `json:"theme,omitempty" example:"dark" enums:"dark,light,dark-daltonized,light-daltonized,dark-ansi,light-ansi"`
	// Whether notifications should be enabled
	NotificationsEnabled *bool `json:"notificationsEnabled,omitempty" example:"true"`
}

// ClaudeHookEvent represents a hook event from Claude Code
// @Description Claude Code hook event for activity tracking
type ClaudeHookEvent struct {
	// Type of the hook event (UserPromptSubmit, Stop, etc.)
	EventType string `json:"event_type" example:"UserPromptSubmit"`
	// Working directory where the event occurred
	WorkingDirectory string `json:"working_directory" example:"/workspace/my-project"`
	// Session ID if available
	SessionID string `json:"session_id,omitempty" example:"abc123-def456-ghi789"`
	// Additional event-specific data
	Data map[string]interface{} `json:"data,omitempty"`
}

// ClaudeOnboardingStatus represents the current status of the onboarding process
// @Description Status of the automated Claude Code onboarding/login flow
type ClaudeOnboardingStatus struct {
	// Current state in the onboarding flow
	State string `json:"state" example:"auth_waiting" enums:"idle,theme_select,auth_method,auth_url,auth_waiting,auth_confirm,security_notes,terminal_setup,complete,error"`
	// OAuth URL for authentication (only present in auth_url and auth_waiting states)
	OAuthURL string `json:"oauth_url,omitempty" example:"https://claude.ai/oauth/authorize?..."`
	// Human-readable message about the current state
	Message string `json:"message,omitempty" example:"Please visit the OAuth URL and paste the code"`
	// Error message if state is 'error'
	ErrorMessage string `json:"error_message,omitempty" example:"Timeout in state auth_waiting after 3m0s"`
	// Full PTY output buffer for debugging (only included if requested)
	Output string `json:"output,omitempty"`
}

// ClaudeOnboardingSubmitCodeRequest represents a request to submit an OAuth code
// @Description Request to submit OAuth code during onboarding
type ClaudeOnboardingSubmitCodeRequest struct {
	// OAuth code obtained from the authentication flow
	Code string `json:"code" example:"abc123def456" binding:"required"`
}
