package services

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bytes"
	"encoding/json"
	"github.com/fsnotify/fsnotify"
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

// WorktreeTodoMonitor monitors Todo updates for a single worktree
type WorktreeTodoMonitor struct {
	workDir       string
	projectDir    string
	claudeService *ClaudeService
	claudeMonitor *ClaudeMonitorService
	gitService    *GitService
	ticker        *time.Ticker
	stopCh        chan struct{}
	lastModTime   time.Time
	lastTodos     []models.Todo
	lastTodosJSON string // JSON representation for comparison
}

// NewClaudeMonitorService creates a new Claude monitor service
func NewClaudeMonitorService(gitService *GitService, sessionService *SessionService, claudeService *ClaudeService, stateManager *WorktreeStateManager) *ClaudeMonitorService {
	// Get log path from environment or use runtime-appropriate default
	titlesLogPath := os.Getenv("CATNIP_TITLE_LOG")
	if titlesLogPath == "" {
		titlesLogPath = filepath.Join(config.Runtime.VolumeDir, "title_events.log")
	}

	return &ClaudeMonitorService{
		gitService:         gitService,
		sessionService:     sessionService,
		claudeService:      claudeService,
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
	logger.Info("🚀 Starting Claude monitor service")

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
		if !os.IsNotExist(err) {
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

		timestamp := parts[0]
		// pid := parts[1]
		cwd := parts[2]
		title := parts[3]

		logger.Debugf("🪧 Title change detected at %s: %q in %s", timestamp, title, cwd)

		// Check if this is a worktree directory
		if s.isWorktreeDirectory(cwd) {
			// Clean the title before processing
			cleanedTitle := cleanTitle(title)
			if cleanedTitle != "" { // Only process if title isn't empty after cleaning
				s.handleTitleChange(cwd, cleanedTitle, "log")
			}
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

	// Update activity time for title changes and also update Claude service activity
	now := time.Now()
	s.activityMutex.Lock()
	s.lastActivityTimes[workDir] = now
	s.activityMutex.Unlock()

	// Also update the Claude service activity tracking
	s.claudeService.UpdateActivity(workDir)

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

	manager.HandleTitleChange(newTitle)
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
		Resume:           true,
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

	// Convert worktree path to project directory
	// Claude replaces both "/" and "." with "-"
	projectDirName := strings.ReplaceAll(worktreePath, "/", "-")
	projectDirName = strings.ReplaceAll(projectDirName, ".", "-")
	projectDirName = strings.TrimPrefix(projectDirName, "-")
	projectDirName = "-" + projectDirName
	logger.Debugf("🔍 Looking for project directory: %s", projectDirName)
	projectDir := s.findProjectDirectory(projectDirName)

	if projectDir == "" {
		logger.Warnf("⚠️  No Claude project directory found for %s (expected: %s)", worktreePath, projectDirName)
		return
	}

	logger.Debugf("📁 Found project directory: %s", projectDir)

	monitor := &WorktreeTodoMonitor{
		workDir:       worktreePath,
		projectDir:    projectDir,
		claudeService: s.claudeService,
		claudeMonitor: s,
		gitService:    s.gitService,
		stopCh:        make(chan struct{}),
	}

	s.todoMonitors[worktreePath] = monitor
	go monitor.Start(worktreeID)

	logger.Debugf("📊 Started Todo monitor for worktree: %s", worktreePath)
}

// findProjectDirectory finds the Claude project directory for a given project name
func (s *ClaudeMonitorService) findProjectDirectory(projectDirName string) string {
	claudeProjectsDir := filepath.Join(config.Runtime.HomeDir, ".claude", "projects")
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
	// Find the most recently modified JSONL file
	latestFile, modTime, err := m.findLatestSessionFile()
	if err != nil {
		return // No session files yet
	}

	// Check if file has been modified since last check
	if !modTime.After(m.lastModTime) {
		return // No changes
	}

	// Read todos from the end of the file
	todos, err := m.readTodosFromEnd(latestFile)
	if err != nil {
		logger.Warnf("⚠️  Failed to read todos from %s: %v", latestFile, err)
		return
	}

	// Convert todos to JSON for comparison
	todosJSON, err := json.Marshal(todos)
	if err != nil {
		logger.Warnf("⚠️  Failed to marshal todos: %v", err)
		return
	}
	todosJSONStr := string(todosJSON)

	// Check if todos have changed
	if todosJSONStr == m.lastTodosJSON {
		m.lastModTime = modTime // Update mod time even if content is same
		return                  // No change in todos
	}

	// Todos have changed!
	logger.Debugf("📝 Todo update detected for worktree %s: %d todos", m.workDir, len(todos))

	// Update activity time to prevent session cleanup
	now := time.Now()
	m.claudeMonitor.activityMutex.Lock()
	m.claudeMonitor.lastActivityTimes[m.workDir] = now
	m.claudeMonitor.activityMutex.Unlock()

	// Also update the Claude service activity tracking
	m.claudeMonitor.claudeService.UpdateActivity(m.workDir)

	// Update state
	m.lastModTime = modTime
	m.lastTodos = todos
	m.lastTodosJSON = todosJSONStr

	// Check if we should trigger branch renaming based on todos
	m.checkTodoBasedBranchRenaming(todos)

	// Update worktree state
	updates := map[string]interface{}{
		"todos": todos,
	}

	if err := m.gitService.stateManager.UpdateWorktree(worktreeID, updates); err != nil {
		logger.Warnf("⚠️  Failed to update worktree todos for %s: %v", worktreeID, err)
	}
}

// findLatestSessionFile finds the most recently modified JSONL file in the project directory
func (m *WorktreeTodoMonitor) findLatestSessionFile() (string, time.Time, error) {
	entries, err := os.ReadDir(m.projectDir)
	if err != nil {
		return "", time.Time{}, err
	}

	var latestFile string
	var latestModTime time.Time

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			filePath := filepath.Join(m.projectDir, entry.Name())
			info, err := entry.Info()
			if err != nil {
				continue
			}

			if info.ModTime().After(latestModTime) {
				latestFile = filePath
				latestModTime = info.ModTime()
			}
		}
	}

	if latestFile == "" {
		return "", time.Time{}, fmt.Errorf("no session files found")
	}

	return latestFile, latestModTime, nil
}

// readTodosFromEnd reads todos from the end of a JSONL file efficiently
func (m *WorktreeTodoMonitor) readTodosFromEnd(filePath string) ([]models.Todo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := stat.Size()

	// Read from the end in chunks
	const chunkSize = 64 * 1024 // 64KB chunks
	var todos []models.Todo
	var foundTodos bool

	// Start from the end and work backwards
	for offset := fileSize; offset > 0 && !foundTodos; {
		// Calculate chunk boundaries
		readSize := int64(chunkSize)
		if offset < chunkSize {
			readSize = offset
		}
		offset -= readSize

		// Seek to position
		if _, err := file.Seek(offset, 0); err != nil {
			return nil, err
		}

		// Read chunk
		chunk := make([]byte, int(readSize))
		if _, err := file.Read(chunk); err != nil {
			return nil, err
		}

		// Find line boundaries
		lines := m.extractCompleteLines(chunk, offset == 0)

		// Process lines in reverse order (newest first)
		for i := len(lines) - 1; i >= 0 && !foundTodos; i-- {
			line := lines[i]
			if len(line) == 0 {
				continue
			}

			// Try to parse as Claude message
			var message models.ClaudeSessionMessage
			if err := json.Unmarshal(line, &message); err != nil {
				continue
			}

			// Check if this contains TodoWrite
			if message.Type == "assistant" && message.Message != nil {
				if todosFound := m.extractTodosFromMessage(message.Message); todosFound != nil {
					todos = todosFound
					foundTodos = true
					break
				}
			}
		}
	}

	return todos, nil
}

// extractCompleteLines extracts complete JSON lines from a chunk
func (m *WorktreeTodoMonitor) extractCompleteLines(chunk []byte, isStart bool) [][]byte {
	var lines [][]byte

	// Split by newlines
	rawLines := bytes.Split(chunk, []byte("\n"))

	for i, line := range rawLines {
		// Skip incomplete lines unless it's the start of file
		if i == 0 && !isStart {
			continue // First line might be incomplete
		}
		if i == len(rawLines)-1 && len(line) == 0 {
			continue // Last empty line
		}

		lines = append(lines, line)
	}

	return lines
}

// extractTodosFromMessage extracts todos from a Claude message
func (m *WorktreeTodoMonitor) extractTodosFromMessage(messageData map[string]interface{}) []models.Todo {
	content, exists := messageData["content"]
	if !exists {
		return nil
	}

	contentArray, ok := content.([]interface{})
	if !ok {
		return nil
	}

	for _, contentItem := range contentArray {
		contentMap, ok := contentItem.(map[string]interface{})
		if !ok {
			continue
		}

		if contentType, exists := contentMap["type"]; exists && contentType == "tool_use" {
			if name, exists := contentMap["name"]; exists && name == "TodoWrite" {
				if input, exists := contentMap["input"]; exists {
					if inputMap, ok := input.(map[string]interface{}); ok {
						if todos, exists := inputMap["todos"]; exists {
							if todosArray, ok := todos.([]interface{}); ok {
								var parsedTodos []models.Todo
								for _, todoItem := range todosArray {
									if todoMap, ok := todoItem.(map[string]interface{}); ok {
										todo := models.Todo{}
										if id, exists := todoMap["id"]; exists {
											if idStr, ok := id.(string); ok {
												todo.ID = idStr
											}
										}
										if content, exists := todoMap["content"]; exists {
											if contentStr, ok := content.(string); ok {
												todo.Content = contentStr
											}
										}
										if status, exists := todoMap["status"]; exists {
											if statusStr, ok := status.(string); ok {
												todo.Status = statusStr
											}
										}
										if priority, exists := todoMap["priority"]; exists {
											if priorityStr, ok := priority.(string); ok {
												todo.Priority = priorityStr
											}
										}
										parsedTodos = append(parsedTodos, todo)
									}
								}
								return parsedTodos
							}
						}
					}
				}
			}
		}
	}

	return nil
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

// GetClaudeActivityState returns the Claude activity state based on PTY activity tracking
func (s *ClaudeMonitorService) GetClaudeActivityState(worktreePath string) models.ClaudeActivityState {
	// Check activity using the new Claude service tracking
	if s.claudeService.IsActiveSession(worktreePath, 2*time.Minute) {
		return models.ClaudeActive
	}

	// Check if there's any recent activity (within 10 minutes) to determine if "running"
	if s.claudeService.IsActiveSession(worktreePath, 10*time.Minute) {
		return models.ClaudeRunning
	}

	// No recent activity
	return models.ClaudeInactive
}
