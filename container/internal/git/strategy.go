package git

import (
	"fmt"

	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/git/executor"
	"github.com/vanpelt/catnip/internal/logger"
)

// FetchStrategy defines the strategy for fetching branches
type FetchStrategy struct {
	Branch         string // Branch to fetch
	Remote         string // Remote name or path
	RemoteName     string // Remote name for refs (defaults to remote name)
	IsLocalRepo    bool   // Whether this is a local repo fetch
	Depth          int    // Fetch depth (0 = no depth limit)
	UpdateLocalRef bool   // Whether to update local refs after fetch
	RefSpec        string // Custom refspec (optional)
}

// PushStrategy defines the strategy for pushing branches
type PushStrategy struct {
	Branch       string // Branch to push (defaults to worktree.Branch)
	Remote       string // Remote name (defaults to "origin")
	RemoteURL    string // Remote URL (optional, for local repos)
	SyncOnFail   bool   // Whether to sync with upstream on push failure
	SetUpstream  bool   // Whether to set upstream (-u flag)
	ConvertHTTPS bool   // Whether to convert SSH URLs to HTTPS (includes workflow detection)
	Force        bool   // Whether to force push (--force-with-lease)
}

// FetchExecutor handles fetch operations with strategy pattern
type FetchExecutor struct {
	executor executor.CommandExecutor
}

// NewFetchExecutor creates a new fetch executor
func NewFetchExecutor(executor executor.CommandExecutor) *FetchExecutor {
	return &FetchExecutor{executor: executor}
}

// FetchBranch executes a fetch strategy
func (f *FetchExecutor) FetchBranch(repoPath string, strategy FetchStrategy) error {
	// Set defaults
	if strategy.Remote == "" {
		strategy.Remote = "origin"
	}
	if strategy.RemoteName == "" {
		strategy.RemoteName = strategy.Remote
	}

	// Skip fetch for local repos if no remote specified
	if strategy.IsLocalRepo && strategy.Remote == "origin" {
		return nil
	}

	// Build fetch command
	args := []string{"fetch"}

	// Add remote
	args = append(args, strategy.Remote)

	// Add refspec
	if strategy.RefSpec != "" {
		args = append(args, strategy.RefSpec)
	} else if strategy.Branch != "" {
		if strategy.IsLocalRepo {
			// For local repos, use custom refspec format
			args = append(args, fmt.Sprintf("%s:refs/remotes/%s/%s", strategy.Branch, strategy.RemoteName, strategy.Branch))
		} else {
			// For remote repos, use standard refspec
			args = append(args, fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", strategy.Branch, strategy.RemoteName, strategy.Branch))
		}
	}

	// Add depth if specified
	if strategy.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", strategy.Depth))
	}

	// Execute fetch
	output, err := f.executor.ExecuteGitWithWorkingDir(repoPath, args...)
	if err != nil {
		return fmt.Errorf("failed to fetch branch: %v\n%s", err, output)
	}

	// Update local branch ref if requested
	if strategy.UpdateLocalRef && strategy.Branch != "" && !strategy.IsLocalRepo {
		_, err = f.executor.ExecuteGitWithWorkingDir(repoPath, "update-ref",
			fmt.Sprintf("refs/heads/%s", strategy.Branch),
			fmt.Sprintf("refs/remotes/%s/%s", strategy.RemoteName, strategy.Branch))
		if err != nil {
			logger.Debug("⚠️ Could not update local branch ref: %v", err)
		}
	}

	return nil
}

// FetchBranchFast performs a highly optimized fetch for status updates
func (f *FetchExecutor) FetchBranchFast(repoPath, branch string) error {
	strategy := FetchStrategy{
		Branch:     branch,
		Remote:     "origin",
		RemoteName: "origin",
		Depth:      1,
		RefSpec:    fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch),
	}

	// Add optimization flags
	args := []string{
		"fetch",
		strategy.Remote,
		strategy.RefSpec,
		"--depth", "1",
		"--no-tags",               // Skip tags to reduce transfer
		"--quiet",                 // Reduce output noise
		"--no-recurse-submodules", // Skip submodules
	}

	output, err := f.executor.ExecuteGitWithWorkingDir(repoPath, args...)
	if err != nil {
		return fmt.Errorf("failed to fetch branch optimized: %v\n%s", err, output)
	}

	return nil
}

// FetchBranchFull performs a full fetch for operations that need complete history
func (f *FetchExecutor) FetchBranchFull(repoPath, branch string) error {
	args := []string{
		"fetch",
		"origin",
		fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch),
		"--quiet", // Reduce output noise
	}

	output, err := f.executor.ExecuteGitWithWorkingDir(repoPath, args...)
	if err != nil {
		return fmt.Errorf("failed to fetch branch full: %v\n%s", err, output)
	}

	return nil
}

// PushExecutor handles push operations with strategy pattern
type PushExecutor struct {
	executor   executor.CommandExecutor
	urlManager *URLManager
}

// NewPushExecutor creates a new push executor
func NewPushExecutor(executor executor.CommandExecutor) *PushExecutor {
	return &PushExecutor{
		executor:   executor,
		urlManager: NewURLManager(executor),
	}
}

// PushBranch executes a push strategy
func (p *PushExecutor) PushBranch(worktreePath string, strategy PushStrategy) error {
	// Set defaults
	if strategy.Remote == "" {
		strategy.Remote = "origin"
	}

	// Build push command
	args := []string{"push"}
	if strategy.SetUpstream {
		args = append(args, "-u")
	}
	if strategy.Force {
		args = append(args, "--force-with-lease")
	}
	args = append(args, strategy.Remote, strategy.Branch)

	// Execute push with URL rewriting if HTTPS is needed (safer than modifying .git/config)
	// Only apply URL rewriting in containerized mode to avoid interfering with native git config
	var output []byte
	var err error
	if strategy.ConvertHTTPS && config.Runtime.IsContainerized() {
		// Use git config URL rewriting - works for SSH (converts) and HTTPS (no-op)
		// This avoids OAuth scope issues and doesn't modify .git/config
		gitArgs := append([]string{"-c", "url.https://github.com/.insteadOf=git@github.com:"}, args...)
		logger.Debug("🔄 Executing git push with URL rewriting: %v", gitArgs)
		output, err = p.executor.ExecuteGitWithWorkingDir(worktreePath, gitArgs...)
	} else {
		// Normal push execution (native mode or no HTTPS conversion needed)
		if strategy.ConvertHTTPS && config.Runtime.IsNative() {
			logger.Debug("🔄 Native mode: skipping URL rewriting, using existing git configuration")
		}
		logger.Debug("🔄 Executing git push without URL rewriting: %v", args)
		output, err = p.executor.ExecuteGitWithWorkingDir(worktreePath, args...)
	}
	if err != nil {
		// Handle push rejection with sync retry if configured
		if strategy.SyncOnFail && IsPushRejected(err, string(output)) {
			logger.Debug("🔄 Push rejected due to upstream changes, sync would be needed")
			// Note: Actual sync logic would need to be implemented by caller
			// as it requires access to worktree and sync operations
		}
		return fmt.Errorf("failed to push branch %s to %s: %v\n%s", strategy.Branch, strategy.Remote, err, output)
	}

	logger.Debug("✅ Pushed branch %s to %s", strategy.Branch, strategy.Remote)
	return nil
}
