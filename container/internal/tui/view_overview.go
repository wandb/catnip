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
// Note: Global navigation keys (Ctrl+O, Ctrl+L, Ctrl+T, etc.) are handled in the global handler
func (v *OverviewViewImpl) HandleKey(m *Model, msg tea.KeyMsg) (*Model, tea.Cmd) {
	// Overview view has no view-specific keys - all navigation is now global
	// Any unhandled keys are just ignored in overview view
	return m, nil
}

// HandleResize processes window resize for the overview view
func (v *OverviewViewImpl) HandleResize(m *Model, msg tea.WindowSizeMsg) (*Model, tea.Cmd) {
	// Overview view doesn't need special resize handling
	return m, nil
}

// Render generates the overview view content
func (v *OverviewViewImpl) Render(m *Model) string {
	// Check if we have enough width for the logo (110 is minimum)
	showLogo := m.width >= 110

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

	// Determine the actual port from customPorts
	mainPort := "8080"
	for _, p := range m.customPorts {
		// Parse port mapping (e.g., "8181:8080" or "8080:8080")
		parts := strings.Split(p, ":")
		if len(parts) >= 1 {
			mainPort = parts[0]
			break
		}
	}

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
		sections = append(sections, fmt.Sprintf("  %s Main UI → http://localhost:%s", components.KeyHighlightStyle.Render("0."), mainPort))
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
	return v.renderWithASCIIView(m, contentText)
}

// Helper methods

func (v *OverviewViewImpl) createNewShellSessionWithCmd(m *Model) (*Model, tea.Cmd) {
	var sessionID string
	// Use "default" for the first shell, then fall back to timestamp format
	if globalShellManager == nil || len(globalShellManager.sessions) == 0 {
		sessionID = "default"
	} else {
		sessionID = fmt.Sprintf("shell-%d", time.Now().Unix())
	}
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

func (v *OverviewViewImpl) renderWithASCIIView(m *Model, content string) string {
	// Load the appropriate logo based on current width
	logo := loadLogo(m.width)

	// If no logo should be shown, just return content
	if len(logo) == 0 {
		return content
	}

	// Calculate column widths - content gets 40% of space
	contentWidth := (m.width * 40) / 100 // Content gets 40% of space
	if contentWidth < 40 {
		contentWidth = 40 // Minimum content width
	}
	logoWidth := m.width - contentWidth - 4 // Logo gets remainder minus padding

	if logoWidth < 20 {
		// If logo column is too narrow, just return content only
		return content
	}

	// Render the ASCII art with proper spacing
	logoContent := strings.Join(logo, "\n")

	// Create content column with proper styling and width constraint
	contentColumn := lipgloss.NewStyle().
		Width(contentWidth).
		Align(lipgloss.Left).
		Render(content)

	// Use lipgloss JoinHorizontal but keep logo raw (logoContent already defined above)
	result := lipgloss.JoinHorizontal(
		lipgloss.Top, // Align to top
		contentColumn,
		strings.Repeat(" ", 4), // 4 spaces between columns
		logoContent,            // Raw ASCII art with original ANSI codes
	)

	return result
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
