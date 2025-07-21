package git

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GoGitWorktree implements Worktree interface using go-git
type GoGitWorktree struct {
	worktree *git.Worktree
	repo     *git.Repository
	path     string
	branch   string
}

// NewGoGitWorktree creates a new go-git worktree
func NewGoGitWorktree(repo *git.Repository, path, branch string) (Worktree, error) {
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	return &GoGitWorktree{
		worktree: worktree,
		repo:     repo,
		path:     path,
		branch:   branch,
	}, nil
}

// GetPath returns the worktree path
func (w *GoGitWorktree) GetPath() string {
	return w.path
}

// GetBranch returns the current branch
func (w *GoGitWorktree) GetBranch() string {
	return w.branch
}

// Checkout switches to a different branch
func (w *GoGitWorktree) Checkout(branch string) error {
	err := w.worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName("refs/heads/" + branch),
	})
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
	}

	w.branch = branch
	return nil
}

// Status returns the worktree status
func (w *GoGitWorktree) Status() (*WorktreeStatus, error) {
	status, err := w.worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	result := &WorktreeStatus{
		Branch:         w.branch,
		IsDirty:        !status.IsClean(),
		HasConflicts:   false,
		UnstagedFiles:  []string{},
		StagedFiles:    []string{},
		UntrackedFiles: []string{},
	}

	for filename, fileStatus := range status {
		switch fileStatus.Staging {
		case git.Added, git.Modified, git.Deleted, git.Renamed, git.Copied:
			result.StagedFiles = append(result.StagedFiles, filename)
		}

		switch fileStatus.Worktree {
		case git.Modified, git.Deleted:
			result.UnstagedFiles = append(result.UnstagedFiles, filename)
		case git.Untracked:
			result.UntrackedFiles = append(result.UntrackedFiles, filename)
		}
	}

	return result, nil
}

// IsDirty checks if the worktree has uncommitted changes
func (w *GoGitWorktree) IsDirty() bool {
	status, err := w.worktree.Status()
	if err != nil {
		return false
	}
	return !status.IsClean()
}

// HasConflicts checks if the worktree has merge conflicts
func (w *GoGitWorktree) HasConflicts() bool {
	status, err := w.worktree.Status()
	if err != nil {
		return false
	}

	for _, fileStatus := range status {
		// Check for conflict indicators in go-git
		if fileStatus.Staging == git.UpdatedButUnmerged || fileStatus.Worktree == git.UpdatedButUnmerged {
			return true
		}
	}
	return false
}

// GetConflictedFiles returns a list of files with conflicts
func (w *GoGitWorktree) GetConflictedFiles() []string {
	status, err := w.worktree.Status()
	if err != nil {
		return []string{}
	}

	var files []string
	for filename, fileStatus := range status {
		if fileStatus.Staging == git.UpdatedButUnmerged || fileStatus.Worktree == git.UpdatedButUnmerged {
			files = append(files, filename)
		}
	}
	return files
}

// Add stages files for commit
func (w *GoGitWorktree) Add(paths ...string) error {
	for _, path := range paths {
		_, err := w.worktree.Add(path)
		if err != nil {
			return fmt.Errorf("failed to add %s: %w", path, err)
		}
	}
	return nil
}

// Commit creates a new commit
func (w *GoGitWorktree) Commit(message string) error {
	_, err := w.worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

// GetCommitHash returns the current commit hash
func (w *GoGitWorktree) GetCommitHash() (string, error) {
	head, err := w.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	return head.Hash().String(), nil
}

// Pull pulls changes from the remote
func (w *GoGitWorktree) Pull() error {
	err := w.worktree.Pull(&git.PullOptions{
		RemoteName: "origin",
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to pull: %w", err)
	}
	return nil
}

// Push pushes changes to the remote
func (w *GoGitWorktree) Push() error {
	err := w.repo.Push(&git.PushOptions{
		RemoteName: "origin",
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push: %w", err)
	}
	return nil
}

// PushForce force pushes changes to the remote
func (w *GoGitWorktree) PushForce() error {
	err := w.repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Force:      true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to force push: %w", err)
	}
	return nil
}

// Merge merges another branch into the current branch
func (w *GoGitWorktree) Merge(branch string) error {
	// Note: go-git has limited merge support
	// For a full implementation, you'd need to implement merge logic
	return fmt.Errorf("merge operation not fully supported in go-git implementation")
}

// Rebase rebases the current branch onto another branch
func (w *GoGitWorktree) Rebase(branch string) error {
	// Note: go-git doesn't have built-in rebase support
	// This is a simplified implementation
	return fmt.Errorf("rebase not supported in go-git worktree implementation")
}

// Diff returns the diff of the worktree
func (w *GoGitWorktree) Diff() (*WorktreeDiff, error) {
	// Get HEAD commit
	head, err := w.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	headCommit, err := w.repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD commit: %w", err)
	}

	// Get HEAD tree (not used in this simplified implementation)
	_, err = headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD tree: %w", err)
	}

	// Get worktree status to find changed files
	status, err := w.worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	diff := &WorktreeDiff{
		Files: []FileDiff{},
	}

	// Convert status to diff format
	for filename, fileStatus := range status {
		if fileStatus.Worktree != git.Unmodified || fileStatus.Staging != git.Unmodified {
			fileDiff := FileDiff{
				Path:   filename,
				Status: w.getFileStatusString(fileStatus.Worktree),
				// Note: go-git doesn't provide line-level diff stats easily
				// For a complete implementation, you'd need to compute patches
				Insertions: 0,
				Deletions:  0,
			}
			diff.Files = append(diff.Files, fileDiff)
		}
	}

	diff.FilesChanged = len(diff.Files)
	return diff, nil
}

// DiffWithBranch returns the diff between current branch and another branch
func (w *GoGitWorktree) DiffWithBranch(branch string) (*WorktreeDiff, error) {
	// Note: Full diff implementation would require tree comparison
	// This is simplified for the interface

	// Note: Full diff implementation would require more complex logic
	// For now, return a simple diff structure
	diff := &WorktreeDiff{
		Files:        []FileDiff{},
		FilesChanged: 0,
		Insertions:   0,
		Deletions:    0,
	}

	// This is a simplified implementation - a real implementation would
	// compare the trees and calculate actual differences
	return diff, nil
}

// getFileStatusString converts go-git status to string
func (w *GoGitWorktree) getFileStatusString(status git.StatusCode) string {
	switch status {
	case git.Modified:
		return "modified"
	case git.Added:
		return "added"
	case git.Deleted:
		return "deleted"
	case git.Renamed:
		return "renamed"
	case git.Copied:
		return "copied"
	case git.Untracked:
		return "untracked"
	case git.UpdatedButUnmerged:
		return "unmerged"
	default:
		return "unknown"
	}
}
