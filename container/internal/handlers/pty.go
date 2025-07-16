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
}

// Session represents a PTY session
type Session struct {
	ID          string
	PTY         *os.File
	Cmd         *exec.Cmd
	CreatedAt   time.Time
	LastAccess  time.Time
	WorkDir     string
	Agent       string
	Title       string
	ClaudeSessionID string // Track Claude session UUID for resume functionality
	connections map[*websocket.Conn]bool
	connMutex   sync.RWMutex
	// Buffer to store PTY output for replay
	outputBuffer []byte
	bufferMutex  sync.RWMutex
	maxBufferSize int
	// Terminal dimensions
	cols uint16
	rows uint16
	// Buffered dimensions - the size when buffer was captured
	bufferedCols uint16
	bufferedRows uint16
	// WebSocket write protection
	writeMutex sync.Mutex
}

// ResizeMsg represents terminal resize message
type ResizeMsg struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// ControlMsg represents control commands
type ControlMsg struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
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
	return string(title), true
}

// NewPTYHandler creates a new PTY handler
func NewPTYHandler(gitService *services.GitService) *PTYHandler {
	return &PTYHandler{
		sessions:       make(map[string]*Session),
		gitService:     gitService,
		sessionService: services.NewSessionService(),
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
		return websocket.New(func(conn *websocket.Conn) {
			h.handlePTYConnection(conn, sessionID, agent)
		})(c)
	}
	return fiber.ErrUpgradeRequired
}

func (h *PTYHandler) handlePTYConnection(conn *websocket.Conn, sessionID, agent string) {
	if agent != "" {
		log.Printf("📡 New PTY connection for session: %s with agent: %s", sessionID, agent)
	} else {
		log.Printf("📡 New PTY connection for session: %s", sessionID)
	}

	// Get or create session
	session := h.getOrCreateSession(sessionID, agent)
	if session == nil {
		log.Printf("❌ Failed to create session: %s", sessionID)
		conn.Close()
		return
	}
	
	// Add connection to session
	session.connMutex.Lock()
	session.connections[conn] = true
	session.connMutex.Unlock()

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
		delete(session.connections, conn)
		connectionCount := len(session.connections)
		session.connMutex.Unlock()
		
		log.Printf("🔌 PTY connection closed for session %s (remaining: %d)", sessionID, connectionCount)
		
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
				session.Title = title
				log.Printf("🪧 Updated terminal title: %q", title)
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

		// Write data to PTY
		if _, err := session.PTY.Write(data); err != nil {
			log.Printf("❌ PTY write error: %v", err)
			break
		}
		
		// Update last access time
		session.LastAccess = time.Now()
	}
}

func (h *PTYHandler) getOrCreateSession(sessionID, agent string) *Session {
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
	
	// Priority order for workspace directory:
	// 1. Session ID in repo/branch format maps to Git worktree
	// 2. Active Git worktree (if available and no specific session)
	// 3. Mounted directory at /workspace/{sessionID} 
	// 4. Default /workspace
	
	// Check if session ID is in repo/branch format (e.g., "myrepo/main")
	if strings.Contains(sessionID, "/") && h.gitService != nil {
		// This might be a Git worktree session
		// The session format is "repo/branch" which maps to /workspace/repo/branch or /workspace/repo
		parts := strings.SplitN(sessionID, "/", 2)
		if len(parts) == 2 {
			repo := parts[0]
			branch := parts[1]
			
			// Check for worktree at /workspace/repo/branch (our standard pattern)
			branchWorktreePath := filepath.Join("/workspace", repo, branch)
			if info, err := os.Stat(branchWorktreePath); err == nil && info.IsDir() {
				if _, err := os.Stat(filepath.Join(branchWorktreePath, ".git")); err == nil {
					workDir = branchWorktreePath
					log.Printf("📁 Using Git worktree for session %s: %s", sessionID, workDir)
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
		sessionWorkDir := filepath.Join("/workspace", sessionID)
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
	if agent == "claude" {
		if existingState, err := h.sessionService.FindSessionByDirectory(workDir); err == nil && existingState != nil {
			resumeSessionID = existingState.ClaudeSessionID
			log.Printf("🔄 Found existing Claude session in %s, will resume: %s", workDir, resumeSessionID)
		}
	}

	// Create command based on agent parameter
	cmd := h.createCommand(sessionID, agent, workDir, resumeSessionID)

	// Start PTY
	ptmx, err := pty.Start(cmd)
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
		connections:   make(map[*websocket.Conn]bool),
		outputBuffer:  make([]byte, 0),
		maxBufferSize: 5 * 1024 * 1024, // 5MB buffer
		cols:          80,
		rows:          24,
		bufferedCols:  80,
		bufferedRows:  24,
	}

	h.sessions[sessionID] = session
	log.Printf("✅ Created new PTY session: %s in %s", sessionID, workDir)

	// Track active session for this workspace
	if agent == "claude" {
		// Start session tracking - we'll update with actual Claude session UUID later
		if err := h.sessionService.StartActiveSession(workDir, ""); err != nil {
			log.Printf("⚠️  Failed to start session tracking for %s: %v", workDir, err)
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

func (h *PTYHandler) createCommand(sessionID, agent, workDir, resumeSessionID string) *exec.Cmd {
	var cmd *exec.Cmd
	if agent == "claude" {
		// Build Claude command with optional resume flag
		args := []string{"--dangerously-skip-permissions"}
		if resumeSessionID != "" {
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
	} else {
		// Default bash shell
		cmd = exec.Command("bash", "--login")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("SESSION_ID=%s", sessionID),
			"HOME=/home/catnip",
			"USER=catnip",
			"TERM=xterm-direct",
			"COLORTERM=truecolor",
		)
		log.Printf("🐚 Starting bash shell for session: %s", sessionID)
	}
	cmd.Dir = workDir
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

	// Create new command using the same agent
	cmd := h.createCommand(session.ID, session.Agent, session.WorkDir, resumeSessionID)

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

	// Resize to match previous size
	_ = h.resizePTY(ptmx, session.cols, session.rows)

	log.Printf("✅ PTY recreated successfully for session: %s", session.ID)
}

func (h *PTYHandler) cleanupSession(session *Session) {
	h.sessionMutex.Lock()
	defer h.sessionMutex.Unlock()

	log.Printf("🧹 Cleaning up idle session: %s", session.ID)

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

	// Remove from sessions map
	delete(h.sessions, session.ID)
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
	defer s.connMutex.RUnlock()
	
	var disconnectedConns []*websocket.Conn
	
	for conn := range s.connections {
		if err := conn.WriteMessage(messageType, data); err != nil {
			log.Printf("❌ WebSocket write error: %v", err)
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
			if err := h.sessionService.StartActiveSession(session.WorkDir, sessionID); err != nil {
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

// GetSessionService returns the session service for external access
func (h *PTYHandler) GetSessionService() *services.SessionService {
	return h.sessionService
}