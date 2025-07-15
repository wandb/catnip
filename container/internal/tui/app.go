package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
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

func NewApp(containerService *services.ContainerService, containerName, workDir string) *App {
	return &App{
		containerService: containerService,
		containerName:    containerName,
	}
}

func (a *App) Run(ctx context.Context, workDir string) error {
	// Initialize search input
	searchInput := textinput.New()
	searchInput.Placeholder = "Enter search pattern (regex supported)..."
	searchInput.CharLimit = 100
	searchInput.Width = 50
	searchInput.Prompt = "🔍 "
	searchInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	searchInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	searchInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	
	// Initialize viewport (will be sized in Update)
	logsViewport := viewport.New(80, 20)
	
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
		logo:             loadLogo(),
		lastUpdate:       time.Now(),
		logsViewport:     logsViewport,
		searchInput:      searchInput,
		searchMode:       false,
		searchPattern:    "",
	}

	a.program = tea.NewProgram(m, tea.WithAltScreen())

	go func() {
		<-ctx.Done()
		a.program.Quit()
	}()

	_, err := a.program.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tick(),
		m.fetchContainerInfo(),
		m.fetchPorts(),
		m.fetchRepositoryInfo(),
		m.fetchContainerRepos(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		// Update viewport size for logs view
		if m.currentView == logsView {
			headerHeight := 4 // Height for header and search bar
			m.logsViewport.Width = msg.Width - 4
			m.logsViewport.Height = msg.Height - headerHeight
			m.searchInput.Width = msg.Width - 20
		}
		return m, nil

	case tea.KeyMsg:
		// Handle search mode keys first
		if m.currentView == logsView && m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchInput.Blur()
				return m, nil
			case "enter":
				m.searchMode = false
				m.searchInput.Blur()
				m.searchPattern = m.searchInput.Value()
				m = m.updateLogFilter()
				return m, nil
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
		}
		
		// Handle logs view navigation
		if m.currentView == logsView && !m.searchMode {
			switch msg.String() {
			case "/":
				m.searchMode = true
				cmd := m.searchInput.Focus()
				return m, cmd
			case "c":
				// Clear search filter
				m.searchPattern = ""
				m.searchInput.SetValue("")
				m = m.updateLogFilter()
				return m, nil
			case "up", "k":
				m.logsViewport.ScrollUp(1)
				return m, nil
			case "down", "j":
				m.logsViewport.ScrollDown(1)
				return m, nil
			case "pgup", "b":
				m.logsViewport.PageUp()
				return m, nil
			case "pgdown", "f":
				m.logsViewport.PageDown()
				return m, nil
			case "home", "g":
				m.logsViewport.GotoTop()
				return m, nil
			case "end", "G":
				m.logsViewport.GotoBottom()
				return m, nil
			}
		}
		
		// Global key handlers
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "l":
			if m.currentView == logsView {
				m.currentView = overviewView
			} else {
				m.currentView = logsView
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
			m.currentView = overviewView
		case "r":
			if m.currentView == logsView {
				return m, m.fetchLogs()
			}
			return m, tea.Batch(
				m.fetchContainerInfo(),
				m.fetchPorts(),
			)
		case "0":
			if m.currentView == overviewView {
				// Open the main UI at port 8080
				url := "http://localhost:8080"
				go func() { _ = browser.OpenURL(url) }()
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if m.currentView == overviewView {
				portIndex := int(msg.String()[0] - '1')
				if portIndex < len(m.ports) {
					port := m.ports[portIndex]
					url := fmt.Sprintf("http://localhost:8080/%s", port)
					go func() { _ = browser.OpenURL(url) }()
				}
			}
		}

	case tickMsg:
		m.lastUpdate = time.Time(msg)
		return m, tea.Batch(
			tick(),
			m.fetchContainerInfo(),
			m.fetchPorts(),
			m.fetchContainerRepos(),
		)

	case containerInfoMsg:
		m.containerInfo = map[string]interface{}(msg)

	case repositoryInfoMsg:
		m.repositoryInfo = map[string]interface{}(msg)

	case containerReposMsg:
		m.containerRepos = map[string]interface{}(msg)

	case logsMsg:
		m.logs = []string(msg)
		m = m.updateLogFilter()

	case portsMsg:
		m.ports = []string(msg)

	case errMsg:
		m.err = error(msg)
	}

	return m, nil
}


func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	var content string
	
	switch m.currentView {
	case overviewView:
		content = m.renderOverview()
	case logsView:
		content = m.renderLogs()
	}

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Width(m.width - 2).
		Padding(0, 1)

	header := headerStyle.Render(fmt.Sprintf("🐱 Catnip - %s", m.containerName))

	// Footer
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		Width(m.width - 2).
		Padding(0, 1)

	var footer string
	if m.currentView == overviewView {
		footer = footerStyle.Render(m.renderMarkdown("Press **l** for logs, **0** to open UI, **1-9** to open ports, **r** to refresh, **q** to quit"))
	} else {
		if m.searchMode {
			// Replace footer with search input
			searchPrompt := "Search: "
			searchContent := searchPrompt + m.searchInput.View() + " (Enter to apply, Esc to cancel)"
			footer = footerStyle.Render(searchContent)
		} else {
			footer = footerStyle.Render(m.renderMarkdown("**/** search, **c** clear filter, **↑↓** scroll, **o** overview, **r** refresh, **q** quit"))
		}
	}

	// Main content area
	mainHeight := m.height - 4 // Account for header and footer
	mainStyle := lipgloss.NewStyle().
		Width(m.width - 2).
		Height(mainHeight).
		Padding(1)

	mainContent := mainStyle.Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, header, mainContent, footer)
}

func (m model) renderOverview() string {
	// Check if we have enough width for the logo (70 cols + 70 for content = 140+ total)
	showLogo := m.width >= 150 && len(m.logo) > 0
	
	var sections []string

	// Container Status
	statusStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("2"))
	
	sections = append(sections, statusStyle.Render("📦 Container Status"))
	
	if m.containerInfo["name"] != nil {
		sections = append(sections, fmt.Sprintf("  Name: %v", m.containerInfo["name"]))
		sections = append(sections, fmt.Sprintf("  Runtime: %v", m.containerInfo["runtime"]))
		sections = append(sections, fmt.Sprintf("  Last updated: %s", m.lastUpdate.Format("15:04:05")))
	} else {
		sections = append(sections, "  Status: Starting...")
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
		sections = append(sections, "  Stats: Loading...")
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
	return tea.Tick(time.Second*3, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) fetchContainerInfo() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		info, err := m.containerService.GetContainerInfo(ctx, m.containerName)
		if err != nil {
			return errMsg(err)
		}
		return containerInfoMsg(info)
	}
}

func (m model) fetchLogs() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		cmd, err := m.containerService.GetContainerLogs(ctx, m.containerName, false)
		if err != nil {
			return errMsg(err)
		}
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			return errMsg(err)
		}
		
		lines := strings.Split(string(output), "\n")
		return logsMsg(lines)
	}
}

func (m model) fetchPorts() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		ports, err := m.containerService.GetContainerPorts(ctx, m.containerName)
		if err != nil {
			return errMsg(err)
		}
		return portsMsg(ports)
	}
}

func (m model) fetchRepositoryInfo() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		info := m.containerService.GetRepositoryInfo(ctx, m.workDir)
		return repositoryInfoMsg(info)
	}
}

func (m model) fetchContainerRepos() tea.Cmd {
	return func() tea.Msg {
		// Try to fetch repositories from the container's API
		client := &http.Client{Timeout: 2 * time.Second}
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