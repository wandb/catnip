package tui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vanpelt/catnip/internal/services"
	"github.com/vanpelt/catnip/internal/tui/components"
)

//go:embed logo.ascii
var embeddedLogo string

//go:embed logo-small.ascii
var embeddedSmallLogo string

var debugEnabled bool

func init() {
	debugEnabled = os.Getenv("DEBUG") == "true"
	if debugEnabled {
		writeToDebugFile("=== TUI DEBUG LOG STARTED ===")
	}
}

func debugLog(format string, args ...interface{}) {
	if debugEnabled {
		msg := fmt.Sprintf(format, args...)
		writeToDebugFile(msg)
	}
}

func writeToDebugFile(msg string) {
	file, err := os.OpenFile("/tmp/catnip-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	defer file.Close()

	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	_, _ = fmt.Fprintf(file, "%s TUI: %s\n", timestamp, msg)
}

// App represents the main TUI application
type App struct {
	containerService *services.ContainerService
	containerName    string
	program          *tea.Program
	sseClient        *SSEClient
	portForwarder    *PortForwardManager
	powerManager     *HostPowerManager

	// Initialization parameters
	containerImage string
	devMode        bool
	refreshFlag    bool
	sshEnabled     bool
	version        string
	runtime        string
	rmFlag         bool
	envVars        []string
	dind           bool
}

// NewApp creates a new application instance
func NewApp(containerService *services.ContainerService, containerName, workDir, containerImage string, devMode, refreshFlag bool, customPorts []string, sshEnabled bool, version string, rmFlag bool, envVars []string, dind bool) *App {
	// Get runtime information from container service
	runtime := string(containerService.GetRuntime())

	return &App{
		containerService: containerService,
		containerName:    containerName,
		containerImage:   containerImage,
		devMode:          devMode,
		refreshFlag:      refreshFlag,
		sshEnabled:       sshEnabled,
		version:          version,
		runtime:          runtime,
		rmFlag:           rmFlag,
		envVars:          envVars,
		dind:             dind,
	}
}

// Run starts the TUI application and returns the final active container name
func (a *App) Run(ctx context.Context, workDir string, customPorts []string) (string, error) {
	// Initialize search input
	searchInput := textinput.New()
	searchInput.Placeholder = "Enter search pattern (regex supported)..."
	searchInput.CharLimit = 100
	searchInput.Width = 50
	searchInput.Prompt = "🔍 "
	searchInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(components.ColorPrimary)).Bold(true)
	searchInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(components.ColorText))
	searchInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(components.ColorAccent)).Bold(true)

	// Initialize viewports
	logsViewport := viewport.New(80, 20)
	shellViewport := viewport.New(80, 24)

	// Determine the actual port from customPorts
	mainPort := "8080"
	for _, p := range customPorts {
		// Parse port mapping (e.g., "8181:8080" or "8080:8080")
		parts := strings.Split(p, ":")
		if len(parts) >= 1 {
			mainPort = parts[0]
			break
		}
	}

	// Initialize power manager (only on macOS)
	a.powerManager = NewHostPowerManager()

	// Initialize SSE client
	sseClient := NewSSEClient(fmt.Sprintf("http://localhost:%s/v1/events", mainPort), nil)

	// Set up worktree update hook for power management
	sseClient.onWorktreeUpdate = func(worktrees []WorktreeInfo) {
		if a.powerManager != nil {
			a.powerManager.UpdateWorktreeBatch(worktrees)
		}
	}

	// Initialize port forwarder (uses backend on mainPort)
	a.portForwarder = NewPortForwardManager(fmt.Sprintf("http://localhost:%s", mainPort))
	// Start forwarding when ports open (only if SSH enabled)
	sseClient.onEvent = func(ev AppEvent) {
		if !a.sshEnabled || a.portForwarder == nil {
			return
		}
		switch ev.Type {
		case PortOpenedEvent:
			if payload, ok := ev.Payload.(map[string]interface{}); ok {
				if pf, ok := payload["port"].(float64); ok {
					cp := int(pf)
					a.portForwarder.EnsureForward(cp)
					// Note: EnsureForward already announces the new mapping
				}
			}
		case PortClosedEvent:
			if payload, ok := ev.Payload.(map[string]interface{}); ok {
				if pf, ok := payload["port"].(float64); ok {
					cp := int(pf)
					a.portForwarder.StopForward(cp)
				}
			}
		}
	}

	// Create the model - always with initialization
	m := NewModel(a.containerService, a.containerName, workDir, a.containerImage, a.devMode, a.refreshFlag, customPorts, a.sshEnabled, a.version, a.rmFlag, a.envVars, a.dind)
	m.logsViewport = logsViewport
	m.searchInput = searchInput
	m.shellViewport = shellViewport
	m.shellSpinner = spinner.New()
	m.sseClient = sseClient
	m.sseStarted = true // SSE will be started immediately

	// Initialize spinner
	m.shellSpinner.Spinner = spinner.Dot
	m.shellSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	a.program = tea.NewProgram(m, tea.WithAltScreen())

	// Initialize the shell manager with the program
	InitShellManager(a.program)

	// Update SSE client with the program reference
	sseClient.program = a.program
	a.sseClient = sseClient

	// Start SSE client immediately
	sseClient.Start()

	finalModel, err := a.program.Run()

	// Clean up SSE client if it was started
	if a.sseClient != nil {
		a.sseClient.Stop()
	}
	if a.portForwarder != nil {
		a.portForwarder.StopAll()
	}
	// Clean up power manager
	if a.powerManager != nil {
		a.powerManager.Shutdown()
	}

	// Best-effort terminal reset to avoid leaving the user's terminal in an odd state
	// Reset SGR, re-enable line wrap, show cursor, and try to restore sane tty settings
	_, _ = os.Stdout.WriteString("\x1b[0m\x1b[?7h\x1b[?25h")
	_ = exec.Command("stty", "sane").Run()

	// Try to extract the final container name from the model
	finalName := a.containerName
	if finalModel != nil {
		if m, ok := finalModel.(Model); ok {
			if strings.TrimSpace(m.containerName) != "" {
				finalName = m.containerName
			}
		}
	}

	return finalName, err
}

// Init initializes the model and returns initial commands
func (m Model) Init() tea.Cmd {
	return m.initCommands()
}

// View renders the current view
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	// Get content from current view
	content := m.GetCurrentView().Render(&m)

	// Header
	headerStyle := components.HeaderStyle.Width(m.width-2).Padding(0, 1)
	headerText := fmt.Sprintf("🐱 Catnip - %s (%s)", m.version, m.runtime)
	if m.upgradeAvailable {
		headerText += " • ⚠️ Upgrade Available"
	}
	header := headerStyle.Render(headerText)

	// Footer
	footer := m.renderFooter()

	// Main content area
	mainHeight := m.height - 4 // Account for header and footer
	mainStyle := components.MainContentStyle.Width(m.width - 2).Height(mainHeight)
	mainContent := mainStyle.Render(content)

	result := lipgloss.JoinVertical(lipgloss.Left, header, mainContent, footer)

	// Overlay port selector if active
	if m.showPortSelector {
		overlay := m.renderPortSelector()
		result = m.overlayOnContent(result, overlay)
	}

	return result
}

// renderFooter renders the appropriate footer for the current view
func (m Model) renderFooter() string {
	footerStyle := components.FooterStyle.Width(m.width - 2)

	switch m.currentView {
	case InitializationView:
		if initView, ok := m.views[InitializationView].(*InitializationViewImpl); ok && initView.currentAction != "" {
			return footerStyle.Render(fmt.Sprintf("%s • Press Ctrl+Q to quit", initView.currentAction))
		}
		return footerStyle.Render("Initializing container... Press Ctrl+Q to quit")
	case OverviewView:
		return footerStyle.Render("Ctrl+L: logs | Ctrl+T: terminal | Ctrl+B: browser | Ctrl+Q: quit")
	case ShellView:
		scrollKey := "Alt"
		if runtime.GOOS == "darwin" {
			scrollKey = "Option"
		}
		return footerStyle.Render(fmt.Sprintf("Ctrl+O: overview | Ctrl+L: logs | Ctrl+B: browser | Ctrl+Q: quit | %s+↑↓/PgUp/PgDn: scroll", scrollKey))
	case LogsView:
		if m.searchMode {
			// Replace footer with search input
			searchPrompt := "Search: "
			searchContent := searchPrompt + m.searchInput.View() + " (Enter to apply, Esc to cancel)"
			return footerStyle.Render(searchContent)
		} else {
			if m.searchPattern != "" {
				return footerStyle.Render("/ search, c clear filter, ↑↓ scroll, Ctrl+O overview, Ctrl+B browser, Ctrl+Q quit • Streaming filtered logs")
			} else {
				return footerStyle.Render("/ search, c clear filter, ↑↓ scroll, Ctrl+O overview, Ctrl+B browser, Ctrl+Q quit • Auto-refresh: ON")
			}
		}
	}
	return footerStyle.Render("")
}

// Helper functions that are still needed

// loadLogo reads the ASCII logo from the embedded string with fallback based on width
func loadLogo(width int) []string {
	var logoContent string

	// Use smaller logo for medium widths, no logo for small widths
	if width < 100 {
		return []string{} // No logo for very narrow terminals
	} else if width <= 140 {
		logoContent = embeddedSmallLogo // Small logo for medium terminals (100-140)
	} else {
		logoContent = embeddedLogo // Full logo for wide terminals (>140)
	}

	lines := strings.Split(logoContent, "\n")
	return lines
}

// isAppReady checks if the app is ready by hitting the /health endpoint
func isAppReady(baseURL string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// renderPortSelector renders the port selection overlay
func (m Model) renderPortSelector() string {
	// Create port list with main app option
	items := []string{"🏠 Main App (localhost:8080)"}

	// Add detected ports
	for _, port := range m.ports {
		if port.Port != "8080" {
			title := port.Title
			if title == "" {
				title = fmt.Sprintf("Port %s", port.Port)
			}
			items = append(items, fmt.Sprintf("🔗 %s (localhost:8080/%s)", title, port.Port))
		}
	}

	// Build the menu content
	var menuItems []string
	for i, item := range items {
		prefix := "  "
		if i == m.selectedPortIndex {
			prefix = "▶ "
		}
		menuItems = append(menuItems, prefix+item)
	}

	// Add instructions
	instructions := []string{
		"",
		"↑↓/jk: Navigate • Enter/1-9: Select • Esc: Cancel",
	}

	content := append(menuItems, instructions...)
	menuContent := strings.Join(content, "\n")

	// Style the menu box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("15"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Align(lipgloss.Center)

	title := titleStyle.Render("🌐 Select Browser Target")

	return boxStyle.Render(title + "\n\n" + menuContent)
}

// overlayOnContent centers an overlay on top of the main content
func (m Model) overlayOnContent(content, overlay string) string {
	// Use lipgloss.Place to properly center the overlay
	centeredOverlay := lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		overlay,
		lipgloss.WithWhitespaceChars(" "),
	)

	return centeredOverlay
}

// ContainerVersionInfo represents the response from /v1/info endpoint
type ContainerVersionInfo struct {
	Version string `json:"version"`
	Build   struct {
		Commit  string `json:"commit"`
		Date    string `json:"date"`
		BuiltBy string `json:"builtBy"`
	} `json:"build"`
}

// fetchContainerVersion fetches the version information from the running container
func fetchContainerVersion() (*ContainerVersionInfo, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:8080/v1/info")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch container version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("container version endpoint returned status %d", resp.StatusCode)
	}

	var versionInfo ContainerVersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		return nil, fmt.Errorf("failed to decode container version response: %w", err)
	}

	return &versionInfo, nil
}

// compareVersions compares two version strings and returns true if they differ
// This is a simple string comparison - for more complex versioning, a proper semver library could be used
func compareVersions(cliVersion, containerVersion string) bool {
	// Remove "v" prefix if present and normalize
	cliVersion = strings.TrimPrefix(cliVersion, "v")
	containerVersion = strings.TrimPrefix(containerVersion, "v")

	// Simple string comparison - different versions indicate an upgrade is available
	return cliVersion != containerVersion
}
