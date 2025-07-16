package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		debugLog("fetchRepositoryInfo() calling GetRepositoryInfo - elapsed: %v", time.Since(start))
		info := m.containerService.GetRepositoryInfo(ctx, m.workDir)
		debugLog("fetchRepositoryInfo() GetRepositoryInfo returned - elapsed: %v", time.Since(start))

		return repositoryInfoMsg(info)
	}
}

// fetchHealthStatus checks the health of the main application
func (m *Model) fetchHealthStatus() tea.Cmd {
	return func() tea.Msg {
		healthy := isAppReady("http://localhost:8080")
		return healthStatusMsg(healthy)
	}
}

// Batch commands for initialization
func (m *Model) initCommands() tea.Cmd {
	return tea.Batch(
		m.fetchRepositoryInfo(),
		m.fetchHealthStatus(),
		m.fetchPorts(),
		m.fetchContainerInfo(),
		m.shellSpinner.Tick,
		tick(),
		animationTick(),
		logsTick(),
	)
}
