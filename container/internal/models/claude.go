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
type ClaudeSessionSummary struct {
	WorktreePath     string     `json:"worktreePath"`
	SessionStartTime *time.Time `json:"sessionStartTime"`
	SessionEndTime   *time.Time `json:"sessionEndTime"`
	TurnCount        int        `json:"turnCount"`
	IsActive         bool       `json:"isActive"`
	LastSessionId    *string    `json:"lastSessionId"`
	CurrentSessionId *string    `json:"currentSessionId,omitempty"` // ID of session we're reading timing from
	
	// Metrics (from completed sessions)
	LastCost              *float64 `json:"lastCost,omitempty"`
	LastDuration          *int     `json:"lastDuration,omitempty"`
	LastTotalInputTokens  *int     `json:"lastTotalInputTokens,omitempty"`
	LastTotalOutputTokens *int     `json:"lastTotalOutputTokens,omitempty"`
}