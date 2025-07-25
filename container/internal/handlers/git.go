package handlers

import (
	"errors"
	"log"
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// GitHandler handles Git-related API endpoints
type GitHandler struct {
	gitService     *services.GitService
	gitHTTPService *services.GitHTTPService
	sessionService *services.SessionService
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
func NewGitHandler(gitService *services.GitService, gitHTTPService *services.GitHTTPService, sessionService *services.SessionService) *GitHandler {
	return &GitHandler{
		gitService:     gitService,
		gitHTTPService: gitHTTPService,
		sessionService: sessionService,
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

	log.Printf("📦 Checkout request: %s/%s (branch: %s)", org, repo, branch)

	repository, worktree, err := h.gitService.CheckoutRepository(org, repo, branch)
	if err != nil {
		log.Printf("❌ Checkout failed: %v", err)
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

		// Check if there's an existing Claude session that would trigger --continue behavior
		// This matches the same logic used in pty.go for determining whether to use --continue
		if existingState, err := h.sessionService.FindSessionByDirectory(worktree.Path); err == nil && existingState != nil {
			worktree.HasActiveClaudeSession = true
		}

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

	if err := h.gitService.DeleteWorktree(worktreeID); err != nil {
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
		if cleanupErr := h.gitService.DeleteWorktree(worktreeID); cleanupErr != nil {
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
		log.Printf("❌ CheckMergeConflicts failed for worktree %s: %v", worktreeID, err)
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
