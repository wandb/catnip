package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/gitutil"
	"github.com/vanpelt/catnip/internal/services"
	"github.com/vanpelt/catnip/internal/tui"
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
	image   string
	name    string
	detach  bool
	noTUI   bool
	ports   []string
	dev     bool
	refresh bool
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// No flag validation needed for --refresh as it works with both dev and production modes

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

	// Find git root early - all our operations should be relative to this
	gitRoot, isGitRepo := gitutil.FindGitRoot(workDir)
	if !isGitRepo {
		return fmt.Errorf("not in a git repository")
	}

	// Generate container name if not provided
	if name == "" {
		basename := filepath.Base(gitRoot)
		name = fmt.Sprintf("catnip-%s", basename)
		if dev {
			name = name + "-dev"
		}
	}

	// Initialize container service
	containerService, err := services.NewContainerService()
	if err != nil {
		return err
	}

	// Determine container image
	containerImage := image
	if dev {
		containerImage = "catnip-dev:dev"
	} else if image == "" {
		// Use versioned image for production
		cleanVersion := cleanVersionForProduction(version)
		containerImage = fmt.Sprintf("wandb/catnip:%s", cleanVersion)
	}

	// For non-TTY mode, handle initialization directly
	if !isTTY() && !containerService.IsContainerRunning(ctx, name) {
		// Check if we need to build/pull image
		if dev {
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
		if err := containerService.RunContainer(ctx, containerImage, name, gitRoot, ports, dev); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
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
	tuiApp := tui.NewApp(containerService, name, gitRoot, containerImage, dev, refresh, ports)
	if err := tuiApp.Run(ctx, gitRoot, ports); err != nil {
		// Clean up container on TUI error
		fmt.Printf("Stopping container '%s'...\n", name)
		_ = containerService.StopContainer(ctx, name)
		return fmt.Errorf("TUI error: %w", err)
	}

	// Clean up container when TUI exits normally
	fmt.Printf("Stopping container '%s'...\n", name)
	if err := containerService.StopContainer(ctx, name); err != nil {
		fmt.Printf("Warning: Failed to stop container: %v\n", err)
	} else {
		fmt.Printf("Container stopped successfully.\n")
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
			fmt.Println(line)
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
	containerService, err := services.NewContainerService()
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
