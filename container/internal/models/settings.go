package models

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Settings manages persistence of Claude and GitHub configuration files
type Settings struct {
	ticker       *time.Ticker
	done         chan bool
	volumePath   string
	homePath     string
	lastModTimes map[string]time.Time
}

// NewSettings creates a new settings manager
func NewSettings() *Settings {
	return &Settings{
		volumePath:   "/volume",
		homePath:     "/home/catnip",
		lastModTimes: make(map[string]time.Time),
		done:         make(chan bool),
	}
}

// Start begins the settings synchronization process
func (s *Settings) Start() {
	log.Println("🔧 Starting settings persistence manager")
	
	// First, restore settings from volume if they exist
	s.restoreFromVolume()
	
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
	s.done <- true
}

// restoreFromVolume copies settings from volume to home directory on startup
func (s *Settings) restoreFromVolume() {
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
		
		// Check if destination file already exists - if so, don't overwrite
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

// checkAndSyncFiles checks if files have changed and syncs them to volume
func (s *Settings) checkAndSyncFiles() {
	files := []struct {
		sourcePath string
		volumeDir  string
		destName   string
	}{
		{filepath.Join(s.homePath, ".claude", ".credentials.json"), filepath.Join(s.volumePath, ".claude", ".claude"), ".credentials.json"},
		{filepath.Join(s.homePath, ".claude.json"), filepath.Join(s.volumePath, ".claude"), "claude.json"},
		{filepath.Join(s.homePath, ".config", "gh", "config.yml"), filepath.Join(s.volumePath, ".github"), "config.yml"},
		{filepath.Join(s.homePath, ".config", "gh", "hosts.yml"), filepath.Join(s.volumePath, ".github"), "hosts.yml"},
	}
	
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
		
		// File has changed, copy to volume
		destPath := filepath.Join(file.volumeDir, file.destName)
		
		// Ensure volume directory exists
		if err := os.MkdirAll(file.volumeDir, 0755); err != nil {
			log.Printf("❌ Failed to create volume directory: %v", err)
			continue
		}
		
		if err := s.copyFile(file.sourcePath, destPath); err != nil {
			log.Printf("❌ Failed to sync %s to volume: %v", file.sourcePath, err)
			continue
		}
		
		// Try to fix ownership (might fail if not root)
		os.Chown(destPath, 1000, 1000)
		
		s.lastModTimes[file.sourcePath] = info.ModTime()
		log.Printf("📋 Synced %s to volume", file.destName)
	}
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