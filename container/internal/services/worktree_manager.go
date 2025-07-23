package services

import (
	"fmt"
	"log"

	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
)

// WorktreeManager handles worktree lifecycle operations
type WorktreeManager struct {
	operations         git.Operations
	gitWorktreeManager *git.WorktreeManager
}

// NewWorktreeManager creates a new worktree manager
func NewWorktreeManager(operations git.Operations) *WorktreeManager {
	return &WorktreeManager{
		operations:         operations,
		gitWorktreeManager: git.NewWorktreeManager(operations),
	}
}

// CreateLocalWorktree creates a new worktree for a local repository (delegates to git layer)
func (wm *WorktreeManager) CreateLocalWorktree(repo *models.Repository, sourceBranch, branchName, workspaceDir string) (*models.Worktree, error) {
	return wm.gitWorktreeManager.CreateLocalWorktree(git.CreateWorktreeRequest{
		Repository:   repo,
		SourceBranch: sourceBranch,
		BranchName:   branchName,
		WorkspaceDir: workspaceDir,
	})
}

// CreateWorktreeFromRequest creates a new worktree using the git layer request format (delegates to git layer)
func (wm *WorktreeManager) CreateWorktreeFromRequest(req git.CreateWorktreeRequest) (*models.Worktree, error) {
	return wm.gitWorktreeManager.CreateWorktree(req)
}

// CreateWorktree creates a new worktree for an existing repository (delegates to git layer)
func (wm *WorktreeManager) CreateWorktree(repo *models.Repository, branch, name string, isLocalRepo bool) (*models.Worktree, error) {
	// Determine which git layer method to use based on repo type
	if isLocalRepo {
		return wm.gitWorktreeManager.CreateLocalWorktree(git.CreateWorktreeRequest{
			Repository:   repo,
			SourceBranch: branch,
			BranchName:   name,
			WorkspaceDir: getWorkspaceDir(),
		})
	} else {
		return wm.gitWorktreeManager.CreateWorktree(git.CreateWorktreeRequest{
			Repository:   repo,
			SourceBranch: branch,
			BranchName:   name,
			WorkspaceDir: getWorkspaceDir(),
		})
	}
}

// DeleteWorktree removes a worktree comprehensively (delegates to git layer)
func (wm *WorktreeManager) DeleteWorktree(repo *models.Repository, worktree *models.Worktree) error {
	return wm.gitWorktreeManager.DeleteWorktree(worktree, repo)
}

// UpdateWorktreeStatus updates commit count and commits behind for a worktree with dynamic state detection
func (wm *WorktreeManager) UpdateWorktreeStatus(worktree *models.Worktree, shouldFetch bool, isLocalRepo bool) {
	// Service-level concern: handle fetching based on repo type
	if shouldFetch && !isLocalRepo {
		wm.fetchLatestReference(worktree, isLocalRepo)
	}

	// Delegate to git layer with service-level source ref resolution
	getSourceRef := func(w *models.Worktree) string {
		if isLocalRepo {
			return w.SourceBranch // Local repos use branch directly
		} else {
			return fmt.Sprintf("origin/%s", w.SourceBranch) // Remote repos use origin prefix
		}
	}

	wm.gitWorktreeManager.UpdateWorktreeStatus(worktree, getSourceRef)
}

// GetWorktreeDiff calculates diff for a worktree against its source branch
func (wm *WorktreeManager) GetWorktreeDiff(worktree *models.Worktree, sourceRef string, fetchLatestRef func(*models.Worktree) error) (*git.WorktreeDiffResponse, error) {
	// Delegate to git layer WorktreeManager
	return wm.gitWorktreeManager.GetWorktreeDiff(worktree, sourceRef, fetchLatestRef)
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
