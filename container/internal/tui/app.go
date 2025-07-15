package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/vanpelt/catnip/internal/services"
)

type view int

const (
	overviewView view = iota
	logsView
)

type App struct {
	containerService *services.ContainerService
	containerName    string
	program          *tea.Program
}

type model struct {
	containerService *services.ContainerService
	containerName    string
	workDir          string
	currentView      view
	containerInfo    map[string]interface{}
	repositoryInfo   map[string]interface{}
	containerRepos   map[string]interface{}
	logs             []string
	filteredLogs     []string
	ports            []string
	logo             []string
	err              error
	width            int
	height           int
	lastUpdate       time.Time
	
	// Enhanced logs view
	logsViewport     viewport.Model
	searchInput      textinput.Model
	searchMode       bool
	searchPattern    string
	compiledRegex    *regexp.Regexp
}

type tickMsg time.Time
type containerInfoMsg map[string]interface{}
type repositoryInfoMsg map[string]interface{}
type containerReposMsg map[string]interface{}
type logsMsg []string
type portsMsg []string
type errMsg error
type quitMsg struct{}

var debugLogger *log.Logger
var debugEnabled bool

func init() {
	debugEnabled = os.Getenv("DEBUG") == "true"
	if debugEnabled {
		logFile, err := os.OpenFile("/tmp/catctrl-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalln("Failed to open debug log file:", err)
		}
		debugLogger = log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)
		debugLogger.Println("=== TUI DEBUG LOG STARTED ===")
	}
}

func debugLog(format string, args ...interface{}) {
	if debugEnabled && debugLogger != nil {
		debugLogger.Printf(format, args...)
	}
}

func NewApp(containerService *services.ContainerService, containerName, workDir string) *App {
	return &App{
		containerService: containerService,
		containerName:    containerName,
	}
}

func (a *App) Run(ctx context.Context, workDir string) error {
	start := time.Now()
	debugLog("TUI Run() starting - workDir: %s", workDir)
	
	// Initialize search input
	debugLog("TUI Run() initializing search input - elapsed: %v", time.Since(start))
	searchInput := textinput.New()
	searchInput.Placeholder = "Enter search pattern (regex supported)..."
	searchInput.CharLimit = 100
	searchInput.Width = 50
	searchInput.Prompt = "🔍 "
	searchInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	searchInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	searchInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	
	// Initialize viewport (will be sized in Update)
	debugLog("TUI Run() initializing viewport - elapsed: %v", time.Since(start))
	logsViewport := viewport.New(80, 20)
	
	debugLog("TUI Run() loading logo - elapsed: %v", time.Since(start))
	logo := loadLogo()
	debugLog("TUI Run() logo loaded - elapsed: %v", time.Since(start))
	
	m := model{
		containerService: a.containerService,
		containerName:    a.containerName,
		workDir:          workDir,
		currentView:      overviewView,
		containerInfo:    make(map[string]interface{}),
		repositoryInfo:   make(map[string]interface{}),
		containerRepos:   make(map[string]interface{}),
		logs:             []string{},
		filteredLogs:     []string{},
		ports:            []string{},
		logo:             logo,
		lastUpdate:       time.Now(),
		logsViewport:     logsViewport,
		searchInput:      searchInput,
		searchMode:       false,
		searchPattern:    "",
	}

	debugLog("TUI Run() creating tea program - elapsed: %v", time.Since(start))
	a.program = tea.NewProgram(m, tea.WithAltScreen())

	debugLog("TUI Run() starting tea program - elapsed: %v", time.Since(start))
	_, err := a.program.Run()
	debugLog("TUI Run() tea program finished - elapsed: %v, err: %v", time.Since(start), err)
	return err
}

func (m model) Init() tea.Cmd {
	start := time.Now()
	debugLog("TUI Init() starting")
	
	// Start background data fetching
	result := tea.Batch(
		m.fetchRepositoryInfo(),
		tea.Tick(time.Second*5, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}),
	)
	
	debugLog("TUI Init() finished - elapsed: %v", time.Since(start))
	return result
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	start := time.Now()
	
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		debugLog("TUI Update() WindowSizeMsg: %dx%d", msg.Width, msg.Height)
		m.width = msg.Width
		m.height = msg.Height
		
		// Update viewport size for logs view
		if m.currentView == logsView {
			headerHeight := 4 // Height for header and search bar
			m.logsViewport.Width = msg.Width - 4
			m.logsViewport.Height = msg.Height - headerHeight
			m.searchInput.Width = msg.Width - 20
		}
		
		debugLog("TUI Update() WindowSizeMsg processed - elapsed: %v", time.Since(start))
		return m, nil

	case tea.KeyMsg:
		debugLog("TUI Update() KeyMsg: %s (type: %T, runes: %v)", msg.String(), msg, msg.Runes)
		
		// Handle search mode keys first
		if m.currentView == logsView && m.searchMode {
			switch msg.String() {
			case "esc":
				debugLog("TUI Update() SEARCH MODE ESC")
				m.searchMode = false
				m.searchInput.Blur()
				return m, nil
			case "enter":
				debugLog("TUI Update() SEARCH MODE ENTER")
				m.searchMode = false
				m.searchInput.Blur()
				m.searchPattern = m.searchInput.Value()
				m = m.updateLogFilter()
				return m, nil
			default:
				debugLog("TUI Update() SEARCH MODE INPUT")
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
		}
		
		// Handle logs view navigation
		if m.currentView == logsView && !m.searchMode {
			switch msg.String() {
			case "/":
				debugLog("TUI Update() LOGS SEARCH KEY")
				m.searchMode = true
				cmd := m.searchInput.Focus()
				return m, cmd
			case "c":
				debugLog("TUI Update() LOGS CLEAR FILTER")
				m.searchPattern = ""
				m.searchInput.SetValue("")
				m = m.updateLogFilter()
				return m, nil
			case "up", "k":
				debugLog("TUI Update() LOGS SCROLL UP")
				m.logsViewport.ScrollUp(1)
				return m, nil
			case "down", "j":
				debugLog("TUI Update() LOGS SCROLL DOWN")
				m.logsViewport.ScrollDown(1)
				return m, nil
			case "pgup", "b":
				debugLog("TUI Update() LOGS PAGE UP")
				m.logsViewport.PageUp()
				return m, nil
			case "pgdown", "f":
				debugLog("TUI Update() LOGS PAGE DOWN")
				m.logsViewport.PageDown()
				return m, nil
			case "home", "g":
				debugLog("TUI Update() LOGS GOTO TOP")
				m.logsViewport.GotoTop()
				return m, nil
			case "end", "G":
				debugLog("TUI Update() LOGS GOTO BOTTOM")
				m.logsViewport.GotoBottom()
				return m, nil
			}
		}
		
		// Global key handlers
		switch msg.String() {
		case "q", "ctrl+c":
			debugLog("TUI Update() QUIT KEY DETECTED: %s", msg.String())
			return m, tea.Quit
		case "l":
			debugLog("TUI Update() LOGS KEY DETECTED")
			if m.currentView == logsView {
				m.currentView = overviewView
				debugLog("TUI Update() switched to overview view")
				return m, nil
			} else {
				m.currentView = logsView
				debugLog("TUI Update() switched to logs view")
				// Update viewport size and content
				if m.height > 0 {
					headerHeight := 4
					m.logsViewport.Width = m.width - 4
					m.logsViewport.Height = m.height - headerHeight
				}
				m = m.updateLogFilter()
				return m, m.fetchLogs()
			}
		case "o":
			debugLog("TUI Update() OVERVIEW KEY DETECTED")
			m.currentView = overviewView
			return m, nil
		case "r":
			debugLog("TUI Update() REFRESH KEY DETECTED")
			if m.currentView == logsView {
				return m, m.fetchLogs()
			}
			return m, tea.Batch(
				m.fetchContainerInfo(),
				m.fetchPorts(),
			)
		case "0":
			debugLog("TUI Update() MAIN UI KEY DETECTED")
			go func() {
				if isAppReady("http://localhost:8080") {
					openBrowser("http://localhost:8080")
				} else {
					// Could show a message to the user, but for now just silently fail
					// The user will see the app isn't ready in the UI
				}
			}()
			return m, nil
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			debugLog("TUI Update() PORT KEY DETECTED: %s", msg.String())
			if m.currentView == overviewView {
				portIndex := int(msg.String()[0] - '1') // Convert '1'-'9' to 0-8
				if portIndex < len(m.ports) {
					port := m.ports[portIndex]
					url := fmt.Sprintf("http://localhost:8080/%s", port)
					debugLog("TUI Update() opening port %s at %s", port, url)
					go func() {
						if isAppReady("http://localhost:8080") {
							openBrowser(url)
						}
					}()
				}
			}
			return m, nil
		}
		
		debugLog("TUI Update() KeyMsg not handled - elapsed: %v", time.Since(start))
		return m, nil

	case tickMsg:
		debugLog("TUI Update() tickMsg - elapsed: %v", time.Since(start))
		m.lastUpdate = time.Time(msg)
		return m, tea.Batch(
			tick(),
			m.fetchContainerInfo(),
			m.fetchPorts(),
		)

	case containerInfoMsg:
		debugLog("TUI Update() containerInfoMsg - elapsed: %v", time.Since(start))
		m.containerInfo = map[string]interface{}(msg)

	case repositoryInfoMsg:
		debugLog("TUI Update() repositoryInfoMsg - elapsed: %v", time.Since(start))
		m.repositoryInfo = map[string]interface{}(msg)

	case containerReposMsg:
		debugLog("TUI Update() containerReposMsg - elapsed: %v", time.Since(start))
		m.containerRepos = map[string]interface{}(msg)

	case logsMsg:
		debugLog("TUI Update() logsMsg - elapsed: %v", time.Since(start))
		m.logs = []string(msg)
		m = m.updateLogFilter()

	case portsMsg:
		debugLog("TUI Update() portsMsg - elapsed: %v", time.Since(start))
		m.ports = []string(msg)

	case errMsg:
		debugLog("TUI Update() errMsg: %v - elapsed: %v", error(msg), time.Since(start))
		m.err = error(msg)
		
	case quitMsg:
		debugLog("TUI Update() quitMsg - elapsed: %v", time.Since(start))
		return m, tea.Quit
	}

	debugLog("TUI Update() finished - elapsed: %v", time.Since(start))
	return m, nil
}


func (m model) View() string {
	start := time.Now()
	debugLog("TUI View() starting - currentView: %d, width: %d", m.currentView, m.width)
	
	if m.width == 0 {
		debugLog("TUI View() returning empty - no width")
		return ""
	}

	var content string
	
	switch m.currentView {
	case overviewView:
		debugLog("TUI View() calling renderOverview() - elapsed: %v", time.Since(start))
		content = m.renderOverview()
		debugLog("TUI View() renderOverview() finished - elapsed: %v", time.Since(start))
	case logsView:
		debugLog("TUI View() calling renderLogs() - elapsed: %v", time.Since(start))
		content = m.renderLogs()
		debugLog("TUI View() renderLogs() finished - elapsed: %v", time.Since(start))
	}

	// Header
	debugLog("TUI View() creating header style - elapsed: %v", time.Since(start))
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Width(m.width - 2).
		Padding(0, 1)

	debugLog("TUI View() rendering header - elapsed: %v", time.Since(start))
	header := headerStyle.Render(fmt.Sprintf("🐱 Catnip - %s", m.containerName))
	debugLog("TUI View() header rendered - elapsed: %v", time.Since(start))

	// Footer
	debugLog("TUI View() creating footer - elapsed: %v", time.Since(start))
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		Width(m.width - 2).
		Padding(0, 1)

	var footer string
	if m.currentView == overviewView {
		debugLog("TUI View() rendering overview footer - elapsed: %v", time.Since(start))
		footer = footerStyle.Render("Press l for logs, 0 to open UI, 1-9 to open ports, r to refresh, q to quit")
		debugLog("TUI View() overview footer rendered - elapsed: %v", time.Since(start))
	} else {
		if m.searchMode {
			// Replace footer with search input
			searchPrompt := "Search: "
			searchContent := searchPrompt + m.searchInput.View() + " (Enter to apply, Esc to cancel)"
			footer = footerStyle.Render(searchContent)
		} else {
			footer = footerStyle.Render("/ search, c clear filter, ↑↓ scroll, o overview, r refresh, q quit")
		}
	}

	// Main content area
	debugLog("TUI View() creating main content area - elapsed: %v", time.Since(start))
	mainHeight := m.height - 4 // Account for header and footer
	mainStyle := lipgloss.NewStyle().
		Width(m.width - 2).
		Height(mainHeight).
		Padding(1)

	debugLog("TUI View() rendering main content - elapsed: %v", time.Since(start))
	mainContent := mainStyle.Render(content)
	debugLog("TUI View() main content rendered - elapsed: %v", time.Since(start))

	debugLog("TUI View() joining vertical layout - elapsed: %v", time.Since(start))
	result := lipgloss.JoinVertical(lipgloss.Left, header, mainContent, footer)
	debugLog("TUI View() finished - elapsed: %v", time.Since(start))
	return result
}

func (m model) renderOverview() string {
	start := time.Now()
	debugLog("renderOverview() starting")
	
	// Check if we have enough width for the logo (70 cols + 70 for content = 140+ total)
	showLogo := m.width >= 150 && len(m.logo) > 0
	debugLog("renderOverview() showLogo: %t - elapsed: %v", showLogo, time.Since(start))
	
	var sections []string

	// Container Status
	statusStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("2"))
	
	sections = append(sections, statusStyle.Render("📦 Container Status"))
	debugLog("renderOverview() added container status header - elapsed: %v", time.Since(start))
	
	if m.containerInfo["name"] != nil {
		sections = append(sections, fmt.Sprintf("  Name: %v", m.containerInfo["name"]))
		sections = append(sections, fmt.Sprintf("  Runtime: %v", m.containerInfo["runtime"]))
		sections = append(sections, fmt.Sprintf("  Last updated: %s", m.lastUpdate.Format("15:04:05")))
		debugLog("renderOverview() added container info - elapsed: %v", time.Since(start))
	} else {
		sections = append(sections, "  Status: Starting...")
		debugLog("renderOverview() added starting status - elapsed: %v", time.Since(start))
	}

	sections = append(sections, "")

	// Main UI section
	uiStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("5"))
	
	keyHighlight := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Bold(true)
	
	sections = append(sections, uiStyle.Render("🖥️  Catnip UI"))
	sections = append(sections, fmt.Sprintf("  %s Main UI → http://localhost:8080", keyHighlight.Render("0.")))
	sections = append(sections, "")

	// Ports
	if len(m.ports) > 0 {
		portsStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("4"))
		
		sections = append(sections, portsStyle.Render("🌐 Detected Services"))
		
		for i, port := range m.ports {
			if i < 9 { // Only show first 9 ports for number shortcuts
				portKey := keyHighlight.Render(fmt.Sprintf("%d.", i+1))
				sections = append(sections, fmt.Sprintf("  %s Port %s → http://localhost:8080/%s", portKey, port, port))
			} else {
				sections = append(sections, fmt.Sprintf("     Port %s → http://localhost:8080/%s", port, port))
			}
		}
	} else {
		sections = append(sections, "🌐 No services detected")
	}

	sections = append(sections, "")

	// Repository Info
	repoStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("3"))
	
	sections = append(sections, repoStyle.Render("📁 Repository Info"))
	
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
	sysStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("5"))
	
	sections = append(sections, sysStyle.Render("📊 System Info"))
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
		errorStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("1"))
		sections = append(sections, errorStyle.Render("❌ Error"))
		sections = append(sections, fmt.Sprintf("  %s", m.err.Error()))
	}

	if !showLogo {
		return strings.Join(sections, "\n")
	}

	// Combine content and logo side by side
	contentText := strings.Join(sections, "\n")
	return m.renderWithLogo(contentText)
}

func (m model) renderLogs() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("3"))
	
	searchStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))
	
	var sections []string
	
	// Header with log count info
	totalLogs := len(m.logs)
	filteredLogs := len(m.filteredLogs)
	headerText := fmt.Sprintf("📄 Container Logs (%d total", totalLogs)
	if m.searchPattern != "" {
		headerText += fmt.Sprintf(", %d filtered", filteredLogs)
	}
	headerText += ")"
	sections = append(sections, headerStyle.Render(headerText))
	
	// Search info/help (only when not in search mode)
	if !m.searchMode {
		if m.searchPattern != "" {
			searchInfo := fmt.Sprintf("Filter: %s (press 'c' to clear, '/' to search)", m.searchPattern)
			sections = append(sections, searchStyle.Render(searchInfo))
		} else {
			helpText := "Press '/' to search, ↑↓/jk to scroll, PgUp/PgDn or b/f for pages, g/G for top/bottom"
			sections = append(sections, searchStyle.Render(helpText))
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
	viewportContent := m.logsViewport.View()
	
	return header + "\n" + viewportContent
}

// Commands
func tick() tea.Cmd {
	return tea.Tick(time.Second*5, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) fetchContainerInfo() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		info, err := m.containerService.GetContainerInfo(ctx, m.containerName)
		if err != nil {
			// Don't show errors for timeout/context cancellation to reduce noise
			return nil
		}
		return containerInfoMsg(info)
	}
}

func (m model) fetchLogs() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		cmd, err := m.containerService.GetContainerLogs(ctx, m.containerName, false)
		if err != nil {
			return nil
		}
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil
		}
		
		lines := strings.Split(string(output), "\n")
		return logsMsg(lines)
	}
}

func (m model) fetchPorts() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		ports, err := m.containerService.GetContainerPorts(ctx, m.containerName)
		if err != nil {
			return nil
		}
		return portsMsg(ports)
	}
}

func (m model) fetchRepositoryInfo() tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		debugLog("fetchRepositoryInfo() starting")
		
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		debugLog("fetchRepositoryInfo() calling GetRepositoryInfo - elapsed: %v", time.Since(start))
		info := m.containerService.GetRepositoryInfo(ctx, m.workDir)
		debugLog("fetchRepositoryInfo() GetRepositoryInfo returned - elapsed: %v", time.Since(start))
		
		return repositoryInfoMsg(info)
	}
}

func (m model) fetchContainerRepos() tea.Cmd {
	return func() tea.Msg {
		// Try to fetch repositories from the container's API
		client := &http.Client{Timeout: 1 * time.Second}
		resp, err := client.Get("http://localhost:8080/v1/git/status")
		if err != nil {
			// If we can't reach the API, return empty repos
			return containerReposMsg(map[string]interface{}{"repositories": []interface{}{}})
		}
		defer resp.Body.Close()
		
		var repoData map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&repoData); err != nil {
			return containerReposMsg(map[string]interface{}{"repositories": []interface{}{}})
		}
		
		return containerReposMsg(repoData)
	}
}

// loadLogo reads the ASCII logo from the public directory
func loadLogo() []string {
	// Try to find the logo file
	possiblePaths := []string{
		"public/logo.ascii",
		"../public/logo.ascii",
		"../../public/logo.ascii",
		"../../../public/logo.ascii",
	}
	
	for _, path := range possiblePaths {
		if content, err := os.ReadFile(path); err == nil {
			lines := strings.Split(string(content), "\n")
			// Remove any trailing empty lines
			for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
				lines = lines[:len(lines)-1]
			}
			return lines
		}
	}
	
	// If we can't find the logo, return empty
	return []string{}
}

// renderWithLogo combines content with the logo side by side
func (m model) renderWithLogo(content string) string {
	contentLines := strings.Split(content, "\n")
	logoLines := m.logo
	
	// Calculate available space
	contentWidth := 70 // Reserve 70 columns for content
	
	// Pad content lines to the specified width
	for i, line := range contentLines {
		// Strip any existing color codes for width calculation
		stripped := stripAnsi(line)
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

// stripAnsi removes ANSI escape sequences for width calculation
func stripAnsi(s string) string {
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

// renderMarkdown renders markdown text using glamour for beautiful help messages
func (m model) renderMarkdown(text string) string {
	// Create a glamour renderer with terminal styling
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(m.width-10), // Leave some padding
	)
	if err != nil {
		// Fallback to plain text if glamour fails
		return text
	}
	
	// Render the markdown
	rendered, err := renderer.Render(text)
	if err != nil {
		// Fallback to plain text if rendering fails
		return text
	}
	
	// Remove trailing newlines that glamour adds
	return strings.TrimRight(rendered, "\n")
}

// updateLogFilter applies the current search pattern to logs and updates the viewport
func (m model) updateLogFilter() model {
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
						return lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0")).Render(match)
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
						lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0")).Render(m.searchPattern))
					m.filteredLogs = append(m.filteredLogs, highlighted)
				}
			}
		}
	}
	
	// Update viewport content
	m.logsViewport.SetContent(strings.Join(m.filteredLogs, "\n"))
	// Scroll to bottom to show most recent logs
	m.logsViewport.GotoBottom()
	return m
}

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) error {
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