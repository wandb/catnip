package models

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// ClaudeSettings manages persistence of Claude and GitHub configuration files
type ClaudeSettings struct {
	ticker       *time.Ticker
	done         chan bool
	volumePath   string
	homePath     string
	lastModTimes map[string]time.Time
}

// NewClaudeSettings creates a new Claude settings manager
func NewClaudeSettings() *ClaudeSettings {
	return &ClaudeSettings{
		volumePath:   "/volume/.claude",
		homePath:     "/home/catnip",
		lastModTimes: make(map[string]time.Time),
		done:         make(chan bool),
	}
}

// Start begins the settings synchronization process
func (cs *ClaudeSettings) Start() {
	log.Println("🔧 Starting Claude settings persistence manager")
	
	// First, restore settings from volume if they exist
	cs.restoreFromVolume()
	
	// Start the ticker to watch for changes
	cs.ticker = time.NewTicker(5 * time.Second)
	go cs.watchForChanges()
}

// Stop stops the settings synchronization
func (cs *ClaudeSettings) Stop() {
	if cs.ticker != nil {
		cs.ticker.Stop()
	}
	cs.done <- true
}

// restoreFromVolume copies settings from volume to home directory on startup
func (cs *ClaudeSettings) restoreFromVolume() {
	// Create volume directory if it doesn't exist
	volumeClaudeDir := filepath.Join(cs.volumePath, ".claude")
	if err := os.MkdirAll(volumeClaudeDir, 0755); err != nil {
		log.Printf("❌ Failed to create volume claude directory: %v", err)
		return
	}
	
	// Fix permissions on volume directory (make it writable by catnip user)
	if err := os.Chown(volumeClaudeDir, 1000, 1000); err != nil {
		log.Printf("⚠️  Failed to chown volume directory (trying sudo): %v", err)
		// Continue anyway, we'll use sudo for writes if needed
	}
	
	// Files to restore
	files := []struct {
		filename string
		destPath string
	}{
		{".credentials.json", filepath.Join(cs.homePath, ".claude", ".credentials.json")},
		{"claude.json", filepath.Join(cs.homePath, ".claude.json")},
		{"gh_config.yml", filepath.Join(cs.homePath, ".config", "gh", "config.yml")},
		{"gh_hosts.yml", filepath.Join(cs.homePath, ".config", "gh", "hosts.yml")},
	}
	
	for _, file := range files {
		sourcePath := filepath.Join(volumeClaudeDir, file.filename)
		
		// Check if file exists in volume
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			continue
		}
		
		// Create destination directory if needed
		destDir := filepath.Dir(file.destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			log.Printf("❌ Failed to create directory %s: %v", destDir, err)
			continue
		}
		
		// Copy file from volume to home
		if err := cs.copyFile(sourcePath, file.destPath); err != nil {
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

// watchForChanges monitors Claude settings files for changes
func (cs *ClaudeSettings) watchForChanges() {
	for {
		select {
		case <-cs.ticker.C:
			cs.checkAndSyncFiles()
		case <-cs.done:
			return
		}
	}
}

// checkAndSyncFiles checks if files have changed and syncs them to volume
func (cs *ClaudeSettings) checkAndSyncFiles() {
	files := []struct {
		sourcePath string
		destName   string
	}{
		{filepath.Join(cs.homePath, ".claude", ".credentials.json"), ".credentials.json"},
		{filepath.Join(cs.homePath, ".claude.json"), "claude.json"},
		{filepath.Join(cs.homePath, ".config", "gh", "config.yml"), "gh_config.yml"},
		{filepath.Join(cs.homePath, ".config", "gh", "hosts.yml"), "gh_hosts.yml"},
	}
	
	volumeClaudeDir := filepath.Join(cs.volumePath, ".claude")
	
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
		lastMod, exists := cs.lastModTimes[file.sourcePath]
		if exists && info.ModTime().Equal(lastMod) {
			continue
		}
		
		// File has changed, copy to volume
		destPath := filepath.Join(volumeClaudeDir, file.destName)
		
		// Ensure volume directory exists
		if err := os.MkdirAll(volumeClaudeDir, 0755); err != nil {
			log.Printf("❌ Failed to create volume directory: %v", err)
			continue
		}
		
		if err := cs.copyFile(file.sourcePath, destPath); err != nil {
			log.Printf("❌ Failed to sync %s to volume: %v", file.sourcePath, err)
			continue
		}
		
		// Try to fix ownership (might fail if not root)
		os.Chown(destPath, 1000, 1000)
		
		cs.lastModTimes[file.sourcePath] = info.ModTime()
		log.Printf("📋 Synced %s to volume", file.destName)
	}
}

// copyFile copies a file from source to destination
func (cs *ClaudeSettings) copyFile(src, dst string) error {
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
func (cs *ClaudeSettings) ValidateSettings() error {
	files := []string{
		filepath.Join(cs.homePath, ".claude", ".credentials.json"),
		filepath.Join(cs.homePath, ".claude.json"),
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