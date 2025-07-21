package git

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSessionName(t *testing.T) {
	name := GenerateSessionName()
	assert.NotEmpty(t, name)
	assert.True(t, strings.HasPrefix(name, "catnip/"), "Branch name should start with catnip/")

	// Extract cat name
	catName := strings.TrimPrefix(name, "catnip/")
	assert.NotEmpty(t, catName)
	assert.LessOrEqual(t, len(catName), 7, "Cat name should be max 7 characters")

	// Verify it's a valid cat name from our list
	found := false
	for _, validName := range catNames {
		if catName == validName {
			found = true
			break
		}
	}
	assert.True(t, found, "Generated name should be from our cat names list")
}

func TestGenerateSessionNameWithAdjective(t *testing.T) {
	name := GenerateSessionNameWithAdjective()
	assert.NotEmpty(t, name)
	assert.True(t, strings.HasPrefix(name, "catnip/"), "Branch name should start with catnip/")

	// Extract the part after catnip/
	namePart := strings.TrimPrefix(name, "catnip/")
	assert.Contains(t, namePart, "-", "Should contain hyphen between adjective and cat name")

	// Split into adjective and cat name
	parts := strings.Split(namePart, "-")
	assert.Len(t, parts, 2, "Should have exactly 2 parts")

	adjective := parts[0]
	catName := parts[1]

	// Verify adjective is valid
	foundAdj := false
	for _, validAdj := range adjectives {
		if adjective == validAdj {
			foundAdj = true
			break
		}
	}
	assert.True(t, foundAdj, "Adjective should be from our adjectives list")

	// Verify cat name is valid
	foundCat := false
	for _, validCat := range catNames {
		if catName == validCat {
			foundCat = true
			break
		}
	}
	assert.True(t, foundCat, "Cat name should be from our cat names list")

	// Check combined length is reasonable
	assert.LessOrEqual(t, len(namePart), 20, "Combined adjective-catname should be reasonable length")
}

func TestIsCatnipBranch(t *testing.T) {
	testCases := []struct {
		branch   string
		expected bool
	}{
		// Simple cat names
		{"catnip/felix", true},
		{"catnip/luna", true},
		{"catnip/milo", true},
		{"catnip/tiger", true},

		// Adjective-cat names
		{"catnip/fuzzy-felix", true},
		{"catnip/silly-luna", true},
		{"catnip/tiny-milo", true},
		{"catnip/happy-tiger", true},

		// Invalid formats
		{"catnip/notacat", false},
		{"catnip/toolongname", false},
		{"catnip/invalid-felix", false},     // Invalid adjective
		{"catnip/fuzzy-notacat", false},     // Invalid cat name
		{"catnip/fuzzy-happy-felix", false}, // Too many parts
		{"feature/something", false},
		{"main", false},
		{"catnip-felix", false}, // Wrong separator
		{"felix", false},        // Missing prefix
	}

	for _, tc := range testCases {
		t.Run(tc.branch, func(t *testing.T) {
			result := IsCatnipBranch(tc.branch)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseGitHubURL(t *testing.T) {
	testCases := []struct {
		url           string
		expectedOwner string
		expectedRepo  string
		shouldError   bool
	}{
		{"https://github.com/owner/repo", "owner", "repo", false},
		{"https://github.com/owner/repo.git", "owner", "repo", false},
		{"git@github.com:owner/repo.git", "owner", "repo", false},
		{"ssh://git@github.com/owner/repo.git", "owner", "repo", false},
		{"https://example.com/owner/repo", "", "", true},
		{"invalid-url", "", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			owner, repo, err := ParseGitHubURL(tc.url)

			if tc.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedOwner, owner)
				assert.Equal(t, tc.expectedRepo, repo)
			}
		})
	}
}

func TestConvertSSHToHTTPS(t *testing.T) {
	testCases := []struct {
		ssh      string
		expected string
	}{
		{"git@github.com:owner/repo.git", "https://github.com/owner/repo.git"},
		{"ssh://git@github.com:owner/repo.git", "https://github.com/owner/repo.git"},
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo.git"}, // Should pass through
	}

	for _, tc := range testCases {
		t.Run(tc.ssh, func(t *testing.T) {
			result := ConvertSSHToHTTPS(tc.ssh)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestExtractConflictFiles(t *testing.T) {
	conflictOutput := `CONFLICT (content): Merge conflict in file1.txt
both modified:   file2.txt
both added:      file3.txt`

	files := ExtractConflictFiles(conflictOutput)

	expectedFiles := []string{"file1.txt", "file2.txt", "file3.txt"}
	assert.ElementsMatch(t, expectedFiles, files)
}

func TestHasConflictMarkers(t *testing.T) {
	testCases := []struct {
		output   string
		expected bool
	}{
		{"<<<<<<< HEAD", true},
		{"=======", true},
		{">>>>>>> branch", true},
		{"CONFLICT", true},
		{"Automatic merge failed", true},
		{"No conflicts here", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.output, func(t *testing.T) {
			result := HasConflictMarkers(tc.output)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsMergeConflict(t *testing.T) {
	testCases := []struct {
		output   string
		expected bool
	}{
		{"CONFLICT (content): Merge conflict in file.txt", true},
		{"Automatic merge failed; fix conflicts and then commit", true},
		{"error: could not apply abc123", true},
		{"No problems here", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.output, func(t *testing.T) {
			result := IsMergeConflict(tc.output)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGenerateUniqueSessionName(t *testing.T) {
	t.Run("FindsAvailableSimpleName", func(t *testing.T) {
		// Mock branch checker that says nothing exists
		branchExists := func(name string) bool {
			return false // All names are available
		}

		name := GenerateUniqueSessionName(branchExists)
		assert.True(t, strings.HasPrefix(name, "catnip/"))
		assert.NotContains(t, name, "-")       // Should be a simple cat name, not adjective-cat
		assert.NotContains(t, name, "special") // Should not use timestamp fallback
	})

	t.Run("FallsBackToAdjectiveName", func(t *testing.T) {
		callCount := 0
		// Mock branch checker that makes simple names unavailable but adjective names available
		branchExists := func(name string) bool {
			callCount++
			// First 20 calls (simple names) return true (exists)
			// Subsequent calls (adjective names) return false (available)
			return callCount <= 20
		}

		name := GenerateUniqueSessionName(branchExists)
		assert.True(t, strings.HasPrefix(name, "catnip/"))
		assert.Contains(t, name, "-")          // Should be adjective-cat format
		assert.NotContains(t, name, "special") // Should not use timestamp fallback
	})

	t.Run("FallsBackToTimestamp", func(t *testing.T) {
		// Mock branch checker that says all names exist
		branchExists := func(name string) bool {
			return true // All names are taken
		}

		name := GenerateUniqueSessionName(branchExists)
		assert.True(t, strings.HasPrefix(name, "catnip/special-"))
		// Should contain timestamp-like number at the end
		parts := strings.Split(name, "-")
		assert.Len(t, parts, 2)
		assert.Equal(t, "catnip/special", parts[0])
		// parts[1] should be a number (timestamp)
		_, err := strconv.ParseInt(parts[1], 10, 64)
		assert.NoError(t, err)
	})
}

func TestExtractWorkspaceName(t *testing.T) {
	testCases := []struct {
		branchName   string
		expectedName string
	}{
		// Simple catnip branches
		{"catnip/felix", "felix"},
		{"catnip/luna", "luna"},
		{"catnip/milo", "milo"},

		// Adjective-cat catnip branches
		{"catnip/fuzzy-felix", "fuzzy-felix"},
		{"catnip/silly-luna", "silly-luna"},
		{"catnip/tiny-milo", "tiny-milo"},

		// Special fallback names
		{"catnip/special-1234567890", "special-1234567890"},

		// Non-catnip branches (should be unchanged)
		{"main", "main"},
		{"feature/something", "feature/something"},
		{"develop", "develop"},
		{"hotfix/bug-123", "hotfix/bug-123"},
	}

	for _, tc := range testCases {
		t.Run(tc.branchName, func(t *testing.T) {
			result := ExtractWorkspaceName(tc.branchName)
			assert.Equal(t, tc.expectedName, result)
		})
	}
}

func TestContains(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}

	assert.True(t, Contains(slice, "banana"))
	assert.False(t, Contains(slice, "orange"))
	assert.False(t, Contains([]string{}, "anything"))
}

func TestFindGitRoot(t *testing.T) {
	// Create a temporary directory structure
	tempDir, err := os.MkdirTemp("", "git-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create nested directories
	nestedDir := filepath.Join(tempDir, "subdir", "deep")
	err = os.MkdirAll(nestedDir, 0755)
	require.NoError(t, err)

	// Create .git directory at root
	gitDir := filepath.Join(tempDir, ".git")
	err = os.Mkdir(gitDir, 0755)
	require.NoError(t, err)

	t.Run("FindFromRoot", func(t *testing.T) {
		root, found := FindGitRoot(tempDir)
		assert.True(t, found)
		assert.Equal(t, tempDir, root)
	})

	t.Run("FindFromNestedDir", func(t *testing.T) {
		root, found := FindGitRoot(nestedDir)
		assert.True(t, found)
		assert.Equal(t, tempDir, root)
	})

	t.Run("NotFound", func(t *testing.T) {
		// Create a directory without .git
		nonGitDir, err := os.MkdirTemp("", "non-git-test")
		require.NoError(t, err)
		defer os.RemoveAll(nonGitDir)

		root, found := FindGitRoot(nonGitDir)
		assert.False(t, found)
		assert.Empty(t, root)
	})

	t.Run("GitWorktreeFile", func(t *testing.T) {
		// Test git worktree case (where .git is a file, not directory)
		worktreeDir, err := os.MkdirTemp("", "worktree-test")
		require.NoError(t, err)
		defer os.RemoveAll(worktreeDir)

		gitFile := filepath.Join(worktreeDir, ".git")
		err = os.WriteFile(gitFile, []byte("gitdir: /path/to/main/.git"), 0644)
		require.NoError(t, err)

		root, found := FindGitRoot(worktreeDir)
		assert.True(t, found)
		assert.Equal(t, worktreeDir, root)
	})
}
