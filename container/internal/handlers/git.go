package handlers

import (
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