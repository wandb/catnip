package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vanpelt/catnip/internal/services"
)

// isCustomImage returns true if the image was manually specified by the user or dev mode is enabled
// In dev mode, the image is fixed (catnip-dev:dev) but is conceptually a custom build; treat it as custom
func isCustomImage(image string, devMode bool, cliVersion string) bool {
	if devMode {
		return true
	}
	// Default image we compute is wandb/catnip:<clean(cliVersion)>
	// If image matches that pattern and tag equals cleaned CLI version, it's not custom
	defaultTag := strings.TrimPrefix(cliVersion, "v")
	defaultTag = strings.TrimSuffix(defaultTag, "-dev")
	expected := "wandb/catnip:" + defaultTag
	return image != expected
}

// parseImageAndTag splits an image string into name and tag
// Examples: "wandb/catnip:1.2.3" -> ("wandb/catnip", "1.2.3"), "catnip:latest" -> ("catnip", "latest")
func parseImageAndTag(image string) (string, string) {
	if idx := strings.LastIndex(image, ":"); idx != -1 && idx > strings.LastIndex(image, "/") {
		return image[:idx], image[idx+1:]
	}
	return image, "latest"
}

// semverCompare compares two version tags using a permissive semver-ish comparison
// Returns 1 if a > b, -1 if a < b, 0 if equal; if non-comparable, returns 2
func semverCompare(a, b string) int {
	normalize := func(s string) []int {
		s = strings.TrimPrefix(s, "v")
		// Drop any pre-release/build metadata for comparison
		for i, ch := range s {
			if ch == '-' || ch == '+' {
				s = s[:i]
				break
			}
		}
		parts := strings.Split(s, ".")
		result := make([]int, 3)
		// Limit to first 3 parts
		if len(parts) > 3 {
			parts = parts[:3]
		}
		for i, part := range parts {
			// best-effort parse
			n := 0
			for _, ch := range part {
				if ch >= '0' && ch <= '9' {
					n = n*10 + int(ch-'0')
				} else {
					break
				}
			}
			result[i] = n //nolint:gosec // Safe: parts is truncated to max 3 elements above
		}
		return result
	}

	av := normalize(a)
	bv := normalize(b)
	// If both look like numbers (any non-zero or any digits seen), compare
	for i := 0; i < 3; i++ {
		if av[i] > bv[i] {
			return 1
		}
		if av[i] < bv[i] {
			return -1
		}
	}
	// Equal numerically; treat as equal
	return 0
}

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

// StreamingTTYReader reads from a byte channel and sends PTY output messages
func StreamingTTYReader(outputChan <-chan []byte, doneChan <-chan bool) tea.Cmd {
	return func() tea.Msg {
		debugLog("StreamingTTYReader: waiting for message...")
		select {
		case data, ok := <-outputChan:
			if !ok {
				select {
				case <-doneChan:
					debugLog("StreamingTTYReader: channel closed with done signal")
					return InitializationCompleteMsg{}
				default:
					debugLog("StreamingTTYReader: channel closed without done signal")
					return InitializationFailedMsg{Error: "Command failed - check output above for details"}
				}
			}
			if len(data) > 0 {
				return InitializationTTYOutputMsg{Data: data, OutputChan: outputChan, DoneChan: doneChan}
			}
		case <-doneChan:
			debugLog("StreamingTTYReader: got done signal")
			return InitializationCompleteMsg{}
		case <-time.After(100 * time.Millisecond):
		}
		return InitializationContinueTTYMsg{OutputChan: outputChan, DoneChan: doneChan}
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
		outputChan := make(chan []byte, 100)
		doneChan := make(chan bool, 1)

		go func() {
			defer close(outputChan)
			defer close(doneChan)

			debugLog("ExecuteStreamingBuildCmd: starting command execution with PTY")

			cmd.Env = append(os.Environ(),
				"TERM=xterm-256color",
				"DOCKER_BUILDKIT=1",
				"FORCE_COLOR=1",
				"CLICOLOR_FORCE=1")

			// Use PTY streaming for all commands to ensure proper terminal handling
			ptmx, err := pty.Start(cmd)
			if err != nil {
				outputChan <- []byte(fmt.Sprintf("Error: Failed to start command: %v", err))
				doneChan <- true
				return
			}
			defer func() { _ = ptmx.Close() }()

			buf := make([]byte, 1024)
			for {
				n, rErr := ptmx.Read(buf)
				if n > 0 {
					data := make([]byte, n)
					copy(data, buf[:n])
					outputChan <- data
				}
				if rErr != nil {
					break
				}
			}

			if err := cmd.Wait(); err != nil {
				// When command fails, try to capture any remaining output
				// The PTY should have captured most output, but show error details
				outputChan <- []byte(fmt.Sprintf("Command failed with error: %v", err))
				// Ensure we emit a terminal reset to avoid leaving the user's terminal in a weird state
				outputChan <- []byte("\x1b[0m\x1b[?7h\x1b[?25h")
				return
			}

			outputChan <- []byte("✅ Command completed successfully!\n")
			// Emit a terminal reset sequence to leave terminal in a good state
			outputChan <- []byte("\x1b[0m\x1b[?7h\x1b[?25h")
			doneChan <- true
		}()

		return StartStreamingTTYReader{OutputChan: outputChan, DoneChan: doneChan}
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
func StartContainerCmd(m *Model) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		containerService := m.containerService
		image := m.containerImage
		name := m.containerName
		gitRoot := m.gitRoot
		devMode := m.devMode
		customPorts := m.customPorts
		sshEnabled := m.sshEnabled
		rmFlag := m.rmFlag
		cliVersion := m.version

		// Use custom ports if provided, otherwise use default
		ports := customPorts
		if len(ports) == 0 {
			ports = []string{"6369:6369"}
		}

		// Check for existing containers and validate compatibility
		if containerService.IsContainerRunning(ctx, name) {
			// Current target container is running; proceed
		} else {
			if runningName, runningImage, ok := containerService.FindRunningCatnipContainer(ctx); ok {
				// Found another catnip container running - validate it's compatible
				if isContainerCompatible(runningName, name, runningImage, cliVersion, m) {
					// Compatible container found, connect to it
					return ContainerStartedMsg{
						ContainerName:    runningName,
						ContainerService: containerService,
					}
				} else {
					// Incompatible container found - stop it and create new one
					debugLog("Found incompatible running container %s, stopping it", runningName)
					_ = containerService.StopContainer(ctx, runningName)
					_ = containerService.RemoveContainer(ctx, runningName)
				}
			}
		}

		// Decide if we should force removal of an existing stopped container
		rmEffective := rmFlag
		if containerService.ContainerExists(ctx, name) && !containerService.IsContainerRunning(ctx, name) {
			// If custom image was specified by the user, always force remove
			if isCustomImage(image, devMode, cliVersion) {
				rmEffective = true
			} else {
				// Compare desired image tag with the existing container's image tag
				if existingImage, err := containerService.GetContainerImageForName(ctx, name); err == nil {
					_, desiredTag := parseImageAndTag(image)
					_, existingTag := parseImageAndTag(existingImage)

					// Check for dev/production mismatch
					if (existingTag == "dev" && !devMode) || (existingTag != "dev" && devMode) {
						debugLog("Dev/production mode mismatch: existing=%s, devMode=%v", existingTag, devMode)
						rmEffective = true
					} else if desiredTag != existingTag {
						// If tags differ and desired is newer (or non-equal), force remove
						// Try semantic compare; if not comparable, prefer desired
						switch semverCompare(desiredTag, existingTag) {
						case 1:
							rmEffective = true
						case 0:
							// equal - no change
						case -1:
							// desired older than existing; keep existing to start
						default:
							// unknown result, be conservative and remove
							rmEffective = true
						}
					}
				}
			}
		}

		// Start the container
		if cmd, err := containerService.RunContainer(ctx, image, name, gitRoot, ports, devMode, sshEnabled, rmEffective, 4.0, 4.0, m.envVars, m.dind); err != nil {
			// Parse the error to extract the base error and output
			errStr := err.Error()
			cmdStr := strings.Join(cmd, " ")

			// Enhanced error handling with specific cases
			errorMsg := detectSpecificErrors(errStr, cmdStr, image, containerService)
			if errorMsg != "" {
				return ContainerStartFailedMsg{Error: errorMsg}
			}

			// Handle "container already exists" error gracefully
			if strings.Contains(errStr, "already exists") || strings.Contains(errStr, "exists:") {
				// Check if the existing container is running
				if containerService.IsContainerRunning(ctx, name) {
					// Container is already running, skip to success
					return ContainerStartedMsg{
						ContainerName:    name,
						ContainerService: containerService,
					}
				}

				// Container exists but isn't running, try to start it
				if err := containerService.StartContainer(ctx, name); err != nil {
					// Starting the existing container failed, remove and recreate
					_ = containerService.StopContainer(ctx, name)   // Stop if partially running
					_ = containerService.RemoveContainer(ctx, name) // Remove the container

					// Give it a moment to clean up
					time.Sleep(500 * time.Millisecond)

					// Try to create a new container
					if cmd, err := containerService.RunContainer(ctx, image, name, gitRoot, ports, devMode, sshEnabled, rmFlag, 4.0, 4.0, m.envVars, m.dind); err != nil {
						// Still failed after cleanup, report the error
						errStr = err.Error()
						cmdStr = strings.Join(cmd, " ")
					} else {
						// Success after cleanup
						return ContainerStartedMsg{
							ContainerName:    name,
							ContainerService: containerService,
						}
					}
				} else {
					// Successfully started existing container
					return ContainerStartedMsg{
						ContainerName:    name,
						ContainerService: containerService,
					}
				}
			}

			// Check if the error already contains "Output:" section
			if strings.Contains(errStr, "\nOutput:") {
				// Replace the error format to put Command first
				parts := strings.Split(errStr, "\nOutput:")
				baseErr := parts[0]
				output := ""
				if len(parts) > 1 {
					output = parts[1]
				}
				return ContainerStartFailedMsg{Error: fmt.Sprintf("%s\nCommand: %s\nOutput: %s", baseErr, cmdStr, output)}
			} else {
				// Simple error without output
				return ContainerStartFailedMsg{Error: fmt.Sprintf("%s\nCommand: %s", errStr, cmdStr)}
			}
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

					// After 2 seconds, start checking if port 6369 is accessible
					if time.Since(startTime) > 2*time.Second {
						// Try to connect to port 6369
						conn, err := net.Dial("tcp", "localhost:6369")
						if err == nil {
							conn.Close()
							outputChan <- "✅ Container is healthy and port 6369 is accessible"
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

			// Ensure terminal reset sequences are emitted at the end of streaming
			outputChan <- "\x1b[0m\x1b[?7h\x1b[?25h"
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

// VersionCheckMsg indicates the result of a version check
type VersionCheckMsg struct {
	UpgradeAvailable bool
	ContainerVersion string
	CLIVersion       string
}

// CheckContainerVersionCmd checks if the container version differs from CLI version
func CheckContainerVersionCmd(cliVersion string, m *Model) tea.Cmd {
	return func() tea.Msg {
		baseURL := m.getBaseURL("") // Use model's configured port
		client := m.createAuthenticatedClient(2 * time.Second)
		containerVersionInfo, err := fetchContainerVersion(baseURL, client)
		if err != nil {
			// If we can't fetch the version, don't show upgrade warning
			debugLog("CheckContainerVersionCmd: failed to fetch container version: %v", err)
			return VersionCheckMsg{
				UpgradeAvailable: false,
				ContainerVersion: "unknown",
				CLIVersion:       cliVersion,
			}
		}

		upgradeAvailable := compareVersions(cliVersion, containerVersionInfo.Version)
		debugLog("CheckContainerVersionCmd: CLI=%s, Container=%s, UpgradeAvailable=%t",
			cliVersion, containerVersionInfo.Version, upgradeAvailable)

		return VersionCheckMsg{
			UpgradeAvailable: upgradeAvailable,
			ContainerVersion: containerVersionInfo.Version,
			CLIVersion:       cliVersion,
		}
	}
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

// detectSpecificErrors analyzes container start errors and provides specific guidance
func detectSpecificErrors(errStr, cmdStr, image string, containerService *services.ContainerService) string {
	errLower := strings.ToLower(errStr)

	// Check for Docker not running
	if strings.Contains(errLower, "cannot connect to the docker daemon") ||
		strings.Contains(errLower, "docker daemon is not running") ||
		strings.Contains(errLower, "connection refused") ||
		(containerService.GetRuntime() == services.RuntimeDocker &&
			(strings.Contains(errLower, "no such file or directory") ||
				strings.Contains(errLower, "command not found"))) {

		return fmt.Sprintf(`Docker is not running or not accessible.

🔧 To fix this:
• Start Docker Desktop (macOS/Windows)
• Or start the Docker daemon (Linux): sudo systemctl start docker
• Make sure your user is in the docker group (Linux): sudo usermod -aG docker $USER

Command: %s
Output: %s`, cmdStr, extractOutput(errStr))
	}

	// Check for missing or inaccessible image
	if strings.Contains(errLower, "unable to find image") ||
		strings.Contains(errLower, "pull access denied") ||
		strings.Contains(errLower, "repository does not exist") ||
		strings.Contains(errLower, "no such image") ||
		strings.Contains(errLower, "manifest unknown") ||
		strings.Contains(errLower, "401 unauthorized") {

		runtime := string(containerService.GetRuntime())
		return fmt.Sprintf(`Container image '%s' is not available locally and could not be pulled.

🔧 To fix this:
• Try manually pulling the image: %s pull %s
• Check if the image name and tag are correct
• If it's a private image, make sure you're authenticated

Command: %s
Output: %s`, image, runtime, image, cmdStr, extractOutput(errStr))
	}

	// Check for port already in use
	if strings.Contains(errLower, "port is already allocated") ||
		strings.Contains(errLower, "bind: address already in use") {

		return fmt.Sprintf(`Port conflict - another service is using the required ports.

🔧 To fix this:
• Stop other containers using the same ports
• Use different ports with the --port flag
• Check what's using the ports: lsof -i :8080

Command: %s
Output: %s`, cmdStr, extractOutput(errStr))
	}

	// Check for insufficient resources
	if strings.Contains(errLower, "insufficient memory") ||
		strings.Contains(errLower, "not enough memory") ||
		strings.Contains(errLower, "no space left on device") {

		return fmt.Sprintf(`Insufficient system resources to start the container.

🔧 To fix this:
• Free up disk space or memory
• Reduce resource limits with --cpus and --memory flags
• Clean up unused Docker images: docker system prune

Command: %s
Output: %s`, cmdStr, extractOutput(errStr))
	}

	return "" // No specific error detected, use generic handling
}

// extractOutput extracts the "Output:" section from an error string, preserving useful details
func extractOutput(errStr string) string {
	if strings.Contains(errStr, "\nOutput:") {
		parts := strings.Split(errStr, "\nOutput:")
		if len(parts) > 1 {
			// Don't use TrimSpace here as it might remove important newlines
			output := strings.TrimPrefix(parts[1], " ")
			return output
		}
		// If no output section, return the full error
		return strings.TrimSpace(parts[0])
	}
	return strings.TrimSpace(errStr)
}

// isContainerCompatible checks if a running container is compatible with the desired container
func isContainerCompatible(runningName, desiredName, runningImage, cliVersion string, m *Model) bool {
	// Extract base names (without -dev suffix) for comparison
	runningBaseName := strings.TrimSuffix(runningName, "-dev")
	desiredBaseName := strings.TrimSuffix(desiredName, "-dev")

	// Names must match (ignoring -dev suffix)
	if runningBaseName != desiredBaseName {
		debugLog("Container name mismatch: running=%s, desired=%s", runningBaseName, desiredBaseName)
		return false
	}

	// Check version compatibility if we can reach the container API
	if err := checkRunningContainerVersion(cliVersion, m); err != nil {
		debugLog("Container version incompatible: %v", err)
		return false
	}

	debugLog("Container %s is compatible", runningName)
	return true
}

// checkRunningContainerVersion checks if the running container version matches CLI version
func checkRunningContainerVersion(cliVersion string, m *Model) error {
	baseURL := m.getBaseURL("") // Use model's configured port
	client := m.createAuthenticatedClient(2 * time.Second)
	resp, err := client.Get(baseURL + "/v1/info")
	if err != nil {
		// If we can't reach the API, assume incompatible
		return fmt.Errorf("cannot reach container API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("container API returned status %d", resp.StatusCode)
	}

	var versionInfo struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		return fmt.Errorf("failed to decode version response: %w", err)
	}

	// Compare versions using the same logic as in run.go
	containerVersion := versionInfo.Version

	// Clean versions for comparison
	cleanCLIVersion := cleanVersionForProduction(cliVersion)
	cleanContainerVersion := cleanVersionForProduction(containerVersion)

	if cleanCLIVersion != cleanContainerVersion {
		return fmt.Errorf("version mismatch: CLI=%s, Container=%s", cleanCLIVersion, cleanContainerVersion)
	}

	return nil
}

// cleanVersionForProduction removes the -dev suffix and v prefix from version string
// This duplicates the function from run.go to avoid import cycles
func cleanVersionForProduction(version string) string {
	// Remove v prefix if present
	version = strings.TrimPrefix(version, "v")

	// Remove -dev suffix if present
	version = strings.TrimSuffix(version, "-dev")

	return version
}
