package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vanpelt/catnip/internal/services"
)

// InitializationStreamMsg represents a line of output from initialization
type InitializationStreamMsg struct {
	Line string
}

// InitializationProcessMsg represents the start of an initialization process
type InitializationProcessMsg struct {
	ContainerService *services.ContainerService
	Image            string
	GitRoot          string
	DevMode          bool
	RefreshFlag      bool
	ContainerName    string
}

// StartInitializationProcess starts the initialization process
func StartInitializationProcess(m *Model) tea.Cmd {
	return func() tea.Msg {
		return InitializationProcessMsg{
			ContainerService: m.containerService,
			Image:            m.containerImage,
			GitRoot:          m.gitRoot,
			DevMode:          m.devMode,
			RefreshFlag:      m.refreshFlag,
			ContainerName:    m.containerName,
		}
	}
}

// ProcessInitialization handles the initialization process
func ProcessInitialization(msg InitializationProcessMsg) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// First check if container is already running
		if msg.ContainerService.IsContainerRunning(ctx, msg.ContainerName) {
			return InitializationStatusMsg{
				Status:         "Container already running",
				Action:         "Connecting to existing instance",
				SkipToOverview: true,
			}
		}

		if msg.DevMode {
			// Check if dev image exists or if rebuild is forced
			devImage := "catnip-dev:dev"
			if !msg.ContainerService.ImageExists(ctx, devImage) || msg.RefreshFlag {
				return InitializationStatusMsg{
					Status: "Building development image...",
					Action: "Running just build-dev",
				}
			}
		} else {
			// Check if production image exists or if refresh is forced
			if !msg.ContainerService.ImageExists(ctx, msg.Image) || msg.RefreshFlag {
				return InitializationStatusMsg{
					Status: "Pulling container image...",
					Action: fmt.Sprintf("Pulling %s", msg.Image),
				}
			}
		}

		// Image exists, need to start container
		return InitializationStatusMsg{
			Status:         "Starting container...",
			Action:         "Starting container",
			StartContainer: true,
		}
	}
}

// RunDockerPullStream runs docker pull with real-time streaming output
func RunDockerPullStream(containerService *services.ContainerService, image string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Get the pull command
		cmd, err := containerService.PullImage(ctx, image)
		if err != nil {
			return InitializationFailedMsg{Error: err.Error()}
		}

		// Start streaming pull command that sends output via tea.Program
		return StartStreamingBuildCmd{Command: cmd}
	}
}

// RunDevBuildStream runs just build-dev with real-time TTY streaming
func RunDevBuildStream(containerService *services.ContainerService, gitRoot string) tea.Cmd {
	return func() tea.Msg {
		// Use the container service to get the build command
		cmd, err := containerService.BuildDevImage(context.Background(), gitRoot)
		if err != nil {
			return InitializationFailedMsg{Error: fmt.Sprintf("Failed to create build command: %v", err)}
		}

		// Set up environment for better output
		cmd.Env = append(os.Environ(), "TERM=xterm-256color", "DOCKER_BUILDKIT=1")

		// Start streaming build command that sends output via tea.Program
		return StartStreamingBuildCmd{Command: cmd}
	}
}

// StreamingOutputReader creates a command that reads from a channel and sends output messages
func StreamingOutputReader(outputChan <-chan string, doneChan <-chan bool) tea.Cmd {
	return func() tea.Msg {
		debugLog("StreamingOutputReader: waiting for message...")
		select {
		case line, ok := <-outputChan:
			if !ok {
				// Channel closed, command finished
				debugLog("StreamingOutputReader: channel closed, completing")
				return InitializationCompleteMsg{}
			}
			if line != "" {
				// Send the output line with continuation info
				debugLog("StreamingOutputReader: got line: %s", line)
				return InitializationOutputMsg{Line: line, OutputChan: outputChan, DoneChan: doneChan}
			}
		case <-doneChan:
			debugLog("StreamingOutputReader: got done signal, completing")
			return InitializationCompleteMsg{}
		case <-time.After(100 * time.Millisecond):
			// Timeout to prevent blocking, continue reading
			debugLog("StreamingOutputReader: timeout, continuing")
		}
		// Continue reading by returning a continue message
		return InitializationContinueStreamingMsg{OutputChan: outputChan, DoneChan: doneChan}
	}
}

// InitializationCompleteWithOutputMsg represents completion with output lines
type InitializationCompleteWithOutputMsg struct {
	Output []string
}

// StartStreamingBuildCmd represents a streaming build command to execute
type StartStreamingBuildCmd struct {
	Command *exec.Cmd
}

// StartStreamingReader represents a message to start the streaming reader
type StartStreamingReader struct {
	OutputChan <-chan string
	DoneChan   <-chan bool
}

// ExecuteStreamingBuildCmd executes a command with real-time streaming output
func ExecuteStreamingBuildCmd(cmd *exec.Cmd) tea.Cmd {
	return func() tea.Msg {
		// Create channels for streaming output
		outputChan := make(chan string, 100)
		doneChan := make(chan bool, 1)

		// Start a goroutine to execute the command and stream output
		go func() {
			defer close(outputChan)
			defer close(doneChan)

			debugLog("ExecuteStreamingBuildCmd: starting command execution")

			// Set up environment for better output with TTY support
			cmd.Env = append(os.Environ(),
				"TERM=xterm-256color",
				"DOCKER_BUILDKIT=1",
				"FORCE_COLOR=1",
				"CLICOLOR_FORCE=1")

			// Create pipes for stdout and stderr
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				debugLog("ExecuteStreamingBuildCmd: failed to create stdout pipe: %v", err)
				outputChan <- fmt.Sprintf("Error: Failed to create stdout pipe: %v", err)
				doneChan <- true
				return
			}

			stderr, err := cmd.StderrPipe()
			if err != nil {
				debugLog("ExecuteStreamingBuildCmd: failed to create stderr pipe: %v", err)
				outputChan <- fmt.Sprintf("Error: Failed to create stderr pipe: %v", err)
				doneChan <- true
				return
			}

			// Start the command
			if err := cmd.Start(); err != nil {
				debugLog("ExecuteStreamingBuildCmd: failed to start command: %v", err)
				outputChan <- fmt.Sprintf("Error: Failed to start command: %v", err)
				doneChan <- true
				return
			}

			// Start goroutines to read stdout and stderr
			go streamReader(stdout, outputChan)
			go streamReader(stderr, outputChan)

			// Wait for the command to complete
			err = cmd.Wait()
			debugLog("ExecuteStreamingBuildCmd: command completed with error: %v", err)

			if err != nil {
				outputChan <- fmt.Sprintf("Build failed with error: %v", err)
			} else {
				outputChan <- "✅ Build completed successfully!"
			}

			debugLog("ExecuteStreamingBuildCmd: sending done signal")
			doneChan <- true
		}()

		// Return a message that will trigger the streaming reader command
		return StartStreamingReader{OutputChan: outputChan, DoneChan: doneChan}
	}
}

// streamReader reads from a reader and sends lines to a channel
func streamReader(reader io.Reader, outputChan chan<- string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			outputChan <- line
		}
	}
}

// StartContainerCmd starts the container after initialization
func StartContainerCmd(containerService *services.ContainerService, image, name, gitRoot string, devMode bool, customPorts []string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Use custom ports if provided, otherwise use default
		ports := customPorts
		if len(ports) == 0 {
			ports = []string{"8080:8080"}
		}

		// Start the container
		if err := containerService.RunContainer(ctx, image, name, gitRoot, ports, devMode); err != nil {
			return ContainerStartFailedMsg{Error: err.Error()}
		}

		return ContainerStartedMsg{}
	}
}
