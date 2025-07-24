# TUI Architecture Documentation

This document describes the architecture and design of the Catnip Terminal User Interface (TUI), built using the Bubble Tea framework.

## Overview

The Catnip TUI is a terminal-based interface that provides users with an interactive dashboard for managing their containerized development environment. It features multiple views, real-time updates, and seamless integration with the container backend services.

## Architecture

### Core Components

#### 1. Model (`model.go`)

The central state container for the entire TUI application:

- Stores current view state (Overview, Shell, Logs)
- Manages container and repository information
- Tracks UI state like window dimensions and error messages
- Handles terminal components (PTY, viewport for logs)

#### 2. Update System (`update.go`)

Implements the Bubble Tea update pattern:

- Routes messages to appropriate handlers based on type
- Manages view transitions and state updates
- Handles keyboard input and system events
- Preserves data consistency (e.g., stats preservation when updates fail)

#### 3. View System

Three main views, each with its own rendering logic:

- **Overview View** (`view_overview.go`): Dashboard showing system stats, repository info, and available actions
- **Shell View** (`view_shell.go`): Full terminal emulation for container interaction
- **Logs View** (`view_logs.go`): Scrollable log viewer with search functionality
- **Initialization View** (`view_initialization.go`): Container startup progress display

#### 4. Commands (`commands.go`)

Background tasks that fetch data and update state:

- `fetchContainerInfo()`: Retrieves container stats and port information
- `fetchRepositoryInfo()`: Gets git repository details
- `fetchHealthStatus()`: Monitors container health
- `fetchPorts()`: Tracks exposed container ports
- Tick commands for periodic updates (5-second intervals)

#### 5. SSE Client (`sse_client.go`)

Server-Sent Events client for real-time updates:

- Connects to the container's event stream
- Handles port notifications, container status changes
- Manages reconnection logic with exponential backoff
- Provides heartbeat monitoring

### Message Flow

1. **Input Messages**: Keyboard events, window resizes
2. **Data Messages**: Container info, repository updates, port changes
3. **Tick Messages**: Periodic triggers for data fetching
4. **SSE Messages**: Real-time events from the container

### Key Design Patterns

#### 1. Stats Preservation

The TUI preserves the last known good stats when docker stats commands fail:

```go
// If new stats are empty but we have old stats, preserve them
if !hasValidNewStats && hasOldStats {
    newInfo["stats"] = oldStats
}
```

#### 2. Graceful Timeouts

Commands use appropriate timeouts to prevent blocking:

- Container info fetch: 3 seconds (docker stats takes ~2.0-2.3s)
- Other fetches: 2 seconds

#### 3. Debug Logging

Minimal logging approach - only log failures:

```go
containerDebugLog("Failed to get container stats for %s: %v", containerName, err)
```

## Component Interactions

### Initialization Flow

1. `NewModel()` creates initial model with default view
2. `Init()` starts all background commands and SSE client
3. Initialization view shows while container starts
4. Transitions to Overview view when ready

### View Transitions

- `Ctrl+O`: Switch to Overview
- `Ctrl+S`: Switch to Shell
- `Ctrl+L`: Switch to Logs
- `q`: Quit application (from Overview only)

### Data Update Cycle

1. Tick message triggers fetch commands
2. Commands query container service
3. Results sent as typed messages
4. Update handlers merge new data with existing state
5. View re-renders with updated data

## Styling

Uses custom styles defined in `components/styles.go`:

- Consistent color scheme with primary/secondary colors
- Styled headers, borders, and UI elements
- Support for both light and dark terminals

## Error Handling

- Network errors: Silent retry with preserved state
- Container errors: Displayed in UI with error styling
- Graceful degradation when services unavailable

## Future Enhancements

1. **File Browser**: Navigate container filesystem
2. **Process Viewer**: Monitor running processes
3. **Resource Graphs**: Visual CPU/memory usage over time
4. **Multi-container Support**: Switch between containers
5. **Custom Themes**: User-configurable color schemes
