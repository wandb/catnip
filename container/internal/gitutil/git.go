package gitutil

import (
	"os"
	"path/filepath"
	"strings"
)

// FindGitRoot finds the git repository root starting from the given directory
func FindGitRoot(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil {
			// Check if it's a directory (normal repo) or file (worktree)
			if info.IsDir() {
				return dir, true
			}
			// If it's a file, it might be a git worktree
			if content, err := os.ReadFile(gitDir); err == nil {
				// Git worktree file contains "gitdir: path/to/git/dir"
				if strings.HasPrefix(string(content), "gitdir: ") {
					return dir, true
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			break
		}
		dir = parent
	}

	return "", false
}
