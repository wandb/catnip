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
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Repository  string `json:"repository"`
	Branch      string `json:"gitStatus.ref"`
	State       string `json:"state"`
	Machine     string `json:"machine"`
	CreatedAt   string `json:"createdAt"`
	LastUsedAt  string `json:"lastUsedAt"`
	WebURL      string `json:"webUrl"`
	VSCodeURL   string `json:"vsCodeUrl"`
}

// CodespaceService manages GitHub Codespaces
type CodespaceService struct {
	configPath string
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

	return &CodespaceService{
		configPath: configPath,
	}, nil
}

// ListCodespaces lists all available codespaces
func (cs *CodespaceService) ListCodespaces(ctx context.Context) ([]CodespaceInfo, error) {
	logger.Debugf("🔍 Listing GitHub Codespaces...")

	cmd := exec.CommandContext(ctx, "gh", "codespace", "list", "--json", "name,displayName,repository,gitStatus,state,machine,createdAt,lastUsedAt,webUrl,vsCodeUrl")
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
	args = append(args, "--json", "name,displayName,repository,gitStatus,state,machine,createdAt,lastUsedAt,webUrl,vsCodeUrl")

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
			return "", fmt.Errorf("command failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to run command: %w", err)
	}

	return string(output), nil
}

// StartCodespaceDaemon starts catnip serve as a daemon in the codespace
func (cs *CodespaceService) StartCodespaceDaemon(ctx context.Context, codespaceName string) error {
	logger.Infof("🚀 Starting catnip daemon in codespace: %s", codespaceName)

	// Install catnip if not already present
	installCmd := "curl -sSfL install.catnip.sh | sh"
	if _, err := cs.RunCommandInCodespace(ctx, codespaceName, "which catnip || ("+installCmd+")"); err != nil {
		return fmt.Errorf("failed to install catnip in codespace: %w", err)
	}

	// Run bootstrap to ensure dependencies are installed
	bootstrapCmd := "catnip bootstrap"
	if _, err := cs.RunCommandInCodespace(ctx, codespaceName, bootstrapCmd); err != nil {
		logger.Debugf("⚠️ Bootstrap failed (may be ok if dependencies already installed): %v", err)
	}

	// Start catnip serve as a daemon
	daemonCmd := "nohup catnip serve --port 8080 > /tmp/catnip.log 2>&1 & echo $!"
	pidOutput, err := cs.RunCommandInCodespace(ctx, codespaceName, daemonCmd)
	if err != nil {
		return fmt.Errorf("failed to start catnip daemon: %w", err)
	}

	pid := strings.TrimSpace(pidOutput)
	logger.Infof("✅ Catnip daemon started with PID: %s", pid)

	// Wait a moment and verify it's still running
	time.Sleep(2 * time.Second)
	checkCmd := fmt.Sprintf("kill -0 %s", pid)
	if _, err := cs.RunCommandInCodespace(ctx, codespaceName, checkCmd); err != nil {
		// Check the log for errors
		logOutput, _ := cs.RunCommandInCodespace(ctx, codespaceName, "tail -20 /tmp/catnip.log")
		return fmt.Errorf("catnip daemon failed to start properly. Log output:\n%s", logOutput)
	}

	logger.Infof("✅ Catnip daemon is running successfully")
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
