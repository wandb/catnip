package git

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

// DefaultCheckpointTimeoutSeconds is the default checkpoint timeout in seconds
const DefaultCheckpointTimeoutSeconds = 30

// GetCheckpointTimeout returns the checkpoint timeout duration from environment or default
func GetCheckpointTimeout() time.Duration {
	if timeoutStr := os.Getenv("CATNIP_COMMIT_TIMEOUT_SECONDS"); timeoutStr != "" {
		if timeout, err := strconv.Atoi(timeoutStr); err == nil && timeout > 0 {
			return time.Duration(timeout) * time.Second
		}
	}
	return DefaultCheckpointTimeoutSeconds * time.Second
}

// CheckpointManager handles checkpoint functionality for sessions
type CheckpointManager interface {
	ShouldCreateCheckpoint() bool
	CreateCheckpoint(title string) error
	Reset()
	UpdateLastCommitTime()
}

// Service interface defines the git operations needed by checkpoint manager
type Service interface {
	GitAddCommitGetHash(workDir, title string) (string, error)
	RefreshWorktreeStatus(workDir string) error
}

// SessionServiceInterface defines the session operations needed by checkpoint manager
type SessionServiceInterface interface {
	AddToSessionHistory(workDir, title, commitHash string) error
	GetActiveSession(workDir string) (interface{}, bool)
	UpdateSessionTitle(workDir, title, commitHash string) error
	GetPreviousTitle(workDir string) string
	UpdatePreviousTitleCommitHash(workDir string, commitHash string) error
}

// SessionCheckpointManager implements CheckpointManager
type SessionCheckpointManager struct {
	lastCommitTime  time.Time
	checkpointCount int
	checkpointMutex sync.RWMutex
	gitService      Service
	sessionService  SessionServiceInterface
	workDir         string
}

// NewSessionCheckpointManager creates a new checkpoint manager
func NewSessionCheckpointManager(workDir string, gitService Service, sessionService SessionServiceInterface) *SessionCheckpointManager {
	return &SessionCheckpointManager{
		lastCommitTime:  time.Now(),
		checkpointCount: 0,
		gitService:      gitService,
		sessionService:  sessionService,
		workDir:         workDir,
	}
}

// ShouldCreateCheckpoint returns true if a checkpoint should be created
func (cm *SessionCheckpointManager) ShouldCreateCheckpoint() bool {
	cm.checkpointMutex.RLock()
	defer cm.checkpointMutex.RUnlock()
	return time.Since(cm.lastCommitTime) >= GetCheckpointTimeout()
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

	// Refresh worktree status to update commit count in frontend
	if err := cm.gitService.RefreshWorktreeStatus(cm.workDir); err != nil {
		log.Printf("⚠️  Failed to refresh worktree status after checkpoint: %v", err)
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
