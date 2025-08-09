package git

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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

	// For local branches, always use show-ref with refs/heads/ prefix
	// This is more reliable than git branch --list, especially when on custom refs
	ref := fmt.Sprintf("refs/heads/%s", branch)
	_, err := b.executor.ExecuteGitWithWorkingDir(repoPath, "show-ref", "--verify", "--quiet", ref)
	return err == nil
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
	// First, try to get the remote URL and use ls-remote for accurate branch list
	remoteURL, err := b.GetRemoteURL(repoPath)
	if err == nil && remoteURL != "" {
		// Try ls-remote with the remote URL directly (more reliable) - use timeout for network operations
		output, err := b.executor.ExecuteWithEnvAndTimeout("", nil, 10*time.Second, "-C", repoPath, "ls-remote", "--heads", remoteURL)
		if err == nil {
			var branches []string
			branchSet := map[string]bool{}

			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Each line is in format: <commit-hash> refs/heads/<branch-name>
				parts := strings.Fields(line)
				if len(parts) >= 2 && strings.HasPrefix(parts[1], "refs/heads/") {
					branch := strings.TrimPrefix(parts[1], "refs/heads/")
					if !branchSet[branch] {
						branches = append(branches, branch)
						branchSet[branch] = true
					}
				}
			}

			if len(branches) > 0 {
				return branches, nil
			}
		}

		// If the direct URL approach failed, try with origin - with timeout
		output, err = b.executor.ExecuteWithEnvAndTimeout("", nil, 10*time.Second, "-C", repoPath, "ls-remote", "--heads", "origin")
		if err == nil {
			var branches []string
			branchSet := map[string]bool{}

			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Each line is in format: <commit-hash> refs/heads/<branch-name>
				parts := strings.Fields(line)
				if len(parts) >= 2 && strings.HasPrefix(parts[1], "refs/heads/") {
					branch := strings.TrimPrefix(parts[1], "refs/heads/")
					if !branchSet[branch] {
						branches = append(branches, branch)
						branchSet[branch] = true
					}
				}
			}

			if len(branches) > 0 {
				return branches, nil
			}
		}
	}

	// Fallback to using local remote-tracking branches (cached)
	var branches []string
	branchSet := map[string]bool{}

	// Start with default branch if we have one
	if defaultBranch != "" {
		branches = append(branches, defaultBranch)
		branchSet[defaultBranch] = true
	}

	// Try to get remote-tracking branches (these are cached locally)
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
