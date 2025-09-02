package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vanpelt/catnip/internal/logger"
)

// RuntimeMode represents the execution environment
type RuntimeMode string

const (
	// DockerMode indicates running inside a Docker container
	DockerMode RuntimeMode = "docker"
	// ContainerMode indicates running inside an Apple Container
	ContainerMode RuntimeMode = "container"
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

// getEnvOrDefault returns the environment variable value if set, otherwise returns the default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// DetectRuntime determines the current runtime environment and returns appropriate configuration
func DetectRuntime() *RuntimeConfig {
	mode := detectMode()

	config := &RuntimeConfig{
		Mode: mode,
	}

	// Get user's home directory for defaults
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("HOME")
		if homeDir == "" {
			homeDir = "."
		}
	}

	// Set defaults based on runtime mode
	switch mode {
	case DockerMode, ContainerMode:
		config.WorkspaceDir = getEnvOrDefault("CATNIP_WORKSPACE_DIR", "/workspace")
		config.VolumeDir = getEnvOrDefault("CATNIP_VOLUME_DIR", "/volume")
		config.LiveDir = getEnvOrDefault("CATNIP_LIVE_DIR", "/live")
		config.HomeDir = getEnvOrDefault("CATNIP_HOME_DIR", "/home/catnip")
		config.TempDir = getEnvOrDefault("CATNIP_TEMP_DIR", "/tmp")
		config.SyncEnabled = true
		config.PortMonitorEnabled = true

	case NativeMode:
		// Create catnip directory in user's home
		catnipDir := filepath.Join(homeDir, ".catnip")

		config.WorkspaceDir = getEnvOrDefault("CATNIP_WORKSPACE_DIR", filepath.Join(catnipDir, "workspace"))
		config.VolumeDir = getEnvOrDefault("CATNIP_VOLUME_DIR", catnipDir) // Settings stored directly in ~/.catnip
		config.LiveDir = getEnvOrDefault("CATNIP_LIVE_DIR", "")            // Will be set if running from a git repo
		config.HomeDir = getEnvOrDefault("CATNIP_HOME_DIR", homeDir)
		config.TempDir = getEnvOrDefault("CATNIP_TEMP_DIR", os.TempDir())
		config.SyncEnabled = false                          // No need to sync in native mode
		config.PortMonitorEnabled = runtime.GOOS == "linux" // Only on Linux

		// Detect if we're running from within a git repository
		if repoRoot := detectGitRepo(); repoRoot != "" {
			// In native mode, we don't set LiveDir to avoid scanning all sibling repos
			// We just track the current repo name and path
			config.CurrentRepo = filepath.Base(repoRoot)
		}

		// Ensure directories exist
		if err := ensureDir(config.WorkspaceDir); err != nil {
			logger.Debugf("Warning: Failed to create workspace directory %s: %v", config.WorkspaceDir, err)
		}
		if err := ensureDir(config.VolumeDir); err != nil {
			logger.Debugf("Warning: Failed to create config directory %s: %v", config.VolumeDir, err)
		}
	}

	return config
}

// detectMode determines if we're running in Docker, Apple Container, GitHub Codespace, or natively
func detectMode() RuntimeMode {
	// Check for container environment variable first (can override detection)
	if containerType := os.Getenv("CATNIP_RUNTIME"); containerType != "" {
		switch containerType {
		case "docker":
			return DockerMode
		case "container", "apple":
			return ContainerMode
		case "codespace":
			// Treat codespaces as Docker mode
			return DockerMode
		case "true":
			// Legacy support - assume Docker
			return DockerMode
		}
	}

	// Check for GitHub Codespace environment - treat as Docker
	if os.Getenv("CODESPACES") == "true" ||
		os.Getenv("CODESPACE_NAME") != "" ||
		os.Getenv("GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN") != "" {
		return DockerMode
	}

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

	// Check for Apple Container environment
	if containerEnv := os.Getenv("container"); containerEnv == "apple" {
		return ContainerMode
	}

	// Default to native mode
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
	case DockerMode, ContainerMode:
		paths := []string{
			"/opt/catnip/bin/claude",                     // Codespace wrapped binary
			"/opt/catnip/nvm/versions/node/*/bin/claude", // Standard container path
			"/usr/local/bin/claude",                      // Standard system path
		}

		// Add user's local bin directory if we can determine it
		if homeDir := rc.HomeDir; homeDir != "" {
			paths = append(paths[:1], append([]string{filepath.Join(homeDir, ".local/bin/claude")}, paths[1:]...)...)
		}

		return paths
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

// IsContainer returns true if running in Container mode
func (rc *RuntimeConfig) IsContainer() bool {
	return rc.Mode == ContainerMode
}

// IsNative returns true if running in Native mode
func (rc *RuntimeConfig) IsNative() bool {
	return rc.Mode == NativeMode
}

// IsContainerized returns true if running in any container (Docker or Apple)
func (rc *RuntimeConfig) IsContainerized() bool {
	return rc.Mode == DockerMode || rc.Mode == ContainerMode
}
