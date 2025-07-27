package services

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/vanpelt/catnip/internal/git"
)

// ClaudeMonitorService monitors all worktrees for Claude sessions and manages checkpoints
type ClaudeMonitorService struct {
	gitService         *GitService
	sessionService     *SessionService
	checkpointManagers map[string]*WorktreeCheckpointManager // Map of worktree path to checkpoint manager
	managersMutex      sync.RWMutex
	titlesWatcher      *fsnotify.Watcher
	stopCh             chan struct{}
	titlesLogPath      string
	lastLogPosition    int64
}

// WorktreeCheckpointManager manages checkpoints for a single worktree
type WorktreeCheckpointManager struct {
	workDir           string
	checkpointManager *git.SessionCheckpointManager
	gitService        *GitService
	sessionService    *SessionService
	currentTitle      string
	checkpointTimer   *time.Timer
	timerMutex        sync.Mutex
}

// NewClaudeMonitorService creates a new Claude monitor service
func NewClaudeMonitorService(gitService *GitService, sessionService *SessionService) *ClaudeMonitorService {
	return &ClaudeMonitorService{
		gitService:         gitService,
		sessionService:     sessionService,
		checkpointManagers: make(map[string]*WorktreeCheckpointManager),
		stopCh:             make(chan struct{}),
		titlesLogPath:      "/tmp/catnip_syscall_titles.log",
	}
}

// Start begins monitoring all worktrees
func (s *ClaudeMonitorService) Start() error {
	log.Printf("🚀 Starting Claude monitor service")

	// Create file watcher for titles log
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create titles watcher: %w", err)
	}
	s.titlesWatcher = watcher

	// Start monitoring the titles log file
	go s.monitorTitlesLog()

	return nil
}

// Stop stops all monitoring
func (s *ClaudeMonitorService) Stop() {
	log.Printf("🛑 Stopping Claude monitor service")
	close(s.stopCh)

	if s.titlesWatcher != nil {
		s.titlesWatcher.Close()
	}

	s.managersMutex.Lock()
	defer s.managersMutex.Unlock()

	for path, manager := range s.checkpointManagers {
		manager.Stop()
		delete(s.checkpointManagers, path)
	}
}

// monitorTitlesLog monitors the titles log file for changes
func (s *ClaudeMonitorService) monitorTitlesLog() {
	log.Printf("👀 Starting to monitor titles log: %s", s.titlesLogPath)

	// Initial read of existing log entries
	s.readTitlesLog()

	// Watch for changes to the log file
	dir := filepath.Dir(s.titlesLogPath)
	if err := s.titlesWatcher.Add(dir); err != nil {
		log.Printf("⚠️  Failed to watch titles log directory: %v", err)
		return
	}

	for {
		select {
		case event, ok := <-s.titlesWatcher.Events:
			if !ok {
				return
			}
			if event.Name == s.titlesLogPath && event.Op&fsnotify.Write == fsnotify.Write {
				s.readTitlesLog()
			}
		case err, ok := <-s.titlesWatcher.Errors:
			if !ok {
				return
			}
			log.Printf("⚠️  Titles watcher error: %v", err)
		case <-s.stopCh:
			return
		}
	}
}

// readTitlesLog reads new entries from the titles log
func (s *ClaudeMonitorService) readTitlesLog() {
	file, err := os.Open(s.titlesLogPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("⚠️  Failed to open titles log: %v", err)
		}
		return
	}
	defer file.Close()

	// Seek to last read position
	if s.lastLogPosition > 0 {
		if _, err := file.Seek(s.lastLogPosition, 0); err != nil {
			log.Printf("⚠️  Failed to seek in titles log: %v", err)
			return
		}
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse log entry: timestamp|pid|cwd|title
		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			log.Printf("⚠️  Invalid log entry format: %s", line)
			continue
		}

		timestamp := parts[0]
		// pid := parts[1]
		cwd := parts[2]
		title := parts[3]

		log.Printf("🪧 Title change detected at %s: %q in %s", timestamp, title, cwd)

		// Check if this is a worktree directory
		if s.isWorktreeDirectory(cwd) {
			s.handleTitleChange(cwd, title)
		}
	}

	// Update last read position
	if pos, err := file.Seek(0, 1); err == nil {
		s.lastLogPosition = pos
	}
}

// isWorktreeDirectory checks if a directory is a git worktree
func (s *ClaudeMonitorService) isWorktreeDirectory(dir string) bool {
	// Check if directory is under /workspace
	if !strings.HasPrefix(dir, "/workspace/") {
		return false
	}

	// Check if it's a git repository
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return false
	}

	return true
}

// handleTitleChange processes a title change for a worktree
func (s *ClaudeMonitorService) handleTitleChange(workDir, newTitle string) {
	s.managersMutex.Lock()
	manager, exists := s.checkpointManagers[workDir]
	if !exists {
		// Create new checkpoint manager for this worktree
		manager = s.createCheckpointManager(workDir)
		s.checkpointManagers[workDir] = manager
		log.Printf("📝 Created checkpoint manager for worktree: %s", workDir)
	}
	s.managersMutex.Unlock()

	manager.HandleTitleChange(newTitle)
}

// createCheckpointManager creates a checkpoint manager for a worktree
func (s *ClaudeMonitorService) createCheckpointManager(workDir string) *WorktreeCheckpointManager {
	return &WorktreeCheckpointManager{
		workDir:           workDir,
		checkpointManager: git.NewSessionCheckpointManager(workDir, NewGitServiceAdapter(s.gitService), NewSessionServiceAdapter(s.sessionService)),
		gitService:        s.gitService,
		sessionService:    s.sessionService,
	}
}

// HandleTitleChange processes a new title change for this worktree
func (m *WorktreeCheckpointManager) HandleTitleChange(newTitle string) {
	m.timerMutex.Lock()
	defer m.timerMutex.Unlock()

	// Get the previous title from session service
	previousTitle := m.sessionService.GetPreviousTitle(m.workDir)

	// If we have a different title, commit the previous work
	if previousTitle != "" && previousTitle != newTitle {
		log.Printf("🪧 Title change detected in %s: %q -> %q", m.workDir, previousTitle, newTitle)
		m.commitPreviousWork(previousTitle)
	}

	// Update session service with the new title (no commit hash yet)
	if err := m.sessionService.UpdateSessionTitle(m.workDir, newTitle, ""); err != nil {
		log.Printf("⚠️  Failed to update session title: %v", err)
	}

	// Update the current title
	m.currentTitle = newTitle
	m.checkpointManager.Reset()

	// Cancel any existing timer
	if m.checkpointTimer != nil {
		m.checkpointTimer.Stop()
	}

	// Start checkpoint timer
	m.startCheckpointTimer()
}

// startCheckpointTimer starts or restarts the checkpoint timer
func (m *WorktreeCheckpointManager) startCheckpointTimer() {
	timeout := git.GetCheckpointTimeout()
	m.checkpointTimer = time.AfterFunc(timeout, func() {
		m.timerMutex.Lock()
		defer m.timerMutex.Unlock()

		if m.currentTitle != "" {
			// Check if there are any uncommitted changes using git operations
			if hasChanges, err := m.gitService.operations.HasUncommittedChanges(m.workDir); err != nil {
				log.Printf("⚠️  Failed to check for uncommitted changes: %v", err)
			} else if hasChanges {
				log.Printf("📝 Creating checkpoint for %s with title: %q", m.workDir, m.currentTitle)
				if err := m.checkpointManager.CreateCheckpoint(m.currentTitle); err != nil {
					log.Printf("⚠️  Failed to create checkpoint: %v", err)
				}
			}
			// Always restart the timer as long as we have a title
			m.startCheckpointTimer()
		}
	})
}

// Stop stops the checkpoint manager and cancels any pending timers
func (m *WorktreeCheckpointManager) Stop() {
	m.timerMutex.Lock()
	defer m.timerMutex.Unlock()

	if m.checkpointTimer != nil {
		m.checkpointTimer.Stop()
	}

	// Commit any pending work
	if m.currentTitle != "" {
		m.commitPreviousWork(m.currentTitle)
	}
}

// commitPreviousWork commits the previous work with the given title
func (m *WorktreeCheckpointManager) commitPreviousWork(title string) {
	if m.gitService == nil {
		return
	}

	commitHash, err := m.gitService.GitAddCommitGetHash(m.workDir, title)
	if err != nil {
		log.Printf("⚠️  Failed to commit previous work: %v", err)
		return
	}

	if commitHash != "" {
		log.Printf("✅ Committed previous work in %s: %q (hash: %s)", m.workDir, title, commitHash)
		m.checkpointManager.UpdateLastCommitTime()

		// Update the previous title's commit hash
		if err := m.sessionService.UpdatePreviousTitleCommitHash(m.workDir, commitHash); err != nil {
			log.Printf("⚠️  Failed to update previous title commit hash: %v", err)
		}
	}
}
