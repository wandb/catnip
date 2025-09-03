package handlers

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// GitHandler handles Git-related API endpoints
type GitHandler struct {
	gitService     *services.GitService
	gitHTTPService *services.GitHTTPService
	sessionService *services.SessionService
	claudeMonitor  *services.ClaudeMonitorService
}

// CheckoutResponse represents the response when checking out a repository
// @Description Response containing repository and worktree information after checkout
type CheckoutResponse struct {
	// Repository information
	Repository models.Repository `json:"repository"`
	// Created worktree information
	Worktree models.Worktree `json:"worktree"`
	// Success message
	Message string `json:"message" example:"Repository checked out successfully"`
}

// GitHubRepository represents a GitHub repository from the API
// @Description GitHub repository information from the GitHub API
type GitHubRepository struct {
	// GitHub repository ID
	ID int `json:"id" example:"123456789"`
	// Repository name
	Name string `json:"name" example:"claude-code"`
	// Full repository name (org/repo)
	FullName string `json:"full_name" example:"anthropics/claude-code"`
	// Repository description
	Description string `json:"description" example:"AI coding assistant"`
	// Whether the repository is private
	Private bool `json:"private" example:"false"`
	// Repository URL
	HTMLURL string `json:"html_url" example:"https://github.com/anthropics/claude-code"`
	// Git clone URL
	CloneURL string `json:"clone_url" example:"https://github.com/anthropics/claude-code.git"`
}

// ConflictCheckResponse represents the response when checking for conflicts
// @Description Response containing conflict information for sync/merge operations
type ConflictCheckResponse struct {
	// Whether conflicts were detected
	HasConflicts bool `json:"has_conflicts" example:"true"`
	// Operation type (sync/merge)
	Operation string `json:"operation,omitempty" example:"sync"`
	// Name of the worktree
	WorktreeName string `json:"worktree_name,omitempty" example:"feature-branch"`
	// List of files with conflicts
	ConflictFiles []string `json:"conflict_files,omitempty" example:"[\"src/main.go\", \"README.md\"]"`
	// Status message
	Message string `json:"message" example:"No conflicts detected"`
}

// CreateGitHubRepositoryRequest represents a request to create a GitHub repository
// @Description Request to create a new GitHub repository and set it as origin
type CreateGitHubRepositoryRequest struct {
	// Repository name
	Name string `json:"name" example:"my-project"`
	// Repository description
	Description string `json:"description" example:"My awesome project"`
	// Whether the repository should be private
	IsPrivate bool `json:"is_private" example:"false"`
}

// CreateGitHubRepositoryResponse represents the response for creating a GitHub repository
// @Description Response containing the created repository information
type CreateGitHubRepositoryResponse struct {
	// URL of the created repository
	URL string `json:"url" example:"https://github.com/user/repo"`
	// Success message
	Message string `json:"message" example:"Repository created and origin updated successfully"`
}

// WorktreeOperationResponse represents the response for worktree operations
// @Description Response for worktree operations like delete, sync, merge, preview
type WorktreeOperationResponse struct {
	// Operation result message
	Message string `json:"message" example:"Worktree deleted successfully"`
	// Worktree ID
	ID string `json:"id" example:"abc123-def456-ghi789"`
	// Strategy used for sync operations
	Strategy string `json:"strategy,omitempty" example:"rebase"`
}

// WorktreeDiffResponse represents the response containing diff information
// @Description Response containing git diff information for a worktree
type WorktreeDiffResponse struct {
	// Raw git diff output
	Diff string `json:"diff" example:"diff --git a/main.go b/main.go..."`
	// List of changed files
	FilesChanged []string `json:"files_changed" example:"[\"main.go\", \"README.md\"]"`
	// Number of lines added
	Additions int `json:"additions" example:"25"`
	// Number of lines deleted
	Deletions int `json:"deletions" example:"10"`
	// Diff summary
	Summary string `json:"summary" example:"2 files changed, 25 insertions(+), 10 deletions(-)"`
}

// NewGitHandler creates a new Git handler
func NewGitHandler(gitService *services.GitService, gitHTTPService *services.GitHTTPService, sessionService *services.SessionService, claudeMonitor *services.ClaudeMonitorService) *GitHandler {
	return &GitHandler{
		gitService:     gitService,
		gitHTTPService: gitHTTPService,
		sessionService: sessionService,
		claudeMonitor:  claudeMonitor,
	}
}

// CheckoutRepository handles repository checkout requests
// @Summary Checkout a GitHub repository
// @Description Clones a GitHub repository as a bare repo and creates initial worktree
// @Tags git
// @Accept json
// @Produce json
// @Param org path string true "Organization name"
// @Param repo path string true "Repository name"
// @Param branch query string false "Branch name (optional)"
// @Success 200 {object} CheckoutResponse
// @Router /v1/git/checkout/{org}/{repo} [post]
func (h *GitHandler) CheckoutRepository(c *fiber.Ctx) error {
	org := c.Params("org")
	repo := c.Params("repo")
	branch := c.Query("branch", "")

	logger.Infof("📦 Checkout request: %s/%s (branch: %s)", org, repo, branch)

	repository, worktree, err := h.gitService.CheckoutRepository(org, repo, branch)
	if err != nil {
		logger.Errorf("❌ Checkout failed: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"repository": repository,
		"worktree":   worktree,
		"message":    "Repository checked out successfully",
	})
}

// GetStatus returns the current Git status
// @Summary Get Git status
// @Description Returns the current repository and worktree status
// @Tags git
// @Produce json
// @Success 200 {object} models.GitStatus
// @Router /v1/git/status [get]
func (h *GitHandler) GetStatus(c *fiber.Ctx) error {
	status := h.gitService.GetStatus()
	return c.JSON(status)
}

// EnhancedWorktree represents a worktree with cache status metadata
type EnhancedWorktree struct {
	*models.Worktree
	CacheStatus *WorktreeCacheStatus `json:"cache_status,omitempty"`
}

// WorktreeCacheStatus represents the cache status for a worktree
type WorktreeCacheStatus struct {
	IsCached    bool   `json:"is_cached"`
	IsLoading   bool   `json:"is_loading"`
	LastUpdated *int64 `json:"last_updated,omitempty"` // Unix timestamp in milliseconds
}

// ListWorktrees returns all worktrees with cache-enhanced responses
// @Summary List all worktrees
// @Description Returns a list of all worktrees for the current repository with fast cache-enhanced responses
// @Tags git
// @Produce json
// @Success 200 {array} EnhancedWorktree
// @Router /v1/git/worktrees [get]
func (h *GitHandler) ListWorktrees(c *fiber.Ctx) error {
	worktrees := h.gitService.ListWorktrees()
	enhancedWorktrees := make([]*EnhancedWorktree, 0, len(worktrees))

	for _, worktree := range worktrees {
		// Enhance worktrees with session information
		if sessionInfo, exists := h.sessionService.GetActiveSession(worktree.Path); exists {
			// Convert services.TitleEntry to models.TitleEntry
			if sessionInfo.Title != nil {
				worktree.SessionTitle = &models.TitleEntry{
					Title:      sessionInfo.Title.Title,
					Timestamp:  sessionInfo.Title.Timestamp,
					CommitHash: sessionInfo.Title.CommitHash,
				}
			}

			// Convert []services.TitleEntry to []models.TitleEntry
			if len(sessionInfo.TitleHistory) > 0 {
				history := make([]models.TitleEntry, len(sessionInfo.TitleHistory))
				for i, entry := range sessionInfo.TitleHistory {
					history[i] = models.TitleEntry{
						Title:      entry.Title,
						Timestamp:  entry.Timestamp,
						CommitHash: entry.CommitHash,
					}
				}
				worktree.SessionTitleHistory = history
			}
		}

		// Determine Claude activity state for this worktree
		claudeActivityState := h.sessionService.GetClaudeActivityState(worktree.Path)
		worktree.ClaudeActivityState = claudeActivityState

		// Set backward compatibility flag
		worktree.HasActiveClaudeSession = (claudeActivityState == models.ClaudeActive || claudeActivityState == models.ClaudeRunning)

		// Get todos for this worktree
		if todos, err := h.claudeMonitor.GetTodos(worktree.Path); err == nil {
			worktree.Todos = todos
		}
		// If there's an error getting todos, we'll leave Todos as nil (which is fine)

		// Create enhanced worktree with cache status
		enhanced := &EnhancedWorktree{
			Worktree: worktree,
			CacheStatus: &WorktreeCacheStatus{
				IsCached:  h.gitService.IsWorktreeStatusCached(worktree.ID),
				IsLoading: !h.gitService.IsWorktreeStatusCached(worktree.ID), // Loading if not cached
			},
		}

		enhancedWorktrees = append(enhancedWorktrees, enhanced)
	}

	return c.JSON(enhancedWorktrees)
}

// UpdateWorktree updates specific fields of a worktree
// @Summary Update worktree fields
// @Description Updates specific fields of a worktree (for testing purposes)
// @Tags git
// @Accept json
// @Produce json
// @Param id path string true "Worktree ID"
// @Param updates body object true "Fields to update"
// @Success 200 {object} models.Worktree
// @Router /v1/git/worktrees/{id} [patch]
func (h *GitHandler) UpdateWorktree(c *fiber.Ctx) error {
	worktreeID := c.Params("id")
	if worktreeID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Worktree ID is required",
		})
	}

	// Parse the request body to get the fields to update
	var updates map[string]interface{}
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("Invalid request body: %v", err),
		})
	}

	// Update the worktree using the state manager
	if err := h.gitService.UpdateWorktreeFields(worktreeID, updates); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to update worktree: %v", err),
		})
	}

	// Get the updated worktree
	worktree, exists := h.gitService.GetWorktree(worktreeID)
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Worktree not found",
		})
	}

	return c.JSON(worktree)
}

// ListGitHubRepositories returns user's GitHub repositories
// @Summary List GitHub repositories
// @Description Returns a list of GitHub repositories accessible to the authenticated user
// @Tags git
// @Produce json
// @Success 200 {array} GitHubRepository
// @Router /v1/git/github/repos [get]
func (h *GitHandler) ListGitHubRepositories(c *fiber.Ctx) error {
	repos, err := h.gitService.ListGitHubRepositories()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(repos)
}

// GetRepositoryBranches returns remote branches for a repository
// @Summary Get repository branches
// @Description Returns a list of remote branches for a specific repository
// @Tags git
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Success 200 {array} string
// @Router /v1/git/branches/{repo_id} [get]
func (h *GitHandler) GetRepositoryBranches(c *fiber.Ctx) error {
	repoID := c.Params("repo_id")

	// URL decode the repo ID to handle slashes
	decodedRepoID, err := url.QueryUnescape(repoID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid repository ID",
		})
	}

	branches, err := h.gitService.GetRepositoryBranches(decodedRepoID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(branches)
}

// DeleteWorktree removes a worktree
// @Summary Delete worktree
// @Description Removes a worktree from the repository
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} WorktreeOperationResponse
// @Router /v1/git/worktrees/{id} [delete]
func (h *GitHandler) DeleteWorktree(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	_, err := h.gitService.DeleteWorktree(worktreeID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Worktree deleted successfully",
		"id":      worktreeID,
	})
}

// SyncWorktree syncs a worktree with its source branch
// @Summary Sync worktree with source branch
// @Description Syncs a worktree with its source branch using merge or rebase strategy
// @Tags git
// @Accept json
// @Produce json
// @Param id path string true "Worktree ID"
// @Param body body map[string]string true "Sync options"
// @Success 200 {object} WorktreeOperationResponse
// @Router /v1/git/worktrees/{id}/sync [post]
func (h *GitHandler) SyncWorktree(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	var syncRequest struct {
		Strategy string `json:"strategy"`
	}

	if err := c.BodyParser(&syncRequest); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Default to rebase strategy if not specified
	if syncRequest.Strategy == "" {
		syncRequest.Strategy = "rebase"
	}

	if err := h.gitService.SyncWorktree(worktreeID, syncRequest.Strategy); err != nil {
		// Check if this is a merge conflict error
		var mergeConflictErr *models.MergeConflictError
		if errors.As(err, &mergeConflictErr) {
			return c.Status(409).JSON(fiber.Map{
				"error":          "merge_conflict",
				"message":        mergeConflictErr.Message,
				"operation":      mergeConflictErr.Operation,
				"worktree_name":  mergeConflictErr.WorktreeName,
				"worktree_path":  mergeConflictErr.WorktreePath,
				"conflict_files": mergeConflictErr.ConflictFiles,
			})
		}
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":  "Worktree synced successfully",
		"id":       worktreeID,
		"strategy": syncRequest.Strategy,
	})
}

// MergeWorktreeToMain merges a worktree's changes back to the main repository
// @Summary Merge worktree to main
// @Description Merges a local repo worktree's changes back to the main repository
// @Tags git
// @Accept json
// @Produce json
// @Param id path string true "Worktree ID"
// @Param body body map[string]string false "Merge options"
// @Success 200 {object} WorktreeOperationResponse
// @Router /v1/git/worktrees/{id}/merge [post]
func (h *GitHandler) MergeWorktreeToMain(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	var mergeRequest struct {
		Squash bool `json:"squash"`
	}

	// Parse body if present, but don't require it for backwards compatibility
	_ = c.BodyParser(&mergeRequest)

	if err := h.gitService.MergeWorktreeToMain(worktreeID, mergeRequest.Squash); err != nil {
		// Check if this is a merge conflict error
		var mergeConflictErr *models.MergeConflictError
		if errors.As(err, &mergeConflictErr) {
			return c.Status(409).JSON(fiber.Map{
				"error":          "merge_conflict",
				"message":        mergeConflictErr.Message,
				"operation":      mergeConflictErr.Operation,
				"worktree_name":  mergeConflictErr.WorktreeName,
				"worktree_path":  mergeConflictErr.WorktreePath,
				"conflict_files": mergeConflictErr.ConflictFiles,
			})
		}
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Parse auto_cleanup parameter (default to false for safety)
	autoCleanup := c.QueryBool("auto_cleanup", false)

	response := fiber.Map{
		"message": "Worktree merged to main successfully",
		"id":      worktreeID,
	}

	// Automatically clean up the worktree after successful merge if requested
	if autoCleanup {
		_, cleanupErr := h.gitService.DeleteWorktree(worktreeID)
		if cleanupErr != nil {
			// Don't fail the response, just warn about cleanup failure
			response["cleanup_warning"] = "Merge succeeded but worktree cleanup failed: " + cleanupErr.Error()
		} else {
			response["cleanup"] = "Worktree automatically deleted after successful merge"
		}
	}

	return c.JSON(response)
}

// CleanupMergedWorktrees removes worktrees that have been fully merged
// @Summary Cleanup merged worktrees
// @Description Removes worktrees that have been fully merged into their source branch
// @Tags git
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/git/worktrees/cleanup [post]
func (h *GitHandler) CleanupMergedWorktrees(c *fiber.Ctx) error {
	cleanedCount, cleanedNames, err := h.gitService.CleanupMergedWorktrees()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":         err.Error(),
			"cleaned_count": cleanedCount,
			"cleaned_names": cleanedNames,
		})
	}

	return c.JSON(fiber.Map{
		"message":       "Merged worktrees cleanup completed successfully",
		"cleaned_count": cleanedCount,
		"cleaned_names": cleanedNames,
	})
}

// CreateWorktreePreview creates a preview branch for viewing changes outside container
// @Summary Create worktree preview
// @Description Creates a preview branch in the main repo for viewing changes outside container
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} WorktreeOperationResponse
// @Router /v1/git/worktrees/{id}/preview [post]
func (h *GitHandler) CreateWorktreePreview(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	if err := h.gitService.CreateWorktreePreview(worktreeID); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Preview branch created successfully",
		"id":      worktreeID,
	})
}

// CheckSyncConflicts checks if syncing a worktree would cause conflicts
// @Summary Check sync conflicts
// @Description Checks if syncing a worktree would cause merge conflicts
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} ConflictCheckResponse
// @Router /v1/git/worktrees/{id}/sync/check [get]
func (h *GitHandler) CheckSyncConflicts(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	conflictErr, err := h.gitService.CheckSyncConflicts(worktreeID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if conflictErr != nil {
		return c.JSON(fiber.Map{
			"has_conflicts":  true,
			"operation":      conflictErr.Operation,
			"worktree_name":  conflictErr.WorktreeName,
			"conflict_files": conflictErr.ConflictFiles,
			"message":        conflictErr.Message,
		})
	}

	return c.JSON(fiber.Map{
		"has_conflicts": false,
		"message":       "No conflicts detected for sync operation",
	})
}

// CheckMergeConflicts checks if merging a worktree would cause conflicts
// @Summary Check merge conflicts
// @Description Checks if merging a worktree to main would cause conflicts
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} ConflictCheckResponse
// @Router /v1/git/worktrees/{id}/merge/check [get]
func (h *GitHandler) CheckMergeConflicts(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	conflictErr, err := h.gitService.CheckMergeConflicts(worktreeID)
	if err != nil {
		logger.Errorf("❌ CheckMergeConflicts failed for worktree %s: %v", worktreeID, err)
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if conflictErr != nil {
		return c.JSON(fiber.Map{
			"has_conflicts":  true,
			"operation":      conflictErr.Operation,
			"worktree_name":  conflictErr.WorktreeName,
			"conflict_files": conflictErr.ConflictFiles,
			"message":        conflictErr.Message,
		})
	}

	return c.JSON(fiber.Map{
		"has_conflicts": false,
		"message":       "No conflicts detected for merge operation",
	})
}

// GetWorktreeDiff returns the diff for a worktree against its source branch
// @Summary Get worktree diff
// @Description Returns the diff for a worktree against its source branch, including all staged/unstaged changes
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} WorktreeDiffResponse
// @Router /v1/git/worktrees/{id}/diff [get]
func (h *GitHandler) GetWorktreeDiff(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	diff, err := h.gitService.GetWorktreeDiff(worktreeID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(diff)
}

// CreatePullRequestRequest represents a request to create a pull request
type CreatePullRequestRequest struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	ForcePush bool   `json:"force_push,omitempty"`
}

// CreatePullRequest creates a pull request for a worktree
// @Summary Create pull request
// @Description Creates a pull request for a worktree branch
// @Tags git
// @Accept json
// @Produce json
// @Param id path string true "Worktree ID"
// @Param request body CreatePullRequestRequest true "Pull request details"
// @Success 200 {object} models.PullRequestResponse
// @Router /v1/git/worktrees/{id}/pr [post]
func (h *GitHandler) CreatePullRequest(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	var req CreatePullRequestRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	pr, err := h.gitService.CreatePullRequest(worktreeID, req.Title, req.Body, req.ForcePush)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(pr)
}

// UpdatePullRequest updates an existing pull request for a worktree
// @Summary Update pull request
// @Description Updates an existing pull request for a worktree branch
// @Tags git
// @Accept json
// @Produce json
// @Param id path string true "Worktree ID"
// @Param request body CreatePullRequestRequest true "Pull request details"
// @Success 200 {object} models.PullRequestResponse
// @Router /v1/git/worktrees/{id}/pr [put]
func (h *GitHandler) UpdatePullRequest(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	var req CreatePullRequestRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	pr, err := h.gitService.UpdatePullRequest(worktreeID, req.Title, req.Body, req.ForcePush)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(pr)
}

// GetPullRequestInfo gets information about an existing pull request for a worktree
// @Summary Get pull request info
// @Description Gets information about an existing pull request for a worktree branch
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} models.PullRequestInfo
// @Router /v1/git/worktrees/{id}/pr [get]
func (h *GitHandler) GetPullRequestInfo(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	prInfo, err := h.gitService.GetPullRequestInfo(worktreeID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(prInfo)
}

// GraduateBranchRequest represents the request to graduate a branch
type GraduateBranchRequest struct {
	// Optional custom branch name to graduate to
	BranchName string `json:"branch_name,omitempty" example:"feature/add-auth"`
}

// GraduateBranch manually triggers renaming of a branch to a semantic name
// @Summary Rename branch
// @Description Triggers renaming of any branch to a semantic name using Claude or a custom name
// @Tags git
// @Accept json
// @Produce json
// @Param id path string true "Worktree ID"
// @Param request body GraduateBranchRequest false "Graduation request with optional custom branch name"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string "Bad request (invalid branch name, branch already exists, etc.)"
// @Failure 404 {object} map[string]string "Worktree not found"
// @Failure 422 {object} map[string]string "No title available for automatic naming"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/git/worktrees/{id}/graduate [post]
func (h *GitHandler) GraduateBranch(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	// Parse request body (optional)
	var req GraduateBranchRequest
	_ = c.BodyParser(&req) // Don't fail if body is empty

	// Find the worktree to get its path
	worktrees := h.gitService.ListWorktrees()
	var workDir string
	for _, worktree := range worktrees {
		if worktree.ID == worktreeID {
			workDir = worktree.Path
			break
		}
	}

	if workDir == "" {
		return c.Status(404).JSON(fiber.Map{
			"error": "Worktree not found",
		})
	}

	// If custom branch name is provided, handle directly
	if req.BranchName != "" {
		// Get current branch name using public ExecuteGit method
		output, err := h.gitService.ExecuteGit(workDir, "rev-parse", "--symbolic-full-name", "HEAD")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to get current branch name: " + err.Error(),
			})
		}
		currentBranch := strings.TrimSpace(string(output))

		// Validate the custom branch name using git check-ref-format
		if _, err := h.gitService.ExecuteGit("", "check-ref-format", "refs/heads/"+req.BranchName); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Invalid branch name: " + req.BranchName,
			})
		}

		// Check if the new branch already exists
		if h.gitService.BranchExists(workDir, req.BranchName, false) {
			return c.Status(400).JSON(fiber.Map{
				"error": "Branch already exists: " + req.BranchName,
			})
		}

		// Create new branch from current HEAD
		if _, err := h.gitService.ExecuteGit(workDir, "checkout", "-b", req.BranchName); err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to create new branch: " + err.Error(),
			})
		}

		// Update the worktree branch name in the GitService so the UI reflects the change
		if err := h.gitService.UpdateWorktreeBranchName(workDir, req.BranchName); err != nil {
			logger.Warnf("⚠️  Failed to update worktree branch name in service: %v", err)
			// Don't fail the whole operation for this, but log the error
		}

		// Delete the old branch reference if it was a catnip ref
		if strings.HasPrefix(currentBranch, "refs/catnip/") {
			if _, err := h.gitService.ExecuteGit(workDir, "update-ref", "-d", currentBranch); err != nil {
				// Log but don't fail - the new branch was created successfully
				// This is just cleanup of the old catnip ref
				logger.Warnf("⚠️  Failed to delete old catnip ref %q: %v", currentBranch, err)
			}
		}
	} else {
		// For automatic naming, use the Claude monitor service
		if h.claudeMonitor == nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Claude monitor service not available",
			})
		}

		// Trigger branch graduation via Claude monitor
		if err := h.claudeMonitor.TriggerBranchRename(workDir, req.BranchName); err != nil {
			// Check for specific error types to return appropriate status codes
			errMsg := err.Error()

			// No title available for automatic naming
			if strings.Contains(errMsg, "no title available") {
				return c.Status(422).JSON(fiber.Map{
					"error": errMsg,
					"code":  "NO_TITLE_AVAILABLE",
				})
			}

			// Not a catnip branch, invalid branch name, branch already exists, etc.
			return c.Status(400).JSON(fiber.Map{
				"error": errMsg,
			})
		}
	}

	response := fiber.Map{
		"message": "Branch rename triggered successfully",
	}

	if req.BranchName != "" {
		response["branch_name"] = req.BranchName
		response["method"] = "custom"
	} else {
		response["method"] = "claude_generated"
	}

	return c.JSON(response)
}

// RefreshWorktreeStatus forces a refresh of a worktree's cached status
// @Summary Force refresh worktree status
// @Description Forces an immediate refresh of a worktree's cached status including commit counts
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} map[string]string
// @Router /v1/git/worktrees/{id}/refresh [post]
func (h *GitHandler) RefreshWorktreeStatus(c *fiber.Ctx) error {
	worktreeID := c.Params("id")

	// Force refresh by calling the git service method that recalculates status
	if err := h.gitService.RefreshWorktreeStatusByID(worktreeID); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Worktree status refreshed successfully",
		"id":      worktreeID,
	})
}

// CreateTemplateRequest defines the request body for creating from template
type CreateTemplateRequest struct {
	TemplateID  string `json:"template_id" binding:"required"`
	ProjectName string `json:"project_name" binding:"required"`
}

// CreateFromTemplate creates a new workspace from a project template
// @Summary Create workspace from template
// @Description Creates a new Git repository and workspace from a predefined project template
// @Tags git
// @Accept json
// @Produce json
// @Param request body CreateTemplateRequest true "Template creation request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string "Invalid request or template not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/git/template [post]
func (h *GitHandler) CreateFromTemplate(c *fiber.Ctx) error {
	var req CreateTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body: " + err.Error(),
		})
	}

	// Validate template ID
	if req.TemplateID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "template_id is required",
		})
	}

	// Validate project name
	if req.ProjectName == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "project_name is required",
		})
	}

	// Create project from template
	repo, worktree, err := h.gitService.CreateFromTemplate(req.TemplateID, req.ProjectName)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Return success response with repository information
	response := fiber.Map{
		"success": true,
		"repo_id": repo.ID,
		"path":    repo.Path,
		"message": fmt.Sprintf("Successfully created project %s from template %s", req.ProjectName, req.TemplateID),
	}

	// Include worktree info if one was created
	if worktree != nil {
		response["worktree"] = worktree.ID
		response["worktree_path"] = worktree.Path
		response["worktree_name"] = worktree.Name
	}

	return c.JSON(response)
}

// CreateGitHubRepository creates a GitHub repository and sets it as origin for a local repo
// @Summary Create GitHub repository
// @Description Creates a new GitHub repository and sets it as the origin for a local repository
// @Tags git
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param request body CreateGitHubRepositoryRequest true "Repository creation request"
// @Success 200 {object} CreateGitHubRepositoryResponse
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/git/repositories/{id}/github [post]
func (h *GitHandler) CreateGitHubRepository(c *fiber.Ctx) error {
	repoID := c.Params("id")
	logger.Infof("🔍 CreateGitHubRepository called with repoID: '%s'", repoID)

	// URL decode the repository ID
	decodedRepoID, err := url.QueryUnescape(repoID)
	if err != nil {
		logger.Errorf("❌ Failed to URL decode repoID '%s': %v", repoID, err)
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid repository ID: " + err.Error(),
		})
	}
	repoID = decodedRepoID
	logger.Infof("🔍 Decoded repoID: '%s'", repoID)

	var req CreateGitHubRepositoryRequest
	if err := c.BodyParser(&req); err != nil {
		logger.Errorf("❌ Failed to parse request body: %v", err)
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body: " + err.Error(),
		})
	}

	logger.Infof("📝 Request parsed: name='%s', description='%s', isPrivate=%v", req.Name, req.Description, req.IsPrivate)

	// Validate request
	if req.Name == "" {
		logger.Errorf("❌ Repository name is required")
		return c.Status(400).JSON(fiber.Map{
			"error": "name is required",
		})
	}

	// Create GitHub repository and update origin
	logger.Infof("🚀 Calling CreateGitHubRepositoryAndSetOrigin with repoID: '%s'", repoID)
	repoURL, err := h.gitService.CreateGitHubRepositoryAndSetOrigin(repoID, req.Name, req.Description, req.IsPrivate)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(CreateGitHubRepositoryResponse{
		URL:     repoURL,
		Message: "Repository created and origin updated successfully",
	})
}

// DeleteRepository removes a repository and all its worktrees
// @Summary Delete repository
// @Description Removes a repository and all its associated worktrees from disk and state management
// @Tags git
// @Produce json
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string "Repository not found or deletion failed"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/git/repositories/{id} [delete]
func (h *GitHandler) DeleteRepository(c *fiber.Ctx) error {
	repoID := c.Params("id")
	logger.Infof("🗑️ DeleteRepository called with repoID: '%s'", repoID)

	// URL decode the repository ID
	decodedRepoID, err := url.QueryUnescape(repoID)
	if err != nil {
		logger.Errorf("❌ Failed to URL decode repoID '%s': %v", repoID, err)
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid repository ID: " + err.Error(),
		})
	}
	repoID = decodedRepoID
	logger.Infof("🗑️ Decoded repoID: '%s'", repoID)

	// Delete the repository
	if err := h.gitService.DeleteRepository(repoID); err != nil {
		logger.Errorf("❌ Failed to delete repository '%s': %v", repoID, err)
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("Repository %s deleted successfully", repoID),
	})
}
