package git

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// GoGitCommandExecutor implements CommandExecutor using go-git library
// Falls back to shell commands for operations not supported by go-git
type GoGitCommandExecutor struct {
	fallbackExecutor CommandExecutor
	repositoryCache  map[string]*gogit.Repository
}

// NewGoGitCommandExecutor creates a new go-git based command executor
func NewGoGitCommandExecutor() CommandExecutor {
	return &GoGitCommandExecutor{
		fallbackExecutor: NewGitCommandExecutor(), // Shell git as fallback
		repositoryCache:  make(map[string]*gogit.Repository),
	}
}

// Execute runs a git command - uses go-git where possible, falls back to shell
func (e *GoGitCommandExecutor) Execute(dir string, args ...string) ([]byte, error) {
	return e.ExecuteGitWithWorkingDir(dir, args...)
}

// ExecuteWithEnv runs a git command with custom environment - falls back to shell for env support
func (e *GoGitCommandExecutor) ExecuteWithEnv(dir string, env []string, args ...string) ([]byte, error) {
	// go-git doesn't support custom env, so fallback to shell
	return e.fallbackExecutor.ExecuteWithEnv(dir, env, args...)
}

// ExecuteGitWithWorkingDir runs a git command with working directory - main implementation
func (e *GoGitCommandExecutor) ExecuteGitWithWorkingDir(workingDir string, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no git command provided")
	}

	command := args[0]

	// Handle commands that we can implement with go-git
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
func (e *GoGitCommandExecutor) ExecuteCommand(command string, args ...string) ([]byte, error) {
	return e.fallbackExecutor.ExecuteCommand(command, args...)
}

// getRepository gets or opens a repository, caching the result
func (e *GoGitCommandExecutor) getRepository(repoPath string) (*gogit.Repository, error) {
	if repoPath == "" {
		repoPath = "."
	}

	// Resolve absolute path for caching
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Check cache first
	if repo, exists := e.repositoryCache[absPath]; exists {
		return repo, nil
	}

	// Try to open existing repository
	repo, err := gogit.PlainOpen(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository at %s: %w", absPath, err)
	}

	// Cache for future use
	e.repositoryCache[absPath] = repo
	return repo, nil
}

// handleStatus implements git status --porcelain
func (e *GoGitCommandExecutor) handleStatus(workingDir string, args []string) ([]byte, error) {
	repo, err := e.getRepository(workingDir)
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"status"}, args...)...)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"status"}, args...)...)
	}

	status, err := worktree.Status()
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
func (e *GoGitCommandExecutor) handleBranch(workingDir string, args []string) ([]byte, error) {
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
func (e *GoGitCommandExecutor) handleRemote(workingDir string, args []string) ([]byte, error) {
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
func (e *GoGitCommandExecutor) handleConfig(workingDir string, args []string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "--get" {
		configKey := args[1]

		// Handle specific config keys we can implement
		switch configKey {
		case "remote.origin.url":
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
func (e *GoGitCommandExecutor) handleRevParse(workingDir string, args []string) ([]byte, error) {
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
func (e *GoGitCommandExecutor) handleSymbolicRef(workingDir string, args []string) ([]byte, error) {
	// This is typically used for getting default branch from remote
	// Fall back to shell for now as it's complex to implement
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"symbolic-ref"}, args...)...)
}

// handleFetch implements git fetch commands
func (e *GoGitCommandExecutor) handleFetch(workingDir string, args []string) ([]byte, error) {
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
func (e *GoGitCommandExecutor) handleShowRef(workingDir string, args []string) ([]byte, error) {
	// show-ref is used for verifying references exist
	// For now, fall back to shell as it's mainly used for verification
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"show-ref"}, args...)...)
}

// handleLsRemote implements git ls-remote commands
func (e *GoGitCommandExecutor) handleLsRemote(workingDir string, args []string) ([]byte, error) {
	// ls-remote is complex and used for remote operations
	// Fall back to shell for reliable implementation
	return e.fallbackExecutor.ExecuteGitWithWorkingDir(workingDir, append([]string{"ls-remote"}, args...)...)
}

// Helper functions

func (e *GoGitCommandExecutor) listBranches(repo *gogit.Repository, includeRemote bool) ([]byte, error) {
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

func (e *GoGitCommandExecutor) getCurrentBranch(repo *gogit.Repository) ([]byte, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	if !head.Name().IsBranch() {
		return nil, fmt.Errorf("HEAD is not on a branch")
	}

	return []byte(head.Name().Short() + "\n"), nil
}

func (e *GoGitCommandExecutor) getStatusCode(status gogit.StatusCode) string {
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
