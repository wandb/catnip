package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"syscall"
	"time"
	"unsafe"

	"github.com/vanpelt/catnip/internal/logger"

	"github.com/creack/pty"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// WorkspaceFailureTracker tracks recreation failures per workspace
type WorkspaceFailureTracker struct {
	FailureCount   int
	FirstFailureAt time.Time
	LastFailureAt  time.Time
	BackoffUntil   time.Time
}

// PTYHandler handles PTY WebSocket connections
type PTYHandler struct {
	sessions       map[string]*Session
	sessionMutex   sync.RWMutex
	failureTracker map[string]*WorkspaceFailureTracker
	failureMutex   sync.RWMutex
	gitService     *services.GitService
	sessionService *services.SessionService
	portService    *services.PortAllocationService
	portMonitor    *services.PortMonitor
	ptyService     *services.PTYService
	claudeMonitor  *services.ClaudeMonitorService
}

// ConnectionInfo tracks metadata for each WebSocket connection
type ConnectionInfo struct {
	ConnectedAt time.Time
	RemoteAddr  string
	ConnID      string
	IsReadOnly  bool
	IsFocused   bool
}

// Session represents a PTY session
type Session struct {
	ID              string
	PTY             *os.File
	Cmd             *exec.Cmd
	CreatedAt       time.Time
	LastAccess      time.Time
	LastRecreation  time.Time
	WorkDir         string
	Agent           string
	Title           string
	ClaudeSessionID string // Track Claude session UUID for resume functionality
	connections     map[*websocket.Conn]*ConnectionInfo
	connMutex       sync.RWMutex
	// Buffer to store PTY output for replay
	outputBuffer  []byte
	bufferMutex   sync.RWMutex
	maxBufferSize int
	// Terminal dimensions
	cols uint16
	rows uint16
	// Buffered dimensions - the size when buffer was captured
	bufferedCols uint16
	bufferedRows uint16
	// WebSocket write protection
	writeMutex sync.Mutex
	// Checkpoint functionality
	checkpointManager git.CheckpointManager
	// Alternate screen buffer detection for TUI applications
	AlternateScreenActive bool
	LastNonTUIBufferSize  int
	// Session-level PTY reading control
	ptyReadDone   chan struct{}
	ptyReadClosed bool
	ptyReadMutex  sync.Mutex
	// Recreation protection - prevents concurrent recreation attempts
	recreationInProgress bool
	recreationMutex      sync.Mutex
	// External workspace read-only protection
	IsReadOnlyWorkspace bool
	// Terminal emulator for Claude sessions (server-side terminal state)
}

// ResizeMsg represents terminal resize message
type ResizeMsg struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// ControlMsg represents control commands
type ControlMsg struct {
	Type   string `json:"type"`
	Data   string `json:"data,omitempty"`
	Submit bool   `json:"submit,omitempty"`
}

// safeClosePTYReadDone safely closes the ptyReadDone channel, preventing double-close panics
func (s *Session) safeClosePTYReadDone() {
	s.ptyReadMutex.Lock()
	defer s.ptyReadMutex.Unlock()

	if !s.ptyReadClosed && s.ptyReadDone != nil {
		close(s.ptyReadDone)
		s.ptyReadClosed = true
	}
}

// sanitizeTitle ensures the extracted title is safe and conforms to expected formats
func sanitizeTitle(title string) string {
	// Allow alphanumeric characters, spaces, and common punctuation
	safeTitle := strings.Map(func(r rune) rune {
		if strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 .,-_:()[]{}/@#$%&*+=!?", r) {
			return r
		}
		return -1
	}, title)

	// Limit the length of the title to prevent abuse
	if len(safeTitle) > 100 {
		safeTitle = safeTitle[:100]
	}

	// Strip leading/trailing whitespace
	return strings.TrimSpace(safeTitle)
}

// extractTitleFromEscapeSequence extracts the fancy Claude terminal title from escape sequences
func extractTitleFromEscapeSequence(data []byte) (string, bool) {
	startSeq := []byte("\x1b]0;")
	endChar := byte('\x07')

	start := bytes.Index(data, startSeq)
	if start == -1 {
		return "", false
	}
	end := bytes.IndexByte(data[start+len(startSeq):], endChar)
	if end == -1 {
		return "", false
	}

	title := data[start+len(startSeq) : start+len(startSeq)+end]
	return sanitizeTitle(string(title)), true
}

// isExternalWorkspace checks if a workspace directory is outside our managed workspace directory
func (h *PTYHandler) isExternalWorkspace(workDir string) bool {
	// Check if the workspace directory is outside our managed WORKSPACE_DIR
	workspaceDir := config.Runtime.WorkspaceDir
	if workspaceDir == "" {
		return false
	}

	// Clean both paths for comparison
	cleanWorkspaceDir := filepath.Clean(workspaceDir)
	cleanWorkDir := filepath.Clean(workDir)

	// Check if workDir is under workspaceDir
	relPath, err := filepath.Rel(cleanWorkspaceDir, cleanWorkDir)
	if err != nil {
		return true // If we can't determine relationship, assume external for safety
	}

	// If the relative path starts with "..", it means workDir is outside workspaceDir
	return strings.HasPrefix(relPath, "..")
}

// findWorktreeByName finds a worktree by its name in the state
func (h *PTYHandler) findWorktreeByName(name string) *models.Worktree {
	if h.gitService == nil {
		return nil
	}

	stateManager := h.gitService.GetStateManager()
	if stateManager == nil {
		return nil
	}

	allWorktrees := stateManager.GetAllWorktrees()
	for _, worktree := range allWorktrees {
		if worktree.Name == name {
			return worktree
		}
	}
	return nil
}

// NewPTYHandler creates a new PTY handler
func NewPTYHandler(gitService *services.GitService, claudeMonitor *services.ClaudeMonitorService, sessionService *services.SessionService, portMonitor *services.PortMonitor) *PTYHandler {
	h := &PTYHandler{
		sessions:       make(map[string]*Session),
		failureTracker: make(map[string]*WorkspaceFailureTracker),
		gitService:     gitService,
		sessionService: sessionService,
		portService:    services.NewPortAllocationService(),
		portMonitor:    portMonitor, // Use the provided portMonitor instead of creating new one
		ptyService:     services.NewPTYService(),
		claudeMonitor:  claudeMonitor,
	}

	// Start periodic cleanup routine for non-existent workspaces
	go h.periodicWorkspaceCleanup()

	return h
}

// findClaudeExecutable finds the claude executable using robust path lookup
func (h *PTYHandler) findClaudeExecutable() string {
	// PRIORITY 1: Try Catnip's wrapper script first (for title interception)
	catnipClaudePath := "/opt/catnip/bin/claude"
	if _, err := os.Stat(catnipClaudePath); err == nil {
		logger.Debugf("Found Catnip claude wrapper: %s", catnipClaudePath)
		return catnipClaudePath
	}

	// PRIORITY 2: Try standard PATH lookup
	if path, err := exec.LookPath("claude"); err == nil {
		logger.Debugf("Found claude in PATH: %s", path)
		return path
	}

	// PRIORITY 3: Try ~/.local/bin/claude (common user install location)
	if homeDir, err := os.UserHomeDir(); err == nil {
		userClaudePath := filepath.Join(homeDir, ".local", "bin", "claude")
		if _, err := os.Stat(userClaudePath); err == nil {
			logger.Debugf("Found claude at user location: %s", userClaudePath)
			return userClaudePath
		}
	}

	// PRIORITY 4: Try NVM_BIN path
	if nvmBin := os.Getenv("NVM_BIN"); nvmBin != "" {
		nvmClaudePath := filepath.Join(nvmBin, "claude")
		if _, err := os.Stat(nvmClaudePath); err == nil {
			logger.Debugf("Found claude at NVM_BIN path: %s", nvmClaudePath)
			return nvmClaudePath
		}
	}

	// PRIORITY 5: Try other common installation paths
	commonPaths := []string{
		"/opt/catnip/nvm/versions/node/v22.17.0/bin/claude",
		"/usr/local/bin/claude",
		"/opt/homebrew/bin/claude",
		"/home/catnip/.local/bin/claude",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			logger.Debugf("Found claude at fallback path: %s", path)
			return path
		}
	}

	// If all else fails, return "claude" and let exec.Command handle the error
	logger.Warnf("Claude executable not found in any known location, falling back to basic 'claude' command")
	return "claude"
}

// HandleWebSocket handles WebSocket connections for PTY
// @Summary Create PTY WebSocket connection
// @Description Establishes a WebSocket connection for terminal access
// @Tags pty
// @Param session query string true "Session ID"
// @Success 101 {string} string "Switching Protocols"
// @Router /v1/pty [get]
func (h *PTYHandler) HandleWebSocket(c *fiber.Ctx) error {
	// Check if it's a WebSocket request
	if websocket.IsWebSocketUpgrade(c) {
		// Extract session ID and agent before WebSocket upgrade
		defaultSession := os.Getenv("CATNIP_SESSION")
		if defaultSession == "" {
			defaultSession = "default"
		}
		sessionID := c.Query("session", defaultSession)
		agent := c.Query("agent", "")
		reset := c.Query("reset", "false") == "true"

		// Debug logging to understand what session ID we're actually receiving
		logger.Debugf("🔍 WebSocket PTY request - Raw session param: %q, Default session: %q, Final sessionID: %q", c.Query("session"), defaultSession, sessionID)

		// Create composite session key: path + agent
		compositeSessionID := sessionID
		if agent != "" {
			compositeSessionID = fmt.Sprintf("%s:%s", sessionID, agent)
		}

		return websocket.New(func(conn *websocket.Conn) {
			h.handlePTYConnection(conn, compositeSessionID, agent, reset)
		})(c)
	}
	return fiber.ErrUpgradeRequired
}

func (h *PTYHandler) handlePTYConnection(conn *websocket.Conn, sessionID, agent string, reset bool) {
	// Generate unique connection ID for logging and tracking
	connID := fmt.Sprintf("%p", conn)

	if agent != "" {
		logger.Debugf("📡 New PTY connection [%s] for session: %s with agent: %s (reset: %t)", connID, sessionID, agent, reset)
	} else {
		logger.Debugf("📡 New PTY connection [%s] for session: %s (reset: %t)", connID, sessionID, reset)
	}

	// Handle reset logic for Claude agent
	if reset && agent == "claude" {
		logger.Infof("🔄 Reset requested for Claude session: %s", sessionID)
		// Shutdown any existing PTY session for this sessionID
		h.sessionMutex.Lock()
		if existingSession, exists := h.sessions[sessionID]; exists {
			logger.Infof("🛑 Shutting down existing session: %s", sessionID)
			h.cleanupSession(existingSession)
			delete(h.sessions, sessionID)
		}
		h.sessionMutex.Unlock()
	}

	// Get or create session
	session := h.getOrCreateSession(sessionID, agent, reset)
	if session == nil {
		logger.Errorf("❌ Failed to create session: %s", sessionID)

		// Send error message to client before closing
		errorMsg := struct {
			Type      string `json:"type"`
			Error     string `json:"error"`
			Message   string `json:"message"`
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		}{
			Type:      "error",
			Error:     "Worktree not found",
			Message:   fmt.Sprintf("The worktree '%s' does not exist", sessionID),
			Code:      "WORKTREE_NOT_FOUND",
			Retryable: false, // This error is not retryable - workspace doesn't exist
		}

		if data, err := json.Marshal(errorMsg); err == nil {
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}

		conn.Close()
		return
	}

	// Add protective error handling around connection registration
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("❌ PANIC in connection registration for session %s: %v", sessionID, r)
			conn.Close()
		}
	}()

	// Add connection to session with read-only logic
	session.connMutex.Lock()

	remoteAddr := conn.RemoteAddr().String()
	logger.Debugf("🔌 New connection [%s] from %s to session %s", connID, remoteAddr, sessionID)

	// NUCLEAR APPROACH: Force-close ALL existing connections when a new one connects
	//
	// TRADEOFF DECISION: This breaks multi-tab functionality but fixes ghost connection bugs
	// - PRO: Eliminates read-only badge issues from stale connections across navigation
	// - PRO: Ensures each terminal always starts with clean state (no ghost connections)
	// - CON: Multi-tab usage will cause tabs to disconnect/reconnect when switching
	// - CON: Users can't have the same workspace open in multiple tabs simultaneously
	//
	// Alternative approaches tried:
	// 1. Smart cleanup by remote address - failed due to cross-navigation state persistence
	// 2. Ping-based stale detection - failed because ghost connections were responding to pings
	// 3. Focus-based promotion - worked but couldn't fix underlying connection count issues
	//
	// TODO: Consider implementing proper connection lifecycle management in the future
	// For now, this ensures terminals work reliably in single-tab scenarios

	existingConnectionCount := len(session.connections)
	if existingConnectionCount > 0 {
		logger.Infof("🧹 FORCE CLEANUP: Found %d existing connections in session %s, closing all", existingConnectionCount, sessionID)

		// Force close all existing connections
		var connectionsToClose []*websocket.Conn
		for conn := range session.connections {
			connectionsToClose = append(connectionsToClose, conn)
		}

		// Close them outside the range loop to avoid map modification during iteration
		for _, existingConn := range connectionsToClose {
			if _, exists := session.connections[existingConn]; exists {
				delete(session.connections, existingConn)
				existingConn.Close()
			}
		}

		logger.Debugf("✅ Cleared all %d existing connections from session %s", len(connectionsToClose), sessionID)
	}

	connectionCount := len(session.connections)
	logger.Debugf("🔍 Connection count for session %s: %d (after cleanup)", sessionID, connectionCount)

	// Determine read-only status: external workspaces are always read-only for safety
	isReadOnly := session.IsReadOnlyWorkspace || connectionCount > 0
	if session.IsReadOnlyWorkspace {
		logger.Debugf("🔒 Setting connection [%s] to read-ONLY mode (external workspace)", connID)
	} else if connectionCount > 0 {
		logger.Debugf("🔒 Setting connection [%s] to read-ONLY mode (existing connections: %d)", connID, connectionCount)
	} else {
		logger.Debugf("✍️ Setting connection [%s] to WRITE mode (first connection)", connID)
	}

	session.connections[conn] = &ConnectionInfo{
		ConnectedAt: time.Now(),
		RemoteAddr:  conn.RemoteAddr().String(),
		ConnID:      connID,
		IsReadOnly:  isReadOnly,
		IsFocused:   false, // Will be updated when focus event is received
	}
	newConnectionCount := len(session.connections)
	session.connMutex.Unlock()

	if isReadOnly {
		logger.Debugf("🔗 Added READ-ONLY connection [%s] to session %s (connections: %d → %d)", connID, sessionID, connectionCount, newConnectionCount)

		// Notify client that it's read-only
		readOnlyMsg := struct {
			Type string `json:"type"`
			Data bool   `json:"data"`
		}{
			Type: "read-only",
			Data: true,
		}
		if data, err := json.Marshal(readOnlyMsg); err == nil {
			_ = session.writeToConnection(conn, websocket.TextMessage, data)
		}
	} else {
		logger.Debugf("🔗 Added WRITE connection [%s] to session %s (connections: %d → %d)", connID, sessionID, connectionCount, newConnectionCount)

		// Notify client that it has write access
		writeAccessMsg := struct {
			Type string `json:"type"`
			Data bool   `json:"data"`
		}{
			Type: "read-only",
			Data: false,
		}
		if data, err := json.Marshal(writeAccessMsg); err == nil {
			_ = session.writeToConnection(conn, websocket.TextMessage, data)
		}

	}

	// Don't replay buffer immediately - wait for client ready signal
	// This prevents race conditions with PTY state

	// Channel to signal when connection should close
	done := make(chan struct{})

	// Clean up connection on exit
	defer func() {
		// Recover from any panics in this connection handler
		if r := recover(); r != nil {
			logger.Errorf("❌ Recovered from panic in PTY connection handler: %v", r)
		}

		close(done) // Signal goroutines to stop
		session.connMutex.Lock()

		// Check if this was a write-enabled connection
		connInfo, exists := session.connections[conn]
		wasWriteConnection := exists && !connInfo.IsReadOnly

		if exists {
			logger.Debugf("🔌❌ Removing connection [%s] from session %s (was write: %v)", connInfo.ConnID, session.ID, !connInfo.IsReadOnly)
		}

		delete(session.connections, conn)
		connectionCount := len(session.connections)
		logger.Debugf("🔍 Connection count for session %s: %d (after removal)", session.ID, connectionCount)

		// If the write connection disconnected, promote the oldest read-only connection
		if wasWriteConnection && connectionCount > 0 {
			var oldestConn *websocket.Conn
			var oldestTime time.Time

			for c, info := range session.connections {
				if info.IsReadOnly && (oldestConn == nil || info.ConnectedAt.Before(oldestTime)) {
					oldestConn = c
					oldestTime = info.ConnectedAt
				}
			}

			if oldestConn != nil {
				session.connections[oldestConn].IsReadOnly = false
				promotedConnID := session.connections[oldestConn].ConnID
				logger.Debugf("🔄 Promoted connection [%s] to WRITE access in session %s", promotedConnID, sessionID)

				// Notify the promoted connection about write access
				writeAccessMsg := struct {
					Type string `json:"type"`
					Data bool   `json:"data"`
				}{
					Type: "read-only",
					Data: false,
				}
				if data, err := json.Marshal(writeAccessMsg); err == nil {
					_ = oldestConn.WriteMessage(websocket.TextMessage, data)
				}
			}
		}

		session.connMutex.Unlock()

		if wasWriteConnection {
			logger.Debugf("🔌 WRITE connection [%s] closed for session %s (remaining: %d)", connID, sessionID, connectionCount)
		} else {
			logger.Debugf("🔌 read-only connection [%s] closed for session %s (remaining: %d)", connID, sessionID, connectionCount)
		}

		// Safe close with error handling
		if err := conn.Close(); err != nil {
			logger.Warnf("❌ Error closing websocket connection: %v", err)
		}
	}()

	// Read from WebSocket and write to PTY
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			// Don't log normal WebSocket close conditions as errors
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				logger.Debugf("🔌 WebSocket connection closed normally: %v", err)
			} else {
				logger.Errorf("❌ WebSocket read error: %v", err)
			}
			break
		}

		// Handle different message types
		if messageType == websocket.TextMessage {
			// Try to parse as JSON for control messages first
			var controlMsg ControlMsg
			if err := json.Unmarshal(data, &controlMsg); err == nil && controlMsg.Type != "" {
				switch controlMsg.Type {
				case "reset":
					logger.Infof("🔄 Reset command received for session: %s", sessionID)
					h.recreateSession(session)
					continue
				case "ready":
					logger.Debugf("🎯 Client ready signal received")

					// For Claude sessions, just send buffer-complete and let frontend resize trigger redraw
					if session.Agent == "claude" {
						logger.Debugf("🔄 Claude session reconnected - frontend resize will trigger redraw")

						// Just send buffer-complete - frontend will send resize which triggers SIGWINCH at right time
						completeMsg := struct {
							Type string `json:"type"`
						}{
							Type: "buffer-complete",
						}

						if data, err := json.Marshal(completeMsg); err == nil {
							_ = session.writeToConnection(conn, websocket.TextMessage, data)
						}
					} else {
						// Traditional buffer replay for non-Claude sessions
						session.bufferMutex.RLock()
						hasBuffer := len(session.outputBuffer) > 0
						bufferCols := session.bufferedCols
						bufferRows := session.bufferedRows
						session.bufferMutex.RUnlock()

						if hasBuffer && bufferCols > 0 && bufferRows > 0 {
							// First, resize PTY to match buffered dimensions
							logger.Infof("📐 Resizing PTY to buffered dimensions %dx%d before replay", bufferCols, bufferRows)
							_ = h.resizePTY(session.PTY, bufferCols, bufferRows)

							// Tell client what size to use for replay
							sizeMsg := struct {
								Type string `json:"type"`
								Cols uint16 `json:"cols"`
								Rows uint16 `json:"rows"`
							}{
								Type: "buffer-size",
								Cols: bufferCols,
								Rows: bufferRows,
							}

							if data, err := json.Marshal(sizeMsg); err == nil {
								_ = session.writeToConnection(conn, websocket.TextMessage, data)
							}

							// Then replay the buffer (filter TUI content if alternate screen is active)
							session.bufferMutex.RLock()
							var bufferToReplay []byte

							if session.AlternateScreenActive && session.LastNonTUIBufferSize > 0 {
								// Only replay content up to where alternate screen was entered
								bufferToReplay = make([]byte, session.LastNonTUIBufferSize)
								copy(bufferToReplay, session.outputBuffer[:session.LastNonTUIBufferSize])
								logger.Debugf("📋 Replaying %d bytes (filtered from %d) - excluding TUI content", len(bufferToReplay), len(session.outputBuffer))
							} else {
								// Replay entire buffer for non-TUI sessions
								bufferToReplay = make([]byte, len(session.outputBuffer))
								copy(bufferToReplay, session.outputBuffer)
								logger.Debugf("📋 Replaying %d bytes of buffered output at %dx%d", len(bufferToReplay), bufferCols, bufferRows)
							}
							session.bufferMutex.RUnlock()

							if err := session.writeToConnection(conn, websocket.BinaryMessage, bufferToReplay); err != nil {
								logger.Warnf("❌ Failed to replay buffer: %v", err)
							}

							// If we filtered TUI content, send a refresh signal to trigger TUI repaint
							if session.AlternateScreenActive && session.LastNonTUIBufferSize > 0 {
								go func() {
									time.Sleep(100 * time.Millisecond)
									logger.Infof("🔄 Sending Ctrl+L to refresh TUI after filtered buffer replay")
									if _, err := session.PTY.Write([]byte("\x0c")); err != nil {
										logger.Warnf("❌ Failed to send refresh signal: %v", err)
									}
								}()
							}
						} else {
							logger.Debugf("📋 No buffer to replay or dimensions not captured")
						}
					}

					// Always send buffer complete signal
					completeMsg := struct {
						Type string `json:"type"`
					}{
						Type: "buffer-complete",
					}

					if data, err := json.Marshal(completeMsg); err == nil {
						_ = session.writeToConnection(conn, websocket.TextMessage, data)
					}
					continue
				case "prompt":
					// Handle prompt injection for Claude TUI
					if controlMsg.Data != "" {
						logger.Infof("📝 Injecting prompt into PTY: %q (submit: %v)", controlMsg.Data, controlMsg.Submit)
						if _, err := session.PTY.Write([]byte(controlMsg.Data)); err != nil {
							logger.Warnf("❌ Failed to write prompt to PTY: %v", err)
						}

						// If submit is true, send a carriage return after a small delay
						// This mimics how a user would type and then press Enter
						if controlMsg.Submit {
							go func() {
								// Small delay to let the TUI process the prompt text
								time.Sleep(100 * time.Millisecond)
								logger.Infof("↩️ Sending carriage return (\\r) to execute prompt")
								if _, err := session.PTY.Write([]byte("\r")); err != nil {
									logger.Warnf("❌ Failed to write carriage return to PTY: %v", err)
								}
							}()
						}
					}
					continue
				case "promote":
					// Handle connection promotion request (swap read/write permissions)
					logger.Infof("🔄 Promotion request received from connection [%s] in session %s", connID, sessionID)
					h.promoteConnection(session, conn)
					continue
				case "focus":
					// Handle focus state change
					var focusMsg struct {
						Type    string `json:"type"`
						Focused bool   `json:"focused"`
					}
					if err := json.Unmarshal(data, &focusMsg); err == nil {
						h.handleFocusChange(session, conn, focusMsg.Focused)
					}
					continue
				}
			}

			// Try to parse as JSON for resize messages
			var resizeMsg ResizeMsg
			if err := json.Unmarshal(data, &resizeMsg); err == nil && resizeMsg.Cols > 0 && resizeMsg.Rows > 0 {
				// For Claude sessions, always force dimension change to ensure SIGWINCH triggers redraw
				if session.Agent == "claude" {
					// First resize to slightly different dimensions to force a change
					_ = h.resizePTY(session.PTY, resizeMsg.Cols-1, resizeMsg.Rows-1)
					// Small delay to ensure the change is processed
					time.Sleep(10 * time.Millisecond)
					// Then resize to the target dimensions
					_ = h.resizePTY(session.PTY, resizeMsg.Cols, resizeMsg.Rows)
					logger.Debugf("📐 Resized Claude PTY to %dx%d", resizeMsg.Cols, resizeMsg.Rows)
				} else {
					// For non-Claude sessions, only resize if dimensions actually changed
					if session.cols != resizeMsg.Cols || session.rows != resizeMsg.Rows {
						_ = h.resizePTY(session.PTY, resizeMsg.Cols, resizeMsg.Rows)
						logger.Debugf("📐 Resized PTY to %dx%d", resizeMsg.Cols, resizeMsg.Rows)
					}
				}
				session.cols = resizeMsg.Cols
				session.rows = resizeMsg.Rows
				continue
			}
		}

		// Check if this connection has write access
		session.connMutex.RLock()
		connInfo, exists := session.connections[conn]
		session.connMutex.RUnlock()

		if !exists {
			logger.Infof("⚠️ Connection [%s] no longer exists in session", connID)
			break
		}

		if connInfo.IsReadOnly {
			logger.Infof("🚫 Ignoring input from read-only connection [%s] in session %s", connID, sessionID)
			continue
		}

		// Write data to PTY (only from write-enabled connections)
		if _, err := session.PTY.Write(data); err != nil {
			logger.Errorf("❌ PTY write error: %v", err)
			break
		}

		// Update last access time
		session.LastAccess = time.Now()
	}
}

func (h *PTYHandler) getOrCreateSession(sessionID, agent string, reset bool) *Session {
	// Sanitize session ID to prevent path traversal
	sessionID = h.sanitizeSessionID(sessionID)

	h.sessionMutex.RLock()
	session, exists := h.sessions[sessionID]
	h.sessionMutex.RUnlock()

	if exists {
		// Check if agent has changed
		if session.Agent != agent {
			logger.Infof("🔄 Agent changed from %s to %s for session %s, recreating...", session.Agent, agent, sessionID)
			// Update the agent and recreate
			session.Agent = agent
			h.recreateSession(session)
		} else {
			logger.Infof("🔄 Reusing existing session %s with agent: '%s'", sessionID, session.Agent)
		}
		return session
	}

	// Create new session
	h.sessionMutex.Lock()
	defer h.sessionMutex.Unlock()

	// Double-check after acquiring write lock
	if session, exists := h.sessions[sessionID]; exists {
		return session
	}

	// For bash sessions, ensure we don't have stale buffer data from other sessions
	// by checking if there are any existing sessions that might be contaminating buffers
	if agent == "" {
		logger.Debugf("🔄 Creating new bash session %s, ensuring clean state", sessionID)
	}

	// Set workspace directory with validation
	var workDir string

	// Extract base session ID without agent suffix for worktree lookups
	baseSessionID := sessionID
	if idx := strings.LastIndex(sessionID, ":"); idx != -1 {
		baseSessionID = sessionID[:idx]
	}

	// Validate session ID and get workspace directory
	// Support both "default" symlink and state-based worktree lookups
	if baseSessionID == "default" {
		// Check if current symlink exists in workspace directory
		currentSymlinkPath := filepath.Join(config.Runtime.WorkspaceDir, "current")
		if target, err := os.Readlink(currentSymlinkPath); err == nil {
			// Symlink exists, check if target is valid
			if info, err := os.Stat(target); err == nil && info.IsDir() {
				workDir = target
				logger.Infof("📁 Using current workspace symlink for default session: %s", workDir)
			} else {
				logger.Errorf("❌ Current workspace symlink target is invalid: %s", target)
				return nil
			}
		} else {
			logger.Errorf("❌ Default session requested but current symlink does not exist at %s", currentSymlinkPath)
			return nil
		}
	} else {
		// Look up workspace in state by name first (supports external repos and standard workspaces)
		worktree := h.findWorktreeByName(baseSessionID)
		if worktree != nil {
			// Use the path from the worktree state
			workDir = worktree.Path
			logger.Infof("📁 Using worktree from state for session %s: %s (name: %s)", baseSessionID, workDir, worktree.Name)
		} else {
			// Fallback to legacy directory-based lookup for backward compatibility
			if strings.Contains(baseSessionID, "/") {
				// Check if session ID is in repo/branch format (e.g., "catnip/pirate")
				parts := strings.SplitN(baseSessionID, "/", 2)
				if len(parts) == 2 {
					repo := parts[0]
					branch := parts[1]

					// Check for worktree at workspace/repo/branch (our standard pattern)
					branchWorktreePath := filepath.Join(config.Runtime.WorkspaceDir, repo, branch)
					if info, err := os.Stat(branchWorktreePath); err == nil && info.IsDir() {
						// Additional validation: check if it's actually a git worktree
						if _, err := os.Stat(filepath.Join(branchWorktreePath, ".git")); err == nil {
							workDir = branchWorktreePath
							logger.Debugf("📁 Using Git worktree for session %s: %s", baseSessionID, workDir)
						} else {
							logger.Errorf("❌ Directory exists but is not a valid git worktree: %s", branchWorktreePath)
							logger.Errorf("❌ CRITICAL: Refusing to create PTY session for non-existent worktree to prevent opening wrong directory")
							return nil
						}
					} else {
						logger.Errorf("❌ Worktree directory does not exist: %s", branchWorktreePath)
						logger.Infof("❌ CRITICAL: Refusing to create PTY session for non-existent worktree to prevent opening wrong directory")
						return nil
					}
				} else {
					logger.Errorf("❌ Invalid session format: %s", baseSessionID)
					return nil
				}
			} else {
				// Single name session - check if directory exists
				sessionWorkDir := filepath.Join(config.Runtime.WorkspaceDir, baseSessionID)
				if info, err := os.Stat(sessionWorkDir); err == nil && info.IsDir() {
					workDir = sessionWorkDir
					logger.Infof("📁 Using existing workspace directory: %s", workDir)
				} else {
					logger.Errorf("❌ Workspace directory does not exist: %s", sessionWorkDir)
					logger.Errorf("❌ CRITICAL: Refusing to create PTY session for non-existent workspace to prevent opening wrong directory")
					return nil
				}
			}
		}
	}

	// workDir should be set at this point or we would have returned nil
	if workDir == "" {
		logger.Errorf("❌ Failed to determine valid workspace directory for session: %s", baseSessionID)
		return nil
	}

	// Check for existing Claude session in this directory for auto-resume
	var resumeSessionID string
	var useContinue bool
	if agent == "claude" && !reset {
		// Only use --continue if there's existing session data
		if existingState, err := h.sessionService.FindSessionByDirectory(workDir); err == nil && existingState != nil {
			// For existing sessions, use --continue instead of --resume
			useContinue = true
			logger.Infof("🔄 Found existing Claude session in %s, will use --continue", workDir)
		} else {
			logger.Infof("🔄 No existing Claude session found in %s, starting fresh", workDir)
		}
	}

	// Allocate ports for this session
	ports, err := h.portService.AllocatePortsForSession(sessionID)
	if err != nil {
		logger.Errorf("❌ Failed to allocate ports for session %s: %v", sessionID, err)
		return nil
	}
	logger.Debugf("🔗 Allocated ports for session %s: PORT=%d, PORTZ=%v", sessionID, ports.PORT, ports.PORTZ)

	// Create command based on agent parameter
	cmd := h.createCommand(sessionID, agent, workDir, resumeSessionID, useContinue, ports)

	var ptmx *os.File

	// Start PTY for all session types including setup
	ptmx, err = pty.Start(cmd)
	if err != nil {
		logger.Errorf("❌ Failed to start PTY: %v", err)
		return nil
	}
	// Set initial size
	_ = h.resizePTY(ptmx, 80, 24)

	session = &Session{
		ID:            sessionID,
		PTY:           ptmx,
		Cmd:           cmd,
		CreatedAt:     time.Now(),
		LastAccess:    time.Now(),
		WorkDir:       workDir,
		Agent:         agent,
		connections:   make(map[*websocket.Conn]*ConnectionInfo),
		outputBuffer:  make([]byte, 0),
		maxBufferSize: 5 * 1024 * 1024, // 5MB buffer
		cols:          80,
		rows:          24,
		bufferedCols:  80,
		bufferedRows:  24,
		checkpointManager: git.NewSessionCheckpointManager(
			workDir,
			services.NewGitServiceAdapter(h.gitService),
			services.NewSessionServiceAdapter(h.sessionService),
		),
		// Initialize alternate screen buffer detection
		AlternateScreenActive: false,
		LastNonTUIBufferSize:  0,
		// Initialize session-level PTY reading control
		ptyReadDone:   make(chan struct{}),
		ptyReadClosed: false,
		// Initialize recreation protection
		recreationInProgress: false,
		// Set read-only mode for external workspaces (Claude sessions only)
		IsReadOnlyWorkspace: agent == "claude" && h.isExternalWorkspace(workDir),
	}

	h.sessions[sessionID] = session
	logger.Debugf("✅ Created new PTY session: %s in %s with agent: %s", sessionID, workDir, agent)

	// Log read-only mode for external workspaces
	if session.IsReadOnlyWorkspace {
		logger.Infof("🔒 External workspace detected, session will be read-only for safety: %s", workDir)
	}

	// Track active session for this workspace
	if agent == "claude" {
		// Start or resume session tracking - we'll update with actual Claude session UUID later
		if activeSession, err := h.sessionService.StartOrResumeActiveSession(workDir, ""); err != nil {
			logger.Infof("⚠️  Failed to start/resume session tracking for %s: %v", workDir, err)
		} else {
			// Inherit the current title from the active session if it exists
			if activeSession.Title != nil && activeSession.Title.Title != "" {
				session.Title = activeSession.Title.Title
				logger.Infof("🔄 Inherited existing title from active session: %q", session.Title)
			}
		}
	}

	// Save initial session state for persistence
	if agent == "claude" {
		go h.saveSessionState(session)
	}

	// Start session cleanup goroutine
	go h.monitorSession(session)

	// Start Claude session ID monitoring for claude sessions
	if agent == "claude" {
		go h.monitorClaudeSession(session)

		// Trigger immediate Claude activity state sync to update frontend quickly
		if h.gitService != nil {
			go func() {
				// Small delay to allow Claude process to fully start
				time.Sleep(1 * time.Second)
				h.gitService.TriggerClaudeActivitySync()
				logger.Infof("🔄 Triggered immediate Claude activity state sync for %s", session.WorkDir)
			}()
		}
	}

	// Start checkpoint monitoring for all sessions
	go h.monitorCheckpoints(session)

	// Start session-level PTY reading to prevent Claude from being blocked
	go h.readPTYContinuously(session)

	return session
}

// readPTYContinuously continuously reads from the PTY to prevent blocking Claude
// even when no WebSocket connections are active
func (h *PTYHandler) readPTYContinuously(session *Session) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("❌ Recovered from panic in continuous PTY reader: %v", r)
		}
	}()

	logger.Debugf("🔄 Starting continuous PTY reader for session: %s", session.ID)
	buf := make([]byte, 1024)

	for {
		select {
		case <-session.ptyReadDone:
			logger.Debugf("🛑 Stopping continuous PTY reader for session: %s", session.ID)
			return
		default:
		}

		// Check if PTY file descriptor is still valid before attempting read
		if session.PTY == nil {
			logger.Debugf("🛑 PTY is nil, stopping continuous reader for session: %s", session.ID)
			return
		}

		// Add protective error handling around PTY read operations
		n, err := func() (int, error) {
			// Check if PTY is nil before attempting read
			if session.PTY == nil {
				return 0, fmt.Errorf("PTY is nil")
			}

			// Add timeout to prevent hanging reads
			_ = session.PTY.SetReadDeadline(time.Now().Add(5 * time.Second))
			defer func() {
				_ = session.PTY.SetReadDeadline(time.Time{}) // Clear deadline
			}()

			return session.PTY.Read(buf)
		}()

		if err != nil {
			// Check for various exit conditions including "file already closed"
			if err == io.EOF ||
				strings.Contains(err.Error(), "read /dev/ptmx: input/output error") ||
				strings.Contains(err.Error(), "file already closed") ||
				strings.Contains(err.Error(), "bad file descriptor") ||
				strings.Contains(err.Error(), "PTY is nil") {
				// For setup sessions, don't recreate - they're meant to exit after showing the log
				if session.Agent == "setup" {
					logger.Infof("✅ Setup session completed normally, stopping continuous reader: %s", session.ID)
					return
				}

				// Check if recreation is already in progress to prevent concurrent recreation attempts
				session.recreationMutex.Lock()
				if session.recreationInProgress {
					logger.Debugf("🔄 Recreation already in progress for session %s, exiting reader goroutine", session.ID)
					session.recreationMutex.Unlock()
					return
				}
				session.recreationInProgress = true
				session.recreationMutex.Unlock()

				// Enhanced rate limiting with exponential backoff per workspace
				canRecreate, waitDuration := h.canRecreateSession(session)
				if !canRecreate {
					workspaceID := extractWorkspaceFromSessionID(session.ID)

					// If wait duration is 0, it means workspace doesn't exist - clean up immediately
					if waitDuration == 0 && !h.workspaceExists(workspaceID) {
						logger.Warnf("🧹 Cleaning up session for non-existent workspace %s: %s", workspaceID, session.ID)
						// Reset recreation flag before cleanup
						session.recreationMutex.Lock()
						session.recreationInProgress = false
						session.recreationMutex.Unlock()
						h.cleanupSession(session)
						return
					}

					if waitDuration >= time.Minute {
						logger.Infof("🚫 Exponential backoff active for workspace %s - waiting %v before next recreation attempt", workspaceID, waitDuration)
					} else {
						logger.Infof("⏸️ Rate limiting PTY recreation for session %s (workspace: %s) - waiting %v", session.ID, workspaceID, waitDuration)
					}
					// Reset recreation flag before sleeping
					session.recreationMutex.Lock()
					session.recreationInProgress = false
					session.recreationMutex.Unlock()
					time.Sleep(waitDuration)
					continue
				}

				session.LastRecreation = time.Now()
				logger.Infof("🔄 PTY closed (shell exited: %v), creating new session...", err)

				// Track this as a potential failure before attempting recreation
				h.trackRecreationFailure(session)

				// Create new PTY (this will clear the buffer)
				h.recreateSession(session)

				// If recreation was successful, reset failure tracking
				if session.PTY != nil {
					h.resetRecreationFailures(session)
				}

				// Continue reading from new PTY
				continue
			}
			// Handle timeout errors gracefully without terminating the session
			if strings.Contains(err.Error(), "i/o timeout") {
				logger.Debugf("⏰ PTY read timeout for session %s, continuing...", session.ID)
				continue
			}
			logger.Errorf("❌ Unexpected PTY read error in continuous reader: %v", err)
			return
		}

		// PTY output alone doesn't indicate Claude activity - rely on hooks and JSONL activity instead

		var outputData []byte

		// Extract title from PTY data for Claude sessions
		if session.Agent == "claude" {
			if title, ok := extractTitleFromEscapeSequence(buf[:n]); ok {
				h.handleTitleUpdate(session, title)
			}
		}

		// Process terminal output based on session type
		if session.Agent != "claude" {
			// Non-Claude sessions use traditional buffering approach with port detection
			session.bufferMutex.Lock()

			if title, ok := extractTitleFromEscapeSequence(buf[:n]); ok {
				h.handleTitleUpdate(session, title)
			}

			// Check for localhost:XXXX patterns in terminal output and register ports
			outputData = h.processTerminalOutput(buf[:n], session)

			// Check for alternate screen buffer sequences (use original data for this)
			if bytes.Contains(buf[:n], []byte("\x1b[?1049h")) {
				// Entering alternate screen - mark position for TUI buffer filtering
				session.AlternateScreenActive = true
				session.LastNonTUIBufferSize = len(session.outputBuffer)
				logger.Infof("🖥️  Detected alternate screen buffer entry at position %d", session.LastNonTUIBufferSize)
			}
			if bytes.Contains(buf[:n], []byte("\x1b[?1049l")) {
				// Exiting alternate screen
				session.AlternateScreenActive = false
				logger.Infof("🖥️  Detected alternate screen buffer exit")
			}

			session.outputBuffer = append(session.outputBuffer, outputData...)
			// Update buffered dimensions to current terminal size
			session.bufferedCols = session.cols
			session.bufferedRows = session.rows

			session.bufferMutex.Unlock()
		} else {
			// Claude sessions: send raw data with no processing or buffering
			outputData = buf[:n]
		}

		// Send to all connections if we have data to send
		if len(outputData) > 0 {
			session.connMutex.RLock()
			connectionCount := len(session.connections)
			session.connMutex.RUnlock()

			logger.Debugf("📤 Broadcasting %d bytes to %d connections in session %s", len(outputData), connectionCount, session.ID)
			session.broadcastToConnections(websocket.BinaryMessage, outputData)
		}
	}
}

func (h *PTYHandler) resizePTY(ptmx *os.File, cols, rows uint16) error {
	ws := &struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}{
		Row: rows,
		Col: cols,
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		ptmx.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(ws)),
	)

	if errno != 0 {
		return errno
	}
	return nil
}

// hasFocusedConnection checks if any connection in the session is currently focused
// This function expects the session.connMutex to already be held by the caller
func (h *PTYHandler) hasFocusedConnection(session *Session) bool {
	focusedCount := 0
	totalConnections := len(session.connections)

	for _, connInfo := range session.connections {
		if connInfo.IsFocused {
			focusedCount++
		}
	}

	// Add debug logging to understand focus state
	if totalConnections > 0 {
		logger.Debugf("🔍 Focus check for %s: %d/%d connections focused", session.ID, focusedCount, totalConnections)
	}

	return focusedCount > 0
}

func (h *PTYHandler) monitorSession(session *Session) {
	// Monitor session and clean up when idle
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		session.connMutex.RLock()
		connectionCount := len(session.connections)
		hasFocusedConnection := h.hasFocusedConnection(session)
		session.connMutex.RUnlock()

		// Use Claude activity-based cleanup logic for Claude sessions
		if session.Agent == "claude" {
			// NEVER cleanup Claude sessions that have focused connections
			if hasFocusedConnection {
				logger.Infof("🎯 Claude session has focused connection, keeping PTY alive: %s", session.ID)
				continue
			}

			// Check Claude session log modification time instead of just connection status
			claudeLogModTime := h.getClaudeSessionLogModTime(session.WorkDir)

			// If there are active connections (even unfocused), be more lenient with timeout
			var timeoutDuration time.Duration
			if connectionCount > 0 {
				// With active connections: give 15 minutes grace period for background tabs
				timeoutDuration = 15 * time.Minute
				if connectionCount > 0 && !hasFocusedConnection {
					logger.Debugf("⚠️ Claude session %s has %d unfocused connections, using extended timeout", session.ID, connectionCount)
				}
			} else {
				// No connections: use standard 5 minute timeout
				timeoutDuration = 5 * time.Minute
			}

			// If Claude log was modified recently (within timeout), keep session alive
			if !claudeLogModTime.IsZero() && time.Since(claudeLogModTime) <= timeoutDuration {
				logger.Infof("🤖 Claude session has recent activity (log modified %v ago), keeping PTY alive: %s", time.Since(claudeLogModTime), session.ID)
				continue
			}

			claudeActivityState := h.sessionService.GetClaudeActivityState(session.WorkDir)

			// Cleanup logic based on Claude activity state:
			// 1. ClaudeInactive: No PTY session or very old activity - cleanup immediately (unless focused)
			// 2. ClaudeRunning: PTY exists but no recent activity - this state means it's been
			//    inactive for more than 2 minutes already, so we can cleanup after it's been
			//    in this state for 5 minutes to give some buffer time (unless focused)
			// 3. ClaudeActive: Recent activity - keep alive regardless of WebSocket connections
			switch claudeActivityState {
			case models.ClaudeInactive:
				logger.Infof("🧹 Claude session inactive, cleaning up PTY session: %s", session.ID)
				h.cleanupSession(session)
				return
			case models.ClaudeRunning:
				// The "running" state means PTY exists but no activity in the last 2+ minutes.
				// However, the Claude process itself might still be active. We should be very
				// conservative about cleaning up running Claude sessions to avoid killing
				// active processes when users switch between views.

				// Only cleanup if no connections AND it's been inactive for a very long time
				if connectionCount == 0 && time.Since(session.LastAccess) > 10*time.Minute {
					logger.Infof("🧹 Claude session running with no connections for >10min, cleaning up PTY session: %s", session.ID)
					h.cleanupSession(session)
					return
				}
			case models.ClaudeActive:
				// For "active" state, keep the session alive regardless of WebSocket connections
			}
		} else {
			// For non-Claude sessions, use the old logic (cleanup after 10 minutes with no connections)
			if connectionCount == 0 && time.Since(session.LastAccess) > 10*time.Minute {
				h.cleanupSession(session)
				return
			}
		}
	}
}

func (h *PTYHandler) monitorCheckpoints(session *Session) {
	logger.Debugf("🔍 Starting checkpoint monitoring for session %s", session.ID)
	// Monitor session for checkpoint opportunities
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Check if we have a title set and if checkpoint is needed
		if session.Title != "" {
			shouldCreate := session.checkpointManager.ShouldCreateCheckpoint()

			if shouldCreate {
				err := session.checkpointManager.CreateCheckpoint(session.Title)
				if err != nil {
					logger.Infof("⚠️  Failed to create checkpoint for session %s: %v", session.ID, err)
				}
			}
		}

		// If no connections, stop monitoring
		session.connMutex.RLock()
		connectionCount := len(session.connections)
		session.connMutex.RUnlock()

		if connectionCount == 0 {
			logger.Infof("🔍 No connections for session %s, stopping checkpoint monitoring", session.ID)
			return
		}
	}
}

func (h *PTYHandler) createCommand(sessionID, agent, workDir, resumeSessionID string, useContinue bool, ports *services.SessionPorts) *exec.Cmd {
	var cmd *exec.Cmd

	// Get port environment variables
	portEnvVars, err := h.portService.GetEnvironmentVariables(sessionID)
	if err != nil {
		logger.Infof("⚠️  Failed to get port environment variables for session %s: %v", sessionID, err)
		portEnvVars = []string{} // fallback to empty
	}

	switch agent {
	case "claude":
		// Build Claude command with optional continue or resume flag
		args := []string{"--dangerously-skip-permissions"}

		// Note: External workspace read-only mode is handled at the WebSocket/PTY level,
		// not via Claude command flags. See Session.IsReadOnlyWorkspace field.

		if useContinue {
			args = append(args, "--continue")
			logger.Infof("🔄 Starting Claude Code with --continue for session: %s", sessionID)
		} else if resumeSessionID != "" {
			args = append(args, "--resume", resumeSessionID)
			logger.Infof("🔄 Starting Claude Code with resume for session: %s (resuming: %s)", sessionID, resumeSessionID)
		} else {
			logger.Debugf("🤖 Starting new Claude Code session: %s", sessionID)
		}

		// Find claude executable using robust path lookup
		claudePath := h.findClaudeExecutable()
		cmd = exec.Command(claudePath, args...)
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("SESSION_ID=%s", sessionID),
			"HOME="+config.Runtime.HomeDir,
			"TERM=xterm-direct",
			"COLORTERM=truecolor",
		)
		// Add port environment variables
		cmd.Env = append(cmd.Env, portEnvVars...)
	case "setup":
		// For setup sessions, run bash that cats the setup log file
		// Replace slashes in sessionID with underscores for valid filename
		safeSessionID := strings.ReplaceAll(sessionID, "/", "_")
		safeSessionID = strings.ReplaceAll(safeSessionID, ":", "_")
		setupLogPath := fmt.Sprintf("/tmp/%s.log", safeSessionID)
		cmd = exec.Command("bash", "-c", fmt.Sprintf("cat '%s' 2>/dev/null || echo 'Setup log not found or setup not yet completed.'", setupLogPath))
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("SESSION_ID=%s", sessionID),
			"TERM=xterm-direct",
			"COLORTERM=truecolor",
		)
		logger.Infof("🔧 Setup session - will cat setup log file: %s", setupLogPath)
	default:
		// Default bash shell
		cmd = exec.Command("bash", "--login")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("SESSION_ID=%s", sessionID),
			"HOME="+config.Runtime.HomeDir,
			"TERM=xterm-direct",
			"COLORTERM=truecolor",
		)
		// Add port environment variables
		cmd.Env = append(cmd.Env, portEnvVars...)
		logger.Infof("🐚 Starting bash shell for session: %s", sessionID)
	}
	if cmd != nil {
		cmd.Dir = workDir
	}
	return cmd
}

// extractWorkspaceFromSessionID extracts the workspace name from session ID (e.g., "catnip/zigzag:claude" -> "catnip/zigzag")
func extractWorkspaceFromSessionID(sessionID string) string {
	parts := strings.Split(sessionID, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return sessionID
}

// workspaceExists checks if a workspace directory exists
func (h *PTYHandler) workspaceExists(workspaceID string) bool {
	if workspaceID == "default" {
		// For default workspace, check the current symlink
		currentSymlinkPath := filepath.Join(config.Runtime.WorkspaceDir, "current")
		if target, err := os.Readlink(currentSymlinkPath); err == nil {
			if info, err := os.Stat(target); err == nil && info.IsDir() {
				return true
			}
		}
		return false
	} else if strings.Contains(workspaceID, "/") {
		// Check if workspace ID is in repo/branch format (e.g., "catnip/zigzag")
		parts := strings.SplitN(workspaceID, "/", 2)
		if len(parts) == 2 {
			repoName := parts[0]
			branchName := parts[1]
			branchWorktreePath := filepath.Join(config.Runtime.WorkspaceDir, repoName, branchName)
			if info, err := os.Stat(branchWorktreePath); err == nil && info.IsDir() {
				return true
			}
		}
		return false
	} else {
		// Single name workspace - check if directory exists
		workspaceDir := filepath.Join(config.Runtime.WorkspaceDir, workspaceID)
		if info, err := os.Stat(workspaceDir); err == nil && info.IsDir() {
			return true
		}
		return false
	}
}

// canRecreateSession checks if session recreation is allowed based on rate limiting and backoff
func (h *PTYHandler) canRecreateSession(session *Session) (bool, time.Duration) {
	now := time.Now()
	workspaceID := extractWorkspaceFromSessionID(session.ID)

	// Check if the workspace still exists before attempting recreation
	if !h.workspaceExists(workspaceID) {
		logger.Warnf("🚫 Workspace %s no longer exists - session %s will be cleaned up", workspaceID, session.ID)
		return false, 0 // Don't recreate, cleanup will happen elsewhere
	}

	// Basic rate limiting - once per second
	if now.Sub(session.LastRecreation) < time.Second {
		return false, time.Second - now.Sub(session.LastRecreation)
	}

	// Check workspace-level failure tracking for exponential backoff
	h.failureMutex.RLock()
	tracker, exists := h.failureTracker[workspaceID]
	h.failureMutex.RUnlock()

	if exists && now.Before(tracker.BackoffUntil) {
		return false, tracker.BackoffUntil.Sub(now)
	}

	return true, 0
}

// trackRecreationFailure tracks failed recreation attempts per workspace with aggressive circuit breaking
func (h *PTYHandler) trackRecreationFailure(session *Session) {
	now := time.Now()
	workspaceID := extractWorkspaceFromSessionID(session.ID)

	h.failureMutex.Lock()
	defer h.failureMutex.Unlock()

	tracker, exists := h.failureTracker[workspaceID]
	if !exists {
		tracker = &WorkspaceFailureTracker{
			FirstFailureAt: now,
		}
		h.failureTracker[workspaceID] = tracker
	}

	tracker.FailureCount++
	tracker.LastFailureAt = now

	// Aggressive circuit breaker logic to prevent cascade failures
	if tracker.FailureCount >= 3 {
		// Calculate exponential backoff based on failure count
		// Start with 30 seconds, then 2 minutes, 5 minutes, 10 minutes, then 30 minutes max
		var backoffDuration time.Duration
		switch {
		case tracker.FailureCount >= 10:
			backoffDuration = 30 * time.Minute // Long circuit breaker for severe failures
		case tracker.FailureCount >= 7:
			backoffDuration = 10 * time.Minute
		case tracker.FailureCount >= 5:
			backoffDuration = 5 * time.Minute
		case tracker.FailureCount >= 4:
			backoffDuration = 2 * time.Minute
		default:
			backoffDuration = 30 * time.Second
		}

		tracker.BackoffUntil = now.Add(backoffDuration)
		logger.Warnf("🚫 CIRCUIT BREAKER: Workspace %s has %d consecutive failures - backing off for %v",
			workspaceID, tracker.FailureCount, backoffDuration)

		// If failures are happening very rapidly (>5 failures in under 30 seconds), be extra aggressive
		if tracker.FailureCount >= 5 && now.Sub(tracker.FirstFailureAt) <= 30*time.Second {
			tracker.BackoffUntil = now.Add(15 * time.Minute) // 15 minute circuit breaker
			logger.Errorf("🚨 EMERGENCY CIRCUIT BREAKER: Workspace %s had %d failures in %v - emergency 15 minute backoff",
				workspaceID, tracker.FailureCount, now.Sub(tracker.FirstFailureAt))
		}
	}
}

// resetRecreationFailures resets failure tracking for successful recreations
func (h *PTYHandler) resetRecreationFailures(session *Session) {
	workspaceID := extractWorkspaceFromSessionID(session.ID)

	h.failureMutex.Lock()
	defer h.failureMutex.Unlock()

	delete(h.failureTracker, workspaceID)
}

// periodicWorkspaceCleanup runs every 5 minutes to clean up sessions for non-existent workspaces
func (h *PTYHandler) periodicWorkspaceCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.sessionMutex.RLock()
		var sessionsToCleanup []*Session

		for _, session := range h.sessions {
			workspaceID := extractWorkspaceFromSessionID(session.ID)
			if !h.workspaceExists(workspaceID) {
				sessionsToCleanup = append(sessionsToCleanup, session)
			}
		}
		h.sessionMutex.RUnlock()

		// Clean up sessions for non-existent workspaces
		for _, session := range sessionsToCleanup {
			workspaceID := extractWorkspaceFromSessionID(session.ID)
			logger.Warnf("🧹 Periodic cleanup: removing session %s for non-existent workspace %s", session.ID, workspaceID)
			h.cleanupSession(session)
		}

		// Also clean up failure tracking for non-existent workspaces
		h.failureMutex.Lock()
		var trackersToDelete []string
		for workspaceID := range h.failureTracker {
			if !h.workspaceExists(workspaceID) {
				trackersToDelete = append(trackersToDelete, workspaceID)
			}
		}
		for _, workspaceID := range trackersToDelete {
			delete(h.failureTracker, workspaceID)
			logger.Debugf("🧹 Cleaned up failure tracker for non-existent workspace: %s", workspaceID)
		}
		h.failureMutex.Unlock()
	}
}

func (h *PTYHandler) recreateSession(session *Session) {
	logger.Infof("🔄 Recreating PTY for session: %s", session.ID)

	// Ensure recreation flag will be cleared even if recreation fails
	defer func() {
		session.recreationMutex.Lock()
		session.recreationInProgress = false
		session.recreationMutex.Unlock()
	}()

	// Close old PTY
	if session.PTY != nil {
		session.PTY.Close()
	}

	// Terminate old process
	if session.Cmd != nil && session.Cmd.Process != nil {
		_ = session.Cmd.Process.Kill()
		_ = session.Cmd.Wait()
	}

	// Close all WebSocket connections to force frontend reconnection and terminal clear
	session.connMutex.Lock()
	connectionCount := len(session.connections)
	if connectionCount > 0 {
		logger.Infof("🔌 Closing %d WebSocket connections to force frontend reconnection", connectionCount)
		for conn := range session.connections {
			if err := conn.Close(); err != nil {
				logger.Warnf("❌ Error closing WebSocket connection during recreation: %v", err)
			}
		}
		// Clear the connections map
		session.connections = make(map[*websocket.Conn]*ConnectionInfo)
	}
	session.connMutex.Unlock()

	// For Claude recreations, use --continue which handles session resumption automatically
	var resumeSessionID string

	// Get ports for this session (should already be allocated)
	ports, exists := h.portService.GetPortsForSession(session.ID)
	if !exists {
		logger.Infof("⚠️  No ports found for session %s during recreation, reallocating", session.ID)
		var err error
		ports, err = h.portService.AllocatePortsForSession(session.ID)
		if err != nil {
			logger.Errorf("❌ Failed to allocate ports for session %s during recreation: %v", session.ID, err)
			return
		}
	}

	// Create new command using the same agent (use --continue for Claude recreations only if session data exists)
	useContinue := false
	if session.Agent == "claude" {
		// Only use --continue if there's existing session data
		if existingState, err := h.sessionService.FindSessionByDirectory(session.WorkDir); err == nil && existingState != nil {
			useContinue = true
			logger.Infof("🔄 Found existing Claude session in %s during recreation, will use --continue", session.WorkDir)
		} else {
			logger.Infof("🔄 No existing Claude session found in %s during recreation, starting fresh", session.WorkDir)
		}
	}
	cmd := h.createCommand(session.ID, session.Agent, session.WorkDir, resumeSessionID, useContinue, ports)

	// Start new PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		logger.Errorf("❌ Failed to recreate PTY: %v", err)
		return
	}

	// Stop the old continuous reader
	session.safeClosePTYReadDone()

	// Update session with new PTY and command
	session.PTY = ptmx
	session.Cmd = cmd

	// Create new done channel for the new PTY reader
	session.ptyReadMutex.Lock()
	session.ptyReadDone = make(chan struct{})
	session.ptyReadClosed = false
	session.ptyReadMutex.Unlock()

	// Clear the output buffer on shell restart - no history between restarts
	// (only for non-Claude sessions since Claude sessions don't buffer)
	session.bufferMutex.Lock()
	if session.Agent != "claude" {
		session.outputBuffer = make([]byte, 0)
	}
	// Reset alternate screen buffer detection state
	session.AlternateScreenActive = false
	session.LastNonTUIBufferSize = 0
	session.bufferMutex.Unlock()

	// Preserve the current title from the active session if it exists
	if session.Agent == "claude" {
		if activeSession, exists := h.sessionService.GetActiveSession(session.WorkDir); exists {
			if activeSession.Title != nil && activeSession.Title.Title != "" {
				session.Title = activeSession.Title.Title
				logger.Infof("🔄 Preserved existing title after PTY recreation: %q", session.Title)
			}
		}
	}

	// Resize to match previous size
	_ = h.resizePTY(ptmx, session.cols, session.rows)

	// Restart the continuous PTY reader for the new PTY
	go h.readPTYContinuously(session)

	logger.Infof("✅ PTY recreated successfully for session: %s", session.ID)
}

func (h *PTYHandler) cleanupSession(session *Session) {
	h.sessionMutex.Lock()
	defer h.sessionMutex.Unlock()

	logger.Infof("🧹 Cleaning up idle session: %s", session.ID)

	// Stop the continuous PTY reader
	session.safeClosePTYReadDone()

	// Perform final git add to catch any uncommitted changes before cleanup
	if h.gitService != nil {
		logger.Infof("🔄 Performing final git add for any uncommitted changes in: %s", session.WorkDir)
		if err := h.performFinalGitAdd(session.WorkDir); err != nil {
			logger.Infof("⚠️  Final git add during cleanup failed (this is often expected): %v", err)
		}
	}

	// End session tracking if it's a claude session
	if session.Agent == "claude" {
		if err := h.sessionService.EndActiveSession(session.WorkDir); err != nil {
			logger.Infof("⚠️  Failed to end session tracking for %s: %v", session.WorkDir, err)
		}
	}

	// Close PTY
	if session.PTY != nil {
		session.PTY.Close()
	}

	// Terminate process
	if session.Cmd != nil && session.Cmd.Process != nil {
		_ = session.Cmd.Process.Kill()
		_ = session.Cmd.Wait()
	}

	// Release ports for this session
	if err := h.portService.ReleasePortsForSession(session.ID); err != nil {
		logger.Infof("⚠️  Failed to release ports for session %s: %v", session.ID, err)
	} else {
		logger.Infof("🔗 Released ports for session: %s", session.ID)
	}

	// Remove from sessions map
	delete(h.sessions, session.ID)
}

// NOTE: cleanupStaleConnections function was removed and replaced with nuclear approach
// that force-closes ALL existing connections when a new one registers.

// performFinalGitAdd stages any uncommitted changes during cleanup
func (h *PTYHandler) performFinalGitAdd(workspaceDir string) error {
	// Check if it's a git repository by trying to run git status
	cmd := exec.Command("git", "-C", workspaceDir, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		// Not a git repository, skip silently
		return nil
	}

	// Stage all changes
	addCmd := exec.Command("git", "-C", workspaceDir, "add", ".")
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %v, output: %s", err, string(output))
	}

	logger.Infof("✅ Staged any remaining changes in: %s", workspaceDir)
	return nil
}

// sanitizeSessionID cleans the session ID to prevent path traversal attacks
// It allows slashes for repo/branch format but removes dangerous patterns
func (h *PTYHandler) sanitizeSessionID(sessionID string) string {
	// Remove any path traversal attempts
	sessionID = strings.ReplaceAll(sessionID, "..", "")
	sessionID = strings.ReplaceAll(sessionID, "~/", "")
	sessionID = strings.ReplaceAll(sessionID, "~", "")

	// Remove any absolute path attempts
	sessionID = strings.TrimPrefix(sessionID, "/")

	// Remove any null bytes or other dangerous characters
	sessionID = strings.ReplaceAll(sessionID, "\x00", "")
	sessionID = strings.ReplaceAll(sessionID, "\n", "")
	sessionID = strings.ReplaceAll(sessionID, "\r", "")

	// Limit the session ID length to prevent DOS
	if len(sessionID) > 100 {
		sessionID = sessionID[:100]
	}

	// Ensure it's not empty after sanitization
	if sessionID == "" {
		sessionID = "default"
	}

	return sessionID
}

// broadcastToConnections safely sends data to all websocket connections
func (s *Session) broadcastToConnections(messageType int, data []byte) {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	s.connMutex.RLock()
	connectionCount := len(s.connections)
	s.connMutex.RUnlock()

	// Only broadcast if we have connections and avoid excessive logging for small data
	if connectionCount == 0 {
		return
	}

	// Log broadcasts for debugging (only for multiple connections or very large data)
	if connectionCount > 1 || len(data) > 10000 {
		logger.Debugf("📤 Broadcasting %d bytes to %d connections in session %s", len(data), connectionCount, s.ID)
	}

	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	var disconnectedConns []*websocket.Conn

	for conn, connInfo := range s.connections {
		if err := conn.WriteMessage(messageType, data); err != nil {
			logger.Warnf("❌ WebSocket write error for connection [%s] in session %s: %v", connInfo.ConnID, s.ID, err)
			// Mark connection for removal
			disconnectedConns = append(disconnectedConns, conn)
		}
	}

	// Remove disconnected connections
	if len(disconnectedConns) > 0 {
		// Need to upgrade to write lock to modify connections map
		s.connMutex.RUnlock()
		s.connMutex.Lock()
		for _, conn := range disconnectedConns {
			delete(s.connections, conn)
			conn.Close()
		}
		s.connMutex.Unlock()
		s.connMutex.RLock()
	}
}

// writeToConnection safely writes to a single websocket connection
func (s *Session) writeToConnection(conn *websocket.Conn, messageType int, data []byte) error {
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	// Check if connection is still in our connections map
	s.connMutex.RLock()
	_, exists := s.connections[conn]
	s.connMutex.RUnlock()

	if !exists {
		return fmt.Errorf("connection no longer exists")
	}

	return conn.WriteMessage(messageType, data)
}

// saveSessionState persists session state to disk
func (h *PTYHandler) saveSessionState(session *Session) {
	state := &services.SessionState{
		ID:               session.ID,
		WorkingDirectory: session.WorkDir,
		Agent:            session.Agent,
		ClaudeSessionID:  session.ClaudeSessionID,
		CreatedAt:        session.CreatedAt,
		LastAccess:       session.LastAccess,
		Environment:      make(map[string]string),
	}

	if err := h.sessionService.SaveSessionState(state); err != nil {
		logger.Infof("⚠️  Failed to save session state for %s: %v", session.ID, err)
	} else {
		logger.Debugf("💾 Saved session state for %s", session.ID)
	}
}

// getClaudeSessionTimeout returns the Claude session monitoring timeout from environment or default
func getClaudeSessionTimeout() time.Duration {
	if timeoutStr := os.Getenv("CATNIP_CLAUDE_SESSION_TIMEOUT_SECONDS"); timeoutStr != "" {
		if timeout, err := strconv.Atoi(timeoutStr); err == nil && timeout > 0 {
			return time.Duration(timeout) * time.Second
		}
	}
	return 120 * time.Second // Default: Give Claude 2 minutes to create session file
}

// monitorClaudeSession monitors .claude/projects directory for new session files
func (h *PTYHandler) monitorClaudeSession(session *Session) {
	logger.Debugf("👀 Starting Claude session monitoring for %s in %s", session.ID, session.WorkDir)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	timeout := getClaudeSessionTimeout()

	claudeProjectsDir := filepath.Join(session.WorkDir, ".claude", "projects")

	for range ticker.C {
		if time.Since(startTime) > timeout {
			logger.Infof("⏰ Claude session monitoring timeout for %s", session.ID)
			return
		}

		// Find newest JSONL file in .claude/projects
		sessionID := h.findNewestClaudeSession(claudeProjectsDir)
		if sessionID != "" && sessionID != session.ClaudeSessionID {
			logger.Infof("🎯 Detected Claude session ID: %s for PTY session: %s", sessionID, session.ID)
			session.ClaudeSessionID = sessionID

			// Update active sessions service with real Claude session UUID
			if _, err := h.sessionService.StartOrResumeActiveSession(session.WorkDir, sessionID); err != nil {
				logger.Infof("⚠️  Failed to update active session with Claude UUID: %v", err)
			}

			// Update persisted state
			go h.saveSessionState(session)
			return
		}
	}
}

// getClaudeSessionLogModTime returns the modification time of the most recently modified Claude session log
func (h *PTYHandler) getClaudeSessionLogModTime(workDir string) time.Time {
	homeDir := config.Runtime.HomeDir

	// Transform workDir path to Claude projects directory format
	transformedPath := strings.ReplaceAll(workDir, "/", "-")
	transformedPath = strings.TrimPrefix(transformedPath, "-")
	transformedPath = "-" + transformedPath // Add back the leading dash

	claudeProjectsDir := filepath.Join(homeDir, ".claude", "projects", transformedPath)

	// Check if .claude/projects directory exists
	if _, err := os.Stat(claudeProjectsDir); os.IsNotExist(err) {
		return time.Time{}
	}

	files, err := os.ReadDir(claudeProjectsDir)
	if err != nil {
		return time.Time{}
	}

	var newestTime time.Time

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		// Extract session ID from filename (remove .jsonl extension)
		sessionID := strings.TrimSuffix(file.Name(), ".jsonl")

		// Validate that it looks like a UUID
		if len(sessionID) != 36 || strings.Count(sessionID, "-") != 4 {
			continue
		}

		// Get file modification time
		filePath := filepath.Join(claudeProjectsDir, file.Name())
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Track the newest modification time
		if fileInfo.ModTime().After(newestTime) {
			newestTime = fileInfo.ModTime()
		}
	}

	return newestTime
}

// findNewestClaudeSession finds the newest JSONL file in .claude/projects directory
func (h *PTYHandler) findNewestClaudeSession(claudeProjectsDir string) string {
	// Check if .claude/projects directory exists
	if _, err := os.Stat(claudeProjectsDir); os.IsNotExist(err) {
		return ""
	}

	files, err := os.ReadDir(claudeProjectsDir)
	if err != nil {
		logger.Infof("⚠️  Failed to read .claude/projects directory: %v", err)
		return ""
	}

	var newestFile string
	var newestTime time.Time

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		// Extract session ID from filename (remove .jsonl extension)
		sessionID := strings.TrimSuffix(file.Name(), ".jsonl")

		// Validate that it looks like a UUID
		if len(sessionID) != 36 || strings.Count(sessionID, "-") != 4 {
			continue
		}

		// Get file modification time
		filePath := filepath.Join(claudeProjectsDir, file.Name())
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Track the newest file
		if fileInfo.ModTime().After(newestTime) {
			newestTime = fileInfo.ModTime()
			newestFile = sessionID
		}
	}

	return newestFile
}

// handleTitleUpdate processes a new terminal title, committing previous work and updating session state
func (h *PTYHandler) handleTitleUpdate(session *Session, title string) {
	logger.Infof("🪧 New terminal title detected: %q", title)

	// Get the previous title before updating
	previousTitle := h.sessionService.GetPreviousTitle(session.WorkDir)

	// Only commit if we have a previous title (new title marks start of new work)
	if previousTitle != "" {
		h.commitPreviousWork(session, previousTitle)
	}

	// Update session service with the new title (no commit hash yet)
	if err := h.sessionService.UpdateSessionTitle(session.WorkDir, title, ""); err != nil {
		logger.Infof("⚠️  Failed to update session title: %v", err)
	}

	// Notify Claude monitor service directly (fallback for when log monitoring fails)
	if h.claudeMonitor != nil {
		h.claudeMonitor.NotifyTitleChange(session.WorkDir, title)
	}

	// Update the session's current title for display
	session.Title = title

	// Reset checkpoint state for new title
	session.checkpointManager.Reset()
}

// commitPreviousWork commits the previous work with the given title and updates the commit hash
func (h *PTYHandler) commitPreviousWork(session *Session, previousTitle string) {
	if h.gitService == nil {
		logger.Infof("⚠️  GitService is nil, skipping git operations")
		return
	}

	commitHash, err := h.gitService.GitAddCommitGetHash(session.WorkDir, previousTitle)
	if err != nil {
		logger.Infof("⚠️  Git operations failed for previous title '%s': %v", previousTitle, err)
		return
	}

	if commitHash == "" {
		return
	}

	logger.Infof("✅ Committed previous work with title: %q (hash: %s)", previousTitle, commitHash)

	// Update the previous title's commit hash
	if err := h.sessionService.UpdatePreviousTitleCommitHash(session.WorkDir, commitHash); err != nil {
		logger.Infof("⚠️  Failed to update previous title commit hash: %v", err)
	}

	// Refresh worktree status to update commit count in frontend
	if err := h.gitService.RefreshWorktreeStatus(session.WorkDir); err != nil {
		logger.Infof("⚠️  Failed to refresh worktree status after commit: %v", err)
	}

	// Update last commit time to reset checkpoint timer
	session.checkpointManager.UpdateLastCommitTime()
}

// GetSessionService returns the session service for external access
func (h *PTYHandler) GetSessionService() *services.SessionService {
	return h.sessionService
}

// ExecuteSetupScript checks for and executes setup.sh in a worktree's PTY session
func (h *PTYHandler) ExecuteSetupScript(worktreePath string) {
	// Delegate to PTY service
	h.ptyService.ExecuteSetupScript(worktreePath)
}

// GetPTYService returns the PTY service for external access
func (h *PTYHandler) GetPTYService() *services.PTYService {
	return h.ptyService
}

// promoteConnection promotes a read-only connection to write access and demotes the current write connection
func (h *PTYHandler) promoteConnection(session *Session, requestingConn *websocket.Conn) {
	session.connMutex.Lock()
	defer session.connMutex.Unlock()

	// External workspaces are always read-only - no promotions allowed
	if session.IsReadOnlyWorkspace {
		logger.Infof("🚫 Connection promotion denied for external workspace (read-only): %s", session.ID)
		return
	}

	requestingConnInfo, exists := session.connections[requestingConn]
	if !exists {
		logger.Warnf("❌ Requesting connection not found in session connections")
		return
	}

	// Find the current write connection (if any)
	var currentWriteConn *websocket.Conn
	var currentWriteConnInfo *ConnectionInfo
	for conn, connInfo := range session.connections {
		if !connInfo.IsReadOnly {
			currentWriteConn = conn
			currentWriteConnInfo = connInfo
			break
		}
	}

	// If requesting connection is already the write connection, do nothing
	if currentWriteConn == requestingConn {
		logger.Infof("🔄 Connection [%s] is already the write connection", requestingConnInfo.ConnID)
		return
	}

	// If there's a current write connection, demote it to read-only
	if currentWriteConn != nil && currentWriteConnInfo != nil {
		currentWriteConnInfo.IsReadOnly = true
		logger.Infof("🔒 Demoted connection [%s] to read-only mode", currentWriteConnInfo.ConnID)

		// Notify the demoted connection
		readOnlyMsg := struct {
			Type string `json:"type"`
			Data bool   `json:"data"`
		}{
			Type: "read-only",
			Data: true,
		}
		if data, err := json.Marshal(readOnlyMsg); err == nil {
			_ = session.writeToConnection(currentWriteConn, websocket.TextMessage, data)
		}
	}

	// Promote the requesting connection to write access
	requestingConnInfo.IsReadOnly = false
	logger.Infof("✍️ Promoted connection [%s] to write mode", requestingConnInfo.ConnID)

	// Notify the promoted connection
	writeAccessMsg := struct {
		Type string `json:"type"`
		Data bool   `json:"data"`
	}{
		Type: "read-only",
		Data: false,
	}
	if data, err := json.Marshal(writeAccessMsg); err == nil {
		_ = session.writeToConnection(requestingConn, websocket.TextMessage, data)
	}

	logger.Infof("🔄 Connection promotion completed in session %s", session.ID)
}

// processTerminalOutput scans terminal output for localhost:XXXX patterns
// and registers discovered ports.
func (h *PTYHandler) processTerminalOutput(data []byte, session *Session) []byte {
	// Convert to string for pattern matching
	output := string(data)

	// Regex to match localhost:XXXX patterns (various formats)
	// Matches: localhost:3000, http://localhost:3000, https://localhost:3000, etc.
	// More restrictive to avoid false positives from numbers in terminal output
	localhostRegex := regexp.MustCompile(`(?:^|\s|>)((?:https?://)?localhost:(\d{4,5})(?:/[^\s\x1b]*)?)`)

	// Find all matches and register ports
	matches := localhostRegex.FindAllStringSubmatch(output, -1)
	for _, match := range matches {
		if len(match) >= 4 {
			portStr := match[3] // Port is now in group 3 due to outer capture group
			if port, err := strconv.Atoi(portStr); err == nil {
				// Skip port 8080 (our own proxy port)
				if port != 8080 && port >= 1024 && port <= 65535 {
					// Register the port with the port monitor
					h.portMonitor.RegisterPortFromTerminalOutput(port, session.WorkDir)
				}
			}
		}
	}

	// No rewriting; UI/CLI forwarding will handle host access
	return []byte(output)
}

// handleFocusChange handles focus state changes and auto-promotes focused connections
func (h *PTYHandler) handleFocusChange(session *Session, conn *websocket.Conn, focused bool) {
	session.connMutex.Lock()
	defer session.connMutex.Unlock()

	connInfo, exists := session.connections[conn]
	if !exists {
		// This can happen during session recreation when WebSocket connections are in transition
		// Just log at debug level instead of warning and skip the focus update
		logger.Debugf("🔍 Connection not found in session connections for focus change (likely during reconnection)")
		return
	}

	// Update focus state
	connInfo.IsFocused = focused
	connID := connInfo.ConnID

	if focused {
		logger.Infof("🎯 Connection [%s] gained focus in session %s", connID, session.ID)

		// Debug logging for promotion logic
		logger.Debugf("🔍 Focus promotion check - IsReadOnly: %v, IsReadOnlyWorkspace: %v", connInfo.IsReadOnly, session.IsReadOnlyWorkspace)

		// Auto-promote focused connection if it's read-only (but not for external workspaces)
		if connInfo.IsReadOnly && !session.IsReadOnlyWorkspace {
			// Find and demote the current write connection
			var currentWriteConn *websocket.Conn
			var currentWriteConnInfo *ConnectionInfo
			for c, info := range session.connections {
				if !info.IsReadOnly && c != conn {
					currentWriteConn = c
					currentWriteConnInfo = info
					break
				}
			}

			// Demote current write connection
			if currentWriteConn != nil && currentWriteConnInfo != nil {
				currentWriteConnInfo.IsReadOnly = true
				logger.Infof("🔒 Auto-demoted connection [%s] to read-only (focus lost)", currentWriteConnInfo.ConnID)

				// Notify the demoted connection
				readOnlyMsg := struct {
					Type string `json:"type"`
					Data bool   `json:"data"`
				}{
					Type: "read-only",
					Data: true,
				}
				if data, err := json.Marshal(readOnlyMsg); err == nil {
					_ = session.writeToConnection(currentWriteConn, websocket.TextMessage, data)
				}
			}

			// Promote the focused connection
			connInfo.IsReadOnly = false
			logger.Infof("✍️ Auto-promoted focused connection [%s] to write mode", connID)

			// Notify the promoted connection
			writeAccessMsg := struct {
				Type string `json:"type"`
				Data bool   `json:"data"`
			}{
				Type: "read-only",
				Data: false,
			}
			if data, err := json.Marshal(writeAccessMsg); err == nil {
				_ = session.writeToConnection(conn, websocket.TextMessage, data)
			}
		} else {
			logger.Debugf("🔍 Skipping auto-promotion - connection is already write-enabled or external workspace")
		}
	} else {
		logger.Infof("👁️ Connection [%s] lost focus in session %s", connID, session.ID)
	}
}
