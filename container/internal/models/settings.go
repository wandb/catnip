package models

import (
	"encoding/json"
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
	NumStartups           int                     `json:"numStartups,omitempty"`
	InstallMethod         string                  `json:"installMethod,omitempty"`
	AutoUpdates           bool                    `json:"autoUpdates,omitempty"`
	Theme                 string                  `json:"theme,omitempty"`
	CustomAPIKeyResponses map[string]interface{}  `json:"customApiKeyResponses,omitempty"`
	TipsHistory           map[string]interface{}  `json:"tipsHistory,omitempty"`
	FirstStartTime        *string                 `json:"firstStartTime,omitempty"`
	Projects              map[string]interface{}  `json:"projects,omitempty"`
	OauthAccount          *map[string]interface{} `json:"oauthAccount,omitempty"`
}

// Settings manages persistence of Claude and GitHub configuration files
type Settings struct {
	ticker              *time.Ticker
	done                chan bool
	volumePath          string
	homePath            string
	claudeConfigDir     string // Claude config directory (respects XDG_CONFIG_HOME on Linux)
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
		claudeConfigDir:     config.Runtime.ClaudeConfigDir,
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
		{volumeClaudeNestedDir, ".credentials.json", filepath.Join(s.claudeConfigDir, ".credentials.json")},
		{volumeClaudeDir, "claude.json", filepath.Join(s.homePath, ".claude.json")},
		{volumeClaudeNestedDir, "history.jsonl", filepath.Join(s.claudeConfigDir, "history.jsonl")},
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
			// Never overwrite existing credentials - if it exists, leave it alone
			logger.Debugf("🔒 Skipping .credentials.json - file already exists")
			shouldRestore = false
		} else if file.filename == "claude.json" {
			// Check if home claude.json has oauthAccount configured
			if s.shouldPreferVolumeClaudeConfig(sourcePath, file.destPath) {
				shouldRestore = true
				logger.Debugf("🔄 Restoring %s - home config has no oauthAccount", file.filename)
			} else {
				logger.Debugf("⚪ Keeping existing %s - oauthAccount is configured", file.filename)
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
	homeIDEDir := filepath.Join(s.claudeConfigDir, "ide")

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
	homeProjectsDir := filepath.Join(s.claudeConfigDir, "projects")

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
	homeProjectsDir := filepath.Join(s.claudeConfigDir, "projects")
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
		{filepath.Join(s.claudeConfigDir, ".credentials.json"), filepath.Join(s.volumePath, ".claude", ".claude"), ".credentials.json", true},
		{filepath.Join(s.homePath, ".claude.json"), filepath.Join(s.volumePath, ".claude"), "claude.json", true},
		{filepath.Join(s.claudeConfigDir, "history.jsonl"), filepath.Join(s.volumePath, ".claude", ".claude"), "history.jsonl", false},
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

		// Debug log for credentials file changes
		if file.destName == ".credentials.json" {
			logger.Debugf("🔍 Credentials file changed: %s (last: %v, current: %v)", file.sourcePath, lastMod, info.ModTime())
		}

		// For sensitive files, check if this is a valid/configured file before syncing
		if file.sensitive && file.destName == "claude.json" {
			if !s.isClaudeConfigValid(file.sourcePath) {
				// Only warn every 5 minutes to reduce noise (mutex already held)
				lastWarned, warned := s.invalidConfigWarned[file.sourcePath]
				shouldWarn := !warned || time.Since(lastWarned) > 5*time.Minute
				if shouldWarn {
					s.invalidConfigWarned[file.sourcePath] = time.Now()
				}

				if shouldWarn {
					logger.Warnf("⚠️  Skipping sync of %s - appears to be unconfigured or invalid", file.sourcePath)
				}
				continue
			}
			// Config is valid now, clear any previous warning (mutex already held)
			delete(s.invalidConfigWarned, file.sourcePath)
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

	// Always log sync for credentials, debug for others
	if destName == ".credentials.json" {
		logger.Infof("📋 Synced credentials to volume")
	} else if _, exists := s.lastModTimes[sourcePath]; !exists || (!strings.Contains(destName, ".json") && !strings.Contains(destName, ".yml")) {
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
		filepath.Join(s.claudeConfigDir, ".credentials.json"),
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

// shouldPreferVolumeClaudeConfig checks if volume claude.json should be used over home version
func (s *Settings) shouldPreferVolumeClaudeConfig(volumePath, homePath string) bool {
	homeConfig := s.parseClaudeConfig(homePath)

	// If home config doesn't exist, use volume
	if homeConfig == nil {
		return true
	}

	// If home config has oauthAccount set, don't overwrite
	if homeConfig.OauthAccount != nil {
		logger.Debugf("🔒 Keeping home claude.json - oauthAccount is configured")
		return false
	}

	// Home config exists but no oauthAccount - safe to use volume
	logger.Debugf("🔄 Using volume claude.json - home config has no oauthAccount")
	return true
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

// isClaudeConfigValid checks if a claude.json file is valid JSON
func (s *Settings) isClaudeConfigValid(filePath string) bool {
	config := s.parseClaudeConfig(filePath)
	return config != nil
}
