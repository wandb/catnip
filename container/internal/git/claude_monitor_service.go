package git

import (
	"log"
	"sync"
	"time"
)

// GitServiceWithWorktrees extends GitService with worktree operations
type GitServiceWithWorktrees interface {
	GitService
	ListWorktrees() ([]WorktreeInfo, error)
}

// ClaudeMonitorService monitors all worktrees for Claude sessions and manages checkpoints
type ClaudeMonitorService struct {
	gitService     GitServiceWithWorktrees
	sessionService SessionService
	monitors       map[string]*WorktreeMonitor // Map of worktree path to monitor
	monitorsMutex  sync.RWMutex
	stopCh         chan struct{}
}

// WorktreeMonitor monitors a single worktree for Claude sessions
type WorktreeMonitor struct {
	workDir           string
	checkpointManager *SessionCheckpointManager
	detector          *ClaudeSessionDetector
	currentTitle      string
	stopCh            chan struct{}
}

// NewClaudeMonitorService creates a new Claude monitor service
func NewClaudeMonitorService(gitService GitServiceWithWorktrees, sessionService SessionService) *ClaudeMonitorService {
	return &ClaudeMonitorService{
		gitService:     gitService,
		sessionService: sessionService,
		monitors:       make(map[string]*WorktreeMonitor),
		stopCh:         make(chan struct{}),
	}
}

// Start begins monitoring all worktrees
func (s *ClaudeMonitorService) Start() error {
	log.Printf("🚀 Starting Claude monitor service")

	// Start periodic scanning for worktrees
	go s.scanWorktrees()

	return nil
}

// Stop stops all monitoring
func (s *ClaudeMonitorService) Stop() {
	log.Printf("🛑 Stopping Claude monitor service")
	close(s.stopCh)

	s.monitorsMutex.Lock()
	defer s.monitorsMutex.Unlock()

	for path, monitor := range s.monitors {
		monitor.Stop()
		delete(s.monitors, path)
	}
}

// scanWorktrees periodically scans for worktrees and starts monitoring them
func (s *ClaudeMonitorService) scanWorktrees() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Do initial scan
	s.updateWorktreeMonitors()

	for {
		select {
		case <-ticker.C:
			s.updateWorktreeMonitors()
		case <-s.stopCh:
			return
		}
	}
}

// updateWorktreeMonitors updates the list of monitored worktrees
func (s *ClaudeMonitorService) updateWorktreeMonitors() {
	// Get list of all worktrees
	worktrees, err := s.gitService.ListWorktrees()
	if err != nil {
		log.Printf("⚠️  Failed to list worktrees: %v", err)
		return
	}

	s.monitorsMutex.Lock()
	defer s.monitorsMutex.Unlock()

	// Start monitors for new worktrees
	for _, worktree := range worktrees {
		if _, exists := s.monitors[worktree.Path]; !exists {
			monitor := s.createWorktreeMonitor(worktree.Path)
			if monitor != nil {
				s.monitors[worktree.Path] = monitor
				go monitor.Start()
				log.Printf("👀 Started monitoring worktree: %s", worktree.Path)
			}
		}
	}

	// Stop monitors for removed worktrees
	for path, monitor := range s.monitors {
		found := false
		for _, worktree := range worktrees {
			if worktree.Path == path {
				found = true
				break
			}
		}
		if !found {
			monitor.Stop()
			delete(s.monitors, path)
			log.Printf("🛑 Stopped monitoring removed worktree: %s", path)
		}
	}
}

// createWorktreeMonitor creates a monitor for a specific worktree
func (s *ClaudeMonitorService) createWorktreeMonitor(workDir string) *WorktreeMonitor {
	checkpointManager := NewSessionCheckpointManager(workDir, s.gitService, s.sessionService)
	
	monitor := &WorktreeMonitor{
		workDir:           workDir,
		checkpointManager: checkpointManager,
		detector:          NewClaudeSessionDetector(workDir),
		stopCh:            make(chan struct{}),
	}

	// Set up file change handler
	checkpointManager.SetFileChangeHandler(func() {
		monitor.checkForCheckpoint()
	})

	return monitor
}

// Start begins monitoring the worktree
func (m *WorktreeMonitor) Start() {
	// Start file watcher
	if err := m.checkpointManager.StartFileWatcher(); err != nil {
		log.Printf("⚠️  Failed to start file watcher for %s: %v", m.workDir, err)
	}

	// Start Claude session detection loop
	go m.detectClaudeSessions()
}

// Stop stops monitoring the worktree
func (m *WorktreeMonitor) Stop() {
	close(m.stopCh)
	m.checkpointManager.StopFileWatcher()
}

// detectClaudeSessions periodically checks for Claude sessions
func (m *WorktreeMonitor) detectClaudeSessions() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sessionInfo, err := m.detector.DetectClaudeSession()
			if err == nil && sessionInfo != nil {
				// Update current title if changed
				if sessionInfo.Title != "" && sessionInfo.Title != m.currentTitle {
					log.Printf("🪧 Detected Claude title change in %s: %q -> %q", 
						m.workDir, m.currentTitle, sessionInfo.Title)
					
					// Commit previous work if there was a title
					if m.currentTitle != "" {
						m.commitPreviousWork(m.currentTitle)
					}

					m.currentTitle = sessionInfo.Title
					m.checkpointManager.Reset()
				}

				// Check if we need a checkpoint
				m.checkForCheckpoint()
			}
		case <-m.stopCh:
			return
		}
	}
}

// checkForCheckpoint checks if a checkpoint is needed and creates one
func (m *WorktreeMonitor) checkForCheckpoint() {
	if m.currentTitle == "" {
		return
	}

	if m.checkpointManager.ShouldCreateCheckpoint() {
		log.Printf("📝 Creating checkpoint for %s with title: %q", m.workDir, m.currentTitle)
		if err := m.checkpointManager.CreateCheckpoint(m.currentTitle); err != nil {
			log.Printf("⚠️  Failed to create checkpoint: %v", err)
		}
	}
}

// commitPreviousWork commits the previous work with the given title
func (m *WorktreeMonitor) commitPreviousWork(title string) {
	gitService := m.checkpointManager.gitService
	if gitService == nil {
		return
	}

	commitHash, err := gitService.GitAddCommitGetHash(m.workDir, title)
	if err != nil {
		log.Printf("⚠️  Failed to commit previous work: %v", err)
		return
	}

	if commitHash != "" {
		log.Printf("✅ Committed previous work in %s: %q (hash: %s)", m.workDir, title, commitHash)
		m.checkpointManager.UpdateLastCommitTime()
	}
}