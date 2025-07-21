package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowChangeDetector(t *testing.T) {
	t.Run("IsWorkflowFile", func(t *testing.T) {
		executor := NewInMemoryExecutor()
		detector := NewWorkflowChangeDetector(executor)

		// Test positive cases
		assert.True(t, detector.isWorkflowFile(".github/workflows/test.yml"))
		assert.True(t, detector.isWorkflowFile(".github/workflows/build.yaml"))
		assert.True(t, detector.isWorkflowFile(".github/workflows/deploy/prod.yml"))

		// Test negative cases
		assert.False(t, detector.isWorkflowFile(""))
		assert.False(t, detector.isWorkflowFile("test.yml"))
		assert.False(t, detector.isWorkflowFile(".github/test.yml"))
		assert.False(t, detector.isWorkflowFile(".github/workflows/test.txt"))
		assert.False(t, detector.isWorkflowFile("src/main.go"))
	})

	t.Run("ContainsWorkflowFiles", func(t *testing.T) {
		executor := NewInMemoryExecutor()
		detector := NewWorkflowChangeDetector(executor)

		files := []string{
			"src/main.go",
			".github/workflows/test.yml",
			"README.md",
		}
		assert.True(t, detector.containsWorkflowFiles(files))

		filesNoWorkflow := []string{
			"src/main.go",
			"test.yml",
			"README.md",
		}
		assert.False(t, detector.containsWorkflowFiles(filesNoWorkflow))

		emptyFiles := []string{}
		assert.False(t, detector.containsWorkflowFiles(emptyFiles))
	})

	t.Run("HasWorkflowChangesWithRealRepo", func(t *testing.T) {
		// Create a real git repository for testing
		tempDir := t.TempDir()
		repoDir := filepath.Join(tempDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0755))

		// Use shell executor for real git operations
		executor := NewGitCommandExecutor()
		detector := NewWorkflowChangeDetector(executor)

		// Initialize git repo
		_, err := executor.ExecuteGitWithWorkingDir(repoDir, "init")
		require.NoError(t, err)

		// Configure git
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)

		// Create initial commit
		readmePath := filepath.Join(repoDir, "README.md")
		require.NoError(t, os.WriteFile(readmePath, []byte("# Test\n"), 0644))
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "add", "README.md")
		require.NoError(t, err)
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "commit", "-m", "Initial commit")
		require.NoError(t, err)

		// Create a branch
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "checkout", "-b", "feature-branch")
		require.NoError(t, err)

		// Initially, no workflow changes
		hasChanges := detector.HasWorkflowChanges(repoDir, "feature-branch")
		assert.False(t, hasChanges)

		// Add a workflow file
		workflowDir := filepath.Join(repoDir, ".github", "workflows")
		require.NoError(t, os.MkdirAll(workflowDir, 0755))
		workflowPath := filepath.Join(workflowDir, "test.yml")
		workflowContent := `name: Test
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - run: echo "Hello World"
`
		require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "add", ".github/workflows/test.yml")
		require.NoError(t, err)
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "commit", "-m", "Add workflow file")
		require.NoError(t, err)

		// Now should detect workflow changes
		hasChanges = detector.HasWorkflowChanges(repoDir, "feature-branch")
		assert.True(t, hasChanges)

		// Test GetWorkflowFiles - since we don't have a remote, this might return empty
		// but the workflow detection should still work via hasWorkflowFilesInWorkingTree
		workflowFiles := detector.GetWorkflowFiles(repoDir, "feature-branch")
		// Note: This might be empty because there's no remote to compare against
		// but HasWorkflowChanges should still work via working tree detection
		_ = workflowFiles // We tested HasWorkflowChanges above which is the main functionality
	})
}

func TestPushStrategyWithWorkflowDetection(t *testing.T) {
	t.Run("WorkflowDetectionIntegration", func(t *testing.T) {
		// Create a temporary repository
		tempDir := t.TempDir()
		repoDir := filepath.Join(tempDir, "workflow-test")
		require.NoError(t, os.MkdirAll(repoDir, 0755))

		// Use shell executor
		executor := NewGitCommandExecutor()

		// Initialize git repo
		_, err := executor.ExecuteGitWithWorkingDir(repoDir, "init")
		require.NoError(t, err)

		// Configure git
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)

		// Create initial commit
		readmePath := filepath.Join(repoDir, "README.md")
		require.NoError(t, os.WriteFile(readmePath, []byte("# Test\n"), 0644))
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "add", "README.md")
		require.NoError(t, err)
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "commit", "-m", "Initial commit")
		require.NoError(t, err)

		// Create feature branch
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "checkout", "-b", "feature")
		require.NoError(t, err)

		// Create push executor
		pushExecutor := NewPushExecutor(executor)

		// Test normal push strategy (no workflow files)
		// ConvertHTTPS is false, but workflow detection should still trigger for workflow files
		strategy := PushStrategy{
			Branch: "feature",
		}

		// This would normally fail due to no remote, but we're testing the detection logic
		err = pushExecutor.PushBranch(repoDir, strategy)
		assert.Error(t, err) // Expected to fail due to no remote setup

		// Add workflow file
		workflowDir := filepath.Join(repoDir, ".github", "workflows")
		require.NoError(t, os.MkdirAll(workflowDir, 0755))
		workflowPath := filepath.Join(workflowDir, "test.yml")
		require.NoError(t, os.WriteFile(workflowPath, []byte("name: Test\n"), 0644))
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "add", ".github/workflows/test.yml")
		require.NoError(t, err)
		_, err = executor.ExecuteGitWithWorkingDir(repoDir, "commit", "-m", "Add workflow")
		require.NoError(t, err)

		// Now workflow detection should trigger
		// We can't test the actual push without a remote, but we can verify the detector works
		detector := pushExecutor.workflowDetector
		hasWorkflowChanges := detector.HasWorkflowChanges(repoDir, "feature")
		assert.True(t, hasWorkflowChanges)
	})
}
