package git

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
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

// extractGitHubRepoFromURL extracts owner/repo from a GitHub URL
func (g *GitHubManager) extractGitHubRepoFromURL(remoteURL string) string {
	// Handle various GitHub URL formats:
	// - https://github.com/owner/repo.git
	// - git@github.com:owner/repo.git
	// - https://github.com/owner/repo

	// Remove .git suffix if present
	remoteURL = strings.TrimSuffix(remoteURL, ".git")

	// Handle SSH format (git@github.com:owner/repo)
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		return strings.TrimPrefix(remoteURL, "git@github.com:")
	}

	// Handle HTTPS format (https://github.com/owner/repo)
	if strings.HasPrefix(remoteURL, "https://github.com/") {
		return strings.TrimPrefix(remoteURL, "https://github.com/")
	}

	// Not a recognized GitHub URL
	return ""
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
	ForcePush        bool
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
			logger.Warnf("⚠️ Failed to check uncommitted changes for %s: %v", req.Worktree.Name, err)
		} else if hasChanges {
			logger.Debugf("📝 Worktree %s has uncommitted changes, creating temporary commit for PR", req.Worktree.Name)
			if hash, err := req.CreateTempCommit(req.Worktree.Path); err != nil {
				logger.Warnf("⚠️ Failed to create temporary commit for PR: %v", err)
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
	ownerRepo := req.Repository.ID

	// For local repos, extract the GitHub owner/repo from the remote URL
	if strings.HasPrefix(req.Repository.ID, "local/") {
		// Get the remote URL from the worktree
		remoteURL, err := g.operations.GetRemoteURL(req.Worktree.Path)
		if err != nil {
			return nil, fmt.Errorf("cannot create PR: no remote configured for local repository")
		}

		// Extract owner/repo from URL (e.g., git@github.com:owner/repo.git -> owner/repo)
		ownerRepo = g.extractGitHubRepoFromURL(remoteURL)
		if ownerRepo == "" {
			return nil, fmt.Errorf("cannot create PR: remote URL is not a GitHub repository: %s", remoteURL)
		}

		logger.Debugf("🔄 Using GitHub repo %s for local repository %s", ownerRepo, req.Repository.ID)
	} else {
		// For non-local repos, validate format
		parts := strings.Split(req.Repository.ID, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repository ID format: %s (expected owner/repo)", req.Repository.ID)
		}
	}

	if req.IsUpdate {
		return g.updatePullRequestWithGH(req.Worktree, ownerRepo, req.Title, req.Body, req.ForcePush)
	} else {
		return g.createPullRequestWithGH(req.Worktree, ownerRepo, req.Title, req.Body, req.ForcePush)
	}
}

// GetPullRequestInfo retrieves PR information for a worktree
func (g *GitHubManager) GetPullRequestInfo(worktree *models.Worktree, repository *models.Repository) (*models.PullRequestInfo, error) {
	// For local repos, we still want to check if there are commits
	// Allow PR creation if there are commits, regardless of being behind base branch
	prInfo := &models.PullRequestInfo{
		HasCommitsAhead: worktree.CommitCount > 0, // Enable PR if there are commits in worktree
		Exists:          false,
	}

	// For local repos, we can't check for existing PRs without a remote URL
	if strings.HasPrefix(repository.ID, "local/") {
		// If there's a remote URL configured, we could check for existing PRs
		// but for now, just return whether there are commits to push
		return prInfo, nil
	}

	ownerRepo := repository.ID

	// Try to find existing PR
	if err := g.checkExistingPR(worktree, ownerRepo, prInfo); err != nil {
		logger.Warnf("ℹ️ Could not check for existing PR: %v", err)
	}

	return prInfo, nil
}

// updatePullRequestWithGH updates an existing PR using GitHub CLI
func (g *GitHubManager) updatePullRequestWithGH(worktree *models.Worktree, ownerRepo, title, body string, forcePush bool) (*models.PullRequestResponse, error) {
	logger.Debugf("🔄 Updating PR for branch %s in %s", worktree.Branch, ownerRepo)

	// Handle custom refs (e.g., refs/catnip/ninja) by using the simple branch name
	branchToPush := worktree.Branch
	if strings.HasPrefix(worktree.Branch, "refs/catnip/") {
		// Extract the simple branch name from the custom ref
		branchToPush = strings.TrimPrefix(worktree.Branch, "refs/catnip/")
	}

	// First, push the branch to ensure it's up to date
	if err := g.operations.PushBranch(worktree.Path, PushStrategy{
		Branch:       branchToPush,
		Remote:       "origin",
		SetUpstream:  true,
		ConvertHTTPS: true,
		Force:        forcePush,
	}); err != nil {
		return nil, fmt.Errorf("failed to push branch before PR update: %v", err)
	}

	// Update the PR
	cmd := g.execCommand("gh", "pr", "edit", branchToPush,
		"--repo", ownerRepo,
		"--title", title,
		"--body", body)

	_, err := cmd.Output()
	if err != nil {
		// For error reporting, capture stderr if available
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to update PR: %v\nStderr: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to update PR: %v", err)
	}

	logger.Infof("✅ Updated PR for branch %s", worktree.Branch)

	// Get the PR details
	cmd = g.execCommand("gh", "pr", "view", worktree.Branch, "--repo", ownerRepo, "--json", "number,url,title,body")
	output, err := cmd.Output()
	if err != nil {
		logger.Warnf("⚠️ Could not get PR details: %v", err)
		return &models.PullRequestResponse{
			Number: 0,
			URL:    "",
			Title:  title,
			Body:   body,
		}, nil
	}

	var result struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		logger.Warnf("⚠️ Could not parse PR details: %v", err)
		return &models.PullRequestResponse{
			Number: 0,
			URL:    "",
			Title:  title,
			Body:   body,
		}, nil
	}

	return &models.PullRequestResponse{
		Number:     result.Number,
		URL:        result.URL,
		Title:      result.Title,
		Body:       result.Body,
		HeadBranch: branchToPush,
		BaseBranch: worktree.SourceBranch,
	}, nil
}

// createPullRequestWithGH creates a new PR using GitHub CLI
func (g *GitHubManager) createPullRequestWithGH(worktree *models.Worktree, ownerRepo, title, body string, forcePush bool) (*models.PullRequestResponse, error) {
	logger.Debugf("🚀 Creating PR for branch %s in %s", worktree.Branch, ownerRepo)

	// Handle custom refs (e.g., refs/catnip/ninja) by using the nice branch for pushing
	branchToPush := worktree.Branch
	if strings.HasPrefix(worktree.Branch, "refs/catnip/") {
		// Check if there's a nice branch mapped to this custom ref
		configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(worktree.Branch, "/", "."))
		niceBranchOutput, err := g.operations.GetConfig(worktree.Path, configKey)
		if err == nil && strings.TrimSpace(niceBranchOutput) != "" {
			// Use the mapped nice branch
			branchToPush = strings.TrimSpace(niceBranchOutput)
			logger.Debugf("🔍 Using nice branch %s for PR (worktree remains on %s)", branchToPush, worktree.Branch)

			// Ensure the nice branch is up to date with the custom ref
			currentCommit, _ := g.operations.GetCommitHash(worktree.Path, "HEAD")
			if currentCommit != "" {
				_, err = g.operations.ExecuteGit(worktree.Path, "branch", "-f", branchToPush, currentCommit)
				if err != nil {
					logger.Warnf("⚠️ Failed to update nice branch to current commit: %v", err)
				}
			}
		} else {
			// Fallback: Extract the simple branch name from the custom ref and create a branch
			simpleBranchName := strings.TrimPrefix(worktree.Branch, "refs/catnip/")
			logger.Debugf("🔄 Creating fallback branch %s from custom ref %s", simpleBranchName, worktree.Branch)

			// Create the branch WITHOUT switching to it (worktree stays on custom ref)
			currentCommit, _ := g.operations.GetCommitHash(worktree.Path, "HEAD")
			if currentCommit != "" {
				if !g.operations.BranchExists(worktree.Path, simpleBranchName, false) {
					err := g.operations.CreateBranch(worktree.Path, simpleBranchName, currentCommit)
					if err != nil {
						return nil, fmt.Errorf("failed to create branch from custom ref: %v", err)
					}
					logger.Debugf("✅ Created branch %s (worktree remains on %s)", simpleBranchName, worktree.Branch)
				} else {
					// Update existing branch to current commit
					_, err = g.operations.ExecuteGit(worktree.Path, "branch", "-f", simpleBranchName, currentCommit)
					if err != nil {
						logger.Warnf("⚠️ Failed to update branch to current commit: %v", err)
					}
				}
			}
			branchToPush = simpleBranchName
		}
	}

	// Push the branch
	logger.Debugf("🔍 PR Creation: About to push branch %s with ConvertHTTPS=true, Force=%v", branchToPush, forcePush)
	if err := g.operations.PushBranch(worktree.Path, PushStrategy{
		Branch:       branchToPush,
		Remote:       "origin",
		SetUpstream:  true,
		ConvertHTTPS: true,
		Force:        forcePush,
	}); err != nil {
		logger.Errorf("❌ PR Creation: Push failed: %v", err)
		return nil, fmt.Errorf("failed to push branch before PR creation: %v", err)
	}
	logger.Debugf("✅ PR Creation: Push successful for branch %s", branchToPush)

	// Create the PR
	logger.Debugf("🔍 PR Creation: About to create PR with gh pr create --repo %s", ownerRepo)
	cmd := g.execCommand("gh", "pr", "create",
		"--repo", ownerRepo,
		"--base", worktree.SourceBranch,
		"--head", branchToPush,
		"--title", title,
		"--body", body)

	output, err := cmd.Output()
	if err != nil {
		// For error checking, we need to capture stderr separately
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			// Check if it's because PR already exists
			if strings.Contains(stderr, "already exists") {
				return nil, fmt.Errorf("PR_ALREADY_EXISTS: A pull request for this branch already exists")
			}
			return nil, fmt.Errorf("failed to create PR: %v\nStderr: %s", err, stderr)
		}
		return nil, fmt.Errorf("failed to create PR: %v", err)
	}

	logger.Infof("✅ Created PR for branch %s", branchToPush)

	// Extract URL from output (gh pr create returns the URL)
	// Split by newlines and take the last non-empty line to handle mixed output
	outputStr := strings.TrimSpace(string(output))
	lines := strings.Split(outputStr, "\n")

	var url string
	// Find the last line that looks like a GitHub PR URL
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "https://github.com/") && strings.Contains(line, "/pull/") {
			url = line
			break
		}
	}

	if url == "" {
		return nil, fmt.Errorf("failed to extract valid GitHub PR URL from output: %s", outputStr)
	}

	// Extract PR number from URL (e.g., https://github.com/owner/repo/pull/123)
	prNumber := 0
	if strings.Contains(url, "/pull/") {
		parts := strings.Split(url, "/pull/")
		if len(parts) == 2 {
			if num, err := strconv.Atoi(parts[1]); err == nil {
				prNumber = num
			}
		}
	}

	return &models.PullRequestResponse{
		Number:     prNumber,
		URL:        url,
		Title:      title,
		Body:       body,
		HeadBranch: branchToPush,
		BaseBranch: worktree.SourceBranch,
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

	logger.Debugf("✅ Found existing PR #%d for branch %s", existingPR.Number, worktree.Branch)
	return nil
}

// IsAuthenticated checks if GitHub CLI is authenticated
func (g *GitHubManager) IsAuthenticated() bool {
	cmd := g.execCommand("gh", "auth", "status")
	return cmd.Run() == nil
}

// ConfigureGitCredentials sets up Git to use gh CLI for GitHub authentication
func (g *GitHubManager) ConfigureGitCredentials() error {
	if config.Runtime.IsNative() {
		logger.Debugf("ℹ️ Running in native mode - skipping git credential configuration")
		return nil
	}

	if !g.IsAuthenticated() {
		logger.Warnf("ℹ️ GitHub CLI not authenticated, Git operations will only work with public repositories")
		return fmt.Errorf("GitHub CLI not authenticated")
	}

	logger.Debugf("🔐 Configuring Git to use GitHub CLI for authentication")

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

// CreateRepository creates a new GitHub repository
func (g *GitHubManager) CreateRepository(name, description string, isPrivate bool) (string, error) {
	args := []string{"repo", "create", name, "--description", description}

	if isPrivate {
		args = append(args, "--private")
	} else {
		args = append(args, "--public")
	}

	cmd := g.execCommand("gh", args...)
	output, err := cmd.Output()
	if err != nil {
		// For error reporting, capture stderr if available
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to create GitHub repository: %v\nStderr: %s", err, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to create GitHub repository: %v", err)
	}

	// Extract repository URL from output
	outputStr := strings.TrimSpace(string(output))
	lines := strings.Split(outputStr, "\n")

	// Find the line that contains the repository URL
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://github.com/") {
			return line, nil
		}
	}

	// If we can't find the URL in output, construct it based on the authenticated user
	userCmd := g.execCommand("gh", "api", "user", "--jq", ".login")
	userOutput, err := userCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get authenticated user: %v", err)
	}

	username := strings.TrimSpace(string(userOutput))
	return fmt.Sprintf("https://github.com/%s/%s", username, name), nil
}
