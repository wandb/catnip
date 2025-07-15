package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type ContainerRuntime string

const (
	// RuntimeDocker represents the Docker container runtime
	RuntimeDocker ContainerRuntime = "docker"
	// RuntimeApple represents the Apple container runtime
	RuntimeApple  ContainerRuntime = "container"
)

type ContainerService struct {
	runtime ContainerRuntime
}

func NewContainerService() (*ContainerService, error) {
	runtime, err := detectContainerRuntime()
	if err != nil {
		return nil, err
	}
	
	return &ContainerService{
		runtime: runtime,
	}, nil
}

func detectContainerRuntime() (ContainerRuntime, error) {
	if commandExists("docker") {
		return RuntimeDocker, nil
	}
	
	return "", fmt.Errorf("no container runtime found. Please install Docker:\n\n" +
		"macOS: brew install --cask docker\n" +
		"Linux: https://docs.docker.com/engine/install/\n" +
		"Windows: https://docs.docker.com/desktop/install/windows-install/")
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func (cs *ContainerService) RunContainer(ctx context.Context, image, name, workDir string, ports []string, isDevMode bool) error {
	args := []string{
		"run",
		"--rm",
		"--name", name,
		"-d",
	}
	
	// Add quality of life volume mounts and environment variables
	// TODO: Apple Container SDK doesn't support named volumes, so state volume mount
	// would need to be something like ~/.config/catnip/state when using containers
	args = append(args, "-v", "catnip-state:/volume")
	
	// Mount Claude IDE config if it exists
	claudeIDEPath := expandPath("~/.claude/ide")
	if _, err := os.Stat(claudeIDEPath); err == nil {
		args = append(args, "-v", fmt.Sprintf("%s:/volume/.claude/ide", claudeIDEPath))
	}
	
	// Environment variables
	args = append(args, "-e", "CLAUDE_CODE_IDE_HOST_OVERRIDE=host.docker.internal")
	args = append(args, "-e", "CATNIP_SESSION=catnip")
	if user := os.Getenv("USER"); user != "" {
		args = append(args, "-e", fmt.Sprintf("CATNIP_USERNAME=%s", user))
	}
	
	// Dev mode specific mounts
	if isDevMode {
		args = append(args, "-v", "catnip-dev-node-modules:/live/catnip/node_modules")
	}
	
	// Check if we're in a git repository and determine mount strategy
	gitRoot, isGitRepo := findGitRoot(workDir)
	if isGitRepo {
		// Use git repository basename for mount path
		basename := filepath.Base(gitRoot)
		mountPath := fmt.Sprintf("/live/%s", basename)
		args = append(args, "-v", fmt.Sprintf("%s:%s", gitRoot, mountPath))
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
	
	cmd := exec.CommandContext(ctx, string(cs.runtime), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run container: %w\nOutput: %s", err, string(output))
	}
	
	return nil
}

func (cs *ContainerService) GetContainerLogs(ctx context.Context, containerName string, follow bool) (*exec.Cmd, error) {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, containerName)
	
	cmd := exec.CommandContext(ctx, string(cs.runtime), args...)
	return cmd, nil
}

func (cs *ContainerService) StopContainer(ctx context.Context, containerName string) error {
	cmd := exec.CommandContext(ctx, string(cs.runtime), "stop", containerName)
	_, err := cmd.CombinedOutput()
	return err
}

func (cs *ContainerService) IsContainerRunning(ctx context.Context, containerName string) bool {
	cmd := exec.CommandContext(ctx, string(cs.runtime), "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	
	return strings.TrimSpace(string(output)) == containerName
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
	statsCmd := exec.CommandContext(ctx, string(cs.runtime), "stats", "--no-stream", "--format", "table {{.CPUPerc}}\t{{.MemUsage}}", containerName)
	if statsOutput, err := statsCmd.Output(); err == nil {
		info["stats"] = string(statsOutput)
	}
	
	// Get container port mappings with timeout  
	portsCmd := exec.CommandContext(ctx, string(cs.runtime), "port", containerName)
	if portsOutput, err := portsCmd.Output(); err == nil {
		info["ports"] = string(portsOutput)
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

// findGitRoot finds the git repository root starting from the given directory
func findGitRoot(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}
	
	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil {
			// Check if it's a directory (normal repo) or file (worktree)
			if info.IsDir() {
				return dir, true
			}
			// For git worktrees, .git is a file pointing to the real git dir
			if !info.IsDir() {
				return dir, true
			}
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			break
		}
		dir = parent
	}
	
	return "", false
}

var containerLogger *log.Logger
var containerDebugEnabled bool

func init() {
	containerDebugEnabled = os.Getenv("DEBUG") == "true"
	if containerDebugEnabled {
		logFile, err := os.OpenFile("/tmp/catctrl-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalln("Failed to open debug log file:", err)
		}
		containerLogger = log.New(logFile, "[CONTAINER] ", log.LstdFlags|log.Lmicroseconds)
	}
}

func containerDebugLog(format string, args ...interface{}) {
	if containerDebugEnabled && containerLogger != nil {
		containerLogger.Printf(format, args...)
	}
}

// GetRepositoryInfo returns information about mounted repositories
func (cs *ContainerService) GetRepositoryInfo(ctx context.Context, workDir string) map[string]interface{} {
	start := time.Now()
	containerDebugLog("GetRepositoryInfo() starting - workDir: %s", workDir)
	
	containerDebugLog("GetRepositoryInfo() calling findGitRoot - elapsed: %v", time.Since(start))
	gitRoot, isGitRepo := findGitRoot(workDir)
	containerDebugLog("GetRepositoryInfo() findGitRoot returned - isGitRepo: %t, elapsed: %v", isGitRepo, time.Since(start))
	
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