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
	// Add :setup suffix to match the composite session ID used in PTYHandler
	compositeSessionID := fmt.Sprintf("%s:setup", sessionID)

	// Create or get existing session for this worktree
	session := s.getOrCreateSetupSession(compositeSessionID, worktreePath)
	if session == nil {
		log.Printf("❌ Failed to create/get session for setup.sh execution: %s", compositeSessionID)
		return
	}

	log.Printf("✅ Started setup.sh execution in PTY session %s for worktree %s", compositeSessionID, worktreePath)
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

	// Create setup log file path
	setupLogPath := filepath.Join(workDir, ".catnip", "logs", "setup.log")
	if err := os.MkdirAll(filepath.Dir(setupLogPath), 0755); err != nil {
		log.Printf("❌ Failed to create .catnip/logs directory for setup log: %v", err)
		return nil
	}

	// Ensure .catnip/logs/* is in .gitignore
	s.ensureGitIgnore(workDir)

	// Create setup log file
	logFile, err := os.Create(setupLogPath)
	if err != nil {
		log.Printf("❌ Failed to create setup log file %s: %v", setupLogPath, err)
		return nil
	}

	// Create command to run setup script and capture output to file
	cmd := exec.Command("bash", "-c", "chmod +x setup.sh && echo '🔧 Running setup.sh...' && ./setup.sh && echo '\n✅ Setup completed'")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SESSION_ID=%s", sessionID),
		"HOME=/home/catnip",
		"USER=catnip",
		"TERM=xterm-direct",
		"COLORTERM=truecolor",
	)
	cmd.Dir = workDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	session.Cmd = cmd
	session.PTY = nil // No PTY needed for setup

	// Start the command and wait for completion in a goroutine
	go func() {
		defer logFile.Close()

		log.Printf("🔧 Starting setup script execution for session: %s", sessionID)
		if err := cmd.Run(); err != nil {
			log.Printf("❌ Setup script failed for session %s: %v", sessionID, err)
			// Write error to log file
			if _, writeErr := fmt.Fprintf(logFile, "\n❌ Setup script failed: %v\n", err); writeErr != nil {
				log.Printf("⚠️ Failed to write error to setup log: %v", writeErr)
			}
		} else {
			log.Printf("✅ Setup script completed successfully for session: %s", sessionID)
		}
	}()

	// Store session
	s.sessions[sessionID] = session

	log.Printf("✅ Created setup session: %s in directory: %s", sessionID, workDir)
	return session
}

// GetSetupSession retrieves a setup session by ID
func (s *PTYService) GetSetupSession(sessionID string) (*SetupSession, bool) {
	s.sessionMutex.RLock()
	defer s.sessionMutex.RUnlock()

	session, exists := s.sessions[sessionID]
	return session, exists
}

// GetSetupSessionBuffer returns the setup log content for a session
func (s *PTYService) GetSetupSessionBuffer(sessionID string) ([]byte, bool) {
	// Convert URL-encoded session ID back to file path format
	sessionID = strings.ReplaceAll(sessionID, "%2F", "/")

	s.sessionMutex.RLock()
	session, exists := s.sessions[sessionID]
	s.sessionMutex.RUnlock()

	if !exists {
		log.Printf("⚠️ Setup session not found: %s", sessionID)
		return nil, false
	}

	// Read from the setup log file
	setupLogPath := filepath.Join(session.WorkDir, ".catnip", "logs", "setup.log")
	content, err := os.ReadFile(setupLogPath)
	if err != nil {
		log.Printf("⚠️ Failed to read setup log file %s: %v", setupLogPath, err)
		return nil, false
	}

	log.Printf("✅ Read %d bytes from setup log file: %s", len(content), setupLogPath)
	return content, true
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

// ensureGitIgnore adds .catnip/logs/* to .gitignore if not already present
func (s *PTYService) ensureGitIgnore(workDir string) {
	gitignorePath := filepath.Join(workDir, ".gitignore")
	ignorePattern := ".catnip/logs/*"

	// Read existing .gitignore content
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		log.Printf("⚠️ Failed to read .gitignore: %v", err)
		return
	}

	contentStr := string(content)

	// Check if pattern already exists
	if strings.Contains(contentStr, ignorePattern) {
		return // Already present
	}

	// Add the ignore pattern
	if len(contentStr) > 0 && !strings.HasSuffix(contentStr, "\n") {
		contentStr += "\n"
	}
	contentStr += ignorePattern + "\n"

	// Write back to .gitignore
	if err := os.WriteFile(gitignorePath, []byte(contentStr), 0644); err != nil {
		log.Printf("⚠️ Failed to update .gitignore: %v", err)
	} else {
		log.Printf("✅ Added .catnip/logs/* to .gitignore")
	}
}
