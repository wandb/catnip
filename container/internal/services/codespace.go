package services

import (
	"bufio"
	"context"
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

// CodespaceInfo represents information about a GitHub Codespace
type CodespaceInfo struct {
	Name        string                 `json:"name"`
	DisplayName string                 `json:"displayName"`
	Repository  string                 `json:"repository"`
	GitStatus   map[string]interface{} `json:"gitStatus"`
	State       string                 `json:"state"`
	Machine     string                 `json:"machineName"`
	CreatedAt   string                 `json:"createdAt"`
	LastUsedAt  string                 `json:"lastUsedAt"`
	Owner       string                 `json:"owner"`
	VSCSTarget  string                 `json:"vscsTarget"`
	// Computed fields
	WebURL    string `json:"-"`
	VSCodeURL string `json:"-"`
}

// CodespaceService manages GitHub Codespaces
type CodespaceService struct {
	configPath string
	tokenPath  string
}

// NewCodespaceService creates a new codespace service
func NewCodespaceService() (*CodespaceService, error) {
	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("GitHub CLI (gh) is not installed. Run 'catnip bootstrap' to install it")
	}

	// Get user's home directory for config storage
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".catnip", "codespaces.json")
	tokenPath := filepath.Join(homeDir, ".catnip", "codespace-token")

	return &CodespaceService{
		configPath: configPath,
		tokenPath:  tokenPath,
	}, nil
}

// computeCodespaceURLs adds the computed WebURL and VSCodeURL to a codespace
func (cs *CodespaceService) computeCodespaceURLs(codespace *CodespaceInfo) {
	// GitHub Codespaces web URL format
	codespace.WebURL = fmt.Sprintf("https://github.com/codespaces/%s", codespace.Name)
	// VS Code URL format
	codespace.VSCodeURL = fmt.Sprintf("https://%s.github.dev", codespace.Name)
}

// ListCodespaces lists all available codespaces
func (cs *CodespaceService) ListCodespaces(ctx context.Context) ([]CodespaceInfo, error) {
	logger.Debugf("🔍 Listing GitHub Codespaces...")

	cmd := exec.CommandContext(ctx, "gh", "codespace", "list", "--json", "name,displayName,repository,gitStatus,state,machineName,createdAt,lastUsedAt,owner,vscsTarget")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to list codespaces: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to list codespaces: %w", err)
	}

	var codespaces []CodespaceInfo
	if err := json.Unmarshal(output, &codespaces); err != nil {
		return nil, fmt.Errorf("failed to parse codespaces list: %w", err)
	}

	// Compute URLs for each codespace
	for i := range codespaces {
		cs.computeCodespaceURLs(&codespaces[i])
	}

	logger.Debugf("✅ Found %d codespaces", len(codespaces))
	return codespaces, nil
}

// CreateCodespace creates a new codespace for the specified repository
func (cs *CodespaceService) CreateCodespace(ctx context.Context, repo, branch string) (*CodespaceInfo, error) {
	logger.Infof("🚀 Creating new codespace for %s...", repo)

	args := []string{"codespace", "create", "--repo", repo}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--json", "name,displayName,repository,gitStatus,state,machineName,createdAt,lastUsedAt,owner,vscsTarget")

	cmd := exec.CommandContext(ctx, "gh", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to create codespace: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to create codespace: %w", err)
	}

	var codespace CodespaceInfo
	if err := json.Unmarshal(output, &codespace); err != nil {
		return nil, fmt.Errorf("failed to parse codespace info: %w", err)
	}

	// Compute URLs for the new codespace
	cs.computeCodespaceURLs(&codespace)

	logger.Infof("✅ Created codespace: %s", codespace.Name)
	return &codespace, nil
}

// ConnectToCodespace connects to a codespace via SSH and executes commands
func (cs *CodespaceService) ConnectToCodespace(ctx context.Context, codespaceName string, commands []string) error {
	logger.Infof("🔌 Connecting to codespace: %s", codespaceName)

	// First, check if codespace is running
	if err := cs.ensureCodespaceRunning(ctx, codespaceName); err != nil {
		return fmt.Errorf("failed to ensure codespace is running: %w", err)
	}

	// Execute each command via SSH
	for _, command := range commands {
		logger.Debugf("📝 Executing: %s", command)

		cmd := exec.CommandContext(ctx, "gh", "codespace", "ssh", "--codespace", codespaceName, "--", "bash", "-c", command)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to execute command '%s' in codespace: %w", command, err)
		}
	}

	return nil
}

// RunCommandInCodespace runs a single command in the codespace and returns the output
func (cs *CodespaceService) RunCommandInCodespace(ctx context.Context, codespaceName, command string) (string, error) {
	logger.Debugf("📝 Running command in codespace %s: %s", codespaceName, command)

	cmd := exec.CommandContext(ctx, "gh", "codespace", "ssh", "--codespace", codespaceName, "--", "bash", "-c", command)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			// Check for SSH configuration error
			if strings.Contains(stderr, "failed to start SSH server") || strings.Contains(stderr, "SSH server is installed") {
				return "", fmt.Errorf("SSH is not configured in this codespace.\n\n"+
					"To fix this, add SSH support to your .devcontainer/devcontainer.json:\n\n"+
					"  \"features\": {\n"+
					"    \"ghcr.io/devcontainers/features/sshd:1\": {\n"+
					"      \"version\": \"latest\"\n"+
					"    }\n"+
					"  }\n\n"+
					"Then rebuild your codespace with: gh codespace rebuild --codespace %s", codespaceName)
			}
			return "", fmt.Errorf("command failed: %s", stderr)
		}
		return "", fmt.Errorf("failed to run command: %w", err)
	}

	return string(output), nil
}

// StartCodespaceDaemon starts catnip serve as a daemon in the codespace
func (cs *CodespaceService) StartCodespaceDaemon(ctx context.Context, codespaceName string) error {
	logger.Infof("🚀 Starting catnip daemon in codespace: %s", codespaceName)

	// Check if catnip is already installed
	logger.Debugf("Checking if catnip is installed...")
	catnipPath, err := cs.RunCommandInCodespace(ctx, codespaceName, "which catnip 2>/dev/null || echo $HOME/.local/bin/catnip")
	catnipPath = strings.TrimSpace(catnipPath)

	// Check if the found path actually exists and is executable
	if err == nil && catnipPath != "" && !strings.HasSuffix(catnipPath, ".local/bin/catnip") {
		// Only test executability if we found it via 'which', not if we defaulted to .local/bin
		testOutput, testErr := cs.RunCommandInCodespace(ctx, codespaceName, fmt.Sprintf("test -x %s && echo 'exists' || echo 'not-found'", catnipPath))
		if testErr != nil || strings.TrimSpace(testOutput) != "exists" {
			err = fmt.Errorf("catnip binary not executable")
		}
	} else if strings.HasSuffix(catnipPath, ".local/bin/catnip") {
		// For the default path, check if it exists
		testOutput, testErr := cs.RunCommandInCodespace(ctx, codespaceName, fmt.Sprintf("test -f %s && echo 'exists' || echo 'not-found'", catnipPath))
		if testErr != nil || strings.TrimSpace(testOutput) != "exists" {
			err = fmt.Errorf("catnip binary not found at default location")
		}
	}
	if err != nil {
		logger.Infof("📦 Catnip not found, installing...")

		// Check if we should use development installation mode
		var installCmd string
		catnipDev := os.Getenv("CATNIP_DEV")

		switch catnipDev {
		case "", "false":
			// Use the standard installation script
			installCmd = "curl -sSfL install.catnip.sh | sh"
		case "true":
			// Clone the repo and run 'just install' from main branch
			logger.Infof("🔧 Using development installation mode (CATNIP_DEV=true)")
			installCmd = "cd /tmp && rm -rf catnip && git clone https://github.com/vanpelt/catnip.git && cd catnip/container && just install"
		default:
			// Use CATNIP_DEV value as the branch name
			logger.Infof("🔧 Using development installation mode from branch: %s", catnipDev)
			installCmd = fmt.Sprintf("cd /tmp && rm -rf catnip && git clone -b %s https://github.com/vanpelt/catnip.git && cd catnip/container && just install", catnipDev)
		}

		installOutput, err := cs.RunCommandInCodespace(ctx, codespaceName, installCmd)
		if err != nil {
			return fmt.Errorf("failed to install catnip in codespace: %w", err)
		}
		logger.Debugf("Install output: %s", installOutput)

		// Check again after installation - look in the default install location
		catnipPath, err = cs.RunCommandInCodespace(ctx, codespaceName, "which catnip 2>/dev/null || echo $HOME/.local/bin/catnip")
		if err != nil {
			return fmt.Errorf("catnip not found after installation: %w", err)
		}
		catnipPath = strings.TrimSpace(catnipPath)
	}
	logger.Infof("✅ Catnip found at: %s", strings.TrimSpace(catnipPath))

	// Check catnip version
	versionOutput, err := cs.RunCommandInCodespace(ctx, codespaceName, fmt.Sprintf("%s --version", catnipPath))
	if err != nil {
		logger.Warnf("⚠️ Could not get catnip version: %v", err)
	} else {
		logger.Infof("📌 Catnip version: %s", strings.TrimSpace(versionOutput))
	}

	// Run bootstrap to ensure dependencies are installed
	logger.Debugf("Running bootstrap...")
	bootstrapCmd := fmt.Sprintf("%s bootstrap", catnipPath)
	if bootstrapOutput, err := cs.RunCommandInCodespace(ctx, codespaceName, bootstrapCmd); err != nil {
		logger.Debugf("⚠️ Bootstrap failed (may be ok if dependencies already installed): %v", err)
	} else {
		logger.Debugf("Bootstrap output: %s", bootstrapOutput)
	}

	// Kill any existing catnip serve processes
	logger.Debugf("Cleaning up any existing catnip processes...")
	_, _ = cs.RunCommandInCodespace(ctx, codespaceName, "pkill -f 'catnip serve' || true")
	time.Sleep(1 * time.Second)

	// Start catnip serve as a daemon
	logger.Infof("🚀 Starting catnip serve daemon...")
	daemonCmd := fmt.Sprintf("nohup '%s' serve --port 6369 > /tmp/catnip.log 2>&1 & echo $!", catnipPath)
	logger.Debugf("Daemon command: %s", daemonCmd)
	pidOutput, err := cs.RunCommandInCodespace(ctx, codespaceName, daemonCmd)
	if err != nil {
		// Try to get any error output
		logOutput, _ := cs.RunCommandInCodespace(ctx, codespaceName, "cat /tmp/catnip.log 2>/dev/null || echo 'No log file found'")
		return fmt.Errorf("failed to start catnip daemon: %w\nLog output: %s", err, logOutput)
	}

	pid := strings.TrimSpace(pidOutput)
	logger.Infof("✅ Catnip daemon started with PID: %s", pid)

	// Wait a moment and verify it's still running
	logger.Debugf("Waiting for daemon to stabilize...")
	time.Sleep(3 * time.Second)

	checkCmd := fmt.Sprintf("ps -p %s > /dev/null 2>&1 && echo 'running' || echo 'stopped'", pid)
	statusOutput, err := cs.RunCommandInCodespace(ctx, codespaceName, checkCmd)
	if err != nil || strings.TrimSpace(statusOutput) != "running" {
		// Get detailed error information
		logOutput, _ := cs.RunCommandInCodespace(ctx, codespaceName, "cat /tmp/catnip.log 2>/dev/null || echo 'No log file'")
		psOutput, _ := cs.RunCommandInCodespace(ctx, codespaceName, fmt.Sprintf("ps aux | grep %s || echo 'Process not found'", pid))
		return fmt.Errorf("catnip daemon failed to start properly (PID %s).\nStatus: %s\nLog output:\n%s\nProcess info:\n%s",
			pid, statusOutput, logOutput, psOutput)
	}

	logger.Infof("✅ Catnip daemon is running successfully on port 6369")
	return nil
}

// ensureCodespaceRunning starts the codespace if it's not already running
func (cs *CodespaceService) ensureCodespaceRunning(ctx context.Context, codespaceName string) error {
	logger.Debugf("🔍 Checking codespace status: %s", codespaceName)

	// Get codespace info
	cmd := exec.CommandContext(ctx, "gh", "codespace", "list", "--json", "name,state")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check codespace status: %w", err)
	}

	var codespaces []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}

	if err := json.Unmarshal(output, &codespaces); err != nil {
		return fmt.Errorf("failed to parse codespace status: %w", err)
	}

	var targetState string
	for _, cs := range codespaces {
		if cs.Name == codespaceName {
			targetState = cs.State
			break
		}
	}

	if targetState == "" {
		return fmt.Errorf("codespace %s not found", codespaceName)
	}

	if targetState == "Available" {
		logger.Debugf("✅ Codespace is already running")
		return nil
	}

	if targetState == "Shutdown" {
		logger.Infof("🚀 Starting codespace: %s", codespaceName)
		startCmd := exec.CommandContext(ctx, "gh", "codespace", "start", "--codespace", codespaceName)
		startCmd.Stdout = os.Stdout
		startCmd.Stderr = os.Stderr

		if err := startCmd.Run(); err != nil {
			return fmt.Errorf("failed to start codespace: %w", err)
		}

		logger.Infof("✅ Codespace started successfully")
	}

	return nil
}

// SaveCodespaceConfig saves codespace configuration to the config file
func (cs *CodespaceService) SaveCodespaceConfig(codespaces map[string]string) error {
	logger.Debugf("💾 Saving codespace configuration to %s", cs.configPath)

	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(cs.configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Save configuration
	data, err := json.MarshalIndent(codespaces, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cs.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logger.Debugf("✅ Configuration saved successfully")
	return nil
}

// SaveCodespaceToken stores the provided token for a specific codespace
// in ~/.catnip/config.json so external tools can authenticate against
// the forwarded ports.
func (cs *CodespaceService) SaveCodespaceToken(codespaceName, token string) error {
	configPath := filepath.Join(filepath.Dir(cs.configPath), "config.json")
	logger.Debugf("💾 Saving GITHUB_TOKEN for %s to %s", codespaceName, configPath)

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	config := make(map[string]string)
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to unmarshal config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	config[codespaceName] = token

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logger.Debugf("✅ GITHUB_TOKEN saved successfully")
	return nil
}

// LoadCodespaceConfig loads codespace configuration from the config file
func (cs *CodespaceService) LoadCodespaceConfig() (map[string]string, error) {
	logger.Debugf("📂 Loading codespace configuration from %s", cs.configPath)

	if _, err := os.Stat(cs.configPath); os.IsNotExist(err) {
		logger.Debugf("📄 Config file doesn't exist, returning empty config")
		return make(map[string]string), nil
	}

	data, err := os.ReadFile(cs.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config map[string]string
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	logger.Debugf("✅ Loaded configuration with %d entries", len(config))
	return config, nil
}

// SelectCodespace presents a list of codespaces for user selection
func (cs *CodespaceService) SelectCodespace(ctx context.Context, codespaces []CodespaceInfo) (*CodespaceInfo, error) {
	if len(codespaces) == 0 {
		return nil, fmt.Errorf("no codespaces available")
	}

	if len(codespaces) == 1 {
		logger.Infof("📦 Only one codespace available: %s", codespaces[0].Name)
		return &codespaces[0], nil
	}

	// Display codespaces for selection
	fmt.Println("\n📦 Available Codespaces:")
	for i, cs := range codespaces {
		fmt.Printf("  [%d] %s (%s) - %s [%s]\n",
			i+1, cs.DisplayName, cs.Repository, cs.State, cs.Machine)
	}

	fmt.Print("\nSelect codespace (1-" + strconv.Itoa(len(codespaces)) + "): ")

	// Read user input
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(codespaces) {
		return nil, fmt.Errorf("invalid selection: %s", strings.TrimSpace(input))
	}

	selected := &codespaces[choice-1]
	logger.Infof("✅ Selected codespace: %s", selected.Name)
	return selected, nil
}

// PromptForNewCodespace asks the user if they want to create a new codespace
func (cs *CodespaceService) PromptForNewCodespace() (bool, string, string, error) {
	fmt.Print("\nWould you like to create a new codespace? (y/n): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, "", "", fmt.Errorf("failed to read input: %w", err)
	}

	response := strings.TrimSpace(strings.ToLower(input))
	if response != "y" && response != "yes" {
		return false, "", "", nil
	}

	fmt.Print("Enter repository (e.g., owner/repo): ")
	repoInput, err := reader.ReadString('\n')
	if err != nil {
		return false, "", "", fmt.Errorf("failed to read repository input: %w", err)
	}
	repo := strings.TrimSpace(repoInput)

	fmt.Print("Enter branch (optional, press enter for default): ")
	branchInput, err := reader.ReadString('\n')
	if err != nil {
		return false, "", "", fmt.Errorf("failed to read branch input: %w", err)
	}
	branch := strings.TrimSpace(branchInput)

	return true, repo, branch, nil
}

// SaveGlobalCodespaceToken saves a global codespace token for authenticated requests
func (cs *CodespaceService) SaveGlobalCodespaceToken(token string) error {
	logger.Debugf("💾 Saving global codespace token to %s", cs.tokenPath)

	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(cs.tokenPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Save token with restricted permissions
	if err := os.WriteFile(cs.tokenPath, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	logger.Debugf("✅ Global token saved successfully")
	return nil
}

// LoadCodespaceToken loads the codespace token for authenticated requests
func (cs *CodespaceService) LoadCodespaceToken() (string, error) {
	logger.Debugf("📂 Loading codespace token from %s", cs.tokenPath)

	if _, err := os.Stat(cs.tokenPath); os.IsNotExist(err) {
		logger.Debugf("📄 Token file doesn't exist")
		return "", nil
	}

	data, err := os.ReadFile(cs.tokenPath)
	if err != nil {
		return "", fmt.Errorf("failed to read token file: %w", err)
	}

	token := strings.TrimSpace(string(data))
	logger.Debugf("✅ Loaded token successfully")
	return token, nil
}

// GetCodespaceURL derives the catnip URL from a codespace name
func (cs *CodespaceService) GetCodespaceURL(codespaceName string, port string) string {
	if port == "" {
		port = "6369"
	}
	// GitHub Codespaces URL pattern: https://{codespaceName}-{port}.app.github.dev
	return fmt.Sprintf("https://%s-%s.app.github.dev", codespaceName, port)
}

// DeleteCodespaceToken removes the saved token
func (cs *CodespaceService) DeleteCodespaceToken() error {
	logger.Debugf("🗑️ Deleting codespace token")

	if _, err := os.Stat(cs.tokenPath); os.IsNotExist(err) {
		logger.Debugf("📄 Token file doesn't exist, nothing to delete")
		return nil
	}

	if err := os.Remove(cs.tokenPath); err != nil {
		return fmt.Errorf("failed to delete token file: %w", err)
	}

	logger.Debugf("✅ Token deleted successfully")
	return nil
}
