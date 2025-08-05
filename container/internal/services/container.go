package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vanpelt/catnip/internal/git"
)

type ContainerRuntime string

const (
	// RuntimeDocker represents the Docker container runtime
	RuntimeDocker ContainerRuntime = "docker"
	// RuntimeApple represents the Apple container runtime
	RuntimeApple ContainerRuntime = "container"
)

type ContainerService struct {
	runtime ContainerRuntime
}

// GetRuntime returns the current container runtime
func (cs *ContainerService) GetRuntime() ContainerRuntime {
	return cs.runtime
}

func NewContainerService() (*ContainerService, error) {
	return NewContainerServiceWithRuntime("")
}

func NewContainerServiceWithRuntime(preferredRuntime string) (*ContainerService, error) {
	var runtime ContainerRuntime
	var err error

	if preferredRuntime != "" {
		// Use the specified runtime if provided
		switch preferredRuntime {
		case "docker":
			if !commandExists("docker") {
				return nil, fmt.Errorf("docker runtime requested but docker command not found")
			}
			runtime = RuntimeDocker
		case "container", "apple":
			if !commandExists("container") {
				return nil, fmt.Errorf("container runtime requested but container command not found")
			}
			runtime = RuntimeApple
		case "native":
			return nil, fmt.Errorf("native runtime is not supported for container operations. Use 'catnip serve' for native mode")
		default:
			return nil, fmt.Errorf("unknown runtime: %s (valid options: docker, container)", preferredRuntime)
		}
	} else {
		// Auto-detect runtime
		runtime, err = detectContainerRuntime()
		if err != nil {
			return nil, err
		}
	}

	// Using runtime for container operations

	return &ContainerService{
		runtime: runtime,
	}, nil
}

func detectContainerRuntime() (ContainerRuntime, error) {
	// Check for Apple Container SDK first (preferred)
	if commandExists("container") {
		return RuntimeApple, nil
	}

	// Check for Docker as fallback
	if commandExists("docker") {
		return RuntimeDocker, nil
	}

	return "", fmt.Errorf("no container runtime found. Please install Docker or Apple Container SDK:\n\n" +
		"Docker:\n" +
		"  macOS: brew install --cask docker\n" +
		"  Linux: https://docs.docker.com/engine/install/\n" +
		"  Windows: https://docs.docker.com/desktop/install/windows-install/\n\n" +
		"Apple Container SDK:\n" +
		"  macOS: https://github.com/apple/container")
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func (cs *ContainerService) RunContainer(ctx context.Context, image, name, workDir string, ports []string, isDevMode bool, sshEnabled bool, rmFlag bool, cpus float64, memoryGB float64, envVars []string) ([]string, error) {
	// Check if container already exists
	if cs.ContainerExists(ctx, name) {
		if cs.IsContainerRunning(ctx, name) {
			// Container is already running, return success
			return []string{"container", "already", "running"}, nil
		}

		// Container exists but is stopped
		if rmFlag {
			// If --rm flag was specified, remove the existing container first
			if err := cs.RemoveContainer(ctx, name); err != nil {
				return nil, fmt.Errorf("failed to remove existing container: %w", err)
			}
			// Continue to create new container below
		} else {
			// Try to start the existing container
			if err := cs.StartContainer(ctx, name); err != nil {
				// If starting fails, remove and recreate as fallback
				_ = cs.StopContainer(ctx, name)   // Stop if partially running
				_ = cs.RemoveContainer(ctx, name) // Remove the container
				// Continue to create new container below
			} else {
				// Successfully started existing container
				return []string{"container", "start", name}, nil
			}
		}
	}

	// Create new container
	args := []string{
		"run",
		"--name", name,
		"-d",
	}

	// Only add --rm flag if explicitly requested
	if rmFlag {
		args = append(args, "--rm")
	}

	// Add resource limits if specified
	if cpus > 0 {
		switch cs.runtime {
		case RuntimeDocker:
			args = append(args, "--cpus", fmt.Sprintf("%.1f", cpus))
		case RuntimeApple:
			args = append(args, "--cpus", fmt.Sprintf("%.0f", cpus))
		}
	}
	if memoryGB > 0 {
		// Convert GB to bytes for Docker, but use GB format for Apple Container
		switch cs.runtime {
		case RuntimeDocker:
			memoryBytes := int64(memoryGB * 1024 * 1024 * 1024)
			args = append(args, "--memory", fmt.Sprintf("%d", memoryBytes))
		case RuntimeApple:
			args = append(args, "--memory", fmt.Sprintf("%.0fG", memoryGB))
		}
	}

	// Add quality of life volume mounts and environment variables
	// Both runtimes now use ~/.catnip/volume for state persistence
	stateVolumePath := expandPath("~/.catnip/volume")
	// Ensure the directory exists
	if err := os.MkdirAll(stateVolumePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state volume directory: %w", err)
	}
	args = append(args, "-v", fmt.Sprintf("%s:/volume", stateVolumePath))

	// Mount Claude IDE config if it exists
	claudeIDEPath := expandPath("~/.claude/ide")
	if _, err := os.Stat(claudeIDEPath); err == nil {
		args = append(args, "-v", fmt.Sprintf("%s:/volume/.claude/ide", claudeIDEPath))
	}

	// Environment variables
	switch cs.runtime {
	case RuntimeDocker:
		args = append(args, "-e", "CLAUDE_CODE_IDE_HOST_OVERRIDE=host.docker.internal")
		args = append(args, "-e", "CATNIP_RUNTIME=docker")
	case RuntimeApple:
		// Apple containers might use a different host override
		args = append(args, "-e", "CLAUDE_CODE_IDE_HOST_OVERRIDE=host.containers.internal")
		args = append(args, "-e", "CATNIP_RUNTIME=container")
	}
	args = append(args, "-e", "CATNIP_SESSION=catnip")
	if user := os.Getenv("USER"); user != "" {
		args = append(args, "-e", fmt.Sprintf("CATNIP_USERNAME=%s", user))
	}

	// Add user-specified environment variables
	for _, envVar := range envVars {
		args = append(args, "-e", envVar)
	}

	// Mount SSH public key if SSH is enabled
	if sshEnabled {
		args = append(args, "-e", "CATNIP_SSH_ENABLED=true")
		publicKeyPath := expandPath("~/.ssh/catnip_remote.pub")
		if _, err := os.Stat(publicKeyPath); err == nil {
			switch cs.runtime {
			case RuntimeDocker:
				args = append(args, "-v", fmt.Sprintf("%s:/home/catnip/.ssh/catnip_remote.pub:ro", publicKeyPath))
			case RuntimeApple:
				// Apple Container doesn't support file mounts, only directory mounts
				// Create a temporary directory and copy the SSH key there
				sshDir := expandPath("~/.catnip/ssh")
				if err := os.MkdirAll(sshDir, 0700); err != nil {
					return nil, fmt.Errorf("failed to create SSH directory: %w", err)
				}

				// Copy the public key to the catnip ssh directory
				sshKeyDest := filepath.Join(sshDir, "catnip_remote.pub")
				if err := copyFile(publicKeyPath, sshKeyDest); err != nil {
					return nil, fmt.Errorf("failed to copy SSH key: %w", err)
				}

				// Mount the entire ssh directory
				args = append(args, "-v", fmt.Sprintf("%s:/home/catnip/.ssh", sshDir))
			}
		}
	}

	// Check if we're in a git repository and determine mount strategy
	gitRoot, isGitRepo := git.FindGitRoot(workDir)
	if isGitRepo {
		if isDevMode {
			// In dev mode, always mount to /live/catnip for consistency with dev-entrypoint
			args = append(args, "-v", fmt.Sprintf("%s:/live/catnip", gitRoot))
		} else {
			// In normal mode, use the basename of the repo path
			repoName := filepath.Base(gitRoot)
			args = append(args, "-v", fmt.Sprintf("%s:/live/%s", gitRoot, repoName))
		}
	}

	// Dev mode specific mounts (AFTER git repo mount so they override)
	if isDevMode {
		switch cs.runtime {
		case RuntimeDocker:
			// Docker supports named volumes
			args = append(args, "-v", "catnip-dev-node-modules:/live/catnip/node_modules")
		case RuntimeApple:
			// Apple Container SDK doesn't support named volumes, use host path
			nodeModulesPath := expandPath("~/.catnip/node_modules")
			if err := os.MkdirAll(nodeModulesPath, 0755); err != nil {
				return nil, fmt.Errorf("failed to create node_modules directory: %w", err)
			}
			args = append(args, "-v", fmt.Sprintf("%s:/live/catnip/node_modules", nodeModulesPath))
		}
	}
	// If not a git repo, don't mount any directory
	var hasVite = false
	for _, port := range ports {
		args = append(args, "-p", port)
		if strings.HasPrefix(port, "5173") {
			hasVite = true
		}
	}
	// Forward 5137 for HMR / live reload in dev mode...
	if !hasVite && isDevMode {
		args = append(args, "-p", "5173:5173")
	}

	args = append(args, image)

	// Store the full command before execution for error reporting
	fullCmd := append([]string{string(cs.runtime)}, args...)

	cmd := exec.CommandContext(ctx, string(cs.runtime), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fullCmd, fmt.Errorf("failed to run container: %w\nOutput: %s", err, string(output))
	}

	return fullCmd, nil
}

// ImageExists checks if a container image exists locally
func (cs *ContainerService) ImageExists(ctx context.Context, image string) bool {
	var cmd *exec.Cmd
	switch cs.runtime {
	case RuntimeDocker:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "image", "inspect", image)
	case RuntimeApple:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "image", "inspect", image)
	}
	_, err := cmd.Output()
	return err == nil
}

// PullImage pulls a container image and returns a command to stream the output
func (cs *ContainerService) PullImage(ctx context.Context, image string) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	switch cs.runtime {
	case RuntimeDocker:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "pull", image)
	case RuntimeApple:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "image", "pull", image)
	}
	return cmd, nil
}

// BuildDevImage builds the development image using just build-dev
func (cs *ContainerService) BuildDevImage(ctx context.Context, gitRoot string) (*exec.Cmd, error) {
	// Run just build-dev from git root directory
	cmd := exec.CommandContext(ctx, "just", "build-dev")
	cmd.Dir = gitRoot
	return cmd, nil
}

func (cs *ContainerService) GetContainerLogs(ctx context.Context, containerName string, follow bool) (*exec.Cmd, error) {
	var args []string
	switch cs.runtime {
	case RuntimeDocker:
		args = []string{"logs"}
		if follow {
			args = append(args, "-f")
		}
		args = append(args, containerName)
	case RuntimeApple:
		args = []string{"logs"}
		if follow {
			args = append(args, "-f")
		}
		args = append(args, containerName)
	}

	cmd := exec.CommandContext(ctx, string(cs.runtime), args...)
	return cmd, nil
}

func (cs *ContainerService) StopContainer(ctx context.Context, containerName string) error {
	var cmd *exec.Cmd
	switch cs.runtime {
	case RuntimeDocker:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "stop", containerName)
	case RuntimeApple:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "stop", containerName)
	}
	_, err := cmd.CombinedOutput()
	return err
}

func (cs *ContainerService) RemoveContainer(ctx context.Context, containerName string) error {
	var cmd *exec.Cmd
	switch cs.runtime {
	case RuntimeDocker:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "rm", containerName)
	case RuntimeApple:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "rm", containerName)
	}
	_, err := cmd.CombinedOutput()
	return err
}

func (cs *ContainerService) StartContainer(ctx context.Context, containerName string) error {
	var cmd *exec.Cmd
	switch cs.runtime {
	case RuntimeDocker:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "start", containerName)
	case RuntimeApple:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "start", containerName)
	}
	_, err := cmd.CombinedOutput()
	return err
}

func (cs *ContainerService) ContainerExists(ctx context.Context, containerName string) bool {
	var cmd *exec.Cmd
	switch cs.runtime {
	case RuntimeDocker:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "ps", "-a", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	case RuntimeApple:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "list", "--all")
	}
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	switch cs.runtime {
	case RuntimeDocker:
		return strings.TrimSpace(string(output)) == containerName
	case RuntimeApple:
		// Parse Apple Container table output (including stopped containers)
		// Format: ID      IMAGE                         OS     ARCH   STATE    ADDR
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			// Skip header line
			if strings.HasPrefix(line, "ID") {
				continue
			}
			// Parse table columns (space-separated)
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				id := fields[0]
				if id == containerName {
					return true
				}
			}
		}
		return false
	}

	return false
}

func (cs *ContainerService) IsContainerRunning(ctx context.Context, containerName string) bool {
	var cmd *exec.Cmd
	switch cs.runtime {
	case RuntimeDocker:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	case RuntimeApple:
		cmd = exec.CommandContext(ctx, string(cs.runtime), "list")
	}
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	switch cs.runtime {
	case RuntimeDocker:
		return strings.TrimSpace(string(output)) == containerName
	case RuntimeApple:
		// Parse Apple Container table output
		// Format: ID      IMAGE                         OS     ARCH   STATE    ADDR
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			// Skip header line
			if strings.HasPrefix(line, "ID") {
				continue
			}
			// Parse table columns (space-separated)
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				id := fields[0]
				state := fields[4]
				if id == containerName && state == "running" {
					return true
				}
			}
		}
		return false
	}

	return false
}

func (cs *ContainerService) GetContainerPorts(ctx context.Context, containerName string) ([]string, error) {
	// Instead of using docker port, we should query the container's /v1/ports endpoint
	// to get the actual detected services, not the exposed ports

	// First, check if container is running
	if !cs.IsContainerRunning(ctx, containerName) {
		return []string{}, nil
	}

	// Try to fetch ports from the container's API
	// This assumes the container is running our catnip server on port 8080
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:8080/v1/ports")
	if err != nil {
		// If we can't reach the API, fall back to empty list
		return []string{}, nil
	}
	defer resp.Body.Close()

	var portData struct {
		Ports map[string]interface{} `json:"ports"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&portData); err != nil {
		return []string{}, nil
	}

	var ports []string
	for portStr := range portData.Ports {
		ports = append(ports, portStr)
	}

	// Remove duplicates and sort
	unique := make(map[string]bool)
	var result []string
	for _, port := range ports {
		if !unique[port] {
			unique[port] = true
			result = append(result, port)
		}
	}

	return result, nil
}

func (cs *ContainerService) GetContainerInfo(ctx context.Context, containerName string) (map[string]interface{}, error) {
	// Check if container is running first
	if !cs.IsContainerRunning(ctx, containerName) {
		return map[string]interface{}{
			"name":    containerName,
			"runtime": string(cs.runtime),
			"ports":   "",
			"stats":   "",
		}, nil
	}

	info := map[string]interface{}{
		"name":    containerName,
		"runtime": string(cs.runtime),
		"ports":   "",
		"stats":   "",
	}

	// Get container stats with timeout
	var statsCmd *exec.Cmd
	switch cs.runtime {
	case RuntimeDocker:
		statsCmd = exec.CommandContext(ctx, string(cs.runtime), "stats", "--no-stream", "--format", "table {{.CPUPerc}}\t{{.MemUsage}}", containerName)
	case RuntimeApple:
		// Apple Container doesn't have stats command like Docker, skip stats for now
		statsCmd = nil
	}
	if statsCmd != nil {
		if statsOutput, err := statsCmd.Output(); err == nil {
			info["stats"] = string(statsOutput)
		} else {
			// Only log failures to keep logs clean
			containerDebugLog("Failed to get container stats for %s: %v", containerName, err)
		}
	}

	// Get container port mappings with timeout
	var portsCmd *exec.Cmd
	switch cs.runtime {
	case RuntimeDocker:
		portsCmd = exec.CommandContext(ctx, string(cs.runtime), "port", containerName)
	case RuntimeApple:
		// Apple Container doesn't have port command like Docker, skip port info for now
		portsCmd = nil
	}
	if portsCmd != nil {
		if portsOutput, err := portsCmd.Output(); err == nil {
			info["ports"] = string(portsOutput)
		}
	}

	return info, nil
}

func IsProcessRunning(pid int) bool {
	process, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid)).Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(process))) > 0
}

func KillProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

var containerLogger *log.Logger
var containerDebugEnabled bool

func init() {
	containerDebugEnabled = os.Getenv("DEBUG") == "true"
	if containerDebugEnabled {
		logFile, err := os.OpenFile("/tmp/catnip-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			log.Fatalln("Failed to open debug log file:", err)
		}
		containerLogger = log.New(logFile, "[CONTAINER] ", log.LstdFlags|log.Lmicroseconds)
	}
}

func containerDebugLog(format string, args ...interface{}) {
	if containerDebugEnabled && containerLogger != nil {
		containerLogger.Printf(format+"\n", args...)
	}
}

// GetRepositoryInfo returns information about mounted repositories
func (cs *ContainerService) GetRepositoryInfo(ctx context.Context, workDir string) map[string]interface{} {
	start := time.Now()
	containerDebugLog("GetRepositoryInfo() starting - workDir: %s", workDir)

	containerDebugLog("GetRepositoryInfo() calling git.FindGitRoot - elapsed: %v", time.Since(start))
	gitRoot, isGitRepo := git.FindGitRoot(workDir)
	containerDebugLog("GetRepositoryInfo() git.FindGitRoot returned - isGitRepo: %t, elapsed: %v", isGitRepo, time.Since(start))

	info := map[string]interface{}{
		"is_git_repo": isGitRepo,
	}

	if isGitRepo {
		info["git_root"] = gitRoot
		info["repo_name"] = filepath.Base(gitRoot)

		// Get current branch if possible
		containerDebugLog("GetRepositoryInfo() calling getCurrentBranch - elapsed: %v", time.Since(start))
		if branch := getCurrentBranch(gitRoot); branch != "" {
			info["current_branch"] = branch
		}
		containerDebugLog("GetRepositoryInfo() getCurrentBranch returned - elapsed: %v", time.Since(start))

		// Get remote origin if possible
		containerDebugLog("GetRepositoryInfo() calling getRemoteOrigin - elapsed: %v", time.Since(start))
		if origin := getRemoteOrigin(gitRoot); origin != "" {
			info["remote_origin"] = origin
		}
		containerDebugLog("GetRepositoryInfo() getRemoteOrigin returned - elapsed: %v", time.Since(start))
	}

	containerDebugLog("GetRepositoryInfo() finished - elapsed: %v", time.Since(start))
	return info
}

// getCurrentBranch gets the current git branch
func getCurrentBranch(gitRoot string) string {
	start := time.Now()
	containerDebugLog("getCurrentBranch() starting - gitRoot: %s", gitRoot)

	cmd := exec.Command("git", "-C", gitRoot, "rev-parse", "--abbrev-ref", "HEAD")
	containerDebugLog("getCurrentBranch() executing git command - elapsed: %v", time.Since(start))

	if output, err := cmd.Output(); err == nil {
		result := strings.TrimSpace(string(output))
		containerDebugLog("getCurrentBranch() finished - result: %s, elapsed: %v", result, time.Since(start))
		return result
	}
	containerDebugLog("getCurrentBranch() failed - elapsed: %v", time.Since(start))
	return ""
}

// getRemoteOrigin gets the remote origin URL
func getRemoteOrigin(gitRoot string) string {
	start := time.Now()
	containerDebugLog("getRemoteOrigin() starting - gitRoot: %s", gitRoot)

	cmd := exec.Command("git", "-C", gitRoot, "remote", "get-url", "origin")
	containerDebugLog("getRemoteOrigin() executing git command - elapsed: %v", time.Since(start))

	if output, err := cmd.Output(); err == nil {
		result := strings.TrimSpace(string(output))
		containerDebugLog("getRemoteOrigin() finished - result: %s, elapsed: %v", result, time.Since(start))
		return result
	}
	containerDebugLog("getRemoteOrigin() failed - elapsed: %v", time.Since(start))
	return ""
}

// expandPath expands ~ to the user's home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}
	return path
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Copy file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}
