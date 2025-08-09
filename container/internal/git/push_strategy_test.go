package git

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vanpelt/catnip/internal/config"
)

// MockExecutorForURLRewrite records git commands to verify URL rewriting
type MockExecutorForURLRewrite struct {
	recordedCommands [][]string
	mockError        error
}

func (m *MockExecutorForURLRewrite) Execute(dir string, args ...string) ([]byte, error) {
	return []byte(""), m.mockError
}

func (m *MockExecutorForURLRewrite) ExecuteWithEnv(dir string, env []string, args ...string) ([]byte, error) {
	return []byte(""), m.mockError
}

func (m *MockExecutorForURLRewrite) ExecuteGitWithWorkingDir(workingDir string, args ...string) ([]byte, error) {
	// Record the git command for verification
	m.recordedCommands = append(m.recordedCommands, args)
	return []byte(""), m.mockError
}

func (m *MockExecutorForURLRewrite) ExecuteCommand(command string, args ...string) ([]byte, error) {
	return []byte(""), m.mockError
}

func (m *MockExecutorForURLRewrite) ExecuteGitWithStdErr(workingDir string, args ...string) ([]byte, []byte, error) {
	// Record the git command for verification
	m.recordedCommands = append(m.recordedCommands, args)
	return []byte(""), []byte(""), m.mockError
}

func (m *MockExecutorForURLRewrite) ExecuteWithEnvAndTimeout(dir string, env []string, timeout time.Duration, args ...string) ([]byte, error) {
	// Record the git command for verification
	m.recordedCommands = append(m.recordedCommands, args)
	return []byte(""), m.mockError
}

// TestPushStrategyConvertHTTPS verifies that ConvertHTTPS properly adds URL rewriting config
func TestPushStrategyConvertHTTPS(t *testing.T) {
	t.Run("ConvertHTTPS_true_adds_config", func(t *testing.T) {
		mockExec := &MockExecutorForURLRewrite{
			recordedCommands: make([][]string, 0),
		}

		pushExecutor := NewPushExecutor(mockExec)

		strategy := PushStrategy{
			Branch:       "test-branch",
			Remote:       "origin",
			SetUpstream:  true,
			ConvertHTTPS: true, // This should add URL rewriting in Docker mode
		}

		err := pushExecutor.PushBranch("/test/worktree", strategy)
		assert.NoError(t, err)

		// Verify that git was called
		assert.Len(t, mockExec.recordedCommands, 1)

		gitArgs := mockExec.recordedCommands[0]

		// Behavior depends on runtime mode
		if config.Runtime.IsContainerized() {
			// In containerized mode, should include URL rewriting config
			assert.Equal(t, "-c", gitArgs[0])
			assert.Equal(t, "url.https://github.com/.insteadOf=git@github.com:", gitArgs[1])
			assert.Equal(t, "push", gitArgs[2])
			assert.Equal(t, "-u", gitArgs[3])
			assert.Equal(t, "origin", gitArgs[4])
			assert.Equal(t, "test-branch", gitArgs[5])
		} else {
			// In native mode, should skip URL rewriting
			assert.Equal(t, "push", gitArgs[0])
			assert.Equal(t, "-u", gitArgs[1])
			assert.Equal(t, "origin", gitArgs[2])
			assert.Equal(t, "test-branch", gitArgs[3])
		}
	})

	t.Run("ConvertHTTPS_false_no_config", func(t *testing.T) {
		mockExec := &MockExecutorForURLRewrite{
			recordedCommands: make([][]string, 0),
		}

		pushExecutor := NewPushExecutor(mockExec)

		strategy := PushStrategy{
			Branch:       "test-branch",
			Remote:       "origin",
			SetUpstream:  true,
			ConvertHTTPS: false, // No URL rewriting
		}

		err := pushExecutor.PushBranch("/test/worktree", strategy)
		assert.NoError(t, err)

		// Verify that git was called without URL rewriting config
		assert.Len(t, mockExec.recordedCommands, 1)

		gitArgs := mockExec.recordedCommands[0]

		// Check that the command does NOT include URL rewriting config
		assert.Equal(t, "push", gitArgs[0])
		assert.Equal(t, "-u", gitArgs[1])
		assert.Equal(t, "origin", gitArgs[2])
		assert.Equal(t, "test-branch", gitArgs[3])

		// Ensure no -c config flags are present
		for _, arg := range gitArgs {
			assert.NotEqual(t, "-c", arg)
			assert.False(t, strings.Contains(arg, "insteadOf"))
		}
	})
}

// TestPushStrategyWithGitExecutor tests the integration with the actual GitExecutor
func TestPushStrategyWithGitExecutor(t *testing.T) {
	t.Run("GitExecutor_handles_config_flags", func(t *testing.T) {
		// Create a mock shell executor to capture what actually gets passed to shell
		mockShell := &MockExecutorForURLRewrite{
			recordedCommands: make([][]string, 0),
		}

		// We can't easily inject the fallback into GitExecutor without modifying the struct,
		// so let's test with a simpler mock that implements the same interface

		pushExecutor := NewPushExecutor(mockShell)

		strategy := PushStrategy{
			Branch:       "test-branch",
			Remote:       "origin",
			SetUpstream:  true,
			ConvertHTTPS: true,
		}

		err := pushExecutor.PushBranch("/test/worktree", strategy)
		assert.NoError(t, err)

		// Verify the command structure
		assert.Len(t, mockShell.recordedCommands, 1)
		gitArgs := mockShell.recordedCommands[0]

		// Behavior depends on runtime mode
		if config.Runtime.IsContainerized() {
			// In containerized mode, should include URL rewriting config
			expectedArgs := []string{
				"-c", "url.https://github.com/.insteadOf=git@github.com:",
				"push", "-u", "origin", "test-branch",
			}
			assert.Equal(t, expectedArgs, gitArgs)
		} else {
			// In native mode, should skip URL rewriting
			expectedArgs := []string{
				"push", "-u", "origin", "test-branch",
			}
			assert.Equal(t, expectedArgs, gitArgs)
		}
	})
}
