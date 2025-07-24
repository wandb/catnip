package git

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vanpelt/catnip/internal/models"
)

// WorktreeManager handles all worktree lifecycle operations
type WorktreeManager struct {
	operations Operations
}

// NewWorktreeManager creates a new worktree manager
func NewWorktreeManager(operations Operations) *WorktreeManager {
	return &WorktreeManager{
		operations: operations,
	}
}

// CreateWorktreeRequest contains parameters for worktree creation
type CreateWorktreeRequest struct {
	Repository   *models.Repository
	SourceBranch string
	BranchName   string
	WorkspaceDir string
	IsInitial    bool
}

// CreateWorktree creates a new worktree for a repository
func (w *WorktreeManager) CreateWorktree(req CreateWorktreeRequest) (*models.Worktree, error) {
	id := uuid.New().String()

	// Extract repo name from repo ID (e.g., "owner/repo" -> "repo")
	repoParts := strings.Split(req.Repository.ID, "/")
	repoName := repoParts[len(repoParts)-1]

	// All worktrees use repo/branch pattern for consistency
	workspaceName := ExtractWorkspaceName(req.BranchName)
	worktreePath := filepath.Join(req.WorkspaceDir, repoName, workspaceName)

	// Create worktree with new branch using the branch name
	err := w.operations.CreateWorktree(req.Repository.Path, worktreePath, req.BranchName, req.SourceBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %v", err)
	}

	// Get current commit hash
	commitHash, err := w.operations.GetCommitHash(worktreePath, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}

	// Determine source branch (resolve if it's a commit or branch)
	sourceBranch := req.SourceBranch
	if len(req.SourceBranch) == 40 { // Looks like a commit hash
		// Try to find which branch contains this commit
		sourceBranch = w.findSourceBranch(req.Repository.Path, req.SourceBranch, req.BranchName)
	}

	// Calculate commit count ahead of source
	commitCount := 0
	if sourceBranch != req.BranchName {
		if count, err := w.operations.GetCommitCount(worktreePath, sourceBranch, "HEAD"); err == nil {
			commitCount = count
		}
	}

	// Create display name with repo name prefix
	displayName := fmt.Sprintf("%s/%s", repoName, workspaceName)

	worktree := &models.Worktree{
		ID:           id,
		RepoID:       req.Repository.ID,
		Name:         displayName,
		Path:         worktreePath,
		Branch:       req.BranchName,
		SourceBranch: sourceBranch,
		CommitHash:   commitHash,
		CommitCount:  commitCount,
		IsDirty:      false,
		HasConflicts: false,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}

	return worktree, nil
}

// CreateLocalWorktree creates a worktree for a local repository
func (w *WorktreeManager) CreateLocalWorktree(req CreateWorktreeRequest) (*models.Worktree, error) {
	id := uuid.New().String()

	// Extract directory name from repo path
	dirName := filepath.Base(req.Repository.Path)
	workspaceName := ExtractWorkspaceName(req.BranchName)
	worktreePath := filepath.Join(req.WorkspaceDir, dirName, workspaceName)

	// Create worktree directory first
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree directory: %v", err)
	}

	// Create worktree with new branch
	err := w.operations.CreateWorktree(req.Repository.Path, worktreePath, req.BranchName, req.SourceBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %v", err)
	}

	// Add the "live" remote to the worktree pointing back to the main repo
	if err := w.operations.AddRemote(worktreePath, "live", req.Repository.Path); err != nil {
		log.Printf("⚠️ Failed to add live remote: %v", err)
	} else {
		// Fetch the source branch from the live remote to get latest state
		log.Printf("🔄 Fetching latest %s from live remote", req.SourceBranch)
		if err := w.operations.FetchBranch(worktreePath, FetchStrategy{
			Branch:     req.SourceBranch,
			Remote:     "live",
			RemoteName: "live",
			Depth:      1,
		}); err != nil {
			log.Printf("⚠️ Failed to fetch %s from live remote: %v", req.SourceBranch, err)
		}
	}

	// Get current commit hash
	commitHash, err := w.operations.GetCommitHash(worktreePath, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}

	// Clean up source branch name
	sourceBranch := strings.TrimSpace(req.SourceBranch)
	sourceBranch = strings.TrimPrefix(sourceBranch, "*")
	sourceBranch = strings.TrimPrefix(sourceBranch, "+")
	sourceBranch = strings.TrimSpace(sourceBranch)
	sourceBranch = strings.TrimPrefix(sourceBranch, "origin/")

	// Calculate commit count ahead of source
	commitCount := 0
	if sourceBranch != req.BranchName {
		if count, err := w.operations.GetCommitCount(worktreePath, sourceBranch, "HEAD"); err == nil {
			commitCount = count
		}
	}

	// Create display name
	displayName := fmt.Sprintf("%s/%s", dirName, workspaceName)

	worktree := &models.Worktree{
		ID:            id,
		RepoID:        req.Repository.ID,
		Name:          displayName,
		Path:          worktreePath,
		Branch:        req.BranchName,
		SourceBranch:  sourceBranch,
		CommitHash:    commitHash,
		CommitCount:   commitCount,
		CommitsBehind: 0, // Will be calculated later
		IsDirty:       false,
		HasConflicts:  false,
		CreatedAt:     time.Now(),
		LastAccessed:  time.Now(),
	}

	return worktree, nil
}

// DeleteWorktree removes a worktree comprehensively
func (w *WorktreeManager) DeleteWorktree(worktree *models.Worktree, repo *models.Repository) error {
	log.Printf("🗑️ Starting comprehensive cleanup for worktree %s", worktree.Name)

	// Step 1: Remove the worktree directory
	if err := w.operations.RemoveWorktree(repo.Path, worktree.Path, true); err != nil {
		log.Printf("⚠️ Failed to remove worktree directory (continuing with cleanup): %v", err)
	} else {
		log.Printf("✅ Removed worktree directory: %s", worktree.Path)
	}

	// Step 2: Remove the worktree branch
	if worktree.Branch != "" && worktree.Branch != worktree.SourceBranch {
		if err := w.operations.DeleteBranch(repo.Path, worktree.Branch, true); err != nil {
			log.Printf("⚠️ Failed to remove branch %s (may not exist or be in use): %v", worktree.Branch, err)
		} else {
			log.Printf("✅ Removed branch: %s", worktree.Branch)
		}
	}

	// Step 3: Remove preview branch if it exists
	workspaceName := ExtractWorkspaceName(worktree.Branch)
	previewBranchName := fmt.Sprintf("catnip/%s", workspaceName)
	if err := w.operations.DeleteBranch(repo.Path, previewBranchName, true); err != nil {
		log.Printf("ℹ️ No preview branch to remove: %s", previewBranchName)
	} else {
		log.Printf("✅ Removed preview branch: %s", previewBranchName)
	}

	// Step 4: Force remove any remaining files
	if _, err := os.Stat(worktree.Path); err == nil {
		if removeErr := os.RemoveAll(worktree.Path); removeErr != nil {
			log.Printf("⚠️ Failed to force remove worktree directory %s: %v", worktree.Path, removeErr)
		} else {
			log.Printf("✅ Force removed remaining worktree directory: %s", worktree.Path)
		}
	}

	// Step 5: Run garbage collection
	if err := w.operations.GarbageCollect(repo.Path); err != nil {
		log.Printf("⚠️ Failed to run garbage collection after worktree deletion: %v", err)
	} else {
		log.Printf("✅ Ran garbage collection to clean up dangling objects")
	}

	log.Printf("✅ Completed comprehensive cleanup for worktree %s", worktree.Name)
	return nil
}

// detectWorktreeActualState inspects the actual Git state of a worktree
// and returns the real branch/ref. For source branch detection, we rely on stored metadata
// since determining the "correct" source branch is a business logic decision, not a git operation.
func (w *WorktreeManager) detectWorktreeActualState(worktreePath string) (actualBranch string, err error) {
	// Get the actual HEAD reference
	branchOutput, err := w.operations.ExecuteGit(worktreePath, "symbolic-ref", "HEAD")
	if err != nil {
		// Might be detached HEAD, get the commit hash
		if commitHash, hashErr := w.operations.ExecuteGit(worktreePath, "rev-parse", "HEAD"); hashErr == nil {
			actualBranch = strings.TrimSpace(string(commitHash))
		} else {
			return "", fmt.Errorf("failed to get HEAD reference: %v, %v", err, hashErr)
		}
	} else {
		actualBranch = strings.TrimSpace(string(branchOutput))
	}

	return actualBranch, nil
}

// UpdateWorktreeStatus updates the status of a worktree with dynamic state detection
// Note: Fetching should be handled at the service layer before calling this method
func (w *WorktreeManager) UpdateWorktreeStatus(worktree *models.Worktree, getSourceRef func(*models.Worktree) string) {
	// Update basic status
	worktree.IsDirty = w.operations.IsDirty(worktree.Path)
	worktree.HasConflicts = w.operations.HasConflicts(worktree.Path)

	// Detect actual worktree state (branch/ref only - source branch is business logic)
	actualBranch, err := w.detectWorktreeActualState(worktree.Path)
	if err != nil {
		log.Printf("⚠️ Failed to detect actual worktree state for %s: %v", worktree.Name, err)
		// Fall back to stored metadata
	} else {
		// Update stored metadata if it differs from reality
		if actualBranch != worktree.Branch {
			log.Printf("🔄 Worktree %s actual branch (%s) differs from stored (%s), updating",
				worktree.Name, actualBranch, worktree.Branch)
			worktree.Branch = actualBranch
		}
	}

	if worktree.SourceBranch == "" || worktree.SourceBranch == worktree.Branch {
		return
	}

	// Update commit hash to current HEAD
	if commitHash, err := w.operations.GetCommitHash(worktree.Path, "HEAD"); err == nil {
		worktree.CommitHash = commitHash
	}

	// Get source reference
	sourceRef := getSourceRef(worktree)

	// Count commits ahead (our commits)
	if count, err := w.operations.GetCommitCount(worktree.Path, sourceRef, "HEAD"); err == nil {
		worktree.CommitCount = count
	}

	// Count commits behind (missing commits)
	if count, err := w.operations.GetCommitCount(worktree.Path, "HEAD", sourceRef); err == nil {
		worktree.CommitsBehind = count
	}
}

// findSourceBranch tries to find which branch contains a commit, excluding preview branches
func (w *WorktreeManager) findSourceBranch(repoPath, commitHash, currentBranch string) string {
	// Get all branches that might contain this commit
	branchOutput, err := w.operations.ExecuteGit(repoPath, "branch", "--contains", commitHash)
	if err != nil {
		return commitHash // Fall back to commit hash if we can't determine branch
	}

	branches := strings.Split(strings.TrimSpace(string(branchOutput)), "\n")
	for _, branch := range branches {
		// Clean up branch name
		cleanBranch := strings.TrimSpace(branch)
		cleanBranch = strings.TrimPrefix(cleanBranch, "*")
		cleanBranch = strings.TrimPrefix(cleanBranch, "+")
		cleanBranch = strings.TrimSpace(cleanBranch)
		cleanBranch = strings.TrimPrefix(cleanBranch, "origin/")

		// Skip preview branches and the current branch itself
		if strings.HasPrefix(cleanBranch, "preview/") || cleanBranch == currentBranch {
			continue
		}

		// Prefer main/master branches
		if cleanBranch == "main" || cleanBranch == "master" {
			return cleanBranch
		}
	}

	// If no preferred branch found, return the first valid one
	for _, branch := range branches {
		cleanBranch := strings.TrimSpace(branch)
		cleanBranch = strings.TrimPrefix(cleanBranch, "*")
		cleanBranch = strings.TrimPrefix(cleanBranch, "+")
		cleanBranch = strings.TrimSpace(cleanBranch)
		cleanBranch = strings.TrimPrefix(cleanBranch, "origin/")

		if !strings.HasPrefix(cleanBranch, "preview/") && cleanBranch != currentBranch && cleanBranch != "" {
			return cleanBranch
		}
	}

	return commitHash // Fall back to commit hash
}

// CleanupMergedWorktreesRequest contains parameters for cleanup
type CleanupMergedWorktreesRequest struct {
	Worktrees    map[string]*models.Worktree
	Repositories map[string]*models.Repository
	IsLocalRepo  func(string) bool
	DeleteFunc   func(string) error
}

// CleanupMergedWorktreesResponse contains cleanup results
type CleanupMergedWorktreesResponse struct {
	CleanedCount int
	CleanedNames []string
	Errors       []error
}

// CleanupMergedWorktrees removes worktrees that have been fully merged
func (w *WorktreeManager) CleanupMergedWorktrees(req CleanupMergedWorktreesRequest) *CleanupMergedWorktreesResponse {
	var cleanedUp []string
	var errors []error

	log.Printf("🧹 Starting cleanup of merged worktrees, checking %d worktrees", len(req.Worktrees))

	for worktreeID, worktree := range req.Worktrees {
		log.Printf("🔍 Checking worktree %s: dirty=%v, conflicts=%v, commits_ahead=%d, source=%s",
			worktree.Name, worktree.IsDirty, worktree.HasConflicts, worktree.CommitCount, worktree.SourceBranch)

		// Skip if worktree has uncommitted changes or conflicts
		if worktree.IsDirty || worktree.HasConflicts || worktree.CommitCount > 0 {
			log.Printf("⏭️ Skipping cleanup of worktree: %s (dirty=%v, conflicts=%v, commits=%d)",
				worktree.Name, worktree.IsDirty, worktree.HasConflicts, worktree.CommitCount)
			continue
		}

		// Check if the worktree branch exists in the source repo
		repo, exists := req.Repositories[worktree.RepoID]
		if !exists {
			continue
		}

		isMerged := w.isWorktreeMerged(worktree, repo, req.IsLocalRepo(worktree.RepoID))
		if isMerged {
			log.Printf("🧹 Found merged worktree to cleanup: %s", worktree.Name)
			if cleanupErr := req.DeleteFunc(worktreeID); cleanupErr != nil {
				errors = append(errors, fmt.Errorf("failed to cleanup worktree %s: %v", worktree.Name, cleanupErr))
			} else {
				cleanedUp = append(cleanedUp, worktree.Name)
			}
		}
	}

	if len(cleanedUp) > 0 {
		log.Printf("✅ Cleaned up %d merged worktrees: %s", len(cleanedUp), strings.Join(cleanedUp, ", "))
	}

	return &CleanupMergedWorktreesResponse{
		CleanedCount: len(cleanedUp),
		CleanedNames: cleanedUp,
		Errors:       errors,
	}
}

// isWorktreeMerged checks if a worktree has been merged into its source branch
func (w *WorktreeManager) isWorktreeMerged(worktree *models.Worktree, repo *models.Repository, isLocal bool) bool {
	if isLocal {
		// For local repos, check if the branch exists in the main repo
		if !w.operations.BranchExists(repo.Path, worktree.Branch, false) {
			log.Printf("✅ Branch %s no longer exists in main repo (likely merged and deleted)", worktree.Branch)
			return true
		}
	}

	// Check if branch is merged into source branch
	branches, err := w.operations.ListBranches(repo.Path, ListBranchesOptions{Merged: worktree.SourceBranch})
	if err != nil {
		log.Printf("⚠️ Failed to check merged status for %s: %v", worktree.Name, err)
		return false
	}

	for _, branch := range branches {
		// Clean up branch name
		cleanBranch := strings.TrimSpace(branch)
		cleanBranch = strings.TrimPrefix(cleanBranch, "*")
		cleanBranch = strings.TrimPrefix(cleanBranch, "+")
		cleanBranch = strings.TrimSpace(cleanBranch)
		if cleanBranch == worktree.Branch {
			log.Printf("✅ Found %s in merged branches list", worktree.Branch)
			return true
		}
	}

	return false
}

// FileDiff represents a file difference in a worktree
type FileDiff struct {
	FilePath   string `json:"file_path"`
	ChangeType string `json:"change_type"` // "added", "deleted", "modified"
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
	DiffText   string `json:"diff_text,omitempty"`
	IsExpanded bool   `json:"is_expanded"` // Default expansion state
}

// WorktreeDiffResponse represents the diff response for a worktree
type WorktreeDiffResponse struct {
	WorktreeID   string     `json:"worktree_id"`
	WorktreeName string     `json:"worktree_name"`
	SourceBranch string     `json:"source_branch"`
	ForkCommit   string     `json:"fork_commit"` // The commit where this worktree was forked from
	FileDiffs    []FileDiff `json:"file_diffs"`
	TotalFiles   int        `json:"total_files"`
	Summary      string     `json:"summary"`
}

// GetWorktreeDiff calculates diff for a worktree against its source branch
func (w *WorktreeManager) GetWorktreeDiff(worktree *models.Worktree, sourceRef string, fetchLatestRef func(*models.Worktree) error) (*WorktreeDiffResponse, error) {
	// Try to get diff without fetching first (much faster for local changes)

	// Attempt to find merge base with existing references
	mergeBaseOutput, err := w.operations.ExecuteGit(worktree.Path, "merge-base", "HEAD", sourceRef)

	// If merge base fails, try fetching the latest reference and retry
	if err != nil {
		log.Printf("🔄 Merge base not found with existing refs, fetching latest reference for diff")
		if fetchLatestRef != nil {
			if fetchErr := fetchLatestRef(worktree); fetchErr != nil {
				log.Printf("⚠️ Failed to fetch latest reference: %v", fetchErr)
			}
		}

		mergeBaseOutput, err = w.operations.ExecuteGit(worktree.Path, "merge-base", "HEAD", sourceRef)
		if err != nil {
			return nil, fmt.Errorf("failed to find merge base: %v", err)
		}
	}

	forkCommit := strings.TrimSpace(string(mergeBaseOutput))

	// Get the list of changed files from the fork point
	output, err := w.operations.ExecuteGit(worktree.Path, "diff", "--name-status", fmt.Sprintf("%s..HEAD", forkCommit))
	if err != nil {
		return nil, fmt.Errorf("failed to get diff list: %v", err)
	}

	var fileDiffs []FileDiff
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Process committed changes
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		changeType := parts[0]
		filePath := parts[1]

		fileDiff := FileDiff{
			FilePath:   filePath,
			IsExpanded: false, // Default to collapsed for added/deleted files
		}

		switch changeType {
		case "A":
			fileDiff.ChangeType = "added"
			fileDiff.IsExpanded = false // Collapse by default
		case "D":
			fileDiff.ChangeType = "deleted"
			fileDiff.IsExpanded = false // Collapse by default
		case "M":
			fileDiff.ChangeType = "modified"
			fileDiff.IsExpanded = true // Expand by default for modifications
		default:
			fileDiff.ChangeType = "modified"
			fileDiff.IsExpanded = true
		}

		// Get the old content (from fork commit)
		if oldOutput, err := w.operations.ExecuteGit(worktree.Path, "show", fmt.Sprintf("%s:%s", forkCommit, filePath)); err == nil {
			fileDiff.OldContent = string(oldOutput)
		}

		// Get the new content (current HEAD)
		if newOutput, err := w.operations.ExecuteGit(worktree.Path, "show", fmt.Sprintf("HEAD:%s", filePath)); err == nil {
			fileDiff.NewContent = string(newOutput)
		}

		// Also keep the unified diff for fallback
		if diffOutput, err := w.operations.ExecuteGit(worktree.Path, "diff", fmt.Sprintf("%s..HEAD", forkCommit), "--", filePath); err == nil {
			fileDiff.DiffText = string(diffOutput)
		}

		fileDiffs = append(fileDiffs, fileDiff)
	}

	// Also check for unstaged changes
	if unstagedOutput, err := w.operations.ExecuteGit(worktree.Path, "diff", "--name-status"); err == nil {
		unstagedLines := strings.Split(strings.TrimSpace(string(unstagedOutput)), "\n")
		for _, line := range unstagedLines {
			if line == "" {
				continue
			}

			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}

			changeType := parts[0]
			filePath := parts[1]

			// Check if this file already exists in our diff list
			found := false
			for i := range fileDiffs {
				if fileDiffs[i].FilePath == filePath {
					// Update the existing entry to show it has unstaged changes
					if fileDiffs[i].ChangeType == "added" {
						fileDiffs[i].ChangeType = "added + modified (unstaged)"
					} else {
						fileDiffs[i].ChangeType = "modified (unstaged)"
					}

					// Update content to show working directory state
					if newContent, err := os.ReadFile(filepath.Join(worktree.Path, filePath)); err == nil {
						fileDiffs[i].NewContent = string(newContent)
					}

					// Update diff to show unstaged changes
					if diffOutput, err := w.operations.ExecuteGit(worktree.Path, "diff", "--", filePath); err == nil {
						fileDiffs[i].DiffText = string(diffOutput)
					}

					fileDiffs[i].IsExpanded = true
					found = true
					break
				}
			}

			if !found {
				fileDiff := FileDiff{
					FilePath:   filePath,
					IsExpanded: true, // Unstaged changes should be visible
				}

				switch changeType {
				case "A":
					fileDiff.ChangeType = "added (unstaged)"
				case "D":
					fileDiff.ChangeType = "deleted (unstaged)"
				case "M":
					fileDiff.ChangeType = "modified (unstaged)"
				default:
					fileDiff.ChangeType = "modified (unstaged)"
				}

				// Get old content (HEAD version)
				if oldOutput, err := w.operations.ExecuteGit(worktree.Path, "show", fmt.Sprintf("HEAD:%s", filePath)); err == nil {
					fileDiff.OldContent = string(oldOutput)
				}

				// Get new content (working directory)
				if newContent, err := os.ReadFile(filepath.Join(worktree.Path, filePath)); err == nil {
					fileDiff.NewContent = string(newContent)
				}

				// Get unstaged diff content as fallback
				if diffOutput, err := w.operations.ExecuteGit(worktree.Path, "diff", "--", filePath); err == nil {
					fileDiff.DiffText = string(diffOutput)
				}

				fileDiffs = append(fileDiffs, fileDiff)
			}
		}
	}

	// Check for untracked files
	if untrackedOutput, err := w.operations.ExecuteGit(worktree.Path, "ls-files", "--others", "--exclude-standard"); err == nil {
		untrackedFiles := strings.Split(strings.TrimSpace(string(untrackedOutput)), "\n")
		for _, filePath := range untrackedFiles {
			if filePath == "" {
				continue
			}

			fileDiff := FileDiff{
				FilePath:   filePath,
				ChangeType: "added (untracked)",
				IsExpanded: false, // Collapse by default
			}

			// Read file content for untracked files
			if content, err := os.ReadFile(filepath.Join(worktree.Path, filePath)); err == nil {
				fileDiff.NewContent = string(content)
			}

			fileDiffs = append(fileDiffs, fileDiff)
		}
	}

	// Generate summary
	var summary string
	totalFiles := len(fileDiffs)
	switch totalFiles {
	case 0:
		summary = "No changes"
	case 1:
		summary = "1 file changed"
	default:
		summary = fmt.Sprintf("%d files changed", totalFiles)
	}

	return &WorktreeDiffResponse{
		WorktreeName: worktree.Name,
		SourceBranch: worktree.SourceBranch,
		ForkCommit:   forkCommit,
		FileDiffs:    fileDiffs,
		TotalFiles:   totalFiles,
		Summary:      summary,
	}, nil
}
