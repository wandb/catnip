package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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

	return m, nil
}

func (m Model) handleError(msg errMsg) (tea.Model, tea.Cmd) {
	m.err = error(msg)
	return m, nil
}

// SSE event handlers
func (m Model) handleSSEConnected(msg sseConnectedMsg) (tea.Model, tea.Cmd) {
	m.sseConnected = true
	m.appHealthy = true // SSE connection indicates app is healthy
	debugLog("SSE connected")
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
	if m.currentView == ShellView {
		shellView := m.views[ShellView].(*ShellViewImpl)
		newModel, cmd := shellView.handleShellOutput(&m, msg)
		return *newModel, cmd
	}
	return m, nil
}

func (m Model) handleShellError(msg shellErrorMsg) (tea.Model, tea.Cmd) {
	if m.currentView == ShellView {
		shellView := m.views[ShellView].(*ShellViewImpl)
		newModel, cmd := shellView.handleShellError(&m, msg)
		return *newModel, cmd
	}
	return m, nil
}
