package services

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
)

// PRSyncManager handles periodic synchronization of pull request states
type PRSyncManager struct {
	stateManager *WorktreeStateManager
	prStateCache map[string]*models.PullRequestState
	syncInterval time.Duration
	ticker       *time.Ticker
	stopChan     chan bool
	mutex        sync.RWMutex
	isRunning    bool
}

var (
	prSyncManagerInstance *PRSyncManager
	prSyncManagerOnce     sync.Once
)

// GetPRSyncManager returns the singleton instance of PRSyncManager
func GetPRSyncManager(stateManager *WorktreeStateManager) *PRSyncManager {
	prSyncManagerOnce.Do(func() {
		prSyncManagerInstance = &PRSyncManager{
			stateManager: stateManager,
			prStateCache: make(map[string]*models.PullRequestState),
			syncInterval: time.Minute, // Sync every minute
			stopChan:     make(chan bool),
		}
	})

	// If stateManager provided and instance exists but has nil stateManager, set it
	if stateManager != nil && prSyncManagerInstance.stateManager == nil {
		prSyncManagerInstance.stateManager = stateManager
	}

	return prSyncManagerInstance
}

// Start begins the periodic PR sync process
func (pm *PRSyncManager) Start() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if pm.isRunning {
		logger.Debug("PR sync manager is already running")
		return
	}

	logger.Info("Starting PR sync manager with %v interval", pm.syncInterval)
	pm.ticker = time.NewTicker(pm.syncInterval)
	pm.isRunning = true

	go pm.syncLoop()
}

// Stop halts the periodic PR sync process
func (pm *PRSyncManager) Stop() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if !pm.isRunning {
		return
	}

	logger.Info("Stopping PR sync manager")
	pm.isRunning = false

	if pm.ticker != nil {
		pm.ticker.Stop()
	}

	select {
	case pm.stopChan <- true:
	default:
	}
}

// syncLoop runs the periodic sync process
func (pm *PRSyncManager) syncLoop() {
	// Perform initial sync
	pm.performSync()

	for {
		select {
		case <-pm.ticker.C:
			pm.performSync()
		case <-pm.stopChan:
			logger.Debug("PR sync loop stopped")
			return
		}
	}
}

// performSync executes a single sync cycle
func (pm *PRSyncManager) performSync() {
	logger.Debug("Starting PR sync cycle")

	// Get all worktrees with PR URLs
	prRequests := pm.collectPRRequests()

	if len(prRequests) == 0 {
		logger.Debug("No PRs to sync")
		return
	}

	logger.Debug("Found %d repositories with PRs to sync", len(prRequests))

	// Sync PR states for each repository
	for repoID, prNumbers := range prRequests {
		states, err := pm.syncRepositoryPRs(repoID, prNumbers)
		if err != nil {
			logger.Warnf("Failed to sync PRs for repository %s: %v", repoID, err)
			continue
		}

		// Update cache
		pm.updateCache(states)
	}

	logger.Debug("PR sync cycle completed")
}

// collectPRRequests gathers all PR numbers that need syncing, grouped by repository
func (pm *PRSyncManager) collectPRRequests() map[string][]int {
	if pm.stateManager == nil {
		return nil
	}

	prRequests := make(map[string][]int)
	prPattern := regexp.MustCompile(`github\.com/([^/]+/[^/]+)/pull/(\d+)`)

	// Get all worktrees from state manager
	allWorktrees := pm.stateManager.GetAllWorktrees()
	for _, worktree := range allWorktrees {
		if worktree.PullRequestURL == "" {
			continue
		}

		matches := prPattern.FindStringSubmatch(worktree.PullRequestURL)
		if len(matches) != 3 {
			continue
		}

		repoID := matches[1]
		prNumber, err := strconv.Atoi(matches[2])
		if err != nil {
			logger.Warnf("Invalid PR number in URL %s: %v", worktree.PullRequestURL, err)
			continue
		}

		// Add to requests, avoiding duplicates
		if _, exists := prRequests[repoID]; !exists {
			prRequests[repoID] = []int{}
		}

		// Check if PR number already in list
		found := false
		for _, existing := range prRequests[repoID] {
			if existing == prNumber {
				found = true
				break
			}
		}
		if !found {
			prRequests[repoID] = append(prRequests[repoID], prNumber)
		}
	}

	return prRequests
}

// syncRepositoryPRs syncs PR states for a single repository using GraphQL
func (pm *PRSyncManager) syncRepositoryPRs(repoID string, prNumbers []int) (map[string]*models.PullRequestState, error) {
	if len(prNumbers) == 0 {
		return nil, nil
	}

	query := pm.buildBatchPRQuery(repoID, prNumbers)

	// Execute GraphQL query via gh cli
	cmd := exec.Command("gh", "api", "graphql", "-f", fmt.Sprintf("query=%s", query))
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %v", err)
	}

	// Parse response
	states, err := pm.parseBatchPRResponse(output, repoID, prNumbers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %v", err)
	}

	return states, nil
}

// buildBatchPRQuery creates a GraphQL query with aliases for multiple PRs
func (pm *PRSyncManager) buildBatchPRQuery(repoID string, prNumbers []int) string {
	parts := strings.Split(repoID, "/")
	if len(parts) != 2 {
		return ""
	}
	owner, repo := parts[0], parts[1]

	var aliases []string
	for _, num := range prNumbers {
		alias := fmt.Sprintf("pr%d: pullRequest(number: %d) { number title state url }", num, num)
		aliases = append(aliases, alias)
	}

	return fmt.Sprintf(`query { repository(owner: "%s", name: "%s") { %s } }`,
		owner, repo, strings.Join(aliases, " "))
}

// parseBatchPRResponse parses the GraphQL response and returns PR states
func (pm *PRSyncManager) parseBatchPRResponse(output []byte, repoID string, prNumbers []int) (map[string]*models.PullRequestState, error) {
	var response struct {
		Data struct {
			Repository map[string]struct {
				Number int    `json:"number"`
				Title  string `json:"title"`
				State  string `json:"state"`
				URL    string `json:"url"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}

	states := make(map[string]*models.PullRequestState)
	now := time.Now()

	for alias, pr := range response.Data.Repository {
		if pr.Number == 0 {
			continue // PR not found or error
		}

		key := fmt.Sprintf("%s#%d", repoID, pr.Number)
		states[key] = &models.PullRequestState{
			Number:      pr.Number,
			State:       pr.State,
			Repository:  repoID,
			URL:         pr.URL,
			Title:       pr.Title,
			LastSynced:  now,
			WorktreeIDs: pm.getWorktreeIDsForPR(repoID, pr.Number),
		}
	}

	return states, nil
}

// getWorktreeIDsForPR finds all worktree IDs that reference a specific PR
func (pm *PRSyncManager) getWorktreeIDsForPR(repoID string, prNumber int) []string {
	if pm.stateManager == nil {
		return nil
	}

	var worktreeIDs []string
	expectedURL := fmt.Sprintf("https://github.com/%s/pull/%d", repoID, prNumber)

	allWorktrees := pm.stateManager.GetAllWorktrees()
	for id, worktree := range allWorktrees {
		if worktree.PullRequestURL == expectedURL {
			worktreeIDs = append(worktreeIDs, id)
		}
	}

	return worktreeIDs
}

// updateCache updates the in-memory cache and persists to disk
func (pm *PRSyncManager) updateCache(states map[string]*models.PullRequestState) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Update in-memory cache
	for key, state := range states {
		pm.prStateCache[key] = state
	}

	// The state manager will automatically persist PR states when saveStateInternal is called
	// This happens automatically during normal worktree state updates
	logger.Debug("Updated PR cache with %d states", len(states))
}

// GetPRState returns the cached state for a specific PR
func (pm *PRSyncManager) GetPRState(repoID string, prNumber int) *models.PullRequestState {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	key := fmt.Sprintf("%s#%d", repoID, prNumber)
	return pm.prStateCache[key]
}

// GetAllPRStates returns all cached PR states
func (pm *PRSyncManager) GetAllPRStates() map[string]*models.PullRequestState {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*models.PullRequestState)
	for key, state := range pm.prStateCache {
		result[key] = state
	}
	return result
}

// LoadPersistedStates loads PR states from disk into memory cache
func (pm *PRSyncManager) LoadPersistedStates() error {
	// For now, we'll load from the state manager's state file since it's already integrated
	// In a future iteration, we could optimize this to use gitState if needed
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// The state manager will have already loaded the states, we just need to initialize our cache
	if pm.prStateCache == nil {
		pm.prStateCache = make(map[string]*models.PullRequestState)
	}

	logger.Debug("PR sync manager cache initialized")
	return nil
}

// LoadStatesFromData loads PR states from provided data (used during state restoration)
func (pm *PRSyncManager) LoadStatesFromData(states map[string]*models.PullRequestState) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.prStateCache = make(map[string]*models.PullRequestState)
	for key, state := range states {
		pm.prStateCache[key] = state
	}

	logger.Debug("Loaded %d persisted PR states into cache", len(pm.prStateCache))
}
