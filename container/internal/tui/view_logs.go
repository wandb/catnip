package tui

import (
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vanpelt/catnip/internal/tui/components"
)

// LogsViewImpl handles the logs view functionality
type LogsViewImpl struct{}

// NewLogsView creates a new logs view instance
func NewLogsView() *LogsViewImpl {
	return &LogsViewImpl{}
}

// GetViewType returns the view type identifier
func (v *LogsViewImpl) GetViewType() ViewType {
	return LogsView
}

// Update handles logs-specific message processing
func (v *LogsViewImpl) Update(m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	var cmd tea.Cmd

	// Update search input if in search mode
	if m.searchMode {
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	// Update viewport
	m.logsViewport, cmd = m.logsViewport.Update(msg)
	return m, cmd
}

// HandleKey processes key messages for the logs view
// Note: Global navigation keys (Ctrl+O, Ctrl+L, Ctrl+Q, etc.) are handled in the global handler
func (v *LogsViewImpl) HandleKey(m *Model, msg tea.KeyMsg) (*Model, tea.Cmd) {
	// Handle search mode keys first
	if m.searchMode {
		switch msg.String() {
		case components.KeyEscape:
			m.searchMode = false
			m.searchInput.Blur()
			return m, nil
		case components.KeyEnter:
			m.searchMode = false
			m.searchInput.Blur()
			m.searchPattern = m.searchInput.Value()
			m = v.updateLogFilter(m)
			return m, nil
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}
	}

	// Handle view-specific logs navigation keys
	switch msg.String() {
	case components.KeyLogsSearch:
		m.searchMode = true
		cmd := m.searchInput.Focus()
		return m, cmd

	case components.KeyLogsClear:
		m.searchPattern = ""
		m.searchInput.SetValue("")
		m = v.updateLogFilter(m)
		return m, nil

	case components.KeyUp, components.KeyVimUp:
		m.logsViewport.ScrollUp(1)
		return m, nil

	case components.KeyDown, components.KeyVimDown:
		m.logsViewport.ScrollDown(1)
		return m, nil

	case components.KeyPageUp, components.KeyVimPageUp:
		m.logsViewport.PageUp()
		return m, nil

	case components.KeyPageDown, components.KeyVimPageDown:
		m.logsViewport.PageDown()
		return m, nil

	case components.KeyHome, components.KeyVimTop:
		m.logsViewport.GotoTop()
		return m, nil

	case components.KeyEnd, components.KeyVimBottom:
		m.logsViewport.GotoBottom()
		return m, nil
	}

	return m, nil
}

// HandleResize processes window resize for the logs view
func (v *LogsViewImpl) HandleResize(m *Model, msg tea.WindowSizeMsg) (*Model, tea.Cmd) {
	headerHeight := 4 // Height for header and search bar
	m.logsViewport.Width = msg.Width - 4
	m.logsViewport.Height = msg.Height - headerHeight
	m.searchInput.Width = msg.Width - 20
	return m, nil
}

// Render generates the logs view content
func (v *LogsViewImpl) Render(m *Model) string {
	var sections []string

	// Header with log count info
	headerText := "📄 Container Logs"
	if m.searchPattern != "" {
		headerText += " (filtered)"
	}

	sections = append(sections, components.SectionHeaderStyle.Render(headerText))

	// Search info/help (only when not in search mode)
	if !m.searchMode {
		if m.searchPattern != "" {
			searchInfo := "Filter: " + m.searchPattern + " (press 'c' to clear, '/' to search)"
			sections = append(sections, components.MutedStyle.Render(searchInfo))
		} else {
			helpText := "Press '/' to search, ↑↓/jk to scroll, PgUp/PgDn or b/f for pages, g/G for top/bottom"
			sections = append(sections, components.MutedStyle.Render(helpText))
		}
	}

	sections = append(sections, "")

	// Main content area with viewport
	if len(m.logs) == 0 {
		sections = append(sections, "No logs available")
		return strings.Join(sections, "\n")
	}

	// Return header + viewport content
	header := strings.Join(sections, "\n")

	// Viewport shows the scrollable content
	currentOffset := m.logsViewport.YOffset
	viewportContent := m.logsViewport.View()
	afterOffset := m.logsViewport.YOffset

	if currentOffset != afterOffset {
		debugLog("renderLogs: viewport offset changed during View()! before=%d, after=%d", currentOffset, afterOffset)
	}

	return header + "\n" + viewportContent
}

// updateLogFilter applies the current search pattern to logs and updates the viewport
func (v *LogsViewImpl) updateLogFilter(m *Model) *Model {
	// Check if we should preserve scroll position
	preserveScroll := m.logsViewport.YOffset > 0 && !m.logsViewport.AtBottom()
	currentY := m.logsViewport.YOffset

	if m.searchPattern == "" {
		m.filteredLogs = m.logs
		m.compiledRegex = nil
	} else {
		// Try to compile regex pattern
		if regex, err := regexp.Compile("(?i)" + m.searchPattern); err == nil {
			m.compiledRegex = regex
			m.filteredLogs = []string{}
			for _, line := range m.logs {
				if regex.MatchString(line) {
					// Highlight matches in the line
					highlighted := regex.ReplaceAllStringFunc(line, func(match string) string {
						return components.SearchHighlightStyle.Render(match)
					})
					m.filteredLogs = append(m.filteredLogs, highlighted)
				}
			}
		} else {
			// Fall back to simple string contains search if regex is invalid
			m.compiledRegex = nil
			m.filteredLogs = []string{}
			searchLower := strings.ToLower(m.searchPattern)
			for _, line := range m.logs {
				if strings.Contains(strings.ToLower(line), searchLower) {
					// Simple highlighting for non-regex search
					highlighted := strings.ReplaceAll(line, m.searchPattern,
						components.SearchHighlightStyle.Render(m.searchPattern))
					m.filteredLogs = append(m.filteredLogs, highlighted)
				}
			}
		}
	}

	// Update viewport content
	m.logsViewport.SetContent(strings.Join(m.filteredLogs, "\n"))

	// Only scroll to bottom if this is initial load or user was already at bottom
	if preserveScroll {
		debugLog("updateLogFilter: preserving scroll position at Y=%d", currentY)
		m.logsViewport.SetYOffset(currentY)
	} else {
		debugLog("updateLogFilter: calling GotoBottom() (was at bottom or Y=0)")
		m.logsViewport.GotoBottom()
	}
	return m
}

// streamNewLogs handles streaming new log entries with filtering
func (v *LogsViewImpl) streamNewLogs(m *Model, newLogs []string) *Model {
	// Get only the new entries
	newEntries := newLogs[m.lastLogCount:]

	// Update the complete logs
	m.logs = newLogs

	// If no filter is active, update logs but preserve scroll position
	if m.searchPattern == "" {
		// Calculate if we're at the bottom more precisely
		currentY := m.logsViewport.YOffset
		totalLines := m.logsViewport.TotalLineCount()
		viewHeight := m.logsViewport.Height

		// Check if viewport thinks it's at bottom
		viewportAtBottom := m.logsViewport.AtBottom()

		// Consider "at bottom" if we can see the last line (with 2 line tolerance for edge cases)
		atBottomThreshold := totalLines - viewHeight - 2
		calculatedAtBottom := currentY >= atBottomThreshold && totalLines > viewHeight

		debugLog("streamNewLogs: currentY=%d, totalLines=%d, viewHeight=%d, threshold=%d, calculatedAtBottom=%v, viewportAtBottom=%v, newEntries=%d",
			currentY, totalLines, viewHeight, atBottomThreshold, calculatedAtBottom, viewportAtBottom, len(newEntries))

		// Update the filtered logs
		m.filteredLogs = m.logs

		// Update viewport content
		m.logsViewport.SetContent(strings.Join(m.filteredLogs, "\n"))

		// Decide whether to scroll or preserve position
		if viewportAtBottom || calculatedAtBottom {
			debugLog("streamNewLogs: was at bottom, calling GotoBottom()")
			m.logsViewport.GotoBottom()
		} else {
			// User has scrolled up - preserve their position
			debugLog("streamNewLogs: NOT at bottom, setting position back to Y=%d", currentY)
			m.logsViewport.SetYOffset(currentY)

			// Log what actually happened
			actualY := m.logsViewport.YOffset
			debugLog("streamNewLogs: After SetYOffset call - wanted Y=%d, got Y=%d", currentY, actualY)
		}

		return m
	}

	// Filter is active - preserve viewport position and only filter new entries
	currentY := m.logsViewport.YOffset
	totalLines := m.logsViewport.TotalLineCount()
	viewHeight := m.logsViewport.Height

	// Consider "at bottom" if we can see the last line (with 2 line tolerance for edge cases)
	wasAtBottom := currentY >= (totalLines - viewHeight - 2)

	debugLog("streamNewLogs (filtered): currentY=%d, totalLines=%d, viewHeight=%d, wasAtBottom=%v, newEntries=%d",
		currentY, totalLines, viewHeight, wasAtBottom, len(newEntries))

	// Filter new entries
	var newFilteredEntries []string
	for _, line := range newEntries {
		if m.compiledRegex != nil {
			if m.compiledRegex.MatchString(line) {
				highlighted := m.compiledRegex.ReplaceAllStringFunc(line, func(match string) string {
					return components.SearchHighlightStyle.Render(match)
				})
				newFilteredEntries = append(newFilteredEntries, highlighted)
			}
		} else {
			// Simple string search fallback
			searchLower := strings.ToLower(m.searchPattern)
			if strings.Contains(strings.ToLower(line), searchLower) {
				highlighted := strings.ReplaceAll(line, m.searchPattern,
					components.SearchHighlightStyle.Render(m.searchPattern))
				newFilteredEntries = append(newFilteredEntries, highlighted)
			}
		}
	}

	// Append new filtered entries to existing filtered logs
	m.filteredLogs = append(m.filteredLogs, newFilteredEntries...)

	// Update viewport content
	m.logsViewport.SetContent(strings.Join(m.filteredLogs, "\n"))

	newTotalLines := m.logsViewport.TotalLineCount()
	debugLog("streamNewLogs (filtered): after SetContent - newTotalLines=%d, YOffset=%d",
		newTotalLines, m.logsViewport.YOffset)

	// Only auto-scroll if user was already at the bottom
	if wasAtBottom {
		debugLog("streamNewLogs (filtered): was at bottom, calling GotoBottom()")
		m.logsViewport.GotoBottom()
	} else {
		// Preserve the Y offset
		debugLog("streamNewLogs (filtered): was NOT at bottom, preserving position at Y=%d", currentY)
		// SetContent seems to reset the viewport, so directly set the offset
		m.logsViewport.SetYOffset(currentY)

		// Force a viewport update to ensure the view matches the offset
		m.logsViewport, _ = m.logsViewport.Update(nil)

		// Verify position was set correctly
		actualY := m.logsViewport.YOffset
		debugLog("streamNewLogs (filtered): After SetYOffset - wanted Y=%d, actual Y=%d", currentY, actualY)
	}

	return m
}
