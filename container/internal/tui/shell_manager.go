package tui

import (
	"fmt"
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
