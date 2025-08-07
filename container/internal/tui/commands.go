package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vanpelt/catnip/internal/models"
)

// Ticker commands
func tick() tea.Cmd {
	return tea.Tick(time.Second*5, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func animationTick() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
		return animationTickMsg(t)
	})
}

func logsTick() tea.Cmd {
	return tea.Tick(time.Second*1, func(t time.Time) tea.Msg {
		return logsTickMsg(t)
	})
}

// Data fetching commands
func (m *Model) fetchContainerInfo() tea.Cmd {
	return func() tea.Msg {
		// If quit was requested, don't execute this command
		if m.quitRequested {
			debugLog("fetchContainerInfo: quit requested, skipping")
			return nil
		}

		// Docker stats typically takes ~2.0-2.3 seconds to run, so we need a timeout > 2 seconds
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		info, err := m.containerService.GetContainerInfo(ctx, m.containerName)
		if err != nil {
			// Don't show errors for timeout/context cancellation to reduce noise
			return nil
		}

		return containerInfoMsg(info)
	}
}

func (m *Model) fetchLogs() tea.Cmd {
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

func (m *Model) fetchPorts() tea.Cmd {
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

func (m *Model) fetchRepositoryInfo() tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		debugLog("fetchRepositoryInfo() starting")

		// If quit was requested, don't execute this command
		if m.quitRequested {
			debugLog("fetchRepositoryInfo: quit requested, skipping")
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		debugLog("fetchRepositoryInfo() calling GetRepositoryInfo - elapsed: %v", time.Since(start))
		info := m.containerService.GetRepositoryInfo(ctx, m.gitRoot)
		debugLog("fetchRepositoryInfo() GetRepositoryInfo returned - elapsed: %v", time.Since(start))

		return repositoryInfoMsg(info)
	}
}

// fetchHealthStatus checks the health of the main application
func (m *Model) fetchHealthStatus() tea.Cmd {
	return func() tea.Msg {
		healthy := isAppReady("http://localhost:8080")
		debugLog("Health check result: %v", healthy)
		return healthStatusMsg(healthy)
	}
}

func (m *Model) fetchWorkspaces() tea.Cmd {
	return func() tea.Msg {
		// If quit was requested, don't execute this command
		if m.quitRequested {
			debugLog("fetchWorkspaces: quit requested, skipping")
			return nil
		}

		workspaces, err := fetchWorkspacesFromAPI()
		if err != nil {
			debugLog("fetchWorkspaces: error: %v", err)
			return workspacesMsg{} // Return empty list on error
		}
		return workspacesMsg(workspaces)
	}
}

// Batch commands for initialization
func (m *Model) initCommands() tea.Cmd {
	var commands []tea.Cmd

	// Add spinner tick for initialization view
	if m.currentView == InitializationView {
		if initView, ok := m.views[InitializationView].(*InitializationViewImpl); ok {
			commands = append(commands, initView.spinner.Tick)
			// Start initialization process immediately with model's parameters
			commands = append(commands, StartInitializationProcess(m))
		}
	}

	// Add other commands
	commands = append(commands,
		m.fetchRepositoryInfo(),
		m.fetchHealthStatus(),
		m.fetchPorts(),
		m.fetchWorkspaces(),
		m.fetchContainerInfo(),
		m.shellSpinner.Tick,
		tick(),
		animationTick(),
		logsTick(),
	)

	return tea.Batch(commands...)
}

// fetchWorkspacesFromAPI fetches workspaces from the container API
func fetchWorkspacesFromAPI() ([]WorkspaceInfo, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:8080/v1/git/worktrees")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, err // Return empty if API call fails
	}

	var worktrees []models.Worktree
	if err := json.NewDecoder(resp.Body).Decode(&worktrees); err != nil {
		return nil, err
	}

	// Convert models.Worktree to WorkspaceInfo
	var workspaces []WorkspaceInfo
	for _, wt := range worktrees {
		workspace := WorkspaceInfo{
			ID:           wt.ID,
			Name:         wt.Name,
			Path:         wt.Path,
			Branch:       wt.Branch,
			IsActive:     wt.IsActive,
			ChangedFiles: make([]string, len(wt.ChangedFiles)),
			Ports:        []PortInfo{}, // TODO: Map ports if available in worktree model
		}

		// Copy changed files
		copy(workspace.ChangedFiles, wt.ChangedFiles)

		workspaces = append(workspaces, workspace)
	}

	return workspaces, nil
}
