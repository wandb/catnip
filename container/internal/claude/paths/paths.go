// Package paths provides shared utilities for working with Claude session file paths
package paths

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EncodePathForClaude encodes a filesystem path the way Claude does for project directories.
// Claude replaces both "/" and "." with "-" and ensures a leading dash.
func EncodePathForClaude(path string) string {
	// Claude replaces both "/" and "." with "-"
	encoded := strings.ReplaceAll(path, "/", "-")
	encoded = strings.ReplaceAll(encoded, ".", "-")
	encoded = strings.TrimPrefix(encoded, "-")
	encoded = "-" + encoded
	return encoded
}

// GetProjectDir returns the Claude projects directory path for a given worktree/project path.
// Returns the full path to ~/.claude/projects/<encoded-path>/
func GetProjectDir(worktreePath string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	encodedPath := EncodePathForClaude(worktreePath)
	return filepath.Join(homeDir, ".claude", "projects", encodedPath), nil
}

// IsValidSessionUUID checks if a string is a valid Claude session UUID.
// Valid UUIDs are 36 characters with 4 dashes (e.g., cf568042-7147-4fba-a2ca-c6a646581260)
func IsValidSessionUUID(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

// SessionCandidate represents a candidate session file for selection
type SessionCandidate struct {
	Path    string
	Size    int64
	ModTime int64
}

// FindBestSessionFile finds the most recently modified session file in a directory.
// It filters out snapshot-only files (files containing only file-history-snapshot entries),
// and validates that filenames are valid session UUIDs.
// Returns the full path to the best session file, or an error if none found.
func FindBestSessionFile(projectDir string) (string, error) {
	// Check if directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return "", fmt.Errorf("claude projects directory does not exist: %s", projectDir)
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", fmt.Errorf("failed to read project directory: %w", err)
	}

	var candidates []SessionCandidate

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		// Validate UUID format
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		if !IsValidSessionUUID(sessionID) {
			continue
		}

		fullPath := filepath.Join(projectDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Check if this file has conversation content (not just snapshots)
		if !hasConversationContent(fullPath) {
			continue
		}

		candidates = append(candidates, SessionCandidate{
			Path:    fullPath,
			Size:    info.Size(),
			ModTime: info.ModTime().UnixNano(),
		})
	}

	if len(candidates) == 0 {
		// Fallback: try any valid UUID jsonl file by modification time
		return findAnyValidSessionFile(projectDir, entries)
	}

	// Pick the most recently modified session
	// Tie-breaker: largest size (more content)
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.ModTime > best.ModTime {
			best = c
		} else if c.ModTime == best.ModTime && c.Size > best.Size {
			best = c
		}
	}

	return best.Path, nil
}

// hasConversationContent checks if a session file contains actual conversation messages
// (user, assistant) rather than just file-history-snapshot or summary entries.
// It also filters out forked/sidechain sessions that were created for automated tasks
// (like branch naming or PR generation) which start with queue-operation.
// Returns true if the file has conversation content and is not a forked session.
func hasConversationContent(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Check first 50 lines to determine if this is a real conversation
	lineCount := 0
	foundConversation := false
	for scanner.Scan() && lineCount < 50 {
		line := scanner.Text()
		lineCount++

		// Skip forked/sidechain sessions that start with queue-operation
		// These are automated sessions created by --fork-session for branch naming, PR generation, etc.
		if lineCount == 1 && strings.Contains(line, `"type":"queue-operation"`) {
			return false
		}

		// Look for conversation message types
		// These indicate actual user interaction, not just snapshots
		if strings.Contains(line, `"type":"user"`) ||
			strings.Contains(line, `"type":"assistant"`) {
			foundConversation = true
		}
	}

	return foundConversation
}

// findAnyValidSessionFile is a fallback that finds any valid UUID jsonl file by modification time
func findAnyValidSessionFile(projectDir string, entries []fs.DirEntry) (string, error) {
	var sessionFiles []fs.DirEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		// Only consider valid UUID files
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		if IsValidSessionUUID(sessionID) {
			sessionFiles = append(sessionFiles, entry)
		}
	}

	if len(sessionFiles) == 0 {
		return "", fmt.Errorf("no session files found in %s", projectDir)
	}

	// Sort by modification time (most recent first)
	sort.Slice(sessionFiles, func(i, j int) bool {
		infoI, _ := sessionFiles[i].Info()
		infoJ, _ := sessionFiles[j].Info()
		return infoI.ModTime().After(infoJ.ModTime())
	})

	return filepath.Join(projectDir, sessionFiles[0].Name()), nil
}

// FindGitRoot looks for a .git directory starting from the given path
// and searching up to maxLevels parent directories.
// Returns the git root path, or empty string if not found.
func FindGitRoot(startPath string, maxLevels int) string {
	currentPath := startPath

	for i := 0; i <= maxLevels; i++ {
		gitPath := filepath.Join(currentPath, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return currentPath
		}

		// Move to parent directory
		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			// Reached filesystem root
			break
		}
		currentPath = parent
	}

	return ""
}

// ResolveSessionPath resolves a session identifier to a full file path.
// The identifier can be:
// - A full path to a .jsonl file
// - A session UUID (will be resolved relative to the given projectDir)
//
// Returns the full path to the session file and any error encountered.
func ResolveSessionPath(identifier string, projectDir string) (string, error) {
	// Check if it looks like a UUID
	if IsValidSessionUUID(identifier) {
		sessionFile := filepath.Join(projectDir, identifier+".jsonl")
		if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
			return "", fmt.Errorf("session UUID %q not found in %s", identifier, projectDir)
		}
		return sessionFile, nil
	}

	// Check if it's a path to a file
	if !strings.HasSuffix(identifier, ".jsonl") {
		return "", fmt.Errorf("invalid session file: %q is not a .jsonl file", identifier)
	}

	// Check if the file exists
	if _, err := os.Stat(identifier); os.IsNotExist(err) {
		return "", fmt.Errorf("session file not found: %s", identifier)
	}

	return identifier, nil
}
