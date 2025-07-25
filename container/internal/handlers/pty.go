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
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// PTYHandler handles PTY WebSocket connections
type PTYHandler struct {
	sessions       map[string]*Session
	sessionMutex   sync.RWMutex
	gitService     *services.GitService
	sessionService *services.SessionService
	portService    *services.PortAllocationService
	ptyService     *services.PTYService
}

// ConnectionInfo tracks metadata for each WebSocket connection
type ConnectionInfo struct {
	ConnectedAt time.Time
	RemoteAddr  string
	ConnID      string
	IsReadOnly  bool
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
	checkpointManager CheckpointManager
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
func NewPTYHandler(gitService *services.GitService) *PTYHandler {
	return &PTYHandler{
		sessions:       make(map[string]*Session),
		gitService:     gitService,
		sessionService: services.NewSessionService(),
		portService:    services.NewPortAllocationService(),
		ptyService:     services.NewPTYService(),
	}
}

// RegisterRoutes registers all PTY-related routes
func (h *PTYHandler) RegisterRoutes(v1 fiber.Router) {
	v1.Get("/pty", h.HandleWebSocket)
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
		conn.Close()
		return
	}

	// Add connection to session with read-only logic
	session.connMutex.Lock()
	connectionCount := len(session.connections)

	// First connection gets write access, subsequent ones are read-only
	isReadOnly := connectionCount > 0

	session.connections[conn] = &ConnectionInfo{
		ConnectedAt: time.Now(),
		RemoteAddr:  conn.RemoteAddr().String(),
		ConnID:      connID,
		IsReadOnly:  isReadOnly,
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

		delete(session.connections, conn)
		connectionCount := len(session.connections)

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
			session.outputBuffer = append(session.outputBuffer, buf[:n]...)
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

			// Send to all connections
			session.broadcastToConnections(websocket.BinaryMessage, buf[:n])
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

						// Then replay the buffer
						session.bufferMutex.RLock()
						log.Printf("📋 Replaying %d bytes of buffered output at %dx%d", len(session.outputBuffer), bufferCols, bufferRows)
						bufferCopy := make([]byte, len(session.outputBuffer))
						copy(bufferCopy, session.outputBuffer)
						session.bufferMutex.RUnlock()

						if err := session.writeToConnection(conn, websocket.BinaryMessage, bufferCopy); err != nil {
							log.Printf("❌ Failed to replay buffer: %v", err)
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

	// Set workspace directory
	var workDir string

	// Extract base session ID without agent suffix for directory lookups
	baseSessionID := sessionID
	if idx := strings.LastIndex(sessionID, ":"); idx != -1 {
		baseSessionID = sessionID[:idx]
	}

	// Priority order for workspace directory:
	// 1. Session ID "default" maps to /workspace/current symlink if it exists
	// 2. Session ID in repo/branch format maps to Git worktree
	// 3. Active Git worktree (if available and no specific session)
	// 4. Mounted directory at /workspace/{sessionID}
	// 5. Default /workspace

	// Check if session ID is "default" and /workspace/current symlink exists
	if baseSessionID == "default" {
		currentSymlinkPath := filepath.Join("/workspace", "current")
		if target, err := os.Readlink(currentSymlinkPath); err == nil {
			// Symlink exists, check if target is valid
			if info, err := os.Stat(target); err == nil && info.IsDir() {
				workDir = target
				log.Printf("📁 Using current workspace symlink for default session: %s", workDir)
			} else {
				log.Printf("⚠️ /workspace/current symlink target is invalid: %s", target)
			}
		}
	}

	// Check if session ID is in repo/branch format (e.g., "myrepo/main")
	if workDir == "" && strings.Contains(baseSessionID, "/") && h.gitService != nil {
		// This might be a Git worktree session
		// The session format is "repo/branch" which maps to /workspace/repo/branch or /workspace/repo
		parts := strings.SplitN(baseSessionID, "/", 2)
		if len(parts) == 2 {
			repo := parts[0]
			branch := parts[1]

			// Check for worktree at /workspace/repo/branch (our standard pattern)
			branchWorktreePath := filepath.Join("/workspace", repo, branch)
			if info, err := os.Stat(branchWorktreePath); err == nil && info.IsDir() {
				if _, err := os.Stat(filepath.Join(branchWorktreePath, ".git")); err == nil {
					workDir = branchWorktreePath
					log.Printf("📁 Using Git worktree for session %s: %s", baseSessionID, workDir)
				}
			}
		}
	}

	// If not a Git session, check if Git service has a default worktree
	if workDir == "" && h.gitService != nil {
		gitWorkDir := h.gitService.GetDefaultWorktreePath()
		if gitWorkDir != "/workspace" && gitWorkDir != "" {
			workDir = gitWorkDir
			log.Printf("📁 Using default Git worktree directory: %s", workDir)
		}
	}

	// If no Git worktree, check for session-based directory
	if workDir == "" {
		sessionWorkDir := filepath.Join("/workspace", baseSessionID)
		if info, err := os.Stat(sessionWorkDir); err == nil {
			// Directory exists - could be mounted or created
			if info.IsDir() {
				workDir = sessionWorkDir
				log.Printf("📁 Using existing workspace directory: %s", workDir)
			}
		} else if os.IsNotExist(err) {
			// Directory doesn't exist, try to create it
			if err := os.MkdirAll(sessionWorkDir, 0755); err != nil {
				log.Printf("⚠️  Failed to create workspace directory: %v", err)
			} else {
				workDir = sessionWorkDir
				log.Printf("📁 Created workspace directory: %s", workDir)
			}
		}
	}

	// Fallback to default workspace
	if workDir == "" {
		workDir = "/workspace"
		log.Printf("📁 Using default workspace directory: %s", workDir)
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
		checkpointManager: &SessionCheckpointManager{
			lastCommitTime:  time.Now(),
			checkpointCount: 0,
			gitService:      h.gitService,
			sessionService:  h.sessionService,
			workDir:         workDir,
		},
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

		// If no connections and idle for 10 minutes, clean up
		if connectionCount == 0 && time.Since(session.LastAccess) > 10*time.Minute {
			h.cleanupSession(session)
			return
		}
	}
}

func (h *PTYHandler) monitorCheckpoints(session *Session) {
	log.Printf("🔍 Starting checkpoint monitoring for session %s", session.ID)
	// Monitor session for checkpoint opportunities
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		log.Printf("🔍 Checkpoint monitor tick for session %s (Title: %q)", session.ID, session.Title)

		// Check if we have a title set and if checkpoint is needed
		if session.Title != "" {
			shouldCreate := session.checkpointManager.ShouldCreateCheckpoint()
			log.Printf("🔍 Should create checkpoint: %v (title: %q)", shouldCreate, session.Title)

			if shouldCreate {
				log.Printf("📝 Creating checkpoint for session %s with title: %q", session.ID, session.Title)
				err := session.checkpointManager.CreateCheckpoint(session.Title)
				if err != nil {
					log.Printf("⚠️  Failed to create checkpoint for session %s: %v", session.ID, err)
				}
			}
		} else {
			log.Printf("🔍 No title set for session %s, skipping checkpoint", session.ID)
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
			"HOME=/home/catnip",
			"USER=catnip",
			"TERM=xterm-direct",
			"COLORTERM=truecolor",
		)
		// Add port environment variables
		cmd.Env = append(cmd.Env, portEnvVars...)
	case "setup":
		// For setup sessions, run bash that cats the setup log file
		setupLogPath := filepath.Join(workDir, ".catnip", "logs", "setup.log")
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
			"HOME=/home/catnip",
			"USER=catnip",
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

	// Log broadcasts for debugging (limit to prevent spam)
	if len(data) > 100 || connectionCount > 1 {
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

// monitorClaudeSession monitors .claude/projects directory for new session files
func (h *PTYHandler) monitorClaudeSession(session *Session) {
	log.Printf("👀 Starting Claude session monitoring for %s in %s", session.ID, session.WorkDir)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	timeout := 30 * time.Second // Give Claude 30 seconds to create session file

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

	// Update last commit time to reset checkpoint timer
	session.checkpointManager.UpdateLastCommitTime()
}

// GetSessionService returns the session service for external access
func (h *PTYHandler) GetSessionService() *services.SessionService {
	return h.sessionService
}

// CheckpointManager handles checkpoint functionality for sessions
type CheckpointManager interface {
	ShouldCreateCheckpoint() bool
	CreateCheckpoint(title string) error
	Reset()
	UpdateLastCommitTime()
}

// SessionCheckpointManager implements CheckpointManager
type SessionCheckpointManager struct {
	lastCommitTime  time.Time
	checkpointCount int
	checkpointMutex sync.RWMutex
	gitService      *services.GitService
	sessionService  *services.SessionService
	workDir         string
}

// ShouldCreateCheckpoint returns true if a checkpoint should be created
func (cm *SessionCheckpointManager) ShouldCreateCheckpoint() bool {
	cm.checkpointMutex.RLock()
	defer cm.checkpointMutex.RUnlock()
	return time.Since(cm.lastCommitTime) >= 30*time.Second
}

// CreateCheckpoint creates a checkpoint commit
func (cm *SessionCheckpointManager) CreateCheckpoint(title string) error {
	if cm.gitService == nil {
		return fmt.Errorf("git service not available")
	}

	cm.checkpointMutex.Lock()
	defer cm.checkpointMutex.Unlock()

	checkpointTitle := fmt.Sprintf("%s checkpoint: %d", title, cm.checkpointCount+1)
	commitHash, err := cm.gitService.GitAddCommitGetHash(cm.workDir, checkpointTitle)
	if err != nil {
		return err
	} else if commitHash == "" {
		return nil
	}

	cm.checkpointCount++

	log.Printf("✅ Created checkpoint commit: %q (hash: %s)", checkpointTitle, commitHash)

	// Update last commit time
	cm.lastCommitTime = time.Now()

	// Add the checkpoint to session history (without updating the current title)
	if err := cm.sessionService.AddToSessionHistory(cm.workDir, checkpointTitle, commitHash); err != nil {
		log.Printf("⚠️  Failed to add checkpoint to session history: %v", err)
	}

	return nil
}

// Reset resets the checkpoint state for a new title
func (cm *SessionCheckpointManager) Reset() {
	cm.checkpointMutex.Lock()
	defer cm.checkpointMutex.Unlock()
	cm.checkpointCount = 0
	cm.lastCommitTime = time.Now()
}

// UpdateLastCommitTime updates the last commit time
func (cm *SessionCheckpointManager) UpdateLastCommitTime() {
	cm.checkpointMutex.Lock()
	defer cm.checkpointMutex.Unlock()
	cm.lastCommitTime = time.Now()
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
