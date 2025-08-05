package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vanpelt/catnip/internal/services"
	"github.com/vanpelt/catnip/internal/tui/components"
)

// InitializationViewImpl handles the initial setup screen
type InitializationViewImpl struct {
	spinner          spinner.Model
	status           string
	output           []string
	viewport         viewport.Model
	terminalEmulator *TerminalEmulator
	completed        bool
	failed           bool
	currentAction    string
}

// NewInitializationView creates a new initialization view
func NewInitializationView() *InitializationViewImpl {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	vp := viewport.New(80, 20) // Will be resized properly later

	return &InitializationViewImpl{
		spinner:          s,
		status:           "Checking container image...",
		output:           []string{},
		viewport:         vp,
		terminalEmulator: nil,
	}
}

// GetViewType returns the view type identifier
func (v *InitializationViewImpl) GetViewType() ViewType {
	return InitializationView
}

// Update handles view-specific message processing
func (v *InitializationViewImpl) Update(m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	// If quit was requested, ignore all messages except quit
	if m.quitRequested {
		debugLog("InitializationView: ignoring message after quit requested: %T", msg)
		return m, nil
	}

	switch msg := msg.(type) {
	case InitializationProcessMsg:
		// Start the initialization process
		return m, ProcessInitialization(msg)

	case InitializationCompleteMsg:
		v.completed = true
		v.status = "Initialization complete!"
		// Trigger container start
		return m, StartContainerCmd(m.containerService, m.containerImage, m.containerName, m.gitRoot, m.devMode, m.customPorts, m.sshEnabled, m.rmFlag)

	case InitializationCompleteWithOutputMsg:
		v.completed = true
		v.output = append(v.output, msg.Output...)
		v.status = "Initialization complete!"
		// Trigger container start
		return m, StartContainerCmd(m.containerService, m.containerImage, m.containerName, m.gitRoot, m.devMode, m.customPorts, m.sshEnabled, m.rmFlag)

	case InitializationFailedMsg:
		v.failed = true
		v.status = fmt.Sprintf("Initialization failed: %s", msg.Error)
		// Stop the spinner when failed
		v.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // Red color

	case InitializationStatusMsg:
		v.status = msg.Status
		v.currentAction = msg.Action

		// Check for special cases
		if msg.SkipToOverview {
			// Container already running, skip to overview
			return m, tea.Tick(time.Millisecond*500, func(time.Time) tea.Msg {
				return SwitchViewMsg{ViewType: OverviewView}
			})
		}

		if msg.StartContainer {
			// Need to start container
			return m, StartContainerCmd(m.containerService, m.containerImage, m.containerName, m.gitRoot, m.devMode, m.customPorts, m.sshEnabled, m.rmFlag)
		}

		// Trigger the appropriate streaming command based on the action
		// Check if quit was requested before starting any action
		if m.quitRequested {
			debugLog("InitializationView: quit requested, not starting action: %s", msg.Action)
			return m, tea.Quit
		}

		if strings.Contains(msg.Action, "Pulling") {
			// Extract image name from action
			parts := strings.Split(msg.Action, " ")
			if len(parts) >= 2 {
				image := parts[1]
				return m, RunDockerPullStream(m.containerService, image)
			} else {
				// Invalid pull action, treat as failure
				return m, func() tea.Msg {
					return InitializationFailedMsg{Error: fmt.Sprintf("Invalid pull action: %s", msg.Action)}
				}
			}
		} else if strings.Contains(msg.Action, "build-dev") {
			return m, RunDevBuildStream(m.containerService, m.gitRoot)
		}

	case InitializationOutputMsg:
		debugLog("InitializationView: received output message: %s", msg.Line)
		v.output = append(v.output, msg.Line)
		// Update viewport content
		v.viewport.SetContent(strings.Join(v.output, "\n"))
		// Auto-scroll to bottom
		v.viewport.GotoBottom()
		// Continue streaming by returning the next read command
		return m, StreamingOutputReader(msg.OutputChan, msg.DoneChan)

	case InitializationContinueStreamingMsg:
		// Continue streaming without adding output
		debugLog("InitializationView: received continue streaming message")
		return m, StreamingOutputReader(msg.OutputChan, msg.DoneChan)

	case InitializationTTYOutputMsg:
		// Handle raw PTY output with terminal emulator
		if v.terminalEmulator == nil {
			width := v.viewport.Width - 2
			if width <= 0 {
				width = 80
			}
			height := v.viewport.Height
			if height <= 0 {
				height = 24
			}
			v.terminalEmulator = NewTerminalEmulator(width, height)
		}
		v.terminalEmulator.Write(msg.Data)
		v.viewport.SetContent(v.terminalEmulator.Render())
		v.viewport.GotoBottom()
		return m, StreamingTTYReader(msg.OutputChan, msg.DoneChan)

	case InitializationContinueTTYMsg:
		return m, StreamingTTYReader(msg.OutputChan, msg.DoneChan)

	case StartStreamingBuildCmd:
		// Start streaming the build command
		debugLog("InitializationView: starting streaming build command")
		return m, ExecuteStreamingBuildCmd(msg.Command)

	case StartStreamingReader:
		// Start the streaming reader
		debugLog("InitializationView: starting streaming reader")
		return m, StreamingOutputReader(msg.OutputChan, msg.DoneChan)

	case StartStreamingTTYReader:
		debugLog("InitializationView: starting streaming TTY reader")
		return m, StreamingTTYReader(msg.OutputChan, msg.DoneChan)

	case ContainerStartedMsg:
		v.status = "Container started, checking health..."
		// Start monitoring container health instead of switching to overview
		return m, MonitorContainerHealthCmd(msg.ContainerService, msg.ContainerName)

	case ContainerStartFailedMsg:
		v.failed = true
		v.completed = false // Ensure completed is false when failed
		// Extract only the first line for the header status
		errorLines := strings.Split(msg.Error, "\n")
		v.status = fmt.Sprintf("Failed to start container: %s", errorLines[0])
		// Format the error details nicely for viewport
		v.output = v.formatErrorOutput(msg.Error)
		// Update viewport content
		v.viewport.SetContent(strings.Join(v.output, "\n"))
		// Auto-scroll to bottom
		v.viewport.GotoBottom()

	case ContainerHealthCheckFailedMsg:
		v.failed = true
		v.completed = false // Ensure completed is false when failed
		v.status = fmt.Sprintf("Container health check failed: %s", msg.Error)

	case StartStreamingContainerLogsCmd:
		v.status = "Monitoring container startup..."
		return m, ExecuteStreamingContainerLogsCmd(msg.Command, msg.ContainerName, msg.ContainerService)

	case StartStreamingContainerLogsReader:
		return m, StreamingContainerLogsReader(msg.OutputChan, msg.DoneChan)

	case ContainerLogsOutputMsg:
		// Only add non-empty lines to prevent scrolling away errors
		if strings.TrimSpace(msg.Line) != "" {
			debugLog("InitializationView: received container log output: %s", msg.Line)
			v.output = append(v.output, msg.Line)
			// Update viewport content
			v.viewport.SetContent(strings.Join(v.output, "\n"))
			// Auto-scroll to bottom
			v.viewport.GotoBottom()
		}
		// Continue streaming by returning the next read command
		return m, StreamingContainerLogsReader(msg.OutputChan, msg.DoneChan)

	case ContainerHealthyMsg:
		v.completed = true
		v.status = "Container is healthy and ready!"
		// Trigger version check and switch to overview after a brief delay
		return m, tea.Batch(
			CheckContainerVersionCmd(m.version),
			tea.Tick(time.Second, func(time.Time) tea.Msg {
				return SwitchViewMsg{ViewType: OverviewView}
			}),
		)

	case SwitchViewMsg:
		if msg.ViewType != InitializationView {
			m.SwitchToView(msg.ViewType)
			// Start background tasks when switching to Overview
			if msg.ViewType == OverviewView {
				return m, m.initCommands()
			}
		}
	}

	// Update viewport for scrolling
	var vpCmd tea.Cmd
	v.viewport, vpCmd = v.viewport.Update(msg)

	// Always update spinner for animation
	var spinnerCmd tea.Cmd
	v.spinner, spinnerCmd = v.spinner.Update(msg)

	return m, tea.Batch(vpCmd, spinnerCmd)
}

// Render generates the view content
func (v *InitializationViewImpl) Render(m *Model) string {
	var content strings.Builder

	// Status with spinner (no duplicate header)
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(components.ColorText)).
		MarginBottom(1)

	if v.failed {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			MarginBottom(1)
		content.WriteString(errorStyle.Render("❌ " + v.status))

		// Only show generic help for non-container failures
		if !strings.Contains(v.status, "Container") {
			// Add helpful guidance for failed initialization
			helpStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Italic(true).
				MarginBottom(1)
			content.WriteString("\n")
			content.WriteString(helpStyle.Render("Check the output below for details. Press 'q' to exit or restart the application to try again."))
		}
	} else if v.completed {
		content.WriteString(statusStyle.Render("✅ " + v.status))
	} else {
		content.WriteString(statusStyle.Render(v.spinner.View() + " " + v.status))
	}

	content.WriteString("\n\n")

	// Output section
	if len(v.output) > 0 || v.terminalEmulator != nil {
		// Calculate available height for output
		maxHeight := m.height - 10 // Leave room for header, status, and border
		if maxHeight < 10 {
			maxHeight = 10 // Minimum height
		}
		if maxHeight > 30 {
			maxHeight = 30 // Maximum height
		}

		width := m.width - 10
		if width > 120 {
			width = 120 // Max width for readability
		}

		// Update viewport dimensions if needed
		if v.viewport.Height != maxHeight || v.viewport.Width != width {
			v.viewport.Height = maxHeight
			v.viewport.Width = width
		}

		outputStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(components.ColorBorder)).
			Width(width + 2) // +2 for border

		content.WriteString(outputStyle.Render(v.viewport.View()))
	}

	return content.String()
}

// HandleKey processes key messages for this view
func (v *InitializationViewImpl) HandleKey(m *Model, msg tea.KeyMsg) (*Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitRequested = true // Set global quit flag
		debugLog("InitializationView: quit requested, setting flags")
		return m, tea.Quit
	}
	return m, nil
}

// formatErrorOutput formats error messages with nice boxes and highlighting
func (v *InitializationViewImpl) formatErrorOutput(errorMsg string) []string {
	lines := strings.Split(errorMsg, "\n")
	var formatted []string

	for _, line := range lines {
		if strings.HasPrefix(line, "Command:") {
			// Format command in a nice box
			formatted = append(formatted, "")
			formatted = append(formatted, "\033[36m╭─ Command\033[0m")

			// Extract command part after "Command: "
			cmdPart := strings.TrimSpace(strings.TrimPrefix(line, "Command:"))
			if cmdPart != "" {
				// Word wrap long commands with line continuation characters
				// Use viewport width, but ensure minimum of 60 chars
				maxWidth := v.viewport.Width - 6 // Account for border and padding
				if maxWidth < 60 {
					maxWidth = 60
				}
				wrappedLines := v.wrapCommand(cmdPart, maxWidth)
				for i, wrappedLine := range wrappedLines {
					if i == len(wrappedLines)-1 {
						// Last line, no continuation
						formatted = append(formatted, fmt.Sprintf("\033[36m│\033[0m \033[90m%s\033[0m", wrappedLine))
					} else {
						// Add continuation character
						formatted = append(formatted, fmt.Sprintf("\033[36m│\033[0m \033[90m%s \\\033[0m", wrappedLine))
					}
				}
			}
			formatted = append(formatted, "\033[36m╰─\033[0m")
			formatted = append(formatted, "")
		} else if strings.HasPrefix(line, "Output:") {
			// Format output section
			formatted = append(formatted, "\033[33m╭─ Output\033[0m")
		} else if line != "" {
			// Regular line, but check if it's after Output: section
			if len(formatted) > 0 && strings.Contains(strings.Join(formatted[maxInt(0, len(formatted)-5):], ""), "Output") {
				// This is output content, format it with proper indentation
				formatted = append(formatted, fmt.Sprintf("\033[33m│\033[0m %s", line))
			} else {
				// Regular error line
				formatted = append(formatted, line)
			}
		}
	}

	// Close output box if it was opened
	if len(formatted) > 0 && strings.Contains(strings.Join(formatted, ""), "╭─ Output") {
		formatted = append(formatted, "\033[33m╰─\033[0m")
	}

	return formatted
}

// wrapCommand wraps a long command into multiple lines with proper shell continuation
func (v *InitializationViewImpl) wrapCommand(cmd string, maxWidth int) []string {
	// Account for the box drawing characters and indentation: "│ " = 2 chars
	// Account for the continuation character " \" = 2 chars when needed
	effectiveWidth := maxWidth - 4 // Leave room for "│ " and potential " \"

	if len(cmd) <= effectiveWidth {
		return []string{cmd}
	}

	var lines []string
	words := strings.Fields(cmd)
	currentLine := ""

	for _, word := range words {
		// Check if adding this word would exceed the line length
		testLine := currentLine
		if testLine != "" {
			testLine += " "
		}
		testLine += word

		if len(testLine) <= effectiveWidth {
			// Word fits, add it to current line
			currentLine = testLine
		} else {
			// Word doesn't fit, start new line
			if currentLine != "" {
				lines = append(lines, currentLine)
			}

			// Check if the word itself is too long for a line
			if len(word) > effectiveWidth {
				// Split the word across multiple lines
				for len(word) > effectiveWidth {
					lines = append(lines, word[:effectiveWidth])
					word = word[effectiveWidth:]
				}
				currentLine = word
			} else {
				currentLine = word
			}
		}
	}

	// Add the last line if it has content
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

// maxInt returns the maximum of two integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// HandleResize processes window resize messages
func (v *InitializationViewImpl) HandleResize(m *Model, msg tea.WindowSizeMsg) (*Model, tea.Cmd) {
	// Update viewport dimensions on resize
	maxHeight := msg.Height - 10
	if maxHeight < 10 {
		maxHeight = 10
	}
	if maxHeight > 30 {
		maxHeight = 30
	}

	width := msg.Width - 10
	if width > 120 {
		width = 120
	}

	v.viewport.Height = maxHeight
	v.viewport.Width = width

	return m, nil
}

// InitializationCompleteMsg signals that container initialization has completed successfully
type InitializationCompleteMsg struct{}

// InitializationFailedMsg signals that container initialization has failed
type InitializationFailedMsg struct {
	Error string
}

// InitializationStatusMsg provides status updates during container initialization
type InitializationStatusMsg struct {
	Status         string
	Action         string
	SkipToOverview bool
	StartContainer bool
}
type InitializationOutputMsg struct {
	Line       string
	OutputChan <-chan string
	DoneChan   <-chan bool
}

type InitializationContinueStreamingMsg struct {
	OutputChan <-chan string
	DoneChan   <-chan bool
}

type InitializationTTYOutputMsg struct {
	Data       []byte
	OutputChan <-chan []byte
	DoneChan   <-chan bool
}

type InitializationContinueTTYMsg struct {
	OutputChan <-chan []byte
	DoneChan   <-chan bool
}

type StartStreamingTTYReader struct {
	OutputChan <-chan []byte
	DoneChan   <-chan bool
}
type SwitchViewMsg struct {
	ViewType ViewType
}
type ContainerStartedMsg struct {
	ContainerName    string
	ContainerService *services.ContainerService
}
type ContainerStartFailedMsg struct {
	Error string
}
