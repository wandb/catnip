package services

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
)

// SyncManager handles sync, merge, and rebase operations
type SyncManager struct {
	operations git.Operations
}

// NewSyncManager creates a new sync manager
func NewSyncManager(operations git.Operations) *SyncManager {
	return &SyncManager{
		operations: operations,
	}
}

// SyncWorktree syncs a worktree with its source branch
func (sm *SyncManager) SyncWorktree(worktree *models.Worktree, strategy string, isLocalRepo bool) error {
	// Ensure we have full history for sync operations
	sm.fetchFullHistory(worktree, isLocalRepo)

	// Get the appropriate source reference
	var sourceRef string
	if isLocalRepo {
		// For local repos, use the local branch directly since it's the source of truth
		// The live remote can become stale and doesn't represent the current state
		sourceRef = worktree.SourceBranch
	} else {
		sourceRef = fmt.Sprintf("origin/%s", worktree.SourceBranch)
	}

	// Apply the sync strategy
	if err := sm.applySyncStrategy(worktree, strategy, sourceRef); err != nil {
		return err
	}

	log.Printf("✅ Synced worktree %s with %s strategy", worktree.Name, strategy)
	return nil
}

// MergeWorktreeToMain merges a local repo worktree's changes back to the main repository
func (sm *SyncManager) MergeWorktreeToMain(repo *models.Repository, worktree *models.Worktree, squash bool) error {
	log.Printf("🔄 Merging worktree %s back to main repository", worktree.Name)

	// Ensure we have full history for merge operations
	sm.fetchFullHistory(worktree, true)

	// First, push the worktree branch to the main repo
	err := sm.operations.PushBranch(worktree.Path, git.PushStrategy{
		Remote:    repo.Path,
		Branch:    worktree.Branch,
		RemoteURL: repo.Path,
	})
	if err != nil {
		return fmt.Errorf("failed to push worktree branch to main repo: %v", err)
	}

	// Switch to the source branch in main repo and merge
	// Note: This requires implementing checkout operation in git.Operations
	// For now, we'll use git command execution directly
	output, err := sm.operations.ExecuteGit(repo.Path, "checkout", worktree.SourceBranch)
	if err != nil {
		return fmt.Errorf("failed to checkout source branch in main repo: %v\n%s", err, output)
	}

	// Merge the worktree branch
	var mergeArgs []string
	if squash {
		mergeArgs = []string{"merge", worktree.Branch, "--squash"}
	} else {
		mergeArgs = []string{"merge", worktree.Branch, "--no-ff", "-m", fmt.Sprintf("Merge branch '%s' from worktree", worktree.Branch)}
	}

	output, err = sm.operations.ExecuteGit(repo.Path, mergeArgs...)
	if err != nil {
		// Check if this is a merge conflict
		if sm.isMergeConflict(repo.Path, string(output)) {
			return sm.createMergeConflictError("merge", worktree, string(output))
		}
		return fmt.Errorf("failed to merge worktree branch: %v\n%s", err, output)
	}

	// For squash merges, we need to commit the staged changes
	if squash {
		err = sm.operations.Commit(repo.Path, fmt.Sprintf("Squash merge branch '%s' from worktree", worktree.Branch), git.CommitOptions{})
		if err != nil {
			return fmt.Errorf("failed to commit squash merge: %v", err)
		}
	}

	// Delete the feature branch from main repo (cleanup)
	_ = sm.operations.DeleteBranch(repo.Path, worktree.Branch, false) // Ignore errors - branch might be in use

	// Get the new commit hash from the main branch after merge
	newCommitHash, err := sm.operations.GetCommitHash(repo.Path, "HEAD")
	if err != nil {
		log.Printf("⚠️  Failed to get new commit hash after merge: %v", err)
	} else {
		// Update the worktree's commit hash to the new merge point
		worktree.CommitHash = newCommitHash
		log.Printf("📝 Updated worktree %s CommitHash to %s", worktree.Name, newCommitHash)
	}

	log.Printf("✅ Merged worktree %s to main repository", worktree.Name)
	return nil
}

// CheckSyncConflicts checks if syncing a worktree would cause merge conflicts
func (sm *SyncManager) CheckSyncConflicts(worktree *models.Worktree, isLocalRepo bool) (*models.MergeConflictError, error) {
	return sm.checkConflictsInternal(worktree, "sync", isLocalRepo)
}

// CheckMergeConflicts checks if merging a worktree to main would cause conflicts
func (sm *SyncManager) CheckMergeConflicts(repo *models.Repository, worktree *models.Worktree) (*models.MergeConflictError, error) {
	// Create a temporary branch in the main repo to test the merge
	tempBranch := fmt.Sprintf("temp-merge-check-%d", time.Now().Unix())

	// Push the worktree branch to temp branch in main repo
	err := sm.operations.PushBranch(worktree.Path, git.PushStrategy{
		Remote:    repo.Path,
		Branch:    fmt.Sprintf("%s:%s", worktree.Branch, tempBranch),
		RemoteURL: repo.Path,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to push temp branch for conflict check: %v", err)
	}

	// Clean up temp branch when done
	defer func() {
		_ = sm.operations.DeleteBranch(repo.Path, tempBranch, true) // Force delete
	}()

	// Try a dry-run merge to detect conflicts
	output, err := sm.operations.MergeTree(repo.Path, worktree.SourceBranch, tempBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to check merge conflicts: %v", err)
	}

	// Check if merge-tree output indicates conflicts
	if sm.hasConflictMarkers(output) {
		// Parse conflicted files from merge-tree output
		conflictFiles := sm.parseConflictFiles(output)

		return &models.MergeConflictError{
			Operation:     "merge",
			WorktreeName:  worktree.Name,
			WorktreePath:  worktree.Path,
			ConflictFiles: conflictFiles,
			Message:       fmt.Sprintf("Merge would cause conflicts in worktree '%s'", worktree.Name),
		}, nil
	}

	return nil, nil
}

// applySyncStrategy applies merge or rebase strategy
func (sm *SyncManager) applySyncStrategy(worktree *models.Worktree, strategy, sourceRef string) error {
	switch strategy {
	case "merge":
		return sm.operations.Merge(worktree.Path, sourceRef)
	case "rebase":
		return sm.operations.Rebase(worktree.Path, sourceRef)
	default:
		return fmt.Errorf("unknown sync strategy: %s", strategy)
	}
}

// fetchFullHistory fetches the full history for a worktree (needed for PR/push operations)
func (sm *SyncManager) fetchFullHistory(worktree *models.Worktree, isLocalRepo bool) {
	if isLocalRepo {
		// For local repos, fetch full history from live remote
		if err := sm.operations.FetchBranch(worktree.Path, git.FetchStrategy{
			Remote:     "live",
			Branch:     worktree.SourceBranch,
			RemoteName: "live",
		}); err != nil {
			log.Printf("⚠️ Failed to fetch full history from live remote: %v", err)
		}
	} else {
		// For remote repos, fetch full history from origin
		if err := sm.operations.FetchBranchFull(worktree.Path, worktree.SourceBranch); err != nil {
			log.Printf("⚠️ Failed to fetch full history from origin: %v", err)
		}
	}
}

// checkConflictsInternal consolidated conflict checking logic
func (sm *SyncManager) checkConflictsInternal(worktree *models.Worktree, operation string, isLocalRepo bool) (*models.MergeConflictError, error) {
	// Ensure we have full history for accurate conflict detection
	sm.fetchFullHistory(worktree, isLocalRepo)

	// Get the appropriate source reference
	var sourceRef string
	if isLocalRepo {
		// For local repos, use the local branch directly since it's the source of truth
		// The live remote can become stale and doesn't represent the current state
		sourceRef = worktree.SourceBranch
	} else {
		sourceRef = fmt.Sprintf("origin/%s", worktree.SourceBranch)
	}

	// Try a dry-run merge to detect conflicts
	output, err := sm.operations.MergeTree(worktree.Path, "HEAD", sourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to check for conflicts: %v", err)
	}

	// Check if merge-tree output indicates conflicts
	if sm.hasConflictMarkers(output) {
		// Parse conflicted files from merge-tree output
		conflictFiles := sm.parseConflictFiles(output)

		return &models.MergeConflictError{
			Operation:     operation,
			WorktreeName:  worktree.Name,
			WorktreePath:  worktree.Path,
			ConflictFiles: conflictFiles,
			Message:       fmt.Sprintf("%s would cause conflicts in worktree '%s'", operation, worktree.Name),
		}, nil
	}

	return nil, nil
}

// isMergeConflict checks if the git command output indicates a merge conflict
func (sm *SyncManager) isMergeConflict(repoPath, output string) bool {
	// Check for common merge conflict indicators in git output
	conflictIndicators := []string{
		"CONFLICT",
		"Automatic merge failed",
		"fix conflicts and then commit",
		"Merge conflict",
	}

	for _, indicator := range conflictIndicators {
		if strings.Contains(output, indicator) {
			return true
		}
	}

	// Also check git status for unmerged paths
	status, err := sm.operations.GetStatus(repoPath)
	if err != nil {
		return false
	}

	return status.HasConflicts
}

// createMergeConflictError creates a detailed merge conflict error
func (sm *SyncManager) createMergeConflictError(operation string, worktree *models.Worktree, output string) *models.MergeConflictError {
	// Get list of conflicted files
	conflictFiles, _ := sm.operations.GetConflictedFiles(worktree.Path)

	message := fmt.Sprintf("Merge conflict occurred during %s operation in worktree '%s'. Please resolve conflicts in the terminal.", operation, worktree.Name)

	return &models.MergeConflictError{
		Operation:     operation,
		WorktreeName:  worktree.Name,
		WorktreePath:  worktree.Path,
		ConflictFiles: conflictFiles,
		Message:       message,
	}
}

// hasConflictMarkers checks if the output contains conflict markers
func (sm *SyncManager) hasConflictMarkers(output string) bool {
	return git.HasConflictMarkers(output)
}

// parseConflictFiles extracts file names from merge-tree conflict output
func (sm *SyncManager) parseConflictFiles(output string) []string {
	return git.ExtractConflictFiles(output)
}
