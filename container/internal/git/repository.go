package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RepositoryImpl implements the Repository interface using git commands
type RepositoryImpl struct {
	path     string
	executor CommandExecutor
}

// NewRepository creates a new repository instance
func NewRepository(path string, executor CommandExecutor) Repository {
	return &RepositoryImpl{
		path:     path,
		executor: executor,
	}
}

// Clone clones a repository from a URL to the specified path
func (r *RepositoryImpl) Clone(url, path string) error {
	args := []string{"clone", "--bare", url, path}
	_, err := r.executor.Execute("", args...)
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	r.path = path
	return nil
}

// GetPath returns the repository path
func (r *RepositoryImpl) GetPath() string {
	return r.path
}

// GetRemoteURL returns the remote origin URL
func (r *RepositoryImpl) GetRemoteURL() (string, error) {
	output, err := r.executor.Execute(r.path, "config", "--get", "remote.origin.url")
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetDefaultBranch returns the default branch of the repository
func (r *RepositoryImpl) GetDefaultBranch() (string, error) {
	// Try to get the default branch from the remote
	output, err := r.executor.Execute(r.path, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		branch := strings.TrimSpace(string(output))
		branch = strings.TrimPrefix(branch, "refs/remotes/origin/")
		return branch, nil
	}

	// Fallback to checking HEAD
	output, err = r.executor.Execute(r.path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "main", nil // Default fallback
	}

	return strings.TrimSpace(string(output)), nil
}

// ListBranches returns all branches in the repository
func (r *RepositoryImpl) ListBranches() ([]string, error) {
	output, err := r.executor.Execute(r.path, "branch", "-a")
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	var branches []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Remove branch prefix markers
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimSpace(line)

		// Skip HEAD references
		if strings.Contains(line, "HEAD") {
			continue
		}

		// Extract branch name from remote branches
		if strings.HasPrefix(line, "remotes/origin/") {
			branch := strings.TrimPrefix(line, "remotes/origin/")
			if !Contains(branches, branch) {
				branches = append(branches, branch)
			}
		} else if !strings.HasPrefix(line, "remotes/") {
			// Local branches
			if !Contains(branches, line) {
				branches = append(branches, line)
			}
		}
	}

	return branches, nil
}

// BranchExists checks if a branch exists
func (r *RepositoryImpl) BranchExists(branch string) bool {
	// Check local branches
	_, err := r.executor.Execute(r.path, "rev-parse", "--verify", branch)
	if err == nil {
		return true
	}

	// Check remote branches
	_, err = r.executor.Execute(r.path, "rev-parse", "--verify", "origin/"+branch)
	return err == nil
}

// CreateBranch creates a new branch
func (r *RepositoryImpl) CreateBranch(branch, from string) error {
	args := []string{"branch", branch}
	if from != "" {
		args = append(args, from)
	}

	_, err := r.executor.Execute(r.path, args...)
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branch, err)
	}
	return nil
}

// Fetch fetches all references from the remote
func (r *RepositoryImpl) Fetch() error {
	_, err := r.executor.Execute(r.path, "fetch", "--all", "--prune")
	if err != nil {
		return fmt.Errorf("failed to fetch: %w", err)
	}
	return nil
}

// FetchBranch fetches a specific branch
func (r *RepositoryImpl) FetchBranch(branch string) error {
	_, err := r.executor.Execute(r.path, "fetch", "origin", branch+":"+branch, "--force")
	if err != nil {
		return fmt.Errorf("failed to fetch branch %s: %w", branch, err)
	}
	return nil
}

// FetchWithDepth fetches with a specific depth
func (r *RepositoryImpl) FetchWithDepth(branch string, depth int) error {
	args := []string{"fetch", "origin", branch, "--depth", fmt.Sprintf("%d", depth)}
	_, err := r.executor.Execute(r.path, args...)
	if err != nil {
		return fmt.Errorf("failed to fetch branch %s with depth %d: %w", branch, depth, err)
	}
	return nil
}

// IsBare checks if the repository is bare
func (r *RepositoryImpl) IsBare() bool {
	output, err := r.executor.Execute(r.path, "config", "--get", "core.bare")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// IsShallow checks if the repository is shallow
func (r *RepositoryImpl) IsShallow() bool {
	shallowFile := filepath.Join(r.path, "shallow")
	output, err := r.executor.Execute("", "test", "-f", shallowFile)
	return err == nil && len(output) == 0
}

// Unshallow converts a shallow repository to a complete one
func (r *RepositoryImpl) Unshallow() error {
	if !r.IsShallow() {
		return nil
	}

	_, err := r.executor.Execute(r.path, "fetch", "--unshallow")
	if err != nil {
		return fmt.Errorf("failed to unshallow repository: %w", err)
	}
	return nil
}
