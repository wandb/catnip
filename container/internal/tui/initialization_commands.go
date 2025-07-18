package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
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
				// Channel closed, check if we got a done signal or if it was an error
				select {
				case <-doneChan:
					debugLog("StreamingOutputReader: channel closed with done signal, completing")
					return InitializationCompleteMsg{}
				default:
					debugLog("StreamingOutputReader: channel closed without done signal, assuming failure")
					return InitializationFailedMsg{Error: "Command failed - check output above for details"}
				}
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
				// Don't signal completion on error - this will keep us on the initialization page
				debugLog("ExecuteStreamingBuildCmd: build failed, not signaling completion")
				return
			} else {
				outputChan <- "✅ Build completed successfully!"
				debugLog("ExecuteStreamingBuildCmd: sending done signal")
				doneChan <- true
			}
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
func StartContainerCmd(containerService *services.ContainerService, image, name, gitRoot string, devMode bool, customPorts []string, sshEnabled bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Use custom ports if provided, otherwise use default
		ports := customPorts
		if len(ports) == 0 {
			ports = []string{"8080:8080"}
		}

		// Start the container
		if cmd, err := containerService.RunContainer(ctx, image, name, gitRoot, ports, devMode, sshEnabled); err != nil {
			return ContainerStartFailedMsg{Error: fmt.Sprintf("Failed to start container: %v\nCommand: %s", err, strings.Join(cmd, " "))}
		}

		// Container started, now monitor its health
		return ContainerStartedMsg{
			ContainerName:    name,
			ContainerService: containerService,
		}
	}
}

// MonitorContainerHealthCmd monitors container health after starting
func MonitorContainerHealthCmd(containerService *services.ContainerService, containerName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Get the container logs command
		cmd, err := containerService.GetContainerLogs(ctx, containerName, true) // follow=true
		if err != nil {
			return ContainerHealthCheckFailedMsg{Error: fmt.Sprintf("Failed to get logs command: %v", err)}
		}

		// Start streaming logs
		return StartStreamingContainerLogsCmd{Command: cmd, ContainerName: containerName, ContainerService: containerService}
	}
}

// StartStreamingContainerLogsCmd represents a command to stream container logs
type StartStreamingContainerLogsCmd struct {
	Command          *exec.Cmd
	ContainerName    string
	ContainerService *services.ContainerService
}

// ContainerHealthCheckFailedMsg indicates container health check failed
type ContainerHealthCheckFailedMsg struct {
	Error string
}

// ExecuteStreamingContainerLogsCmd streams container logs and monitors health
func ExecuteStreamingContainerLogsCmd(cmd *exec.Cmd, containerName string, containerService *services.ContainerService) tea.Cmd {
	return func() tea.Msg {
		// Create channels for streaming output
		outputChan := make(chan string, 100)
		doneChan := make(chan bool, 1)
		healthyChan := make(chan bool, 1)

		// Start a goroutine to execute the command and stream output
		go func() {
			defer close(outputChan)
			defer close(doneChan)
			defer close(healthyChan)

			debugLog("ExecuteStreamingContainerLogsCmd: starting container log streaming")

			// Create pipes for stdout and stderr
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				outputChan <- fmt.Sprintf("Error: Failed to create stdout pipe: %v", err)
				doneChan <- true
				return
			}

			stderr, err := cmd.StderrPipe()
			if err != nil {
				outputChan <- fmt.Sprintf("Error: Failed to create stderr pipe: %v", err)
				doneChan <- true
				return
			}

			// Start the command
			if err := cmd.Start(); err != nil {
				outputChan <- fmt.Sprintf("Error: Failed to start logs command: %v", err)
				doneChan <- true
				return
			}

			// Start goroutines to read stdout and stderr
			go streamReader(stdout, outputChan)
			go streamReader(stderr, outputChan)

			// Start health check goroutine
			go func() {
				ctx := context.Background()
				startTime := time.Now()
				maxWaitTime := 30 * time.Second
				checkInterval := 500 * time.Millisecond

				for {
					// Check if container is still running
					if !containerService.IsContainerRunning(ctx, containerName) {
						outputChan <- fmt.Sprintf("❌ Container %s stopped unexpectedly", containerName)
						// Kill the logs command to stop tailing
						if cmd.Process != nil {
							_ = cmd.Process.Kill()
						}
						healthyChan <- false
						return
					}

					// After 2 seconds, start checking if port 8080 is accessible
					if time.Since(startTime) > 2*time.Second {
						// Try to connect to port 8080
						conn, err := net.Dial("tcp", "localhost:8080")
						if err == nil {
							conn.Close()
							outputChan <- "✅ Container is healthy and port 8080 is accessible"
							healthyChan <- true
							return
						}
					}

					// Timeout after maxWaitTime
					if time.Since(startTime) > maxWaitTime {
						outputChan <- fmt.Sprintf("⚠️ Container health check timed out after %v", maxWaitTime)
						// Kill the logs command to stop tailing
						if cmd.Process != nil {
							_ = cmd.Process.Kill()
						}
						healthyChan <- false
						return
					}

					time.Sleep(checkInterval)
				}
			}()

			// Wait for either the command to exit or health check to complete
			select {
			case healthy := <-healthyChan:
				if healthy {
					// Container is healthy, signal success
					time.Sleep(500 * time.Millisecond) // Brief delay to show success message
					doneChan <- true
					if cmd.Process != nil {
						_ = cmd.Process.Kill() // Stop following logs
					}
				} else {
					// Container failed health check, signal failure
					doneChan <- false
				}
			case <-time.After(60 * time.Second):
				// Overall timeout
				outputChan <- "❌ Container startup timed out after 60 seconds"
				doneChan <- false
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			}
		}()

		// Return a message that will trigger the streaming reader with health monitoring
		return StartStreamingContainerLogsReader{OutputChan: outputChan, DoneChan: doneChan, ContainerName: containerName}
	}
}

// StartStreamingContainerLogsReader message to start streaming container logs
type StartStreamingContainerLogsReader struct {
	OutputChan    <-chan string
	DoneChan      <-chan bool
	ContainerName string
}

// ContainerLogsOutputMsg represents a line of container log output
type ContainerLogsOutputMsg struct {
	Line       string
	OutputChan <-chan string
	DoneChan   <-chan bool
}

// ContainerHealthyMsg indicates the container is healthy and ready
type ContainerHealthyMsg struct {
	ContainerName string
}

// StreamingContainerLogsReader reads from container logs channels and sends output messages
func StreamingContainerLogsReader(outputChan <-chan string, doneChan <-chan bool) tea.Cmd {
	return func() tea.Msg {
		debugLog("StreamingContainerLogsReader: waiting for message...")
		select {
		case line, ok := <-outputChan:
			if !ok {
				// Channel closed, check if we got a done signal or if it was an error
				select {
				case healthy := <-doneChan:
					if healthy {
						debugLog("StreamingContainerLogsReader: channel closed with healthy signal")
						return ContainerHealthyMsg{}
					} else {
						debugLog("StreamingContainerLogsReader: channel closed with unhealthy signal")
						return ContainerHealthCheckFailedMsg{Error: "Container failed during startup - check logs below for details"}
					}
				default:
					debugLog("StreamingContainerLogsReader: channel closed without done signal, container failed")
					return ContainerHealthCheckFailedMsg{Error: "Container failed during startup - check logs below for details"}
				}
			}
			// Only send non-empty lines to avoid cluttering output
			if strings.TrimSpace(line) != "" {
				debugLog("StreamingContainerLogsReader: got log line: %s", line)
				return ContainerLogsOutputMsg{Line: line, OutputChan: outputChan, DoneChan: doneChan}
			}
		case healthy := <-doneChan:
			if healthy {
				debugLog("StreamingContainerLogsReader: got healthy signal")
				return ContainerHealthyMsg{}
			} else {
				debugLog("StreamingContainerLogsReader: got unhealthy signal")
				return ContainerHealthCheckFailedMsg{Error: "Container failed during startup - check logs below for details"}
			}
		case <-time.After(100 * time.Millisecond):
			// Timeout to prevent blocking, continue reading
			debugLog("StreamingContainerLogsReader: timeout, continuing")
		}
		// Continue reading (but don't send empty lines)
		return ContainerLogsOutputMsg{Line: "", OutputChan: outputChan, DoneChan: doneChan}
	}
}
