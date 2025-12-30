package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/vanpelt/catnip/internal/claude/parser"
	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
)

// ClaudeMonitorService monitors all worktrees for Claude sessions and manages checkpoints
type ClaudeMonitorService struct {
	gitService         *GitService
	sessionService     *SessionService
	claudeService      *ClaudeService
	parserService      *ParserService
	stateManager       *WorktreeStateManager                 // Centralized state management
	checkpointManagers map[string]*WorktreeCheckpointManager // Map of worktree path to checkpoint manager
	managersMutex      sync.RWMutex
	titlesWatcher      *fsnotify.Watcher
	stopCh             chan struct{}
	titlesLogPath      string
	lastLogPosition    int64
	recentTitles       map[string]titleEvent // Track recent titles to avoid duplicates
	recentTitlesMutex  sync.RWMutex
	lastActivityTimes  map[string]time.Time // Track last activity per worktree path
	activityMutex      sync.RWMutex
	todoMonitors       map[string]*WorktreeTodoMonitor // Map of worktree path to todo monitor
	todoMonitorsMutex  sync.RWMutex
}

// titleEvent represents a title change event with timestamp
type titleEvent struct {
	title     string
	timestamp time.Time
	source    string // "log" or "pty"
}

// WorktreeCheckpointManager manages checkpoints for a single worktree
type WorktreeCheckpointManager struct {
	workDir            string
	worktreeID         string // Cached worktree ID to avoid expensive lookups
	checkpointManager  *git.SessionCheckpointManager
	gitService         *GitService
	sessionService     *SessionService
	claudeService      *ClaudeService
	stateManager       *WorktreeStateManager
	currentTitle       string
	checkpointTimer    *time.Timer
	timerMutex         sync.Mutex
	renamingInProgress bool // Track if a rename is currently in progress
}

// WorktreeTodoMonitor monitors Todo updates and latest Claude messages for a single worktree
type WorktreeTodoMonitor struct {
	workDir         string
	projectDir      string
	claudeService   *ClaudeService
	parserService   *ParserService
	claudeMonitor   *ClaudeMonitorService
	gitService      *GitService
	sessionService  *SessionService
	ticker          *time.Ticker
	stopCh          chan struct{}
	lastTodos       []models.Todo
	lastTodosJSON   string // JSON representation for comparison
	lastMessage     string // Last Claude message content for comparison
	lastMessageType string // "assistant" or "user"
	lastMessageUUID string // UUID of the last message to detect changes
}

// NewClaudeMonitorService creates a new Claude monitor service
func NewClaudeMonitorService(gitService *GitService, sessionService *SessionService, claudeService *ClaudeService, parserService *ParserService, stateManager *WorktreeStateManager) *ClaudeMonitorService {
	// Get log path from environment or use runtime-appropriate default
	titlesLogPath := os.Getenv("CATNIP_TITLE_LOG")
	if titlesLogPath == "" {
		titlesLogPath = filepath.Join(config.Runtime.HomeDir, ".catnip", "title_events.log")
	}

	return &ClaudeMonitorService{
		gitService:         gitService,
		sessionService:     sessionService,
		claudeService:      claudeService,
		parserService:      parserService,
		stateManager:       stateManager,
		checkpointManagers: make(map[string]*WorktreeCheckpointManager),
		stopCh:             make(chan struct{}),
		titlesLogPath:      titlesLogPath,
		recentTitles:       make(map[string]titleEvent),
		lastActivityTimes:  make(map[string]time.Time),
		todoMonitors:       make(map[string]*WorktreeTodoMonitor),
	}
}

// Start begins monitoring all worktrees
func (s *ClaudeMonitorService) Start() error {
	logger.Infof("🚀 Starting Claude monitor service, titles log path: %s", s.titlesLogPath)

	// Ensure the titles log file and directory exist
	if err := s.ensureTitlesLogFile(); err != nil {
		logger.Warnf("⚠️  Failed to ensure titles log file exists: %v", err)
	}

	// Create file watcher for titles log
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create titles watcher: %w", err)
	}
	s.titlesWatcher = watcher

	// Start monitoring the titles log file
	go s.monitorTitlesLog()

	// Start Todo monitoring for all existing worktrees
	go s.startTodoMonitoring()

	return nil
}

// ensureTitlesLogFile ensures the titles log file and directory exist
func (s *ClaudeMonitorService) ensureTitlesLogFile() error {
	// Ensure the directory exists
	dir := filepath.Dir(s.titlesLogPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create titles log directory %s: %w", dir, err)
	}

	// Check if file already exists
	if _, err := os.Stat(s.titlesLogPath); err != nil {
		if os.IsNotExist(err) {
			// Create the file if it doesn't exist
			if err := os.WriteFile(s.titlesLogPath, []byte(""), 0644); err != nil {
				return fmt.Errorf("failed to create titles log file %s: %w", s.titlesLogPath, err)
			}
			logger.Debugf("📝 Created titles log file: %s", s.titlesLogPath)
		} else {
			return fmt.Errorf("failed to stat titles log file %s: %w", s.titlesLogPath, err)
		}
	}

	return nil
}

// Stop stops all monitoring
func (s *ClaudeMonitorService) Stop() {
	logger.Info("🛑 Stopping Claude monitor service")
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

	s.todoMonitorsMutex.Lock()
	defer s.todoMonitorsMutex.Unlock()

	for path, monitor := range s.todoMonitors {
		monitor.Stop()
		delete(s.todoMonitors, path)
	}
}

// monitorTitlesLog monitors the titles log file for changes
func (s *ClaudeMonitorService) monitorTitlesLog() {
	logger.Debugf("👀 Starting to monitor titles log: %s", s.titlesLogPath)

	// Initial read of existing log entries
	s.readTitlesLog()

	// Watch for changes to the log file
	dir := filepath.Dir(s.titlesLogPath)
	if err := s.titlesWatcher.Add(dir); err != nil {
		logger.Warnf("⚠️  Failed to watch titles log directory: %v", err)
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
			logger.Warnf("⚠️  Titles watcher error: %v", err)
		case <-s.stopCh:
			return
		}
	}
}

// readTitlesLog reads new entries from the titles log
func (s *ClaudeMonitorService) readTitlesLog() {
	file, err := os.Open(s.titlesLogPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debugf("📝 Titles log file doesn't exist yet: %s", s.titlesLogPath)
		} else {
			logger.Warnf("⚠️  Failed to open titles log: %v", err)
		}
		return
	}
	defer file.Close()

	// Seek to last read position
	if s.lastLogPosition > 0 {
		if _, err := file.Seek(s.lastLogPosition, 0); err != nil {
			logger.Warnf("⚠️  Failed to seek in titles log: %v", err)
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
			logger.Warnf("⚠️  Invalid log entry format: %s", line)
			continue
		}

		timestampStr := parts[0]
		// pid := parts[1]
		cwd := parts[2]
		title := parts[3]

		// Parse timestamp and filter out old events (only process events from last 30 seconds)
		eventTime, err := time.Parse(time.RFC3339Nano, timestampStr)
		if err != nil {
			logger.Warnf("⚠️  Invalid timestamp format: %s", timestampStr)
			continue
		}

		// Only process events from the last 30 seconds to avoid processing old log entries
		if time.Since(eventTime) > 30*time.Second {
			logger.Debugf("🕒 Skipping old title event (%v ago): %q in %s", time.Since(eventTime), title, cwd)
			continue
		}

		logger.Debugf("🪧 Title change detected at %s: %q in %s", timestampStr, title, cwd)

		// Check if this is a managed worktree directory
		isWorktree := s.isWorktreeDirectory(cwd)
		isExternal := s.isExternalGitRepository(cwd)

		logger.Debugf("📁 Path analysis for %s: isWorktree=%v, isExternal=%v, workspaceDir=%s",
			cwd, isWorktree, isExternal, config.Runtime.WorkspaceDir)

		if isWorktree {
			// Clean the title before processing
			cleanedTitle := cleanTitle(title)
			if cleanedTitle != "" { // Only process if title isn't empty after cleaning
				s.handleTitleChange(cwd, cleanedTitle, "log")
			}
		} else if isExternal {
			// Handle external Git repository - attempt to auto-create workspace reference
			logger.Infof("🔍 External Git repository detected: %s", cwd)

			// Try to create auto-workspace reference (this will be a no-op if already exists)
			if err := s.createAutoWorkspaceForExternalRepo(cwd); err != nil {
				logger.Warnf("⚠️ Failed to create auto-workspace for %s: %v", cwd, err)
			}
			// Note: We don't handle title changes for external repos as they're outside our control
			logger.Debugf("📍 Skipping title handling for external repo (outside our control): %s", cwd)
		} else {
			logger.Debugf("⚠️ Path %s is neither a worktree nor an external Git repo, ignoring", cwd)
		}
	}

	// Update last read position
	if pos, err := file.Seek(0, 1); err == nil {
		s.lastLogPosition = pos
	}
}

// isWorktreeDirectory checks if a directory is a git worktree
func (s *ClaudeMonitorService) isWorktreeDirectory(dir string) bool {
	// Check if directory is under the configured workspace directory (managed worktrees)
	workspaceDir := config.Runtime.WorkspaceDir
	if workspaceDir != "" && strings.HasPrefix(dir, workspaceDir+"/") {
		// Check if it's a git repository
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err != nil {
			return false
		}
		return true
	}
	return false
}

// isExternalGitRepository checks if a directory is a Git repository outside our managed workspace
func (s *ClaudeMonitorService) isExternalGitRepository(dir string) bool {
	// Skip if it's already under our managed workspace
	workspaceDir := config.Runtime.WorkspaceDir
	if workspaceDir != "" && strings.HasPrefix(dir, workspaceDir+"/") {
		return false
	}

	// Check if it's a git repository
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return false
	}
	return true
}

// createAutoWorkspaceForExternalRepo attempts to create an auto-workspace reference for an external repo
// Only creates a workspace reference if the external repo matches a repository we're already tracking
func (s *ClaudeMonitorService) createAutoWorkspaceForExternalRepo(repoPath string) error {
	logger.Infof("🔍 Detected Claude session in external repository: %s", repoPath)

	// Get the remote origin URL from the external repository
	remoteOrigin, err := s.gitService.operations.GetRemoteURL(repoPath)
	if err != nil {
		logger.Infof("📍 External repo %s has no remote origin (error: %v), skipping auto-workspace creation", repoPath, err)
		return nil
	}

	logger.Infof("📍 External repo %s has remote origin: %s", repoPath, remoteOrigin)

	// Find if we already have a repository with this remote URL
	existingRepo := s.findRepositoryByRemoteURL(remoteOrigin)
	if existingRepo == nil {
		logger.Infof("📍 External repo %s (remote: %s) doesn't match any tracked repositories, skipping auto-workspace creation", repoPath, remoteOrigin)
		logger.Debugf("📍 Available tracked repositories:")
		status := s.gitService.GetStatus()
		for _, repo := range status.Repositories {
			logger.Debugf("  - %s: RemoteOrigin=%s, URL=%s", repo.ID, repo.RemoteOrigin, repo.URL)
		}
		return nil
	}

	// Check if we already have a workspace reference pointing to this external path
	// This prevents creating duplicate workspace references for the same external repository
	existingWorktree := s.findWorktreeByExternalPath(repoPath)
	if existingWorktree != nil {
		logger.Infof("🔍 Found existing workspace reference %s for path %s, updating branch if needed", existingWorktree.Name, repoPath)

		// Get the current branch from the external repository
		currentBranch, err := s.getCurrentBranch(repoPath)
		if err != nil {
			logger.Warnf("⚠️ Could not determine current branch for %s: %v", repoPath, err)
			// Still return nil since we found an existing workspace - no need to create a new one
			return nil
		}

		// Update the branch if it's different
		if existingWorktree.Branch != currentBranch {
			logger.Infof("🔄 Updating existing workspace branch from %s to %s", existingWorktree.Branch, currentBranch)
			// Find worktree ID by exact path match
			allWorktrees := s.stateManager.GetAllWorktrees()
			for worktreeID, wt := range allWorktrees {
				if wt.Path == repoPath {
					if err := s.stateManager.UpdateWorktree(worktreeID, map[string]interface{}{
						"branch": currentBranch,
					}); err != nil {
						logger.Warnf("⚠️ Failed to update worktree branch: %v", err)
					} else {
						logger.Infof("✅ Updated workspace %s branch to %s", existingWorktree.Name, currentBranch)
					}
					break
				}
			}
		}

		// Return early - no need to create a new workspace
		return nil
	}

	logger.Infof("🎯 External repo %s matches tracked repository %s, creating workspace reference", repoPath, existingRepo.ID)

	// Get the current branch from the external repository
	currentBranch, err := s.getCurrentBranch(repoPath)
	if err != nil {
		logger.Warnf("⚠️ Could not determine current branch for %s: %v", repoPath, err)
		currentBranch = existingRepo.DefaultBranch // fallback to repo's default branch
	}

	// Generate a name based on the external path
	repoName := filepath.Base(repoPath)
	var workspaceName string
	if currentBranch != existingRepo.DefaultBranch {
		// Replace "/" with "-" in branch names to create valid workspace names
		safeBranchName := strings.ReplaceAll(currentBranch, "/", "-")
		workspaceName = fmt.Sprintf("%s/%s", repoName, safeBranchName)
	} else {
		workspaceName = repoName
	}

	// Create workspace reference (not a physical worktree)
	worktree, err := s.createWorkspaceReference(existingRepo, repoPath, currentBranch, workspaceName)
	if err != nil {
		return fmt.Errorf("failed to create workspace reference for %s: %v", repoPath, err)
	}

	logger.Infof("✅ Created workspace reference for external repository: %s -> %s (branch: %s)", repoPath, worktree.Name, currentBranch)
	return nil
}

// findWorktreeByExternalPath finds an existing worktree that has the exact external path
func (s *ClaudeMonitorService) findWorktreeByExternalPath(externalPath string) *models.Worktree {
	// Get all worktrees from the state manager
	allWorktrees := s.stateManager.GetAllWorktrees()
	for _, worktree := range allWorktrees {
		// Check if the worktree path exactly matches the external path
		if worktree.Path == externalPath {
			return worktree
		}
	}
	return nil
}

// findRepositoryByRemoteURL finds an existing repository that matches the given remote URL
func (s *ClaudeMonitorService) findRepositoryByRemoteURL(remoteURL string) *models.Repository {
	// Get all repositories from the state manager
	status := s.gitService.GetStatus()
	for _, repo := range status.Repositories {
		// Check if the remote origin matches
		if repo.RemoteOrigin == remoteURL {
			return repo
		}
		// Also check the main URL field as fallback
		if repo.URL == remoteURL {
			return repo
		}
	}
	return nil
}

// getCurrentBranch gets the current branch from a repository using existing operations
func (s *ClaudeMonitorService) getCurrentBranch(repoPath string) (string, error) {
	// Use the existing GetDisplayBranch operation to get the current branch
	// This handles both regular branches and any custom refs properly
	output, err := s.gitService.operations.ExecuteGit(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}

	currentBranch := strings.TrimSpace(string(output))
	if currentBranch == "HEAD" || currentBranch == "" {
		return "", fmt.Errorf("repository is in detached HEAD state")
	}

	return currentBranch, nil
}

// createWorkspaceReference creates a workspace reference (not a physical worktree) for an external repository
func (s *ClaudeMonitorService) createWorkspaceReference(repo *models.Repository, externalPath, branch, name string) (*models.Worktree, error) {
	// Create a unique ID for this workspace reference
	id := uuid.New().String()

	// Create a worktree model that references the external path directly
	worktree := &models.Worktree{
		ID:                     id,
		RepoID:                 repo.ID,
		Name:                   name,
		Path:                   externalPath, // Point directly to the external path
		Branch:                 branch,
		SourceBranch:           repo.DefaultBranch,
		HasBeenRenamed:         false,
		CommitHash:             "", // Will be populated by status refresh
		CommitCount:            0,  // Will be populated by status refresh
		CommitsBehind:          0,  // Will be populated by status refresh
		IsDirty:                false,
		HasConflicts:           false,
		PullRequestURL:         "",
		SessionTitle:           nil,
		SessionTitleHistory:    []models.TitleEntry{},
		HasActiveClaudeSession: false,
		ClaudeActivityState:    models.ClaudeInactive,
		Todos:                  []models.Todo{},
	}

	// Add the workspace reference to the state manager
	if err := s.stateManager.AddWorktree(worktree); err != nil {
		return nil, fmt.Errorf("failed to add workspace reference: %v", err)
	}

	// Start todo monitoring for the new workspace reference
	s.OnWorktreeCreated(worktree.ID, worktree.Path)

	// Refresh status to populate commit info
	if err := s.gitService.RefreshWorktreeStatus(externalPath); err != nil {
		logger.Warnf("⚠️ Failed to refresh status for external workspace: %v", err)
	}

	return worktree, nil
}

// handleTitleChange processes a title change for a worktree with duplicate detection
func (s *ClaudeMonitorService) handleTitleChange(workDir, newTitle, source string) {
	// Check for recent duplicate events
	key := workDir + ":" + newTitle
	s.recentTitlesMutex.Lock()

	// Clean up old events (older than 5 seconds)
	cutoff := time.Now().Add(-5 * time.Second)
	for k, event := range s.recentTitles {
		if event.timestamp.Before(cutoff) {
			delete(s.recentTitles, k)
		}
	}

	// Check if we've seen this exact title recently
	if recent, exists := s.recentTitles[key]; exists {
		// If log source and we already have a log entry, skip
		// If pty source and we already have any entry from last 2 seconds, skip
		if source == "log" && recent.source == "log" {
			s.recentTitlesMutex.Unlock()
			return
		}
		if source == "pty" && time.Since(recent.timestamp) < 2*time.Second {
			s.recentTitlesMutex.Unlock()
			return
		}
	}

	// Record this title event
	s.recentTitles[key] = titleEvent{
		title:     newTitle,
		timestamp: time.Now(),
		source:    source,
	}
	s.recentTitlesMutex.Unlock()

	// Update activity time for title changes (but don't update Claude service activity
	// as title changes are passive monitoring, not active Claude usage)
	now := time.Now()
	s.activityMutex.Lock()
	s.lastActivityTimes[workDir] = now
	s.activityMutex.Unlock()

	// Note: We intentionally don't call s.claudeService.UpdateActivity(workDir) here
	// because title change processing is passive monitoring and should not keep workspaces "active"

	s.managersMutex.Lock()
	manager, exists := s.checkpointManagers[workDir]
	if !exists {
		// Create new checkpoint manager for this worktree
		manager = s.createCheckpointManager(workDir)
		s.checkpointManagers[workDir] = manager
		logger.Debugf("📝 Created checkpoint manager for worktree: %s", workDir)
	}
	s.managersMutex.Unlock()

	// Check if we need to start todo monitoring for this worktree
	// This handles the case where Claude starts working after the initial todo monitoring scan
	s.todoMonitorsMutex.RLock()
	_, todoMonitorExists := s.todoMonitors[workDir]
	s.todoMonitorsMutex.RUnlock()

	if !todoMonitorExists {
		// Get worktree ID from GitService
		worktrees := s.gitService.stateManager.GetAllWorktrees()
		for worktreeID, worktree := range worktrees {
			if worktree.Path == workDir {
				logger.Debugf("🔍 Starting todo monitor for worktree %s after title change", workDir)
				s.startWorktreeTodoMonitor(worktreeID, workDir)
				break
			}
		}
	}

	// Update worktree state with latest session title and user prompt
	s.updateWorktreePromptAndTitleData(workDir, newTitle)

	manager.HandleTitleChange(newTitle)
}

// updateWorktreePromptAndTitleData updates the worktree state with latest session title and user prompt
func (s *ClaudeMonitorService) updateWorktreePromptAndTitleData(workDir, latestSessionTitle string) {
	// Find the worktree ID for this path
	worktrees := s.gitService.stateManager.GetAllWorktrees()
	var worktreeID string
	for id, worktree := range worktrees {
		if worktree.Path == workDir {
			worktreeID = id
			break
		}
	}

	if worktreeID == "" {
		logger.Debugf("⚠️ No worktree found for path %s, skipping prompt/title update", workDir)
		return
	}

	// Get the latest user prompt from ~/.claude.json
	latestUserPrompt, err := s.claudeService.GetLatestUserPrompt(workDir)
	if err != nil {
		logger.Debugf("⚠️ Failed to get latest user prompt for %s: %v", workDir, err)
		latestUserPrompt = "" // Continue with empty prompt
	}

	// Prepare updates
	updates := make(map[string]interface{})
	if latestSessionTitle != "" {
		updates["latest_session_title"] = latestSessionTitle
	}
	if latestUserPrompt != "" {
		updates["latest_user_prompt"] = latestUserPrompt
	}

	// Only update if we have something to update
	if len(updates) > 0 {
		if err := s.stateManager.UpdateWorktree(worktreeID, updates); err != nil {
			logger.Warnf("⚠️ Failed to update worktree prompt/title data for %s: %v", worktreeID, err)
		} else {
			logger.Debugf("✅ Updated worktree %s with latest session title and user prompt", worktreeID)
		}
	}
}

// NotifyTitleChange allows direct notification of title changes (fallback for when log monitoring fails)
func (s *ClaudeMonitorService) NotifyTitleChange(workDir, newTitle string) {
	// Check if this is a worktree directory
	if s.isWorktreeDirectory(workDir) {
		// Clean the title before processing
		cleanedTitle := cleanTitle(newTitle)
		if cleanedTitle != "" { // Only process if title isn't empty after cleaning
			s.handleTitleChange(workDir, cleanedTitle, "pty")
		}
	}
}

// findWorktreeIDByPath finds the worktree ID for a given workDir path (expensive - use sparingly)
func (s *ClaudeMonitorService) findWorktreeIDByPath(workDir string) string {
	allWorktrees := s.stateManager.GetAllWorktrees()
	for id, worktree := range allWorktrees {
		if worktree.Path == workDir {
			return id
		}
	}
	logger.Warnf("⚠️  Failed to find worktree ID for path %s", workDir)
	return ""
}

// createCheckpointManager creates a checkpoint manager for a worktree
func (s *ClaudeMonitorService) createCheckpointManager(workDir string) *WorktreeCheckpointManager {
	// Find and cache the worktree ID once to avoid expensive lookups later
	worktreeID := s.findWorktreeIDByPath(workDir)

	return &WorktreeCheckpointManager{
		workDir:           workDir,
		worktreeID:        worktreeID,
		checkpointManager: git.NewSessionCheckpointManager(workDir, NewGitServiceAdapter(s.gitService), NewSessionServiceAdapter(s.sessionService)),
		gitService:        s.gitService,
		sessionService:    s.sessionService,
		claudeService:     s.claudeService,
		stateManager:      s.stateManager,
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
		logger.Debugf("🪧 Title change detected in %s: %q -> %q", m.workDir, previousTitle, newTitle)
		m.commitPreviousWork(previousTitle)
	}

	// Update session service with the new title (no commit hash yet)
	if err := m.sessionService.UpdateSessionTitle(m.workDir, newTitle, ""); err != nil {
		logger.Warnf("⚠️  Failed to update session title: %v", err)
	}

	// Update the current title
	m.currentTitle = newTitle
	m.checkpointManager.Reset()

	// Cancel any existing timer
	if m.checkpointTimer != nil {
		m.checkpointTimer.Stop()
	}

	// Check if we need to rename the branch based on the new title
	// Only rename if we're currently on a catnip branch and not already renaming
	if !m.renamingInProgress && m.currentTitle != "" && m.isCurrentBranchCatnip() {
		m.renamingInProgress = true // Set flag to prevent multiple simultaneous attempts
		go m.checkAndRenameBranch(newTitle)
	}

	// Start checkpoint timer
	m.startCheckpointTimer()
}

// startCheckpointTimer starts or restarts the checkpoint timer
func (m *WorktreeCheckpointManager) startCheckpointTimer() {
	timeout := git.GetCheckpointTimeout()
	// Start timer silently
	m.checkpointTimer = time.AfterFunc(timeout, func() {
		m.timerMutex.Lock()
		defer m.timerMutex.Unlock()

		// Timer fired, check for changes
		if m.currentTitle != "" {
			// Check if there are any uncommitted changes using git operations
			if hasChanges, err := m.gitService.operations.HasUncommittedChanges(m.workDir); err != nil {
				logger.Warnf("⚠️  Failed to check for uncommitted changes: %v", err)
			} else if hasChanges {
				if err := m.checkpointManager.CreateCheckpoint(m.currentTitle); err != nil {
					logger.Warnf("⚠️  Failed to create checkpoint: %v", err)
				} else {
					logger.Infof("✅ Created checkpoint for %s: %q", m.workDir, m.currentTitle)
				}
			}
			// Skip logging when no changes - this is normal
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
		logger.Warnf("⚠️  Failed to commit previous work: %v", err)
		return
	}

	if commitHash != "" {
		logger.Infof("✅ Committed previous work in %s: %q (hash: %s)", m.workDir, title, commitHash)
		m.checkpointManager.UpdateLastCommitTime()

		// Update the previous title's commit hash
		if err := m.sessionService.UpdatePreviousTitleCommitHash(m.workDir, commitHash); err != nil {
			logger.Warnf("⚠️  Failed to update previous title commit hash: %v", err)
		}

		// Refresh worktree status to update commit count in frontend
		if err := m.gitService.RefreshWorktreeStatus(m.workDir); err != nil {
			logger.Warnf("⚠️  Failed to refresh worktree status after commit: %v", err)
		}
	}
}

// checkAndRenameBranch checks if we need to graduate a catnip branch to a semantic name based on the title
func (m *WorktreeCheckpointManager) checkAndRenameBranch(title string) {
	// Clean the title before processing
	cleanedTitle := cleanTitle(title)
	if cleanedTitle == "" {
		return // Skip if title becomes empty after cleaning
	}

	// Ensure we clear the renamingInProgress flag when done
	defer func() {
		m.timerMutex.Lock()
		m.renamingInProgress = false
		m.timerMutex.Unlock()
	}()

	// Get current branch name (full ref) - handle detached HEAD state
	output, err := m.gitService.operations.ExecuteGit(m.workDir, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		logger.Warnf("⚠️  Failed to get current branch name: %v", err)
		return
	}
	currentBranch := strings.TrimSpace(string(output))

	// If we get a commit hash (detached HEAD), try to get the actual branch name
	if len(currentBranch) == 40 && !strings.Contains(currentBranch, "/") {
		// Try to get the branch name from git status
		statusOutput, statusErr := m.gitService.operations.ExecuteGit(m.workDir, "status", "--porcelain=v1", "-b")
		if statusErr == nil {
			statusLines := strings.Split(strings.TrimSpace(string(statusOutput)), "\n")
			if len(statusLines) > 0 && strings.HasPrefix(statusLines[0], "## ") {
				branchInfo := strings.TrimPrefix(statusLines[0], "## ")
				// Extract branch name (before any "..." or "[")
				if dotIndex := strings.Index(branchInfo, "..."); dotIndex != -1 {
					currentBranch = branchInfo[:dotIndex]
				} else if bracketIndex := strings.Index(branchInfo, "["); bracketIndex != -1 {
					currentBranch = strings.TrimSpace(branchInfo[:bracketIndex])
				} else {
					currentBranch = branchInfo
				}
			}
		}
	}

	// Check if we're on a catnip branch that should be graduated
	if !git.IsCatnipBranch(currentBranch) {
		return
	}

	// Call Claude to generate a nice branch name
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := &models.CreateCompletionRequest{
		Prompt: fmt.Sprintf(`Based on this coding session title: "%s"

Generate a git branch name that:
1. Follows conventional patterns like: feature/add-auth, chore/update-deps, refactor/cleanup-api, bug/fix-login, docs/update-readme
2. Uses only lowercase letters, numbers, hyphens, and forward slashes
3. Is concise but descriptive (max 60 characters)
4. Common prefixes: feature, chore, refactor, bug, docs, test, style, perf, fix

Respond with ONLY the branch name, nothing else.`, cleanedTitle),
		SystemPrompt:     "You are a helpful assistant that generates git branch names. Respond only with the branch name, no explanation or additional text.",
		MaxTurns:         1,
		WorkingDirectory: m.workDir,
		Resume:           true, // Resume session to get context (service layer defaults to fork=true and haiku model)
		SuppressEvents:   true, // Suppress notifications during automated branch renaming
	}

	response, err := m.claudeService.CreateCompletion(ctx, req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Warnf("⏰ Claude request timed out after 60 seconds for title: %q", title)
		} else {
			logger.Warnf("⚠️  Failed to get branch name suggestion from Claude: %v", err)
		}
		return
	}

	if response == nil || response.Response == "" {
		logger.Warnf("⚠️  Claude returned empty response for branch name")
		return
	}

	newBranch := strings.TrimSpace(response.Response)

	// Basic validation - just check for valid git branch name
	if !m.isValidGitBranchName(newBranch) {
		logger.Warnf("⚠️  Claude suggested invalid branch name: %q", newBranch)
		return
	}

	// Check if the new branch name already exists and append numbers if needed
	logger.Debugf("🔍 Checking if branch %q exists in %s", newBranch, m.workDir)
	finalBranch := newBranch
	counter := 1
	for m.gitService.branchExists(m.workDir, finalBranch, false) ||
		m.gitService.branchExists(m.workDir, "refs/heads/"+finalBranch, false) {
		logger.Debugf("🔍 Branch %q exists, trying next...", finalBranch)
		finalBranch = fmt.Sprintf("%s-%d", newBranch, counter)
		counter++
		if counter > 100 { // Safety limit to prevent infinite loops
			logger.Warnf("⚠️  Too many similar branches exist for %q, skipping graduation", newBranch)
			return
		}
	}

	if finalBranch != newBranch {
		logger.Debugf("📝 Branch %q already exists, using %q instead", newBranch, finalBranch)
	}
	newBranch = finalBranch

	// Double-check that the final branch name doesn't exist
	if m.gitService.branchExists(m.workDir, newBranch, false) ||
		m.gitService.branchExists(m.workDir, "refs/heads/"+newBranch, false) {
		logger.Errorf("❌ ERROR: Branch %q still exists after collision detection!", newBranch)
		return
	}

	// Rename the branch to the new name using centralized state management
	logger.Debugf("🎓 Renaming branch %q to %q", currentBranch, newBranch)

	// Use cached worktree ID to avoid expensive lookup
	worktreeID := m.findWorktreeIDByPath()
	if worktreeID == "" {
		logger.Warnf("⚠️  Failed to find worktree ID for path %s", m.workDir)
		return
	}

	logger.Debugf("🔄 performBranchRename: calling RenameWorktreeBranch for %s -> %s", worktreeID, newBranch)
	if err := m.stateManager.RenameWorktreeBranch(worktreeID, newBranch, m.gitService.operations); err != nil {
		logger.Warnf("⚠️  Failed to rename branch: %v", err)
		return
	}

	logger.Infof("✅ Successfully renamed to branch %q", newBranch)
}

// findWorktreeIDByPath returns the cached worktree ID for this checkpoint manager
func (m *WorktreeCheckpointManager) findWorktreeIDByPath() string {
	if m.worktreeID == "" {
		logger.Warnf("⚠️  No cached worktree ID for path %s", m.workDir)
	}
	return m.worktreeID
}

// isValidGitBranchName validates basic git branch name rules
func (m *WorktreeCheckpointManager) isValidGitBranchName(branchName string) bool {
	// Check length (reasonable limits)
	if len(branchName) == 0 || len(branchName) > 100 {
		return false
	}

	// Use git's check-ref-format to validate
	_, err := m.gitService.operations.ExecuteCommand("git", "check-ref-format", "refs/heads/"+branchName)
	if err != nil {
		return false
	}

	// Additional checks for patterns we want to avoid
	invalidPatterns := []string{
		"..", "~", "^", ":", "?", "*", "[", "\\", " ",
	}

	for _, pattern := range invalidPatterns {
		if strings.Contains(branchName, pattern) {
			return false
		}
	}

	// Don't allow names that start or end with special characters
	if strings.HasPrefix(branchName, "/") || strings.HasSuffix(branchName, "/") ||
		strings.HasPrefix(branchName, ".") || strings.HasSuffix(branchName, ".") {
		return false
	}

	return true
}

// isCurrentBranchCatnip checks if the current branch in the worktree is a catnip branch
func (m *WorktreeCheckpointManager) isCurrentBranchCatnip() bool {
	// Get current branch name (full ref) - handle detached HEAD state
	output, err := m.gitService.operations.ExecuteGit(m.workDir, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		return false
	}
	currentBranch := strings.TrimSpace(string(output))

	// If we get a commit hash (detached HEAD), try to get the actual branch name
	if len(currentBranch) == 40 && !strings.Contains(currentBranch, "/") {
		// Try to get the branch name from git status
		statusOutput, statusErr := m.gitService.operations.ExecuteGit(m.workDir, "status", "--porcelain=v1", "-b")
		if statusErr == nil {
			statusLines := strings.Split(strings.TrimSpace(string(statusOutput)), "\n")
			if len(statusLines) > 0 && strings.HasPrefix(statusLines[0], "## ") {
				branchInfo := strings.TrimPrefix(statusLines[0], "## ")
				// Extract branch name (before any "..." or "[")
				if dotIndex := strings.Index(branchInfo, "..."); dotIndex != -1 {
					currentBranch = branchInfo[:dotIndex]
				} else if bracketIndex := strings.Index(branchInfo, "["); bracketIndex != -1 {
					currentBranch = strings.TrimSpace(branchInfo[:bracketIndex])
				} else {
					currentBranch = branchInfo
				}
			}
		}
	}

	// First check the state to see if this worktree has already been renamed
	if m.worktreeID != "" {
		if worktree, exists := m.stateManager.GetWorktree(m.worktreeID); exists && worktree != nil {
			// If branch is not in catnip format but has_been_renamed is false,
			// it means it was renamed outside our system - update the flag
			if !git.IsCatnipBranch(worktree.Branch) && !worktree.HasBeenRenamed {
				logger.Debugf("🔍 Branch %q appears to be renamed already, updating has_been_renamed flag", worktree.Branch)
				if err := m.stateManager.UpdateWorktree(m.worktreeID, map[string]interface{}{
					"has_been_renamed": true,
				}); err != nil {
					logger.Warnf("⚠️ Failed to update worktree has_been_renamed flag: %v", err)
				}
				return false
			}

			if worktree.HasBeenRenamed {
				logger.Debugf("🔍 Worktree %s already renamed, skipping further renames", m.worktreeID)
				return false
			}
			// If not renamed, check if current branch is a catnip branch using state data
			return git.IsCatnipBranch(worktree.Branch)
		}
	}

	// Fallback: check current git branch if state lookup fails
	return git.IsCatnipBranch(currentBranch)
}

// cleanTitle removes unwanted characters and symbols from titles
func cleanTitle(title string) string {
	// Remove the ✳ emoji symbol and any leading/trailing whitespace
	cleaned := strings.TrimSpace(strings.ReplaceAll(title, "✳", ""))
	// Remove any other common prefix symbols that might appear
	cleaned = strings.TrimSpace(strings.TrimPrefix(cleaned, "*"))
	return cleaned
}

// TriggerBranchRename manually triggers branch renaming for a worktree
func (s *ClaudeMonitorService) TriggerBranchRename(workDir string, customBranchName string) error {
	s.managersMutex.RLock()
	manager, exists := s.checkpointManagers[workDir]
	s.managersMutex.RUnlock()

	if !exists {
		return fmt.Errorf("no checkpoint manager found for worktree: %s", workDir)
	}

	// Get current branch name (full ref)
	output, err := s.gitService.operations.ExecuteGit(workDir, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current branch name: %v", err)
	}
	currentBranch := strings.TrimSpace(string(output))

	// Allow renaming any branch (not just catnip branches)
	// This enables users to rename branches multiple times if needed

	// If custom branch name is provided, validate it
	if customBranchName != "" {
		if !manager.isValidGitBranchName(customBranchName) {
			return fmt.Errorf("invalid branch name: %q", customBranchName)
		}

		// Check if the branch already exists and append numbers if needed
		finalBranch := customBranchName
		counter := 1
		for s.gitService.branchExists(workDir, finalBranch, false) ||
			s.gitService.branchExists(workDir, "refs/heads/"+finalBranch, false) {
			finalBranch = fmt.Sprintf("%s-%d", customBranchName, counter)
			counter++
			if counter > 100 { // Safety limit
				return fmt.Errorf("too many similar branches exist for %q", customBranchName)
			}
		}

		if finalBranch != customBranchName {
			logger.Debugf("📝 Branch %q already exists, using %q instead", customBranchName, finalBranch)
		}
		customBranchName = finalBranch

		// Rename directly to the custom name using centralized state management
		logger.Debugf("🎓 Renaming branch %q to custom name %q", currentBranch, customBranchName)

		// Use cached worktree ID to avoid expensive lookup
		worktreeID := manager.findWorktreeIDByPath()
		if worktreeID == "" {
			return fmt.Errorf("failed to find worktree ID for path %s", workDir)
		}

		logger.Debugf("🔄 TriggerBranchRename: calling RenameWorktreeBranch for %s -> %s", worktreeID, customBranchName)
		if err := s.stateManager.RenameWorktreeBranch(worktreeID, customBranchName, s.gitService.operations); err != nil {
			return fmt.Errorf("failed to rename branch: %v", err)
		}

		logger.Infof("✅ Successfully renamed to custom branch %q", customBranchName)
		return nil
	}

	// For automatic naming, we need a title
	manager.timerMutex.Lock()
	currentTitle := manager.currentTitle
	manager.timerMutex.Unlock()

	if currentTitle == "" {
		return fmt.Errorf("no title available for Claude-based naming. Please specify a custom branch name or use Claude to set a title first")
	}

	// Trigger the automatic branch rename
	go manager.checkAndRenameBranch(currentTitle)
	return nil
}

// startTodoMonitoring starts monitoring todos for all existing worktrees
func (s *ClaudeMonitorService) startTodoMonitoring() {
	logger.Debugf("🔍 Starting Todo monitoring for all worktrees")

	// Get all existing worktrees
	worktrees := s.gitService.stateManager.GetAllWorktrees()

	for worktreeID, worktree := range worktrees {
		s.startWorktreeTodoMonitor(worktreeID, worktree.Path)
	}
}

// startWorktreeTodoMonitor starts a todo monitor for a specific worktree
func (s *ClaudeMonitorService) startWorktreeTodoMonitor(worktreeID, worktreePath string) {
	s.todoMonitorsMutex.Lock()
	defer s.todoMonitorsMutex.Unlock()

	logger.Debugf("🔍 Attempting to start Todo monitor for worktree %s at path %s", worktreeID, worktreePath)

	// Check if monitor already exists
	if _, exists := s.todoMonitors[worktreePath]; exists {
		logger.Debugf("📊 Todo monitor already exists for %s", worktreePath)
		return
	}

	projectDirName := WorktreePathToProjectDir(worktreePath)
	logger.Debugf("🔍 Looking for project directory: %s", projectDirName)
	projectDir := s.findProjectDirectory(projectDirName)

	if projectDir == "" {
		logger.Debugf("⚠️  No Claude project directory found for %s (expected: %s)", worktreePath, projectDirName)
		return
	}

	logger.Debugf("📁 Found project directory: %s", projectDir)

	monitor := &WorktreeTodoMonitor{
		workDir:        worktreePath,
		projectDir:     projectDir,
		claudeService:  s.claudeService,
		parserService:  s.parserService,
		claudeMonitor:  s,
		gitService:     s.gitService,
		sessionService: s.sessionService,
		stopCh:         make(chan struct{}),
	}

	s.todoMonitors[worktreePath] = monitor
	go monitor.Start(worktreeID)

	logger.Debugf("📊 Started Todo monitor for worktree: %s", worktreePath)
}

// findProjectDirectory finds the Claude project directory for a given project name
func (s *ClaudeMonitorService) findProjectDirectory(projectDirName string) string {
	claudeProjectsDir := config.Runtime.GetClaudeProjectsDir()
	projectPath := filepath.Join(claudeProjectsDir, projectDirName)

	if stat, err := os.Stat(projectPath); err == nil && stat.IsDir() {
		return projectPath
	}

	return ""
}

// Start starts the todo monitoring for this worktree
func (m *WorktreeTodoMonitor) Start(worktreeID string) {
	// Initial check
	m.checkForTodoUpdates(worktreeID)

	// Start ticker for periodic checks (every 1 second)
	m.ticker = time.NewTicker(1 * time.Second)

	for {
		select {
		case <-m.ticker.C:
			m.checkForTodoUpdates(worktreeID)
		case <-m.stopCh:
			return
		}
	}
}

// Stop stops the todo monitor
func (m *WorktreeTodoMonitor) Stop() {
	if m.ticker != nil {
		m.ticker.Stop()
	}
	close(m.stopCh)
}

// checkForTodoUpdates checks for todo updates in the most recent session file
func (m *WorktreeTodoMonitor) checkForTodoUpdates(worktreeID string) {
	// Optimization 1: Skip inactive worktrees to save CPU
	// Only check worktrees that have active or running Claude sessions
	if m.sessionService != nil {
		activityState := m.sessionService.GetClaudeActivityState(m.workDir)
		if activityState != models.ClaudeActive && activityState != models.ClaudeRunning {
			return // Don't waste CPU on inactive worktrees
		}
	}

	// Optimization 2: Use parser directly - it already knows which session file to watch
	// This eliminates redundant os.ReadDir() calls every second
	if m.parserService == nil {
		return
	}

	reader, err := m.parserService.GetOrCreateParser(m.workDir)
	if err != nil {
		return // No session file available yet
	}

	// Read todos directly from parser's cached state
	todos := reader.GetTodos()
	if todos == nil {
		todos = []models.Todo{} // Ensure non-nil
	}

	// Convert todos to JSON for comparison
	todosJSON, err := json.Marshal(todos)
	if err != nil {
		logger.Warnf("⚠️  Failed to marshal todos: %v", err)
		return
	}
	todosJSONStr := string(todosJSON)

	// Check if todos have changed
	todosChanged := todosJSONStr != m.lastTodosJSON

	if todosChanged {
		// Todos have changed!
		logger.Debugf("📝 Todo update detected for worktree %s: %d todos", m.workDir, len(todos))

		// Update activity time for todo monitoring (but don't update Claude service activity
		// as todo monitoring is passive and should not keep workspaces "active")
		now := time.Now()
		m.claudeMonitor.activityMutex.Lock()
		m.claudeMonitor.lastActivityTimes[m.workDir] = now
		m.claudeMonitor.activityMutex.Unlock()

		// Note: We intentionally don't call UpdateActivity here because todo monitoring
		// is passive and should not prevent workspaces from transitioning to inactive

		// Update state
		m.lastTodos = todos
		m.lastTodosJSON = todosJSONStr

		// Check if we should trigger branch renaming based on todos
		m.checkTodoBasedBranchRenaming(todos)
	}

	// Build updates map - always check for user prompt changes during active sessions
	// This ensures the "You asked" section stays populated
	updates := make(map[string]interface{})

	if todosChanged {
		updates["todos"] = todos
	}

	// Always update the latest user prompt from history during active sessions
	// This ensures the "You asked" section stays populated even before todos are created
	latestUserPrompt, err := reader.GetLatestUserPrompt()
	if err == nil && latestUserPrompt != "" {
		updates["latest_user_prompt"] = latestUserPrompt
	}

	// Only update if we have changes
	if len(updates) > 0 {
		if err := m.gitService.stateManager.UpdateWorktree(worktreeID, updates); err != nil {
			logger.Warnf("⚠️  Failed to update worktree for %s: %v", worktreeID, err)
		}
	}

	// Also check for latest Claude message changes from parser
	m.checkForMessageUpdates(worktreeID, reader)
}

// readTodosFromEnd reads todos using the parser service
// Note: filePath parameter is ignored, we use m.workDir to find the parser
func (m *WorktreeTodoMonitor) readTodosFromEnd(filePath string) ([]models.Todo, error) {
	if m.parserService == nil {
		return nil, fmt.Errorf("parser service not initialized")
	}

	reader, err := m.parserService.GetOrCreateParser(m.workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get parser for worktree %s: %w", m.workDir, err)
	}

	return reader.GetTodos(), nil
}

// checkTodoBasedBranchRenaming checks if we should trigger branch renaming based on todos
func (m *WorktreeTodoMonitor) checkTodoBasedBranchRenaming(todos []models.Todo) {
	// Only proceed if we have todos
	if len(todos) == 0 {
		return
	}

	// Check if current branch is a catnip branch that should be graduated
	if !m.isCurrentBranchCatnip() {
		return
	}

	// Get or create a checkpoint manager for this worktree
	claudeMonitor := m.getClaudeMonitorService()
	if claudeMonitor == nil {
		logger.Warnf("⚠️  Claude monitor service not available for todo-based branch renaming")
		return
	}

	// Get or create checkpoint manager for this worktree
	claudeMonitor.managersMutex.Lock()
	manager, exists := claudeMonitor.checkpointManagers[m.workDir]
	if !exists {
		// Create new checkpoint manager for this worktree
		manager = claudeMonitor.createCheckpointManager(m.workDir)
		claudeMonitor.checkpointManagers[m.workDir] = manager
		logger.Debugf("📝 Created checkpoint manager for todo-based branch renaming: %s", m.workDir)
	}
	claudeMonitor.managersMutex.Unlock()

	// Generate a branch name based on the first todo item's content
	if len(todos) > 0 && todos[0].Content != "" {
		// Check if a nice branch mapping already exists for this catnip branch
		branchOutput, err := manager.gitService.operations.ExecuteGit(m.workDir, "symbolic-ref", "HEAD")
		if err != nil {
			logger.Warnf("⚠️  Failed to get current branch for todo-based renaming: %v", err)
			return
		}
		currentBranch := strings.TrimSpace(string(branchOutput))

		// Check if branch mapping already exists in git config
		configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(currentBranch, "/", "."))
		existingNiceBranch, err := manager.gitService.operations.GetConfig(m.workDir, configKey)
		if err == nil && strings.TrimSpace(existingNiceBranch) != "" {
			// Nice branch already exists, no need to rename
			logger.Debugf("🔍 Nice branch %q already exists for %s, skipping todo-based renaming", strings.TrimSpace(existingNiceBranch), currentBranch)
			return
		}

		// Only trigger if not already renaming
		manager.timerMutex.Lock()
		alreadyRenaming := manager.renamingInProgress
		if !alreadyRenaming {
			manager.renamingInProgress = true // Set flag to prevent multiple simultaneous attempts
			logger.Debugf("🎯 Todo-based branch renaming triggered for %s with todo: %q", m.workDir, todos[0].Content)
		}
		manager.timerMutex.Unlock()

		if !alreadyRenaming {
			// Trigger branch renaming in a goroutine
			go manager.checkAndRenameBranch(todos[0].Content)
		}
	}
}

// isCurrentBranchCatnip checks if the current branch in the worktree is a catnip branch
func (m *WorktreeTodoMonitor) isCurrentBranchCatnip() bool {
	// Get current branch name (full ref) - handle detached HEAD state
	output, err := m.gitService.operations.ExecuteGit(m.workDir, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		return false
	}
	currentBranch := strings.TrimSpace(string(output))

	// If we get a commit hash (detached HEAD), try to get the actual branch name
	if len(currentBranch) == 40 && !strings.Contains(currentBranch, "/") {
		// Try to get the branch name from git status
		statusOutput, statusErr := m.gitService.operations.ExecuteGit(m.workDir, "status", "--porcelain=v1", "-b")
		if statusErr == nil {
			statusLines := strings.Split(strings.TrimSpace(string(statusOutput)), "\n")
			if len(statusLines) > 0 && strings.HasPrefix(statusLines[0], "## ") {
				branchInfo := strings.TrimPrefix(statusLines[0], "## ")
				// Extract branch name (before any "..." or "[")
				if dotIndex := strings.Index(branchInfo, "..."); dotIndex != -1 {
					currentBranch = branchInfo[:dotIndex]
				} else if bracketIndex := strings.Index(branchInfo, "["); bracketIndex != -1 {
					currentBranch = strings.TrimSpace(branchInfo[:bracketIndex])
				} else {
					currentBranch = branchInfo
				}
			}
		}
	}

	return git.IsCatnipBranch(currentBranch)
}

// checkForMessageUpdates checks for new Claude messages and emits SSE events
func (m *WorktreeTodoMonitor) checkForMessageUpdates(worktreeID string, reader *parser.SessionFileReader) {
	// Get the latest message from the parser
	latestMsg := reader.GetLatestMessage()
	if latestMsg == nil {
		return // No messages yet
	}

	// Extract message content and metadata
	messageType := latestMsg.Type
	messageUUID := latestMsg.Uuid

	// Extract text content from the message using parser package function
	messageContent := parser.ExtractTextContent(*latestMsg)

	// Check if message has changed (compare UUIDs for efficiency)
	if messageUUID == m.lastMessageUUID {
		return // Same message as before
	}

	// Message has changed!
	logger.Debugf("💬 New Claude message detected for worktree %s (type: %s, length: %d)", m.workDir, messageType, len(messageContent))

	// Update tracking
	m.lastMessage = messageContent
	m.lastMessageType = messageType
	m.lastMessageUUID = messageUUID

	// Emit SSE event for the new message via the state manager
	if m.gitService != nil && m.gitService.stateManager != nil {
		m.gitService.stateManager.EmitClaudeMessage(m.workDir, worktreeID, messageContent, messageType)
		logger.Debugf("📡 Emitted claude:message SSE event for worktree %s", worktreeID)
	}
}

// getClaudeMonitorService returns the Claude monitor service instance
func (m *WorktreeTodoMonitor) getClaudeMonitorService() *ClaudeMonitorService {
	return m.claudeMonitor
}

// GetLastActivityTime returns the last activity time for a worktree path
func (s *ClaudeMonitorService) GetLastActivityTime(worktreePath string) time.Time {
	s.activityMutex.RLock()
	defer s.activityMutex.RUnlock()
	return s.lastActivityTimes[worktreePath]
}

// GetTodos returns the most recent todos for a worktree path
func (s *ClaudeMonitorService) GetTodos(worktreePath string) ([]models.Todo, error) {
	s.todoMonitorsMutex.RLock()
	monitor, exists := s.todoMonitors[worktreePath]
	s.todoMonitorsMutex.RUnlock()

	if exists && len(monitor.lastTodos) > 0 {
		return monitor.lastTodos, nil
	}

	// Fallback to direct read if monitor doesn't exist
	return s.claudeService.GetLatestTodos(worktreePath)
}

// GetLatestClaudeMessage returns the latest Claude message for a worktree path
func (s *ClaudeMonitorService) GetLatestClaudeMessage(worktreePath string) (message, messageType, uuid string) {
	s.todoMonitorsMutex.RLock()
	monitor, exists := s.todoMonitors[worktreePath]
	s.todoMonitorsMutex.RUnlock()

	if exists && monitor.lastMessageUUID != "" {
		return monitor.lastMessage, monitor.lastMessageType, monitor.lastMessageUUID
	}

	return "", "", ""
}

// OnWorktreeCreated handles when a new worktree is created
func (s *ClaudeMonitorService) OnWorktreeCreated(worktreeID, worktreePath string) {
	// Start monitoring todos for this new worktree
	s.startWorktreeTodoMonitor(worktreeID, worktreePath)
}

// OnWorktreeDeleted removes checkpoint manager and todo monitor for the deleted worktree
func (s *ClaudeMonitorService) OnWorktreeDeleted(worktreeID, worktreePath string) {
	logger.Infof("📂 Worktree deleted: %s -> %s", worktreeID, worktreePath)

	// Clean up checkpoint manager
	s.managersMutex.Lock()
	if manager, exists := s.checkpointManagers[worktreePath]; exists {
		manager.Stop()
		delete(s.checkpointManagers, worktreePath)
		logger.Debugf("📂 Removed checkpoint manager for: %s", worktreePath)
	}
	s.managersMutex.Unlock()

	// Clean up todo monitor
	s.todoMonitorsMutex.Lock()
	if monitor, exists := s.todoMonitors[worktreeID]; exists {
		monitor.Stop()
		delete(s.todoMonitors, worktreeID)
		logger.Debugf("📂 Removed todo monitor for: %s", worktreeID)
	}
	s.todoMonitorsMutex.Unlock()
}

// RefreshTodoMonitoring manually refreshes todo monitoring for all worktrees
func (s *ClaudeMonitorService) RefreshTodoMonitoring() {
	logger.Debugf("🔄 Manually refreshing Todo monitoring for all worktrees")
	s.startTodoMonitoring()
}

// GetClaudeService returns the claude service instance (used by PTY handler)
func (s *ClaudeMonitorService) GetClaudeService() *ClaudeService {
	return s.claudeService
}

// GetClaudeActivityState returns the Claude activity state based on hook events and PTY activity tracking
func (s *ClaudeMonitorService) GetClaudeActivityState(worktreePath string) models.ClaudeActivityState {
	now := time.Now()

	// Get all hook-based timestamps
	lastPromptSubmit := s.claudeService.GetLastUserPromptSubmit(worktreePath)
	lastToolUse := s.claudeService.GetLastPostToolUse(worktreePath)
	lastStop := s.claudeService.GetLastStopEvent(worktreePath)

	// Find the most recent activity event (prompt or tool use)
	var mostRecentActivity time.Time
	var activityType string
	if !lastPromptSubmit.IsZero() && (lastToolUse.IsZero() || lastPromptSubmit.After(lastToolUse)) {
		mostRecentActivity = lastPromptSubmit
		activityType = "UserPromptSubmit"
	} else if !lastToolUse.IsZero() {
		mostRecentActivity = lastToolUse
		activityType = "PostToolUse"
	}

	// STOP EVENT OVERRIDE: Recent Stop event immediately transitions to Running
	// regardless of recent activity (Stop indicates Claude finished generating)
	if !lastStop.IsZero() && now.Sub(lastStop) <= 10*time.Minute {
		// Only override if Stop is more recent than last activity, or if Stop is very recent (within 30 seconds)
		if mostRecentActivity.IsZero() || lastStop.After(mostRecentActivity) || now.Sub(lastStop) <= 30*time.Second {
			// logger.Debugf("🟡 Claude RUNNING in %s (Stop override: %v ago)", worktreePath, now.Sub(lastStop))
			return models.ClaudeRunning
		}
	}

	// ACTIVE: Claude is actively working (recent prompt or tool use, no recent Stop)
	if !mostRecentActivity.IsZero() && now.Sub(mostRecentActivity) <= 3*time.Minute {
		logger.Debugf("🟢 Claude ACTIVE in %s (last %s: %v ago)", worktreePath, activityType, now.Sub(mostRecentActivity))
		return models.ClaudeActive
	}

	// RUNNING: Session active but not generating (PTY activity)
	// Check if there's an active PTY session - real user interaction
	if s.sessionService.IsActiveSessionActive(worktreePath) {
		// logger.Debugf("🟡 Claude RUNNING in %s (active PTY session)", worktreePath)
		return models.ClaudeRunning
	}

	// Check if there's any recent PTY activity (within 10 minutes)
	if s.claudeService.IsActiveSession(worktreePath, 10*time.Minute) {
		// logger.Debugf("🟡 Claude RUNNING in %s (recent PTY activity)", worktreePath)
		return models.ClaudeRunning
	}

	// INACTIVE: No recent activity
	// logger.Debugf("⚪ Claude INACTIVE in %s", worktreePath)
	return models.ClaudeInactive
}
