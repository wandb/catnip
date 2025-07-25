package executor

// CommandExecutor abstracts Git command execution
type CommandExecutor interface {
	Execute(dir string, args ...string) ([]byte, error)
	ExecuteWithEnv(dir string, env []string, args ...string) ([]byte, error)
	ExecuteGitWithWorkingDir(workingDir string, args ...string) ([]byte, error)
	ExecuteCommand(command string, args ...string) ([]byte, error)
	// ExecuteGitWithStdErr captures both stdout and stderr for commands that need both
	ExecuteGitWithStdErr(workingDir string, args ...string) (stdout []byte, stderr []byte, err error)
}
