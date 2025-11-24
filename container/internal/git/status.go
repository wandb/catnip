package git

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/vanpelt/catnip/internal/git/executor"
)

// StatusChecker provides Git status checking operations
type StatusChecker struct {
	executor executor.CommandExecutor
}

// NewStatusChecker creates a new status checker
func NewStatusChecker(executor executor.CommandExecutor) *StatusChecker {
	return &StatusChecker{executor: executor}
}

// IsDirty checks if a worktree has uncommitted changes
// Uses fast git operations instead of full status --porcelain which is slow due to gitignore matching
func (s *StatusChecker) IsDirty(worktreePath string) bool {
	// Check for staged or unstaged changes using diff-index (very fast)
	// Returns exit code 1 if there are changes
	_, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "diff-index", "--quiet", "HEAD", "--")
	if err != nil {
		// Non-zero exit code means there are changes
		return true
	}

	// Check for untracked files
	output, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// HasConflicts checks if a worktree is in a conflicted state (rebase/merge in progress)
func (s *StatusChecker) HasConflicts(worktreePath string) bool {
	// Check for rebase in progress
	if _, err := os.Stat(filepath.Join(worktreePath, ".git", "rebase-apply")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(worktreePath, ".git", "rebase-merge")); err == nil {
		return true
	}

	// Check for merge in progress
	if _, err := os.Stat(filepath.Join(worktreePath, ".git", "MERGE_HEAD")); err == nil {
		return true
	}

	// Check for cherry-pick in progress
	if _, err := os.Stat(filepath.Join(worktreePath, ".git", "CHERRY_PICK_HEAD")); err == nil {
		return true
	}

	// Check for unmerged files in git status
	output, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "status", "--porcelain")
	if err != nil {
		return false
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if len(line) >= 2 {
			// Check for conflict markers in status (UU, AA, DD, etc.)
			firstChar := line[0]
			secondChar := line[1]
			if (firstChar == 'U' && secondChar == 'U') || // both modified
				(firstChar == 'A' && secondChar == 'A') || // both added
				(firstChar == 'D' && secondChar == 'D') || // both deleted
				(firstChar == 'A' && secondChar == 'U') || // added by us, modified by them
				(firstChar == 'U' && secondChar == 'A') || // modified by us, added by them
				(firstChar == 'D' && secondChar == 'U') || // deleted by us, modified by them
				(firstChar == 'U' && secondChar == 'D') { // modified by us, deleted by them
				return true
			}
		}
	}

	return false
}

// HasUncommittedChanges checks if the worktree has any uncommitted changes (staged, unstaged, or untracked)
func (s *StatusChecker) HasUncommittedChanges(worktreePath string) (bool, error) {
	// Check for staged changes
	_, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "diff", "--cached", "--quiet")
	if err != nil {
		return true, nil // Has staged changes
	}

	// Check for unstaged changes
	_, err = s.executor.ExecuteGitWithWorkingDir(worktreePath, "diff", "--quiet")
	if err != nil {
		return true, nil // Has unstaged changes
	}

	// Check for untracked files
	output, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return false, err
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

// GetConflictedFiles returns a list of files with conflicts
func (s *StatusChecker) GetConflictedFiles(worktreePath string) ([]string, error) {
	output, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}

	var files []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}

	return files, nil
}

// GetWorktreeStatus returns comprehensive status information
func (s *StatusChecker) GetWorktreeStatus(worktreePath string) (*WorktreeStatus, error) {
	// Get display branch (handles nice name mapping for catnip branches)
	branch, err := s.getDisplayBranch(worktreePath)
	if err != nil {
		return nil, err
	}

	// Get porcelain status
	statusOutput, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	status := &WorktreeStatus{
		Branch:         branch,
		IsDirty:        false,
		HasConflicts:   false,
		UnstagedFiles:  []string{},
		StagedFiles:    []string{},
		UntrackedFiles: []string{},
	}

	lines := strings.Split(strings.TrimSpace(string(statusOutput)), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		indexStatus := line[0]
		workTreeStatus := line[1]
		filename := line[3:]

		status.IsDirty = true

		// Check for conflicts
		if (indexStatus == 'U' && workTreeStatus == 'U') ||
			(indexStatus == 'A' && workTreeStatus == 'A') ||
			(indexStatus == 'D' && workTreeStatus == 'D') ||
			(indexStatus == 'A' && workTreeStatus == 'U') ||
			(indexStatus == 'U' && workTreeStatus == 'A') ||
			(indexStatus == 'D' && workTreeStatus == 'U') ||
			(indexStatus == 'U' && workTreeStatus == 'D') {
			status.HasConflicts = true
		}

		// Categorize files
		if indexStatus != ' ' && indexStatus != '?' {
			status.StagedFiles = append(status.StagedFiles, filename)
		}
		if workTreeStatus != ' ' && workTreeStatus != '?' {
			if workTreeStatus == '?' {
				status.UntrackedFiles = append(status.UntrackedFiles, filename)
			} else {
				status.UnstagedFiles = append(status.UnstagedFiles, filename)
			}
		}
	}

	// Additional conflict checking
	if !status.HasConflicts {
		status.HasConflicts = s.HasConflicts(worktreePath)
	}

	return status, nil
}

// getDisplayBranch returns the display branch name, checking for nice name mapping first
func (s *StatusChecker) getDisplayBranch(worktreePath string) (string, error) {
	// First get the actual git branch/ref
	var actualBranch string
	if branchOutput, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "symbolic-ref", "HEAD"); err == nil {
		actualBranch = strings.TrimSpace(string(branchOutput))
	} else {
		// Fallback to branch --show-current for detached HEAD or other cases
		if branchOutput, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "branch", "--show-current"); err == nil {
			actualBranch = strings.TrimSpace(string(branchOutput))
		} else {
			return "", err
		}
	}

	// If it's a catnip branch, check if there's a nice name mapping
	if strings.HasPrefix(actualBranch, "refs/catnip/") {
		// Check git config for nice branch mapping
		configKey := "catnip.branch-map." + strings.ReplaceAll(actualBranch, "/", ".")
		if niceBranchOutput, err := s.executor.ExecuteGitWithWorkingDir(worktreePath, "config", configKey); err == nil && strings.TrimSpace(string(niceBranchOutput)) != "" {
			// Return the nice branch name
			return strings.TrimSpace(string(niceBranchOutput)), nil
		}
	}

	// Clean up the branch name (remove refs/heads/ prefix if present)
	actualBranch = strings.TrimPrefix(actualBranch, "refs/heads/")

	return actualBranch, nil
}
