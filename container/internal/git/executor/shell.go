package executor

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/vanpelt/catnip/internal/config"
)

// ShellExecutor implements CommandExecutor using the git binary
type ShellExecutor struct {
	defaultEnv []string
}

// NewShellExecutor creates a new shell-based Git command executor
func NewShellExecutor() CommandExecutor {
	return &ShellExecutor{
		defaultEnv: []string{
			"HOME=" + config.Runtime.HomeDir,
		},
	}
}

// NewGitCommandExecutor is deprecated, use NewShellExecutor instead
func NewGitCommandExecutor() CommandExecutor {
	return NewShellExecutor()
}

// Execute runs a git command in the specified directory
func (e *ShellExecutor) Execute(dir string, args ...string) ([]byte, error) {
	return e.ExecuteWithEnv(dir, e.defaultEnv, args...)
}

// ExecuteWithEnv runs a git command with custom environment variables
func (e *ShellExecutor) ExecuteWithEnv(dir string, env []string, args ...string) ([]byte, error) {
	return e.ExecuteWithEnvAndTimeout(dir, env, 0, args...)
}

// ExecuteWithEnvAndTimeout runs a git command with custom environment variables and timeout
func (e *ShellExecutor) ExecuteWithEnvAndTimeout(dir string, env []string, timeout time.Duration, args ...string) ([]byte, error) {
	var ctx context.Context
	var cancel context.CancelFunc

	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	} else {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(cmd.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("git %s timed out after %v", strings.Join(args, " "), timeout)
		}
		return nil, fmt.Errorf("git %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// ExecuteGitWithWorkingDir runs a git command with -C flag for working directory
func (e *ShellExecutor) ExecuteGitWithWorkingDir(workingDir string, args ...string) ([]byte, error) {
	if workingDir != "" {
		args = append([]string{"-C", workingDir}, args...)
	}
	// Only log non-routine git commands
	if len(args) > 0 && args[0] != "-C" && (len(args) < 2 || (args[1] != "symbolic-ref" && args[1] != "rev-list" && args[1] != "rev-parse" && !strings.HasPrefix(args[1], "diff"))) {
		log.Printf("🐚 ShellExecutor: executing git %v", args)
	}
	return e.Execute("", args...)
}

// ExecuteCommand runs any command (not just git) with standard environment
func (e *ShellExecutor) ExecuteCommand(command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(cmd.Environ(), e.defaultEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %v\nstderr: %s", command, strings.Join(args, " "), err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// ExecuteGitWithStdErr runs a git command and returns both stdout and stderr
func (e *ShellExecutor) ExecuteGitWithStdErr(workingDir string, args ...string) ([]byte, []byte, error) {
	if workingDir != "" {
		args = append([]string{"-C", workingDir}, args...)
	}

	cmd := exec.Command("git", args...)
	cmd.Env = append(cmd.Environ(), e.defaultEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// For merge-tree, exit status 1 just means conflicts detected, not an error
	err := cmd.Run()
	if err != nil {
		// Check if this is a merge-tree command with exit status 1
		// Need to search through args since -C flag shifts the position
		isMergeTree := false
		for _, arg := range args {
			if arg == "merge-tree" {
				isMergeTree = true
				break
			}
		}

		if isMergeTree {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				// Exit status 1 for merge-tree just means conflicts detected
				return stdout.Bytes(), stderr.Bytes(), nil
			}
		}
		// For other errors, return them normally
		return nil, nil, fmt.Errorf("git %s failed: %v", strings.Join(args, " "), err)
	}

	return stdout.Bytes(), stderr.Bytes(), nil
}
