package git

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vanpelt/catnip/internal/logger"
)

// ClaudeSessionDetector detects and monitors Claude sessions running in a worktree
// DEPRECATED: This is a fallback mechanism that reads Claude's JSONL files to extract session information.
// It's unreliable as the JSONL format may change and summary records are not always present.
// Prefer using the title information from PTY escape sequences or the syscall title interceptor.
type ClaudeSessionDetector struct {
	workDir string
}

// NewClaudeSessionDetector creates a new Claude session detector
func NewClaudeSessionDetector(workDir string) *ClaudeSessionDetector {
	return &ClaudeSessionDetector{
		workDir: workDir,
	}
}

// ClaudeSessionInfo contains information about a detected Claude session
type ClaudeSessionInfo struct {
	SessionID   string
	Title       string
	PID         int
	StartTime   time.Time
	LastUpdated time.Time
}

// DetectClaudeSession looks for active Claude sessions in the worktree
// NOTE: This is a best-effort approach and may not always return accurate information.
// It attempts to find session files and running processes, but Claude's internal structure may change.
func (d *ClaudeSessionDetector) DetectClaudeSession() (*ClaudeSessionInfo, error) {
	// First, try to find the Claude session file
	sessionInfo := d.findClaudeSessionFromFiles()
	if sessionInfo != nil {
		// Try to enrich with process information
		if pid := d.findClaudeProcess(); pid > 0 {
			sessionInfo.PID = pid
		}
		return sessionInfo, nil
	}

	// If no session file, try to detect from running processes
	pid := d.findClaudeProcess()
	if pid > 0 {
		return &ClaudeSessionInfo{
			PID:       pid,
			StartTime: time.Now(), // Approximate
		}, nil
	}

	return nil, fmt.Errorf("no Claude session detected")
}

// findClaudeSessionFromFiles looks for Claude session files and extracts information
func (d *ClaudeSessionDetector) findClaudeSessionFromFiles() *ClaudeSessionInfo {
	claudeProjectsDir := filepath.Join(d.workDir, ".claude", "projects")
	if _, err := os.Stat(claudeProjectsDir); os.IsNotExist(err) {
		return nil
	}

	files, err := os.ReadDir(claudeProjectsDir)
	if err != nil {
		logger.Debug("⚠️  Failed to read Claude projects directory: %v", err)
		return nil
	}

	var newestFile string
	var newestTime time.Time

	// Find the most recent JSONL file
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		// Extract session ID from filename (remove .jsonl extension)
		sessionID := strings.TrimSuffix(file.Name(), ".jsonl")

		// Validate that it looks like a UUID
		if len(sessionID) != 36 || strings.Count(sessionID, "-") != 4 {
			continue
		}

		filePath := filepath.Join(claudeProjectsDir, file.Name())
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		if fileInfo.ModTime().After(newestTime) {
			newestTime = fileInfo.ModTime()
			newestFile = filePath
		}
	}

	if newestFile == "" {
		return nil
	}

	// Extract session ID from filename
	sessionID := strings.TrimSuffix(filepath.Base(newestFile), ".jsonl")

	// Try to extract title from the JSONL file
	title := d.extractTitleFromJSONL(newestFile)

	fileInfo, _ := os.Stat(newestFile)
	return &ClaudeSessionInfo{
		SessionID:   sessionID,
		Title:       title,
		StartTime:   fileInfo.ModTime(), // Approximate start time
		LastUpdated: fileInfo.ModTime(),
	}
}

// extractTitleFromJSONL reads a Claude JSONL file and extracts the session title from summary records
// WARNING: This is unreliable as Claude doesn't always write summary records and the format may change.
// The "summary" field in JSONL is not guaranteed to contain the actual session title.
func (d *ClaudeSessionDetector) extractTitleFromJSONL(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	var lastTitle string
	scanner := bufio.NewScanner(file)

	// Read through the JSONL file to find summary records
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse the JSON line
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		// Look for summary records which contain the session title
		if eventType, ok := event["type"].(string); ok && eventType == "summary" {
			if summary, ok := event["summary"].(string); ok && summary != "" {
				lastTitle = summary
				// Continue reading to get the most recent summary if there are multiple
			}
		}
	}

	return lastTitle
}

// findClaudeProcess looks for Claude processes running with the worktree as working directory
func (d *ClaudeSessionDetector) findClaudeProcess() int {
	// Try using ps to find claude processes
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Look for lines containing "claude" command
		if !strings.Contains(line, "claude") || strings.Contains(line, "ps aux") {
			continue
		}

		// Parse the ps output to get PID
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		// Try to verify this process is in our worktree
		if d.isProcessInWorktree(pid) {
			return pid
		}
	}

	return 0
}

// isProcessInWorktree checks if a process is running in our worktree
func (d *ClaudeSessionDetector) isProcessInWorktree(pid int) bool {
	// Try to read the process's current working directory
	// On Linux: /proc/[pid]/cwd
	// On macOS: We need to use lsof or similar

	// Try lsof approach (works on both Linux and macOS)
	cmd := exec.Command("lsof", "-p", strconv.Itoa(pid), "-Fn")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Parse lsof output to find cwd
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "fcwd") && i+1 < len(lines) {
			// Next line should have the path
			pathLine := lines[i+1]
			if strings.HasPrefix(pathLine, "n") {
				path := strings.TrimPrefix(pathLine, "n")
				// Check if this path is within our worktree
				if strings.HasPrefix(path, d.workDir) {
					return true
				}
			}
		}
	}

	return false
}

// MonitorTitleChanges monitors a Claude session for new summary records (titles)
func (d *ClaudeSessionDetector) MonitorTitleChanges(sessionID string, titleChangedFunc func(string)) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}

	sessionFile := filepath.Join(d.workDir, ".claude", "projects", sessionID+".jsonl")

	// Start by getting the current title
	lastTitle := d.extractTitleFromJSONL(sessionFile)

	// Monitor for file changes using polling
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Check if file still exists (session might have ended)
		if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
			return fmt.Errorf("session file no longer exists")
		}

		// Re-extract title from the file
		currentTitle := d.extractTitleFromJSONL(sessionFile)
		if currentTitle != "" && currentTitle != lastTitle {
			lastTitle = currentTitle
			if titleChangedFunc != nil {
				titleChangedFunc(currentTitle)
			}
		}
	}

	return nil
}
