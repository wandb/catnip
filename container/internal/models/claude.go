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
	WorktreePath     string     `json:"worktreePath" example:"/workspace/my-project" description:"Path to the worktree directory"`
	SessionStartTime *time.Time `json:"sessionStartTime" example:"2024-01-15T14:30:00Z" description:"When the current session started"`
	SessionEndTime   *time.Time `json:"sessionEndTime" example:"2024-01-15T16:45:30Z" description:"When the last session ended (if not active)"`
	TurnCount        int        `json:"turnCount" example:"15" description:"Number of conversation turns in the session"`
	IsActive         bool       `json:"isActive" example:"true" description:"Whether this session is currently active"`
	LastSessionId    *string    `json:"lastSessionId" example:"abc123-def456" description:"ID of the most recent completed session"`
	CurrentSessionId *string    `json:"currentSessionId,omitempty" example:"xyz789-ghi012" description:"ID of the currently active session"`
	
	// Metrics (from completed sessions)
	LastCost              *float64 `json:"lastCost,omitempty" example:"0.25" description:"Cost in USD of the last completed session"`
	LastDuration          *int     `json:"lastDuration,omitempty" example:"3600" description:"Duration in seconds of the last session"`
	LastTotalInputTokens  *int     `json:"lastTotalInputTokens,omitempty" example:"15000" description:"Total input tokens used in the last session"`
	LastTotalOutputTokens *int     `json:"lastTotalOutputTokens,omitempty" example:"8500" description:"Total output tokens generated in the last session"`
}