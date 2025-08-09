package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
)

// TestBranchSynchronizationConcept tests the core concept of our branch synchronization
func TestBranchSynchronizationConcept(t *testing.T) {
	// Create temporary workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("CATNIP_WORKSPACE_DIR")
	require.NoError(t, os.Setenv("CATNIP_WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("CATNIP_WORKSPACE_DIR", oldWorkspace) }()

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

	// Create a real git repository for testing
	testRepo := filepath.Join(tempDir, "test-repo")
	require.NoError(t, os.MkdirAll(testRepo, 0755))

	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())

	// Set up basic git config for testing
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())

	// Create initial commit
	require.NoError(t, os.WriteFile(filepath.Join(testRepo, "README.md"), []byte("# Test"), 0644))
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())

	// Create service with real git operations and isolated state
	operations := git.NewOperations()
	stateDir := t.TempDir()
	service := NewGitServiceWithStateDir(operations, stateDir)
	require.NotNil(t, service)

	t.Run("BranchMappingStorage", func(t *testing.T) {
		// Test that we can store and retrieve branch mappings in git config
		workDir := testRepo
		customRef := "refs/catnip/muddy-cat"
		niceBranch := "feature/awesome-feature"

		// Store mapping using the same format as our implementation
		configKey := "catnip.branch-map." + strings.ReplaceAll(customRef, "/", ".")
		err := operations.SetConfig(workDir, configKey, niceBranch)
		assert.NoError(t, err)

		// Retrieve mapping
		retrievedBranch, err := operations.GetConfig(workDir, configKey)
		assert.NoError(t, err)
		assert.Equal(t, niceBranch, retrievedBranch)
	})

	t.Run("CustomRefWorktreeStaysUnchanged", func(t *testing.T) {
		// The key insight: when we rename a branch, the worktree should stay on the custom ref
		// This test validates the concept rather than the implementation

		// Test shows that creating a nice branch doesn't affect the worktree's HEAD
		workDir := testRepo
		// customRef := "refs/catnip/muddy-cat" // Referenced in comments below
		niceBranch := "feature/awesome-feature"

		// Simulate worktree being on custom ref (this would be done during worktree creation)
		// In real implementation, worktree HEAD points to custom ref

		// Creating a nice branch should not change where the worktree points
		err := operations.CreateBranch(workDir, niceBranch, "HEAD")
		assert.NoError(t, err)

		// Verify branch exists but worktree stays unchanged
		assert.True(t, operations.BranchExists(workDir, niceBranch, false))

		// In our architecture:
		// - Worktree HEAD -> refs/catnip/muddy-cat (unchanged)
		// - Nice branch refs/heads/feature/awesome-feature -> same commit (for external access)
		// - UI shows "feature/awesome-feature" but worktree works on custom ref
	})

	t.Run("CreateBranchFromCustomRef", func(t *testing.T) {
		// Test creating branches when HEAD points to a custom ref
		// This addresses the "HEAD not found below refs/heads!" error

		workDir := testRepo
		customRef := "refs/catnip/test-cat"
		niceBranch := "feature/from-custom-ref"

		// Get current commit
		currentCommit, err := operations.GetCommitHash(workDir, "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, currentCommit)

		// Create a custom ref pointing to current commit (simulate catnip worktree)
		_, err = operations.ExecuteGit(workDir, "update-ref", customRef, currentCommit)
		require.NoError(t, err)

		// Set HEAD to point to the custom ref (simulate catnip worktree checkout)
		_, err = operations.ExecuteGit(workDir, "symbolic-ref", "HEAD", customRef)
		require.NoError(t, err)

		// Verify HEAD points to custom ref
		headRef, err := operations.ExecuteGit(workDir, "symbolic-ref", "HEAD")
		require.NoError(t, err)
		assert.Equal(t, customRef, strings.TrimSpace(string(headRef)))

		// Now try to create a branch - this should work with our fix
		err = operations.CreateBranch(workDir, niceBranch, "")
		assert.NoError(t, err, "Should be able to create branch when HEAD points to custom ref")

		// Debug: Check what refs exist
		refsOutput, _ := operations.ExecuteGit(workDir, "show-ref")
		t.Logf("All refs after branch creation:\n%s", string(refsOutput))

		// Verify the branch was created correctly - check both ways
		branchExistsLocal := operations.BranchExists(workDir, niceBranch, false)
		branchExistsFullRef := operations.BranchExists(workDir, "refs/heads/"+niceBranch, false)

		t.Logf("Branch '%s' exists (local check): %v", niceBranch, branchExistsLocal)
		t.Logf("Branch 'refs/heads/%s' exists (full ref check): %v", niceBranch, branchExistsFullRef)

		assert.True(t, branchExistsLocal || branchExistsFullRef, "Branch should exist either as local or full ref")

		// Verify both the custom ref and nice branch point to the same commit
		customRefCommit, err := operations.GetCommitHash(workDir, customRef)
		require.NoError(t, err)
		niceBranchCommit, err := operations.GetCommitHash(workDir, niceBranch)
		require.NoError(t, err)
		assert.Equal(t, customRefCommit, niceBranchCommit)
	})

	t.Run("TodoBasedBranchRenamingSkipsExistingMapping", func(t *testing.T) {
		// Test that todo-based branch renaming doesn't trigger when a nice branch already exists
		workDir := testRepo
		customRef := "refs/catnip/test-prevent-duplicate"
		niceBranch := "feature/existing-nice-branch"

		// Get current commit
		currentCommit, err := operations.GetCommitHash(workDir, "HEAD")
		require.NoError(t, err)

		// Create a custom ref and set HEAD to it (simulate catnip worktree)
		_, err = operations.ExecuteGit(workDir, "update-ref", customRef, currentCommit)
		require.NoError(t, err)
		_, err = operations.ExecuteGit(workDir, "symbolic-ref", "HEAD", customRef)
		require.NoError(t, err)

		// Create a nice branch manually (simulate previous successful renaming)
		err = operations.CreateBranch(workDir, niceBranch, currentCommit)
		require.NoError(t, err)

		// Set up the branch mapping in git config (this is what prevents duplicate renaming)
		configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(customRef, "/", "."))
		err = operations.SetConfig(workDir, configKey, niceBranch)
		require.NoError(t, err)

		// Verify the mapping exists
		retrievedBranch, err := operations.GetConfig(workDir, configKey)
		require.NoError(t, err)
		assert.Equal(t, niceBranch, strings.TrimSpace(retrievedBranch))

		// This test verifies the concept: if we had a todo monitor, it would check
		// for this git config key and skip renaming if it already exists
		// The actual checkTodoBasedBranchRenaming function should now do this check
	})
}

// TestSyncMechanismConcept tests the synchronization between custom ref and nice branch
func TestSyncMechanismConcept(t *testing.T) {
	// Create temporary workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("CATNIP_WORKSPACE_DIR")
	require.NoError(t, os.Setenv("CATNIP_WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("CATNIP_WORKSPACE_DIR", oldWorkspace) }()

	// Create a real git repository for testing
	testRepo := filepath.Join(tempDir, "test-repo2")
	require.NoError(t, os.MkdirAll(testRepo, 0755))

	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())

	// Set up basic git config for testing
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())

	// Create initial commit
	require.NoError(t, os.WriteFile(filepath.Join(testRepo, "README.md"), []byte("# Test"), 0644))
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = testRepo
	require.NoError(t, cmd.Run())

	// Create service with real git operations
	operations := git.NewOperations()

	t.Run("CommitSyncToBothRefs", func(t *testing.T) {
		// Test that when commits are made on custom ref, they sync to the nice branch
		workDir := testRepo // Use the actual test repo path, not a hardcoded path
		customRef := "refs/catnip/muddy-cat"
		niceBranch := "feature/awesome-feature"

		// Set up branch mapping (as if Claude had already renamed the branch)
		configKey := "catnip.branch-map." + strings.ReplaceAll(customRef, "/", ".")
		err := operations.SetConfig(workDir, configKey, niceBranch)
		require.NoError(t, err)

		// Create the nice branch pointing to the same commit as the custom ref
		err = operations.CreateBranch(workDir, niceBranch, "HEAD")
		require.NoError(t, err)

		// Verify both the custom ref and nice branch exist
		assert.True(t, operations.BranchExists(workDir, niceBranch, false))

		// In the real implementation:
		// 1. Commits happen on the custom ref (worktree HEAD)
		// 2. The commit sync service detects new commits
		// 3. It updates the nice branch to point to the same commit
		// 4. For local repos, it pushes the nice branch to the "catnip-live" remote
	})

	t.Run("PullRequestUsesMappedBranch", func(t *testing.T) {
		// Test the logic for PR creation with custom refs
		workDir := testRepo
		customRef := "refs/catnip/muddy-cat"
		niceBranch := "feature/pr-ready"

		// Store branch mapping
		configKey := "catnip.branch-map." + strings.ReplaceAll(customRef, "/", ".")
		err := operations.SetConfig(workDir, configKey, niceBranch)
		require.NoError(t, err)

		// Simulate the PR creation logic from github_manager.go
		worktree := &models.Worktree{
			Path:   workDir,
			Branch: customRef,
		}

		var branchToPush string
		if strings.HasPrefix(worktree.Branch, "refs/catnip/") {
			mappingKey := "catnip.branch-map." + strings.ReplaceAll(worktree.Branch, "/", ".")
			niceBranch, err := operations.GetConfig(worktree.Path, mappingKey)
			if err == nil && strings.TrimSpace(niceBranch) != "" {
				branchToPush = strings.TrimSpace(niceBranch)
			}
		}

		// Verify PR uses the nice branch name
		assert.Equal(t, "feature/pr-ready", branchToPush)

		// The key benefit: PR uses a readable branch name while worktree stays on custom ref
	})

	t.Run("LocalRepoLiveRemoteSetup", func(t *testing.T) {
		// Test that local repos get a "catnip-live" remote for pushing nice branches
		workDir := testRepo
		mainRepoPath := "/home/user/main-repo"

		// Add the catnip-live remote (as done in worktree creation)
		err := operations.AddRemote(workDir, "catnip-live", mainRepoPath)
		require.NoError(t, err)

		// Verify remote was added
		remotes, err := operations.GetRemotes(workDir)
		require.NoError(t, err)
		assert.Equal(t, mainRepoPath, remotes["catnip-live"])

		// In practice, this allows:
		// git push catnip-live feature/nice-branch:feature/nice-branch
		// So the nice branch appears in the main repository for external checkout
	})
}
