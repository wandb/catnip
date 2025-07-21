package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// CommandExecutorImpl implements CommandExecutor using the git binary
type CommandExecutorImpl struct{}

// NewGitCommandExecutor creates a new Git command executor
func NewGitCommandExecutor() CommandExecutor {
	return &CommandExecutorImpl{}
}

// Execute runs a git command in the specified directory
func (e *CommandExecutorImpl) Execute(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// ExecuteWithEnv runs a git command with custom environment variables
func (e *CommandExecutorImpl) ExecuteWithEnv(dir string, env []string, args ...string) ([]byte, error) {
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
