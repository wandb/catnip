package config

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RuntimeMode represents the execution environment
type RuntimeMode string

const (
	// DockerMode indicates running inside a Docker container
	DockerMode RuntimeMode = "docker"
	// NativeMode indicates running on the host system
	NativeMode RuntimeMode = "native"
)

// RuntimeConfig holds configuration for different runtime environments
type RuntimeConfig struct {
	Mode               RuntimeMode
	WorkspaceDir       string
	VolumeDir          string
	LiveDir            string
	HomeDir            string
	TempDir            string
	CurrentRepo        string // For native mode, the git repo we're running from
	SyncEnabled        bool   // Whether to sync settings to volume
	PortMonitorEnabled bool   // Whether to use /proc for port monitoring
}

var (
	// Runtime is the global runtime configuration instance
	Runtime *RuntimeConfig
)

func init() {
	Runtime = DetectRuntime()
}

// DetectRuntime determines the current runtime environment and returns appropriate configuration
func DetectRuntime() *RuntimeConfig {
	mode := detectMode()

	config := &RuntimeConfig{
		Mode: mode,
	}

	switch mode {
	case DockerMode:
		config.WorkspaceDir = "/workspace"
		config.VolumeDir = "/volume"
		config.LiveDir = "/live"
		config.HomeDir = "/home/catnip"
		config.TempDir = "/tmp"
		config.SyncEnabled = true
		config.PortMonitorEnabled = true

	case NativeMode:
		// Get user's home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = os.Getenv("HOME")
			if homeDir == "" {
				homeDir = "."
			}
		}

		// Create catnip directory in user's home
		catnipDir := filepath.Join(homeDir, ".catnip")

		config.WorkspaceDir = filepath.Join(catnipDir, "workspace")
		config.VolumeDir = catnipDir // Settings stored directly in ~/.catnip
		config.LiveDir = ""          // Will be set if running from a git repo
		config.HomeDir = homeDir
		config.TempDir = os.TempDir()
		config.SyncEnabled = false                          // No need to sync in native mode
		config.PortMonitorEnabled = runtime.GOOS == "linux" // Only on Linux

		// Detect if we're running from within a git repository
		if repoRoot := detectGitRepo(); repoRoot != "" {
			config.LiveDir = filepath.Dir(repoRoot) // Parent of repo
			config.CurrentRepo = filepath.Base(repoRoot)
		}

		// Ensure directories exist
		if err := ensureDir(config.WorkspaceDir); err != nil {
			log.Printf("Warning: Failed to create workspace directory %s: %v", config.WorkspaceDir, err)
		}
		if err := ensureDir(config.VolumeDir); err != nil {
			log.Printf("Warning: Failed to create config directory %s: %v", config.VolumeDir, err)
		}
	}

	return config
}

// detectMode determines if we're running in Docker or natively
func detectMode() RuntimeMode {
	// Check for Docker-specific files/environment
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return DockerMode
	}

	// Check for Docker in cgroup
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if strings.Contains(string(data), "docker") || strings.Contains(string(data), "containerd") {
			return DockerMode
		}
	}

	// Check for container environment variables
	if os.Getenv("CATNIP_CONTAINER") == "true" {
		return DockerMode
	}

	return NativeMode
}

// detectGitRepo checks if we're running from within a git repository
func detectGitRepo() string {
	// Try to find .git directory by walking up from current directory
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Use git command to find the root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// ensureDir creates a directory if it doesn't exist
func ensureDir(path string) error {
	if path == "" {
		return nil
	}
	return os.MkdirAll(path, 0755)
}

// ResolvePath converts a container path to the appropriate path for the current runtime
func (rc *RuntimeConfig) ResolvePath(containerPath string) string {
	// Handle special cases
	switch {
	case strings.HasPrefix(containerPath, "/workspace/"):
		rel := strings.TrimPrefix(containerPath, "/workspace/")
		return filepath.Join(rc.WorkspaceDir, rel)

	case strings.HasPrefix(containerPath, "/volume/"):
		rel := strings.TrimPrefix(containerPath, "/volume/")
		return filepath.Join(rc.VolumeDir, rel)

	case strings.HasPrefix(containerPath, "/live/"):
		if rc.LiveDir == "" {
			// No live directory in native mode without git repo
			return ""
		}
		rel := strings.TrimPrefix(containerPath, "/live/")
		return filepath.Join(rc.LiveDir, rel)

	case strings.HasPrefix(containerPath, "/home/catnip/"):
		rel := strings.TrimPrefix(containerPath, "/home/catnip/")
		return filepath.Join(rc.HomeDir, rel)

	case containerPath == "/home/catnip":
		return rc.HomeDir

	default:
		// Return as-is for other paths
		return containerPath
	}
}

// GetClaudeBinaryPaths returns the paths to search for Claude binary
func (rc *RuntimeConfig) GetClaudeBinaryPaths() []string {
	switch rc.Mode {
	case DockerMode:
		return []string{
			"/opt/catnip/nvm/versions/node/*/bin/claude",
			"/usr/local/bin/claude",
		}
	case NativeMode:
		// In native mode, assume claude is in PATH
		return []string{
			"claude", // Will use PATH lookup
		}
	}
	return nil
}

// GetProcPath returns the process information path
func (rc *RuntimeConfig) GetProcPath(pid int, subpath string) string {
	if !rc.PortMonitorEnabled {
		return ""
	}
	return fmt.Sprintf("/proc/%d/%s", pid, subpath)
}

// IsDocker returns true if running in Docker mode
func (rc *RuntimeConfig) IsDocker() bool {
	return rc.Mode == DockerMode
}

// IsNative returns true if running in Native mode
func (rc *RuntimeConfig) IsNative() bool {
	return rc.Mode == NativeMode
}
