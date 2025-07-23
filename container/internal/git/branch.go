package git

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vanpelt/catnip/internal/git/executor"
)

// BranchOperations provides branch-related Git operations
type BranchOperations struct {
	executor executor.CommandExecutor
}

// NewBranchOperations creates a new branch operations instance
func NewBranchOperations(exec executor.CommandExecutor) *BranchOperations {
	return &BranchOperations{
		executor: exec,
	}
}

// BranchExistsOptions configures branch existence checking
type BranchExistsOptions struct {
	IsRemote   bool
	RemoteName string // defaults to "origin"
}

// BranchExists checks if a branch exists in a repository with configurable options
func (b *BranchOperations) BranchExists(repoPath, branch string, opts BranchExistsOptions) bool {
	if opts.IsRemote {
		remoteName := opts.RemoteName
		if remoteName == "" {
			remoteName = "origin"
		}
		ref := fmt.Sprintf("refs/remotes/%s/%s", remoteName, branch)
		_, err := b.executor.ExecuteGitWithWorkingDir(repoPath, "show-ref", "--verify", "--quiet", ref)
		return err == nil
	}

	// For branches with full ref path (like refs/catnip/name), use show-ref
	if strings.HasPrefix(branch, "refs/") {
		_, err := b.executor.ExecuteGitWithWorkingDir(repoPath, "show-ref", "--verify", "--quiet", branch)
		return err == nil
	}

	// For local branches, use git branch --list which is more reliable
	output, err := b.executor.ExecuteGitWithWorkingDir(repoPath, "branch", "--list", branch)
	if err != nil {
		return false
	}

	// Check if the output contains the branch name
	return strings.Contains(string(output), branch)
}

// BranchExistsLocal checks if a local branch exists
func (b *BranchOperations) BranchExistsLocal(repoPath, branch string) bool {
	return b.BranchExists(repoPath, branch, BranchExistsOptions{IsRemote: false})
}

// BranchExistsRemote checks if a remote branch exists
func (b *BranchOperations) BranchExistsRemote(repoPath, branch string, remoteName string) bool {
	if remoteName == "" {
		remoteName = "origin"
	}
	return b.BranchExists(repoPath, branch, BranchExistsOptions{
		IsRemote:   true,
		RemoteName: remoteName,
	})
}

// GetCommitCount counts commits between two refs
func (b *BranchOperations) GetCommitCount(repoPath, fromRef, toRef string) (int, error) {
	output, err := b.executor.ExecuteGitWithWorkingDir(repoPath, "rev-list", "--count", fmt.Sprintf("%s..%s", fromRef, toRef))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(output)))
}

// GetRemoteURL gets the remote URL for a repository
func (b *BranchOperations) GetRemoteURL(repoPath string) (string, error) {
	output, err := b.executor.ExecuteGitWithWorkingDir(repoPath, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %v", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetDefaultBranch gets the default branch from a repository
func (b *BranchOperations) GetDefaultBranch(repoPath string) (string, error) {
	// Try symbolic ref first
	output, err := b.executor.ExecuteGitWithWorkingDir(repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		return strings.TrimSpace(strings.TrimPrefix(string(output), "refs/remotes/origin/")), nil
	}

	// Check for main/master in remote branches
	output, err = b.executor.ExecuteGitWithWorkingDir(repoPath, "branch", "-r")
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "origin/main") {
				return "main", nil
			}
			if strings.Contains(line, "origin/master") {
				return "master", nil
			}
		}
	}

	return "main", nil // fallback
}

// GetLocalRepoBranches returns the local branches for a local repository
func (b *BranchOperations) GetLocalRepoBranches(repoPath string) ([]string, error) {
	output, err := b.executor.ExecuteGitWithWorkingDir(repoPath, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("failed to get local branches: %w", err)
	}

	var branches []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}

	return branches, nil
}

// GetRemoteBranches returns remote branches for a repository
func (b *BranchOperations) GetRemoteBranches(repoPath string, defaultBranch string) ([]string, error) {
	// Start with the default branch
	branches := []string{defaultBranch}
	branchSet := map[string]bool{defaultBranch: true}

	output, err := b.executor.ExecuteGitWithWorkingDir(repoPath, "branch", "-r")
	if err != nil {
		return branches, nil // Return at least the default branch
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "HEAD ->") {
			// Remove "origin/" prefix
			branch := line
			if strings.HasPrefix(line, "origin/") {
				branch = strings.TrimPrefix(line, "origin/")
			}

			// Add to list if not already present
			if !branchSet[branch] {
				branches = append(branches, branch)
				branchSet[branch] = true
			}
		}
	}

	return branches, nil
}

// SetupRemoteOrigin sets up or updates the remote origin URL
func (b *BranchOperations) SetupRemoteOrigin(worktreePath, remoteURL string) error {
	_, err := b.executor.ExecuteGitWithWorkingDir(worktreePath, "remote", "set-url", "origin", remoteURL)
	return err
}
