package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
)

// LocalRepoManager handles local repository operations
type LocalRepoManager struct {
	operations git.Operations
}

// NewLocalRepoManager creates a new local repository manager
func NewLocalRepoManager(operations git.Operations) *LocalRepoManager {
	return &LocalRepoManager{
		operations: operations,
	}
}

// DetectLocalRepos scans /live for any Git repositories and loads them
func (lrm *LocalRepoManager) DetectLocalRepos() map[string]*models.Repository {
	repositories := make(map[string]*models.Repository)

	const liveDir = "/live"

	// Check if /live directory exists
	if _, err := os.Stat(liveDir); os.IsNotExist(err) {
		log.Printf("📁 No /live directory found, skipping local repo detection")
		return repositories
	}

	// Read all entries in /live
	entries, err := os.ReadDir(liveDir)
	if err != nil {
		log.Printf("❌ Failed to read /live directory: %v", err)
		return repositories
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repoPath := filepath.Join(liveDir, entry.Name())
		gitPath := filepath.Join(repoPath, ".git")

		// Check if it's a git repository
		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			continue
		}

		log.Printf("🔍 Detected local repository at %s", repoPath)

		// Create repository object
		repoID := fmt.Sprintf("local/%s", entry.Name())
		repo := &models.Repository{
			ID:            repoID,
			URL:           "file://" + repoPath,
			Path:          repoPath,
			DefaultBranch: lrm.getLocalRepoDefaultBranch(repoPath),
			CreatedAt:     time.Now(),
			LastAccessed:  time.Now(),
		}

		repositories[repoID] = repo
		log.Printf("✅ Local repository loaded: %s", repoID)
	}

	return repositories
}

// CreateWorktreePreview creates a preview branch in the main repo for viewing changes outside container
func (lrm *LocalRepoManager) CreateWorktreePreview(repo *models.Repository, worktree *models.Worktree) error {
	previewBranchName := fmt.Sprintf("preview/%s", worktree.Branch)
	log.Printf("🔍 Creating preview branch %s for worktree %s", previewBranchName, worktree.Name)

	// Check if there are uncommitted changes (staged, unstaged, or untracked)
	hasUncommittedChanges, err := lrm.operations.HasUncommittedChanges(worktree.Path)
	if err != nil {
		return fmt.Errorf("failed to check for uncommitted changes: %v", err)
	}

	var tempCommitHash string
	if hasUncommittedChanges {
		// Create a temporary commit with all uncommitted changes
		tempCommitHash, err = lrm.createTemporaryCommit(worktree.Path)
		if err != nil {
			return fmt.Errorf("failed to create temporary commit: %v", err)
		}
		defer func() {
			// Reset to remove the temporary commit after pushing
			if tempCommitHash != "" {
				_ = lrm.operations.ResetMixed(worktree.Path, "HEAD~1")
			}
		}()
	}

	// Check if preview branch already exists and handle accordingly
	shouldForceUpdate, err := lrm.shouldForceUpdatePreviewBranch(repo.Path, previewBranchName)
	if err != nil {
		return fmt.Errorf("failed to check preview branch status: %v", err)
	}

	// Push the worktree branch to a preview branch in main repo
	strategy := git.PushStrategy{
		Remote:    repo.Path,
		Branch:    fmt.Sprintf("%s:%s", worktree.Branch, previewBranchName),
		RemoteURL: repo.Path,
	}

	if shouldForceUpdate {
		log.Printf("🔄 Updating existing preview branch %s", previewBranchName)
		// For force updates, we need to use git command directly since PushStrategy doesn't support force
		output, err := lrm.operations.ExecuteGit(worktree.Path, "push", "--force", repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, previewBranchName))
		if err != nil {
			return fmt.Errorf("failed to create preview branch: %v\n%s", err, output)
		}
	} else {
		err = lrm.operations.PushBranch(worktree.Path, strategy)
		if err != nil {
			return fmt.Errorf("failed to create preview branch: %v", err)
		}
	}

	action := "created"
	if shouldForceUpdate {
		action = "updated"
	}

	if hasUncommittedChanges {
		log.Printf("✅ Preview branch %s %s with uncommitted changes - you can now checkout this branch outside the container", previewBranchName, action)
	} else {
		log.Printf("✅ Preview branch %s %s - you can now checkout this branch outside the container", previewBranchName, action)
	}
	return nil
}

// ShouldCreateInitialWorktree checks if we should create an initial worktree for a repo
func (lrm *LocalRepoManager) ShouldCreateInitialWorktree(repoID string) bool {
	// Check if any worktrees exist for this repo in /workspace
	dirName := filepath.Base(strings.TrimPrefix(repoID, "local/"))
	repoWorkspaceDir := filepath.Join(getWorkspaceDir(), dirName)

	// Check if the repo workspace directory exists and has any worktrees
	if entries, err := os.ReadDir(repoWorkspaceDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				// Check if this directory is a valid git worktree
				if _, err := os.Stat(filepath.Join(repoWorkspaceDir, entry.Name(), ".git")); err == nil {
					log.Printf("🔍 Found existing worktree for %s: %s", repoID, entry.Name())
					return false
				}
			}
		}
	}

	log.Printf("🔍 No existing worktrees found for %s, will create initial worktree", repoID)
	return true
}

// getLocalRepoDefaultBranch gets the current branch of a local repo
func (lrm *LocalRepoManager) getLocalRepoDefaultBranch(repoPath string) string {
	output, err := lrm.operations.ExecuteGit(repoPath, "branch", "--show-current")
	if err != nil {
		log.Printf("⚠️ Could not get current branch for repo at %s, using fallback: main", repoPath)
		return "main"
	}

	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "main"
	}

	return branch
}

// shouldForceUpdatePreviewBranch determines if we should force-update an existing preview branch
func (lrm *LocalRepoManager) shouldForceUpdatePreviewBranch(repoPath, previewBranchName string) (bool, error) {
	// Check if the preview branch exists
	err := lrm.operations.ShowRef(repoPath, fmt.Sprintf("refs/heads/%s", previewBranchName), git.ShowRefOptions{
		Verify: true,
		Quiet:  true,
	})
	if err != nil {
		// Branch doesn't exist, safe to create
		return false, nil
	}

	// Branch exists, check if the last commit was made by us (preview commit)
	output, err := lrm.operations.ExecuteGit(repoPath, "log", "-1", "--pretty=format:%s", previewBranchName)
	if err != nil {
		return false, fmt.Errorf("failed to get last commit message: %v", err)
	}

	lastCommitMessage := strings.TrimSpace(string(output))

	// Check if this looks like our preview commit
	isOurPreviewCommit := strings.Contains(lastCommitMessage, "Preview:") ||
		strings.Contains(lastCommitMessage, "Include all uncommitted changes") ||
		strings.Contains(lastCommitMessage, "preview") // Case insensitive fallback

	if isOurPreviewCommit {
		log.Printf("🔍 Found existing preview branch %s with our commit: '%s'", previewBranchName, lastCommitMessage)
		return true, nil
	}

	// The preview branch exists but doesn't appear to be our commit
	// Let's still allow force update but warn about it
	log.Printf("⚠️  Preview branch %s exists with non-preview commit: '%s' - will force update anyway", previewBranchName, lastCommitMessage)
	return true, nil
}

// createTemporaryCommit creates a temporary commit with all uncommitted changes
func (lrm *LocalRepoManager) createTemporaryCommit(worktreePath string) (string, error) {
	// Add all changes (staged, unstaged, and untracked)
	if err := lrm.operations.Add(worktreePath); err != nil {
		return "", fmt.Errorf("failed to stage changes: %v", err)
	}

	// Create the commit
	if err := lrm.operations.Commit(worktreePath, "Preview: Include all uncommitted changes", git.CommitOptions{}); err != nil {
		return "", fmt.Errorf("failed to create temporary commit: %v", err)
	}

	// Get the commit hash
	commitHash, err := lrm.operations.GetCommitHash(worktreePath, "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %v", err)
	}

	log.Printf("📝 Created temporary commit %s with uncommitted changes", commitHash[:8])
	return commitHash, nil
}
