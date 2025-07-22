// Package git provides Git repository and worktree management functionality
// with support for both command-line git and go-git implementations.
package git

// Re-export commonly used types and functions
var (
	// Default operations instance (can be overridden for testing)
	DefaultOperations = NewOperations()
)
