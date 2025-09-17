package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
)

// CodexMonitor monitors Codex sessions and simulates hook events
type CodexMonitor struct {
	codexAgent        *CodexAgent
	stateManager      *WorktreeStateManager
	gitService        *GitService
	watcher           *fsnotify.Watcher
	stopCh            chan struct{}
	sessionWatchers   map[string]*CodexSessionWatcher
	watchersMutex     sync.RWMutex
	eventHandlers     []func(*models.AgentEvent) error
	eventHandlerMutex sync.RWMutex
}

// CodexSessionWatcher watches a single Codex session for changes
type CodexSessionWatcher struct {
	sessionFile  string
	worktreePath string
	sessionID    string
	lastSize     int64
	lastModTime  time.Time
	lastTodos    []models.Todo
	// lastTitle    string // unused but kept for potential future use
	isActive  bool
	startTime time.Time
	stopCh    chan struct{}
	monitor   *CodexMonitor
}

// NewCodexMonitor creates a new Codex monitor
func NewCodexMonitor(codexAgent *CodexAgent, stateManager *WorktreeStateManager, gitService *GitService) *CodexMonitor {
	return &CodexMonitor{
		codexAgent:      codexAgent,
		stateManager:    stateManager,
		gitService:      gitService,
		stopCh:          make(chan struct{}),
		sessionWatchers: make(map[string]*CodexSessionWatcher),
	}
}

// Start starts the Codex monitor
func (cm *CodexMonitor) Start() error {
	logger.Infof("🚀 Starting Codex monitor service")

	// Create file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	cm.watcher = watcher

	// Watch the Codex sessions directory
	sessionsDir := cm.codexAgent.codexSessionsDir
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Add recursive watching for the sessions directory
	if err := cm.addRecursiveWatch(sessionsDir); err != nil {
		return fmt.Errorf("failed to watch sessions directory: %w", err)
	}

	// Watch the history file
	historyDir := filepath.Dir(cm.codexAgent.codexHistoryPath)
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	if err := cm.watcher.Add(historyDir); err != nil {
		return fmt.Errorf("failed to watch history directory: %w", err)
	}

	// Start the file watching goroutine
	go cm.watchFiles()

	// Start monitoring existing sessions
	go cm.scanExistingSessions()

	return nil
}

// Stop stops the Codex monitor
func (cm *CodexMonitor) Stop() {
	logger.Info("🛑 Stopping Codex monitor service")
	close(cm.stopCh)

	if cm.watcher != nil {
		cm.watcher.Close()
	}

	// Stop all session watchers
	cm.watchersMutex.Lock()
	for _, watcher := range cm.sessionWatchers {
		close(watcher.stopCh)
	}
	cm.sessionWatchers = make(map[string]*CodexSessionWatcher)
	cm.watchersMutex.Unlock()
}

// OnWorktreeCreated handles worktree creation
func (cm *CodexMonitor) OnWorktreeCreated(worktreeID, worktreePath string) {
	logger.Debugf("📂 Codex monitor: worktree created %s -> %s", worktreeID, worktreePath)
	// Start monitoring for new sessions in this worktree
	go cm.scanForWorktreeSessions(worktreePath)
}

// OnWorktreeDeleted handles worktree deletion
func (cm *CodexMonitor) OnWorktreeDeleted(worktreeID, worktreePath string) {
	logger.Debugf("📂 Codex monitor: worktree deleted %s -> %s", worktreeID, worktreePath)

	// Stop watching sessions for this worktree
	cm.watchersMutex.Lock()
	defer cm.watchersMutex.Unlock()

	for sessionID, watcher := range cm.sessionWatchers {
		if watcher.worktreePath == worktreePath {
			close(watcher.stopCh)
			delete(cm.sessionWatchers, sessionID)
		}
	}
}

// GetLastActivityTime gets last activity time for a worktree
func (cm *CodexMonitor) GetLastActivityTime(worktreePath string) time.Time {
	return cm.codexAgent.GetLastActivity(worktreePath)
}

// GetTodos gets todos for a worktree
func (cm *CodexMonitor) GetTodos(worktreePath string) ([]models.Todo, error) {
	return cm.codexAgent.GetLatestTodos(worktreePath)
}

// GetActivityState gets activity state for a worktree
func (cm *CodexMonitor) GetActivityState(worktreePath string) models.ClaudeActivityState {
	// Check if there's an active session
	cm.watchersMutex.RLock()
	defer cm.watchersMutex.RUnlock()

	now := time.Now()
	for _, watcher := range cm.sessionWatchers {
		if watcher.worktreePath == worktreePath && watcher.isActive {
			// Check recent activity
			if now.Sub(watcher.lastModTime) <= 3*time.Minute {
				return models.ClaudeActive
			} else if now.Sub(watcher.lastModTime) <= 10*time.Minute {
				return models.ClaudeRunning
			}
		}
	}

	return models.ClaudeInactive
}

// TriggerBranchRename triggers branch renaming (not implemented for Codex)
func (cm *CodexMonitor) TriggerBranchRename(workDir string, customBranchName string) error {
	// Codex doesn't have automatic branch renaming like Claude
	// This could be implemented by analyzing session content and using git operations
	return fmt.Errorf("branch renaming not implemented for Codex")
}

// RefreshTodoMonitoring refreshes todo monitoring
func (cm *CodexMonitor) RefreshTodoMonitoring() {
	logger.Debugf("🔄 Refreshing Codex todo monitoring")
	go cm.scanExistingSessions()
}

// AddEventHandler adds an event handler
func (cm *CodexMonitor) AddEventHandler(handler func(*models.AgentEvent) error) {
	cm.eventHandlerMutex.Lock()
	defer cm.eventHandlerMutex.Unlock()
	cm.eventHandlers = append(cm.eventHandlers, handler)
}

// emitEvent emits a Codex event
func (cm *CodexMonitor) emitEvent(event *models.AgentEvent) {
	// Update activity
	cm.codexAgent.UpdateActivity(event.WorkingDirectory)

	// Call event handlers
	cm.eventHandlerMutex.RLock()
	defer cm.eventHandlerMutex.RUnlock()

	for _, handler := range cm.eventHandlers {
		if err := handler(event); err != nil {
			logger.Warnf("Codex event handler error: %v", err)
		}
	}

	// Also notify the agent
	if err := cm.codexAgent.HandleEvent(event); err != nil {
		logger.Errorf("Failed to handle agent event: %v", err)
	}
}

// watchFiles watches for file system changes
func (cm *CodexMonitor) watchFiles() {
	for {
		select {
		case event, ok := <-cm.watcher.Events:
			if !ok {
				return
			}
			cm.handleFileEvent(event)

		case err, ok := <-cm.watcher.Errors:
			if !ok {
				return
			}
			logger.Warnf("⚠️ Codex file watcher error: %v", err)

		case <-cm.stopCh:
			return
		}
	}
}

// handleFileEvent handles a file system event
func (cm *CodexMonitor) handleFileEvent(event fsnotify.Event) {
	logger.Debugf("📁 Codex file event: %s %s", event.Op, event.Name)

	// Handle session file changes
	if strings.HasSuffix(event.Name, ".jsonl") && strings.Contains(event.Name, cm.codexAgent.codexSessionsDir) {
		if event.Op&fsnotify.Create == fsnotify.Create {
			cm.handleNewSessionFile(event.Name)
		} else if event.Op&fsnotify.Write == fsnotify.Write {
			cm.handleSessionFileUpdate(event.Name)
		}
	}

	// Handle history file changes
	if event.Name == cm.codexAgent.codexHistoryPath && event.Op&fsnotify.Write == fsnotify.Write {
		cm.handleHistoryUpdate()
	}

	// Handle new directories (for date-based organization)
	if event.Op&fsnotify.Create == fsnotify.Create {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			if strings.Contains(event.Name, cm.codexAgent.codexSessionsDir) {
				if err := cm.watcher.Add(event.Name); err != nil {
					logger.Errorf("Failed to add new session file to watcher: %v", err)
				}
			}
		}
	}
}

// handleNewSessionFile handles creation of a new session file
func (cm *CodexMonitor) handleNewSessionFile(sessionFile string) {
	logger.Debugf("📝 New Codex session file: %s", sessionFile)

	// Parse session to get metadata
	worktreePath, sessionMeta, err := cm.codexAgent.getWorktreePathAndMetaFromSession(sessionFile)
	if err != nil {
		logger.Warnf("⚠️ Failed to parse new session file: %v", err)
		return
	}

	// Create session watcher
	watcher := &CodexSessionWatcher{
		sessionFile:  sessionFile,
		worktreePath: worktreePath,
		sessionID:    sessionMeta.ID,
		isActive:     true,
		startTime:    time.Now(),
		stopCh:       make(chan struct{}),
		monitor:      cm,
	}

	// Add to watchers map
	cm.watchersMutex.Lock()
	cm.sessionWatchers[sessionMeta.ID] = watcher
	cm.watchersMutex.Unlock()

	// Start watching this session
	go watcher.watch()

	// Emit SessionStart event
	cm.emitEvent(&models.AgentEvent{
		EventType:        "SessionStart",
		WorkingDirectory: worktreePath,
		SessionID:        sessionMeta.ID,
		AgentType:        models.AgentTypeCodex,
		Timestamp:        time.Now(),
		Data: map[string]interface{}{
			"session_file": sessionFile,
			"cli_version":  sessionMeta.CLIVersion,
		},
	})
}

// handleSessionFileUpdate handles updates to an existing session file
func (cm *CodexMonitor) handleSessionFileUpdate(sessionFile string) {
	// Find the session watcher for this file
	cm.watchersMutex.RLock()
	var watcher *CodexSessionWatcher
	for _, w := range cm.sessionWatchers {
		if w.sessionFile == sessionFile {
			watcher = w
			break
		}
	}
	cm.watchersMutex.RUnlock()

	if watcher != nil {
		watcher.handleUpdate()
	}
}

// handleHistoryUpdate handles updates to the history file
func (cm *CodexMonitor) handleHistoryUpdate() {
	logger.Debugf("📝 Codex history file updated")

	// Read the latest entry from history
	entry, err := cm.getLatestHistoryEntry()
	if err != nil {
		logger.Warnf("⚠️ Failed to read latest history entry: %v", err)
		return
	}

	if entry != nil {
		// Emit UserPromptSubmit event
		cm.emitEvent(&models.AgentEvent{
			EventType:        "UserPromptSubmit",
			WorkingDirectory: "", // Will be filled by finding the session
			SessionID:        entry.SessionID,
			AgentType:        models.AgentTypeCodex,
			Timestamp:        time.Unix(entry.Timestamp, 0),
			Data: map[string]interface{}{
				"prompt": entry.Text,
			},
		})
	}
}

// scanExistingSessions scans for existing sessions and starts monitoring them
func (cm *CodexMonitor) scanExistingSessions() {
	logger.Debugf("🔍 Scanning existing Codex sessions")

	err := filepath.Walk(cm.codexAgent.codexSessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Check if we're already watching this session
		sessionID := cm.extractSessionIDFromPath(path)
		cm.watchersMutex.RLock()
		_, exists := cm.sessionWatchers[sessionID]
		cm.watchersMutex.RUnlock()

		if !exists && cm.isRecentSession(info.ModTime()) {
			cm.handleNewSessionFile(path)
		}

		return nil
	})

	if err != nil {
		logger.Warnf("⚠️ Error scanning existing sessions: %v", err)
	}
}

// scanForWorktreeSessions scans for sessions belonging to a specific worktree
func (cm *CodexMonitor) scanForWorktreeSessions(worktreePath string) {
	logger.Debugf("🔍 Scanning Codex sessions for worktree: %s", worktreePath)

	err := filepath.Walk(cm.codexAgent.codexSessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Check if this session belongs to the worktree
		sessionWorktreePath, err := cm.codexAgent.getWorktreePathFromSession(path)
		if err != nil {
			return nil
		}

		if sessionWorktreePath == worktreePath && cm.isRecentSession(info.ModTime()) {
			sessionID := cm.extractSessionIDFromPath(path)
			cm.watchersMutex.RLock()
			_, exists := cm.sessionWatchers[sessionID]
			cm.watchersMutex.RUnlock()

			if !exists {
				cm.handleNewSessionFile(path)
			}
		}

		return nil
	})

	if err != nil {
		logger.Warnf("⚠️ Error scanning worktree sessions: %v", err)
	}
}

// addRecursiveWatch adds watchers for a directory and all subdirectories
func (cm *CodexMonitor) addRecursiveWatch(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return cm.watcher.Add(path)
		}

		return nil
	})
}

// Helper methods

func (cm *CodexMonitor) extractSessionIDFromPath(path string) string {
	filename := filepath.Base(path)
	// Extract UUID from filename like "rollout-2025-09-17T11-22-47-87bdc047-fc68-4279-9a34-8e51531f361f.jsonl"
	parts := strings.Split(filename, "-")
	if len(parts) >= 5 {
		// UUID is typically the last 5 parts before .jsonl
		uuid := strings.Join(parts[len(parts)-5:], "-")
		return strings.TrimSuffix(uuid, ".jsonl")
	}
	return strings.TrimSuffix(filename, ".jsonl")
}

func (cm *CodexMonitor) isRecentSession(modTime time.Time) bool {
	// Consider sessions from the last 24 hours as potentially active
	return time.Since(modTime) <= 24*time.Hour
}

func (cm *CodexMonitor) getLatestHistoryEntry() (*CodexHistoryEntry, error) {
	file, err := os.Open(cm.codexAgent.codexHistoryPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lastEntry *CodexHistoryEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry CodexHistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		lastEntry = &entry
	}

	return lastEntry, nil
}

// CodexSessionWatcher methods

// watch monitors a single session file for changes
func (w *CodexSessionWatcher) watch() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.checkForUpdates()
		case <-w.stopCh:
			return
		}
	}
}

// handleUpdate handles a file system update to the session file
func (w *CodexSessionWatcher) handleUpdate() {
	w.checkForUpdates()
}

// checkForUpdates checks for updates to the session file
func (w *CodexSessionWatcher) checkForUpdates() {
	info, err := os.Stat(w.sessionFile)
	if err != nil {
		return // File might be deleted
	}

	// Check if file has been modified
	if !info.ModTime().After(w.lastModTime) {
		return
	}

	w.lastModTime = info.ModTime()

	// Check if file size has changed significantly
	if info.Size() <= w.lastSize {
		return
	}

	w.lastSize = info.Size()

	// Parse the session file for new content
	w.parseSessionUpdates()

	// If we haven't seen activity for a while, mark session as stopped
	if time.Since(w.lastModTime) > 5*time.Minute && w.isActive {
		w.isActive = false
		w.emitStopEvent()
	}
}

// parseSessionUpdates parses the session file for new content
func (w *CodexSessionWatcher) parseSessionUpdates() {
	// Read the session file and look for new messages
	file, err := os.Open(w.sessionFile)
	if err != nil {
		return
	}
	defer file.Close()

	var hasNewAssistantMessage bool
	var newTodos []models.Todo

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var msg CodexMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		// Check for assistant messages (PostToolUse equivalent)
		if msg.Type == "response_item" && msg.Payload.Type == "message" && msg.Payload.Role == "assistant" {
			hasNewAssistantMessage = true
		}

		// Extract todos from content (if any)
		// TODO: Implement todo extraction from Codex messages
	}

	// Emit events based on what we found
	if hasNewAssistantMessage {
		w.emitPostToolUseEvent()
	}

	// Check for todo changes
	if len(newTodos) > 0 && !w.todosEqual(w.lastTodos, newTodos) {
		w.lastTodos = newTodos
		// TODO: Emit todo update event
	}
}

// emitPostToolUseEvent emits a PostToolUse event
func (w *CodexSessionWatcher) emitPostToolUseEvent() {
	w.monitor.emitEvent(&models.AgentEvent{
		EventType:        "PostToolUse",
		WorkingDirectory: w.worktreePath,
		SessionID:        w.sessionID,
		AgentType:        models.AgentTypeCodex,
		Timestamp:        time.Now(),
		Data:             map[string]interface{}{},
	})
}

// emitStopEvent emits a Stop event
func (w *CodexSessionWatcher) emitStopEvent() {
	w.monitor.emitEvent(&models.AgentEvent{
		EventType:        "Stop",
		WorkingDirectory: w.worktreePath,
		SessionID:        w.sessionID,
		AgentType:        models.AgentTypeCodex,
		Timestamp:        time.Now(),
		Data:             map[string]interface{}{},
	})
}

// todosEqual compares two todo slices
func (w *CodexSessionWatcher) todosEqual(a, b []models.Todo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Content != b[i].Content || a[i].Status != b[i].Status {
			return false
		}
	}
	return true
}
