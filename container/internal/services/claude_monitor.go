package services

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
)

// ClaudeMonitorService monitors all worktrees for Claude sessions and manages checkpoints
type ClaudeMonitorService struct {
	gitService         *GitService
	sessionService     *SessionService
	claudeService      *ClaudeService
	checkpointManagers map[string]*WorktreeCheckpointManager // Map of worktree path to checkpoint manager
	managersMutex      sync.RWMutex
	titlesWatcher      *fsnotify.Watcher
	stopCh             chan struct{}
	titlesLogPath      string
	lastLogPosition    int64
	recentTitles       map[string]titleEvent // Track recent titles to avoid duplicates
	recentTitlesMutex  sync.RWMutex
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
	checkpointManager  *git.SessionCheckpointManager
	gitService         *GitService
	sessionService     *SessionService
	claudeService      *ClaudeService
	currentTitle       string
	checkpointTimer    *time.Timer
	timerMutex         sync.Mutex
	renamingInProgress bool // Track if a rename is currently in progress
}

// NewClaudeMonitorService creates a new Claude monitor service
func NewClaudeMonitorService(gitService *GitService, sessionService *SessionService, claudeService *ClaudeService) *ClaudeMonitorService {
	// Get log path from environment or use default
	titlesLogPath := os.Getenv("CATNIP_TITLE_LOG")
	if titlesLogPath == "" {
		titlesLogPath = "/home/catnip/.catnip/title_events.log"
	}

	return &ClaudeMonitorService{
		gitService:         gitService,
		sessionService:     sessionService,
		claudeService:      claudeService,
		checkpointManagers: make(map[string]*WorktreeCheckpointManager),
		stopCh:             make(chan struct{}),
		titlesLogPath:      titlesLogPath,
		recentTitles:       make(map[string]titleEvent),
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

// createCheckpointManager creates a checkpoint manager for a worktree
func (s *ClaudeMonitorService) createCheckpointManager(workDir string) *WorktreeCheckpointManager {
	return &WorktreeCheckpointManager{
		workDir:           workDir,
		checkpointManager: git.NewSessionCheckpointManager(workDir, NewGitServiceAdapter(s.gitService), NewSessionServiceAdapter(s.sessionService)),
		gitService:        s.gitService,
		sessionService:    s.sessionService,
		claudeService:     s.claudeService,
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
		log.Printf("⚠️  Failed to get current branch name: %v", err)
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
			log.Printf("⏰ Claude request timed out after 60 seconds for title: %q", title)
		} else {
			log.Printf("⚠️  Failed to get branch name suggestion from Claude: %v", err)
		}
		return
	}

	if response == nil || response.Response == "" {
		log.Printf("⚠️  Claude returned empty response for branch name")
		return
	}

	newBranch := strings.TrimSpace(response.Response)

	// Basic validation - just check for valid git branch name
	if !m.isValidGitBranchName(newBranch) {
		log.Printf("⚠️  Claude suggested invalid branch name: %q", newBranch)
		return
	}

	// Check if the new branch name already exists
	if m.gitService.branchExists(m.workDir, newBranch, false) {
		log.Printf("⚠️  Branch %q already exists, skipping graduation", newBranch)
		return
	}

	// Rename the branch to the new name
	log.Printf("🎓 Renaming branch %q to %q", currentBranch, newBranch)
	if err := m.renameBranch(currentBranch, newBranch); err != nil {
		log.Printf("⚠️  Failed to rename branch: %v", err)
		return
	}

	log.Printf("✅ Successfully renamed to branch %q", newBranch)
}

// renameBranch creates a new branch from the current branch and switches to it
func (m *WorktreeCheckpointManager) renameBranch(oldBranchName, newBranchName string) error {
	// Create and switch to new regular branch in one command - this works even with non-refs/heads branches
	if _, err := m.gitService.operations.ExecuteGit(m.workDir, "checkout", "-b", newBranchName); err != nil {
		return fmt.Errorf("failed to create and checkout new branch %q: %v", newBranchName, err)
	}

	// Remove the old branch ref (optional - could leave it as a backup)
	if err := m.gitService.operations.DeleteBranch(m.workDir, oldBranchName, true); err != nil {
		log.Printf("⚠️  Failed to delete old branch ref %q: %v", oldBranchName, err)
		// Don't fail the whole operation for this
	}

	return nil
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

		// Check if the branch already exists
		if s.gitService.branchExists(workDir, customBranchName, false) {
			return fmt.Errorf("branch %q already exists", customBranchName)
		}

		// Rename directly to the custom name
		log.Printf("🎓 Renaming branch %q to custom name %q", currentBranch, customBranchName)
		if err := manager.renameBranch(currentBranch, customBranchName); err != nil {
			return fmt.Errorf("failed to rename branch: %v", err)
		}

		log.Printf("✅ Successfully renamed to custom branch %q", customBranchName)
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
