package services

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
)

// WorktreeStatusCache provides fast worktree status lookups with background updates
type WorktreeStatusCache struct {
	mu           sync.RWMutex
	statuses     map[string]*CachedWorktreeStatus // key: worktreeID
	operations   git.Operations
	stateManager *WorktreeStateManager        // Central state manager
	watchers     map[string]*fsnotify.Watcher // key: worktreePath
	ctx          context.Context
	cancel       context.CancelFunc
	updateQueue  chan string                             // worktreeID queue for background updates
	pathResolver func(string) (string, *models.Worktree) // Resolves worktreeID to path and worktree
}

// CachedWorktreeStatus represents cached git status for a worktree
type CachedWorktreeStatus struct {
	WorktreeID       string    `json:"worktree_id"`
	IsDirty          *bool     `json:"is_dirty"`       // nil = not cached yet
	HasConflicts     *bool     `json:"has_conflicts"`  // nil = not cached yet
	CommitHash       string    `json:"commit_hash"`    // empty = not cached yet
	CommitCount      *int      `json:"commit_count"`   // nil = not cached yet
	CommitsBehind    *int      `json:"commits_behind"` // nil = not cached yet
	Branch           string    `json:"branch"`         // empty = not cached yet
	LastUpdated      time.Time `json:"last_updated"`
	UpdateInProgress bool      `json:"update_in_progress"`
}

// NewWorktreeStatusCache creates a new worktree status cache
func NewWorktreeStatusCache(operations git.Operations, stateManager *WorktreeStateManager) *WorktreeStatusCache {
	ctx, cancel := context.WithCancel(context.Background())

	cache := &WorktreeStatusCache{
		statuses:     make(map[string]*CachedWorktreeStatus),
		operations:   operations,
		stateManager: stateManager,
		watchers:     make(map[string]*fsnotify.Watcher),
		ctx:          ctx,
		cancel:       cancel,
		updateQueue:  make(chan string, 100), // Buffer for update requests
	}

	// Start background update worker
	go cache.backgroundUpdateWorker()

	return cache
}

// EnhanceWorktreeWithCache enhances a worktree with cached status if available
// This is the key method that enables fast ListWorktrees responses
func (c *WorktreeStatusCache) EnhanceWorktreeWithCache(worktree *models.Worktree) {
	c.mu.RLock()
	cached, exists := c.statuses[worktree.ID]
	c.mu.RUnlock()

	if !exists {
		// No cache entry - create empty one and queue for background update
		c.mu.Lock()
		c.statuses[worktree.ID] = &CachedWorktreeStatus{
			WorktreeID: worktree.ID,
			// All status fields are nil/empty = "loading state"
		}
		c.mu.Unlock()

		// Queue for immediate background update
		select {
		case c.updateQueue <- worktree.ID:
		default:
			// Queue full - update will happen on next periodic cycle
		}
		return
	}

	// Apply cached values to worktree (only if cached)
	if cached.IsDirty != nil {
		worktree.IsDirty = *cached.IsDirty
	}
	if cached.HasConflicts != nil {
		worktree.HasConflicts = *cached.HasConflicts
	}
	if cached.CommitHash != "" {
		worktree.CommitHash = cached.CommitHash
	}
	if cached.CommitCount != nil {
		worktree.CommitCount = *cached.CommitCount
	}
	if cached.CommitsBehind != nil {
		worktree.CommitsBehind = *cached.CommitsBehind
	}
	if cached.Branch != "" {
		worktree.Branch = cached.Branch
	}
}

// IsStatusCached returns true if we have cached status for a worktree
func (c *WorktreeStatusCache) IsStatusCached(worktreeID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.statuses[worktreeID]
	if !exists {
		return false
	}

	// Check if essential fields are cached
	return cached.IsDirty != nil &&
		cached.HasConflicts != nil &&
		cached.CommitHash != "" &&
		cached.CommitCount != nil
}

// AddWorktree adds a new worktree to the cache and starts watching it
func (c *WorktreeStatusCache) AddWorktree(worktreeID, worktreePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create cache entry
	c.statuses[worktreeID] = &CachedWorktreeStatus{
		WorktreeID: worktreeID,
	}

	// Start watching the worktree directory
	c.startWatchingWorktree(worktreeID, worktreePath)

	// Queue for immediate update
	select {
	case c.updateQueue <- worktreeID:
	default:
	}
}

// RemoveWorktree removes a worktree from cache and stops watching
func (c *WorktreeStatusCache) RemoveWorktree(worktreeID string, worktreePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.statuses, worktreeID)

	if watcher, exists := c.watchers[worktreePath]; exists {
		watcher.Close()
		delete(c.watchers, worktreePath)
	}
}

// ForceRefresh forces an immediate update of a worktree's status
func (c *WorktreeStatusCache) ForceRefresh(worktreeID string) {
	select {
	case c.updateQueue <- worktreeID:
	default:
		// Queue full - will be processed on next cycle
	}
}

// startWatchingWorktree sets up filesystem watching for a worktree
func (c *WorktreeStatusCache) startWatchingWorktree(worktreeID, worktreePath string) {
	gitDir := filepath.Join(worktreePath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return // Not a git repository
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("⚠️ Failed to create watcher for %s: %v", worktreePath, err)
		return
	}

	// Watch key git directories for changes
	watchPaths := []string{
		gitDir,
		filepath.Join(gitDir, "refs"),
		filepath.Join(gitDir, "refs", "heads"),
		worktreePath, // Working directory for file changes
	}

	for _, path := range watchPaths {
		if _, err := os.Stat(path); err == nil {
			if err := watcher.Add(path); err != nil {
				log.Printf("⚠️ Failed to watch %s: %v", path, err)
			}
		}
	}

	c.watchers[worktreePath] = watcher

	// Start goroutine to handle events
	go func() {
		defer watcher.Close()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// Filter relevant events
				if c.isRelevantFileEvent(event) {
					log.Printf("🔍 Git change detected in %s: %s", worktreePath, event.Name)

					// Debounce rapid file changes (configurable via CATNIP_CACHE_DEBOUNCE_MS)
					time.AfterFunc(getDebounceInterval(), func() {
						select {
						case c.updateQueue <- worktreeID:
						default:
						}
					})
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("⚠️ Watcher error for %s: %v", worktreePath, err)

			case <-c.ctx.Done():
				return
			}
		}
	}()
}

// isRelevantFileEvent determines if a filesystem event should trigger a cache update
func (c *WorktreeStatusCache) isRelevantFileEvent(event fsnotify.Event) bool {
	name := filepath.Base(event.Name)

	// Git metadata changes
	if strings.Contains(event.Name, ".git/") {
		relevantFiles := []string{"HEAD", "index", "MERGE_HEAD", "CHERRY_PICK_HEAD", "refs"}
		for _, file := range relevantFiles {
			if strings.Contains(event.Name, file) {
				return true
			}
		}
	}

	// Working directory changes (but ignore temporary files)
	if !strings.HasPrefix(name, ".") && !strings.HasSuffix(name, "~") && !strings.HasSuffix(name, ".tmp") {
		return event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0
	}

	return false
}

// getDebounceInterval returns the debounce interval, configurable via CATNIP_CACHE_DEBOUNCE_MS
func getDebounceInterval() time.Duration {
	if envMs := os.Getenv("CATNIP_CACHE_DEBOUNCE_MS"); envMs != "" {
		if ms, err := strconv.Atoi(envMs); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 200 * time.Millisecond // Default: 200ms
}

// getBatchInterval returns the batch processing interval, configurable via CATNIP_CACHE_BATCH_MS
func getBatchInterval() time.Duration {
	if envMs := os.Getenv("CATNIP_CACHE_BATCH_MS"); envMs != "" {
		if ms, err := strconv.Atoi(envMs); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 100 * time.Millisecond // Default: 100ms
}

// backgroundUpdateWorker processes the update queue
func (c *WorktreeStatusCache) backgroundUpdateWorker() {
	ticker := time.NewTicker(30 * time.Second) // Periodic full refresh
	defer ticker.Stop()

	batchTimer := time.NewTimer(0)
	if !batchTimer.Stop() {
		<-batchTimer.C
	}

	pendingUpdates := make(map[string]bool)

	for {
		select {
		case worktreeID := <-c.updateQueue:
			pendingUpdates[worktreeID] = true

			// Reset batch timer to collect more updates
			if !batchTimer.Stop() {
				select {
				case <-batchTimer.C:
				default:
				}
			}
			batchTimer.Reset(getBatchInterval()) // Configurable via CATNIP_CACHE_BATCH_MS

		case <-batchTimer.C:
			if len(pendingUpdates) > 0 {
				c.processBatchUpdates(pendingUpdates)
				pendingUpdates = make(map[string]bool)
			}

		case <-ticker.C:
			// Periodic refresh of all cached statuses
			c.refreshAllStatuses()

		case <-c.ctx.Done():
			return
		}
	}
}

// processBatchUpdates processes a batch of worktree updates efficiently
func (c *WorktreeStatusCache) processBatchUpdates(worktreeIDs map[string]bool) {
	updates := make(map[string]*CachedWorktreeStatus)

	for worktreeID := range worktreeIDs {
		if status := c.updateWorktreeStatus(worktreeID); status != nil {
			updates[worktreeID] = status
		}
	}

	if len(updates) > 0 {
		// Update state manager with batch updates
		if c.stateManager != nil {
			// Convert cached updates to state manager format
			stateUpdates := make(map[string]map[string]interface{})
			for worktreeID, cached := range updates {
				stateUpdate := make(map[string]interface{})
				if cached.IsDirty != nil {
					stateUpdate["is_dirty"] = *cached.IsDirty
				}
				if cached.HasConflicts != nil {
					stateUpdate["has_conflicts"] = *cached.HasConflicts
				}
				if cached.CommitHash != "" {
					stateUpdate["commit_hash"] = cached.CommitHash
				}
				if cached.CommitCount != nil {
					stateUpdate["commit_count"] = *cached.CommitCount
				}
				if cached.CommitsBehind != nil {
					stateUpdate["commits_behind"] = *cached.CommitsBehind
				}
				if cached.Branch != "" {
					stateUpdate["branch"] = cached.Branch
				}
				if len(stateUpdate) > 0 {
					stateUpdates[worktreeID] = stateUpdate
				}
			}
			if len(stateUpdates) > 0 {
				if err := c.stateManager.BatchUpdateWorktrees(stateUpdates); err != nil {
					log.Printf("⚠️ Failed to batch update worktrees in state: %v", err)
				}
			}
		}
	}
}

// updateWorktreeStatus updates a single worktree's cached status
func (c *WorktreeStatusCache) updateWorktreeStatus(worktreeID string) *CachedWorktreeStatus {
	c.mu.RLock()
	cached, exists := c.statuses[worktreeID]
	if !exists {
		c.mu.RUnlock()
		return nil
	}

	// Mark as updating to prevent concurrent updates
	if cached.UpdateInProgress {
		c.mu.RUnlock()
		return nil
	}

	cached.UpdateInProgress = true
	c.mu.RUnlock()

	defer func() {
		c.mu.Lock()
		if cached, exists := c.statuses[worktreeID]; exists {
			cached.UpdateInProgress = false
		}
		c.mu.Unlock()
	}()

	// We need the actual worktree path - this requires lookup from GitService
	// For now, we'll implement this as a callback pattern
	return c.updateWorktreeStatusInternal(worktreeID, cached)
}

// SetWorktreePathResolver allows the GitService to provide worktree path resolution
func (c *WorktreeStatusCache) SetWorktreePathResolver(resolver func(string) (string, *models.Worktree)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pathResolver = resolver
}

// updateWorktreeStatusInternal performs the actual git operations
func (c *WorktreeStatusCache) updateWorktreeStatusInternal(worktreeID string, cached *CachedWorktreeStatus) *CachedWorktreeStatus {
	if c.pathResolver == nil {
		return cached // Can't update without path resolver
	}

	worktreePath, worktree := c.pathResolver(worktreeID)
	if worktreePath == "" || worktree == nil {
		return cached // Worktree not found
	}

	// Perform the expensive git operations

	// Check if dirty
	isDirty := c.operations.IsDirty(worktreePath)
	cached.IsDirty = &isDirty

	// Check for conflicts
	hasConflicts := c.operations.HasConflicts(worktreePath)
	cached.HasConflicts = &hasConflicts

	// Get current commit hash
	if commitHash, err := c.operations.GetCommitHash(worktreePath, "HEAD"); err == nil {
		cached.CommitHash = commitHash
	}

	// Get current branch (detect actual state)
	if branchOutput, err := c.operations.ExecuteGit(worktreePath, "symbolic-ref", "HEAD"); err == nil {
		branch := strings.TrimSpace(string(branchOutput))
		branch = strings.TrimPrefix(branch, "refs/heads/")
		cached.Branch = branch
	}

	// Count commits ahead and behind (only if we have source branch info)
	if worktree.SourceBranch != "" {
		sourceRef := worktree.SourceBranch
		if !strings.HasPrefix(sourceRef, "origin/") {
			// For local repos, use the branch directly since it's the source of truth
			// For remote repos, use origin/ prefix
			if !strings.Contains(worktree.RepoID, "local/") {
				sourceRef = "origin/" + sourceRef
			}
			// For local repos, use sourceRef as-is (no prefix needed)
		}

		// Count commits ahead
		if count, err := c.operations.GetCommitCount(worktreePath, sourceRef, "HEAD"); err == nil {
			cached.CommitCount = &count
		}

		// Count commits behind
		if count, err := c.operations.GetCommitCount(worktreePath, "HEAD", sourceRef); err == nil {
			cached.CommitsBehind = &count
		}
	}

	cached.LastUpdated = time.Now()

	// Removed noisy worktree update logs

	// Store updated status
	c.mu.Lock()
	c.statuses[worktreeID] = cached
	c.mu.Unlock()

	// Update state manager with individual status
	if c.stateManager != nil {
		if err := c.stateManager.UpdateWorktreeStatus(worktreeID, cached); err != nil {
			log.Printf("⚠️ Failed to update worktree status in state: %v", err)
		}
	}

	return cached
}

// refreshAllStatuses refreshes all cached statuses periodically
func (c *WorktreeStatusCache) refreshAllStatuses() {
	c.mu.RLock()
	worktreeIDs := make([]string, 0, len(c.statuses))
	for worktreeID := range c.statuses {
		worktreeIDs = append(worktreeIDs, worktreeID)
	}
	c.mu.RUnlock()

	// Only log if there are many worktrees
	if len(worktreeIDs) > 5 {
		log.Printf("🔄 Starting periodic refresh of %d worktree statuses", len(worktreeIDs))
	}

	pendingUpdates := make(map[string]bool)
	for _, worktreeID := range worktreeIDs {
		pendingUpdates[worktreeID] = true
	}

	c.processBatchUpdates(pendingUpdates)
}

// Stop shuts down the cache and all watchers
func (c *WorktreeStatusCache) Stop() {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, watcher := range c.watchers {
		watcher.Close()
	}
	c.watchers = make(map[string]*fsnotify.Watcher)
}

// GetCacheStats returns cache statistics for monitoring
func (c *WorktreeStatusCache) GetCacheStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalEntries := len(c.statuses)
	cachedEntries := 0

	for _, status := range c.statuses {
		if status.IsDirty != nil && status.CommitHash != "" {
			cachedEntries++
		}
	}

	return map[string]interface{}{
		"total_entries":   totalEntries,
		"cached_entries":  cachedEntries,
		"cache_ratio":     float64(cachedEntries) / float64(totalEntries),
		"active_watchers": len(c.watchers),
	}
}
