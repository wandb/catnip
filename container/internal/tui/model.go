package tui

import (
	"regexp"
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
func NewModel(containerService *services.ContainerService, containerName, gitRoot, containerImage string, devMode, refreshFlag bool, customPorts []string, sshEnabled bool, version string, rmFlag bool) *Model {
	return NewModelWithInitialization(containerService, containerName, gitRoot, containerImage, devMode, refreshFlag, customPorts, sshEnabled, version, rmFlag)
}

// NewModelWithInitialization creates a new application model with initialization parameters
func NewModelWithInitialization(containerService *services.ContainerService, containerName, gitRoot, containerImage string, devMode, refreshFlag bool, customPorts []string, sshEnabled bool, version string, rmFlag bool) *Model {
	// Get runtime information from container service
	runtime := string(containerService.GetRuntime())

	m := &Model{
		containerService: containerService,
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
