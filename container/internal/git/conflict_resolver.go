package git

import (
	"fmt"
	"strings"

	"github.com/vanpelt/catnip/internal/models"
)

// ConflictResolver handles all merge conflict detection and resolution operations
type ConflictResolver struct {
	operations Operations
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(operations Operations) *ConflictResolver {
	return &ConflictResolver{
		operations: operations,
	}
}

// CheckSyncConflicts checks if syncing a worktree would cause merge conflicts
func (c *ConflictResolver) CheckSyncConflicts(worktreePath, sourceRef string) (*models.MergeConflictError, error) {
	return c.checkConflicts(worktreePath, sourceRef, "sync", "", "")
}

// CheckMergeConflicts checks if merging would cause conflicts (for local repos)
func (c *ConflictResolver) CheckMergeConflicts(repoPath, worktreePath, sourceBranch, targetBranch, worktreeName string) (*models.MergeConflictError, error) {
	// Create a temporary branch to test the merge
	tempBranch := fmt.Sprintf("temp-merge-check-%d", GetCurrentTimestamp())

	// Push the source branch to temp branch in main repo
	// Ensure the destination is fully qualified as a branch ref
	tempBranchRef := fmt.Sprintf("refs/heads/%s", tempBranch)
	_, err := c.operations.ExecuteGit(worktreePath, "push", repoPath, fmt.Sprintf("%s:%s", sourceBranch, tempBranchRef))
	if err != nil {
		return nil, fmt.Errorf("failed to push temp branch for conflict check: %v", err)
	}

	// Clean up temp branch when done
	defer func() {
		_ = c.operations.DeleteBranch(repoPath, tempBranch, true)
		// Ignore cleanup errors - temp branch will be garbage collected
	}()

	// Try a dry-run merge to detect conflicts
	output, err := c.operations.MergeTree(repoPath, targetBranch, tempBranchRef)
	if err != nil {
		return nil, fmt.Errorf("failed to check merge conflicts: %v", err)
	}

	// Check if merge-tree output indicates conflicts
	if c.hasConflictMarkers(output) {
		conflictFiles := c.parseConflictFiles(output)
		return &models.MergeConflictError{
			Operation:     "merge",
			WorktreeName:  worktreeName,
			WorktreePath:  worktreePath,
			ConflictFiles: conflictFiles,
			Message:       fmt.Sprintf("Merge would cause conflicts in worktree '%s'", worktreeName),
		}, nil
	}

	return nil, nil
}

// IsMergeConflict checks if an error or output indicates a merge conflict
func (c *ConflictResolver) IsMergeConflict(repoPath, output string) bool {
	// First, check if there's actually an active conflict state requiring resolution
	// (rebase/merge in progress with unmerged files)
	if c.hasActiveConflictState(repoPath) {
		return true
	}

	// If no active conflict state, don't treat text-based conflict indicators as true conflicts
	// This handles cases where rebase fails with conflicts but git auto-aborts the operation
	return false
}

// hasActiveConflictState checks if there's an active rebase/merge requiring manual resolution
func (c *ConflictResolver) hasActiveConflictState(repoPath string) bool {
	// Check git status for unmerged paths (actual conflicts requiring resolution)
	statusOutput, err := c.operations.ExecuteGit(repoPath, "status", "--porcelain")
	if err != nil {
		return false
	}

	// Look for unmerged files (status codes AA, AU, DD, DU, UA, UD, UU)
	lines := strings.Split(string(statusOutput), "\n")
	for _, line := range lines {
		if len(line) >= 2 {
			status := line[:2]
			if strings.Contains("AA AU DD DU UA UD UU", status) {
				return true
			}
		}
	}

	return false
}

// CreateMergeConflictError creates a detailed merge conflict error
func (c *ConflictResolver) CreateMergeConflictError(operation, worktreeName, worktreePath, output string) *models.MergeConflictError {
	// Get list of conflicted files
	conflictFiles := c.getConflictedFiles(worktreePath)

	message := fmt.Sprintf("Merge conflict occurred during %s operation in worktree '%s'. Please resolve conflicts in the terminal.", operation, worktreeName)

	return &models.MergeConflictError{
		Operation:     operation,
		WorktreeName:  worktreeName,
		WorktreePath:  worktreePath,
		ConflictFiles: conflictFiles,
		Message:       message,
	}
}

// GetConflictedFiles returns a list of files with merge conflicts
func (c *ConflictResolver) GetConflictedFiles(worktreePath string) ([]string, error) {
	return c.getConflictedFiles(worktreePath), nil
}

// checkConflicts performs conflict detection using merge-tree
func (c *ConflictResolver) checkConflicts(worktreePath, sourceRef, operation, worktreeName, workingTreePath string) (*models.MergeConflictError, error) {
	// Try a dry-run merge to detect conflicts
	output, err := c.operations.MergeTree(worktreePath, "HEAD", sourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to check for conflicts: %v", err)
	}

	// Check if merge-tree output indicates conflicts
	if c.hasConflictMarkers(output) {
		// Parse conflicted files from merge-tree output
		conflictFiles := c.parseConflictFiles(output)

		return &models.MergeConflictError{
			Operation:     operation,
			WorktreeName:  worktreeName,
			WorktreePath:  workingTreePath,
			ConflictFiles: conflictFiles,
			Message:       fmt.Sprintf("%s would cause conflicts in worktree '%s'", operation, worktreeName),
		}, nil
	}

	return nil, nil
}

// hasConflictMarkers checks if the output contains conflict markers
func (c *ConflictResolver) hasConflictMarkers(output string) bool {
	return strings.Contains(output, "<<<<<<< ") ||
		strings.Contains(output, "======= ") ||
		strings.Contains(output, ">>>>>>> ")
}

// parseConflictFiles extracts file names from merge-tree conflict output
func (c *ConflictResolver) parseConflictFiles(output string) []string {
	var conflictFiles []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		// Look for "CONFLICT" lines that often contain file paths
		if strings.Contains(line, "CONFLICT") && strings.Contains(line, " in ") {
			parts := strings.Split(line, " in ")
			if len(parts) > 1 {
				file := strings.TrimSpace(parts[len(parts)-1])
				if file != "" && !contains(conflictFiles, file) {
					conflictFiles = append(conflictFiles, file)
				}
			}
		}
	}

	// Fallback: if we couldn't parse files, indicate conflicts exist
	if len(conflictFiles) == 0 && (strings.Contains(output, "<<<<<<< ") || strings.Contains(output, "CONFLICT")) {
		conflictFiles = []string{"(multiple files)"}
	}

	return conflictFiles
}

// getConflictedFiles returns a list of files with merge conflicts
func (c *ConflictResolver) getConflictedFiles(repoPath string) []string {
	output, err := c.operations.DiffNameOnly(repoPath, "U")
	if err != nil {
		return []string{}
	}

	var conflictFiles []string
	for _, file := range output {
		if file != "" {
			conflictFiles = append(conflictFiles, file)
		}
	}

	return conflictFiles
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
