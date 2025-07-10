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
}

// NewGitHandler creates a new Git handler
func NewGitHandler(gitService *services.GitService, gitHTTPService *services.GitHTTPService) *GitHandler {
	return &GitHandler{
		gitService:     gitService,
		gitHTTPService: gitHTTPService,
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
// @Success 200 {object} map[string]interface{}
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


// ListWorktrees returns all worktrees
// @Summary List all worktrees
// @Description Returns a list of all worktrees for the current repository
// @Tags git
// @Produce json
// @Success 200 {array} models.Worktree
// @Router /v1/git/worktrees [get]
func (h *GitHandler) ListWorktrees(c *fiber.Ctx) error {
	worktrees := h.gitService.ListWorktrees()
	return c.JSON(worktrees)
}


// ListGitHubRepositories returns user's GitHub repositories
// @Summary List GitHub repositories
// @Description Returns a list of GitHub repositories accessible to the authenticated user
// @Tags git
// @Produce json
// @Success 200 {array} map[string]interface{}
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
// @Success 200 {object} map[string]string
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
// @Success 200 {object} map[string]string
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
		"message": "Worktree synced successfully",
		"id":      worktreeID,
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
// @Success 200 {object} map[string]string
// @Router /v1/git/worktrees/{id}/merge [post]
func (h *GitHandler) MergeWorktreeToMain(c *fiber.Ctx) error {
	worktreeID := c.Params("id")
	
	var mergeRequest struct {
		Squash bool `json:"squash"`
	}
	
	// Parse body if present, but don't require it for backwards compatibility
	c.BodyParser(&mergeRequest)
	
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
	
	return c.JSON(fiber.Map{
		"message": "Worktree merged to main successfully",
		"id":      worktreeID,
	})
}

// CreateWorktreePreview creates a preview branch for viewing changes outside container
// @Summary Create worktree preview
// @Description Creates a preview branch in the main repo for viewing changes outside container
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} map[string]string
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
// @Success 200 {object} map[string]interface{}
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
// @Success 200 {object} map[string]interface{}
// @Router /v1/git/worktrees/{id}/merge/check [get]
func (h *GitHandler) CheckMergeConflicts(c *fiber.Ctx) error {
	worktreeID := c.Params("id")
	
	conflictErr, err := h.gitService.CheckMergeConflicts(worktreeID)
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
		"message":       "No conflicts detected for merge operation",
	})
}

// GetWorktreeDiff returns the diff for a worktree against its source branch
// @Summary Get worktree diff
// @Description Returns the diff for a worktree against its source branch, including all staged/unstaged changes
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} map[string]interface{}
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

