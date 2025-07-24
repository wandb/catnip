package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// PTYService manages PTY sessions and setup script execution
type PTYService struct {
	sessions     map[string]*SetupSession
	sessionMutex sync.RWMutex
}

// SetupSession represents a PTY session used for setup script execution
type SetupSession struct {
	ID          string
	PTY         *os.File
	Cmd         *exec.Cmd
	WorkDir     string
	CreatedAt   time.Time
	Buffer      []byte
	BufferMutex sync.RWMutex
}

// NewPTYService creates a new PTY service instance
func NewPTYService() *PTYService {
	return &PTYService{
		sessions: make(map[string]*SetupSession),
	}
}

// ExecuteSetupScript checks for and executes setup.sh in a worktree's PTY session
func (s *PTYService) ExecuteSetupScript(worktreePath string) {
	setupScriptPath := filepath.Join(worktreePath, "setup.sh")

	// Check if setup.sh exists and is executable
	if _, err := os.Stat(setupScriptPath); os.IsNotExist(err) {
		log.Printf("📄 No setup.sh found in %s, skipping setup", worktreePath)
		return
	}

	log.Printf("🔧 Found setup.sh in %s, executing in terminal", worktreePath)

	// Extract workspace name from worktree path for session ID
	// Format: /workspace/repo/branch -> repo/branch
	parts := strings.Split(strings.TrimPrefix(worktreePath, "/workspace/"), "/")
	if len(parts) < 2 {
		log.Printf("⚠️ Cannot determine session ID from worktree path: %s", worktreePath)
		return
	}
	sessionID := strings.Join(parts, "/")

	// Create or get existing session for this worktree
	session := s.getOrCreateSetupSession(sessionID, worktreePath)
	if session == nil {
		log.Printf("❌ Failed to create/get session for setup.sh execution: %s", sessionID)
		return
	}

	// Execute setup.sh in the PTY session
	// We'll run it with bash and make it executable first, just in case
	setupCommand := "chmod +x setup.sh && echo '🔧 Running setup.sh...' && ./setup.sh\n"

	// Write the command to the PTY
	if _, err := session.PTY.Write([]byte(setupCommand)); err != nil {
		log.Printf("❌ Failed to write setup command to PTY session %s: %v", sessionID, err)
		return
	}

	log.Printf("✅ Executed setup.sh in PTY session %s for worktree %s", sessionID, worktreePath)
}

// getOrCreateSetupSession creates or retrieves a setup session for the given session ID
func (s *PTYService) getOrCreateSetupSession(sessionID, workDir string) *SetupSession {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	// Check if session already exists
	if session, exists := s.sessions[sessionID]; exists {
		return session
	}

	// Create new setup session
	session := &SetupSession{
		ID:        sessionID,
		WorkDir:   workDir,
		CreatedAt: time.Now(),
		Buffer:    make([]byte, 0),
	}

	// Create PTY command for setup execution
	cmd := exec.Command("bash", "--login")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SESSION_ID=%s", sessionID),
		"HOME=/home/catnip",
		"USER=catnip",
		"TERM=xterm-direct",
		"COLORTERM=truecolor",
	)
	cmd.Dir = workDir

	// Start PTY
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		log.Printf("❌ Failed to start PTY for setup session %s: %v", sessionID, err)
		return nil
	}

	session.PTY = ptyFile
	session.Cmd = cmd

	// Start goroutine to read PTY output and buffer it
	go s.readPTYOutput(session)

	// Store session
	s.sessions[sessionID] = session

	log.Printf("✅ Created setup PTY session: %s in directory: %s", sessionID, workDir)
	return session
}

// readPTYOutput reads from PTY and buffers the output for later retrieval
func (s *PTYService) readPTYOutput(session *SetupSession) {
	defer func() {
		if session.PTY != nil {
			session.PTY.Close()
		}
		if session.Cmd != nil && session.Cmd.Process != nil {
			_ = session.Cmd.Process.Kill()
		}
	}()

	buffer := make([]byte, 1024)
	for {
		n, err := session.PTY.Read(buffer)
		if err != nil {
			log.Printf("📖 PTY read ended for setup session %s: %v", session.ID, err)
			break
		}

		if n > 0 {
			// Append to session buffer
			session.BufferMutex.Lock()
			session.Buffer = append(session.Buffer, buffer[:n]...)
			// Keep buffer size reasonable (last 64KB)
			if len(session.Buffer) > 65536 {
				session.Buffer = session.Buffer[len(session.Buffer)-65536:]
			}
			session.BufferMutex.Unlock()
		}
	}
}

// GetSetupSession retrieves a setup session by ID
func (s *PTYService) GetSetupSession(sessionID string) (*SetupSession, bool) {
	s.sessionMutex.RLock()
	defer s.sessionMutex.RUnlock()

	session, exists := s.sessions[sessionID]
	return session, exists
}

// GetSetupSessionBuffer returns the buffered output for a setup session
func (s *PTYService) GetSetupSessionBuffer(sessionID string) ([]byte, bool) {
	s.sessionMutex.RLock()
	session, exists := s.sessions[sessionID]
	s.sessionMutex.RUnlock()

	if !exists {
		return nil, false
	}

	session.BufferMutex.RLock()
	defer session.BufferMutex.RUnlock()

	// Return a copy of the buffer
	bufferCopy := make([]byte, len(session.Buffer))
	copy(bufferCopy, session.Buffer)
	return bufferCopy, true
}

// CleanupSession removes a setup session
func (s *PTYService) CleanupSession(sessionID string) {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	if session, exists := s.sessions[sessionID]; exists {
		if session.PTY != nil {
			session.PTY.Close()
		}
		if session.Cmd != nil && session.Cmd.Process != nil {
			_ = session.Cmd.Process.Kill()
		}
		delete(s.sessions, sessionID)
		log.Printf("🧹 Cleaned up setup session: %s", sessionID)
	}
}

// ListSetupSessions returns a list of all active setup sessions
func (s *PTYService) ListSetupSessions() map[string]*SetupSession {
	s.sessionMutex.RLock()
	defer s.sessionMutex.RUnlock()

	sessions := make(map[string]*SetupSession)
	for id, session := range s.sessions {
		sessions[id] = session
	}
	return sessions
}
