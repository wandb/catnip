package tui

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vanpelt/catnip/internal/tui/components"
)

// OverviewViewImpl handles the main dashboard view
type OverviewViewImpl struct{}

// NewOverviewView creates a new overview view instance
func NewOverviewView() *OverviewViewImpl {
	return &OverviewViewImpl{}
}

// GetViewType returns the view type identifier
func (v *OverviewViewImpl) GetViewType() ViewType {
	return OverviewView
}

// Update handles overview-specific message processing
func (v *OverviewViewImpl) Update(m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	// Override in main update loop - overview view doesn't handle updates directly
	return m, nil
}

// HandleKey processes key messages for the overview view
func (v *OverviewViewImpl) HandleKey(m *Model, msg tea.KeyMsg) (*Model, tea.Cmd) {
	switch msg.String() {
	case components.KeyQuit, components.KeyQuitAlt:
		return m, tea.Quit

	case components.KeyLogs:
		if m.currentView == LogsView {
			m.SwitchToView(OverviewView)
		} else {
			m.SwitchToView(LogsView)
			// Update viewport size and content
			if m.height > 0 {
				headerHeight := 4
				m.logsViewport.Width = m.width - 4
				m.logsViewport.Height = m.height - headerHeight
			}
			m = v.updateLogFilter(m)
			return m, v.fetchLogs(m)
		}

	case components.KeyOverview:
		m.SwitchToView(OverviewView)

	case components.KeyShell:
		switch m.currentView {
		case OverviewView:
			// Check if we have existing sessions
			if globalShellManager != nil && len(globalShellManager.sessions) > 0 {
				m.showSessionList = true
				m.SwitchToView(ShellView)
			} else {
				// Create new session
				return v.createNewShellSessionWithCmd(m)
			}
		case ShellView:
			// Already in shell view, do nothing
			return m, nil
		}

	case components.KeyOpenUI:
		if m.appHealthy {
			go func() {
				_ = v.openBrowser("http://localhost:8080")
			}()
		} else {
			// App is not ready, show bold feedback
			m.bootingBold = true
			m.bootingBoldTimer = time.Now()
		}

	default:
		// Handle port keys (1-9)
		if components.IsPortKey(msg.String()) {
			portIndex := components.GetPortIndex(msg.String())
			if portIndex < len(m.ports) {
				portInfo := m.ports[portIndex]
				url := fmt.Sprintf("http://localhost:8080/%s", portInfo.Port)
				go func() {
					if v.isAppReady("http://localhost:8080") {
						_ = v.openBrowser(url)
					}
				}()
			}
		}
	}

	return m, nil
}

// HandleResize processes window resize for the overview view
func (v *OverviewViewImpl) HandleResize(m *Model, msg tea.WindowSizeMsg) (*Model, tea.Cmd) {
	// Overview view doesn't need special resize handling
	return m, nil
}

// Render generates the overview view content
func (v *OverviewViewImpl) Render(m *Model) string {
	// Check if we have enough width for the logo (70 cols + 70 for content = 140+ total)
	showLogo := m.width >= 150 && len(m.logo) > 0

	var sections []string

	// Container Status
	sections = append(sections, components.SectionHeaderStyle.Render("📦 Container Status"))

	if m.containerInfo["name"] != nil {
		sections = append(sections, fmt.Sprintf("  Name: %v", m.containerInfo["name"]))
		sections = append(sections, fmt.Sprintf("  Runtime: %v", m.containerInfo["runtime"]))
		sections = append(sections, fmt.Sprintf("  Last updated: %s", m.lastUpdate.Format("15:04:05")))

		// SSE connection status
		if m.sseConnected {
			sseStatus := components.StatusConnectedStyle.Render("● Connected")
			sections = append(sections, fmt.Sprintf("  Events: %s", sseStatus))
		} else {
			sseStatus := components.StatusDisconnectedStyle.Render("● Disconnected")
			sections = append(sections, fmt.Sprintf("  Events: %s (using polling)", sseStatus))
		}
	} else {
		sections = append(sections, "  Status: Starting...")
	}

	sections = append(sections, "")

	// Main UI section
	sections = append(sections, components.SubHeaderStyle.Render("🖥️  Catnip UI"))

	// Show booting animation if not healthy
	if !m.appHealthy {
		dots := strings.Repeat(".", m.bootingAnimDots)
		spaces := strings.Repeat(" ", 3-m.bootingAnimDots)
		bootingText := fmt.Sprintf("Booting%s%s", dots, spaces)

		if m.bootingBold {
			bootingStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(components.ColorAccent))
			sections = append(sections, fmt.Sprintf("  %s %s", components.KeyHighlightStyle.Render("0."), bootingStyle.Render(bootingText)))
		} else {
			sections = append(sections, fmt.Sprintf("  %s %s", components.KeyHighlightStyle.Render("0."), bootingText))
		}
	} else {
		sections = append(sections, fmt.Sprintf("  %s Main UI → http://localhost:8080", components.KeyHighlightStyle.Render("0.")))
	}
	sections = append(sections, "")

	// Ports
	if len(m.ports) > 0 {
		sections = append(sections, components.SubHeaderStyle.Render("🌐 Detected Services"))

		for i, portInfo := range m.ports {
			if i < 9 { // Only show first 9 ports for number shortcuts
				portKey := components.KeyHighlightStyle.Render(fmt.Sprintf("%d.", i+1))
				sections = append(sections, fmt.Sprintf("  %s %s → http://localhost:8080/%s", portKey, portInfo.Title, portInfo.Port))
			} else {
				sections = append(sections, fmt.Sprintf("     %s → http://localhost:8080/%s", portInfo.Title, portInfo.Port))
			}
		}
	} else {
		sections = append(sections, "🌐 No services detected")
	}

	sections = append(sections, "")

	// Repository Info
	sections = append(sections, components.SectionHeaderStyle.Render("📁 Repository Info"))

	if isGitRepo, ok := m.repositoryInfo["is_git_repo"].(bool); ok && isGitRepo {
		if repoName, ok := m.repositoryInfo["repo_name"].(string); ok {
			sections = append(sections, fmt.Sprintf("  Repository: %s", repoName))
		}
		if branch, ok := m.repositoryInfo["current_branch"].(string); ok {
			sections = append(sections, fmt.Sprintf("  Branch: %s", branch))
		}
		if origin, ok := m.repositoryInfo["remote_origin"].(string); ok {
			sections = append(sections, fmt.Sprintf("  Origin: %s", origin))
		}
	} else {
		sections = append(sections, "  No git repository detected")
		sections = append(sections, "  Container running without mounted code")
	}

	// Container repositories (from API)
	if repos, ok := m.containerRepos["repositories"].([]interface{}); ok && len(repos) > 0 {
		sections = append(sections, "")
		sections = append(sections, "  Container Repositories:")
		for i, repo := range repos {
			if repoMap, ok := repo.(map[string]interface{}); ok {
				if name, ok := repoMap["name"].(string); ok {
					sections = append(sections, fmt.Sprintf("    %d. %s", i+1, name))
				}
			}
		}
	}

	sections = append(sections, "")

	// System Info
	sections = append(sections, components.SubHeaderStyle.Render("📊 System Info"))
	if statsStr, ok := m.containerInfo["stats"].(string); ok && strings.TrimSpace(statsStr) != "" {
		lines := strings.Split(strings.TrimSpace(statsStr), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				sections = append(sections, fmt.Sprintf("  %s", line))
			}
		}
	} else {
		// Only show "Loading..." if we don't have any container info yet
		if len(m.containerInfo) == 0 {
			sections = append(sections, "  Stats: Loading...")
		} else {
			sections = append(sections, "  Stats: Unavailable")
		}
	}

	// Error display
	if m.err != nil {
		sections = append(sections, "")
		sections = append(sections, components.ErrorStyle.Render("❌ Error"))
		sections = append(sections, fmt.Sprintf("  %s", m.err.Error()))
	}

	if !showLogo {
		return strings.Join(sections, "\n")
	}

	// Combine content and logo side by side
	contentText := strings.Join(sections, "\n")
	return v.renderWithLogo(m, contentText)
}

// Helper methods

func (v *OverviewViewImpl) updateLogFilter(m *Model) *Model {
	// Simplified log filter update for overview
	if m.searchPattern == "" {
		m.filteredLogs = m.logs
		m.compiledRegex = nil
	}
	m.logsViewport.SetContent(strings.Join(m.filteredLogs, "\n"))
	m.logsViewport.GotoBottom()
	return m
}

func (v *OverviewViewImpl) fetchLogs(m *Model) tea.Cmd {
	return func() tea.Msg {
		// This will be moved to commands.go
		return nil
	}
}

func (v *OverviewViewImpl) createNewShellSessionWithCmd(m *Model) (*Model, tea.Cmd) {
	sessionID := fmt.Sprintf("shell-%d", time.Now().Unix())
	m.currentSessionID = sessionID
	m.SwitchToView(ShellView)
	m.shellOutput = ""
	m.shellConnecting = true
	m.shellLastInput = time.Now()

	// Initialize shell viewport
	if m.height > 0 {
		headerHeight := 3
		m.shellViewport.Width = m.width - 2
		m.shellViewport.Height = m.height - headerHeight
		terminalWidth := m.shellViewport.Width - 2
		if m.terminalEmulator == nil {
			m.terminalEmulator = NewTerminalEmulator(terminalWidth, m.shellViewport.Height)
		} else {
			m.terminalEmulator.Clear()
			m.terminalEmulator.Resize(terminalWidth, m.shellViewport.Height)
		}
	}

	terminalWidth := m.shellViewport.Width - 2
	return m, createAndConnectShell(sessionID, terminalWidth, m.shellViewport.Height)
}

func (v *OverviewViewImpl) renderWithLogo(m *Model, content string) string {
	contentLines := strings.Split(content, "\n")
	logoLines := m.logo

	// Calculate available space
	contentWidth := 70 // Reserve 70 columns for content

	// Pad content lines to the specified width
	for i, line := range contentLines {
		// Strip any existing color codes for width calculation
		stripped := v.stripAnsi(line)
		if len(stripped) < contentWidth {
			contentLines[i] = line + strings.Repeat(" ", contentWidth-len(stripped))
		} else if len(stripped) > contentWidth {
			contentLines[i] = line[:contentWidth]
		}
	}

	// Ensure we have enough content lines to match logo height
	maxLines := len(logoLines)
	if len(contentLines) < maxLines {
		for len(contentLines) < maxLines {
			contentLines = append(contentLines, strings.Repeat(" ", contentWidth))
		}
	}

	// Combine content and logo
	var result []string
	for i := 0; i < maxLines; i++ {
		contentLine := ""
		if i < len(contentLines) {
			contentLine = contentLines[i]
		} else {
			contentLine = strings.Repeat(" ", contentWidth)
		}

		logoLine := ""
		if i < len(logoLines) {
			logoLine = logoLines[i]
		}

		// Add some spacing between content and logo
		combined := contentLine + "  " + logoLine
		result = append(result, combined)
	}

	return strings.Join(result, "\n")
}

func (v *OverviewViewImpl) stripAnsi(s string) string {
	// Simple regex-like approach to remove ANSI sequences
	var result strings.Builder
	inEscape := false

	for _, r := range s {
		if r == '\033' { // ESC character
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' { // End of color sequence
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}

	return result.String()
}

func (v *OverviewViewImpl) openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}

func (v *OverviewViewImpl) isAppReady(baseURL string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
