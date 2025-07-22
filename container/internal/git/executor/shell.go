package executor

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ShellExecutor implements CommandExecutor using the git binary
type ShellExecutor struct {
	defaultEnv []string
}

// NewShellExecutor creates a new shell-based Git command executor
func NewShellExecutor() CommandExecutor {
	return &ShellExecutor{
		defaultEnv: []string{
			"HOME=/home/catnip",
			"USER=catnip",
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
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(cmd.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// ExecuteGitWithWorkingDir runs a git command with -C flag for working directory
func (e *ShellExecutor) ExecuteGitWithWorkingDir(workingDir string, args ...string) ([]byte, error) {
	if workingDir != "" {
		args = append([]string{"-C", workingDir}, args...)
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
