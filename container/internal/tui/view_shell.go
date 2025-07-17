package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vanpelt/catnip/internal/tui/components"
)

// ShellViewImpl handles the shell view functionality
type ShellViewImpl struct{}

// NewShellView creates a new shell view instance
func NewShellView() *ShellViewImpl {
	return &ShellViewImpl{}
}

// GetViewType returns the view type identifier
func (v *ShellViewImpl) GetViewType() ViewType {
	return ShellView
}

// Update handles shell-specific message processing
func (v *ShellViewImpl) Update(m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	// Handle shell output and error messages
	switch msg := msg.(type) {
	case shellOutputMsg:
		if msg.sessionID == m.currentSessionID {
			return v.handleShellOutput(m, msg)
		}
	case shellErrorMsg:
		if msg.sessionID == m.currentSessionID {
			return v.handleShellError(m, msg)
		}
	}

	// Update shell viewport
	var cmd tea.Cmd
	m.shellViewport, cmd = m.shellViewport.Update(msg)
	return m, cmd
}

// HandleKey processes key messages for the shell view
func (v *ShellViewImpl) HandleKey(m *Model, msg tea.KeyMsg) (*Model, tea.Cmd) {
	// Handle session list navigation
	if m.showSessionList {
		switch msg.String() {
		case components.KeyEscape:
			m.showSessionList = false
			m.SwitchToView(OverviewView)
			return m, nil
		case components.KeyShellNewSession:
			m.showSessionList = false
			newModel, cmd := v.createNewShellSessionWithCmd(m)
			return newModel, cmd
		case components.KeyPort1, components.KeyPort2, components.KeyPort3, components.KeyPort4, components.KeyPort5,
			components.KeyPort6, components.KeyPort7, components.KeyPort8, components.KeyPort9:
			i := int(msg.String()[0] - '1')
			if globalShellManager != nil {
				sessionIDs := make([]string, 0, len(globalShellManager.sessions))
				for id := range globalShellManager.sessions {
					sessionIDs = append(sessionIDs, id)
				}
				if i < len(sessionIDs) {
					m.showSessionList = false
					m = v.switchToShellSession(m, sessionIDs[i])
				}
			}
			return m, nil
		default:
			// For any other key in session list mode, just ignore
			return m, nil
		}
	}

	// Handle active shell session keys
	switch msg.String() {
	case components.KeyShellOverview:
		m.SwitchToView(OverviewView)
		return m, nil

	case components.KeyShellQuit:
		return m, tea.Quit

	// Handle viewport scrolling
	case components.KeyPageUp, components.KeyShellPageUp:
		m.shellViewport.PageUp()
		return m, nil

	case components.KeyPageDown, components.KeyShellPageDown:
		m.shellViewport.PageDown()
		return m, nil

	case components.KeyHome, components.KeyShellHome:
		m.shellViewport.GotoTop()
		return m, nil

	case components.KeyEnd, components.KeyShellEnd:
		m.shellViewport.GotoBottom()
		return m, nil

	// Alt/Option key combinations (for Mac)
	case components.KeyShellScrollUp:
		m.shellViewport.ScrollUp(1)
		return m, nil

	case components.KeyShellScrollDown:
		m.shellViewport.ScrollDown(1)
		return m, nil

	default:
		// Forward all other input to PTY
		v.forwardPty(m, msg)
		return m, nil
	}
}

// HandleResize processes window resize for the shell view
func (v *ShellViewImpl) HandleResize(m *Model, msg tea.WindowSizeMsg) (*Model, tea.Cmd) {
	// Update shell viewport size
	headerHeight := 3
	m.shellViewport.Width = msg.Width - 2
	m.shellViewport.Height = msg.Height - headerHeight

	// Resize terminal emulator
	terminalWidth := m.shellViewport.Width - 2
	if m.terminalEmulator != nil {
		m.terminalEmulator.Resize(terminalWidth, m.shellViewport.Height)
	}

	// Send resize to PTY
	if globalShellManager != nil {
		if session := globalShellManager.GetSession(m.currentSessionID); session != nil && session.Client != nil {
			go func(width, height int) {
				if err := session.Client.Resize(width, height); err != nil {
					debugLog("Failed to resize PTY: %v", err)
				}
			}(terminalWidth, m.shellViewport.Height)
		}
	}

	return m, nil
}

// Render generates the shell view content
func (v *ShellViewImpl) Render(m *Model) string {
	if m.showSessionList {
		return v.renderSessionList(m)
	}

	// Header with session info
	headerStyle := components.ShellHeaderStyle.Width(m.width - 2)
	header := headerStyle.Render(fmt.Sprintf("Shell Session: %s | Press Ctrl+O to return to overview", m.currentSessionID))

	// If connecting, show spinner
	if m.shellConnecting {
		connectingStyle := components.CenteredStyle.
			Padding(2, 0).
			Width(m.width - 2).
			Height(m.height - 6)

		connectingContent := fmt.Sprintf("%s Connecting to shell...\n\nPlease wait while we establish a connection to the container.", m.shellSpinner.View())
		return fmt.Sprintf("%s\n%s", header, connectingStyle.Render(connectingContent))
	}

	// Shell output is already rendered with cursor by terminal emulator
	m.shellViewport.SetContent(m.shellOutput)

	return fmt.Sprintf("%s\n%s", header, m.shellViewport.View())
}

// Helper methods

func (v *ShellViewImpl) renderSessionList(m *Model) string {
	listStyle := lipgloss.NewStyle().
		Padding(1, 2).
		Width(m.width - 4)

	var content strings.Builder
	content.WriteString("Active Shell Sessions:\n\n")

	i := 1
	if globalShellManager != nil {
		for sessionID, session := range globalShellManager.sessions {
			status := "disconnected"
			if session.Connected {
				status = "connected"
			}
			content.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i, sessionID, status))
			i++
		}
	}

	content.WriteString("\n  n. Create new session")
	content.WriteString("\n  ESC. Cancel\n")

	return listStyle.Render(content.String())
}

func (v *ShellViewImpl) handleShellOutput(m *Model, msg shellOutputMsg) (*Model, tea.Cmd) {
	// First output means we're connected
	if m.shellConnecting {
		m.shellConnecting = false
		m.shellOutput = "" // Clear the "Connecting..." message
		if m.terminalEmulator != nil {
			m.terminalEmulator.Clear()
		}
	}

	// Initialize terminal emulator if needed
	if m.terminalEmulator == nil {
		// Use current viewport dimensions
		terminalWidth := m.shellViewport.Width - 2
		m.terminalEmulator = NewTerminalEmulator(terminalWidth, m.shellViewport.Height)
	}

	// Process output through terminal emulator
	m.terminalEmulator.Write(msg.data)
	// Always use the terminal emulator for proper handling
	m.shellOutput = m.terminalEmulator.Render()
	m.shellViewport.SetContent(m.shellOutput)
	// Auto-scroll to bottom for new output
	m.shellViewport.GotoBottom()

	return m, nil
}

func (v *ShellViewImpl) handleShellError(m *Model, msg shellErrorMsg) (*Model, tea.Cmd) {
	m.shellConnecting = false
	debugLog("Shell error for session %s: %v", msg.sessionID, msg.err)

	// Check if it's a connection error (broken pipe, EOF, closed connection, etc.)
	if msg.err != nil {
		errStr := msg.err.Error()
		isConnectionError := strings.Contains(errStr, "broken pipe") ||
			strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "EOF") ||
			strings.Contains(errStr, "use of closed network connection") ||
			strings.Contains(errStr, "connection reset")

		if isConnectionError {
			// Switch back to overview screen
			m.SwitchToView(OverviewView)
			debugLog("Connection error detected (%s), switching to overview", errStr)
		} else {
			// Show error in terminal for other errors
			terminalWidth := m.shellViewport.Width - 2
			if m.terminalEmulator == nil {
				m.terminalEmulator = NewTerminalEmulator(terminalWidth, m.shellViewport.Height)
			}
			// Write error to terminal emulator
			errorMsg := fmt.Sprintf("\n\r[Error: %v]\n\r", msg.err)
			m.terminalEmulator.Write([]byte(errorMsg))
			m.shellOutput = m.terminalEmulator.Render()
			m.shellViewport.SetContent(m.shellOutput)
		}
	}

	return m, nil
}

func (v *ShellViewImpl) forwardPty(m *Model, msg tea.KeyMsg) {
	// Send input to PTY
	debugLog("Shell view default case for key: %s", msg.String())
	if globalShellManager != nil {
		if session := globalShellManager.GetSession(m.currentSessionID); session != nil && session.Client != nil {
			var data []byte
			if len(msg.Runes) > 0 {
				data = []byte(string(msg.Runes))
			} else {
				// Handle special keys
				switch msg.Type {
				case tea.KeyEnter:
					data = []byte("\r")
				case tea.KeyBackspace:
					data = []byte{127}
				case tea.KeyTab:
					data = []byte("\t")
				case tea.KeyEsc:
					data = []byte{27}
				case tea.KeyUp:
					data = []byte("\x1b[A")
				case tea.KeyDown:
					data = []byte("\x1b[B")
				case tea.KeyRight:
					data = []byte("\x1b[C")
				case tea.KeyLeft:
					data = []byte("\x1b[D")
				default:
					// Handle Ctrl+C, Ctrl+D, etc.
					switch msg.String() {
					case components.KeyCtrlC:
						data = []byte{3}
					case components.KeyCtrlD:
						data = []byte{4}
					case components.KeyCtrlZ:
						data = []byte{26}
					}
				}
			}
			if len(data) > 0 {
				go func(d []byte, sessionID string) {
					if err := session.Client.Send(d); err != nil {
						debugLog("Failed to send data to PTY: %v", err)
						// Send error message to handle broken pipe
						if globalShellManager != nil && globalShellManager.program != nil {
							globalShellManager.program.Send(shellErrorMsg{
								sessionID: sessionID,
								err:       err,
							})
						}
					}
				}(data, m.currentSessionID)
			}
		}
	}
}

func (v *ShellViewImpl) switchToShellSession(m *Model, sessionID string) *Model {
	if globalShellManager != nil {
		if session := globalShellManager.GetSession(sessionID); session != nil {
			m.currentSessionID = sessionID
			m.SwitchToView(ShellView)
			m.showSessionList = false
			// Initialize terminal emulator if needed
			terminalWidth := m.shellViewport.Width - 2
			if m.terminalEmulator == nil {
				m.terminalEmulator = NewTerminalEmulator(terminalWidth, m.shellViewport.Height)
			} else {
				m.terminalEmulator.Clear()
			}
			// Replay output through terminal emulator
			m.terminalEmulator.Write(session.Output)
			m.shellOutput = m.terminalEmulator.Render()
			m.shellViewport.SetContent(m.shellOutput)
		}
	}
	return m
}

func (v *ShellViewImpl) createNewShellSessionWithCmd(m *Model) (*Model, tea.Cmd) {
	// This is the same logic as in overview view - could be extracted to shared function
	return (&OverviewViewImpl{}).createNewShellSessionWithCmd(m)
}
