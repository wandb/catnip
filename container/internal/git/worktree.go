package git

import (
	"fmt"
	"strings"
)

// WorktreeImpl implements the Worktree interface
type WorktreeImpl struct {
	path     string
	branch   string
	executor CommandExecutor
}

// NewWorktree creates a new worktree instance
func NewWorktree(path, branch string, executor CommandExecutor) Worktree {
	return &WorktreeImpl{
		path:     path,
		branch:   branch,
		executor: executor,
	}
}

// GetPath returns the worktree path
func (w *WorktreeImpl) GetPath() string {
	return w.path
}

// GetBranch returns the current branch
func (w *WorktreeImpl) GetBranch() string {
	return w.branch
}

// Checkout switches to a different branch
func (w *WorktreeImpl) Checkout(branch string) error {
	_, err := w.executor.Execute(w.path, "checkout", branch)
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
	}
	w.branch = branch
	return nil
}

// Status returns the worktree status
func (w *WorktreeImpl) Status() (*WorktreeStatus, error) {
	output, err := w.executor.Execute(w.path, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	status := &WorktreeStatus{
		Branch:         w.branch,
		IsDirty:        false,
		HasConflicts:   false,
		UnstagedFiles:  []string{},
		StagedFiles:    []string{},
		UntrackedFiles: []string{},
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		if len(line) < 3 {
			continue
		}

		statusCode := line[:2]
		filename := strings.TrimSpace(line[3:])

		// Check for conflicts
		if statusCode == "UU" || statusCode == "AA" || statusCode == "DD" {
			status.HasConflicts = true
		}

		// Parse status codes
		switch statusCode[0] {
		case 'M', 'A', 'D', 'R', 'C':
			status.StagedFiles = append(status.StagedFiles, filename)
			status.IsDirty = true
		}

		switch statusCode[1] {
		case 'M', 'D':
			status.UnstagedFiles = append(status.UnstagedFiles, filename)
			status.IsDirty = true
		case '?':
			status.UntrackedFiles = append(status.UntrackedFiles, filename)
			status.IsDirty = true
		}
	}

	return status, nil
}

// IsDirty checks if the worktree has uncommitted changes
func (w *WorktreeImpl) IsDirty() bool {
	output, err := w.executor.Execute(w.path, "status", "--porcelain")
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// HasConflicts checks if the worktree has merge conflicts
func (w *WorktreeImpl) HasConflicts() bool {
	output, err := w.executor.Execute(w.path, "status", "--porcelain")
	if err != nil {
		return false
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "UU") || strings.HasPrefix(line, "AA") || strings.HasPrefix(line, "DD") {
			return true
		}
	}
	return false
}

// GetConflictedFiles returns a list of files with conflicts
func (w *WorktreeImpl) GetConflictedFiles() []string {
	output, err := w.executor.Execute(w.path, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return []string{}
	}

	var files []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// Add stages files for commit
func (w *WorktreeImpl) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := w.executor.Execute(w.path, args...)
	if err != nil {
		return fmt.Errorf("failed to add files: %w", err)
	}
	return nil
}

// Commit creates a new commit
func (w *WorktreeImpl) Commit(message string) error {
	_, err := w.executor.Execute(w.path, "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

// GetCommitHash returns the current commit hash
func (w *WorktreeImpl) GetCommitHash() (string, error) {
	output, err := w.executor.Execute(w.path, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Pull pulls changes from the remote
func (w *WorktreeImpl) Pull() error {
	_, err := w.executor.Execute(w.path, "pull", "origin", w.branch)
	if err != nil {
		return fmt.Errorf("failed to pull: %w", err)
	}
	return nil
}

// Push pushes changes to the remote
func (w *WorktreeImpl) Push() error {
	_, err := w.executor.Execute(w.path, "push", "origin", w.branch)
	if err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}
	return nil
}

// PushForce force pushes changes to the remote
func (w *WorktreeImpl) PushForce() error {
	_, err := w.executor.Execute(w.path, "push", "--force-with-lease", "origin", w.branch)
	if err != nil {
		return fmt.Errorf("failed to force push: %w", err)
	}
	return nil
}

// Merge merges another branch into the current branch
func (w *WorktreeImpl) Merge(branch string) error {
	_, err := w.executor.Execute(w.path, "merge", branch)
	if err != nil {
		return fmt.Errorf("failed to merge %s: %w", branch, err)
	}
	return nil
}

// Rebase rebases the current branch onto another branch
func (w *WorktreeImpl) Rebase(branch string) error {
	_, err := w.executor.Execute(w.path, "rebase", branch)
	if err != nil {
		return fmt.Errorf("failed to rebase onto %s: %w", branch, err)
	}
	return nil
}

// Diff returns the diff of the worktree
func (w *WorktreeImpl) Diff() (*WorktreeDiff, error) {
	// Get diff statistics
	output, err := w.executor.Execute(w.path, "diff", "--stat")
	if err != nil {
		return nil, fmt.Errorf("failed to get diff: %w", err)
	}

	// Parse the diff output
	diff := &WorktreeDiff{
		Files: []FileDiff{},
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse file diff lines
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			if len(parts) == 2 {
				filename := strings.TrimSpace(parts[0])
				changes := strings.TrimSpace(parts[1])

				fileDiff := FileDiff{
					Path:   filename,
					Status: "modified",
				}

				// Parse insertions and deletions
				if strings.Contains(changes, "+") && strings.Contains(changes, "-") {
					// Count + and - characters
					for _, ch := range changes {
						switch ch {
						case '+':
							fileDiff.Insertions++
						case '-':
							fileDiff.Deletions++
						}
					}
				}

				diff.Files = append(diff.Files, fileDiff)
				diff.Insertions += fileDiff.Insertions
				diff.Deletions += fileDiff.Deletions
			}
		}
	}

	diff.FilesChanged = len(diff.Files)
	return diff, nil
}

// DiffWithBranch returns the diff between the current branch and another branch
func (w *WorktreeImpl) DiffWithBranch(branch string) (*WorktreeDiff, error) {
	// Get diff statistics
	output, err := w.executor.Execute(w.path, "diff", "--stat", branch)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff with %s: %w", branch, err)
	}

	// Parse the diff output (similar to Diff() method)
	diff := &WorktreeDiff{
		Files: []FileDiff{},
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse file diff lines
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			if len(parts) == 2 {
				filename := strings.TrimSpace(parts[0])
				changes := strings.TrimSpace(parts[1])

				fileDiff := FileDiff{
					Path:   filename,
					Status: "modified",
				}

				// Parse insertions and deletions
				if strings.Contains(changes, "+") && strings.Contains(changes, "-") {
					// Count + and - characters
					for _, ch := range changes {
						switch ch {
						case '+':
							fileDiff.Insertions++
						case '-':
							fileDiff.Deletions++
						}
					}
				}

				diff.Files = append(diff.Files, fileDiff)
				diff.Insertions += fileDiff.Insertions
				diff.Deletions += fileDiff.Deletions
			}
		}
	}

	diff.FilesChanged = len(diff.Files)
	return diff, nil
}
