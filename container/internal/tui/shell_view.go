package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ShellManager handles shell sessions with proper tea.Cmd integration
type ShellManager struct {
	sessions map[string]*ShellSession
	program  *tea.Program
}

type ShellSession struct {
	ID        string
	Client    *PTYClient
	Output    []byte
	Connected bool
	Error     error
}

var globalShellManager *ShellManager

func InitShellManager(p *tea.Program) {
	globalShellManager = &ShellManager{
		sessions: make(map[string]*ShellSession),
		program:  p,
	}
}

func (sm *ShellManager) CreateSession(sessionID string) *ShellSession {
	session := &ShellSession{
		ID:     sessionID,
		Client: NewPTYClient(sessionID),
		Output: []byte{},
	}
	
	sm.sessions[sessionID] = session
	
	// Set up handlers
	session.Client.SetMessageHandler(func(data []byte) {
		session.Output = append(session.Output, data...)
		if sm.program != nil {
			sm.program.Send(shellOutputMsg{
				sessionID: sessionID,
				data:      data,
			})
		}
	})
	
	session.Client.SetErrorHandler(func(err error) {
		session.Error = err
		if sm.program != nil {
			sm.program.Send(shellErrorMsg{
				sessionID: sessionID,
				err:       err,
			})
		}
	})
	
	return session
}

func (sm *ShellManager) ConnectSession(sessionID string) error {
	if session, exists := sm.sessions[sessionID]; exists {
		err := session.Client.Connect("http://localhost:8080")
		if err == nil {
			session.Connected = true
		}
		return err
	}
	return fmt.Errorf("session not found: %s", sessionID)
}


func (sm *ShellManager) GetSession(sessionID string) *ShellSession {
	return sm.sessions[sessionID]
}

// Helper function to create and connect a new shell session
func createAndConnectShell(sessionID string, width, height int) tea.Cmd {
	return func() tea.Msg {
		if globalShellManager == nil {
			return shellErrorMsg{
				sessionID: sessionID,
				err:       fmt.Errorf("shell manager not initialized"),
			}
		}
		
		_ = globalShellManager.CreateSession(sessionID)
		
		// Connect in background and send initial size
		go func() {
			err := globalShellManager.ConnectSession(sessionID)
			if err != nil {
				debugLog("Failed to connect shell session %s: %v", sessionID, err)
				return
			}
			
			// Send initial terminal size after connection with a small delay
			time.Sleep(200 * time.Millisecond) // Give PTY time to initialize
			if session := globalShellManager.GetSession(sessionID); session != nil && session.Client != nil {
				debugLog("Sending initial PTY size: %dx%d", width, height)
				if err := session.Client.Resize(width, height); err != nil {
					debugLog("Failed to send initial PTY size: %v", err)
				}
				
			}
		}()
		
		// Give it a moment to connect
		time.Sleep(100 * time.Millisecond)
		
		// Don't send any initial message - let the shell prompt show
		return nil
	}
}

// Updated createNewShellSession to use the shell manager
func (m model) createNewShellSessionWithCmd() (model, tea.Cmd) {
	sessionID := fmt.Sprintf("shell-%d", time.Now().Unix())
	m.currentSessionID = sessionID
	m.currentView = shellView
	m.shellOutput = ""
	m.shellConnecting = true // Set connecting state
	m.shellLastInput = time.Now() // Initialize cursor timer
	
	// Initialize shell viewport
	if m.height > 0 {
		headerHeight := 3
		m.shellViewport.Width = m.width - 2
		m.shellViewport.Height = m.height - headerHeight
		// Initialize or resize terminal emulator to match viewport
		// Account for viewport padding/borders
		terminalWidth := m.shellViewport.Width - 2  // Subtract 2 for viewport borders
		if m.terminalEmulator == nil {
			debugLog("Creating terminal emulator with size: %dx%d", terminalWidth, m.shellViewport.Height)
			m.terminalEmulator = NewTerminalEmulator(terminalWidth, m.shellViewport.Height)
		} else {
			m.terminalEmulator.Clear()
			m.terminalEmulator.Resize(terminalWidth, m.shellViewport.Height)
		}
	}
	
	// Return the command to create and connect the session
	// Use terminal width (viewport width - 2 for borders)
	terminalWidth := m.shellViewport.Width - 2
	return m, createAndConnectShell(sessionID, terminalWidth, m.shellViewport.Height)
}

// Helper to format session list for display
func formatSessionList(sessions map[string]*ShellSession) string {
	if len(sessions) == 0 {
		return "No active sessions"
	}
	
	var sb strings.Builder
	i := 1
	for id, session := range sessions {
		status := "disconnected"
		if session.Connected {
			status = "connected"
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%s)\n", i, id, status))
		i++
	}
	return sb.String()
}