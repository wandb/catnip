package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/vanpelt/catnip/internal/tui/components"
)

// Update is the main update function that routes messages to appropriate handlers
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// First, handle global window sizing
	if windowMsg, ok := msg.(tea.WindowSizeMsg); ok {
		return m.handleWindowResize(windowMsg)
	}

	// Route key messages to current view
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKeyMessage(keyMsg)
	}

	// Handle spinner updates
	if spinnerMsg, ok := msg.(spinner.TickMsg); ok {
		return m.handleSpinnerTick(spinnerMsg)
	}

	// Route other messages by type
	switch msg := msg.(type) {
	case tickMsg:
		return m.handleTick(msg)
	case animationTickMsg:
		return m.handleAnimationTick(msg)
	case logsTickMsg:
		return m.handleLogsTick(msg)
	case containerInfoMsg:
		return m.handleContainerInfo(msg)
	case repositoryInfoMsg:
		return m.handleRepositoryInfo(msg)
	case containerReposMsg:
		return m.handleContainerRepos(msg)
	case logsMsg:
		return m.handleLogs(msg)
	case portsMsg:
		return m.handlePorts(msg)
	case healthStatusMsg:
		return m.handleHealthStatus(msg)
	case workspacesMsg:
		return m.handleWorkspaces(msg)
	case sseWorktreeUpdatedMsg:
		return m.handleSSEWorktreeUpdated(msg)
	case errMsg:
		return m.handleError(msg)
	case quitMsg:
		return m, tea.Quit
	case sseConnectedMsg:
		return m.handleSSEConnected(msg)
	case sseDisconnectedMsg:
		return m.handleSSEDisconnected(msg)
	case ssePortOpenedMsg:
		return m.handleSSEPortOpened(msg)
	case ssePortClosedMsg:
		return m.handleSSEPortClosed(msg)
	case sseContainerStatusMsg:
		return m.handleSSEContainerStatus(msg)
	case sseErrorMsg:
		return m.handleSSEError(msg)
	case shellOutputMsg:
		return m.handleShellOutput(msg)
	case shellErrorMsg:
		return m.handleShellError(msg)
	case VersionCheckMsg:
		return m.handleVersionCheck(msg)
	}

	// Let current view handle any remaining messages
	newModel, cmd := m.GetCurrentView().Update(&m, msg)
	return *newModel, cmd
}

// Window resize handler
func (m Model) handleWindowResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// Let current view handle resize specifics
	newModel, cmd := m.GetCurrentView().HandleResize(&m, msg)
	return *newModel, cmd
}

// Key message router with global key handling
func (m Model) handleKeyMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	debugLog("KeyMsg received: %s", msg.String())

	// Handle global navigation keys first (available in all views)
	if newModel, cmd, handled := m.handleGlobalKeys(msg); handled {
		return *newModel, cmd
	}

	// Let current view handle the key
	newModel, cmd := m.GetCurrentView().HandleKey(&m, msg)
	return *newModel, cmd
}

// handleGlobalKeys processes global navigation keys (available in all views)
func (m Model) handleGlobalKeys(msg tea.KeyMsg) (*Model, tea.Cmd, bool) {
	keyStr := msg.String()

	switch keyStr {
	case components.KeyQuit, components.KeyQuitAlt:
		m.quitRequested = true
		return &m, tea.Quit, true

	case components.KeyOverview:
		if m.currentView != OverviewView {
			m.SwitchToView(OverviewView)
		}
		return &m, nil, true

	case components.KeyLogs:
		if m.currentView != LogsView {
			m.SwitchToView(LogsView)
			// Update viewport size and content when switching to logs
			if m.height > 0 {
				headerHeight := 4
				m.logsViewport.Width = m.width - 4
				m.logsViewport.Height = m.height - headerHeight
			}
			// Update log filter and fetch logs
			logsView := m.views[LogsView].(*LogsViewImpl)
			m = *logsView.updateLogFilter(&m)
			return &m, m.fetchLogs(), true
		}
		return &m, nil, true

	case components.KeyShell:
		if m.currentView != ShellView {
			// Check if we have existing sessions
			if globalShellManager != nil && len(globalShellManager.sessions) > 0 {
				m.showSessionList = true
				m.SwitchToView(ShellView)
			} else {
				// Create new session
				overviewView := m.views[OverviewView].(*OverviewViewImpl)
				newModel, cmd := overviewView.createNewShellSessionWithCmd(&m)
				return newModel, cmd, true
			}
		}
		return &m, nil, true

	case components.KeyOpenBrowser:
		// Open browser with port selection overlay if multiple ports, or directly if only main app
		if len(m.ports) > 0 {
			// Show port selector overlay
			m.showPortSelector = true
			m.selectedPortIndex = 0 // Default to first port
		} else if m.appHealthy {
			// No other ports, open main app directly
			overviewView := m.views[OverviewView].(*OverviewViewImpl)
			go func() {
				_ = overviewView.openBrowser("http://localhost:8080")
			}()
		} else {
			// App is not ready, show bold feedback
			m.bootingBold = true
			m.bootingBoldTimer = time.Now()
		}
		return &m, nil, true

	case components.KeyWorkspace:
		// Show workspace selector overlay if we have workspaces
		if len(m.workspaces) > 0 {
			m.showWorkspaceSelector = true
			m.selectedWorkspaceIndex = 0 // Default to first workspace
		} else {
			// Set flag to show selector when workspaces load and fetch workspaces from API
			m.waitingToShowWorkspaces = true
			return &m, m.fetchWorkspaces(), true
		}
		return &m, nil, true
	}

	// Handle port selector overlay if active
	if m.showPortSelector {
		return m.handlePortSelectorKeys(msg)
	}

	// Handle workspace selector overlay if active
	if m.showWorkspaceSelector {
		return m.handleWorkspaceSelectorKeys(msg)
	}

	// Key not handled globally
	return &m, nil, false
}

// Spinner tick handler
func (m Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if m.currentView == ShellView && m.shellConnecting {
		var cmd tea.Cmd
		m.shellSpinner, cmd = m.shellSpinner.Update(msg)
		return m, cmd
	}

	// Handle initialization view spinner
	if m.currentView == InitializationView {
		// Update the spinner in the initialization view
		updatedModel, cmd := m.GetCurrentView().Update(&m, msg)
		return *updatedModel, cmd
	}

	return m, nil
}

// Periodic tick handler
func (m Model) handleTick(msg tickMsg) (tea.Model, tea.Cmd) {
	m.lastUpdate = time.Time(msg)

	// If quit was requested, stop scheduling new commands
	if m.quitRequested {
		debugLog("handleTick: quit requested, stopping background commands")
		return m, nil
	}

	// Build batch of commands based on connection state
	cmds := []tea.Cmd{tick(), m.fetchContainerInfo()}

	// Only fetch health status if SSE is not connected
	// Once SSE is connected, we use that as our health indicator
	if !m.sseConnected {
		cmds = append(cmds, m.fetchHealthStatus())
	}

	// Fetch workspaces periodically (every 5 ticks = 25 seconds)
	// This is a fallback in case SSE events are missed
	if int(m.lastUpdate.Unix())%25 == 0 {
		cmds = append(cmds, m.fetchWorkspaces())
	}

	return m, tea.Batch(cmds...)
}

// Animation tick handler
func (m Model) handleAnimationTick(msg animationTickMsg) (tea.Model, tea.Cmd) {
	// If quit was requested, stop scheduling new commands
	if m.quitRequested {
		debugLog("handleAnimationTick: quit requested, stopping animation")
		return m, nil
	}

	// Update animation state
	m.bootingAnimDots = (m.bootingAnimDots + 1) % 4

	// Check if we need to turn off bold
	if m.bootingBold && time.Since(m.bootingBoldTimer) > 3*time.Second {
		m.bootingBold = false
	}

	return m, animationTick()
}

// Logs tick handler
func (m Model) handleLogsTick(msg logsTickMsg) (tea.Model, tea.Cmd) {
	// If quit was requested, stop scheduling new commands
	if m.quitRequested {
		debugLog("handleLogsTick: quit requested, stopping logs tick")
		return m, nil
	}

	// Auto-refresh logs only when in logs view
	switch m.currentView {
	case LogsView:
		return m, tea.Batch(
			logsTick(),
			m.fetchLogs(),
		)
	case ShellView:
		// Schedule next tick for cursor blinking
		return m, tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
			return logsTickMsg(t)
		})
	default:
		// If not in logs or shell view, just schedule next tick
		return m, logsTick()
	}
}

// Data message handlers
func (m Model) handleContainerInfo(msg containerInfoMsg) (tea.Model, tea.Cmd) {
	// Merge new info with existing, preserving stats if not present in new info
	newInfo := map[string]interface{}(msg)

	// Check if new stats are valid (present and non-empty)
	newStats, hasNewStats := newInfo["stats"]
	newStatsStr, isString := newStats.(string)
	hasValidNewStats := hasNewStats && isString && strings.TrimSpace(newStatsStr) != ""

	// If the new info doesn't have valid stats but we have previous stats, keep them
	if !hasValidNewStats {
		if oldStats, hasOldStats := m.containerInfo["stats"]; hasOldStats {
			if oldStatsStr, ok := oldStats.(string); ok && strings.TrimSpace(oldStatsStr) != "" {
				newInfo["stats"] = oldStats
			}
		}
	}

	m.containerInfo = newInfo

	return m, nil
}

func (m Model) handleRepositoryInfo(msg repositoryInfoMsg) (tea.Model, tea.Cmd) {
	m.repositoryInfo = map[string]interface{}(msg)
	return m, nil
}

func (m Model) handleContainerRepos(msg containerReposMsg) (tea.Model, tea.Cmd) {
	m.containerRepos = map[string]interface{}(msg)
	return m, nil
}

func (m Model) handleLogs(msg logsMsg) (tea.Model, tea.Cmd) {
	newLogs := []string(msg)

	// Check if this is new logs or a full refresh
	if len(newLogs) > m.lastLogCount {
		// We have new logs to stream
		switch m.currentView {
		case LogsView:
			logsView := m.views[LogsView].(*LogsViewImpl)
			m = *logsView.streamNewLogs(&m, newLogs)
		}
	} else if len(newLogs) < m.lastLogCount || m.lastLogCount == 0 {
		// Full refresh (manual refresh or first load)
		m.logs = newLogs
		switch m.currentView {
		case LogsView:
			logsView := m.views[LogsView].(*LogsViewImpl)
			m = *logsView.updateLogFilter(&m)
		}
	}

	m.lastLogCount = len(newLogs)
	return m, nil
}

func (m Model) handlePorts(msg portsMsg) (tea.Model, tea.Cmd) {
	// Convert string ports to PortInfo
	m.ports = []PortInfo{}
	for _, port := range msg {
		m.ports = append(m.ports, PortInfo{
			Port:  port,
			Title: fmt.Sprintf("Port %s", port),
		})
	}
	return m, nil
}

func (m Model) handleHealthStatus(msg healthStatusMsg) (tea.Model, tea.Cmd) {
	wasHealthy := m.appHealthy
	m.appHealthy = bool(msg)
	debugLog("handleHealthStatus: wasHealthy=%v, appHealthy=%v, sseStarted=%v", wasHealthy, m.appHealthy, m.sseStarted)

	// Start SSE client when app becomes healthy for the first time
	if m.appHealthy && !wasHealthy && !m.sseStarted && m.sseClient != nil {
		m.sseClient.Start()
		m.sseStarted = true
		debugLog("Started SSE client after health check passed")
	}

	// Auto-open browser when app becomes healthy for the first time
	if m.appHealthy && !wasHealthy && !m.browserOpened {
		m.browserOpened = true
		overviewView := m.views[OverviewView].(*OverviewViewImpl)
		if err := overviewView.openBrowser("http://localhost:8080"); err != nil {
			debugLog("Failed to open browser: %v", err)
		} else {
			debugLog("Automatically opened browser at http://localhost:8080")
		}
	}

	return m, nil
}

func (m Model) handleError(msg errMsg) (tea.Model, tea.Cmd) {
	m.err = error(msg)
	return m, nil
}

// SSE event handlers
func (m Model) handleSSEConnected(msg sseConnectedMsg) (tea.Model, tea.Cmd) {
	wasHealthy := m.appHealthy
	m.sseConnected = true
	m.appHealthy = true // SSE connection indicates app is healthy
	debugLog("SSE connected")

	// Auto-open browser when SSE connects and app becomes healthy for the first time
	if m.appHealthy && !wasHealthy && !m.browserOpened {
		m.browserOpened = true
		overviewView := m.views[OverviewView].(*OverviewViewImpl)
		if err := overviewView.openBrowser("http://localhost:8080"); err != nil {
			debugLog("Failed to open browser: %v", err)
		} else {
			debugLog("Automatically opened browser at http://localhost:8080")
		}
	}

	return m, nil
}

func (m Model) handleSSEDisconnected(msg sseDisconnectedMsg) (tea.Model, tea.Cmd) {
	m.sseConnected = false
	debugLog("SSE disconnected")
	// Fall back to polling when disconnected
	return m, tea.Batch(m.fetchPorts(), m.fetchHealthStatus())
}

func (m Model) handleSSEPortOpened(msg ssePortOpenedMsg) (tea.Model, tea.Cmd) {
	// Add port to our list
	portStr := fmt.Sprintf("%d", msg.port)
	found := false
	for _, p := range m.ports {
		if p.Port == portStr {
			found = true
			break
		}
	}
	if !found {
		// Use title if available, otherwise default format
		title := msg.title
		if title == "" {
			title = fmt.Sprintf("Port %d", msg.port)
		}
		m.ports = append(m.ports, PortInfo{
			Port:     portStr,
			Title:    title,
			Service:  msg.service,
			Protocol: msg.protocol,
		})
		debugLog("SSE: Port opened: %d (title: %s)", msg.port, title)
	}
	return m, nil
}

func (m Model) handleSSEPortClosed(msg ssePortClosedMsg) (tea.Model, tea.Cmd) {
	// Remove port from our list
	portStr := fmt.Sprintf("%d", msg.port)
	newPorts := []PortInfo{}
	for _, p := range m.ports {
		if p.Port != portStr {
			newPorts = append(newPorts, p)
		}
	}
	m.ports = newPorts
	debugLog("SSE: Port closed: %d", msg.port)
	return m, nil
}

func (m Model) handleSSEContainerStatus(msg sseContainerStatusMsg) (tea.Model, tea.Cmd) {
	// Update container status if needed
	debugLog("SSE: Container status: %s", msg.status)
	return m, nil
}

func (m Model) handleSSEError(msg sseErrorMsg) (tea.Model, tea.Cmd) {
	debugLog("SSE error: %v", msg.err)
	// Fall back to polling on error
	return m, m.fetchPorts()
}

// Shell message handlers
func (m Model) handleShellOutput(msg shellOutputMsg) (tea.Model, tea.Cmd) {
	switch m.currentView {
	case ShellView:
		shellView := m.views[ShellView].(*ShellViewImpl)
		newModel, cmd := shellView.handleShellOutput(&m, msg)
		return *newModel, cmd
	case WorkspaceView:
		workspaceView := m.views[WorkspaceView].(*WorkspaceViewImpl)
		newModel, cmd := workspaceView.Update(&m, msg)
		return *newModel, cmd
	default:
		return m, nil
	}
}

func (m Model) handleShellError(msg shellErrorMsg) (tea.Model, tea.Cmd) {
	switch m.currentView {
	case ShellView:
		shellView := m.views[ShellView].(*ShellViewImpl)
		newModel, cmd := shellView.handleShellError(&m, msg)
		return *newModel, cmd
	case WorkspaceView:
		workspaceView := m.views[WorkspaceView].(*WorkspaceViewImpl)
		newModel, cmd := workspaceView.Update(&m, msg)
		return *newModel, cmd
	default:
		return m, nil
	}
}

// handlePortSelectorKeys handles key input for the port selector overlay
func (m Model) handlePortSelectorKeys(msg tea.KeyMsg) (*Model, tea.Cmd, bool) {
	keyStr := msg.String()

	switch keyStr {
	case components.KeyEscape:
		// Close port selector
		m.showPortSelector = false
		return &m, nil, true

	case components.KeyEnter:
		// Open selected port
		var url string
		if m.selectedPortIndex == 0 {
			// Main app selected (first item)
			url = "http://localhost:8080"
		} else {
			// Find the corresponding port (skip index 0 which is main app)
			portIndex := 0
			for _, port := range m.ports {
				if port.Port != "8080" {
					portIndex++
					if portIndex == m.selectedPortIndex {
						url = fmt.Sprintf("http://localhost:8080/%s", port.Port)
						break
					}
				}
			}
		}

		if url != "" {
			overviewView := m.views[OverviewView].(*OverviewViewImpl)
			go func() {
				if overviewView.isAppReady("http://localhost:8080") {
					_ = overviewView.openBrowser(url)
				}
			}()
		}
		m.showPortSelector = false
		return &m, nil, true

	case components.KeyUp, "k":
		// Move up in port list
		// Calculate total items: 1 (main app) + filtered ports
		totalItems := 1 // main app
		for _, port := range m.ports {
			if port.Port != "8080" {
				totalItems++
			}
		}

		if m.selectedPortIndex > 0 {
			m.selectedPortIndex--
		} else {
			m.selectedPortIndex = totalItems - 1 // Wrap to bottom
		}
		return &m, nil, true

	case components.KeyDown, "j":
		// Move down in port list
		// Calculate total items: 1 (main app) + filtered ports
		totalItems := 1 // main app
		for _, port := range m.ports {
			if port.Port != "8080" {
				totalItems++
			}
		}

		if m.selectedPortIndex < totalItems-1 {
			m.selectedPortIndex++
		} else {
			m.selectedPortIndex = 0 // Wrap to top
		}
		return &m, nil, true

	default:
		// Check for number keys 1-9 for direct selection
		if len(keyStr) == 1 && keyStr >= "1" && keyStr <= "9" {
			index := int(keyStr[0] - '1') // Convert to 0-based index

			// Calculate total items: 1 (main app) + filtered ports
			totalItems := 1 // main app
			for _, port := range m.ports {
				if port.Port != "8080" {
					totalItems++
				}
			}

			if index < totalItems {
				var url string
				if index == 0 {
					// Main app selected (first item)
					url = "http://localhost:8080"
				} else {
					// Find the corresponding port (skip index 0 which is main app)
					portIndex := 0
					for _, port := range m.ports {
						if port.Port != "8080" {
							portIndex++
							if portIndex == index {
								url = fmt.Sprintf("http://localhost:8080/%s", port.Port)
								break
							}
						}
					}
				}

				if url != "" {
					overviewView := m.views[OverviewView].(*OverviewViewImpl)
					go func() {
						if overviewView.isAppReady("http://localhost:8080") {
							_ = overviewView.openBrowser(url)
						}
					}()
				}
				m.showPortSelector = false
			}
			return &m, nil, true
		}
	}

	return &m, nil, true
}

// handleWorkspaceSelectorKeys handles key input for the workspace selector overlay
func (m Model) handleWorkspaceSelectorKeys(msg tea.KeyMsg) (*Model, tea.Cmd, bool) {
	keyStr := msg.String()

	switch keyStr {
	case components.KeyEscape:
		// Close workspace selector
		m.showWorkspaceSelector = false
		return &m, nil, true

	case components.KeyEnter:
		// Select workspace and switch to workspace view
		if m.selectedWorkspaceIndex < len(m.workspaces) {
			workspace := &m.workspaces[m.selectedWorkspaceIndex]
			m.currentWorkspace = workspace
			m.SwitchToView(WorkspaceView)

			// Create workspace terminal sessions
			workspaceView := m.views[WorkspaceView].(*WorkspaceViewImpl)
			newModel, cmd := workspaceView.CreateWorkspaceSessions(&m, workspace)
			m.showWorkspaceSelector = false
			return newModel, cmd, true
		}
		m.showWorkspaceSelector = false
		return &m, nil, true

	case components.KeyUp, "k":
		// Move up in workspace list
		if m.selectedWorkspaceIndex > 0 {
			m.selectedWorkspaceIndex--
		} else {
			m.selectedWorkspaceIndex = len(m.workspaces) - 1 // Wrap to bottom
		}
		return &m, nil, true

	case components.KeyDown, "j":
		// Move down in workspace list
		if m.selectedWorkspaceIndex < len(m.workspaces)-1 {
			m.selectedWorkspaceIndex++
		} else {
			m.selectedWorkspaceIndex = 0 // Wrap to top
		}
		return &m, nil, true

	default:
		// Check for number keys 1-9 for direct selection
		if len(keyStr) == 1 && keyStr >= "1" && keyStr <= "9" {
			index := int(keyStr[0] - '1') // Convert to 0-based index
			if index < len(m.workspaces) {
				workspace := &m.workspaces[index]
				m.currentWorkspace = workspace
				m.SwitchToView(WorkspaceView)

				// Create workspace terminal sessions
				workspaceView := m.views[WorkspaceView].(*WorkspaceViewImpl)
				newModel, cmd := workspaceView.CreateWorkspaceSessions(&m, workspace)
				m.showWorkspaceSelector = false
				return newModel, cmd, true
			}
		}
	}

	return &m, nil, true
}

// initializeMockWorkspaces creates mock workspace data for development
func (m Model) initializeMockWorkspaces() []WorkspaceInfo {
	// TODO: Replace this with actual API call to fetch workspaces
	return []WorkspaceInfo{
		{
			ID:       "workspace-1",
			Name:     "catnip-main",
			Path:     "/workspace/catnip",
			Branch:   "main",
			IsActive: true,
			ChangedFiles: []string{
				"container/internal/tui/view_workspace.go",
				"container/internal/tui/model.go",
				"src/components/WorkspaceRightSidebar.tsx",
			},
			Ports: []PortInfo{
				{Port: "3000", Title: "React Dev Server", Service: "vite"},
				{Port: "8080", Title: "Main API", Service: "go-api"},
			},
		},
		{
			ID:       "workspace-2",
			Name:     "feature-branch",
			Path:     "/workspace/catnip-feature",
			Branch:   "feature/workspace-ui",
			IsActive: false,
			ChangedFiles: []string{
				"frontend/src/App.tsx",
				"README.md",
			},
			Ports: []PortInfo{
				{Port: "3001", Title: "Test Server", Service: "node"},
			},
		},
		{
			ID:           "workspace-3",
			Name:         "tom-repo",
			Path:         "/workspace/tom",
			Branch:       "main",
			IsActive:     false,
			ChangedFiles: []string{},
			Ports:        []PortInfo{},
		},
	}
}

// Version check handler
func (m Model) handleVersionCheck(msg VersionCheckMsg) (tea.Model, tea.Cmd) {
	m.upgradeAvailable = msg.UpgradeAvailable
	if msg.UpgradeAvailable {
		debugLog("Version mismatch detected: CLI=%s, Container=%s", msg.CLIVersion, msg.ContainerVersion)
	} else {
		debugLog("Versions match: CLI=%s, Container=%s", msg.CLIVersion, msg.ContainerVersion)
	}
	return m, nil
}

// Workspaces message handler
func (m Model) handleWorkspaces(msg workspacesMsg) (tea.Model, tea.Cmd) {
	m.workspaces = []WorkspaceInfo(msg)
	debugLog("Updated workspaces: %d workspaces loaded", len(m.workspaces))

	// If we were waiting to show workspaces and now have some, show the selector
	if len(m.workspaces) > 0 && m.waitingToShowWorkspaces {
		m.waitingToShowWorkspaces = false
		m.showWorkspaceSelector = true
		m.selectedWorkspaceIndex = 0
	}

	return m, nil
}

// SSE worktree updated handler
func (m Model) handleSSEWorktreeUpdated(msg sseWorktreeUpdatedMsg) (tea.Model, tea.Cmd) {
	debugLog("SSE worktree updated event received, refreshing workspaces")
	// Refresh workspaces when SSE event is received
	return m, m.fetchWorkspaces()
}
