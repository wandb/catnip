package models

import (
	"time"
)

// ClaudeProjectMetadata represents the metadata for a Claude Code project
type ClaudeProjectMetadata struct {
	Path                                    string                 `json:"path"`
	AllowedTools                            []string               `json:"allowedTools"`
	History                                 []ClaudeHistoryEntry   `json:"history"`
	McpContextUris                          []string               `json:"mcpContextUris"`
	McpServers                              map[string]any `json:"mcpServers"`
	EnabledMcpjsonServers                   []string               `json:"enabledMcpjsonServers"`
	DisabledMcpjsonServers                  []string               `json:"disabledMcpjsonServers"`
	HasTrustDialogAccepted                  bool                   `json:"hasTrustDialogAccepted"`
	ProjectOnboardingSeenCount              int                    `json:"projectOnboardingSeenCount"`
	HasClaudeMdExternalIncludesApproved     bool                   `json:"hasClaudeMdExternalIncludesApproved"`
	HasClaudeMdExternalIncludesWarningShown bool                   `json:"hasClaudeMdExternalIncludesWarningShown"`
	
	// Session metrics (only present for completed sessions)
	LastCost                           *float64 `json:"lastCost,omitempty"`
	LastAPIDuration                    *int     `json:"lastAPIDuration,omitempty"`
	LastDuration                       *int     `json:"lastDuration,omitempty"`
	LastLinesAdded                     *int     `json:"lastLinesAdded,omitempty"`
	LastLinesRemoved                   *int     `json:"lastLinesRemoved,omitempty"`
	LastTotalInputTokens               *int     `json:"lastTotalInputTokens,omitempty"`
	LastTotalOutputTokens              *int     `json:"lastTotalOutputTokens,omitempty"`
	LastTotalCacheCreationInputTokens  *int     `json:"lastTotalCacheCreationInputTokens,omitempty"`
	LastTotalCacheReadInputTokens      *int     `json:"lastTotalCacheReadInputTokens,omitempty"`
	LastTotalWebSearchRequests         *int     `json:"lastTotalWebSearchRequests,omitempty"`
	LastSessionId                      *string  `json:"lastSessionId,omitempty"`
	
	// Computed fields
	SessionStartTime                   *time.Time `json:"sessionStartTime,omitempty"`
	SessionEndTime                     *time.Time `json:"sessionEndTime,omitempty"`
	TurnCount                          int        `json:"turnCount"`
	IsActiveSession                    bool       `json:"isActiveSession"`
}

// ClaudeHistoryEntry represents an entry in the Claude history
type ClaudeHistoryEntry struct {
	Display        string                 `json:"display"`
	PastedContents map[string]any `json:"pastedContents"`
}

// ClaudeSessionMessage represents a message in a Claude session file
type ClaudeSessionMessage struct {
	Cwd          string                 `json:"cwd"`
	IsMeta       bool                   `json:"isMeta"`
	IsSidechain  bool                   `json:"isSidechain"`
	Message      map[string]any `json:"message"`
	ParentUuid   string                 `json:"parentUuid"`
	SessionId    string                 `json:"sessionId"`
	Timestamp    string                 `json:"timestamp"`
	Type         string                 `json:"type"`
	UserType     string                 `json:"userType"`
	Uuid         string                 `json:"uuid"`
	Version      int                    `json:"version"`
}

// ClaudeSessionSummary represents aggregated session information
// @Description Claude Code session summary with metrics and timing information
type ClaudeSessionSummary struct {
	// Path to the worktree directory
	WorktreePath     string     `json:"worktreePath" example:"/workspace/my-project"`
	// When the current session started
	SessionStartTime *time.Time `json:"sessionStartTime" example:"2024-01-15T14:30:00Z"`
	// When the last session ended (if not active)
	SessionEndTime   *time.Time `json:"sessionEndTime" example:"2024-01-15T16:45:30Z"`
	// Number of conversation turns in the session
	TurnCount        int        `json:"turnCount" example:"15"`
	// Whether this session is currently active
	IsActive         bool       `json:"isActive" example:"true"`
	// ID of the most recent completed session
	LastSessionId    *string    `json:"lastSessionId" example:"abc123-def456"`
	// ID of the currently active session
	CurrentSessionId *string    `json:"currentSessionId,omitempty" example:"xyz789-ghi012"`
	
	// Metrics (from completed sessions)
	// Cost in USD of the last completed session
	LastCost              *float64 `json:"lastCost,omitempty" example:"0.25"`
	// Duration in seconds of the last session
	LastDuration          *int     `json:"lastDuration,omitempty" example:"3600"`
	// Total input tokens used in the last session
	LastTotalInputTokens  *int     `json:"lastTotalInputTokens,omitempty" example:"15000"`
	// Total output tokens generated in the last session
	LastTotalOutputTokens *int     `json:"lastTotalOutputTokens,omitempty" example:"8500"`
}