package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vanpelt/catnip/internal/tui/components"
)

// WorkspaceViewImpl handles the workspace view functionality
type WorkspaceViewImpl struct{}

// NewWorkspaceView creates a new workspace view instance
func NewWorkspaceView() *WorkspaceViewImpl {
	return &WorkspaceViewImpl{}
}

// GetViewType returns the view type identifier
func (v *WorkspaceViewImpl) GetViewType() ViewType {
	return WorkspaceView
}

// Update handles workspace-specific message processing
func (v *WorkspaceViewImpl) Update(m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	// Handle shell output and error messages for Claude terminal only (simplified view)
	switch msg := msg.(type) {
	case shellOutputMsg:
		debugLog("WorkspaceView Update - received shellOutputMsg for session: %s", msg.sessionID)
		// Only handle Claude terminal messages (check if it matches our current workspace)
		if m.currentWorkspace != nil && strings.HasSuffix(msg.sessionID, ":claude") && strings.HasPrefix(msg.sessionID, m.currentWorkspace.Name) {
			debugLog("WorkspaceView Update - handling Claude output message")
			return v.handleClaudeOutput(m, msg)
		}
	case shellErrorMsg:
		debugLog("WorkspaceView Update - received shellErrorMsg for session: %s, error: %v", msg.sessionID, msg.err)
		// Only handle Claude terminal errors (check if it matches our current workspace)
		if m.currentWorkspace != nil && strings.HasSuffix(msg.sessionID, ":claude") && strings.HasPrefix(msg.sessionID, m.currentWorkspace.Name) {
			debugLog("WorkspaceView Update - handling Claude error message")
			return v.handleClaudeError(m, msg)
		}
	}

	// Update only Claude viewport model (simplified view)
	var cmd tea.Cmd
	m.workspaceClaudeTerminal, cmd = m.workspaceClaudeTerminal.Update(msg)

	return m, cmd
}

// HandleKey processes key messages for the workspace view
func (v *WorkspaceViewImpl) HandleKey(m *Model, msg tea.KeyMsg) (*Model, tea.Cmd) {
	// Handle escape key to return to overview
	switch msg.String() {
	case components.KeyEscape:
		m.SwitchToView(OverviewView)
		return m, nil
	default:
		// Forward all other input to the Claude terminal (simplified view)
		v.forwardToClaudeTerminal(m, msg)
		return m, nil
	}
}

// HandleResize processes window resize for the workspace view
func (v *WorkspaceViewImpl) HandleResize(m *Model, msg tea.WindowSizeMsg) (*Model, tea.Cmd) {
	// Calculate layout dimensions with proper padding
	headerHeight := 3
	padding := 2 // Overall padding
	availableHeight := msg.Height - headerHeight - padding
	availableWidth := msg.Width - padding

	// Ensure minimum dimensions
	if availableHeight < 10 {
		availableHeight = 10
	}
	if availableWidth < 40 {
		availableWidth = 40
	}

	// Simplified layout: Claude terminal (70% width) + right sidebar (30% width)
	mainWidth := (availableWidth * 70) / 100

	// Claude terminal takes full height (simplified view)
	claudeHeight := availableHeight

	// Ensure minimum terminal height
	if claudeHeight < 10 {
		claudeHeight = 10
	}

	// Update viewport size (account for terminal borders)
	terminalWidth := mainWidth - 4 // Account for terminal borders (2 per side)
	if terminalWidth < 20 {
		terminalWidth = 20
	}

	m.workspaceClaudeTerminal.Width = terminalWidth
	m.workspaceClaudeTerminal.Height = claudeHeight - 2 // Account for terminal border

	// Resize terminal emulator if it exists
	if m.workspaceClaudeTerminalEmulator != nil {
		m.workspaceClaudeTerminalEmulator.Resize(terminalWidth, claudeHeight-2)
		debugLog("HandleResize - resized terminal emulator: width=%d, height=%d", terminalWidth, claudeHeight-2)
	}

	// Resize PTY session if it exists - only Claude terminal now
	v.resizeClaudeTerminal(m, terminalWidth, claudeHeight-2)

	return m, nil
}

// Render generates the workspace view content
func (v *WorkspaceViewImpl) Render(m *Model) string {
	debugLog("WorkspaceView Render called - currentWorkspace: %+v", m.currentWorkspace)
	debugLog("WorkspaceView Render - terminal dimensions: width=%d height=%d", m.width, m.height)
	if m.currentWorkspace == nil {
		debugLog("WorkspaceView Render - no current workspace, showing no workspace screen")
		return v.renderNoWorkspace(m)
	}

	// Calculate layout dimensions with proper padding
	headerHeight := 3
	padding := 2 // Overall padding
	availableHeight := m.height - headerHeight - padding
	availableWidth := m.width - padding

	// Ensure minimum dimensions
	if availableHeight < 10 {
		availableHeight = 10
	}
	if availableWidth < 40 {
		availableWidth = 40
	}

	// Claude terminal takes full height (simplified view)
	claudeHeight := availableHeight

	// Ensure minimum terminal height
	if claudeHeight < 10 {
		claudeHeight = 10
	}

	// Fixed sidebar width: 20-30 columns, terminal takes the rest
	newSidebarWidth := 25 // Default to 25 columns
	if newSidebarWidth > 30 {
		newSidebarWidth = 30
	}
	if newSidebarWidth < 20 {
		newSidebarWidth = 20
	}

	terminalWidth := availableWidth - newSidebarWidth

	// Ensure minimum terminal width
	if terminalWidth < 60 {
		terminalWidth = 60
		newSidebarWidth = availableWidth - terminalWidth
		// Clamp sidebar to valid range after adjustment
		if newSidebarWidth > 30 {
			newSidebarWidth = 30
		}
		if newSidebarWidth < 20 {
			newSidebarWidth = 20
		}
	}

	claudeContent := v.renderClaudeTerminal(m, terminalWidth, claudeHeight)
	sidebarContent := v.renderSimpleSidebar(m, newSidebarWidth, claudeHeight)
	debugLog("WorkspaceView Render - Claude: %d chars, Sidebar: %d chars", len(claudeContent), len(sidebarContent))

	// Use lipgloss JoinHorizontal with no borders
	workspaceContent := lipgloss.JoinHorizontal(lipgloss.Top, claudeContent, sidebarContent)
	debugLog("WorkspaceView Render - final content length: %d", len(workspaceContent))

	return workspaceContent
}

// Helper methods

func (v *WorkspaceViewImpl) renderNoWorkspace(m *Model) string {
	centerStyle := components.CenteredStyle.
		Padding(2, 0).
		Width(m.width - 2).
		Height(m.height - 6)

	content := "No workspaces detected.\n\nPress Ctrl+W to select a workspace."
	return centerStyle.Render(content)
}

func (v *WorkspaceViewImpl) renderClaudeTerminal(m *Model, width, height int) string {
	debugLog("renderClaudeTerminal called - width=%d, height=%d", width, height)
	// FUCK THE STYLING - Just return raw terminal content
	debugLog("renderClaudeTerminal - SIMPLIFIED: no lipgloss styling")

	// Set content for Claude terminal viewport
	if m.currentWorkspace != nil {
		claudeSessionID := m.currentWorkspace.Name
		debugLog("renderClaudeTerminal - looking for session: %s", claudeSessionID)
		if globalShellManager != nil {
			debugLog("renderClaudeTerminal - globalShellManager exists, sessions count: %d", len(globalShellManager.sessions))
			// DEBUG: List all sessions to see what actually exists
			for sessionID := range globalShellManager.sessions {
				debugLog("renderClaudeTerminal - existing session: %s", sessionID)
			}
			if session := globalShellManager.GetSession(claudeSessionID); session != nil {
				debugLog("renderClaudeTerminal - found session, output length: %d, connected: %v", len(session.Output), session.Connected)

				// Initialize terminal emulator if needed
				if m.workspaceClaudeTerminalEmulator == nil {
					terminalWidth := width - 4 // Some padding
					debugLog("renderClaudeTerminal - initializing terminal emulator with width=%d, height=%d", terminalWidth, height)
					m.workspaceClaudeTerminalEmulator = NewTerminalEmulator(terminalWidth, height)
				}

				// Only process session output if it has changed
				if len(session.Output) > 0 {
					// Check if output length has changed since last render
					if len(session.Output) != m.workspaceLastOutputLength {
						debugLog("renderClaudeTerminal - output changed: %d -> %d bytes", m.workspaceLastOutputLength, len(session.Output))
						// Process PTY output directly (JSON filtering now handled at WebSocket level)
						m.workspaceClaudeTerminalEmulator.Clear()
						m.workspaceClaudeTerminalEmulator.Write(session.Output)
						terminalOutput := m.workspaceClaudeTerminalEmulator.Render()
						debugLog("renderClaudeTerminal - processed %d bytes through emulator, got %d chars", len(session.Output), len(terminalOutput))
						m.workspaceClaudeTerminal.SetContent(terminalOutput)
						m.workspaceClaudeTerminal.GotoBottom()
						m.workspaceLastOutputLength = len(session.Output)
					} else {
						// Content hasn't changed, don't reprocess
						debugLog("renderClaudeTerminal - content unchanged (%d bytes), skipping emulator processing", len(session.Output))
					}
				} else {
					m.workspaceClaudeTerminal.SetContent("Connecting to Claude terminal...")
				}
			} else {
				debugLog("renderClaudeTerminal - no session found, showing connecting message")
				m.workspaceClaudeTerminal.SetContent("Connecting to Claude terminal...")
			}
		} else {
			debugLog("renderClaudeTerminal - globalShellManager is nil")
			m.workspaceClaudeTerminal.SetContent("Shell manager not available")
		}
	} else {
		debugLog("renderClaudeTerminal - no current workspace")
		m.workspaceClaudeTerminal.SetContent("No workspace")
	}

	// Return just the raw terminal view - clean, no header
	terminalView := m.workspaceClaudeTerminal.View()
	debugLog("renderClaudeTerminal - terminal view length: %d", len(terminalView))
	return terminalView
}

func (v *WorkspaceViewImpl) renderSimpleSidebar(m *Model, width, height int) string {
	debugLog("renderSimpleSidebar called - width=%d, height=%d", width, height)

	var sections []string

	if m.currentWorkspace != nil {
		// Workspace info - no borders, just clean text
		sections = append(sections, "📁 "+m.currentWorkspace.Name)
		sections = append(sections, "🌿 "+m.currentWorkspace.Branch)
		sections = append(sections, "📂 "+m.currentWorkspace.Path)
		sections = append(sections, "")

		// Git status
		if len(m.currentWorkspace.ChangedFiles) > 0 {
			sections = append(sections, fmt.Sprintf("📝 %d changes", len(m.currentWorkspace.ChangedFiles)))
			for i, file := range m.currentWorkspace.ChangedFiles {
				if i >= 3 { // Limit to first 3 files
					sections = append(sections, fmt.Sprintf("   ...%d more", len(m.currentWorkspace.ChangedFiles)-3))
					break
				}
				// Extract just filename
				filename := file
				if lastSlash := strings.LastIndex(file, "/"); lastSlash != -1 {
					filename = file[lastSlash+1:]
				}
				sections = append(sections, "   • "+filename)
			}
		} else {
			sections = append(sections, "📝 No changes")
		}
		sections = append(sections, "")

		// Ports
		if len(m.currentWorkspace.Ports) > 0 {
			sections = append(sections, "🌐 Active Ports")
			for _, port := range m.currentWorkspace.Ports {
				sections = append(sections, fmt.Sprintf("   :%s %s", port.Port, port.Title))
			}
		} else {
			sections = append(sections, "🌐 No ports")
		}
	} else {
		sections = append(sections, "No workspace")
	}

	// Join all sections and ensure it fits the width
	content := strings.Join(sections, "\n")

	// Simple style with just width constraint, no borders
	style := lipgloss.NewStyle().Width(width).Align(lipgloss.Left)
	result := style.Render(content)

	debugLog("renderSimpleSidebar - final length: %d", len(result))
	return result
}

func (v *WorkspaceViewImpl) handleClaudeOutput(m *Model, msg shellOutputMsg) (*Model, tea.Cmd) {
	debugLog("handleClaudeOutput - received %d bytes of data", len(msg.data))

	// Initialize terminal emulator if needed
	if m.workspaceClaudeTerminalEmulator == nil {
		terminalWidth := m.workspaceClaudeTerminal.Width - 2
		debugLog("handleClaudeOutput - initializing terminal emulator with width=%d, height=%d", terminalWidth, m.workspaceClaudeTerminal.Height)
		m.workspaceClaudeTerminalEmulator = NewTerminalEmulator(terminalWidth, m.workspaceClaudeTerminal.Height)
	}

	// Process PTY output directly (JSON filtering now handled at WebSocket level)
	if len(msg.data) > 0 {
		// Process output through terminal emulator
		m.workspaceClaudeTerminalEmulator.Write(msg.data)
		// Always use the terminal emulator for proper handling
		terminalOutput := m.workspaceClaudeTerminalEmulator.Render()
		debugLog("handleClaudeOutput - terminal emulator rendered %d chars", len(terminalOutput))
		m.workspaceClaudeTerminal.SetContent(terminalOutput)
		// Auto-scroll to bottom for new output
		m.workspaceClaudeTerminal.GotoBottom()
	}

	return m, nil
}

func (v *WorkspaceViewImpl) handleClaudeError(m *Model, msg shellErrorMsg) (*Model, tea.Cmd) {
	debugLog("Claude terminal error for workspace %s: %v", m.currentWorkspace.ID, msg.err)
	// Could add error display to Claude terminal
	return m, nil
}

func (v *WorkspaceViewImpl) forwardToClaudeTerminal(m *Model, msg tea.KeyMsg) {
	if m.currentWorkspace == nil {
		return
	}

	// Send input to the Claude terminal PTY session (use base name for PTY lookup)
	claudeSessionID := m.currentWorkspace.Name
	debugLog("Workspace view forwarding key to Claude terminal: %s", msg.String())

	if globalShellManager != nil {
		if session := globalShellManager.GetSession(claudeSessionID); session != nil && session.Client != nil {
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
				case tea.KeyUp:
					data = []byte("\x1b[A")
				case tea.KeyDown:
					data = []byte("\x1b[B")
				case tea.KeyRight:
					data = []byte("\x1b[C")
				case tea.KeyLeft:
					data = []byte("\x1b[D")
				default:
					// Handle Ctrl combinations
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
						debugLog("Failed to send data to workspace Claude terminal: %v", err)
						if globalShellManager != nil && globalShellManager.program != nil {
							globalShellManager.program.Send(shellErrorMsg{
								sessionID: sessionID,
								err:       err,
							})
						}
					}
				}(data, claudeSessionID)
			}
		}
	}
}

func (v *WorkspaceViewImpl) resizeClaudeTerminal(m *Model, width, height int) {
	if m.currentWorkspace == nil {
		return
	}

	if globalShellManager != nil {
		// Resize Claude terminal (use base name for PTY lookup)
		claudeSessionID := m.currentWorkspace.Name
		if session := globalShellManager.GetSession(claudeSessionID); session != nil && session.Client != nil {
			go func() {
				if err := session.Client.Resize(width, height); err != nil {
					debugLog("Failed to resize Claude terminal: %v", err)
				}
			}()
		}
	}
}

// CreateWorkspaceSessions creates both Claude and regular terminal sessions for a workspace
func (v *WorkspaceViewImpl) CreateWorkspaceSessions(m *Model, workspace *WorkspaceInfo) (*Model, tea.Cmd) {
	debugLog("CreateWorkspaceSessions called for workspace: %+v", workspace)
	if workspace == nil {
		debugLog("CreateWorkspaceSessions - workspace is nil")
		return m, nil
	}

	claudeSessionID := workspace.Name

	// Calculate terminal dimensions using same logic as Render method (simplified)
	headerHeight := 3
	padding := 2
	availableHeight := m.height - headerHeight - padding
	availableWidth := m.width - padding

	// Ensure minimum dimensions
	if availableHeight < 10 {
		availableHeight = 10
	}
	if availableWidth < 40 {
		availableWidth = 40
	}

	mainWidth := (availableWidth * 70) / 100
	claudeHeight := availableHeight // Full height for simplified view

	// Ensure minimum terminal height
	if claudeHeight < 10 {
		claudeHeight = 10
	}

	terminalWidth := mainWidth - 4 // Account for terminal borders
	if terminalWidth < 20 {
		terminalWidth = 20
	}

	// Initialize viewport (account for terminal borders)
	m.workspaceClaudeTerminal.Width = terminalWidth
	m.workspaceClaudeTerminal.Height = claudeHeight - 2 // Account for terminal border
	debugLog("CreateWorkspaceSessions - initialized viewport: width=%d, height=%d", m.workspaceClaudeTerminal.Width, m.workspaceClaudeTerminal.Height)

	// Initialize terminal emulator
	if m.workspaceClaudeTerminalEmulator == nil {
		m.workspaceClaudeTerminalEmulator = NewTerminalEmulator(terminalWidth, claudeHeight-2)
		debugLog("CreateWorkspaceSessions - initialized terminal emulator: width=%d, height=%d", terminalWidth, claudeHeight-2)
	} else {
		m.workspaceClaudeTerminalEmulator.Clear()
		m.workspaceClaudeTerminalEmulator.Resize(terminalWidth, claudeHeight-2)
		debugLog("CreateWorkspaceSessions - resized existing terminal emulator: width=%d, height=%d", terminalWidth, claudeHeight-2)
	}

	debugLog("CreateWorkspaceSessions - creating Claude session: %s, terminalWidth=%d, terminalHeight=%d", claudeSessionID, terminalWidth, claudeHeight-2)
	// Create Claude terminal session
	claudeCmd := createAndConnectWorkspaceShell(claudeSessionID, terminalWidth, claudeHeight-2, workspace.Path, "?agent=claude")

	return m, claudeCmd
}

// createAndConnectWorkspaceShell creates a shell session for a workspace with specific parameters
func createAndConnectWorkspaceShell(sessionID string, width, height int, workspacePath, agentParam string) tea.Cmd {
	return func() tea.Msg {
		debugLog("createAndConnectWorkspaceShell called - sessionID: %s, width: %d, height: %d, path: %s, agent: %s", sessionID, width, height, workspacePath, agentParam)
		if globalShellManager == nil {
			debugLog("createAndConnectWorkspaceShell - globalShellManager is nil")
			return shellErrorMsg{sessionID: sessionID, err: fmt.Errorf("shell manager not initialized")}
		}

		// Create session using the existing shell manager pattern
		session := globalShellManager.CreateSession(sessionID)
		debugLog("createAndConnectWorkspaceShell - created session: %+v", session)

		// Connect in background and send initial size
		go func() {
			baseURL := "http://localhost:8080"
			if agentParam != "" {
				baseURL += agentParam
			}
			debugLog("createAndConnectWorkspaceShell - connecting to: %s", baseURL)

			err := session.Client.Connect(baseURL)
			if err != nil {
				debugLog("createAndConnectWorkspaceShell - Failed to connect workspace shell session %s: %v", sessionID, err)
				return
			}
			debugLog("createAndConnectWorkspaceShell - connected successfully")

			// Send resize to set initial terminal size
			if err := session.Client.Resize(width, height); err != nil {
				debugLog("Failed to resize workspace terminal %s: %v", sessionID, err)
			}

			// Note: Directory changing is handled by the backend automatically
			// No need to inject cd commands that show up in the terminal
		}()

		return shellConnectedMsg{sessionID: sessionID}
	}
}
