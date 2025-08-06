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
	// Handle shell output and error messages for both terminals
	switch msg := msg.(type) {
	case shellOutputMsg:
		// Determine which terminal this is for based on session ID
		if strings.HasSuffix(msg.sessionID, "-claude") {
			return v.handleClaudeOutput(m, msg)
		} else if strings.HasSuffix(msg.sessionID, "-regular") {
			return v.handleRegularOutput(m, msg)
		}
	case shellErrorMsg:
		// Handle errors for both terminals
		if strings.HasSuffix(msg.sessionID, "-claude") {
			return v.handleClaudeError(m, msg)
		} else if strings.HasSuffix(msg.sessionID, "-regular") {
			return v.handleRegularError(m, msg)
		}
	}

	// Update both viewport models
	var cmd1, cmd2 tea.Cmd
	m.workspaceClaudeTerminal, cmd1 = m.workspaceClaudeTerminal.Update(msg)
	m.workspaceRegularTerminal, cmd2 = m.workspaceRegularTerminal.Update(msg)

	return m, tea.Batch(cmd1, cmd2)
}

// HandleKey processes key messages for the workspace view
func (v *WorkspaceViewImpl) HandleKey(m *Model, msg tea.KeyMsg) (*Model, tea.Cmd) {
	// Handle escape key to return to overview
	switch msg.String() {
	case components.KeyEscape:
		m.SwitchToView(OverviewView)
		return m, nil
	default:
		// Forward all other input to the regular terminal (bottom terminal)
		v.forwardToRegularTerminal(m, msg)
		return m, nil
	}
}

// HandleResize processes window resize for the workspace view
func (v *WorkspaceViewImpl) HandleResize(m *Model, msg tea.WindowSizeMsg) (*Model, tea.Cmd) {
	// Calculate layout dimensions
	headerHeight := 3
	totalHeight := msg.Height - headerHeight
	
	// Main content area is 75% width, right sidebar is 25%
	mainWidth := (msg.Width * 75) / 100
	sidebarWidth := msg.Width - mainWidth - 2 // Account for borders
	
	// Claude terminal gets 75% of main height, regular terminal gets 25%
	claudeHeight := (totalHeight * 75) / 100
	regularHeight := totalHeight - claudeHeight - 1 // Account for separator
	
	// Update viewport sizes
	m.workspaceClaudeTerminal.Width = mainWidth - 2
	m.workspaceClaudeTerminal.Height = claudeHeight
	m.workspaceRegularTerminal.Width = mainWidth - 2
	m.workspaceRegularTerminal.Height = regularHeight

	// Resize PTY sessions if they exist
	v.resizeWorkspaceTerminals(m, mainWidth-2, claudeHeight, regularHeight)

	return m, nil
}

// Render generates the workspace view content
func (v *WorkspaceViewImpl) Render(m *Model) string {
	if m.currentWorkspace == nil {
		return v.renderNoWorkspace(m)
	}

	// Calculate layout dimensions
	headerHeight := 3
	totalHeight := m.height - headerHeight
	
	// Main content area is 75% width, right sidebar is 25%
	mainWidth := (m.width * 75) / 100
	sidebarWidth := m.width - mainWidth - 2 // Account for borders
	
	// Claude terminal gets 75% of main height, regular terminal gets 25%
	claudeHeight := (totalHeight * 75) / 100
	regularHeight := totalHeight - claudeHeight - 1 // Account for separator

	// Header for the workspace
	headerStyle := components.ShellHeaderStyle.Width(m.width - 2)
	header := headerStyle.Render(fmt.Sprintf("📁 Workspace: %s (%s) | Press Esc to return to overview", 
		m.currentWorkspace.Name, m.currentWorkspace.Branch))

	// Main content area (left side)
	mainContent := v.renderMainContent(m, mainWidth, claudeHeight, regularHeight)
	
	// Right sidebar content
	sidebarContent := v.renderSidebar(m, sidebarWidth, totalHeight)
	
	// Combine main and sidebar horizontally
	workspaceContent := lipgloss.JoinHorizontal(
		lipgloss.Top,
		mainContent,
		sidebarContent,
	)

	return fmt.Sprintf("%s\n%s", header, workspaceContent)
}

// Helper methods

func (v *WorkspaceViewImpl) renderNoWorkspace(m *Model) string {
	centerStyle := components.CenteredStyle.
		Padding(2, 0).
		Width(m.width - 2).
		Height(m.height - 6)

	content := "No workspace selected.\n\nPress Ctrl+W to select a workspace."
	return centerStyle.Render(content)
}

func (v *WorkspaceViewImpl) renderMainContent(m *Model, width, claudeHeight, regularHeight int) string {
	// Claude terminal (top 75%)
	claudeStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(width).
		Height(claudeHeight + 2) // Account for border

	claudeHeader := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Padding(0, 1).
		Render("🤖 Claude Terminal (?agent=claude)")

	// Set content for Claude terminal viewport
	if m.currentWorkspace != nil {
		claudeSessionID := m.currentWorkspace.ID + "-claude"
		if globalShellManager != nil {
			if session := globalShellManager.GetSession(claudeSessionID); session != nil {
				m.workspaceClaudeTerminal.SetContent(string(session.Output))
			} else {
				m.workspaceClaudeTerminal.SetContent("Connecting to Claude terminal...")
			}
		}
	}

	claudeContent := claudeStyle.Render(claudeHeader + "\n" + m.workspaceClaudeTerminal.View())

	// Regular terminal (bottom 25%)
	regularStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(width).
		Height(regularHeight + 2) // Account for border

	regularHeader := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Padding(0, 1).
		Render("💻 Regular Terminal")

	// Set content for regular terminal viewport
	if m.currentWorkspace != nil {
		regularSessionID := m.currentWorkspace.ID + "-regular"
		if globalShellManager != nil {
			if session := globalShellManager.GetSession(regularSessionID); session != nil {
				m.workspaceRegularTerminal.SetContent(string(session.Output))
			} else {
				m.workspaceRegularTerminal.SetContent("Connecting to terminal...")
			}
		}
	}

	regularContent := regularStyle.Render(regularHeader + "\n" + m.workspaceRegularTerminal.View())

	return lipgloss.JoinVertical(
		lipgloss.Left,
		claudeContent,
		regularContent,
	)
}

func (v *WorkspaceViewImpl) renderSidebar(m *Model, width, height int) string {
	sidebarStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(width).
		Height(height + 2). // Account for border
		Padding(1)

	var sections []string
	
	if m.currentWorkspace != nil {
		// Git status section
		sections = append(sections, lipgloss.NewStyle().Bold(true).Render("📝 Git Status"))
		sections = append(sections, fmt.Sprintf("Branch: %s", m.currentWorkspace.Branch))
		sections = append(sections, "")
		
		// Changed files section
		sections = append(sections, lipgloss.NewStyle().Bold(true).Render("📄 Changed Files"))
		if len(m.currentWorkspace.ChangedFiles) > 0 {
			for i, file := range m.currentWorkspace.ChangedFiles {
				if i >= 5 { // Limit display to first 5 files
					sections = append(sections, fmt.Sprintf("... and %d more", len(m.currentWorkspace.ChangedFiles)-5))
					break
				}
				// Extract just filename from path
				filename := file
				if lastSlash := strings.LastIndex(file, "/"); lastSlash != -1 {
					filename = file[lastSlash+1:]
				}
				sections = append(sections, fmt.Sprintf("• %s", filename))
			}
		} else {
			sections = append(sections, "No changes")
		}
		sections = append(sections, "")
		
		// Ports section
		sections = append(sections, lipgloss.NewStyle().Bold(true).Render("🌐 Active Ports"))
		if len(m.currentWorkspace.Ports) > 0 {
			for _, port := range m.currentWorkspace.Ports {
				sections = append(sections, fmt.Sprintf(":%s %s", port.Port, port.Title))
			}
		} else {
			sections = append(sections, "No active ports")
		}
	} else {
		sections = append(sections, "No workspace data available")
	}

	content := strings.Join(sections, "\n")
	return sidebarStyle.Render(content)
}

func (v *WorkspaceViewImpl) handleClaudeOutput(m *Model, msg shellOutputMsg) (*Model, tea.Cmd) {
	// Update the Claude terminal viewport
	m.workspaceClaudeTerminal.SetContent(string(msg.data))
	m.workspaceClaudeTerminal.GotoBottom()
	return m, nil
}

func (v *WorkspaceViewImpl) handleRegularOutput(m *Model, msg shellOutputMsg) (*Model, tea.Cmd) {
	// Update the regular terminal viewport
	m.workspaceRegularTerminal.SetContent(string(msg.data))
	m.workspaceRegularTerminal.GotoBottom()
	return m, nil
}

func (v *WorkspaceViewImpl) handleClaudeError(m *Model, msg shellErrorMsg) (*Model, tea.Cmd) {
	debugLog("Claude terminal error for workspace %s: %v", m.currentWorkspace.ID, msg.err)
	// Could add error display to Claude terminal
	return m, nil
}

func (v *WorkspaceViewImpl) handleRegularError(m *Model, msg shellErrorMsg) (*Model, tea.Cmd) {
	debugLog("Regular terminal error for workspace %s: %v", m.currentWorkspace.ID, msg.err)
	// Could add error display to regular terminal
	return m, nil
}

func (v *WorkspaceViewImpl) forwardToRegularTerminal(m *Model, msg tea.KeyMsg) {
	if m.currentWorkspace == nil {
		return
	}

	// Send input to the regular terminal PTY session
	regularSessionID := m.currentWorkspace.ID + "-regular"
	debugLog("Workspace view forwarding key to regular terminal: %s", msg.String())
	
	if globalShellManager != nil {
		if session := globalShellManager.GetSession(regularSessionID); session != nil && session.Client != nil {
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
						debugLog("Failed to send data to workspace regular terminal: %v", err)
						if globalShellManager != nil && globalShellManager.program != nil {
							globalShellManager.program.Send(shellErrorMsg{
								sessionID: sessionID,
								err:       err,
							})
						}
					}
				}(data, regularSessionID)
			}
		}
	}
}

func (v *WorkspaceViewImpl) resizeWorkspaceTerminals(m *Model, width, claudeHeight, regularHeight int) {
	if m.currentWorkspace == nil {
		return
	}

	if globalShellManager != nil {
		// Resize Claude terminal
		claudeSessionID := m.currentWorkspace.ID + "-claude"
		if session := globalShellManager.GetSession(claudeSessionID); session != nil && session.Client != nil {
			go func() {
				if err := session.Client.Resize(width, claudeHeight); err != nil {
					debugLog("Failed to resize Claude terminal: %v", err)
				}
			}()
		}

		// Resize regular terminal
		regularSessionID := m.currentWorkspace.ID + "-regular"
		if session := globalShellManager.GetSession(regularSessionID); session != nil && session.Client != nil {
			go func() {
				if err := session.Client.Resize(width, regularHeight); err != nil {
					debugLog("Failed to resize regular terminal: %v", err)
				}
			}()
		}
	}
}

// CreateWorkspaceSessions creates both Claude and regular terminal sessions for a workspace
func (v *WorkspaceViewImpl) CreateWorkspaceSessions(m *Model, workspace *WorkspaceInfo) (*Model, tea.Cmd) {
	if workspace == nil {
		return m, nil
	}

	claudeSessionID := workspace.ID + "-claude"
	regularSessionID := workspace.ID + "-regular"

	// Calculate terminal dimensions
	headerHeight := 3
	totalHeight := m.height - headerHeight
	mainWidth := (m.width * 75) / 100
	claudeHeight := (totalHeight * 75) / 100
	regularHeight := totalHeight - claudeHeight - 1

	terminalWidth := mainWidth - 2

	// Initialize viewports
	m.workspaceClaudeTerminal.Width = terminalWidth
	m.workspaceClaudeTerminal.Height = claudeHeight
	m.workspaceRegularTerminal.Width = terminalWidth
	m.workspaceRegularTerminal.Height = regularHeight

	// Create both terminal sessions
	var cmds []tea.Cmd

	// Create Claude terminal session with ?agent=claude parameter
	claudeCmd := createAndConnectWorkspaceShell(claudeSessionID, terminalWidth, claudeHeight, workspace.Path, "?agent=claude")
	cmds = append(cmds, claudeCmd)

	// Create regular terminal session
	regularCmd := createAndConnectWorkspaceShell(regularSessionID, terminalWidth, regularHeight, workspace.Path, "")
	cmds = append(cmds, regularCmd)

	return m, tea.Batch(cmds...)
}

// createAndConnectWorkspaceShell creates a shell session for a workspace with specific parameters
func createAndConnectWorkspaceShell(sessionID string, width, height int, workspacePath, agentParam string) tea.Cmd {
	return func() tea.Msg {
		if globalShellManager == nil {
			return shellErrorMsg{sessionID: sessionID, err: fmt.Errorf("shell manager not initialized")}
		}

		// Create PTY client with workspace-specific settings
		client := NewPTYClient("http://localhost:8080/v1/pty/"+sessionID+agentParam, sessionID)
		
		// Set working directory to workspace path
		client.workingDir = workspacePath
		
		if err := client.Connect(width, height); err != nil {
			return shellErrorMsg{sessionID: sessionID, err: err}
		}

		// Add session to manager
		globalShellManager.AddSession(sessionID, client)

		return shellConnectedMsg{sessionID: sessionID}
	}
}