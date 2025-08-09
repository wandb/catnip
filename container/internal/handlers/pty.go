package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

	"github.com/creack/pty"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// PTYHandler handles PTY WebSocket connections
type PTYHandler struct {
	sessions       map[string]*Session
	sessionMutex   sync.RWMutex
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

// NewPTYHandler creates a new PTY handler
func NewPTYHandler(gitService *services.GitService, claudeMonitor *services.ClaudeMonitorService, sessionService *services.SessionService, portMonitor *services.PortMonitor) *PTYHandler {
	return &PTYHandler{
		sessions:       make(map[string]*Session),
		gitService:     gitService,
		sessionService: sessionService,
		portService:    services.NewPortAllocationService(),
		portMonitor:    portMonitor, // Use the provided portMonitor instead of creating new one
		ptyService:     services.NewPTYService(),
		claudeMonitor:  claudeMonitor,
	}
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
		log.Printf("📡 New PTY connection [%s] for session: %s with agent: %s (reset: %t)", connID, sessionID, agent, reset)
	} else {
		log.Printf("📡 New PTY connection [%s] for session: %s (reset: %t)", connID, sessionID, reset)
	}

	// Handle reset logic for Claude agent
	if reset && agent == "claude" {
		log.Printf("🔄 Reset requested for Claude session: %s", sessionID)
		// Shutdown any existing PTY session for this sessionID
		h.sessionMutex.Lock()
		if existingSession, exists := h.sessions[sessionID]; exists {
			log.Printf("🛑 Shutting down existing session: %s", sessionID)
			h.cleanupSession(existingSession)
			delete(h.sessions, sessionID)
		}
		h.sessionMutex.Unlock()
	}

	// Get or create session
	session := h.getOrCreateSession(sessionID, agent, reset)
	if session == nil {
		log.Printf("❌ Failed to create session: %s", sessionID)

		// Send error message to client before closing
		errorMsg := struct {
			Type    string `json:"type"`
			Error   string `json:"error"`
			Message string `json:"message"`
			Code    string `json:"code"`
		}{
			Type:    "error",
			Error:   "Worktree not found",
			Message: fmt.Sprintf("The worktree '%s' does not exist", sessionID),
			Code:    "WORKTREE_NOT_FOUND",
		}

		if data, err := json.Marshal(errorMsg); err == nil {
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}

		conn.Close()
		return
	}

	// Add connection to session with read-only logic
	session.connMutex.Lock()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("🔌 New connection [%s] from %s to session %s", connID, remoteAddr, sessionID)

	// Clean up any stale connections from the same client before determining read-only status
	h.cleanupStaleConnections(session, remoteAddr)

	connectionCount := len(session.connections)
	log.Printf("🔍 Connection count for session %s: %d (after cleanup)", sessionID, connectionCount)

	// First connection gets write access, subsequent ones are read-only
	isReadOnly := connectionCount > 0
	if isReadOnly {
		log.Printf("🔒 Setting connection [%s] to read-ONLY mode (existing connections: %d)", connID, connectionCount)
	} else {
		log.Printf("✍️ Setting connection [%s] to WRITE mode (first connection)", connID)
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
		log.Printf("🔗 Added READ-ONLY connection [%s] to session %s (connections: %d → %d)", connID, sessionID, connectionCount, newConnectionCount)

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
		log.Printf("🔗 Added WRITE connection [%s] to session %s (connections: %d → %d)", connID, sessionID, connectionCount, newConnectionCount)

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
			log.Printf("❌ Recovered from panic in PTY connection handler: %v", r)
		}

		close(done) // Signal goroutines to stop
		session.connMutex.Lock()

		// Check if this was a write-enabled connection
		connInfo, exists := session.connections[conn]
		wasWriteConnection := exists && !connInfo.IsReadOnly

		if exists {
			log.Printf("🔌❌ Removing connection [%s] from session %s (was write: %v)", connInfo.ConnID, session.ID, !connInfo.IsReadOnly)
		}

		delete(session.connections, conn)
		connectionCount := len(session.connections)
		log.Printf("🔍 Connection count for session %s: %d (after removal)", session.ID, connectionCount)

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
				log.Printf("🔄 Promoted connection [%s] to WRITE access in session %s", promotedConnID, sessionID)

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
			log.Printf("🔌 WRITE connection [%s] closed for session %s (remaining: %d)", connID, sessionID, connectionCount)
		} else {
			log.Printf("🔌 read-only connection [%s] closed for session %s (remaining: %d)", connID, sessionID, connectionCount)
		}

		// Safe close with error handling
		if err := conn.Close(); err != nil {
			log.Printf("❌ Error closing websocket connection: %v", err)
		}
	}()

	// Start goroutine to read from PTY and send to WebSocket
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ Recovered from panic in PTY read goroutine: %v", r)
			}
		}()

		buf := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
			}

			n, err := session.PTY.Read(buf)
			if err != nil {
				// Check for various exit conditions
				if err == io.EOF || err.Error() == "read /dev/ptmx: input/output error" {
					// For setup sessions, don't recreate - they're meant to exit after showing the log
					if session.Agent == "setup" {
						log.Printf("✅ Setup session completed normally, not recreating: %s", session.ID)
						return
					}

					// Rate limit recreation to prevent CPU pegging
					now := time.Now()
					if now.Sub(session.LastRecreation) < time.Second {
						log.Printf("⏸️ Rate limiting PTY recreation for session %s (last recreation: %v ago)", session.ID, now.Sub(session.LastRecreation))
						time.Sleep(time.Second)
						continue
					}
					session.LastRecreation = now

					log.Printf("🔄 PTY closed (shell exited: %v), creating new session...", err)

					// Create new PTY (this will clear the buffer)
					h.recreateSession(session)

					// Continue reading from new PTY
					continue
				}
				log.Printf("❌ PTY read error: %v", err)
				return
			}

			// Add to buffer (unlimited growth for TUI compatibility)
			session.bufferMutex.Lock()
			if title, ok := extractTitleFromEscapeSequence(buf[:n]); ok {
				h.handleTitleUpdate(session, title)
			}

			// Check for localhost:XXXX patterns in terminal output and rewrite them
			outputData := h.processTerminalOutput(buf[:n], session)

			// Check for alternate screen buffer sequences (use original data for this)
			if bytes.Contains(buf[:n], []byte("\x1b[?1049h")) {
				// Entering alternate screen - mark position for TUI buffer filtering
				session.AlternateScreenActive = true
				session.LastNonTUIBufferSize = len(session.outputBuffer)
				log.Printf("🖥️  Detected alternate screen buffer entry at position %d", session.LastNonTUIBufferSize)
			}
			if bytes.Contains(buf[:n], []byte("\x1b[?1049l")) {
				// Exiting alternate screen
				session.AlternateScreenActive = false
				log.Printf("🖥️  Detected alternate screen buffer exit")
			}

			session.outputBuffer = append(session.outputBuffer, outputData...)
			// Update buffered dimensions to current terminal size
			session.bufferedCols = session.cols
			session.bufferedRows = session.rows
			session.bufferMutex.Unlock()

			// Check if connection is still valid before writing
			select {
			case <-done:
				return
			default:
			}

			// Send to all connections (use the processed output with rewritten URLs)
			session.broadcastToConnections(websocket.BinaryMessage, outputData)
		}
	}()

	// Read from WebSocket and write to PTY
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("❌ WebSocket read error: %v", err)
			break
		}

		// Handle different message types
		if messageType == websocket.TextMessage {
			// Try to parse as JSON for control messages first
			var controlMsg ControlMsg
			if err := json.Unmarshal(data, &controlMsg); err == nil && controlMsg.Type != "" {
				switch controlMsg.Type {
				case "reset":
					log.Printf("🔄 Reset command received for session: %s", sessionID)
					h.recreateSession(session)
					continue
				case "ready":
					log.Printf("🎯 Client ready signal received")

					// Get buffer info
					session.bufferMutex.RLock()
					hasBuffer := len(session.outputBuffer) > 0
					bufferCols := session.bufferedCols
					bufferRows := session.bufferedRows
					session.bufferMutex.RUnlock()

					if hasBuffer && bufferCols > 0 && bufferRows > 0 {
						// First, resize PTY to match buffered dimensions
						log.Printf("📐 Resizing PTY to buffered dimensions %dx%d before replay", bufferCols, bufferRows)
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
							log.Printf("📋 Replaying %d bytes (filtered from %d) - excluding TUI content", len(bufferToReplay), len(session.outputBuffer))
						} else {
							// Replay entire buffer for non-TUI sessions
							bufferToReplay = make([]byte, len(session.outputBuffer))
							copy(bufferToReplay, session.outputBuffer)
							log.Printf("📋 Replaying %d bytes of buffered output at %dx%d", len(bufferToReplay), bufferCols, bufferRows)
						}
						session.bufferMutex.RUnlock()

						if err := session.writeToConnection(conn, websocket.BinaryMessage, bufferToReplay); err != nil {
							log.Printf("❌ Failed to replay buffer: %v", err)
						}

						// If we filtered TUI content, send a refresh signal to trigger TUI repaint
						if session.AlternateScreenActive && session.LastNonTUIBufferSize > 0 {
							go func() {
								time.Sleep(100 * time.Millisecond)
								log.Printf("🔄 Sending Ctrl+L to refresh TUI after filtered buffer replay")
								if _, err := session.PTY.Write([]byte("\x0c")); err != nil {
									log.Printf("❌ Failed to send refresh signal: %v", err)
								}
							}()
						}
					} else {
						log.Printf("📋 No buffer to replay or dimensions not captured")
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
						log.Printf("📝 Injecting prompt into PTY: %q (submit: %v)", controlMsg.Data, controlMsg.Submit)
						if _, err := session.PTY.Write([]byte(controlMsg.Data)); err != nil {
							log.Printf("❌ Failed to write prompt to PTY: %v", err)
						}

						// If submit is true, send a carriage return after a small delay
						// This mimics how a user would type and then press Enter
						if controlMsg.Submit {
							go func() {
								// Small delay to let the TUI process the prompt text
								time.Sleep(100 * time.Millisecond)
								log.Printf("↩️ Sending carriage return (\\r) to execute prompt")
								if _, err := session.PTY.Write([]byte("\r")); err != nil {
									log.Printf("❌ Failed to write carriage return to PTY: %v", err)
								}
							}()
						}
					}
					continue
				case "promote":
					// Handle connection promotion request (swap read/write permissions)
					log.Printf("🔄 Promotion request received from connection [%s] in session %s", connID, sessionID)
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
				session.cols = resizeMsg.Cols
				session.rows = resizeMsg.Rows
				_ = h.resizePTY(session.PTY, resizeMsg.Cols, resizeMsg.Rows)
				log.Printf("📐 Resized PTY to %dx%d", resizeMsg.Cols, resizeMsg.Rows)
				continue
			}
		}

		// Check if this connection has write access
		session.connMutex.RLock()
		connInfo, exists := session.connections[conn]
		session.connMutex.RUnlock()

		if !exists {
			log.Printf("⚠️ Connection [%s] no longer exists in session", connID)
			break
		}

		if connInfo.IsReadOnly {
			log.Printf("🚫 Ignoring input from read-only connection [%s] in session %s", connID, sessionID)
			continue
		}

		// Write data to PTY (only from write-enabled connections)
		if _, err := session.PTY.Write(data); err != nil {
			log.Printf("❌ PTY write error: %v", err)
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
			log.Printf("🔄 Agent changed from %s to %s for session %s, recreating...", session.Agent, agent, sessionID)
			// Update the agent and recreate
			session.Agent = agent
			h.recreateSession(session)
		} else {
			log.Printf("🔄 Reusing existing session %s with agent: %s", sessionID, session.Agent)
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

	// Set workspace directory with validation
	var workDir string

	// Extract base session ID without agent suffix for directory lookups
	baseSessionID := sessionID
	if idx := strings.LastIndex(sessionID, ":"); idx != -1 {
		baseSessionID = sessionID[:idx]
	}

	// Validate session ID and get workspace directory
	// Only allow "default" or existing worktree directories
	if baseSessionID == "default" {
		// Check if current symlink exists in workspace directory
		currentSymlinkPath := filepath.Join(config.Runtime.WorkspaceDir, "current")
		if target, err := os.Readlink(currentSymlinkPath); err == nil {
			// Symlink exists, check if target is valid
			if info, err := os.Stat(target); err == nil && info.IsDir() {
				workDir = target
				log.Printf("📁 Using current workspace symlink for default session: %s", workDir)
			} else {
				log.Printf("❌ Current workspace symlink target is invalid: %s", target)
				return nil
			}
		} else {
			log.Printf("❌ Default session requested but current symlink does not exist at %s", currentSymlinkPath)
			return nil
		}
	} else if strings.Contains(baseSessionID, "/") {
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
					log.Printf("📁 Using Git worktree for session %s: %s", baseSessionID, workDir)
				} else {
					log.Printf("❌ Directory exists but is not a valid git worktree: %s", branchWorktreePath)
					return nil
				}
			} else {
				log.Printf("❌ Worktree directory does not exist: %s", branchWorktreePath)
				return nil
			}
		} else {
			log.Printf("❌ Invalid session format: %s", baseSessionID)
			return nil
		}
	} else {
		// Single name session - check if directory exists
		sessionWorkDir := filepath.Join(config.Runtime.WorkspaceDir, baseSessionID)
		if info, err := os.Stat(sessionWorkDir); err == nil && info.IsDir() {
			workDir = sessionWorkDir
			log.Printf("📁 Using existing workspace directory: %s", workDir)
		} else {
			log.Printf("❌ Workspace directory does not exist: %s", sessionWorkDir)
			return nil
		}
	}

	// workDir should be set at this point or we would have returned nil
	if workDir == "" {
		log.Printf("❌ Failed to determine valid workspace directory for session: %s", baseSessionID)
		return nil
	}

	// Check for existing Claude session in this directory for auto-resume
	var resumeSessionID string
	var useContinue bool
	if agent == "claude" && !reset {
		if existingState, err := h.sessionService.FindSessionByDirectory(workDir); err == nil && existingState != nil {
			// For existing sessions, use --continue instead of --resume
			useContinue = true
			log.Printf("🔄 Found existing Claude session in %s, will use --continue", workDir)
		}
	}

	// Allocate ports for this session
	ports, err := h.portService.AllocatePortsForSession(sessionID)
	if err != nil {
		log.Printf("❌ Failed to allocate ports for session %s: %v", sessionID, err)
		return nil
	}
	log.Printf("🔗 Allocated ports for session %s: PORT=%d, PORTZ=%v", sessionID, ports.PORT, ports.PORTZ)

	// Create command based on agent parameter
	cmd := h.createCommand(sessionID, agent, workDir, resumeSessionID, useContinue, ports)

	var ptmx *os.File

	// Start PTY for all session types including setup
	ptmx, err = pty.Start(cmd)
	if err != nil {
		log.Printf("❌ Failed to start PTY: %v", err)
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
	}

	h.sessions[sessionID] = session
	log.Printf("✅ Created new PTY session: %s in %s", sessionID, workDir)

	// Track active session for this workspace
	if agent == "claude" {
		// Start or resume session tracking - we'll update with actual Claude session UUID later
		if activeSession, err := h.sessionService.StartOrResumeActiveSession(workDir, ""); err != nil {
			log.Printf("⚠️  Failed to start/resume session tracking for %s: %v", workDir, err)
		} else {
			// Inherit the current title from the active session if it exists
			if activeSession.Title != nil && activeSession.Title.Title != "" {
				session.Title = activeSession.Title.Title
				log.Printf("🔄 Inherited existing title from active session: %q", session.Title)
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
				log.Printf("🔄 Triggered immediate Claude activity state sync for %s", session.WorkDir)
			}()
		}
	}

	// Start checkpoint monitoring for all sessions
	go h.monitorCheckpoints(session)

	return session
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

func (h *PTYHandler) monitorSession(session *Session) {
	// Monitor session and clean up when idle
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		session.connMutex.RLock()
		connectionCount := len(session.connections)
		session.connMutex.RUnlock()

		// Use Claude activity-based cleanup logic for Claude sessions
		if session.Agent == "claude" {
			// Check Claude session log modification time instead of just connection status
			claudeLogModTime := h.getClaudeSessionLogModTime(session.WorkDir)

			// If Claude log was modified recently (within 5 minutes), keep session alive
			if !claudeLogModTime.IsZero() && time.Since(claudeLogModTime) <= 5*time.Minute {
				log.Printf("🤖 Claude session has recent activity (log modified %v ago), keeping PTY alive: %s", time.Since(claudeLogModTime), session.ID)
				continue
			}

			claudeActivityState := h.sessionService.GetClaudeActivityState(session.WorkDir)

			// Cleanup logic based on Claude activity state:
			// 1. ClaudeInactive: No PTY session or very old activity - cleanup immediately
			// 2. ClaudeRunning: PTY exists but no recent activity - this state means it's been
			//    inactive for more than 2 minutes already, so we can cleanup after it's been
			//    in this state for 5 minutes to give some buffer time
			// 3. ClaudeActive: Recent activity - keep alive regardless of WebSocket connections
			switch claudeActivityState {
			case models.ClaudeInactive:
				log.Printf("🧹 Claude session inactive, cleaning up PTY session: %s", session.ID)
				h.cleanupSession(session)
				return
			case models.ClaudeRunning:
				// The "running" state means PTY exists but no activity in the last 2+ minutes.
				// However, the Claude process itself might still be active. We should be very
				// conservative about cleaning up running Claude sessions to avoid killing
				// active processes when users switch between views.

				// Only cleanup if no connections AND it's been inactive for a very long time
				if connectionCount == 0 && time.Since(session.LastAccess) > 10*time.Minute {
					log.Printf("🧹 Claude session running with no connections for >10min, cleaning up PTY session: %s", session.ID)
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
	log.Printf("🔍 Starting checkpoint monitoring for session %s", session.ID)
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
					log.Printf("⚠️  Failed to create checkpoint for session %s: %v", session.ID, err)
				}
			}
		}

		// If no connections, stop monitoring
		session.connMutex.RLock()
		connectionCount := len(session.connections)
		session.connMutex.RUnlock()

		if connectionCount == 0 {
			log.Printf("🔍 No connections for session %s, stopping checkpoint monitoring", session.ID)
			return
		}
	}
}

func (h *PTYHandler) createCommand(sessionID, agent, workDir, resumeSessionID string, useContinue bool, ports *services.SessionPorts) *exec.Cmd {
	var cmd *exec.Cmd

	// Get port environment variables
	portEnvVars, err := h.portService.GetEnvironmentVariables(sessionID)
	if err != nil {
		log.Printf("⚠️  Failed to get port environment variables for session %s: %v", sessionID, err)
		portEnvVars = []string{} // fallback to empty
	}

	switch agent {
	case "claude":
		// Build Claude command with optional continue or resume flag
		args := []string{"--dangerously-skip-permissions"}
		if useContinue {
			args = append(args, "--continue")
			log.Printf("🔄 Starting Claude Code with --continue for session: %s", sessionID)
		} else if resumeSessionID != "" {
			args = append(args, "--resume", resumeSessionID)
			log.Printf("🔄 Starting Claude Code with resume for session: %s (resuming: %s)", sessionID, resumeSessionID)
		} else {
			log.Printf("🤖 Starting new Claude Code session: %s", sessionID)
		}

		cmd = exec.Command("claude", args...)
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
		log.Printf("🔧 Setup session - will cat setup log file: %s", setupLogPath)
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
		log.Printf("🐚 Starting bash shell for session: %s", sessionID)
	}
	if cmd != nil {
		cmd.Dir = workDir
	}
	return cmd
}

func (h *PTYHandler) recreateSession(session *Session) {
	log.Printf("🔄 Recreating PTY for session: %s", session.ID)

	// Close old PTY
	if session.PTY != nil {
		session.PTY.Close()
	}

	// Terminate old process
	if session.Cmd != nil && session.Cmd.Process != nil {
		_ = session.Cmd.Process.Kill()
		_ = session.Cmd.Wait()
	}

	// Check for existing Claude session to resume (for recreated sessions)
	var resumeSessionID string
	if session.Agent == "claude" {
		if existingState, err := h.sessionService.FindSessionByDirectory(session.WorkDir); err == nil && existingState != nil {
			resumeSessionID = existingState.ClaudeSessionID
		}
	}

	// Get ports for this session (should already be allocated)
	ports, exists := h.portService.GetPortsForSession(session.ID)
	if !exists {
		log.Printf("⚠️  No ports found for session %s during recreation, reallocating", session.ID)
		var err error
		ports, err = h.portService.AllocatePortsForSession(session.ID)
		if err != nil {
			log.Printf("❌ Failed to allocate ports for session %s during recreation: %v", session.ID, err)
			return
		}
	}

	// Create new command using the same agent (use old resume logic for recreation)
	cmd := h.createCommand(session.ID, session.Agent, session.WorkDir, resumeSessionID, false, ports)

	// Start new PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("❌ Failed to recreate PTY: %v", err)
		return
	}

	// Update session with new PTY and command
	session.PTY = ptmx
	session.Cmd = cmd

	// Clear the output buffer on shell restart - no history between restarts
	session.bufferMutex.Lock()
	session.outputBuffer = make([]byte, 0)
	// Reset alternate screen buffer detection state
	session.AlternateScreenActive = false
	session.LastNonTUIBufferSize = 0
	session.bufferMutex.Unlock()

	// Preserve the current title from the active session if it exists
	if session.Agent == "claude" {
		if activeSession, exists := h.sessionService.GetActiveSession(session.WorkDir); exists {
			if activeSession.Title != nil && activeSession.Title.Title != "" {
				session.Title = activeSession.Title.Title
				log.Printf("🔄 Preserved existing title after PTY recreation: %q", session.Title)
			}
		}
	}

	// Resize to match previous size
	_ = h.resizePTY(ptmx, session.cols, session.rows)

	log.Printf("✅ PTY recreated successfully for session: %s", session.ID)
}

func (h *PTYHandler) cleanupSession(session *Session) {
	h.sessionMutex.Lock()
	defer h.sessionMutex.Unlock()

	log.Printf("🧹 Cleaning up idle session: %s", session.ID)

	// Perform final git add to catch any uncommitted changes before cleanup
	if h.gitService != nil {
		log.Printf("🔄 Performing final git add for any uncommitted changes in: %s", session.WorkDir)
		if err := h.performFinalGitAdd(session.WorkDir); err != nil {
			log.Printf("⚠️  Final git add during cleanup failed (this is often expected): %v", err)
		}
	}

	// End session tracking if it's a claude session
	if session.Agent == "claude" {
		if err := h.sessionService.EndActiveSession(session.WorkDir); err != nil {
			log.Printf("⚠️  Failed to end session tracking for %s: %v", session.WorkDir, err)
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
		log.Printf("⚠️  Failed to release ports for session %s: %v", session.ID, err)
	} else {
		log.Printf("🔗 Released ports for session: %s", session.ID)
	}

	// Remove from sessions map
	delete(h.sessions, session.ID)
}

// cleanupStaleConnections removes stale connections from the same remote address
// This prevents race conditions where old connections haven't been cleaned up yet
func (h *PTYHandler) cleanupStaleConnections(session *Session, remoteAddr string) {
	var staleConnections []*websocket.Conn

	// Find connections from the same remote address that are no longer active
	existingFromSameAddr := 0
	for conn, connInfo := range session.connections {
		if connInfo.RemoteAddr == remoteAddr {
			existingFromSameAddr++
			log.Printf("🔍 Found existing connection [%s] from %s, testing if stale...", connInfo.ConnID, remoteAddr)
			// Test if connection is still active by trying to ping it
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("🧹 Found stale connection [%s] from %s, removing (ping failed: %v)", connInfo.ConnID, remoteAddr, err)
				staleConnections = append(staleConnections, conn)
			} else {
				log.Printf("✅ Connection [%s] from %s is still active", connInfo.ConnID, remoteAddr)
			}
		}
	}

	log.Printf("🔍 Stale cleanup for %s: found %d existing connections, %d are stale", remoteAddr, existingFromSameAddr, len(staleConnections))

	// Remove stale connections
	for _, conn := range staleConnections {
		delete(session.connections, conn)
		conn.Close()
	}

	if len(staleConnections) > 0 {
		log.Printf("🧹 Cleaned up %d stale connections from %s in session %s", len(staleConnections), remoteAddr, session.ID)
	}
}

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

	log.Printf("✅ Staged any remaining changes in: %s", workspaceDir)
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
		log.Printf("📤 Broadcasting %d bytes to %d connections in session %s", len(data), connectionCount, s.ID)
	}

	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	var disconnectedConns []*websocket.Conn

	for conn, connInfo := range s.connections {
		if err := conn.WriteMessage(messageType, data); err != nil {
			log.Printf("❌ WebSocket write error for connection [%s] in session %s: %v", connInfo.ConnID, s.ID, err)
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
		log.Printf("⚠️  Failed to save session state for %s: %v", session.ID, err)
	} else {
		log.Printf("💾 Saved session state for %s", session.ID)
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
	log.Printf("👀 Starting Claude session monitoring for %s in %s", session.ID, session.WorkDir)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	timeout := getClaudeSessionTimeout()

	claudeProjectsDir := filepath.Join(session.WorkDir, ".claude", "projects")

	for range ticker.C {
		if time.Since(startTime) > timeout {
			log.Printf("⏰ Claude session monitoring timeout for %s", session.ID)
			return
		}

		// Find newest JSONL file in .claude/projects
		sessionID := h.findNewestClaudeSession(claudeProjectsDir)
		if sessionID != "" && sessionID != session.ClaudeSessionID {
			log.Printf("🎯 Detected Claude session ID: %s for PTY session: %s", sessionID, session.ID)
			session.ClaudeSessionID = sessionID

			// Update active sessions service with real Claude session UUID
			if _, err := h.sessionService.StartOrResumeActiveSession(session.WorkDir, sessionID); err != nil {
				log.Printf("⚠️  Failed to update active session with Claude UUID: %v", err)
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
		log.Printf("⚠️  Failed to read .claude/projects directory: %v", err)
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
	log.Printf("🪧 New terminal title detected: %q", title)

	// Get the previous title before updating
	previousTitle := h.sessionService.GetPreviousTitle(session.WorkDir)

	// Only commit if we have a previous title (new title marks start of new work)
	if previousTitle != "" {
		h.commitPreviousWork(session, previousTitle)
	}

	// Update session service with the new title (no commit hash yet)
	if err := h.sessionService.UpdateSessionTitle(session.WorkDir, title, ""); err != nil {
		log.Printf("⚠️  Failed to update session title: %v", err)
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
		log.Printf("⚠️  GitService is nil, skipping git operations")
		return
	}

	commitHash, err := h.gitService.GitAddCommitGetHash(session.WorkDir, previousTitle)
	if err != nil {
		log.Printf("⚠️  Git operations failed for previous title '%s': %v", previousTitle, err)
		return
	}

	if commitHash == "" {
		return
	}

	log.Printf("✅ Committed previous work with title: %q (hash: %s)", previousTitle, commitHash)

	// Update the previous title's commit hash
	if err := h.sessionService.UpdatePreviousTitleCommitHash(session.WorkDir, commitHash); err != nil {
		log.Printf("⚠️  Failed to update previous title commit hash: %v", err)
	}

	// Refresh worktree status to update commit count in frontend
	if err := h.gitService.RefreshWorktreeStatus(session.WorkDir); err != nil {
		log.Printf("⚠️  Failed to refresh worktree status after commit: %v", err)
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

	requestingConnInfo, exists := session.connections[requestingConn]
	if !exists {
		log.Printf("❌ Requesting connection not found in session connections")
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
		log.Printf("🔄 Connection [%s] is already the write connection", requestingConnInfo.ConnID)
		return
	}

	// If there's a current write connection, demote it to read-only
	if currentWriteConn != nil && currentWriteConnInfo != nil {
		currentWriteConnInfo.IsReadOnly = true
		log.Printf("🔒 Demoted connection [%s] to read-only mode", currentWriteConnInfo.ConnID)

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
	log.Printf("✍️ Promoted connection [%s] to write mode", requestingConnInfo.ConnID)

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

	log.Printf("🔄 Connection promotion completed in session %s", session.ID)
}

// processTerminalOutput scans terminal output for localhost:XXXX patterns
// and registers discovered ports.
func (h *PTYHandler) processTerminalOutput(data []byte, session *Session) []byte {
	// Convert to string for pattern matching
	output := string(data)

	// Regex to match localhost:XXXX patterns (various formats)
	// Matches: localhost:3000, http://localhost:3000, https://localhost:3000, etc.
	localhostRegex := regexp.MustCompile(`(https?://)?localhost:(\d{4,5})(/[^\s\x1b]*)?`)

	// Find all matches and register ports
	matches := localhostRegex.FindAllStringSubmatch(output, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			portStr := match[2]
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
		log.Printf("❌ Connection not found in session connections for focus change")
		return
	}

	// Update focus state
	connInfo.IsFocused = focused
	connID := connInfo.ConnID

	if focused {
		log.Printf("🎯 Connection [%s] gained focus in session %s", connID, session.ID)

		// Auto-promote focused connection if it's read-only
		if connInfo.IsReadOnly {
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
				log.Printf("🔒 Auto-demoted connection [%s] to read-only (focus lost)", currentWriteConnInfo.ConnID)

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
			log.Printf("✍️ Auto-promoted focused connection [%s] to write mode", connID)

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
		}
	} else {
		log.Printf("👁️ Connection [%s] lost focus in session %s", connID, session.ID)
	}
}
