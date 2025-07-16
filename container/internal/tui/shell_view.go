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
func createAndConnectShell(sessionID string) tea.Cmd {
	return func() tea.Msg {
		if globalShellManager == nil {
			return shellErrorMsg{
				sessionID: sessionID,
				err:       fmt.Errorf("shell manager not initialized"),
			}
		}
		
		_ = globalShellManager.CreateSession(sessionID)
		
		// Connect in background
		go func() {
			err := globalShellManager.ConnectSession(sessionID)
			if err != nil {
				debugLog("Failed to connect shell session %s: %v", sessionID, err)
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
	}
	
	// Return the command to create and connect the session
	return m, createAndConnectShell(sessionID)
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