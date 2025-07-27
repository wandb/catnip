package git

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultCheckpointTimeoutSeconds is the default checkpoint timeout in seconds
const DefaultCheckpointTimeoutSeconds = 30

// getCheckpointTimeout returns the checkpoint timeout duration from environment or default
func getCheckpointTimeout() time.Duration {
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
	StartFileWatcher() error
	StopFileWatcher()
	DetectClaudeTitle() (string, error)
	SetPtyTitle(title string)
	GetPtyTitle() string
}

// Service interface defines the git operations needed by checkpoint manager
type Service interface {
	GitAddCommitGetHash(workDir, title string) (string, error)
}

// SessionServiceInterface defines the session operations needed by checkpoint manager
type SessionServiceInterface interface {
	AddToSessionHistory(workDir, title, commitHash string) error
	GetActiveSession(workDir string) (interface{}, bool)
}

// SessionCheckpointManager implements CheckpointManager
type SessionCheckpointManager struct {
	lastCommitTime    time.Time
	checkpointCount   int
	checkpointMutex   sync.RWMutex
	gitService        Service
	sessionService    SessionServiceInterface
	workDir           string
	watcher           *fsnotify.Watcher
	watcherStopCh     chan struct{}
	fileChangeHandler func()
	claudeDetector    *ClaudeSessionDetector
	ptyTitle          string      // PTY-extracted title (highest priority)
	ptyTitleMutex     sync.RWMutex // Protects ptyTitle
}

// NewSessionCheckpointManager creates a new checkpoint manager
func NewSessionCheckpointManager(workDir string, gitService Service, sessionService SessionServiceInterface) *SessionCheckpointManager {
	return &SessionCheckpointManager{
		lastCommitTime:  time.Now(),
		checkpointCount: 0,
		gitService:      gitService,
		sessionService:  sessionService,
		workDir:         workDir,
		claudeDetector:  NewClaudeSessionDetector(workDir),
	}
}

// ShouldCreateCheckpoint returns true if a checkpoint should be created
func (cm *SessionCheckpointManager) ShouldCreateCheckpoint() bool {
	cm.checkpointMutex.RLock()
	defer cm.checkpointMutex.RUnlock()
	return time.Since(cm.lastCommitTime) >= getCheckpointTimeout()
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

// StartFileWatcher starts watching for file changes in the worktree
func (cm *SessionCheckpointManager) StartFileWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	cm.watcher = watcher
	cm.watcherStopCh = make(chan struct{})

	// Watch the worktree directory recursively
	if err := cm.addWatchRecursive(cm.workDir); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to add watch: %w", err)
	}

	// Start the watcher goroutine
	go cm.watcherLoop()

	log.Printf("👀 Started file watcher for worktree: %s", cm.workDir)
	return nil
}

// StopFileWatcher stops the file watcher
func (cm *SessionCheckpointManager) StopFileWatcher() {
	if cm.watcher != nil {
		close(cm.watcherStopCh)
		cm.watcher.Close()
		log.Printf("🛑 Stopped file watcher for worktree: %s", cm.workDir)
	}
}

// SetFileChangeHandler sets the handler for file changes
func (cm *SessionCheckpointManager) SetFileChangeHandler(handler func()) {
	cm.fileChangeHandler = handler
}

// addWatchRecursive adds watches recursively, excluding .git and other system directories
func (cm *SessionCheckpointManager) addWatchRecursive(path string) error {
	return filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories we can't access
		}

		if !info.IsDir() {
			return nil
		}

		// Skip .git, node_modules, and other common directories we don't want to watch
		base := filepath.Base(walkPath)
		if base == ".git" || base == "node_modules" || base == ".next" || base == "dist" || base == "build" {
			return filepath.SkipDir
		}

		if err := cm.watcher.Add(walkPath); err != nil {
			log.Printf("⚠️  Failed to watch directory %s: %v", walkPath, err)
		}
		return nil
	})
}

// watcherLoop handles file system events
func (cm *SessionCheckpointManager) watcherLoop() {
	// Debounce timer to avoid triggering on rapid changes
	var debounceTimer *time.Timer
	debounceDuration := 2 * time.Second

	for {
		select {
		case event, ok := <-cm.watcher.Events:
			if !ok {
				return
			}

			// Ignore .git directory changes
			gitDir := filepath.Join(cm.workDir, ".git")
			if strings.HasPrefix(event.Name, gitDir) {
				continue
			}

			// Reset debounce timer on each event
			if debounceTimer != nil {
				debounceTimer.Stop()
			}

			debounceTimer = time.AfterFunc(debounceDuration, func() {
				cm.checkpointMutex.Lock()
				lastChange := time.Since(cm.lastCommitTime)
				cm.checkpointMutex.Unlock()

				// Only trigger if enough time has passed since last commit
				if lastChange >= getCheckpointTimeout() && cm.fileChangeHandler != nil {
					log.Printf("📝 File changes detected after %v, triggering checkpoint check", lastChange)
					cm.fileChangeHandler()
				}
			})

		case err, ok := <-cm.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("⚠️  File watcher error: %v", err)

		case <-cm.watcherStopCh:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		}
	}
}

// SetPtyTitle sets the PTY-extracted title (highest priority)
func (cm *SessionCheckpointManager) SetPtyTitle(title string) {
	cm.ptyTitleMutex.Lock()
	defer cm.ptyTitleMutex.Unlock()
	cm.ptyTitle = title
	log.Printf("🖥️  PTY title updated: %q", title)
}

// GetPtyTitle returns the current PTY-extracted title
func (cm *SessionCheckpointManager) GetPtyTitle() string {
	cm.ptyTitleMutex.RLock()
	defer cm.ptyTitleMutex.RUnlock()
	return cm.ptyTitle
}

// DetectClaudeTitle attempts to detect the title of a Claude session running in the worktree
// Priority order: 1) PTY-extracted title, 2) Active session service, 3) JSONL file analysis
func (cm *SessionCheckpointManager) DetectClaudeTitle() (string, error) {
	// HIGHEST PRIORITY: PTY-extracted title from terminal escape sequences
	if ptyTitle := cm.GetPtyTitle(); ptyTitle != "" {
		log.Printf("📺 Using PTY-extracted title: %q", ptyTitle)
		return ptyTitle, nil
	}

	// SECOND PRIORITY: Active session tracked by our session service
	if activeSessionInterface, exists := cm.sessionService.GetActiveSession(cm.workDir); exists {
		// Type assert to access the title - this assumes the interface has a specific structure
		// In a real implementation, you might want to define a more specific interface
		if sessionMap, ok := activeSessionInterface.(map[string]interface{}); ok {
			if titleInfo, ok := sessionMap["title"].(map[string]interface{}); ok {
				if title, ok := titleInfo["title"].(string); ok && title != "" {
					log.Printf("📋 Using session service title: %q", title)
					return title, nil
				}
			}
		}
	}

	// LOWEST PRIORITY: Use the Claude detector to find active sessions from JSONL files
	if cm.claudeDetector != nil {
		sessionInfo, err := cm.claudeDetector.DetectClaudeSession()
		if err == nil && sessionInfo != nil && sessionInfo.Title != "" {
			log.Printf("📄 Using JSONL summary title: %q", sessionInfo.Title)
			return sessionInfo.Title, nil
		}
	}

	return "", fmt.Errorf("no Claude session or title found in worktree")
}

// MonitorClaudeSession starts monitoring for Claude sessions and title changes
func (cm *SessionCheckpointManager) MonitorClaudeSession(titleChangedFunc func(string)) error {
	if cm.claudeDetector == nil {
		return fmt.Errorf("claude detector not initialized")
	}

	// Detect current Claude session
	sessionInfo, err := cm.claudeDetector.DetectClaudeSession()
	if err != nil {
		return fmt.Errorf("failed to detect Claude session: %w", err)
	}

	// Start monitoring title changes
	return cm.claudeDetector.MonitorTitleChanges(sessionInfo.SessionID, titleChangedFunc)
}
