package parser

import (
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// SessionStats contains aggregated statistics about a Claude session
type SessionStats struct {
	TotalMessages         int
	UserMessages          int
	AssistantMessages     int
	HumanPromptCount      int // User messages with text content (not tool results)
	ToolCallCount         int
	TotalInputTokens      int64
	TotalOutputTokens     int64
	CacheReadTokens       int64
	CacheCreationTokens   int64
	LastContextSizeTokens int64 // Last message's cache_read (actual context size)
	APICallCount          int
	SessionDuration       time.Duration // Wall-clock time: last message - first message
	ActiveDuration        time.Duration // Active time: sum of time from each user prompt to Claude's last response
	ThinkingBlockCount    int
	SubAgentCount         int
	CompactionCount       int // Number of times conversation was compacted
	ImageCount            int // Number of images in the conversation
	FirstMessageTime      time.Time
	LastMessageTime       time.Time
	ActiveToolNames       map[string]int // Tool name -> count
}

// ThinkingBlock represents a thinking content block from Claude's response
type ThinkingBlock struct {
	Content   string
	Timestamp time.Time
	Triggers  []ThinkingTrigger
	Level     string
	MessageID string
}

// ThinkingTrigger represents a trigger for thinking mode
type ThinkingTrigger struct {
	Start int
	End   int
	Text  string
}

// SubAgentInfo contains information about a sub-agent (dispatched agent)
type SubAgentInfo struct {
	AgentID      string
	SessionID    string
	SubagentType string // e.g., "Explore", "superpowers:code-reviewer", etc.
	Description  string // Description of the sub-agent's task
	MessageCount int
	FirstSeen    time.Time
	LastSeen     time.Time
	Messages     []models.ClaudeSessionMessage // Optional: full message history
}

// MessageFilter defines criteria for filtering messages
type MessageFilter struct {
	SkipWarmup      bool
	SkipAutomated   bool
	SkipSidechain   bool
	SkipErrors      bool
	OnlyType        string // "" = all, "user", "assistant", etc.
	OnlyContentType string // "" = all, "tool_use", "thinking", etc.
}

// DefaultFilter is the standard message filter that skips warmup and automated messages
var DefaultFilter = MessageFilter{
	SkipWarmup:    true,
	SkipAutomated: true,
	SkipSidechain: false,
	SkipErrors:    false,
}

// ToolUseBlock represents a tool_use content block
type ToolUseBlock struct {
	Type  string
	ID    string
	Name  string
	Input map[string]interface{}
}

// ContentBlock represents a generic content block in a message
type ContentBlock struct {
	Type      string
	Text      string
	Thinking  string
	Signature string
	ToolUse   *ToolUseBlock
}

// ThinkingMetadata represents metadata about thinking blocks
type ThinkingMetadata struct {
	Level    string
	Disabled bool
	Triggers []ThinkingTrigger
}
