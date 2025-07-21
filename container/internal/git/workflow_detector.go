package git

import (
	"strings"
)

// WorkflowChangeDetector detects if workflow files are being pushed
type WorkflowChangeDetector struct {
	executor CommandExecutor
}

// NewWorkflowChangeDetector creates a new workflow change detector
func NewWorkflowChangeDetector(executor CommandExecutor) *WorkflowChangeDetector {
	return &WorkflowChangeDetector{executor: executor}
}

// HasWorkflowChanges checks if the current branch has workflow file changes
// compared to the remote tracking branch or origin/main
func (w *WorkflowChangeDetector) HasWorkflowChanges(repoPath, branch string) bool {
	// Try to detect changes against remote tracking branch first
	if hasChanges := w.checkWorkflowChangesAgainstRef(repoPath, "origin/"+branch); hasChanges {
		return true
	}

	// Fallback to checking against origin/main if tracking branch doesn't exist
	if hasChanges := w.checkWorkflowChangesAgainstRef(repoPath, "origin/main"); hasChanges {
		return true
	}

	// Fallback to checking against origin/master
	if hasChanges := w.checkWorkflowChangesAgainstRef(repoPath, "origin/master"); hasChanges {
		return true
	}

	// If no remote exists, check for any workflow files in the current working tree
	return w.HasStagedWorkflowChanges(repoPath) || w.hasWorkflowFilesInWorkingTree(repoPath)
}

// checkWorkflowChangesAgainstRef checks for workflow changes against a specific ref
func (w *WorkflowChangeDetector) checkWorkflowChangesAgainstRef(repoPath, ref string) bool {
	// Get list of changed files between current HEAD and the ref
	output, err := w.executor.ExecuteGitWithWorkingDir(repoPath, "diff", "--name-only", ref+"..HEAD")
	if err != nil {
		// If we can't compare, assume no workflow changes to be safe
		return false
	}

	changedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	return w.containsWorkflowFiles(changedFiles)
}

// HasStagedWorkflowChanges checks if there are staged workflow file changes
func (w *WorkflowChangeDetector) HasStagedWorkflowChanges(repoPath string) bool {
	// Get list of staged files
	output, err := w.executor.ExecuteGitWithWorkingDir(repoPath, "diff", "--cached", "--name-only")
	if err != nil {
		return false
	}

	stagedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	return w.containsWorkflowFiles(stagedFiles)
}

// containsWorkflowFiles checks if the file list contains any workflow files
func (w *WorkflowChangeDetector) containsWorkflowFiles(files []string) bool {
	for _, file := range files {
		if w.isWorkflowFile(file) {
			return true
		}
	}
	return false
}

// isWorkflowFile checks if a file path is a GitHub workflow file
func (w *WorkflowChangeDetector) isWorkflowFile(filePath string) bool {
	if filePath == "" {
		return false
	}

	// Check for GitHub Actions workflows
	if strings.HasPrefix(filePath, ".github/workflows/") &&
		(strings.HasSuffix(filePath, ".yml") || strings.HasSuffix(filePath, ".yaml")) {
		return true
	}

	return false
}

// GetWorkflowFiles returns the list of workflow files that have changed
func (w *WorkflowChangeDetector) GetWorkflowFiles(repoPath, branch string) []string {
	// Try against remote tracking branch first
	if files := w.getWorkflowFilesAgainstRef(repoPath, "origin/"+branch); len(files) > 0 {
		return files
	}

	// Fallback to origin/main
	if files := w.getWorkflowFilesAgainstRef(repoPath, "origin/main"); len(files) > 0 {
		return files
	}

	// Fallback to origin/master
	return w.getWorkflowFilesAgainstRef(repoPath, "origin/master")
}

// getWorkflowFilesAgainstRef gets workflow files changed against a specific ref
func (w *WorkflowChangeDetector) getWorkflowFilesAgainstRef(repoPath, ref string) []string {
	output, err := w.executor.ExecuteGitWithWorkingDir(repoPath, "diff", "--name-only", ref+"..HEAD")
	if err != nil {
		return nil
	}

	changedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	var workflowFiles []string

	for _, file := range changedFiles {
		if w.isWorkflowFile(file) {
			workflowFiles = append(workflowFiles, file)
		}
	}

	return workflowFiles
}

// hasWorkflowFilesInWorkingTree checks if there are any workflow files in the working tree
func (w *WorkflowChangeDetector) hasWorkflowFilesInWorkingTree(repoPath string) bool {
	// List all files in .github/workflows directory if it exists
	output, err := w.executor.ExecuteGitWithWorkingDir(repoPath, "ls-files", ".github/workflows/*.yml", ".github/workflows/*.yaml")
	if err != nil {
		return false
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(files) == 1 && files[0] == "" {
		return false // No files found
	}

	return len(files) > 0
}
