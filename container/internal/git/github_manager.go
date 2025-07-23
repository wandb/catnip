package git

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/vanpelt/catnip/internal/models"
)

// GitHubManager handles all GitHub CLI operations (auth, repos, pull requests, etc.)
// nolint:revive
type GitHubManager struct {
	operations Operations
}

// NewGitHubManager creates a new GitHub manager
func NewGitHubManager(operations Operations) *GitHubManager {
	return &GitHubManager{
		operations: operations,
	}
}

// execCommand creates a command with proper environment
func (g *GitHubManager) execCommand(command string, args ...string) *exec.Cmd {
	cmd := exec.Command(command, args...)
	return cmd
}

// CreatePullRequestRequest contains parameters for PR creation
type CreatePullRequestRequest struct {
	Worktree         *models.Worktree
	Repository       *models.Repository
	Title            string
	Body             string
	IsUpdate         bool
	FetchFullHistory func(*models.Worktree)
	CreateTempCommit func(string) (string, error)
	RevertTempCommit func(string, string)
}

// CreatePullRequest creates or updates a GitHub pull request
func (g *GitHubManager) CreatePullRequest(req CreatePullRequestRequest) (*models.PullRequestResponse, error) {
	// Ensure we have full git history for accurate commit tracking
	req.FetchFullHistory(req.Worktree)

	// For GitHub repos, check uncommitted changes and create temporary commit if needed
	var tempCommitHash string
	if !strings.HasPrefix(req.Repository.ID, "local/") {
		if hasChanges, err := g.operations.HasUncommittedChanges(req.Worktree.Path); err != nil {
			log.Printf("⚠️ Failed to check uncommitted changes for %s: %v", req.Worktree.Name, err)
		} else if hasChanges {
			log.Printf("📝 Worktree %s has uncommitted changes, creating temporary commit for PR", req.Worktree.Name)
			if hash, err := req.CreateTempCommit(req.Worktree.Path); err != nil {
				log.Printf("⚠️ Failed to create temporary commit for PR: %v", err)
			} else {
				tempCommitHash = hash
			}
		}
	}

	// Cleanup temp commit when done
	defer func() {
		if tempCommitHash != "" {
			req.RevertTempCommit(req.Worktree.Path, tempCommitHash)
		}
	}()

	// Parse owner/repo from repository ID
	parts := strings.Split(req.Repository.ID, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository ID format: %s (expected owner/repo)", req.Repository.ID)
	}
	ownerRepo := req.Repository.ID

	if req.IsUpdate {
		return g.updatePullRequestWithGH(req.Worktree, ownerRepo, req.Title, req.Body)
	} else {
		return g.createPullRequestWithGH(req.Worktree, ownerRepo, req.Title, req.Body)
	}
}

// GetPullRequestInfo retrieves PR information for a worktree
func (g *GitHubManager) GetPullRequestInfo(worktree *models.Worktree, repository *models.Repository) (*models.PullRequestInfo, error) {
	// For local repos, return empty info
	if strings.HasPrefix(repository.ID, "local/") {
		return &models.PullRequestInfo{
			HasCommitsAhead: false,
			Exists:          false,
		}, nil
	}

	ownerRepo := repository.ID

	// Check for existing PR first
	prInfo := &models.PullRequestInfo{
		HasCommitsAhead: true, // Will be updated by caller if needed
		Exists:          false,
	}

	// Try to find existing PR
	if err := g.checkExistingPR(worktree, ownerRepo, prInfo); err != nil {
		log.Printf("ℹ️ Could not check for existing PR: %v", err)
	}

	return prInfo, nil
}

// updatePullRequestWithGH updates an existing PR using GitHub CLI
func (g *GitHubManager) updatePullRequestWithGH(worktree *models.Worktree, ownerRepo, title, body string) (*models.PullRequestResponse, error) {
	log.Printf("🔄 Updating PR for branch %s in %s", worktree.Branch, ownerRepo)

	// First, push the branch to ensure it's up to date
	if err := g.operations.PushBranch(worktree.Path, PushStrategy{
		Branch:      worktree.Branch,
		Remote:      "origin",
		SetUpstream: true,
	}); err != nil {
		return nil, fmt.Errorf("failed to push branch before PR update: %v", err)
	}

	// Update the PR
	cmd := g.execCommand("gh", "pr", "edit", worktree.Branch,
		"--repo", ownerRepo,
		"--title", title,
		"--body", body)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to update PR: %v\nOutput: %s", err, string(output))
	}

	log.Printf("✅ Updated PR for branch %s", worktree.Branch)

	// Get the PR URL
	cmd = g.execCommand("gh", "pr", "view", worktree.Branch, "--repo", ownerRepo, "--json", "url")
	output, err = cmd.Output()
	if err != nil {
		log.Printf("⚠️ Could not get PR URL: %v", err)
		return &models.PullRequestResponse{
			URL:   "",
			Title: title,
			Body:  body,
		}, nil
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		log.Printf("⚠️ Could not parse PR URL: %v", err)
		return &models.PullRequestResponse{
			URL:   "",
			Title: title,
			Body:  body,
		}, nil
	}

	return &models.PullRequestResponse{
		URL:   result.URL,
		Title: title,
		Body:  body,
	}, nil
}

// createPullRequestWithGH creates a new PR using GitHub CLI
func (g *GitHubManager) createPullRequestWithGH(worktree *models.Worktree, ownerRepo, title, body string) (*models.PullRequestResponse, error) {
	log.Printf("🚀 Creating PR for branch %s in %s", worktree.Branch, ownerRepo)

	// First, push the branch
	if err := g.operations.PushBranch(worktree.Path, PushStrategy{
		Branch:      worktree.Branch,
		Remote:      "origin",
		SetUpstream: true,
	}); err != nil {
		return nil, fmt.Errorf("failed to push branch before PR creation: %v", err)
	}

	// Create the PR
	cmd := g.execCommand("gh", "pr", "create",
		"--repo", ownerRepo,
		"--base", worktree.SourceBranch,
		"--head", worktree.Branch,
		"--title", title,
		"--body", body)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's because PR already exists
		if strings.Contains(string(output), "already exists") {
			return nil, fmt.Errorf("PR_ALREADY_EXISTS: A pull request for this branch already exists")
		}
		return nil, fmt.Errorf("failed to create PR: %v\nOutput: %s", err, string(output))
	}

	log.Printf("✅ Created PR for branch %s", worktree.Branch)

	// Extract URL from output (gh pr create returns the URL)
	url := strings.TrimSpace(string(output))

	return &models.PullRequestResponse{
		URL:   url,
		Title: title,
		Body:  body,
	}, nil
}

// checkExistingPR checks if a PR already exists for the branch
func (g *GitHubManager) checkExistingPR(worktree *models.Worktree, ownerRepo string, prInfo *models.PullRequestInfo) error {
	// Use GitHub CLI to check for existing PR
	cmd := g.execCommand("gh", "pr", "view", worktree.Branch, "--repo", ownerRepo, "--json", "number,url,title,body")

	output, err := cmd.Output()
	if err != nil {
		// If no PR exists, that's fine
		if strings.Contains(err.Error(), "no pull requests found") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to check for existing PR: %v", err)
	}

	// Parse the existing PR info
	var existingPR struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}

	if err := json.Unmarshal(output, &existingPR); err != nil {
		return fmt.Errorf("failed to parse existing PR info: %v", err)
	}

	// Update PR info with existing data
	prInfo.Exists = true
	prInfo.Number = existingPR.Number
	prInfo.URL = existingPR.URL
	prInfo.Title = existingPR.Title
	prInfo.Body = existingPR.Body

	log.Printf("✅ Found existing PR #%d for branch %s", existingPR.Number, worktree.Branch)
	return nil
}

// IsAuthenticated checks if GitHub CLI is authenticated
func (g *GitHubManager) IsAuthenticated() bool {
	cmd := g.execCommand("gh", "auth", "status")
	return cmd.Run() == nil
}

// ConfigureGitCredentials sets up Git to use gh CLI for GitHub authentication
func (g *GitHubManager) ConfigureGitCredentials() error {
	if !g.IsAuthenticated() {
		log.Printf("ℹ️ GitHub CLI not authenticated, Git operations will only work with public repositories")
		return fmt.Errorf("GitHub CLI not authenticated")
	}

	log.Printf("🔐 Configuring Git to use GitHub CLI for authentication")

	// Configure Git to use gh as credential helper for GitHub
	return g.operations.SetGlobalConfig("credential.https://github.com.helper", "!gh auth git-credential")
}

// GitHubRepository represents a GitHub repository from the API
// nolint:revive
type GitHubRepository struct {
	Name        string                 `json:"name"`
	URL         string                 `json:"url"`
	IsPrivate   bool                   `json:"isPrivate"`
	Description string                 `json:"description"`
	Owner       map[string]interface{} `json:"owner"`
}

// ListRepositories lists GitHub repositories accessible to the authenticated user
func (g *GitHubManager) ListRepositories() ([]GitHubRepository, error) {
	cmd := g.execCommand("gh", "repo", "list", "--limit", "100", "--json", "name,url,isPrivate,description,owner")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list GitHub repositories: %w", err)
	}

	var repos []GitHubRepository
	if err := json.Unmarshal(output, &repos); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub repositories: %w", err)
	}

	return repos, nil
}
