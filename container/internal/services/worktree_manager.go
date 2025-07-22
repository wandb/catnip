package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
)

// WorktreeManager handles worktree lifecycle operations
type WorktreeManager struct {
	operations git.Operations
}

// NewWorktreeManager creates a new worktree manager
func NewWorktreeManager(operations git.Operations) *WorktreeManager {
	return &WorktreeManager{
		operations: operations,
	}
}

// CreateWorktree creates a new worktree for an existing repository
func (wm *WorktreeManager) CreateWorktree(repo *models.Repository, branch, name string, isLocalRepo bool) (*models.Worktree, error) {
	id := uuid.New().String()

	var worktreePath string
	var displayName string

	if isLocalRepo {
		// For local repos: /workspace/{repoDir}/{workspaceName}
		dirName := filepath.Base(repo.Path)
		workspaceName := git.ExtractWorkspaceName(name)
		worktreePath = filepath.Join(getWorkspaceDir(), dirName, workspaceName)
		displayName = fmt.Sprintf("%s/%s", dirName, workspaceName)
	} else {
		// For remote repos: /workspace/{repoName}/{workspaceName}
		repoParts := strings.Split(repo.ID, "/")
		repoName := repoParts[len(repoParts)-1]
		workspaceName := git.ExtractWorkspaceName(name)
		worktreePath = filepath.Join(getWorkspaceDir(), repoName, workspaceName)
		displayName = workspaceName
	}

	// Create worktree directory first
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree directory: %v", err)
	}

	// Create worktree using operations interface
	err := wm.operations.CreateWorktree(repo.Path, worktreePath, name, branch)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %v", err)
	}

	// For local repos, add the "live" remote pointing back to main repo
	if isLocalRepo {
		if err := wm.operations.AddRemote(worktreePath, "live", repo.Path); err != nil {
			log.Printf("⚠️ Failed to add live remote: %v", err)
		} else {
			// Fetch the source branch from the live remote to get latest state
			log.Printf("🔄 Fetching latest %s from live remote", branch)
			if err := wm.operations.FetchBranch(worktreePath, git.FetchStrategy{
				Remote:     "live",
				Branch:     branch,
				RemoteName: "live",
			}); err != nil {
				log.Printf("⚠️ Failed to fetch %s from live remote: %v", branch, err)
			}
		}
	}

	// Get current commit hash
	commitHash, err := wm.operations.GetCommitHash(worktreePath, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}

	// Clean up branch name to ensure it's a proper source branch
	sourceBranch := strings.TrimSpace(branch)
	sourceBranch = strings.TrimPrefix(sourceBranch, "*")
	sourceBranch = strings.TrimPrefix(sourceBranch, "+")
	sourceBranch = strings.TrimSpace(sourceBranch)
	sourceBranch = strings.TrimPrefix(sourceBranch, "origin/")

	// Calculate commit count ahead of source
	commitCount := 0
	if sourceBranch != name { // Only count if different from current branch
		if count, err := wm.operations.GetCommitCount(worktreePath, sourceBranch, "HEAD"); err == nil {
			commitCount = count
		}
	}

	worktree := &models.Worktree{
		ID:            id,
		RepoID:        repo.ID,
		Name:          displayName,
		Path:          worktreePath,
		Branch:        name,
		SourceBranch:  sourceBranch,
		CommitHash:    commitHash,
		CommitCount:   commitCount,
		CommitsBehind: 0, // Will be calculated later
		IsDirty:       false,
		HasConflicts:  false,
		CreatedAt:     time.Now(),
		LastAccessed:  time.Now(),
	}

	return worktree, nil
}

// DeleteWorktree removes a worktree comprehensively
func (wm *WorktreeManager) DeleteWorktree(repo *models.Repository, worktree *models.Worktree) error {
	log.Printf("🗑️ Starting comprehensive cleanup for worktree %s", worktree.Name)

	// Step 1: Remove the worktree directory
	err := wm.operations.RemoveWorktree(repo.Path, worktree.Path, true)
	if err != nil {
		log.Printf("⚠️ Failed to remove worktree directory (continuing with cleanup): %v", err)
	} else {
		log.Printf("✅ Removed worktree directory: %s", worktree.Path)
	}

	// Step 2: Remove the worktree branch from the repository
	if worktree.Branch != "" && worktree.Branch != worktree.SourceBranch {
		err = wm.operations.DeleteBranch(repo.Path, worktree.Branch, true)
		if err != nil {
			log.Printf("⚠️ Failed to remove branch %s (may not exist or be in use): %v", worktree.Branch, err)
		} else {
			log.Printf("✅ Removed branch: %s", worktree.Branch)
		}
	}

	// Step 3: Remove preview branch if it exists
	previewBranchName := fmt.Sprintf("preview/%s", worktree.Branch)
	err = wm.operations.DeleteBranch(repo.Path, previewBranchName, true)
	if err != nil {
		// Preview branch might not exist, don't log as warning
		log.Printf("ℹ️ No preview branch to remove: %s", previewBranchName)
	} else {
		log.Printf("✅ Removed preview branch: %s", previewBranchName)
	}

	// Step 4: Force remove any remaining files in the worktree directory
	if _, err := os.Stat(worktree.Path); err == nil {
		if removeErr := os.RemoveAll(worktree.Path); removeErr != nil {
			log.Printf("⚠️ Failed to force remove worktree directory %s: %v", worktree.Path, removeErr)
		} else {
			log.Printf("✅ Force removed remaining worktree directory: %s", worktree.Path)
		}
	}

	// Step 5: Run git garbage collection to clean up dangling objects
	if err := wm.operations.GarbageCollect(repo.Path); err != nil {
		log.Printf("⚠️ Failed to run garbage collection after worktree deletion: %v", err)
	} else {
		log.Printf("✅ Ran garbage collection to clean up dangling objects")
	}

	log.Printf("✅ Completed comprehensive cleanup for worktree %s", worktree.Name)
	return nil
}

// UpdateWorktreeStatus updates commit count and commits behind for a worktree
func (wm *WorktreeManager) UpdateWorktreeStatus(worktree *models.Worktree, shouldFetch bool, isLocalRepo bool) {
	if worktree.SourceBranch == "" || worktree.SourceBranch == worktree.Branch {
		return
	}

	// Fetch latest reference only if requested
	if shouldFetch {
		wm.fetchLatestReference(worktree, isLocalRepo)
	}

	// Determine source reference based on repo type
	var sourceRef string
	if isLocalRepo {
		sourceRef = fmt.Sprintf("live/%s", worktree.SourceBranch)
	} else {
		sourceRef = fmt.Sprintf("origin/%s", worktree.SourceBranch)
	}

	// Count commits ahead (our commits)
	if count, err := wm.operations.GetCommitCount(worktree.Path, sourceRef, "HEAD"); err == nil {
		worktree.CommitCount = count
	}

	// Count commits behind (missing commits)
	if count, err := wm.operations.GetCommitCount(worktree.Path, "HEAD", sourceRef); err == nil {
		worktree.CommitsBehind = count
	}

	// Update status flags
	worktree.IsDirty = wm.operations.IsDirty(worktree.Path)
	worktree.HasConflicts = wm.operations.HasConflicts(worktree.Path)
}

// fetchLatestReference fetches the latest reference for a worktree
func (wm *WorktreeManager) fetchLatestReference(worktree *models.Worktree, isLocalRepo bool) {
	if isLocalRepo {
		// For local repos, fetch from the live remote
		if err := wm.operations.FetchBranch(worktree.Path, git.FetchStrategy{
			Remote:     "live",
			Branch:     worktree.SourceBranch,
			RemoteName: "live",
			Depth:      1,
		}); err != nil {
			log.Printf("⚠️ Failed to fetch latest reference from live remote: %v", err)
		}
	} else {
		// For remote repos, fetch from origin
		if err := wm.operations.FetchBranchFast(worktree.Path, worktree.SourceBranch); err != nil {
			log.Printf("⚠️ Failed to fetch latest reference from origin: %v", err)
		}
	}
}
