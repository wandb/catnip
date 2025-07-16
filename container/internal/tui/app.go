package tui

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vanpelt/catnip/internal/services"
)

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
		debugLogger.Printf(format, args...)
	}
}

type view int

const (
	overviewView view = iota
	logsView
	shellView
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
	
	// Health status and animation
	appHealthy       bool
	bootingAnimDots  int
	bootingBold      bool
	bootingBoldTimer time.Time
	
	// Enhanced logs view
	logsViewport     viewport.Model
	searchInput      textinput.Model
	searchMode       bool
	searchPattern    string
	compiledRegex    *regexp.Regexp
	lastLogCount     int
	
	// Shell view
	ptyClient        *PTYClient
	shellViewport    viewport.Model
	shellOutput      string
	shellSessions    map[string]*PTYClient
	showSessionList  bool
	currentSessionID string
	shellConnecting  bool
	shellSpinner     spinner.Model
	shellCursorVisible bool
	shellLastInput   time.Time
	
	// Logs view scroll preservation
	logsScrollLocked bool
	logsLockedPosition int
}

type tickMsg time.Time
type containerInfoMsg map[string]interface{}
type repositoryInfoMsg map[string]interface{}
type containerReposMsg map[string]interface{}
type logsMsg []string
type portsMsg []string
type errMsg error
type quitMsg struct{}
type healthStatusMsg bool
type animationTickMsg time.Time
type logsTickMsg time.Time
type shellOutputMsg struct {
	sessionID string
	data      []byte
}
type shellErrorMsg struct {
	sessionID string
	err       error
}


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
	
	logo := loadLogo()
	
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
		lastLogCount:     0,
		shellSessions:    make(map[string]*PTYClient),
		shellViewport:    viewport.New(80, 24),
		shellOutput:      "",
		showSessionList:  false,
		currentSessionID: "",
		shellConnecting:  false,
		shellSpinner:     spinner.New(),
	}

	a.program = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	
	// Initialize the shell manager with the program
	InitShellManager(a.program)

	_, err := a.program.Run()
	return err
}

func (m model) Init() tea.Cmd {
	// Initialize spinner
	m.shellSpinner.Spinner = spinner.Dot
	m.shellSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	
	// Start background data fetching
	return tea.Batch(
		m.fetchRepositoryInfo(),
		m.fetchHealthStatus(),
		m.shellSpinner.Tick,
		tea.Tick(time.Second*5, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}),
		tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
			return animationTickMsg(t)
		}),
		tea.Tick(time.Second*1, func(t time.Time) tea.Msg {
			return logsTickMsg(t)
		}),
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
		} else if m.currentView == shellView {
			// Update shell viewport size
			headerHeight := 3
			m.shellViewport.Width = msg.Width - 2
			m.shellViewport.Height = msg.Height - headerHeight
			// Send resize to PTY
			if globalShellManager != nil {
				if session := globalShellManager.GetSession(m.currentSessionID); session != nil && session.Client != nil {
					go func(width, height int) {
						if err := session.Client.Resize(width, height); err != nil {
							log.Printf("Failed to resize PTY: %v", err)
						}
					}(m.shellViewport.Width, m.shellViewport.Height)
				}
			}
		}
		
		return m, nil

	case tea.MouseMsg:
		// Handle mouse wheel scrolling in different views
		// Check for wheel events (they may have action=0 or action=1 depending on terminal)
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.currentView == shellView && !m.showSessionList {
					m.shellViewport.ScrollUp(3)
				} else if m.currentView == logsView && !m.searchMode {
					m.logsViewport.ScrollUp(3)
				}
				return m, nil
			case tea.MouseButtonWheelDown:
				if m.currentView == shellView && !m.showSessionList {
					m.shellViewport.ScrollDown(3)
				} else if m.currentView == logsView && !m.searchMode {
					m.logsViewport.ScrollDown(3)
				}
				return m, nil
			}
		}
		// Ignore other mouse events
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
		
		// Handle shell view input
		if m.currentView == shellView {
			if m.showSessionList {
				// Handle session list navigation
				switch msg.String() {
				case "esc":
					m.showSessionList = false
					m.currentView = overviewView
					return m, nil
				case "n":
					m.showSessionList = false
					newModel, cmd := m.createNewShellSessionWithCmd()
					return newModel, cmd
				case "1", "2", "3", "4", "5", "6", "7", "8", "9":
					i := int(msg.String()[0] - '1')
					if globalShellManager != nil {
						sessionIDs := make([]string, 0, len(globalShellManager.sessions))
						for id := range globalShellManager.sessions {
							sessionIDs = append(sessionIDs, id)
						}
						if i < len(sessionIDs) {
							m.showSessionList = false
							m = m.switchToShellSession(sessionIDs[i])
						}
					}
					return m, nil
				}
			} else {
				// Forward all input to PTY except special keys
				switch msg.String() {
				case "ctrl+o":
					m.currentView = overviewView
					return m, nil
				case "ctrl+q":
					return m, tea.Quit
				// Handle viewport scrolling
				case "pgup", "ctrl+b":
					m.shellViewport.PageUp()
					return m, nil
				case "pgdown", "ctrl+f":
					m.shellViewport.PageDown()
					return m, nil
				case "home", "ctrl+home":
					m.shellViewport.GotoTop()
					return m, nil
				case "end", "ctrl+end":
					m.shellViewport.GotoBottom()
					return m, nil
				// Alt/Option key combinations (for Mac)
				case "alt+up":
					m.shellViewport.ScrollUp(1)
					return m, nil
				case "alt+down":
					m.shellViewport.ScrollDown(1)
					return m, nil
				default:
					// Send input to PTY
					if globalShellManager != nil {
						if session := globalShellManager.GetSession(m.currentSessionID); session != nil && session.Client != nil {
						var data []byte
						if len(msg.Runes) > 0 {
							data = []byte(string(msg.Runes))
						} else {
							// Handle special keys
							switch msg.Type {
							case tea.KeyEnter:
								data = []byte("\r")
							case tea.KeyBackspace:
								data = []byte{127}
							case tea.KeyTab:
								data = []byte("\t")
							case tea.KeyEsc:
								data = []byte{27}
							case tea.KeyUp:
								data = []byte("\x1b[A")
							case tea.KeyDown:
								data = []byte("\x1b[B")
							case tea.KeyRight:
								data = []byte("\x1b[C")
							case tea.KeyLeft:
								data = []byte("\x1b[D")
							default:
								// Handle Ctrl+C, Ctrl+D, etc.
								switch msg.String() {
								case "ctrl+c":
									data = []byte{3}
								case "ctrl+d":
									data = []byte{4}
								case "ctrl+z":
									data = []byte{26}
								}
							}
						}
						if len(data) > 0 {
							m.shellLastInput = time.Now() // Update last input time for cursor
							go func(d []byte) {
								if err := session.Client.Send(d); err != nil {
									log.Printf("Failed to send data to PTY: %v", err)
								}
							}(data)
						}
					}
					}
					return m, nil
				}
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
		case "q", "ctrl+c":
			return m, tea.Quit
		case "l":
			if m.currentView == logsView {
				m.currentView = overviewView
				return m, nil
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
			return m, nil
		case "s":
			if m.currentView == overviewView {
				// Check if we have existing sessions
				if globalShellManager != nil && len(globalShellManager.sessions) > 0 {
					m.showSessionList = true
					m.currentView = shellView // Switch to shell view to show the list
					return m, nil
				} else {
					// Create new session
					newModel, cmd := m.createNewShellSessionWithCmd()
					return newModel, cmd
				}
			} else if m.currentView == shellView {
				// Already in shell view, do nothing
				return m, nil
			}
		case "0":
			if m.appHealthy {
				go func() {
					if err := openBrowser("http://localhost:8080"); err != nil {
						// Error opening browser
					}
				}()
			} else {
				// App is not ready, show bold feedback
				m.bootingBold = true
				m.bootingBoldTimer = time.Now()
			}
			return m, nil
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if m.currentView == overviewView {
				portIndex := int(msg.String()[0] - '1') // Convert '1'-'9' to 0-8
				if portIndex < len(m.ports) {
					port := m.ports[portIndex]
					url := fmt.Sprintf("http://localhost:8080/%s", port)
					go func() {
						if isAppReady("http://localhost:8080") {
							if err := openBrowser(url); err != nil {
								// Error opening browser
							}
						}
					}()
				}
			}
			return m, nil
		}
		
		return m, nil

	case spinner.TickMsg:
		if m.currentView == shellView && m.shellConnecting {
			var cmd tea.Cmd
			m.shellSpinner, cmd = m.shellSpinner.Update(msg)
			return m, cmd
		}
		return m, nil
	
	case tickMsg:
		m.lastUpdate = time.Time(msg)
		return m, tea.Batch(
			tick(),
			m.fetchContainerInfo(),
			m.fetchPorts(),
			m.fetchHealthStatus(),
		)
	
	case animationTickMsg:
		// Update animation state
		m.bootingAnimDots = (m.bootingAnimDots + 1) % 4
		
		// Check if we need to turn off bold
		if m.bootingBold && time.Since(m.bootingBoldTimer) > 3*time.Second {
			m.bootingBold = false
		}
		
		return m, tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
			return animationTickMsg(t)
		})
	
	case logsTickMsg:
		// Auto-refresh logs only when in logs view
		if m.currentView == logsView {
			return m, tea.Batch(
				tea.Tick(time.Second*1, func(t time.Time) tea.Msg {
					return logsTickMsg(t)
				}),
				m.fetchLogs(),
			)
		} else if m.currentView == shellView {
			// Schedule next tick for cursor blinking
			return m, tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
				return logsTickMsg(t)
			})
		}
		// If not in logs or shell view, just schedule next tick
		return m, tea.Tick(time.Second*1, func(t time.Time) tea.Msg {
			return logsTickMsg(t)
		})
	
	case shellOutputMsg:
		if msg.sessionID == m.currentSessionID {
			// First output means we're connected
			if m.shellConnecting {
				m.shellConnecting = false
				m.shellOutput = "" // Clear the "Connecting..." message
			}
			m.shellOutput += string(msg.data)
			m.shellViewport.SetContent(m.shellOutput)
			m.shellViewport.GotoBottom()
		}
		return m, nil
	
	case shellErrorMsg:
		if msg.sessionID == m.currentSessionID {
			m.shellConnecting = false
			m.shellOutput += fmt.Sprintf("\n\r[Error: %v]\n\r", msg.err)
			m.shellViewport.SetContent(m.shellOutput)
			m.shellViewport.GotoBottom()
		}
		return m, nil

	case containerInfoMsg:
		m.containerInfo = map[string]interface{}(msg)

	case repositoryInfoMsg:
		m.repositoryInfo = map[string]interface{}(msg)

	case containerReposMsg:
		m.containerRepos = map[string]interface{}(msg)

	case logsMsg:
		newLogs := []string(msg)
		
		// Check if this is new logs or a full refresh
		if len(newLogs) > m.lastLogCount {
			// We have new logs to stream
			m = m.streamNewLogs(newLogs)
		} else if len(newLogs) < m.lastLogCount || m.lastLogCount == 0 {
			// Full refresh (manual refresh or first load)
			m.logs = newLogs
			m = m.updateLogFilter()
		} else {
			// Same number of logs, no update needed
		}
		
		m.lastLogCount = len(newLogs)

	case portsMsg:
		m.ports = []string(msg)

	case healthStatusMsg:
		m.appHealthy = bool(msg)

	case errMsg:
		m.err = error(msg)
		
	case quitMsg:
		return m, tea.Quit
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
	case shellView:
		content = m.renderShell()
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
		footer = footerStyle.Render("Press l for logs, s for shell, 0 to open UI, 1-9 to open ports, q to quit")
	} else if m.currentView == shellView {
		scrollKey := "Alt"
		if runtime.GOOS == "darwin" {
			scrollKey = "Option"
		}
		footer = footerStyle.Render(fmt.Sprintf("Ctrl+O: overview | Ctrl+Q: quit | %s+↑↓/PgUp/PgDn: scroll", scrollKey))
	} else {
		if m.searchMode {
			// Replace footer with search input
			searchPrompt := "Search: "
			searchContent := searchPrompt + m.searchInput.View() + " (Enter to apply, Esc to cancel)"
			footer = footerStyle.Render(searchContent)
		} else {
			if m.searchPattern != "" {
				footer = footerStyle.Render("/ search, c clear filter, ↑↓ scroll, o overview, q quit • Streaming filtered logs")
			} else {
				footer = footerStyle.Render("/ search, c clear filter, ↑↓ scroll, o overview, q quit • Auto-refresh: ON")
			}
		}
	}

	// Main content area
	mainHeight := m.height - 4 // Account for header and footer
	mainStyle := lipgloss.NewStyle().
		Width(m.width - 2).
		Height(mainHeight).
		Padding(1)

	mainContent := mainStyle.Render(content)

	result := lipgloss.JoinVertical(lipgloss.Left, header, mainContent, footer)
	return result
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
	
	// Show booting animation if not healthy
	if !m.appHealthy {
		dots := strings.Repeat(".", m.bootingAnimDots)
		spaces := strings.Repeat(" ", 3-m.bootingAnimDots)
		bootingText := fmt.Sprintf("Booting%s%s", dots, spaces)
		
		if m.bootingBold {
			bootingStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
			sections = append(sections, fmt.Sprintf("  %s %s", keyHighlight.Render("0."), bootingStyle.Render(bootingText)))
		} else {
			sections = append(sections, fmt.Sprintf("  %s %s", keyHighlight.Render("0."), bootingText))
		}
	} else {
		sections = append(sections, fmt.Sprintf("  %s Main UI → http://localhost:8080", keyHighlight.Render("0.")))
	}
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
	currentOffset := m.logsViewport.YOffset
	viewportContent := m.logsViewport.View()
	afterOffset := m.logsViewport.YOffset
	
	if currentOffset != afterOffset {
		debugLog("renderLogs: viewport offset changed during View()! before=%d, after=%d", currentOffset, afterOffset)
	}
	
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

func (m model) renderShell() string {
	if m.showSessionList {
		return m.renderSessionList()
	}
	
	// Header with session info
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("8")).
		Padding(0, 1).
		Width(m.width - 2)
	
	header := headerStyle.Render(fmt.Sprintf("Shell Session: %s | Press Ctrl+O to return to overview", m.currentSessionID))
	
	// If connecting, show spinner
	if m.shellConnecting {
		connectingStyle := lipgloss.NewStyle().
			Padding(2, 0).
			Align(lipgloss.Center).
			Width(m.width - 2).
			Height(m.height - 6)
		
		connectingContent := fmt.Sprintf("%s Connecting to shell...\n\nPlease wait while we establish a connection to the container.", m.shellSpinner.View())
		return fmt.Sprintf("%s\n%s", header, connectingStyle.Render(connectingContent))
	}
	
	// Shell output in viewport with cursor
	output := m.shellOutput
	
	// Add blinking cursor at the end
	if time.Since(m.shellLastInput).Milliseconds()%1000 < 500 {
		output += "█"
	} else {
		output += " "
	}
	
	m.shellViewport.SetContent(output)
	
	return fmt.Sprintf("%s\n%s", header, m.shellViewport.View())
}

func (m model) renderSessionList() string {
	listStyle := lipgloss.NewStyle().
		Padding(1, 2).
		Width(m.width - 4)
	
	var content strings.Builder
	content.WriteString("Active Shell Sessions:\n\n")
	
	i := 1
	if globalShellManager != nil {
		for sessionID, session := range globalShellManager.sessions {
			status := "disconnected"
			if session.Connected {
				status = "connected"
			}
			content.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i, sessionID, status))
			i++
		}
	}
	
	content.WriteString("\n  n. Create new session")
	content.WriteString("\n  ESC. Cancel\n")
	
	return listStyle.Render(content.String())
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


// streamNewLogs handles streaming new log entries with filtering
func (m model) streamNewLogs(newLogs []string) model {
	// Get only the new entries
	newEntries := newLogs[m.lastLogCount:]
	
	// Update the complete logs
	m.logs = newLogs
	
	// If no filter is active, update logs but preserve scroll position
	if m.searchPattern == "" {
		// Calculate if we're at the bottom more precisely
		// We're at bottom if the current view can see the last line
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
	// Calculate if we're at the bottom more precisely
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
					return lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0")).Render(match)
				})
				newFilteredEntries = append(newFilteredEntries, highlighted)
			}
		} else {
			// Simple string search fallback
			searchLower := strings.ToLower(m.searchPattern)
			if strings.Contains(strings.ToLower(line), searchLower) {
				highlighted := strings.ReplaceAll(line, m.searchPattern, 
					lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0")).Render(m.searchPattern))
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

// updateLogFilter applies the current search pattern to logs and updates the viewport
func (m model) updateLogFilter() model {
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

// fetchHealthStatus checks the health of the main application
func (m model) fetchHealthStatus() tea.Cmd {
	return func() tea.Msg {
		healthy := isAppReady("http://localhost:8080")
		return healthStatusMsg(healthy)
	}
}

// Shell-related methods
func (m model) switchToShellSession(sessionID string) model {
	if globalShellManager != nil {
		if session := globalShellManager.GetSession(sessionID); session != nil {
			m.currentSessionID = sessionID
			m.currentView = shellView
			m.showSessionList = false
			m.shellOutput = string(session.Output)
			m.shellViewport.SetContent(m.shellOutput)
		}
	}
	return m
}