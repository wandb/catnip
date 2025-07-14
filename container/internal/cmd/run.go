package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/services"
	"github.com/vanpelt/catnip/internal/tui"
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
- Enable development-specific features`,
	RunE: runContainer,
}

var (
	image      string
	name       string
	detach     bool
	noTUI      bool
	ports      []string
	dev        bool
)

func init() {
	rootCmd.AddCommand(runCmd)
	
	runCmd.Flags().StringVarP(&image, "image", "i", "ghcr.io/wandb/catnip:latest", "Container image to run")
	runCmd.Flags().StringVarP(&name, "name", "n", "", "Container name (auto-generated if not provided)")
	runCmd.Flags().BoolVarP(&detach, "detach", "d", false, "Run container in detached mode")
	runCmd.Flags().BoolVar(&noTUI, "no-tui", false, "Disable TUI and tail logs directly")
	runCmd.Flags().StringSliceVarP(&ports, "port", "p", []string{"8080:8080"}, "Port mappings")
	runCmd.Flags().BoolVar(&dev, "dev", false, "Run in development mode with dev image and node_modules volume")
}

func runContainer(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Get current working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Generate container name if not provided
	if name == "" {
		basename := filepath.Base(workDir)
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

	// Override image for dev mode
	containerImage := image
	if dev {
		containerImage = "catnip-dev:dev"
	}
	
	// Check if container is already running
	if containerService.IsContainerRunning(ctx, name) {
		fmt.Printf("Container '%s' is already running. Connecting to existing instance...\n", name)
	} else {
		mode := "production"
		if dev {
			mode = "development"
		}
		fmt.Printf("Starting container '%s' in %s mode from image '%s'...\n", name, mode, containerImage)
		
		// Start the container
		if err := containerService.RunContainer(ctx, containerImage, name, workDir, ports, dev); err != nil {
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

	// Start the TUI
	tuiApp := tui.NewApp(containerService, name, workDir)
	if err := tuiApp.Run(ctx, workDir); err != nil {
		// Clean up container on TUI error
		fmt.Printf("Stopping container '%s'...\n", name)
		containerService.StopContainer(ctx, name)
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
			cmd.Process.Kill()
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