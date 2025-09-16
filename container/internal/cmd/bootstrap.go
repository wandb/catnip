package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	goruntime "runtime"

	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/logger"
)

var bootstrapCmd = &cobra.Command{
	Use:    "bootstrap",
	Short:  "🚀 Bootstrap Catnip environment with required dependencies",
	Hidden: true,
	Long: `# 🐱 Bootstrap Catnip Environment

**Ensures all required dependencies are installed for optimal Catnip experience.**

## 🔧 What it installs:

- **GitHub CLI (gh)** - For codespace management and GitHub integration
- **Claude Code** - Anthropic's AI-powered coding assistant
- **Codespace directories** - Creates required directories for codespace operation

## 🌐 Supported Platforms:

- **Linux** (including GitHub Codespaces)
- **macOS** (Intel and Apple Silicon)

## 📁 GitHub Codespace Integration:

When running in a GitHub Codespace, this command will:
- Set up the proper directory structure in /home/vscode/.catnip
- Configure environment variables for codespace operation
- Ensure sudo access for /opt/catnip directory creation

Use this command before running **catnip run --runtime codespace** for the best experience.`,
	RunE: runBootstrap,
}

func init() {
	rootCmd.AddCommand(bootstrapCmd)
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	// Configure logging
	logLevel := logger.GetLogLevelFromEnv(false)
	logger.Configure(logLevel, true)

	logger.Infof("🚀 Starting Catnip bootstrap process...")

	// Detect if we're in a GitHub Codespace
	isCodespace := detectGitHubCodespace()
	if isCodespace {
		logger.Infof("📦 GitHub Codespace detected, configuring for codespace environment...")
		if err := setupCodespaceEnvironment(); err != nil {
			return fmt.Errorf("failed to setup codespace environment: %w", err)
		}
	}

	// Install GitHub CLI
	logger.Infof("🔧 Checking GitHub CLI installation...")
	if err := ensureGitHubCLI(); err != nil {
		return fmt.Errorf("failed to install GitHub CLI: %w", err)
	}

	// Install Claude Code
	logger.Infof("🤖 Checking Claude Code installation...")
	if err := ensureClaudeCode(); err != nil {
		return fmt.Errorf("failed to install Claude Code: %w", err)
	}

	logger.Infof("✅ Bootstrap completed successfully!")
	if isCodespace {
		logger.Infof("💡 You can now run 'catnip serve' to start the server in this codespace")
		logger.Infof("💡 Or use 'catnip run --runtime codespace' to manage codespaces remotely")
	}

	return nil
}

// detectGitHubCodespace checks if we're running inside a GitHub Codespace
func detectGitHubCodespace() bool {
	// GitHub Codespaces set these environment variables
	return os.Getenv("CODESPACES") == "true" ||
		os.Getenv("CODESPACE_NAME") != "" ||
		os.Getenv("GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN") != ""
}

// setupCodespaceEnvironment configures the environment for GitHub Codespaces
func setupCodespaceEnvironment() error {
	logger.Infof("🏠 Setting up codespace directories...")

	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// Define codespace-specific paths
	catnipRoot := "/home/vscode/.catnip"
	volumeDir := "/home/vscode/.catnip/volume"
	optDir := "/opt/catnip"

	// Create user directories
	userDirs := []string{catnipRoot, volumeDir}
	for _, dir := range userDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		logger.Debugf("✅ Created directory: %s", dir)
	}

	// Create /opt/catnip with sudo and fix ownership
	logger.Debugf("🔒 Creating %s with sudo...", optDir)

	// Check if sudo is available
	if _, err := exec.LookPath("sudo"); err != nil {
		logger.Debugf("⚠️  sudo not available, skipping /opt/catnip creation")
	} else {
		// Create directory with sudo
		sudoCmd := exec.Command("sudo", "mkdir", "-p", optDir)
		if err := sudoCmd.Run(); err != nil {
			logger.Debugf("⚠️  Failed to create %s with sudo: %v", optDir, err)
		} else {
			// Fix ownership back to current user
			chownCmd := exec.Command("sudo", "chown", "-R", currentUser.Username+":"+currentUser.Username, optDir)
			if err := chownCmd.Run(); err != nil {
				logger.Debugf("⚠️  Failed to change ownership of %s: %v", optDir, err)
			} else {
				logger.Debugf("✅ Created and configured directory: %s", optDir)
			}
		}
	}

	logger.Infof("✅ Codespace environment setup completed")
	return nil
}

// ensureGitHubCLI installs GitHub CLI if not already present
func ensureGitHubCLI() error {
	// Check if gh is already installed
	if _, err := exec.LookPath("gh"); err == nil {
		// Verify it works
		versionCmd := exec.Command("gh", "--version")
		if err := versionCmd.Run(); err == nil {
			logger.Infof("✅ GitHub CLI already installed")
			return nil
		}
	}

	logger.Infof("📦 Installing GitHub CLI...")

	switch goruntime.GOOS {
	case "linux":
		return installGitHubCLILinux()
	case "darwin":
		return installGitHubCLIMacOS()
	default:
		return fmt.Errorf("unsupported operating system: %s", goruntime.GOOS)
	}
}

// installGitHubCLILinux installs GitHub CLI on Linux systems
func installGitHubCLILinux() error {
	// Use the official GitHub CLI installation script
	logger.Debugf("🔧 Installing GitHub CLI on Linux...")

	// Download and run the official install script
	installCmd := exec.Command("bash", "-c", `
		type -p curl >/dev/null || (sudo apt update && sudo apt install curl -y)
		curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
		&& sudo chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
		&& echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
		&& sudo apt update \
		&& sudo apt install gh -y
	`)

	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr

	if err := installCmd.Run(); err != nil {
		// Fallback: try using snap if available
		logger.Debugf("⚠️  APT installation failed, trying snap...")
		snapCmd := exec.Command("sudo", "snap", "install", "gh")
		snapCmd.Stdout = os.Stdout
		snapCmd.Stderr = os.Stderr
		if err := snapCmd.Run(); err != nil {
			return fmt.Errorf("failed to install GitHub CLI via both APT and snap: %w", err)
		}
	}

	logger.Infof("✅ GitHub CLI installed successfully")
	return nil
}

// installGitHubCLIMacOS installs GitHub CLI on macOS systems
func installGitHubCLIMacOS() error {
	logger.Debugf("🔧 Installing GitHub CLI on macOS...")

	// Check if Homebrew is available
	if _, err := exec.LookPath("brew"); err == nil {
		logger.Debugf("🍺 Using Homebrew to install GitHub CLI...")
		brewCmd := exec.Command("brew", "install", "gh")
		brewCmd.Stdout = os.Stdout
		brewCmd.Stderr = os.Stderr
		if err := brewCmd.Run(); err != nil {
			return fmt.Errorf("failed to install GitHub CLI via Homebrew: %w", err)
		}
	} else {
		// Fallback to downloading binary directly
		logger.Debugf("📦 Downloading GitHub CLI binary...")

		// Detect architecture
		arch := "amd64"
		if goruntime.GOARCH == "arm64" {
			arch = "arm64"
		}

		downloadCmd := exec.Command("bash", "-c", fmt.Sprintf(`
			cd /tmp
			curl -LO "https://github.com/cli/cli/releases/latest/download/gh_*_macOS_%s.tar.gz"
			tar -xzf gh_*_macOS_%s.tar.gz
			sudo cp gh_*_macOS_%s/bin/gh /usr/local/bin/
			rm -rf gh_*_macOS_%s*
		`, arch, arch, arch, arch))

		downloadCmd.Stdout = os.Stdout
		downloadCmd.Stderr = os.Stderr

		if err := downloadCmd.Run(); err != nil {
			return fmt.Errorf("failed to install GitHub CLI via direct download: %w", err)
		}
	}

	logger.Infof("✅ GitHub CLI installed successfully")
	return nil
}

// ensureClaudeCode installs Claude Code if not already present
func ensureClaudeCode() error {
	// Check if claude is already installed
	if _, err := exec.LookPath("claude"); err == nil {
		// Verify it works
		versionCmd := exec.Command("claude", "--version")
		if err := versionCmd.Run(); err == nil {
			logger.Infof("✅ Claude Code already installed")
			return nil
		}
	}

	logger.Infof("🤖 Installing Claude Code...")

	// Use the official Claude installation script
	installCmd := exec.Command("bash", "-c", "curl -fsSL claude.ai/install.sh | bash")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr

	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("failed to install Claude Code: %w", err)
	}

	// Source the shell profile to make claude available in current session
	logger.Debugf("🔄 Updating shell environment...")

	// Get current user's home directory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		// Try to source common shell profile files
		profiles := []string{
			filepath.Join(homeDir, ".bashrc"),
			filepath.Join(homeDir, ".bash_profile"),
			filepath.Join(homeDir, ".zshrc"),
			filepath.Join(homeDir, ".profile"),
		}

		for _, profile := range profiles {
			if _, err := os.Stat(profile); err == nil {
				logger.Debugf("📄 Found shell profile: %s", profile)
				// Note: We can't actually source in the current shell, but the install script should handle PATH updates
				break
			}
		}
	}

	logger.Infof("✅ Claude Code installed successfully")
	logger.Infof("💡 You may need to restart your shell or run 'source ~/.bashrc' to use the 'claude' command")

	return nil
}
