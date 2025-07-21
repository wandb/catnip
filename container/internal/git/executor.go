package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// CommandExecutorImpl implements CommandExecutor using the git binary
type CommandExecutorImpl struct {
	defaultEnv []string
}

// NewGitCommandExecutor creates a new Git command executor
func NewGitCommandExecutor() CommandExecutor {
	return &CommandExecutorImpl{
		defaultEnv: []string{
			"HOME=/home/catnip",
			"USER=catnip",
		},
	}
}

// Execute runs a git command in the specified directory
func (e *CommandExecutorImpl) Execute(dir string, args ...string) ([]byte, error) {
	return e.ExecuteWithEnv(dir, e.defaultEnv, args...)
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

// ExecuteGitWithWorkingDir runs a git command with -C flag for working directory
func (e *CommandExecutorImpl) ExecuteGitWithWorkingDir(workingDir string, args ...string) ([]byte, error) {
	if workingDir != "" {
		args = append([]string{"-C", workingDir}, args...)
	}
	return e.Execute("", args...)
}

// ExecuteCommand runs any command (not just git) with standard environment
func (e *CommandExecutorImpl) ExecuteCommand(command string, args ...string) ([]byte, error) {
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
