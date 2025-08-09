package executor

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

// InMemoryExecutor implements executor.CommandExecutor using go-git in-memory repositories
type InMemoryExecutor struct {
	repositories map[string]*TestRepository // Map of path to repository
}

// NewInMemoryExecutor creates a new in-memory git executor for testing
func NewInMemoryExecutor() CommandExecutor {
	return &InMemoryExecutor{
		repositories: make(map[string]*TestRepository),
	}
}

// AddRepository adds a test repository at the given path
func (e *InMemoryExecutor) AddRepository(path string, repo *TestRepository) {
	e.repositories[path] = repo
}

// CreateRepository creates a new test repository at the given path
func (e *InMemoryExecutor) CreateRepository(path string) (*TestRepository, error) {
	repo, err := NewTestRepository(path)
	if err != nil {
		return nil, err
	}
	e.AddRepository(path, repo)
	return repo, nil
}

// Execute implements CommandExecutor.Execute for general commands
func (e *InMemoryExecutor) Execute(dir string, args ...string) ([]byte, error) {
	// For non-git commands, we need to handle them specifically or return an error
	if len(args) == 0 {
		return nil, fmt.Errorf("no command provided")
	}

	command := args[0]

	switch command {
	case "echo":
		if len(args) > 1 {
			return []byte(strings.Join(args[1:], " ") + "\n"), nil
		}
		return []byte("\n"), nil
	default:
		return nil, fmt.Errorf("command not supported in memory executor: %s", command)
	}
}

// ExecuteWithEnv implements CommandExecutor.ExecuteWithEnv
func (e *InMemoryExecutor) ExecuteWithEnv(dir string, env []string, args ...string) ([]byte, error) {
	// For simplicity, ignore env vars in testing and delegate to Execute
	return e.Execute(dir, args...)
}

// ExecuteCommand implements CommandExecutor.ExecuteCommand
func (e *InMemoryExecutor) ExecuteCommand(command string, args ...string) ([]byte, error) {
	allArgs := append([]string{command}, args...)
	return e.Execute("", allArgs...)
}

// ExecuteGitWithWorkingDir implements CommandExecutor.ExecuteGitWithWorkingDir
func (e *InMemoryExecutor) ExecuteGitWithWorkingDir(workingDir string, args ...string) ([]byte, error) {
	// Find the repository for this working directory
	repo := e.findRepository(workingDir)
	if repo == nil {
		return nil, fmt.Errorf("no repository found for path: %s", workingDir)
	}

	// Handle git commands using go-git operations
	return e.handleGitCommand(repo, workingDir, args...)
}

// ExecuteGitWithStdErr implements CommandExecutor.ExecuteGitWithStdErr for testing
func (e *InMemoryExecutor) ExecuteGitWithStdErr(workingDir string, args ...string) ([]byte, []byte, error) {
	// For testing, we'll simulate merge-tree behavior
	if len(args) >= 1 && args[0] == "merge-tree" {
		// Simulate merge-tree output with no conflicts for testing
		stdout := []byte("mock-tree-hash\n")
		stderr := []byte("")
		return stdout, stderr, nil
	}

	// For other commands, delegate to regular execution and return empty stderr
	output, err := e.ExecuteGitWithWorkingDir(workingDir, args...)
	return output, []byte(""), err
}

// findRepository finds the test repository that corresponds to the working directory
func (e *InMemoryExecutor) findRepository(workingDir string) *TestRepository {
	// Try exact match first
	if repo, exists := e.repositories[workingDir]; exists {
		return repo
	}

	// Try parent directories
	for path, repo := range e.repositories {
		if strings.HasPrefix(workingDir, path) {
			return repo
		}
	}

	return nil
}

// handleGitCommand handles specific git commands using go-git
func (e *InMemoryExecutor) handleGitCommand(repo *TestRepository, workingDir string, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no git command provided")
	}

	command := args[0]

	switch command {
	case "status":
		return e.handleStatus(repo, args[1:])
	case "branch":
		return e.handleBranch(repo, args[1:])
	case "rev-parse":
		return e.handleRevParse(repo, args[1:])
	case "rev-list":
		return e.handleRevList(repo, args[1:])
	case "remote":
		return e.handleRemote(repo, args[1:])
	case "fetch":
		return e.handleFetch(repo, args[1:])
	case "push":
		return e.handlePush(repo, args[1:])
	case "diff":
		return e.handleDiff(repo, args[1:])
	case "ls-files":
		return e.handleLsFiles(repo, args[1:])
	case "show":
		return e.handleShow(repo, args[1:])
	case "merge-base":
		return e.handleMergeBase(repo, args[1:])
	case "worktree":
		return e.handleWorktree(repo, args[1:])
	case "add":
		return e.handleAdd(repo, args[1:])
	case "commit":
		return e.handleCommit(repo, args[1:])
	case "checkout":
		return e.handleCheckout(repo, args[1:])
	case "config":
		return e.handleConfig(repo, args[1:])
	case "show-ref":
		return e.handleShowRef(repo, args[1:])
	default:
		return nil, fmt.Errorf("git command not implemented in memory executor: %s", command)
	}
}

// handleStatus implements git status --porcelain
func (e *InMemoryExecutor) handleStatus(repo *TestRepository, args []string) ([]byte, error) {
	// For now, return empty status (clean working directory)
	return []byte(""), nil
}

// handleBranch implements various git branch commands
func (e *InMemoryExecutor) handleBranch(repo *TestRepository, args []string) ([]byte, error) {
	gitRepo := repo.GetRepository()

	if len(args) > 0 {
		switch args[0] {
		case "--show-current":
			head, err := gitRepo.Head()
			if err != nil {
				return nil, err
			}
			branchName := head.Name().Short()
			return []byte(branchName + "\n"), nil
		case "--list":
			if len(args) > 1 {
				branchName := args[1]
				// Check if branch exists
				_, err := gitRepo.Reference(plumbing.ReferenceName("refs/heads/"+branchName), true)
				if err != nil {
					return []byte(""), nil // Branch doesn't exist
				}
				return []byte("  " + branchName + "\n"), nil
			}
		case "-r":
			// List remote branches (simplified)
			return []byte("  origin/main\n"), nil
		case "--format=%(refname:short)":
			// List all local branches
			refs, err := gitRepo.References()
			if err != nil {
				return nil, err
			}
			var branches []string
			_ = refs.ForEach(func(ref *plumbing.Reference) error {
				if ref.Name().IsBranch() {
					branches = append(branches, ref.Name().Short())
				}
				return nil
			})
			return []byte(strings.Join(branches, "\n") + "\n"), nil
		}
	}

	return []byte(""), nil
}

// handleRevParse implements git rev-parse
func (e *InMemoryExecutor) handleRevParse(repo *TestRepository, args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("rev-parse requires an argument")
	}

	gitRepo := repo.GetRepository()

	switch args[0] {
	case "HEAD":
		head, err := gitRepo.Head()
		if err != nil {
			return nil, err
		}
		return []byte(head.Hash().String() + "\n"), nil
	case "--verify":
		if len(args) > 1 {
			ref := args[1]
			_, err := gitRepo.Reference(plumbing.ReferenceName(ref), true)
			if err != nil {
				return nil, err
			}
			return []byte(""), nil
		}
	}

	return []byte(""), nil
}

// handleRevList implements git rev-list --count
func (e *InMemoryExecutor) handleRevList(repo *TestRepository, args []string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "--count" {
		// For simplicity, return a mock count
		return []byte("0\n"), nil
	}
	return []byte(""), nil
}

// handleRemote implements git remote commands
func (e *InMemoryExecutor) handleRemote(repo *TestRepository, args []string) ([]byte, error) {
	if len(args) == 0 {
		return []byte(""), nil
	}

	switch args[0] {
	case "get-url":
		if len(args) > 1 && args[1] == "origin" {
			return []byte("https://github.com/test/repo.git\n"), nil
		}
	case "set-url":
		// Mock success
		return []byte(""), nil
	case "add":
		// Mock success
		return []byte(""), nil
	}

	return []byte(""), nil
}

// handleFetch implements git fetch
func (e *InMemoryExecutor) handleFetch(repo *TestRepository, args []string) ([]byte, error) {
	// Mock successful fetch
	return []byte(""), nil
}

// handlePush implements git push
func (e *InMemoryExecutor) handlePush(repo *TestRepository, args []string) ([]byte, error) {
	// Mock successful push
	return []byte(""), nil
}

// handleDiff implements git diff commands
func (e *InMemoryExecutor) handleDiff(repo *TestRepository, args []string) ([]byte, error) {
	// For testing, return empty diff (no changes)
	return []byte(""), nil
}

// handleLsFiles implements git ls-files
func (e *InMemoryExecutor) handleLsFiles(repo *TestRepository, args []string) ([]byte, error) {
	// Return empty list for simplicity
	return []byte(""), nil
}

// handleShow implements git show
func (e *InMemoryExecutor) handleShow(repo *TestRepository, args []string) ([]byte, error) {
	// Mock file content for testing
	return []byte("mock file content\n"), nil
}

// handleMergeBase implements git merge-base
func (e *InMemoryExecutor) handleMergeBase(repo *TestRepository, args []string) ([]byte, error) {
	if len(args) >= 2 {
		gitRepo := repo.GetRepository()
		head, err := gitRepo.Head()
		if err != nil {
			return nil, err
		}
		// Return HEAD hash as merge base for testing
		return []byte(head.Hash().String() + "\n"), nil
	}
	return []byte(""), nil
}

// handleWorktree implements git worktree commands
func (e *InMemoryExecutor) handleWorktree(repo *TestRepository, args []string) ([]byte, error) {
	// Mock successful worktree operations
	return []byte(""), nil
}

// handleAdd implements git add
func (e *InMemoryExecutor) handleAdd(repo *TestRepository, args []string) ([]byte, error) {
	// Mock successful add
	return []byte(""), nil
}

// handleCommit implements git commit
func (e *InMemoryExecutor) handleCommit(repo *TestRepository, args []string) ([]byte, error) {
	// Mock successful commit
	return []byte(""), nil
}

// handleCheckout implements git checkout
func (e *InMemoryExecutor) handleCheckout(repo *TestRepository, args []string) ([]byte, error) {
	// Mock successful checkout
	return []byte(""), nil
}

// handleConfig implements git config
func (e *InMemoryExecutor) handleConfig(repo *TestRepository, args []string) ([]byte, error) {
	// Mock successful config
	return []byte(""), nil
}

// handleShowRef implements git show-ref
func (e *InMemoryExecutor) handleShowRef(repo *TestRepository, args []string) ([]byte, error) {
	gitRepo := repo.GetRepository()

	// Parse flags
	verify := false
	quiet := false
	refName := ""

	for _, arg := range args {
		switch arg {
		case "--verify":
			verify = true
		case "--quiet":
			quiet = true
		default:
			if strings.HasPrefix(arg, "refs/") {
				refName = arg
			}
		}
	}

	// If we have a specific ref to verify
	if verify && refName != "" {
		// Look up the reference
		ref, err := gitRepo.Reference(plumbing.ReferenceName(refName), true)
		if err != nil {
			// Reference not found - return non-zero exit code
			return nil, fmt.Errorf("reference not found: %s", refName)
		}

		if quiet {
			// --quiet mode: return empty output on success
			return []byte(""), nil
		} else {
			// Return hash and ref name
			return []byte(fmt.Sprintf("%s %s\n", ref.Hash().String(), refName)), nil
		}
	}

	// Default behavior: list all references (not implemented for now)
	return []byte(""), nil
}

// ExecuteWithEnvAndTimeout implements the timeout interface for testing - ignores timeout
func (e *InMemoryExecutor) ExecuteWithEnvAndTimeout(dir string, env []string, timeout time.Duration, args ...string) ([]byte, error) {
	return e.ExecuteWithEnv(dir, env, args...)
}
