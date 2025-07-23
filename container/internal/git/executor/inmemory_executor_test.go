package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestInMemoryExecutorMethods tests uncovered methods in InMemoryExecutor
func TestInMemoryExecutorMethods(t *testing.T) {
	exec := NewInMemoryExecutor()

	t.Run("Execute", func(t *testing.T) {
		// Test Execute method with echo command (supported)
		output, err := exec.Execute("/tmp", "echo", "hello")
		assert.NoError(t, err)
		assert.Equal(t, "hello\n", string(output))

		// Test Execute method with unsupported command
		_, err = exec.Execute("/tmp", "unsupported-cmd")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "command not supported in memory executor")
	})

	t.Run("ExecuteWithEnv", func(t *testing.T) {
		// Test ExecuteWithEnv method (delegates to Execute)
		output, err := exec.ExecuteWithEnv("/tmp", []string{"TEST_VAR=value"}, "echo", "world")
		assert.NoError(t, err)
		assert.Equal(t, "world\n", string(output))
	})

	t.Run("ExecuteCommand", func(t *testing.T) {
		// Test ExecuteCommand with echo command (should work)
		output, err := exec.ExecuteCommand("echo", "hello")
		assert.NoError(t, err)
		assert.Equal(t, "hello\n", string(output))

		// Test ExecuteCommand with unsupported command
		_, err = exec.ExecuteCommand("unsupported-cmd", "arg")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "command not supported in memory executor")
	})
}
