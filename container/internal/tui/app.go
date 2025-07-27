package tui

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
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

var debugLogger *log.Logger
var debugEnabled bool

func init() {
	debugEnabled = os.Getenv("DEBUG") == "true"
	if debugEnabled {
		logFile, err := os.OpenFile("/tmp/catctrl-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			log.Fatalln("Failed to open debug log file:", err)
		}
		debugLogger = log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)
		debugLogger.Println("=== TUI DEBUG LOG STARTED ===")
	}
}

func debugLog(format string, args ...interface{}) {
	if debugEnabled && debugLogger != nil {
		debugLogger.Printf(format+"\n", args...)
	}
}

// App represents the main TUI application
type App struct {
	containerService *services.ContainerService
	containerName    string
	program          *tea.Program
	sseClient        *SSEClient

	// Initialization parameters
	containerImage string
	devMode        bool
	refreshFlag    bool
	sshEnabled     bool
	version        string
}

// NewApp creates a new application instance
func NewApp(containerService *services.ContainerService, containerName, workDir, containerImage string, devMode, refreshFlag bool, customPorts []string, sshEnabled bool, version string) *App {
	return &App{
		containerService: containerService,
		containerName:    containerName,
		containerImage:   containerImage,
		devMode:          devMode,
		refreshFlag:      refreshFlag,
		sshEnabled:       sshEnabled,
		version:          version,
	}
}

// Run starts the TUI application
func (a *App) Run(ctx context.Context, workDir string, customPorts []string) error {
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

	// Initialize SSE client
	sseClient := NewSSEClient("http://localhost:8080/v1/events", nil)

	// Create the model - always with initialization
	m := NewModel(a.containerService, a.containerName, workDir, a.containerImage, a.devMode, a.refreshFlag, customPorts, a.sshEnabled, a.version)
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

	_, err := a.program.Run()

	// Clean up SSE client if it was started
	if a.sseClient != nil {
		a.sseClient.Stop()
	}

	return err
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
	header := headerStyle.Render(fmt.Sprintf("🐱 Catnip - %s", m.version))

	// Footer
	footer := m.renderFooter()

	// Main content area
	mainHeight := m.height - 4 // Account for header and footer
	mainStyle := components.MainContentStyle.Width(m.width - 2).Height(mainHeight)
	mainContent := mainStyle.Render(content)

	result := lipgloss.JoinVertical(lipgloss.Left, header, mainContent, footer)
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
		return footerStyle.Render("Ctrl+L: logs | Ctrl+T: terminal | Ctrl+0: UI | Ctrl+1-9: ports | Ctrl+Q: quit")
	case ShellView:
		scrollKey := "Alt"
		if runtime.GOOS == "darwin" {
			scrollKey = "Option"
		}
		return footerStyle.Render(fmt.Sprintf("Ctrl+O: overview | Ctrl+L: logs | Ctrl+Q: quit | %s+↑↓/PgUp/PgDn: scroll", scrollKey))
	case LogsView:
		if m.searchMode {
			// Replace footer with search input
			searchPrompt := "Search: "
			searchContent := searchPrompt + m.searchInput.View() + " (Enter to apply, Esc to cancel)"
			return footerStyle.Render(searchContent)
		} else {
			if m.searchPattern != "" {
				return footerStyle.Render("/ search, c clear filter, ↑↓ scroll, Ctrl+O overview, Ctrl+Q quit • Streaming filtered logs")
			} else {
				return footerStyle.Render("/ search, c clear filter, ↑↓ scroll, Ctrl+O overview, Ctrl+Q quit • Auto-refresh: ON")
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
