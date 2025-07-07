package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// PTYHandler handles PTY WebSocket connections
type PTYHandler struct {
	sessions     map[string]*Session
	sessionMutex sync.RWMutex
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
	connections map[*websocket.Conn]bool
	connMutex   sync.RWMutex
	// Buffer to store PTY output for replay
	outputBuffer []byte
	bufferMutex  sync.RWMutex
	maxBufferSize int
	// Terminal dimensions
	cols uint16
	rows uint16
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

// NewPTYHandler creates a new PTY handler
func NewPTYHandler() *PTYHandler {
	return &PTYHandler{
		sessions: make(map[string]*Session),
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

	// Replay buffered output to new connection
	session.bufferMutex.RLock()
	if len(session.outputBuffer) > 0 {
		log.Printf("📋 Replaying %d bytes of buffered output to new connection", len(session.outputBuffer))
		if err := conn.WriteMessage(websocket.BinaryMessage, session.outputBuffer); err != nil {
			log.Printf("❌ Failed to replay buffer: %v", err)
		}
	}
	session.bufferMutex.RUnlock()

	// Channel to signal when connection should close
	done := make(chan struct{})
	
	// Clean up connection on exit
	defer func() {
		close(done) // Signal goroutines to stop
		session.connMutex.Lock()
		delete(session.connections, conn)
		connectionCount := len(session.connections)
		session.connMutex.Unlock()
		
		log.Printf("🔌 PTY connection closed for session %s (remaining: %d)", sessionID, connectionCount)
		conn.Close()
	}()

	// Start goroutine to read from PTY and send to WebSocket
	go func() {
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
					
					// Notify all connections that shell has exited
					session.connMutex.RLock()
					for conn := range session.connections {
						conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[33m[Shell exited, starting new session...]\x1b[0m\r\n"))
					}
					session.connMutex.RUnlock()
					
					// Create new PTY
					h.recreateSession(session)
					
					// Continue reading from new PTY
					continue
				}
				log.Printf("❌ PTY read error: %v", err)
				return
			}
			
			// Add to buffer (with size limit)
			session.bufferMutex.Lock()
			session.outputBuffer = append(session.outputBuffer, buf[:n]...)
			if len(session.outputBuffer) > session.maxBufferSize {
				// Keep only the last maxBufferSize bytes
				excess := len(session.outputBuffer) - session.maxBufferSize
				session.outputBuffer = session.outputBuffer[excess:]
			}
			session.bufferMutex.Unlock()
			
			// Check if connection is still valid before writing
			select {
			case <-done:
				return
			default:
			}
			
			// Send to all connections
			session.connMutex.RLock()
			for conn := range session.connections {
				if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					log.Printf("❌ WebSocket write error: %v", err)
					// Don't return here - continue with other connections
				}
			}
			session.connMutex.RUnlock()
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
				}
			}
			
			// Try to parse as JSON for resize messages
			var resizeMsg ResizeMsg
			if err := json.Unmarshal(data, &resizeMsg); err == nil && resizeMsg.Cols > 0 && resizeMsg.Rows > 0 {
				session.cols = resizeMsg.Cols
				session.rows = resizeMsg.Rows
				h.resizePTY(session.PTY, resizeMsg.Cols, resizeMsg.Rows)
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

	// Set workspace directory (create only if it doesn't exist)
	workDir := filepath.Join("/workspace", sessionID)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		// Directory doesn't exist, try to create it
		if err := os.MkdirAll(workDir, 0755); err != nil {
			log.Printf("❌ Failed to create workspace directory: %v", err)
			workDir = "/workspace"
		} else {
			log.Printf("📁 Created workspace directory: %s", workDir)
		}
	} else {
		// Directory already exists (likely mounted)
		log.Printf("📁 Using existing workspace directory: %s", workDir)
	}

	// Create command based on agent parameter
	cmd := h.createCommand(sessionID, agent, workDir)

	// Start PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("❌ Failed to start PTY: %v", err)
		return nil
	}

	// Set initial size
	h.resizePTY(ptmx, 80, 24)

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
		maxBufferSize: 32 * 1024, // 32KB buffer
		cols:          80,
		rows:          24,
	}

	h.sessions[sessionID] = session
	log.Printf("✅ Created new PTY session: %s in %s", sessionID, workDir)

	// Start session cleanup goroutine
	go h.monitorSession(session)

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

func (h *PTYHandler) createCommand(sessionID, agent, workDir string) *exec.Cmd {
	var cmd *exec.Cmd
	if agent == "claude" {
		// Start Claude Code directly
		cmd = exec.Command("claude", "--dangerously-skip-permissions")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("SESSION_ID=%s", sessionID),
			"HOME=/home/catnip",
			"USER=catnip",
			"TERM=xterm-direct",
			"COLORTERM=truecolor",
		)
		log.Printf("🤖 Starting Claude Code for session: %s", sessionID)
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
		session.Cmd.Process.Kill()
		session.Cmd.Wait()
	}

	// Create new command using the same agent
	cmd := h.createCommand(session.ID, session.Agent, session.WorkDir)

	// Start new PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("❌ Failed to recreate PTY: %v", err)
		return
	}

	// Update session with new PTY and command
	session.PTY = ptmx
	session.Cmd = cmd

	// Clear the output buffer for fresh start
	session.bufferMutex.Lock()
	session.outputBuffer = make([]byte, 0)
	session.bufferMutex.Unlock()

	// Resize to match previous size
	h.resizePTY(ptmx, session.cols, session.rows)

	log.Printf("✅ PTY recreated successfully for session: %s", session.ID)
}

func (h *PTYHandler) cleanupSession(session *Session) {
	h.sessionMutex.Lock()
	defer h.sessionMutex.Unlock()

	log.Printf("🧹 Cleaning up idle session: %s", session.ID)

	// Close PTY
	if session.PTY != nil {
		session.PTY.Close()
	}

	// Terminate process
	if session.Cmd != nil && session.Cmd.Process != nil {
		session.Cmd.Process.Kill()
		session.Cmd.Wait()
	}

	// Remove from sessions map
	delete(h.sessions, session.ID)
}