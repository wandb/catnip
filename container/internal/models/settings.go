package models

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Settings manages persistence of Claude and GitHub configuration files
type Settings struct {
	ticker       *time.Ticker
	done         chan bool
	volumePath   string
	homePath     string
	lastModTimes map[string]time.Time
	debounceMap  map[string]*time.Timer // For debouncing file changes
	syncMutex    sync.Mutex              // Protects lastModTimes and debounceMap
}

// NewSettings creates a new settings manager
func NewSettings() *Settings {
	return &Settings{
		volumePath:   "/volume",
		homePath:     "/home/catnip",
		lastModTimes: make(map[string]time.Time),
		debounceMap:  make(map[string]*time.Timer),
		done:         make(chan bool),
	}
}

// Start begins the settings synchronization process
func (s *Settings) Start() {
	log.Println("🔧 Starting settings persistence manager")
	
	// ONLY restore settings from volume on boot - never during runtime
	log.Println("📥 Boot-time restore: copying settings from volume to home directory")
	s.restoreFromVolumeOnBoot()
	
	// Restore IDE directory if it exists
	s.restoreIDEDirectory()
	
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
			log.Printf("❌ Failed to create volume directory %s: %v", dir, err)
			continue
		}
		
		// Fix permissions (make it writable by catnip user)
		if err := os.Chown(dir, 1000, 1000); err != nil {
			log.Printf("⚠️  Failed to chown volume directory %s: %v", dir, err)
		}
	}
	
	// Create nested directory for credentials
	volumeClaudeNestedDir := filepath.Join(volumeClaudeDir, ".claude")
	if err := os.MkdirAll(volumeClaudeNestedDir, 0755); err != nil {
		log.Printf("❌ Failed to create nested volume directory %s: %v", volumeClaudeNestedDir, err)
	} else {
		if err := os.Chown(volumeClaudeNestedDir, 1000, 1000); err != nil {
			log.Printf("⚠️  Failed to chown nested volume directory %s: %v", volumeClaudeNestedDir, err)
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
		
		// Check if destination file already exists - if so, skip (boot-time only restore)
		if _, err := os.Stat(file.destPath); err == nil {
			log.Printf("⚪ Skipping restore of %s - file already exists in home directory", file.filename)
			continue
		}
		
		// Create destination directory if needed
		destDir := filepath.Dir(file.destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			log.Printf("❌ Failed to create directory %s: %v", destDir, err)
			continue
		}
		
		// Copy file from volume to home
		if err := s.copyFile(sourcePath, file.destPath); err != nil {
			log.Printf("❌ Failed to restore %s: %v", file.filename, err)
		} else {
			log.Printf("✅ Restored %s from volume", file.filename)
			
			// Set proper ownership for catnip user
			if err := os.Chown(file.destPath, 1000, 1000); err != nil {
				log.Printf("⚠️  Failed to chown %s: %v", file.destPath, err)
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
	
	log.Printf("📁 Restoring IDE directory from volume")
	
	// Remove existing home IDE directory if it exists
	if err := os.RemoveAll(homeIDEDir); err != nil {
		log.Printf("⚠️  Failed to remove existing IDE directory: %v", err)
	}
	
	// Copy the entire directory
	if err := s.copyDirectory(volumeIDEDir, homeIDEDir); err != nil {
		log.Printf("❌ Failed to restore IDE directory: %v", err)
	} else {
		log.Printf("✅ Restored IDE directory from volume")
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
		log.Printf("⚠️  Failed to chown directory %s: %v", dst, err)
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
				log.Printf("⚠️  Failed to chown file %s: %v", dstPath, err)
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
		log.Printf("⚠️  Lock file %s is not valid JSON, copying as-is: %v", src, err)
		return s.copyFile(src, dst)
	}
	
	// Remove the PID key if it exists
	if _, exists := lockData["pid"]; exists {
		delete(lockData, "pid")
		log.Printf("🔧 Removed PID from lock file %s", filepath.Base(src))
	}
	
	// Map workspace folders to container paths
	if workspaceFolders, exists := lockData["workspaceFolders"]; exists {
		if folders, ok := workspaceFolders.([]interface{}); ok {
			for i, folder := range folders {
				if folderPath, ok := folder.(string); ok {
					if mappedPath := s.mapWorkspacePath(folderPath); mappedPath != folderPath {
						folders[i] = mappedPath
						log.Printf("🔧 Mapped workspace path %s -> %s", folderPath, mappedPath)
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
	containerWorkspacePath := filepath.Join("/workspace", baseName)
	
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
	
	for _, file := range files {
		// Check if source file exists
		info, err := os.Stat(file.sourcePath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			log.Printf("❌ Error checking file %s: %v", file.sourcePath, err)
			continue
		}
		
		// Check if file has been modified
		lastMod, exists := s.lastModTimes[file.sourcePath]
		if exists && info.ModTime().Equal(lastMod) {
			continue
		}
		
		// File has changed - schedule debounced sync
		s.scheduleDebounceSync(file.sourcePath, file.volumeDir, file.destName, file.sensitive, info.ModTime())
	}
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
		log.Printf("⚠️  File %s no longer exists, skipping sync", sourcePath)
		return
	}
	if err != nil {
		log.Printf("❌ Error re-checking file %s: %v", sourcePath, err)
		return
	}
	
	// If file has been modified again since we scheduled this sync, skip it
	if !info.ModTime().Equal(expectedModTime) {
		log.Printf("⚠️  File %s was modified again, skipping this sync", sourcePath)
		return
	}
	
	// For sensitive files, check for potential lock files or concurrent access
	if sensitive {
		if s.isFileBeingAccessed(sourcePath) {
			log.Printf("⚠️  File %s appears to be in use, deferring sync", sourcePath)
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
		log.Printf("❌ Failed to create volume directory: %v", err)
		return
	}
	
	// Perform the sync with atomic write for sensitive files
	if sensitive {
		err = s.copyFileAtomic(sourcePath, destPath)
	} else {
		err = s.copyFile(sourcePath, destPath)
	}
	
	if err != nil {
		log.Printf("❌ Failed to sync %s to volume: %v", sourcePath, err)
		return
	}
	
	// Try to fix ownership (might fail if not root)
	_ = os.Chown(destPath, 1000, 1000)
	
	s.lastModTimes[sourcePath] = info.ModTime()
	log.Printf("📋 Synced %s to volume", destName)
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
			log.Printf("🔒 Lock file detected: %s", lockPath)
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
		log.Printf("⚠️  Could not preserve permissions on %s: %v", dst, err)
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
			log.Printf("⚠️  Invalid JSON in %s: %v", file, err)
			return err
		}
	}
	
	return nil
}