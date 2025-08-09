package executor

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// GitExecutor implements CommandExecutor using go-git library
// Falls back to shell commands for operations not supported by go-git
//
//revive:disable:exported
type GitExecutor struct {
	fallbackExecutor CommandExecutor
	repositoryCache  map[string]*gogit.Repository
	cacheMutex       sync.RWMutex           // Protects repositoryCache access
	operationMutexes map[string]*sync.Mutex // Per-repository operation mutexes
	mutexMapMutex    sync.RWMutex           // Protects operationMutexes map
}

// NewGitExecutor creates a new go-git based command executor (the main production executor)
func NewGitExecutor() CommandExecutor {
	return &GitExecutor{
		fallbackExecutor: NewShellExecutor(), // Shell git as fallback
		repositoryCache:  make(map[string]*gogit.Repository),
		operationMutexes: make(map[string]*sync.Mutex),
	}
}

// NewGoGitCommandExecutor is deprecated, use NewGitExecutor instead
func NewGoGitCommandExecutor() CommandExecutor {
	return NewGitExecutor()
}

// Execute runs a git command - uses go-git where possible, falls back to shell
func (e *GitExecutor) Execute(dir string, args ...string) ([]byte, error) {
	return e.ExecuteGitWithWorkingDir(dir, args...)
}

// ExecuteWithEnv runs a git command with custom environment - falls back to shell for env support
func (e *GitExecutor) ExecuteWithEnv(dir string, env []string, args ...string) ([]byte, error) {
	// go-git doesn't support custom env, so fallback to shell
	return e.fallbackExecutor.ExecuteWithEnv(dir, env, args...)
}

// ExecuteGitWithWorkingDir runs a git command with working directory - main implementation
func (e *GitExecutor) ExecuteGitWithWorkingDir(workingDir string, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no git command provided")
	}

	// Find the actual git command, skipping over git config flags (-c)
	command := ""
	commandIndex := -1
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "-c" {
			// Skip -c and its value
			i += 2 // Skip both -c and the config value
			continue
		} else if !strings.HasPrefix(arg, "-") {
			// Found the git command (not a flag)
			command = arg
			commandIndex = i
			break
		}
		i++
	}

	if command == "" {
		return nil, fmt.Errorf("no git command found in arguments")
	}

	// For commands with config flags, we need to pass all args to shell executor
	// since go-git doesn't handle -c config flags
	if commandIndex > 0 {
		// Has config flags, use shell executor for all operations
		log.Printf("🔧 GitExecutor: detected config flags, using shell fallback for: %v", args)
		log.Printf("🔍 GitExecutor: command=%s, commandIndex=%d, workingDir=%s", command, commandIndex, workingDir)
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, args...)
	}

	// Handle commands that we can implement with go-git (no config flags)
	switch command {
	case "status":
		return e.handleStatus(workingDir, args[1:])
	case "branch":
		return e.handleBranch(workingDir, args[1:])
	case "remote":
		return e.handleRemote(workingDir, args[1:])
	case "config":
		return e.handleConfig(workingDir, args[1:])
	case "rev-parse":
		return e.handleRevParse(workingDir, args[1:])
	case "symbolic-ref":
		return e.handleSymbolicRef(workingDir, args[1:])
	case "fetch":
		return e.handleFetch(workingDir, args[1:])
	case "show-ref":
		return e.handleShowRef(workingDir, args[1:])
	case "ls-remote":
		return e.handleLsRemote(workingDir, args[1:])
	case "rev-list":
		// rev-list operations are complex and better handled by shell git
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, args...)
	// Complex operations that require shell git
	case "merge", "rebase", "clone", "worktree", "push", "pull":
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, args...)
	// Diff operations with complex formatting
	case "diff":
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, args...)
	// Add, commit, and other working directory operations
	case "add", "commit", "checkout":
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, args...)
	default:
		// For commands not implemented in go-git, fall back to shell
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, args...)
	}
}

// ExecuteCommand runs any command (not just git) - always use fallback
func (e *GitExecutor) ExecuteCommand(command string, args ...string) ([]byte, error) {
	return e.fallbackExecutor.ExecuteCommand(command, args...)
}

// ExecuteGitWithStdErr runs a git command and returns both stdout and stderr - delegates to fallback
func (e *GitExecutor) ExecuteGitWithStdErr(workingDir string, args ...string) ([]byte, []byte, error) {
	return e.fallbackExecutor.ExecuteGitWithStdErr(workingDir, args...)
}

// getRepositoryMutex gets or creates a mutex for the given repository path
func (e *GitExecutor) getRepositoryMutex(absPath string) *sync.Mutex {
	// Check if mutex exists with read lock
	e.mutexMapMutex.RLock()
	if mutex, exists := e.operationMutexes[absPath]; exists {
		e.mutexMapMutex.RUnlock()
		return mutex
	}
	e.mutexMapMutex.RUnlock()

	// Create new mutex with write lock
	e.mutexMapMutex.Lock()
	defer e.mutexMapMutex.Unlock()

	// Double-check after acquiring write lock
	if mutex, exists := e.operationMutexes[absPath]; exists {
		return mutex
	}

	// Create and store new mutex
	mutex := &sync.Mutex{}
	e.operationMutexes[absPath] = mutex
	return mutex
}

// getRepository gets or opens a repository, caching the result
func (e *GitExecutor) getRepository(repoPath string) (*gogit.Repository, error) {
	if repoPath == "" {
		repoPath = "."
	}

	// Resolve absolute path for caching
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Check cache first with read lock
	e.cacheMutex.RLock()
	if repo, exists := e.repositoryCache[absPath]; exists {
		e.cacheMutex.RUnlock()
		return repo, nil
	}
	e.cacheMutex.RUnlock()

	// Not in cache, acquire write lock to open and cache
	e.cacheMutex.Lock()
	defer e.cacheMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine may have cached it)
	if repo, exists := e.repositoryCache[absPath]; exists {
		return repo, nil
	}

	// Try to open existing repository with full worktree support
	repo, err := gogit.PlainOpenWithOptions(absPath, &gogit.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open repository at %s: %w", absPath, err)
	}

	// Cache for future use
	e.repositoryCache[absPath] = repo
	return repo, nil
}

// handleStatus implements git status --porcelain
func (e *GitExecutor) handleStatus(workingDir string, args []string) ([]byte, error) {
	// Resolve workingDir to an absolute path for mutex lookup
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"status"}, args...)...)
	}

	// Get per-repository mutex and lock it for the entire operation
	mutex := e.getRepositoryMutex(absWorkingDir)
	mutex.Lock()
	defer mutex.Unlock()

	repo, err := e.getRepository(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"status"}, args...)...)
	}

	// Get all worktrees for the repository
	worktree, err := repo.Worktree()
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"status"}, args...)...)
	}
	worktrees := []*gogit.Worktree{worktree}

	// Find the worktree that matches the current working directory
	var targetWorktree *gogit.Worktree
	for _, wt := range worktrees {
		wtPath := wt.Filesystem.Root()
		absWtPath, err := filepath.Abs(wtPath)
		if err != nil {
			continue
		}

		if absWtPath == absWorkingDir {
			targetWorktree = wt
			break
		}
	}

	// If no matching worktree is found, something is wrong. Fallback to shell.
	if targetWorktree == nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"status"}, args...)...)
	}

	// Now use the correct worktree to get the status
	status, err := targetWorktree.Status()
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"status"}, args...)...)
	}

	// Check if --porcelain flag is present
	porcelain := false
	for _, arg := range args {
		if arg == "--porcelain" {
			porcelain = true
			break
		}
	}

	var output bytes.Buffer
	if porcelain {
		// Format as porcelain output
		for filename, fileStatus := range status {
			stagingCode := e.getStatusCode(fileStatus.Staging)
			worktreeCode := e.getStatusCode(fileStatus.Worktree)
			output.WriteString(fmt.Sprintf("%s%s %s\n", stagingCode, worktreeCode, filename))
		}
	} else {
		// Fall back to shell git for non-porcelain status (more complex formatting)
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"status"}, args...)...)
	}

	return output.Bytes(), nil
}

// handleBranch implements various git branch commands
func (e *GitExecutor) handleBranch(workingDir string, args []string) ([]byte, error) {
	// Resolve workingDir to an absolute path for mutex lookup
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"branch"}, args...)...)
	}

	// Get per-repository mutex and lock it for the entire operation
	mutex := e.getRepositoryMutex(absWorkingDir)
	mutex.Lock()
	defer mutex.Unlock()

	repo, err := e.getRepository(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"branch"}, args...)...)
	}

	// Handle different branch subcommands
	if len(args) == 0 {
		// List local branches
		return e.listBranches(repo, false)
	}

	if len(args) >= 1 {
		switch args[0] {
		case "-a", "--all":
			return e.listBranches(repo, true)
		case "--show-current":
			return e.getCurrentBranch(repo)
		default:
			// For branch creation, deletion, etc., fall back to shell
			return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"branch"}, args...)...)
		}
	}

	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"branch"}, args...)...)
}

// handleRemote implements git remote commands
func (e *GitExecutor) handleRemote(workingDir string, args []string) ([]byte, error) {
	// Resolve workingDir to an absolute path for mutex lookup
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"remote"}, args...)...)
	}

	// Get per-repository mutex and lock it for the entire operation
	mutex := e.getRepositoryMutex(absWorkingDir)
	mutex.Lock()
	defer mutex.Unlock()

	repo, err := e.getRepository(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"remote"}, args...)...)
	}

	if len(args) >= 2 && args[0] == "get-url" {
		remoteName := args[1]
		remote, err := repo.Remote(remoteName)
		if err != nil {
			return nil, fmt.Errorf("remote %s not found: %w", remoteName, err)
		}

		if len(remote.Config().URLs) == 0 {
			return nil, fmt.Errorf("no URLs configured for remote %s", remoteName)
		}

		return []byte(remote.Config().URLs[0] + "\n"), nil
	}

	// Fall back to shell for other remote operations
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"remote"}, args...)...)
}

// handleConfig implements basic git config queries
func (e *GitExecutor) handleConfig(workingDir string, args []string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "--get" {
		configKey := args[1]

		// Handle specific config keys we can implement
		switch configKey {
		case "remote.origin.url":
			// Resolve workingDir to an absolute path for mutex lookup
			absWorkingDir, err := filepath.Abs(workingDir)
			if err != nil {
				return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"config"}, args...)...)
			}

			// Get per-repository mutex and lock it for the entire operation
			mutex := e.getRepositoryMutex(absWorkingDir)
			mutex.Lock()
			defer mutex.Unlock()

			repo, err := e.getRepository(workingDir)
			if err != nil {
				return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"config"}, args...)...)
			}

			remote, err := repo.Remote("origin")
			if err != nil {
				return nil, fmt.Errorf("remote origin not found: %w", err)
			}

			if len(remote.Config().URLs) == 0 {
				return nil, fmt.Errorf("no URLs configured for origin remote")
			}

			return []byte(remote.Config().URLs[0] + "\n"), nil
		case "core.bare":
			// Resolve workingDir to an absolute path for mutex lookup
			absWorkingDir, err := filepath.Abs(workingDir)
			if err != nil {
				return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"config"}, args...)...)
			}

			// Get per-repository mutex and lock it for the entire operation
			mutex := e.getRepositoryMutex(absWorkingDir)
			mutex.Lock()
			defer mutex.Unlock()

			// Check if repository is bare
			repo, err := e.getRepository(workingDir)
			if err != nil {
				return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"config"}, args...)...)
			}

			// Try to get worktree - if it fails, likely bare
			_, err = repo.Worktree()
			if err != nil {
				return []byte("true\n"), nil
			}
			return []byte("false\n"), nil
		}
	}

	// Fall back to shell for other config operations
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"config"}, args...)...)
}

// handleRevParse implements git rev-parse commands
func (e *GitExecutor) handleRevParse(workingDir string, args []string) ([]byte, error) {
	// Resolve workingDir to an absolute path for mutex lookup
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"rev-parse"}, args...)...)
	}

	// Get per-repository mutex and lock it for the entire operation
	mutex := e.getRepositoryMutex(absWorkingDir)
	mutex.Lock()
	defer mutex.Unlock()

	repo, err := e.getRepository(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"rev-parse"}, args...)...)
	}

	for _, arg := range args {
		switch arg {
		case "--abbrev-ref":
			if len(args) >= 2 && args[1] == "HEAD" {
				return e.getCurrentBranch(repo)
			}
		case "--verify":
			// For branch verification, we can implement this
			if len(args) >= 2 {
				refName := args[1]
				_, err := repo.Reference(plumbing.ReferenceName(refName), true)
				if err != nil {
					return nil, fmt.Errorf("reference %s not found: %w", refName, err)
				}
				return []byte(""), nil // Success with empty output
			}
		case "HEAD":
			head, err := repo.Head()
			if err != nil {
				return nil, fmt.Errorf("failed to get HEAD: %w", err)
			}
			return []byte(head.Hash().String() + "\n"), nil
		}
	}

	// Fall back to shell for other rev-parse operations
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"rev-parse"}, args...)...)
}

// handleSymbolicRef implements git symbolic-ref commands
func (e *GitExecutor) handleSymbolicRef(workingDir string, args []string) ([]byte, error) {
	// This is typically used for getting default branch from remote
	// Fall back to shell for now as it's complex to implement
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"symbolic-ref"}, args...)...)
}

// handleFetch implements git fetch commands
func (e *GitExecutor) handleFetch(workingDir string, args []string) ([]byte, error) {
	// Resolve workingDir to an absolute path for mutex lookup
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"fetch"}, args...)...)
	}

	// Get per-repository mutex and lock it for the entire operation
	mutex := e.getRepositoryMutex(absWorkingDir)
	mutex.Lock()
	defer mutex.Unlock()

	repo, err := e.getRepository(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"fetch"}, args...)...)
	}

	// Simple fetch all
	if len(args) == 0 || (len(args) == 2 && args[0] == "--all" && args[1] == "--prune") {
		err := repo.Fetch(&gogit.FetchOptions{
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{"refs/*:refs/*"},
		})

		if err != nil && err != gogit.NoErrAlreadyUpToDate {
			return nil, fmt.Errorf("fetch failed: %w", err)
		}

		return []byte(""), nil // git fetch typically has no output on success
	}

	// For more complex fetch operations, fall back to shell
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"fetch"}, args...)...)
}

// handleShowRef implements git show-ref commands
func (e *GitExecutor) handleShowRef(workingDir string, args []string) ([]byte, error) {
	// show-ref is used for verifying references exist
	// For now, fall back to shell as it's mainly used for verification
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"show-ref"}, args...)...)
}

// handleLsRemote implements git ls-remote commands
func (e *GitExecutor) handleLsRemote(workingDir string, args []string) ([]byte, error) {
	// ls-remote is complex and used for remote operations
	// Fall back to shell for reliable implementation
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"ls-remote"}, args...)...)
}

// Helper functions

func (e *GitExecutor) listBranches(repo *gogit.Repository, includeRemote bool) ([]byte, error) {
	// Note: Repository operations are already protected by per-repository mutex in calling function
	refs, err := repo.References()
	if err != nil {
		return nil, fmt.Errorf("failed to get references: %w", err)
	}

	var branches []string
	currentBranch := ""

	// Get current branch
	head, err := repo.Head()
	if err == nil && head.Name().IsBranch() {
		currentBranch = head.Name().Short()
	}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name()

		if name.IsBranch() {
			branch := name.Short()
			prefix := "  "
			if branch == currentBranch {
				prefix = "* "
			}
			branches = append(branches, prefix+branch)
		} else if includeRemote && name.IsRemote() {
			branch := name.Short()
			branches = append(branches, "  remotes/"+branch)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to iterate references: %w", err)
	}

	output := strings.Join(branches, "\n")
	if output != "" {
		output += "\n"
	}

	return []byte(output), nil
}

func (e *GitExecutor) getCurrentBranch(repo *gogit.Repository) ([]byte, error) {
	// Note: Repository operations are already protected by per-repository mutex in calling function
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	if !head.Name().IsBranch() {
		return nil, fmt.Errorf("HEAD is not on a branch")
	}

	return []byte(head.Name().Short() + "\n"), nil
}

func (e *GitExecutor) getStatusCode(status gogit.StatusCode) string {
	switch status {
	case gogit.Unmodified:
		return " "
	case gogit.Modified:
		return "M"
	case gogit.Added:
		return "A"
	case gogit.Deleted:
		return "D"
	case gogit.Renamed:
		return "R"
	case gogit.Copied:
		return "C"
	case gogit.UpdatedButUnmerged:
		return "U"
	case gogit.Untracked:
		return "?"
	default:
		return "?"
	}
}

// ExecuteWithEnvAndTimeout runs a command with timeout - delegates to fallback executor
func (e *GitExecutor) ExecuteWithEnvAndTimeout(dir string, env []string, timeout time.Duration, args ...string) ([]byte, error) {
	return e.fallbackExecutor.ExecuteWithEnvAndTimeout(dir, env, timeout, args...)
}
