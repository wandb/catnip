package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/git/executor"
	"github.com/vanpelt/catnip/internal/logger"
)

// OperationsImpl implements the Operations interface using gogit where possible
type OperationsImpl struct {
	executor      executor.CommandExecutor
	branchOps     *BranchOperations
	fetchExecutor *FetchExecutor
	pushExecutor  *PushExecutor
	statusChecker *StatusChecker
	urlManager    *URLManager
}

// NewOperations creates a new Operations implementation using gogit by default
func NewOperations() Operations {
	exec := executor.NewGitExecutor() // Use gogit by default
	return &OperationsImpl{
		executor:      exec,
		branchOps:     NewBranchOperations(exec),
		fetchExecutor: NewFetchExecutor(exec),
		pushExecutor:  NewPushExecutor(exec),
		statusChecker: NewStatusChecker(exec),
		urlManager:    NewURLManager(exec),
	}
}

// NewOperationsWithExecutor creates Operations with a specific executor (for testing)
func NewOperationsWithExecutor(exec executor.CommandExecutor) Operations {
	return &OperationsImpl{
		executor:      exec,
		branchOps:     NewBranchOperations(exec),
		fetchExecutor: NewFetchExecutor(exec),
		pushExecutor:  NewPushExecutor(exec),
		statusChecker: NewStatusChecker(exec),
		urlManager:    NewURLManager(exec),
	}
}

// Core command execution

func (o *OperationsImpl) ExecuteGit(workingDir string, args ...string) ([]byte, error) {
	return o.executor.ExecuteGitWithWorkingDir(workingDir, args...)
}

func (o *OperationsImpl) ExecuteCommand(command string, args ...string) ([]byte, error) {
	return o.executor.ExecuteCommand(command, args...)
}

// Branch operations

func (o *OperationsImpl) BranchExists(repoPath, branch string, isRemote bool) bool {
	return o.branchOps.BranchExists(repoPath, branch, BranchExistsOptions{IsRemote: isRemote})
}

func (o *OperationsImpl) GetCommitCount(repoPath, fromRef, toRef string) (int, error) {
	return o.branchOps.GetCommitCount(repoPath, fromRef, toRef)
}

func (o *OperationsImpl) GetRemoteURL(repoPath string) (string, error) {
	return o.branchOps.GetRemoteURL(repoPath)
}

func (o *OperationsImpl) GetDefaultBranch(repoPath string) (string, error) {
	return o.branchOps.GetDefaultBranch(repoPath)
}

func (o *OperationsImpl) GetLocalBranches(repoPath string) ([]string, error) {
	return o.branchOps.GetLocalRepoBranches(repoPath)
}

func (o *OperationsImpl) GetRemoteBranches(repoPath string, defaultBranch string) ([]string, error) {
	return o.branchOps.GetRemoteBranches(repoPath, defaultBranch)
}

func (o *OperationsImpl) GetRemoteBranchesFromURL(remoteURL string) ([]string, error) {
	// Use git ls-remote to fetch branches from remote URL without cloning
	output, err := o.ExecuteGit("", "ls-remote", "--heads", remoteURL)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote branches from %s: %v", remoteURL, err)
	}

	var branches []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Each line is in format: <commit-hash> refs/heads/<branch-name>
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "refs/heads/") {
			branchName := strings.TrimPrefix(parts[1], "refs/heads/")
			branches = append(branches, branchName)
		}
	}

	return branches, nil
}

func (o *OperationsImpl) CreateBranch(repoPath, branch, fromRef string) error {
	// Determine the commit to create the branch from
	var commitHash string
	var err error

	if fromRef != "" {
		// If fromRef is already a commit hash, use it directly
		// If it's a reference, resolve it to a commit hash
		commitHash, err = o.GetCommitHash(repoPath, fromRef)
		if err != nil {
			return fmt.Errorf("failed to resolve fromRef %q to commit hash: %v", fromRef, err)
		}
	} else {
		// Resolve HEAD to a commit hash
		commitHash, err = o.GetCommitHash(repoPath, "HEAD")
		if err != nil {
			return fmt.Errorf("failed to resolve HEAD to commit hash: %v", err)
		}
	}

	// Use update-ref to create the branch reference directly
	// This bypasses git branch's safety checks about HEAD being under refs/heads
	branchRef := "refs/heads/" + branch
	_, err = o.ExecuteGit(repoPath, "update-ref", branchRef, commitHash)
	if err != nil {
		return fmt.Errorf("failed to create branch %q: %v", branch, err)
	}

	return nil
}

func (o *OperationsImpl) DeleteBranch(repoPath, branch string, force bool) error {
	// Check if this is a catnip ref (refs/catnip/...)
	if strings.HasPrefix(branch, "refs/catnip/") {
		// For catnip refs, use update-ref to delete the ref directly
		_, err := o.ExecuteGit(repoPath, "update-ref", "-d", branch)
		return err
	} else {
		// For regular branches, use the original logic
		args := []string{"branch"}
		if force {
			args = append(args, "-D")
		} else {
			args = append(args, "-d")
		}
		args = append(args, branch)
		_, err := o.ExecuteGit(repoPath, args...)
		return err
	}
}

func (o *OperationsImpl) RenameBranch(repoPath, oldBranch, newBranch string) error {
	// Use git branch -m to rename the branch
	_, err := o.ExecuteGit(repoPath, "branch", "-m", oldBranch, newBranch)
	return err
}

func (o *OperationsImpl) ListBranches(repoPath string, options ListBranchesOptions) ([]string, error) {
	args := []string{"branch"}
	if options.All {
		args = append(args, "-a")
	} else if options.Remote {
		args = append(args, "-r")
	}
	if options.Merged != "" {
		args = append(args, "--merged", options.Merged)
	}

	output, err := o.ExecuteGit(repoPath, args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var branches []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimPrefix(line, "+")
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// Worktree operations

func (o *OperationsImpl) CreateWorktree(repoPath, worktreePath, branch, fromRef string) error {
	// Check if this is a catnip ref (refs/catnip/...)
	if strings.HasPrefix(branch, "refs/catnip/") {
		// For catnip refs, we need to create the ref manually then create the worktree
		// First create the worktree without a branch (detached HEAD)
		args := []string{"worktree", "add", "--detach", worktreePath}
		if fromRef != "" {
			args = append(args, fromRef)
		}
		_, err := o.ExecuteGit(repoPath, args...)
		if err != nil {
			// If it fails due to missing worktree, try to resolve the specific conflict
			if strings.Contains(err.Error(), "missing but already registered worktree") {
				logger.Debug("⚠️  Worktree registration conflict detected. Attempting targeted cleanup.")

				// Try to remove the specific conflicting worktree registration
				if cleanupErr := o.cleanupOrphanedWorktreeRegistration(repoPath, worktreePath); cleanupErr != nil {
					logger.Debugf("❌ Failed to cleanup orphaned worktree registration: %v", cleanupErr)
					// Fall back to suggesting manual prune
					logger.Debugf("⚠️  To fix manually: run 'git worktree prune' in %s after ensuring all workspaces are backed up", repoPath)
					return err
				}

				// Retry the worktree creation after cleanup
				logger.Debug("🔄 Retrying worktree creation after cleanup...")
				_, retryErr := o.ExecuteGit(repoPath, args...)
				if retryErr != nil {
					return fmt.Errorf("worktree creation failed even after cleanup: %v", retryErr)
				}
			} else {
				return err
			}
		}

		// Then create the branch ref and check it out in the worktree
		// First, we need to get the commit hash for the ref we want to base on
		commitHash := fromRef
		if fromRef == "" {
			commitHash = "HEAD"
		}

		// Create the ref in the main repo pointing to the correct commit
		_, err = o.ExecuteGit(repoPath, "update-ref", branch, commitHash)
		if err != nil {
			// Cleanup the worktree if ref creation fails
			_ = o.RemoveWorktree(repoPath, worktreePath, true)
			return err
		}

		// Now we need to update the worktree to use our custom ref
		// We use symbolic-ref to set the HEAD to our custom ref
		_, err = o.ExecuteGit(worktreePath, "symbolic-ref", "HEAD", branch)
		return err
	} else {
		// For regular branches, use the original logic
		args := []string{"worktree", "add", "-b", branch, worktreePath}
		if fromRef != "" {
			args = append(args, fromRef)
		}
		_, err := o.ExecuteGit(repoPath, args...)
		if err != nil {
			// If it fails due to missing worktree, try to resolve the specific conflict
			if strings.Contains(err.Error(), "missing but already registered worktree") {
				logger.Debug("⚠️  Worktree registration conflict detected. Attempting targeted cleanup.")

				// Try to remove the specific conflicting worktree registration
				if cleanupErr := o.cleanupOrphanedWorktreeRegistration(repoPath, worktreePath); cleanupErr != nil {
					logger.Debugf("❌ Failed to cleanup orphaned worktree registration: %v", cleanupErr)
					// Fall back to suggesting manual prune
					logger.Debugf("⚠️  To fix manually: run 'git worktree prune' in %s after ensuring all workspaces are backed up", repoPath)
					return err
				}

				// Retry the worktree creation after cleanup
				logger.Debug("🔄 Retrying worktree creation after cleanup...")
				_, retryErr := o.ExecuteGit(repoPath, args...)
				if retryErr != nil {
					return fmt.Errorf("worktree creation failed even after cleanup: %v", retryErr)
				}
				return nil
			}
		}
		return err
	}
}

func (o *OperationsImpl) RemoveWorktree(repoPath, worktreePath string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)
	_, err := o.ExecuteGit(repoPath, args...)
	return err
}

func (o *OperationsImpl) ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	output, err := o.ExecuteGit(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []WorktreeInfo
	var current WorktreeInfo

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "HEAD ") {
			current.Commit = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			// Extract the full branch reference
			fullRef := strings.TrimPrefix(line, "branch ")
			current.Branch = fullRef
		} else if line == "bare" {
			current.Bare = true
		}
	}

	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

func (o *OperationsImpl) PruneWorktrees(repoPath string) error {
	_, err := o.ExecuteGit(repoPath, "worktree", "prune")
	return err
}

// cleanupOrphanedWorktreeRegistration removes a specific orphaned worktree registration
// This is safer than `git worktree prune` which removes all orphaned registrations
func (o *OperationsImpl) cleanupOrphanedWorktreeRegistration(repoPath, worktreePath string) error {
	// First, list all worktrees to find the conflicting one
	worktrees, err := o.ListWorktrees(repoPath)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %v", err)
	}

	// Find the specific conflicting worktree
	var conflictingWorktree *WorktreeInfo
	for _, wt := range worktrees {
		if wt.Path == worktreePath {
			conflictingWorktree = &wt
			break
		}
	}

	if conflictingWorktree == nil {
		// No conflicting worktree found, nothing to clean up
		return fmt.Errorf("no conflicting worktree registration found for path: %s", worktreePath)
	}

	// Check if the worktree path actually exists and is valid
	if _, err := os.Stat(worktreePath); err == nil {
		// Path exists - check if it's a valid git worktree
		if _, gitErr := os.Stat(filepath.Join(worktreePath, ".git")); gitErr == nil {
			// It's a valid worktree, don't remove it
			return fmt.Errorf("path %s exists and appears to be a valid worktree", worktreePath)
		}
	}

	// The worktree is registered but the path doesn't exist or isn't valid
	// We can safely remove this specific registration
	logger.Infof("🧹 Removing orphaned worktree registration for: %s", worktreePath)

	// Use `git worktree remove` with --force to remove the registration
	// even if the directory doesn't exist
	_, err = o.ExecuteGit(repoPath, "worktree", "remove", "--force", worktreePath)
	if err != nil {
		return fmt.Errorf("failed to remove orphaned worktree registration: %v", err)
	}

	logger.Infof("✅ Successfully removed orphaned worktree registration for: %s", worktreePath)
	return nil
}

// Status operations

func (o *OperationsImpl) IsDirty(worktreePath string) bool {
	return o.statusChecker.IsDirty(worktreePath)
}

func (o *OperationsImpl) HasConflicts(worktreePath string) bool {
	return o.statusChecker.HasConflicts(worktreePath)
}

func (o *OperationsImpl) HasUncommittedChanges(worktreePath string) (bool, error) {
	return o.statusChecker.HasUncommittedChanges(worktreePath)
}

func (o *OperationsImpl) GetConflictedFiles(worktreePath string) ([]string, error) {
	return o.statusChecker.GetConflictedFiles(worktreePath)
}

func (o *OperationsImpl) GetStatus(worktreePath string) (*WorktreeStatus, error) {
	return o.statusChecker.GetWorktreeStatus(worktreePath)
}

// Fetch operations

func (o *OperationsImpl) FetchBranch(repoPath string, strategy FetchStrategy) error {
	return o.fetchExecutor.FetchBranch(repoPath, strategy)
}

func (o *OperationsImpl) FetchBranchFast(repoPath, branch string) error {
	return o.fetchExecutor.FetchBranchFast(repoPath, branch)
}

func (o *OperationsImpl) FetchBranchFull(repoPath, branch string) error {
	return o.fetchExecutor.FetchBranchFull(repoPath, branch)
}

// Push operations

func (o *OperationsImpl) PushBranch(worktreePath string, strategy PushStrategy) error {
	return o.pushExecutor.PushBranch(worktreePath, strategy)
}

// Remote operations

func (o *OperationsImpl) AddRemote(repoPath, name, url string) error {
	_, err := o.ExecuteGit(repoPath, "remote", "add", name, url)
	return err
}

func (o *OperationsImpl) RemoveRemote(repoPath, name string) error {
	_, err := o.ExecuteGit(repoPath, "remote", "remove", name)
	return err
}

func (o *OperationsImpl) SetRemoteURL(repoPath, name, url string) error {
	return o.urlManager.SetupRemoteURL(repoPath, name, url)
}

func (o *OperationsImpl) GetRemotes(repoPath string) (map[string]string, error) {
	output, err := o.ExecuteGit(repoPath, "remote", "-v")
	if err != nil {
		return nil, err
	}

	remotes := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.Contains(line, "(fetch)") {
			remotes[parts[0]] = parts[1]
		}
	}
	return remotes, nil
}

// Clone operations

func (o *OperationsImpl) Clone(url, path string, options CloneOptions) error {
	args := []string{"clone"}
	if options.Bare {
		args = append(args, "--bare")
	}
	if options.Depth > 0 {
		args = append(args, "--depth", strconv.Itoa(options.Depth))
	}
	if options.SingleBranch {
		args = append(args, "--single-branch")
	}
	if options.Branch != "" {
		args = append(args, "--branch", options.Branch)
	}
	args = append(args, url, path)

	_, err := o.ExecuteGit("", args...)
	return err
}

// Commit operations

func (o *OperationsImpl) Add(worktreePath string, paths ...string) error {
	args := []string{"add"}
	if len(paths) == 0 {
		args = append(args, ".")
	} else {
		args = append(args, paths...)
	}
	_, err := o.ExecuteGit(worktreePath, args...)
	return err
}

func (o *OperationsImpl) Commit(worktreePath, message string, options CommitOptions) error {
	args := []string{"commit", "-m", message}
	if options.NoVerify {
		args = append(args, "-n")
	}
	if options.Amend {
		args = append(args, "--amend")
	}
	if options.Author != "" {
		args = append(args, "--author", options.Author)
	}
	_, err := o.ExecuteGit(worktreePath, args...)
	return err
}

func (o *OperationsImpl) GetCommitHash(worktreePath, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	output, err := o.ExecuteGit(worktreePath, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (o *OperationsImpl) ResetMixed(worktreePath, ref string) error {
	_, err := o.ExecuteGit(worktreePath, "reset", "--mixed", ref)
	return err
}

// Merge/Rebase operations

func (o *OperationsImpl) Merge(worktreePath, ref string) error {
	_, err := o.ExecuteGit(worktreePath, "merge", ref)
	return err
}

func (o *OperationsImpl) Rebase(worktreePath, ref string) error {
	_, err := o.ExecuteGit(worktreePath, "rebase", ref)
	return err
}

func (o *OperationsImpl) CherryPick(worktreePath, commit string) error {
	_, err := o.ExecuteGit(worktreePath, "cherry-pick", commit)
	return err
}

func (o *OperationsImpl) AbortRebase(worktreePath string) error {
	_, err := o.ExecuteGit(worktreePath, "rebase", "--abort")
	return err
}

func (o *OperationsImpl) ContinueRebase(worktreePath string) error {
	_, err := o.ExecuteGit(worktreePath, "rebase", "--continue")
	return err
}

// Diff operations

func (o *OperationsImpl) DiffNameOnly(worktreePath, filter string) ([]string, error) {
	args := []string{"diff", "--name-only"}
	if filter != "" {
		args = append(args, "--diff-filter="+filter)
	}

	output, err := o.ExecuteGit(worktreePath, args...)
	if err != nil {
		return nil, err
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	for _, file := range files {
		if file != "" {
			result = append(result, file)
		}
	}
	return result, nil
}

func (o *OperationsImpl) MergeTree(worktreePath, base, head string) (string, error) {
	// Use the modern merge-tree command which automatically finds the merge base
	// We need to capture both stdout and stderr since conflict messages go to stderr
	stdout, stderr, err := o.executor.ExecuteGitWithStdErr(worktreePath, "merge-tree", "--write-tree", base, head)
	if err != nil {
		return "", fmt.Errorf("merge-tree command failed: %v", err)
	}

	// Combine stdout and stderr - conflict info is in stderr
	combinedOutput := string(stdout) + string(stderr)

	return combinedOutput, nil
}

// Stash operations

func (o *OperationsImpl) Stash(worktreePath string) error {
	_, err := o.ExecuteGit(worktreePath, "stash")
	return err
}

func (o *OperationsImpl) StashPop(worktreePath string) error {
	_, err := o.ExecuteGit(worktreePath, "stash", "pop")
	return err
}

// Tag operations

func (o *OperationsImpl) CreateTag(repoPath, tag, ref string) error {
	args := []string{"tag", tag}
	if ref != "" {
		args = append(args, ref)
	}
	_, err := o.ExecuteGit(repoPath, args...)
	return err
}

func (o *OperationsImpl) DeleteTag(repoPath, tag string) error {
	_, err := o.ExecuteGit(repoPath, "tag", "-d", tag)
	return err
}

func (o *OperationsImpl) ListTags(repoPath string) ([]string, error) {
	output, err := o.ExecuteGit(repoPath, "tag")
	if err != nil {
		return nil, err
	}

	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	for _, tag := range tags {
		if tag != "" {
			result = append(result, tag)
		}
	}
	return result, nil
}

// Config operations

func (o *OperationsImpl) GetConfig(repoPath, key string) (string, error) {
	output, err := o.ExecuteGit(repoPath, "config", "--get", key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (o *OperationsImpl) SetConfig(repoPath, key, value string) error {
	_, err := o.ExecuteGit(repoPath, "config", key, value)
	return err
}

func (o *OperationsImpl) UnsetConfig(repoPath, key string) error {
	_, err := o.ExecuteGit(repoPath, "config", "--unset", key)
	return err
}

func (o *OperationsImpl) SetGlobalConfig(key, value string) error {
	// Execute without working directory for global config
	cmd := exec.Command("git", "config", "--global", key, value)
	// Set HOME for git config location, preserve existing USER
	cmd.Env = append(os.Environ(),
		"HOME="+config.Runtime.HomeDir,
	)
	return cmd.Run()
}

// GetDisplayBranch returns the display branch name, checking for nice name mapping first
func (o *OperationsImpl) GetDisplayBranch(worktreePath string) (string, error) {
	// First get the actual git branch/ref
	var actualBranch string
	if branchOutput, err := o.ExecuteGit(worktreePath, "symbolic-ref", "HEAD"); err == nil {
		actualBranch = strings.TrimSpace(string(branchOutput))
	} else {
		// Fallback to branch --show-current for detached HEAD or other cases
		if branchOutput, err := o.ExecuteGit(worktreePath, "branch", "--show-current"); err == nil {
			actualBranch = strings.TrimSpace(string(branchOutput))
		} else {
			return "", err
		}
	}

	// If it's a catnip branch, check if there's a nice name mapping
	if strings.HasPrefix(actualBranch, "refs/catnip/") {
		// Check git config for nice branch mapping
		configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(actualBranch, "/", "."))
		if niceBranch, err := o.GetConfig(worktreePath, configKey); err == nil && strings.TrimSpace(niceBranch) != "" {
			// Return the nice branch name
			return strings.TrimSpace(niceBranch), nil
		}
	}

	// Clean up the branch name (remove refs/heads/ prefix if present)
	actualBranch = strings.TrimPrefix(actualBranch, "refs/heads/")

	return actualBranch, nil
}

// Rev operations

func (o *OperationsImpl) RevParse(repoPath, ref string) (string, error) {
	output, err := o.ExecuteGit(repoPath, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (o *OperationsImpl) RevList(repoPath string, options RevListOptions) ([]string, error) {
	args := []string{"rev-list"}
	if options.Count {
		args = append(args, "--count")
	}
	if options.MaxCount > 0 {
		args = append(args, "--max-count", strconv.Itoa(options.MaxCount))
	}
	if options.Since != "" {
		args = append(args, "--since", options.Since)
	}
	if options.Until != "" {
		args = append(args, "--until", options.Until)
	}

	output, err := o.ExecuteGit(repoPath, args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

func (o *OperationsImpl) ShowRef(repoPath, ref string, options ShowRefOptions) error {
	args := []string{"show-ref"}
	if options.Verify {
		args = append(args, "--verify")
	}
	if options.Quiet {
		args = append(args, "--quiet")
	}
	args = append(args, ref)

	_, err := o.ExecuteGit(repoPath, args...)
	return err
}

// Garbage collection

func (o *OperationsImpl) GarbageCollect(repoPath string) error {
	_, err := o.ExecuteGit(repoPath, "gc", "--prune=now")
	return err
}

// Utility operations

func (o *OperationsImpl) IsGitRepository(path string) bool {
	// Use rev-parse --git-dir which is more reliable for checking if it's a git repo
	_, err := o.ExecuteGit(path, "rev-parse", "--git-dir")
	return err == nil
}

func (o *OperationsImpl) GetGitRoot(path string) (string, error) {
	root, found := FindGitRoot(path)
	if !found {
		return "", fmt.Errorf("not a git repository")
	}
	return root, nil
}
