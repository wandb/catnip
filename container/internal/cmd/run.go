package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/services"
	"github.com/vanpelt/catnip/internal/tui"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "🚀 Run a Catnip container with interactive TUI",
	Long: `# 🐱 Run Catnip Container

**Start a new Catnip container** from the production image and enter an interactive TUI.

## 📁 Repository Mounting
- If you're in a **git repository**, it will mount the repository root
- Otherwise, no directory is mounted

## 🌐 Network Access
- Container exposes **port 8080** for web access
- Automatically shuts down when you quit the TUI

## 🎯 Development Mode
Use the **--dev** flag to:
- Run the development image (**catnip-dev:dev**)
- Mount node_modules volume for faster builds
- Enable development-specific features

Use the **--refresh** flag to:
- Force refresh: rebuild dev image with **just build-dev** or pull production image from registry
- Useful for testing changes to the container setup or getting latest production image`,
	RunE: runContainer,
}

var (
	image      string
	name       string
	detach     bool
	noTUI      bool
	ports      []string
	dev        bool
	refresh    bool
	disableSSH bool
	runtime    string
	rmFlag     bool
	cpus       float64
	memoryGB   float64
	envVars    []string
)

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVarP(&image, "image", "i", "", "Container image to run")
	runCmd.Flags().StringVarP(&name, "name", "n", "", "Container name (auto-generated if not provided)")
	runCmd.Flags().BoolVarP(&detach, "detach", "d", false, "Run container in detached mode")
	runCmd.Flags().BoolVar(&noTUI, "no-tui", false, "Disable TUI and tail logs directly")
	runCmd.Flags().StringSliceVarP(&ports, "port", "p", []string{"8080:8080"}, "Port mappings")
	runCmd.Flags().BoolVar(&dev, "dev", false, "Run in development mode with dev image and node_modules volume")
	runCmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh: rebuild dev image with 'just build-dev' or pull production image from registry")
	runCmd.Flags().BoolVar(&disableSSH, "disable-ssh", false, "Disable SSH server (enabled by default on port 2222)")
	runCmd.Flags().StringVar(&runtime, "runtime", "", "Container runtime to use (docker, container, or auto-detect if not specified)")
	runCmd.Flags().BoolVar(&rmFlag, "rm", false, "Automatically remove the container when it exits (default: false - container is stopped and can be restarted)")
	runCmd.Flags().Float64Var(&cpus, "cpus", 4.0, "Number of CPUs to allocate to the container (default: 4.0)")
	runCmd.Flags().Float64Var(&memoryGB, "memory", 4.0, "Amount of memory in GB to allocate to the container (default: 4.0)")
	runCmd.Flags().StringSliceVarP(&envVars, "env", "e", nil, "Set environment variables (e.g., -e FOO=bar or -e VAR to forward from host)")
}

// cleanVersionForProduction removes the -dev suffix and v prefix from version string
func cleanVersionForProduction(version string) string {
	// Remove v prefix if present
	version = strings.TrimPrefix(version, "v")

	// Remove -dev suffix if present
	version = strings.TrimSuffix(version, "-dev")

	return version
}

func runContainer(cmd *cobra.Command, args []string) error {
	// Configure logging based on dev mode and environment
	logLevel := logger.GetLogLevelFromEnv(dev)
	logger.Configure(logLevel, dev)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// No flag validation needed for --refresh as it works with both dev and production modes

	// Check if SSH command is available
	if !disableSSH {
		if _, err := exec.LookPath("ssh"); err != nil {
			logger.Debug("SSH command not found. Disabling SSH support.")
			disableSSH = true
		}
	}

	// Handle SSH setup if not disabled
	if !disableSSH {
		if err := setupSSH(); err != nil {
			logger.Debugf("Failed to setup SSH keys (%v). Disabling SSH support.", err)
			disableSSH = true
		} else {
			// Add SSH port to ports if not already present
			sshPortPresent := false
			for _, p := range ports {
				if strings.HasPrefix(p, "2222:") || strings.HasSuffix(p, ":2222") {
					sshPortPresent = true
					break
				}
			}
			if !sshPortPresent {
				ports = append(ports, "2222:2222")
			}
		}
	}

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Get current working directory and find git root
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Find git root - operations will be relative to this if it exists
	gitRoot, isGitRepo := git.FindGitRoot(workDir)

	// Generate container name if not provided
	if name == "" {
		if isGitRepo {
			basename := filepath.Base(gitRoot)
			// Avoid double "catnip" in name
			if basename == "catnip" {
				name = "catnip"
			} else {
				name = fmt.Sprintf("catnip-%s", basename)
			}
		} else {
			// Use current directory name if not in git repo
			name = fmt.Sprintf("catnip-%s", filepath.Base(workDir))
		}
		if dev {
			name = name + "-dev"
		}
	}

	// Initialize container service with optional runtime
	containerService, err := services.NewContainerServiceWithRuntime(runtime)
	if err != nil {
		return err
	}

	// Process environment variables (handle both FOO=bar and FOO formats)
	processedEnvVars := make([]string, 0, len(envVars)+1)

	// Always pass DEBUG environment variable to container
	// In dev mode, default to DEBUG=true unless explicitly set to false
	// In production mode, forward from host environment
	debugValue := os.Getenv("DEBUG")
	if dev {
		if debugValue == "" || (strings.ToLower(debugValue) != "false" && debugValue != "0") {
			processedEnvVars = append(processedEnvVars, "DEBUG=true")
		} else {
			processedEnvVars = append(processedEnvVars, "DEBUG=false")
		}
	} else {
		if debugValue != "" {
			processedEnvVars = append(processedEnvVars, fmt.Sprintf("DEBUG=%s", debugValue))
		}
	}

	for _, envVar := range envVars {
		if strings.Contains(envVar, "=") {
			// Format: FOO=bar - use as-is
			processedEnvVars = append(processedEnvVars, envVar)
		} else {
			// Format: FOO - forward value from host environment
			if value, exists := os.LookupEnv(envVar); exists {
				processedEnvVars = append(processedEnvVars, fmt.Sprintf("%s=%s", envVar, value))
			}
			// If the environment variable doesn't exist on the host, we skip it
		}
	}

	// Determine container image
	containerImage := image
	if dev {
		containerImage = "catnip-dev:dev"
	} else if image == "" {
		// Use versioned image for production
		cleanVersion := cleanVersionForProduction(GetVersion())
		containerImage = fmt.Sprintf("wandb/catnip:%s", cleanVersion)
	}

	// For non-TTY mode, handle initialization directly
	if !isTTY() && !containerService.IsContainerRunning(ctx, name) {
		// Check if we need to build/pull image
		if dev {
			if !isGitRepo {
				return fmt.Errorf("development mode requires a git repository")
			}
			if !containerService.ImageExists(ctx, containerImage) || refresh {
				fmt.Printf("Running 'just build-dev' in container directory...\n")
				if err := runBuildDevDirect(gitRoot); err != nil {
					return fmt.Errorf("build failed: %w", err)
				}
			}
		} else {
			if !containerService.ImageExists(ctx, containerImage) || refresh {
				fmt.Printf("Running 'docker pull %s'...\n", containerImage)
				if err := runDockerPullDirect(ctx, containerService, containerImage); err != nil {
					return fmt.Errorf("pull failed: %w", err)
				}
			}
		}

		// Start the container
		fmt.Printf("Starting container '%s'...\n", name)
		workDirForContainer := workDir
		if isGitRepo {
			workDirForContainer = gitRoot
		}
		if cmd, err := containerService.RunContainer(ctx, containerImage, name, workDirForContainer, ports, dev, !disableSSH, rmFlag, cpus, memoryGB, processedEnvVars); err != nil {
			return fmt.Errorf("failed to run %s: %w", cmd, err)
		}
		fmt.Printf("Container started successfully!\n")
	}

	// If detached mode, just exit
	if detach {
		fmt.Printf("Container '%s' is running in detached mode.\n", name)
		fmt.Printf("Use 'catnip attach %s' to connect to it later.\n", name)
		return nil
	}

	// If no-tui mode, tail logs directly
	if noTUI {
		fmt.Printf("Tailing logs for container '%s' (press Ctrl+C to stop)...\n", name)
		return tailContainerLogs(ctx, containerService, name)
	}

	// Check if we have a TTY, if not, fallback to no-tui mode
	if !isTTY() {
		fmt.Printf("No TTY detected, falling back to log tailing mode...\n")
		fmt.Printf("Tailing logs for container '%s' (press Ctrl+C to stop)...\n", name)
		return tailContainerLogs(ctx, containerService, name)
	}

	// Start the TUI - it will handle all initialization and container management
	workDirForTUI := workDir
	if isGitRepo {
		workDirForTUI = gitRoot
	}
	tuiApp := tui.NewApp(containerService, name, workDirForTUI, containerImage, dev, refresh, ports, !disableSSH, GetVersion(), rmFlag)
	finalContainerName, err := tuiApp.Run(ctx, workDirForTUI, ports)
	if err != nil {
		// Clean up container on TUI error
		fmt.Printf("Stopping container '%s'...\n", finalContainerName)
		_ = containerService.StopContainer(ctx, finalContainerName)
		return fmt.Errorf("TUI error: %w", err)
	}

	// Clean up container when TUI exits normally
	if rmFlag {
		fmt.Printf("Stopping and removing container '%s'...\n", finalContainerName)
		if err := containerService.StopContainer(ctx, finalContainerName); err != nil {
			fmt.Printf("Warning: Failed to stop container: %v\n", err)
		} else {
			if err := containerService.RemoveContainer(ctx, finalContainerName); err != nil {
				fmt.Printf("Warning: Failed to remove container: %v\n", err)
			} else {
				fmt.Printf("Container stopped and removed successfully.\n")
			}
		}
	} else {
		fmt.Printf("Stopping container '%s'...\n", finalContainerName)
		if err := containerService.StopContainer(ctx, finalContainerName); err != nil {
			fmt.Printf("Warning: Failed to stop container: %v\n", err)
		} else {
			fmt.Printf("Container stopped successfully.\n")
		}
	}

	return nil
}

func tailContainerLogs(ctx context.Context, containerService *services.ContainerService, containerName string) error {
	// Get the logs command with follow flag
	cmd, err := containerService.GetContainerLogs(ctx, containerName, true)
	if err != nil {
		return fmt.Errorf("failed to get logs command: %w", err)
	}

	// Set up stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Set up stderr pipe
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start logs command: %w", err)
	}

	// Set up cleanup
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	// Create channels for output
	outputChan := make(chan string)
	errorChan := make(chan error)

	// Start goroutines to read stdout and stderr
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case outputChan <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errorChan <- fmt.Errorf("stdout scanner error: %w", err)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			select {
			case outputChan <- "[STDERR] " + scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errorChan <- fmt.Errorf("stderr scanner error: %w", err)
		}
	}()

	// Print logs until context is cancelled
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nStopping log tail...")
			return nil
		case line := <-outputChan:
			fmt.Println(line) // Keep raw output for log tailing
		case err := <-errorChan:
			return err
		}
	}
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// runBuildDevDirect runs 'just build-dev' directly without TUI
func runBuildDevDirect(gitRoot string) error {
	// Use the container service to get the build command
	containerService, err := services.NewContainerServiceWithRuntime(runtime)
	if err != nil {
		return fmt.Errorf("failed to create container service: %w", err)
	}

	cmd, err := containerService.BuildDevImage(context.Background(), gitRoot)
	if err != nil {
		return fmt.Errorf("failed to create build command: %w", err)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runDockerPullDirect runs 'docker pull' directly without TUI
func runDockerPullDirect(ctx context.Context, containerService *services.ContainerService, image string) error {
	cmd, err := containerService.PullImage(ctx, image)
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// setupSSH generates SSH key pair if needed and updates SSH config
func setupSSH() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	privateKeyPath := filepath.Join(sshDir, "catnip_remote")
	publicKeyPath := filepath.Join(sshDir, "catnip_remote.pub")

	// Create .ssh directory if it doesn't exist
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Check if SSH key already exists
	if _, err := os.Stat(privateKeyPath); err == nil {
		fmt.Printf("Using existing SSH key: %s\n", privateKeyPath)
	} else {
		// Generate SSH key pair
		fmt.Println("Generating SSH key pair for Catnip...")
		privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return fmt.Errorf("failed to generate private key: %w", err)
		}

		// Save private key
		privateKeyPEM := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		}
		privateKeyFile, err := os.OpenFile(privateKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("failed to create private key file: %w", err)
		}
		defer privateKeyFile.Close()

		if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
			return fmt.Errorf("failed to write private key: %w", err)
		}

		// Generate SSH public key
		pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
		if err != nil {
			return fmt.Errorf("failed to generate public key: %w", err)
		}

		// Get current user for comment
		currentUser, _ := user.Current()
		comment := fmt.Sprintf("catnip@%s", currentUser.Username)
		publicKeyData := fmt.Sprintf("%s %s\n", strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))), comment)

		// Save public key
		if err := os.WriteFile(publicKeyPath, []byte(publicKeyData), 0644); err != nil {
			return fmt.Errorf("failed to write public key: %w", err)
		}

		fmt.Printf("SSH key pair generated: %s\n", privateKeyPath)
	}

	// Update SSH config
	if err := updateSSHConfig(homeDir); err != nil {
		return fmt.Errorf("failed to update SSH config: %w", err)
	}

	return nil
}

// updateSSHConfig adds or updates the catnip host entry in ~/.ssh/config
func updateSSHConfig(homeDir string) error {
	configPath := filepath.Join(homeDir, ".ssh", "config")

	// Read existing config
	content, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read SSH config: %w", err)
	}

	contentStr := string(content)
	startMarker := "# BEGIN CATNIP MANAGED BLOCK"
	endMarker := "# END CATNIP MANAGED BLOCK"

	// Check if catnip managed block already exists
	if strings.Contains(contentStr, startMarker) {
		// Find and replace the existing block
		startIdx := strings.Index(contentStr, startMarker)
		endIdx := strings.Index(contentStr, endMarker)
		if endIdx != -1 {
			endIdx += len(endMarker)
			// Get current username
			currentUser, err := user.Current()
			if err != nil {
				return fmt.Errorf("failed to get current user: %w", err)
			}

			// Prepare new catnip entry with fences
			newEntry := fmt.Sprintf(`%s
Host catnip
  HostName 127.0.0.1
  Port 2222
  User %s
  IdentityFile ~/.ssh/catnip_remote
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  SendEnv WORKDIR
%s`, startMarker, currentUser.Username, endMarker)

			// Replace the old block with the new one
			newContent := contentStr[:startIdx] + newEntry + contentStr[endIdx:]

			// Write the updated content
			if err := os.WriteFile(configPath, []byte(newContent), 0600); err != nil {
				return fmt.Errorf("failed to write SSH config: %w", err)
			}

			fmt.Println("Updated existing catnip SSH config entry")
			return nil
		}
	}

	// Get current username
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// Prepare catnip host entry with comment fences
	catnipEntry := fmt.Sprintf(`
%s
Host catnip
  HostName 127.0.0.1
  Port 2222
  User %s
  IdentityFile ~/.ssh/catnip_remote
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  SendEnv WORKDIR
%s
`, startMarker, currentUser.Username, endMarker)

	// Append to config
	configFile, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open SSH config: %w", err)
	}
	defer configFile.Close()

	if _, err := configFile.WriteString(catnipEntry); err != nil {
		return fmt.Errorf("failed to write SSH config: %w", err)
	}

	fmt.Println("Added catnip SSH config entry")
	return nil
}
