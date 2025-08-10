package handlers

import (
	"time"
)

// TerminalStateMessage represents different types of terminal state messages
type TerminalStateMessage struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// TerminalSnapshot represents a complete terminal state
type TerminalSnapshot struct {
	Content   string `json:"content"`   // Complete rendered terminal content
	CursorRow int    `json:"cursorRow"` // Cursor row position
	CursorCol int    `json:"cursorCol"` // Cursor column position
	Cols      int    `json:"cols"`      // Terminal columns
	Rows      int    `json:"rows"`      // Terminal rows
	Version   int64  `json:"version"`   // State version for change tracking
}

// TerminalDelta represents an incremental change to terminal state
type TerminalDelta struct {
	FromVersion int64            `json:"fromVersion"`         // Previous version this builds on
	ToVersion   int64            `json:"toVersion"`           // New version after applying changes
	Changes     []TerminalChange `json:"changes"`             // List of changes to apply
	CursorRow   *int             `json:"cursorRow,omitempty"` // New cursor row if changed
	CursorCol   *int             `json:"cursorCol,omitempty"` // New cursor col if changed
}

// TerminalChange represents a single change to terminal content
type TerminalChange struct {
	Row    int    `json:"row"`    // Row to modify
	Col    int    `json:"col"`    // Starting column
	Length int    `json:"length"` // Number of characters to replace
	Text   string `json:"text"`   // New text to insert
}

// TerminalResizeRequest represents a terminal resize request
type TerminalResizeRequest struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// TerminalInputMessage represents input from client
type TerminalInputMessage struct {
	Data   []byte `json:"data"`   // Input data to send to PTY
	Binary bool   `json:"binary"` // Whether data is binary or text
}

// Message type constants for terminal state protocol
const (
	// Server to Client messages
	MsgTypeTerminalSnapshot = "terminal-snapshot" // Complete terminal state
	MsgTypeTerminalDelta    = "terminal-delta"    // Incremental changes
	MsgTypeTerminalError    = "terminal-error"    // Error occurred

	// Client to Server messages
	MsgTypeTerminalResize = "terminal-resize" // Request terminal resize
	MsgTypeTerminalInput  = "terminal-input"  // Send input to terminal
	MsgTypeTerminalReady  = "terminal-ready"  // Client ready to receive state

	// Bidirectional messages
	MsgTypeTerminalPing = "terminal-ping" // Keepalive/heartbeat
	MsgTypeTerminalPong = "terminal-pong" // Ping response
)

// Protocol Features and Behavior:
//
// 1. CONNECTION ESTABLISHMENT:
//    - Client connects and sends "terminal-ready"
//    - Server responds with "terminal-snapshot" containing full state
//    - Subsequent updates sent as "terminal-delta" messages
//
// 2. STATE VERSIONING:
//    - Each terminal state has a version number
//    - Deltas specify fromVersion and toVersion for consistency
//    - If client is behind, server sends new snapshot
//
// 3. CHANGE DETECTION:
//    - Server tracks previous rendered state
//    - Computes minimal diffs between states
//    - Only sends changes that actually affect visible content
//
// 4. CURSOR TRACKING:
//    - Cursor position included in snapshots and deltas
//    - Client can update cursor independently of content
//
// 5. INPUT HANDLING:
//    - Client sends "terminal-input" messages
//    - Server forwards to PTY, processes through emulator
//    - Changes reflected in next delta/snapshot
//
// 6. ERROR RECOVERY:
//    - If delta application fails, client requests new snapshot
//    - Server can detect inconsistency and send fresh snapshot
//
// 7. PERFORMANCE OPTIMIZATIONS:
//    - Batch multiple changes into single delta
//    - Skip deltas if no visual changes occurred
//    - Compress repeated characters/sequences
//
// Benefits over current approach:
// - Complete state preservation for reconnection
// - Efficient incremental updates
// - Proper cursor and TUI handling
// - No manual escape sequence parsing needed
// - Terminal emulator handles all complexity
