package models

import "time"

// GeminiSessionSummary represents aggregated session information for Gemini.
type GeminiSessionSummary struct {
	UUID        string    `json:"uuid"`
	Title       string    `json:"title"`
	TurnCount   int       `json:"turnCount"`
	LastUpdated time.Time `json:"lastUpdated"`
	Worktree    string    `json:"worktree"`
}

// GeminiSessionMessage represents a single message in a Gemini session.
type GeminiSessionMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// GeminiLogEntry matches the structure of entries in logs.json
type GeminiLogEntry struct {
	SessionID string `json:"sessionId"`
	MessageID int    `json:"messageId"`
	Type      string `json:"type"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}
