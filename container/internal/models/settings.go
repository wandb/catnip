package models

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
)

// ClaudeConfig represents the structure of claude.json for validation
// NOTE: Authentication is in .credentials.json, NOT in claude.json
type ClaudeConfig struct {
	NumStartups           int                    `json:"numStartups,omitempty"`
	InstallMethod         string                 `json:"installMethod,omitempty"`
	AutoUpdates           bool                   `json:"autoUpdates,omitempty"`
	Theme                 string                 `json:"theme,omitempty"`
	CustomApiKeyResponses map[string]interface{} `json:"customApiKeyResponses,omitempty"`
	TipsHistory           map[string]interface{} `json:"tipsHistory,omitempty"`
	FirstStartTime        *string                `json:"firstStartTime,omitempty"`
	Projects              map[string]interface{} `json:"projects,omitempty"`
}

// Settings manages persistence of Claude and GitHub configuration files
type Settings struct {
	ticker              *time.Ticker
	done                chan bool
	volumePath          string
	homePath            string
	lastModTimes        map[string]time.Time
	debounceMap         map[string]*time.Timer // For debouncing file changes
	lastDirSync         map[string]time.Time   // For tracking directory sync times
	invalidConfigWarned map[string]time.Time   // Track when we last warned about invalid configs
	syncMutex           sync.Mutex             // Protects lastModTimes, debounceMap, lastDirSync, and invalidConfigWarned
}

// NewSettings creates a new settings manager
func NewSettings() *Settings {
	return &Settings{
		volumePath:          config.Runtime.VolumeDir,
		homePath:            config.Runtime.HomeDir,
		lastModTimes:        make(map[string]time.Time),
		debounceMap:         make(map[string]*time.Timer),
		lastDirSync:         make(map[string]time.Time),
		invalidConfigWarned: make(map[string]time.Time),
		done:                make(chan bool),
	}
}

// Start begins the settings synchronization process
func (s *Settings) Start() {
	logger.Info("🔧 Starting settings persistence manager")

	// ONLY restore settings from volume on boot - never during runtime
	logger.Info("📥 Boot-time restore: copying settings from volume to home directory")
	s.restoreFromVolumeOnBoot()

	// Restore IDE directory if it exists
	s.restoreIDEDirectory()

	// Restore Claude projects directory if it exists
	s.restoreClaudeProjectsDirectory()

	// Start the ticker to watch for changes
	s.ticker = time.NewTicker(5 * time.Second)
	go s.watchForChanges()
}

// Stop stops the settings synchronization
func (s *Settings) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}

	// Cancel all pending debounce timers
	s.syncMutex.Lock()
	for _, timer := range s.debounceMap {
		timer.Stop()
	}
	s.debounceMap = make(map[string]*time.Timer)
	s.syncMutex.Unlock()

	s.done <- true
}

// restoreFromVolumeOnBoot copies settings from volume to home directory ONLY on startup
func (s *Settings) restoreFromVolumeOnBoot() {
	// Create volume directories if they don't exist
	volumeClaudeDir := filepath.Join(s.volumePath, ".claude")
	volumeGitHubDir := filepath.Join(s.volumePath, ".github")

	for _, dir := range []string{volumeClaudeDir, volumeGitHubDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Errorf("❌ Failed to create volume directory %s: %v", dir, err)
			continue
		}

		// Fix permissions (make it writable by catnip user)
		if err := os.Chown(dir, 1000, 1000); err != nil {
			logger.Warnf("⚠️  Failed to chown volume directory %s: %v", dir, err)
		}
	}

	// Create nested directory for credentials
	volumeClaudeNestedDir := filepath.Join(volumeClaudeDir, ".claude")
	if err := os.MkdirAll(volumeClaudeNestedDir, 0755); err != nil {
		logger.Errorf("❌ Failed to create nested volume directory %s: %v", volumeClaudeNestedDir, err)
	} else {
		if err := os.Chown(volumeClaudeNestedDir, 1000, 1000); err != nil {
			logger.Warnf("⚠️  Failed to chown nested volume directory %s: %v", volumeClaudeNestedDir, err)
		}
	}

	// Create nested directory for Claude projects (session .jsonl files)
	volumeProjectsDir := filepath.Join(volumeClaudeNestedDir, "projects")
	if err := os.MkdirAll(volumeProjectsDir, 0755); err != nil {
		logger.Errorf("❌ Failed to create volume projects directory %s: %v", volumeProjectsDir, err)
	} else {
		if err := os.Chown(volumeProjectsDir, 1000, 1000); err != nil {
			logger.Warnf("⚠️  Failed to chown volume projects directory %s: %v", volumeProjectsDir, err)
		}
	}

	// Files to restore - only restore if home file doesn't exist
	files := []struct {
		volumePath string
		filename   string
		destPath   string
	}{
		{volumeClaudeNestedDir, ".credentials.json", filepath.Join(s.homePath, ".claude", ".credentials.json")},
		{volumeClaudeDir, "claude.json", filepath.Join(s.homePath, ".claude.json")},
		{volumeGitHubDir, "config.yml", filepath.Join(s.homePath, ".config", "gh", "config.yml")},
		{volumeGitHubDir, "hosts.yml", filepath.Join(s.homePath, ".config", "gh", "hosts.yml")},
	}

	for _, file := range files {
		sourcePath := filepath.Join(file.volumePath, file.filename)

		// Check if file exists in volume
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			continue
		}

		// Smart restore logic: prefer volume file if home file doesn't exist OR if volume file is more configured
		shouldRestore := false
		if _, err := os.Stat(file.destPath); os.IsNotExist(err) {
			// Home file doesn't exist - restore from volume
			shouldRestore = true
			logger.Debugf("🔄 Restoring %s - home file doesn't exist", file.filename)
		} else if file.filename == ".credentials.json" {
			// SAFETY: Never overwrite existing credentials (too risky)
			logger.Infof("🔒 SAFETY: Refusing to overwrite existing .credentials.json - too risky")
			shouldRestore = false
		} else if file.filename == "claude.json" {
			// EXTRA SAFETY: Check if we have working credentials before overwriting claude.json
			homeHasCredentials := s.hasWorkingCredentials(s.homePath)

			// NEVER overwrite claude.json if we have working credentials (too risky)
			if homeHasCredentials {
				logger.Infof("🔒 SAFETY: Refusing to overwrite claude.json - working credentials exist, too risky")
				shouldRestore = false
			} else if s.shouldPreferVolumeClaudeConfig(sourcePath, file.destPath) {
				shouldRestore = true
				// Create backup of existing home file before overwriting
				if err := s.backupHomeFile(file.destPath, file.volumePath, file.filename); err != nil {
					logger.Warnf("⚠️  Failed to backup existing %s: %v", file.filename, err)
				}
				logger.Infof("🔄 Restoring %s - volume file appears more configured (with safety checks passed)", file.filename)
			} else {
				logger.Debugf("⚪ Keeping existing %s - home file appears more or equally configured", file.filename)
			}
		} else {
			// For non-Claude files, keep existing home file
			logger.Debugf("⚪ Skipping restore of %s - file already exists in home directory", file.filename)
		}

		if !shouldRestore {
			continue
		}

		// Create destination directory if needed
		destDir := filepath.Dir(file.destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			logger.Errorf("❌ Failed to create directory %s: %v", destDir, err)
			continue
		}

		// Copy file from volume to home
		if err := s.copyFile(sourcePath, file.destPath); err != nil {
			logger.Errorf("❌ Failed to restore %s: %v", file.filename, err)
		} else {
			logger.Infof("✅ Restored %s from volume", file.filename)

			// Set proper ownership for catnip user
			if err := os.Chown(file.destPath, 1000, 1000); err != nil {
				logger.Warnf("⚠️  Failed to chown %s: %v", file.destPath, err)
			}
		}
	}
}

// restoreIDEDirectory copies the IDE directory from volume to home if it exists
func (s *Settings) restoreIDEDirectory() {
	volumeIDEDir := filepath.Join(s.volumePath, ".claude", "ide")
	homeIDEDir := filepath.Join(s.homePath, ".claude", "ide")

	// Check if volume IDE directory exists
	if _, err := os.Stat(volumeIDEDir); os.IsNotExist(err) {
		return
	}

	logger.Info("📁 Restoring IDE directory from volume")

	// Remove existing home IDE directory if it exists
	if err := os.RemoveAll(homeIDEDir); err != nil {
		logger.Warnf("⚠️  Failed to remove existing IDE directory: %v", err)
	}

	// Copy the entire directory
	if err := s.copyDirectory(volumeIDEDir, homeIDEDir); err != nil {
		logger.Errorf("❌ Failed to restore IDE directory: %v", err)
	} else {
		logger.Info("✅ Restored IDE directory from volume")
	}
}

// restoreClaudeProjectsDirectory copies the Claude projects directory from volume to home if it exists
func (s *Settings) restoreClaudeProjectsDirectory() {
	volumeProjectsDir := filepath.Join(s.volumePath, ".claude", ".claude", "projects")
	homeProjectsDir := filepath.Join(s.homePath, ".claude", "projects")

	// Check if volume projects directory exists
	if _, err := os.Stat(volumeProjectsDir); os.IsNotExist(err) {
		return
	}

	logger.Info("📂 Restoring Claude projects directory from volume")

	// Remove existing home projects directory if it exists
	if err := os.RemoveAll(homeProjectsDir); err != nil {
		logger.Warnf("⚠️  Failed to remove existing projects directory: %v", err)
	}

	// Copy the entire directory
	if err := s.copyDirectory(volumeProjectsDir, homeProjectsDir); err != nil {
		logger.Errorf("❌ Failed to restore Claude projects directory: %v", err)
	} else {
		logger.Info("✅ Restored Claude projects directory from volume")
	}
}

// copyDirectory recursively copies a directory from src to dst
func (s *Settings) copyDirectory(src, dst string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Set ownership
	if err := os.Chown(dst, 1000, 1000); err != nil {
		logger.Warnf("⚠️  Failed to chown directory %s: %v", dst, err)
	}

	// Read source directory
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := s.copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Special handling for .lock files - remove PID key
			if strings.HasSuffix(entry.Name(), ".lock") {
				if err := s.copyLockFile(srcPath, dstPath); err != nil {
					return err
				}
			} else {
				// Copy regular file
				if err := s.copyFile(srcPath, dstPath); err != nil {
					return err
				}
			}
			// Set ownership for file
			if err := os.Chown(dstPath, 1000, 1000); err != nil {
				logger.Warnf("⚠️  Failed to chown file %s: %v", dstPath, err)
			}
		}
	}

	return nil
}

// copyLockFile copies a lock file while removing the PID key from the JSON
func (s *Settings) copyLockFile(src, dst string) error {
	// Read the source file
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Parse JSON
	var lockData map[string]interface{}
	if err := json.Unmarshal(data, &lockData); err != nil {
		// If it's not valid JSON, just copy as-is
		logger.Warnf("⚠️  Lock file %s is not valid JSON, copying as-is: %v", src, err)
		return s.copyFile(src, dst)
	}

	// Remove the PID key if it exists
	if _, exists := lockData["pid"]; exists {
		delete(lockData, "pid")
		logger.Debugf("🔧 Removed PID from lock file %s", filepath.Base(src))
	}

	// Map workspace folders to container paths
	if workspaceFolders, exists := lockData["workspaceFolders"]; exists {
		if folders, ok := workspaceFolders.([]interface{}); ok {
			for i, folder := range folders {
				if folderPath, ok := folder.(string); ok {
					if mappedPath := s.mapWorkspacePath(folderPath); mappedPath != folderPath {
						folders[i] = mappedPath
						logger.Debugf("🔧 Mapped workspace path %s -> %s", folderPath, mappedPath)
					}
				}
			}
		}
	}

	// Marshal back to JSON with indentation
	modifiedData, err := json.MarshalIndent(lockData, "", "  ")
	if err != nil {
		return err
	}

	// Write to destination
	if err := os.WriteFile(dst, modifiedData, 0644); err != nil {
		return err
	}

	return nil
}

// mapWorkspacePath maps host workspace paths to container workspace paths
func (s *Settings) mapWorkspacePath(hostPath string) string {
	// Get the final component of the path
	baseName := filepath.Base(hostPath)
	containerWorkspacePath := filepath.Join(config.Runtime.WorkspaceDir, baseName)

	// Check if this workspace exists in the container
	if _, err := os.Stat(containerWorkspacePath); err == nil {
		return containerWorkspacePath
	}

	// If no matching workspace found, return the original path
	return hostPath
}

// watchForChanges monitors settings files for changes
func (s *Settings) watchForChanges() {
	for {
		select {
		case <-s.ticker.C:
			s.checkAndSyncFiles()
		case <-s.done:
			return
		}
	}
}

// checkAndSyncClaudeProjects monitors the Claude projects directory for changes and syncs .jsonl files
func (s *Settings) checkAndSyncClaudeProjects() {
	homeProjectsDir := filepath.Join(s.homePath, ".claude", "projects")
	volumeProjectsDir := filepath.Join(s.volumePath, ".claude", ".claude", "projects")

	// Check if home projects directory exists
	if _, err := os.Stat(homeProjectsDir); os.IsNotExist(err) {
		return
	}

	// Check if it's time to sync (throttle to every 10 seconds for directory scanning)
	lastSync, exists := s.lastDirSync[homeProjectsDir]
	if exists && time.Since(lastSync) < 10*time.Second {
		return
	}

	// Walk through the projects directory and sync .jsonl files that have changed
	err := filepath.Walk(homeProjectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking even if there's an error with one file
		}

		// Only process .jsonl files
		if info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Check if file has been modified since last sync
		lastMod, exists := s.lastModTimes[path]
		if exists && info.ModTime().Equal(lastMod) {
			return nil
		}

		// Calculate relative path from projects directory
		relPath, err := filepath.Rel(homeProjectsDir, path)
		if err != nil {
			logger.Errorf("❌ Error calculating relative path for %s: %v", path, err)
			return nil
		}

		// Schedule sync for this .jsonl file
		volumeFilePath := filepath.Join(volumeProjectsDir, relPath)
		s.scheduleClaudeProjectFileSync(path, volumeFilePath, info.ModTime())

		return nil
	})

	if err != nil {
		logger.Errorf("❌ Error walking Claude projects directory: %v", err)
	}

	// Update last sync time for directory
	s.lastDirSync[homeProjectsDir] = time.Now()
}

// scheduleClaudeProjectFileSync schedules a debounced sync for a Claude project file
func (s *Settings) scheduleClaudeProjectFileSync(sourcePath, destPath string, modTime time.Time) {
	// Cancel existing timer if it exists
	if timer, exists := s.debounceMap[sourcePath]; exists {
		timer.Stop()
	}

	// Create new debounced timer (shorter delay for session files)
	debounceDelay := 1 * time.Second

	s.debounceMap[sourcePath] = time.AfterFunc(debounceDelay, func() {
		s.performClaudeProjectFileSync(sourcePath, destPath, modTime)
	})
}

// performClaudeProjectFileSync performs the actual sync of a Claude project file
func (s *Settings) performClaudeProjectFileSync(sourcePath, destPath string, expectedModTime time.Time) {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()

	// Double-check the file still exists and hasn't changed again
	info, err := os.Stat(sourcePath)
	if os.IsNotExist(err) {
		logger.Debugf("⚠️  File %s no longer exists, skipping sync", sourcePath)
		return
	}
	if err != nil {
		logger.Errorf("❌ Error re-checking file %s: %v", sourcePath, err)
		return
	}

	// If file has been modified again since we scheduled this sync, skip it
	if !info.ModTime().Equal(expectedModTime) {
		logger.Debugf("⚠️  File %s was modified again, skipping this sync", sourcePath)
		return
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		logger.Errorf("❌ Failed to create directory for %s: %v", destPath, err)
		return
	}

	// Copy the file
	if err := s.copyFile(sourcePath, destPath); err != nil {
		logger.Errorf("❌ Failed to sync Claude project file %s: %v", filepath.Base(sourcePath), err)
		return
	}

	// Try to fix ownership
	_ = os.Chown(destPath, 1000, 1000)

	// Update last modification time
	s.lastModTimes[sourcePath] = info.ModTime()

	// Silent sync - no log per file to reduce noise
}

// checkAndSyncFiles checks if files have changed and schedules debounced syncing to volume
func (s *Settings) checkAndSyncFiles() {
	files := []struct {
		sourcePath string
		volumeDir  string
		destName   string
		sensitive  bool // True for files that need extra care (like ~/.claude.json)
	}{
		{filepath.Join(s.homePath, ".claude", ".credentials.json"), filepath.Join(s.volumePath, ".claude", ".claude"), ".credentials.json", true},
		{filepath.Join(s.homePath, ".claude.json"), filepath.Join(s.volumePath, ".claude"), "claude.json", true},
		{filepath.Join(s.homePath, ".config", "gh", "config.yml"), filepath.Join(s.volumePath, ".github"), "config.yml", false},
		{filepath.Join(s.homePath, ".config", "gh", "hosts.yml"), filepath.Join(s.volumePath, ".github"), "hosts.yml", false},
	}

	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()

	// Check individual files
	for _, file := range files {
		// Check if source file exists
		info, err := os.Stat(file.sourcePath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			logger.Errorf("❌ Error checking file %s: %v", file.sourcePath, err)
			continue
		}

		// Check if file has been modified
		lastMod, exists := s.lastModTimes[file.sourcePath]
		if exists && info.ModTime().Equal(lastMod) {
			continue
		}

		// For sensitive files, check if this is a valid/configured file before syncing
		if file.sensitive && file.destName == "claude.json" {
			if !s.isClaudeConfigValid(file.sourcePath) {
				// Only warn every 5 minutes to reduce noise
				s.syncMutex.Lock()
				lastWarned, warned := s.invalidConfigWarned[file.sourcePath]
				shouldWarn := !warned || time.Since(lastWarned) > 5*time.Minute
				if shouldWarn {
					s.invalidConfigWarned[file.sourcePath] = time.Now()
				}
				s.syncMutex.Unlock()

				if shouldWarn {
					logger.Warnf("⚠️  Skipping sync of %s - appears to be unconfigured or invalid", file.sourcePath)
				}
				continue
			}
			// Config is valid now, clear any previous warning
			s.syncMutex.Lock()
			delete(s.invalidConfigWarned, file.sourcePath)
			s.syncMutex.Unlock()
		}

		// File has changed - schedule debounced sync
		s.scheduleDebounceSync(file.sourcePath, file.volumeDir, file.destName, file.sensitive, info.ModTime())
	}

	// Check Claude projects directory for changes
	s.checkAndSyncClaudeProjects()
}

// scheduleDebounceSync schedules a debounced sync operation for a file
func (s *Settings) scheduleDebounceSync(sourcePath, volumeDir, destName string, sensitive bool, modTime time.Time) {
	// Cancel existing timer if it exists
	if timer, exists := s.debounceMap[sourcePath]; exists {
		timer.Stop()
	}

	// Create new debounced timer
	debounceDelay := 2 * time.Second
	if sensitive {
		debounceDelay = 5 * time.Second // Extra delay for sensitive files
	}

	s.debounceMap[sourcePath] = time.AfterFunc(debounceDelay, func() {
		s.performSafeSync(sourcePath, volumeDir, destName, sensitive, modTime)
	})
}

// performSafeSync performs the actual file sync with proper locking and validation
func (s *Settings) performSafeSync(sourcePath, volumeDir, destName string, sensitive bool, expectedModTime time.Time) {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()

	// Double-check the file still exists and hasn't changed again
	info, err := os.Stat(sourcePath)
	if os.IsNotExist(err) {
		logger.Debugf("⚠️  File %s no longer exists, skipping sync", sourcePath)
		return
	}
	if err != nil {
		logger.Errorf("❌ Error re-checking file %s: %v", sourcePath, err)
		return
	}

	// If file has been modified again since we scheduled this sync, skip it
	if !info.ModTime().Equal(expectedModTime) {
		logger.Debugf("⚠️  File %s was modified again, skipping this sync", sourcePath)
		return
	}

	// For sensitive files, check for potential lock files or concurrent access
	if sensitive {
		if s.isFileBeingAccessed(sourcePath) {
			logger.Debugf("⚠️  File %s appears to be in use, deferring sync", sourcePath)
			// Reschedule for later
			time.AfterFunc(10*time.Second, func() {
				s.performSafeSync(sourcePath, volumeDir, destName, sensitive, expectedModTime)
			})
			return
		}
	}

	destPath := filepath.Join(volumeDir, destName)

	// Ensure volume directory exists
	if err := os.MkdirAll(volumeDir, 0755); err != nil {
		logger.Errorf("❌ Failed to create volume directory: %v", err)
		return
	}

	// Perform the sync with atomic write for sensitive files
	if sensitive {
		err = s.copyFileAtomic(sourcePath, destPath)
	} else {
		err = s.copyFile(sourcePath, destPath)
	}

	if err != nil {
		logger.Errorf("❌ Failed to sync %s to volume: %v", sourcePath, err)
		return
	}

	// Try to fix ownership (might fail if not root)
	_ = os.Chown(destPath, 1000, 1000)

	// Only log sync for non-routine files or first sync
	if _, exists := s.lastModTimes[sourcePath]; !exists || (!strings.Contains(destName, ".json") && !strings.Contains(destName, ".yml")) {
		logger.Debugf("📋 Synced %s to volume", destName)
	}
	s.lastModTimes[sourcePath] = info.ModTime()
}

// isFileBeingAccessed checks if a file might be currently being written to
func (s *Settings) isFileBeingAccessed(filePath string) bool {
	// Check for common lock file patterns that Claude might use
	lockPatterns := []string{
		filePath + ".lock",
		filePath + ".tmp",
		filepath.Dir(filePath) + "/.lock",
		filepath.Dir(filePath) + "/lock",
	}

	for _, lockPath := range lockPatterns {
		if _, err := os.Stat(lockPath); err == nil {
			logger.Debugf("🔒 Lock file detected: %s", lockPath)
			return true
		}
	}

	// Try to open the file exclusively to see if it's in use
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		// If we can't open it, assume it's in use
		return true
	}
	file.Close()

	return false
}

// copyFileAtomic copies a file atomically by writing to a temp file first
func (s *Settings) copyFileAtomic(src, dst string) error {
	// Create temp file in same directory as destination
	tempFile := dst + ".tmp." + filepath.Base(src)

	// Copy to temp file first
	if err := s.copyFile(src, tempFile); err != nil {
		return err
	}

	// Atomically rename temp file to final destination
	if err := os.Rename(tempFile, dst); err != nil {
		os.Remove(tempFile) // Clean up temp file on error
		return err
	}

	return nil
}

// copyFile copies a file from source to destination
func (s *Settings) copyFile(src, dst string) error {
	// Open source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Get source file stats to preserve permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	// Create destination file
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy contents
	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Try to preserve permissions
	if err := destFile.Chmod(sourceInfo.Mode()); err != nil {
		// Log but don't fail
		logger.Warnf("⚠️  Could not preserve permissions on %s: %v", dst, err)
	}

	return nil
}

// ValidateSettings checks if the Claude settings files contain valid JSON
func (s *Settings) ValidateSettings() error {
	files := []string{
		filepath.Join(s.homePath, ".claude", ".credentials.json"),
		filepath.Join(s.homePath, ".claude.json"),
	}

	for _, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		var js json.RawMessage
		if err := json.Unmarshal(data, &js); err != nil {
			logger.Warnf("⚠️  Invalid JSON in %s: %v", file, err)
			return err
		}
	}

	return nil
}

// shouldPreferVolumeClaudeConfig compares two claude.json files and determines if the volume file should be preferred
func (s *Settings) shouldPreferVolumeClaudeConfig(volumePath, homePath string) bool {
	volumeConfig := s.parseClaudeConfig(volumePath)
	homeConfig := s.parseClaudeConfig(homePath)

	// If either file is invalid, prefer the valid one
	if volumeConfig == nil && homeConfig != nil {
		return false // Keep home
	}
	if volumeConfig != nil && homeConfig == nil {
		return true // Use volume
	}
	if volumeConfig == nil && homeConfig == nil {
		return false // Both invalid, keep current
	}

	// ULTRA-CONSERVATIVE LOGIC: Check for working credentials first
	homeHasCredentials := s.hasWorkingCredentials(s.homePath)

	// If home has working credentials, NEVER overwrite (too dangerous)
	if homeHasCredentials {
		logger.Infof("🔒 Keeping home claude.json - working credentials exist")
		return false
	}

	// Compare configuration levels only if no credentials at risk
	volumeConfigLevel := s.getClaudeConfigLevel(volumeConfig)
	homeConfigLevel := s.getClaudeConfigLevel(homeConfig)

	logger.Debugf("📊 Claude config comparison - Volume: %d, Home: %d",
		volumeConfigLevel, homeConfigLevel)

	// Be extremely conservative: require significantly higher score to overwrite
	// Only overwrite if volume score is at least 100 points higher
	return volumeConfigLevel > (homeConfigLevel + 100)
}

// hasAuthentication checks if authentication exists by checking .credentials.json
// NOTE: Authentication is stored separately in .credentials.json, not in claude.json
func (s *Settings) hasAuthentication(config *ClaudeConfig) bool {
	// Since authentication is not in claude.json, we need to check .credentials.json
	// Extract the directory from the claude.json path and look for .credentials.json
	// For now, we'll use a more conservative approach - if the config file exists and is parseable,
	// we assume it's a valid config. Authentication check should be done separately.
	return config != nil
}

// hasWorkingCredentials checks if .credentials.json exists and has valid OAuth tokens
func (s *Settings) hasWorkingCredentials(homeDir string) bool {
	credentialsPath := filepath.Join(homeDir, ".claude", ".credentials.json")
	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		return false
	}

	var credentials map[string]interface{}
	if err := json.Unmarshal(data, &credentials); err != nil {
		return false
	}

	// Check for OAuth structure
	if oauth, exists := credentials["claudeAiOauth"]; exists {
		if oauthMap, ok := oauth.(map[string]interface{}); ok {
			// Check for access token
			if accessToken, exists := oauthMap["accessToken"]; exists {
				if tokenStr, ok := accessToken.(string); ok && tokenStr != "" {
					return true
				}
			}
		}
	}

	return false
}

// parseClaudeConfig safely parses a claude.json file
func (s *Settings) parseClaudeConfig(filePath string) *ClaudeConfig {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var config ClaudeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}

	return &config
}

// getClaudeConfigLevel returns a score indicating how "configured" a Claude config is
func (s *Settings) getClaudeConfigLevel(config *ClaudeConfig) int {
	score := 0

	// Basic configuration score based on actual claude.json structure
	if config.NumStartups > 0 {
		score += 5 // Basic usage indicator
	}

	if config.Theme != "" {
		score += 5 // Has theme preference
	}

	if config.TipsHistory != nil && len(config.TipsHistory) > 0 {
		score += 10 // Has usage history
	}

	// Has projects with actual configuration
	if config.Projects != nil {
		for _, project := range config.Projects {
			if projectMap, ok := project.(map[string]interface{}); ok {
				// Check for configured tools, MCP servers, etc.
				if tools, exists := projectMap["allowedTools"]; exists {
					if toolsArray, ok := tools.([]interface{}); ok && len(toolsArray) > 0 {
						score += 20
					}
				}
				if history, exists := projectMap["history"]; exists {
					if historyArray, ok := history.([]interface{}); ok && len(historyArray) > 0 {
						score += 30
					}
				}
				if mcpServers, exists := projectMap["mcpServers"]; exists {
					if serversMap, ok := mcpServers.(map[string]interface{}); ok && len(serversMap) > 0 {
						score += 25
					}
				}
				if trusted, exists := projectMap["hasTrustDialogAccepted"]; exists {
					if trustBool, ok := trusted.(bool); ok && trustBool {
						score += 15
					}
				}
			}
		}
	}

	// Penalize very recent firstStartTime (likely fresh installation)
	if config.FirstStartTime != nil {
		if startTime, err := time.Parse(time.RFC3339, *config.FirstStartTime); err == nil {
			if time.Since(startTime) < 1*time.Hour {
				score -= 50 // Heavy penalty for very recent start times
				logger.Debugf("🕒 Recent firstStartTime detected: %s (penalty applied)", *config.FirstStartTime)
			}
		}
	}

	return score
}

// isClaudeConfigValid checks if a claude.json file is valid and appears configured
func (s *Settings) isClaudeConfigValid(filePath string) bool {
	config := s.parseClaudeConfig(filePath)
	if config == nil {
		return false
	}

	// Check for signs of a fresh/unconfigured installation
	configLevel := s.getClaudeConfigLevel(config)

	// If config level is too low (likely fresh installation), don't sync it
	if configLevel < 5 {
		logger.Debugf("🚫 Claude config appears unconfigured (score: %d)", configLevel)
		return false
	}

	return true
}

// backupHomeFile creates a timestamped backup of a home file in the volume directory
func (s *Settings) backupHomeFile(homeFilePath, volumeDir, filename string) error {
	// Generate timestamped backup filename
	timestamp := time.Now().Format("20060102-150405")
	backupFilename := fmt.Sprintf("%s.backup.%s", filename, timestamp)
	backupPath := filepath.Join(volumeDir, backupFilename)

	// Create backup directory if needed
	if err := os.MkdirAll(volumeDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %v", err)
	}

	// Copy the home file to backup location
	if err := s.copyFile(homeFilePath, backupPath); err != nil {
		return fmt.Errorf("failed to copy file to backup: %v", err)
	}

	// Set proper ownership
	if err := os.Chown(backupPath, 1000, 1000); err != nil {
		logger.Warnf("⚠️  Failed to chown backup file %s: %v", backupPath, err)
	}

	logger.Infof("💾 Created backup: %s", backupFilename)
	return nil
}
