package tui

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/vanpelt/catnip/internal/services"
)

// ViewType represents the different views in the application
type ViewType int

const (
	// InitializationView represents the initial setup screen
	InitializationView ViewType = iota
	// OverviewView represents the main dashboard view
	OverviewView
	// LogsView represents the logs viewing interface
	LogsView
	// ShellView represents the shell terminal interface
	ShellView
)

// View interface that all views must implement
type View interface {
	// Update handles view-specific message processing
	Update(m *Model, msg tea.Msg) (*Model, tea.Cmd)

	// Render generates the view content
	Render(m *Model) string

	// HandleKey processes key messages for this view
	HandleKey(m *Model, msg tea.KeyMsg) (*Model, tea.Cmd)

	// HandleResize processes window resize messages
	HandleResize(m *Model, msg tea.WindowSizeMsg) (*Model, tea.Cmd)

	// GetViewType returns the view type identifier
	GetViewType() ViewType
}

// PortInfo represents information about an open port
type PortInfo struct {
	Port     string
	Title    string
	Service  string
	Protocol string
}

// Model represents the main application state
type Model struct {
	// Core dependencies
	containerService *services.ContainerService
	codespaceService *services.CodespaceService
	containerName    string
	gitRoot          string
	sseClient        *SSEClient

	// Initialization parameters
	containerImage string
	devMode        bool
	refreshFlag    bool
	customPorts    []string
	sshEnabled     bool
	version        string
	runtime        string
	rmFlag         bool
	envVars        []string
	dind           bool

	// Network configuration
	baseURL      string // Base URL for API calls (e.g., "http://localhost:2287")
	internalPort string // Internal container port (default: 2287)
	externalPort string // External host port (parsed from customPorts)

	// Current state
	currentView      ViewType
	width            int
	height           int
	lastUpdate       time.Time
	err              error
	quitRequested    bool
	upgradeAvailable bool

	// Data state
	containerInfo  map[string]interface{}
	repositoryInfo map[string]interface{}
	containerRepos map[string]interface{}
	logs           []string
	filteredLogs   []string
	ports          []PortInfo

	// Health status and animation
	appHealthy       bool
	bootingAnimDots  int
	bootingBold      bool
	bootingBoldTimer time.Time

	// Enhanced logs view
	logsViewport  viewport.Model
	searchInput   textinput.Model
	searchMode    bool
	searchPattern string
	compiledRegex *regexp.Regexp
	lastLogCount  int

	// Shell view
	shellViewport    viewport.Model
	shellOutput      string
	shellSessions    map[string]*PTYClient
	showSessionList  bool
	currentSessionID string
	shellConnecting  bool
	shellSpinner     spinner.Model
	shellLastInput   time.Time
	terminalEmulator *TerminalEmulator

	// Port selector overlay
	showPortSelector  bool
	selectedPortIndex int

	// SSE connection state
	sseConnected bool
	sseStarted   bool

	// Browser auto-open state
	browserOpened bool

	// View instances
	views map[ViewType]View
}

// NewModel creates a new application model with initialized views
func NewModel(containerService *services.ContainerService, containerName, gitRoot, containerImage string, devMode, refreshFlag bool, customPorts []string, sshEnabled bool, version string, rmFlag bool, envVars []string, dind bool) *Model {
	return NewModelWithInitialization(containerService, containerName, gitRoot, containerImage, devMode, refreshFlag, customPorts, sshEnabled, version, rmFlag, envVars, dind)
}

// NewModelWithInitialization creates a new application model with initialization parameters
func NewModelWithInitialization(containerService *services.ContainerService, containerName, gitRoot, containerImage string, devMode, refreshFlag bool, customPorts []string, sshEnabled bool, version string, rmFlag bool, envVars []string, dind bool) *Model {
	// Get runtime information from container service
	runtime := string(containerService.GetRuntime())

	// Initialize codespace service (ignore errors for now)
	codespaceService, _ := services.NewCodespaceService()

	// Parse port configuration
	internalPort := "2287" // Default internal port
	externalPort := "2287" // Default external port
	if len(customPorts) > 0 {
		// Parse port mapping (e.g., "8181:2287" means external:internal)
		parts := strings.Split(customPorts[0], ":")
		if len(parts) >= 1 {
			externalPort = parts[0]
		}
		if len(parts) >= 2 {
			internalPort = parts[1]
		}
	}

	// Determine base URL (will be overridden by getBaseURL if in codespace)
	// This will be recalculated dynamically by getBaseURL() method
	baseURL := fmt.Sprintf("http://localhost:%s", externalPort)
	if hostURL := os.Getenv("CATNIP_HOST_URL"); hostURL != "" {
		baseURL = hostURL
	}

	m := &Model{
		containerService: containerService,
		codespaceService: codespaceService,
		containerName:    containerName,
		gitRoot:          gitRoot,
		containerImage:   containerImage,
		devMode:          devMode,
		refreshFlag:      refreshFlag,
		customPorts:      customPorts,
		sshEnabled:       sshEnabled,
		version:          version,
		runtime:          runtime,
		rmFlag:           rmFlag,
		envVars:          envVars,
		dind:             dind,
		baseURL:          baseURL,
		internalPort:     internalPort,
		externalPort:     externalPort,
		currentView:      InitializationView,
		containerInfo:    make(map[string]interface{}),
		repositoryInfo:   make(map[string]interface{}),
		containerRepos:   make(map[string]interface{}),
		logs:             []string{},
		filteredLogs:     []string{},
		ports:            []PortInfo{},
		lastUpdate:       time.Now(),
		shellSessions:    make(map[string]*PTYClient),
		views:            make(map[ViewType]View),
	}

	// Initialize views
	m.views[InitializationView] = NewInitializationView()
	m.views[OverviewView] = NewOverviewView()
	m.views[LogsView] = NewLogsView()
	m.views[ShellView] = NewShellView()

	return m
}

// GetCurrentView returns the currently active view
func (m *Model) GetCurrentView() View {
	return m.views[m.currentView]
}

// SwitchToView changes the current view
func (m *Model) SwitchToView(viewType ViewType) {
	m.currentView = viewType
}

// isInCodespace checks if we're running in a GitHub Codespace
func (m *Model) isInCodespace() bool {
	return os.Getenv("CODESPACES") == "true"
}

// getCodespaceName returns the codespace name from environment
func (m *Model) getCodespaceName() string {
	return os.Getenv("CODESPACE_NAME")
}

// createAuthenticatedClient creates an HTTP client with codespace token if available
func (m *Model) createAuthenticatedClient(timeout time.Duration) *http.Client {
	client := &http.Client{Timeout: timeout}

	// If we're in a codespace and have a token, add it to requests
	if m.isInCodespace() && m.codespaceService != nil {
		if token, err := m.codespaceService.LoadCodespaceToken(); err == nil && token != "" {
			// Create a transport that adds the Authorization header
			client.Transport = &authTransport{
				token: token,
				base:  http.DefaultTransport,
			}
		}
	}

	return client
}

// getHost returns just the host part (e.g., "localhost" or "mycodespace-2287.app.github.dev")
func (m *Model) getHost() string {
	// If we're in a codespace, return the codespace host
	if m.isInCodespace() && m.codespaceService != nil {
		codespaceName := m.getCodespaceName()
		if codespaceName != "" {
			// Return codespace host without protocol or port
			return fmt.Sprintf("%s-%s.app.github.dev", codespaceName, m.externalPort)
		}
	}

	// Check for custom host URL
	if hostURL := os.Getenv("CATNIP_HOST_URL"); hostURL != "" {
		// Parse the URL to extract just the host
		hostURL = strings.TrimPrefix(hostURL, "http://")
		hostURL = strings.TrimPrefix(hostURL, "https://")
		if idx := strings.Index(hostURL, ":"); idx > 0 {
			return hostURL[:idx] // Return just the host part
		}
		if idx := strings.Index(hostURL, "/"); idx > 0 {
			return hostURL[:idx]
		}
		return hostURL
	}

	// Default to localhost
	return "localhost"
}

// getProtocol returns the protocol (http or https) based on the environment
func (m *Model) getProtocol() string {
	// Codespaces always use HTTPS
	if m.isInCodespace() {
		return "https"
	}

	// Check if custom URL has https
	if hostURL := os.Getenv("CATNIP_HOST_URL"); hostURL != "" {
		if strings.HasPrefix(hostURL, "https://") {
			return "https"
		}
	}

	return "http"
}

// getBaseURL returns the appropriate base URL (local or codespace)
func (m *Model) getBaseURL(port string) string {
	if m.isInCodespace() && m.codespaceService != nil {
		codespaceName := m.getCodespaceName()
		if codespaceName != "" {
			return m.codespaceService.GetCodespaceURL(codespaceName, port)
		}
	}

	// Use configured base URL if no specific port is provided
	if port == "" {
		return m.baseURL
	}

	// Otherwise build URL with specific port
	// Extract host from baseURL
	if hostURL := os.Getenv("CATNIP_HOST_URL"); hostURL != "" {
		// If custom host URL is set, replace just the port
		if idx := strings.LastIndex(hostURL, ":"); idx > 0 && idx < len(hostURL)-1 {
			// Has port in URL
			return hostURL[:idx+1] + port
		}
		// No port in URL, add it
		return hostURL + ":" + port
	}

	return fmt.Sprintf("%s://%s:%s", m.getProtocol(), m.getHost(), port)
}

// authTransport adds Authorization header to HTTP requests
type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}
