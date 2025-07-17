package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vanpelt/catnip/internal/tui/components"
)

// InitializationViewImpl handles the initial setup screen
type InitializationViewImpl struct {
	spinner       spinner.Model
	status        string
	output        []string
	viewport      viewport.Model
	completed     bool
	failed        bool
	currentAction string
}

// NewInitializationView creates a new initialization view
func NewInitializationView() *InitializationViewImpl {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	vp := viewport.New(80, 20) // Will be resized properly later

	return &InitializationViewImpl{
		spinner:  s,
		status:   "Checking container image...",
		output:   []string{},
		viewport: vp,
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
		return m, StartContainerCmd(m.containerService, m.containerImage, m.containerName, m.gitRoot, m.devMode, m.customPorts)

	case InitializationCompleteWithOutputMsg:
		v.completed = true
		v.output = append(v.output, msg.Output...)
		v.status = "Initialization complete!"
		// Trigger container start
		return m, StartContainerCmd(m.containerService, m.containerImage, m.containerName, m.gitRoot, m.devMode, m.customPorts)

	case InitializationFailedMsg:
		v.failed = true
		v.status = fmt.Sprintf("Initialization failed: %s", msg.Error)

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
			return m, StartContainerCmd(m.containerService, m.containerImage, m.containerName, m.gitRoot, m.devMode, m.customPorts)
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

	case StartStreamingBuildCmd:
		// Start streaming the build command
		debugLog("InitializationView: starting streaming build command")
		return m, ExecuteStreamingBuildCmd(msg.Command)

	case StartStreamingReader:
		// Start the streaming reader
		debugLog("InitializationView: starting streaming reader")
		return m, StreamingOutputReader(msg.OutputChan, msg.DoneChan)

	case ContainerStartedMsg:
		v.status = "Container started successfully!"
		// Switch to overview after a brief delay to show the success message
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg {
			return SwitchViewMsg{ViewType: OverviewView}
		})

	case ContainerStartFailedMsg:
		v.failed = true
		v.status = fmt.Sprintf("Failed to start container: %s", msg.Error)

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

	if v.completed {
		content.WriteString(statusStyle.Render("✅ " + v.status))
	} else if v.failed {
		content.WriteString(statusStyle.Render("❌ " + v.status))
	} else {
		content.WriteString(statusStyle.Render(v.spinner.View() + " " + v.status))
	}

	content.WriteString("\n\n")

	// Output section
	if len(v.output) > 0 {
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

// Initialization message types
type InitializationCompleteMsg struct{}
type InitializationFailedMsg struct {
	Error string
}
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
type SwitchViewMsg struct {
	ViewType ViewType
}
type ContainerStartedMsg struct{}
type ContainerStartFailedMsg struct {
	Error string
}
