package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/logger"
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

// DetectLocalRepos scans the live directory for any Git repositories and loads them
func (lrm *LocalRepoManager) DetectLocalRepos() map[string]*models.Repository {
	repositories := make(map[string]*models.Repository)

	liveDir := config.Runtime.LiveDir

	// In native mode, only detect the current repo to avoid scanning all sibling repos
	if config.Runtime.IsNative() {
		if config.Runtime.CurrentRepo != "" {
			logger.Infof("📦 Native mode: Running from git repository: %s", config.Runtime.CurrentRepo)
			return lrm.detectCurrentRepo()
		}
		logger.Debug("📁 Native mode: Not running from a git repository, no local repos to detect")
		return repositories
	}

	// Docker mode: Check if live directory exists
	if liveDir == "" {
		logger.Debug("📁 No live directory configured, skipping local repo detection")
		return repositories
	}

	if _, err := os.Stat(liveDir); os.IsNotExist(err) {
		logger.Debug("📁 Live directory does not exist, skipping local repo detection")
		return repositories
	}

	// Read all entries in live directory
	entries, err := os.ReadDir(liveDir)
	if err != nil {
		logger.Errorf("❌ Failed to read live directory: %v", err)
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

		logger.Debugf("🔍 Detected local repository at %s", repoPath)

		// Create repository object
		repoID := fmt.Sprintf("local/%s", entry.Name())
		remoteOrigin, hasGitHubRemote := lrm.getRemoteOriginInfo(repoPath)
		repo := &models.Repository{
			ID:              repoID,
			URL:             "file://" + repoPath,
			Path:            repoPath,
			DefaultBranch:   lrm.getLocalRepoDefaultBranch(repoPath),
			CreatedAt:       time.Now(),
			LastAccessed:    time.Now(),
			RemoteOrigin:    remoteOrigin,
			HasGitHubRemote: hasGitHubRemote,
		}

		repositories[repoID] = repo
		logger.Debugf("✅ Local repository loaded: %s", repoID)
	}

	return repositories
}

// detectCurrentRepo handles the case where we're running from within a git repo in native mode
func (lrm *LocalRepoManager) detectCurrentRepo() map[string]*models.Repository {
	repositories := make(map[string]*models.Repository)

	if config.Runtime.CurrentRepo == "" {
		return repositories
	}

	// Find the actual git repository root using git command
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return repositories
	}
	repoPath := strings.TrimSpace(string(output))

	gitPath := filepath.Join(repoPath, ".git")
	// Verify the git directory exists
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		return repositories
	}

	// Get current branch
	branchOutput, err := lrm.operations.ExecuteGit(repoPath, "branch", "--show-current")
	branch := "main"
	if err == nil {
		branch = strings.TrimSpace(string(branchOutput))
		if branch == "" {
			branch = "main"
		}
	}

	// Create repository object
	repoName := config.Runtime.CurrentRepo
	repoID := fmt.Sprintf("local/%s", repoName)
	remoteOrigin, hasGitHubRemote := lrm.getRemoteOriginInfo(repoPath)

	repo := &models.Repository{
		ID:              repoID,
		URL:             "file://" + repoPath,
		Path:            repoPath,
		DefaultBranch:   branch,
		CreatedAt:       time.Now(),
		LastAccessed:    time.Now(),
		Description:     fmt.Sprintf("Local repository: %s", repoName),
		RemoteOrigin:    remoteOrigin,
		HasGitHubRemote: hasGitHubRemote,
	}

	logger.Infof("✅ Detected current repository: %s (branch: %s)", repoName, branch)
	repositories[repoID] = repo

	return repositories
}

// CreateWorktreePreview creates a preview branch in the main repo for viewing changes outside container
func (lrm *LocalRepoManager) CreateWorktreePreview(repo *models.Repository, worktree *models.Worktree) error {
	// Extract workspace name for preview branch
	workspaceName := git.ExtractWorkspaceName(worktree.Branch)
	previewBranchName := fmt.Sprintf("catnip/%s", workspaceName)
	logger.Debugf("🔍 Creating preview branch %s for worktree %s", previewBranchName, worktree.Name)

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
		Branch:    fmt.Sprintf("%s:refs/heads/%s", worktree.Branch, previewBranchName),
		RemoteURL: repo.Path,
	}

	if shouldForceUpdate {
		logger.Debugf("🔄 Updating existing preview branch %s", previewBranchName)
		// For force updates, we need to use git command directly since PushStrategy doesn't support force
		output, err := lrm.operations.ExecuteGit(worktree.Path, "push", "--force", repo.Path, fmt.Sprintf("%s:refs/heads/%s", worktree.Branch, previewBranchName))
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
		logger.Infof("✅ Preview branch %s %s with uncommitted changes - you can now checkout this branch outside the container", previewBranchName, action)
	} else {
		logger.Infof("✅ Preview branch %s %s - you can now checkout this branch outside the container", previewBranchName, action)
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
					logger.Debugf("🔍 Found existing worktree for %s: %s", repoID, entry.Name())
					return false
				}
			}
		}
	}

	logger.Debugf("🔍 No existing worktrees found for %s, will create initial worktree", repoID)
	return true
}

// getLocalRepoDefaultBranch delegates to git helper for determining the actual default branch
func (lrm *LocalRepoManager) getLocalRepoDefaultBranch(repoPath string) string {
	// Use the git helper function to determine the default branch
	// This ensures consistent logic across the codebase
	return git.GetDefaultBranch(lrm.operations, repoPath)
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

	// Branch exists - always force update preview branches since they should reflect latest worktree state
	output, err := lrm.operations.ExecuteGit(repoPath, "log", "-1", "--pretty=format:%s", previewBranchName)
	if err != nil {
		return false, fmt.Errorf("failed to get last commit message: %v", err)
	}

	lastCommitMessage := strings.TrimSpace(string(output))
	logger.Debugf("🔄 Found existing preview branch %s with commit: '%s' - will force update", previewBranchName, lastCommitMessage)
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

	logger.Debugf("📝 Created temporary commit %s with uncommitted changes", commitHash[:8])
	return commitHash, nil
}

// getRemoteOriginInfo gets the remote origin URL and determines if it's a GitHub repository
func (lrm *LocalRepoManager) getRemoteOriginInfo(repoPath string) (string, bool) {
	remoteURL, err := lrm.operations.GetRemoteURL(repoPath)
	if err != nil {
		logger.Debugf("🔍 No remote origin found for %s: %v", repoPath, err)
		return "", false
	}

	// Check if it's a GitHub URL
	isGitHub := strings.Contains(remoteURL, "github.com")

	logger.Debugf("🔍 Remote origin for %s: %s (GitHub: %v)", repoPath, remoteURL, isGitHub)
	return remoteURL, isGitHub
}
