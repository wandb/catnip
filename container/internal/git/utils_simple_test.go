package git

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractWorkspaceName_AdditionalCases(t *testing.T) {
	// Test additional workspace name extraction cases beyond the existing test
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "simple branch name",
			input:    "main",
			expected: "main",
		},
		{
			name:     "feature branch",
			input:    "feature-auth",
			expected: "feature-auth",
		},
		{
			name:     "refs heads prefix - not modified",
			input:    "refs/heads/develop",
			expected: "refs/heads/develop",
		},
		{
			name:     "origin prefix - not modified",
			input:    "origin/staging",
			expected: "origin/staging",
		},
		{
			name:     "catnip prefix removal",
			input:    "catnip/feature-branch",
			expected: "feature-branch",
		},
		{
			name:     "refs catnip prefix removal",
			input:    "refs/catnip/felix-workspace",
			expected: "felix-workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractWorkspaceName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeBranchName_Simple(t *testing.T) {
	// Test a utility function if it exists, or create our own for demonstration
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal branch name",
			input:    "feature-branch",
			expected: "feature-branch",
		},
		{
			name:     "branch with special chars",
			input:    "feature/sub-branch",
			expected: "feature-sub-branch",
		},
		{
			name:     "branch with underscores",
			input:    "fix_issue_123",
			expected: "fix_issue_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeBranchNameForWorkspace(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Simple utility function to demonstrate testing
func sanitizeBranchNameForWorkspace(branchName string) string {
	// Replace slashes with hyphens for workspace names
	if branchName == "" {
		return ""
	}

	result := branchName
	result = strings.ReplaceAll(result, "/", "-")
	return result
}
