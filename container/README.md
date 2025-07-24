# 🐱 Catnip Container

Go-based server application providing containerized development environments with Git worktree management, WebSocket PTY sessions, and AI agent integration.

## Architecture Overview

The container application consists of several key components:

### Core Applications

- **CLI (`catctrl`)**: Terminal user interface for container management
- **Server (`catnip`)**: HTTP/WebSocket API server with Swagger documentation
- **Container Runtime**: Docker-based isolated development environments

### Directory Structure

```
container/
├── cmd/
│   ├── cli/main.go         # catctrl CLI entry point
│   └── server/main.go      # catnip server entry point
├── internal/
│   ├── models/             # Data models and structures
│   ├── services/           # Business logic layer
│   ├── handlers/           # HTTP/WebSocket request handlers
│   ├── git/                # Git operations and worktree management
│   └── tui/                # Terminal UI components
├── docs/                   # Swagger documentation
└── setup/                  # Container environment setup scripts
```

## Core Components

### Models (`internal/models/`)

Data structures and domain models:

- **`git.go`**: Repository, Worktree, GitStatus models
- **`claude.go`**: Claude AI session and message structures
- **`gemini.go`**: Google Gemini integration models
- **`settings.go`**: Application configuration

### Services (`internal/services/`)

Business logic and core functionality:

- **`git.go`**: Git repository and worktree management service
- **`worktree_manager.go`**: Worktree lifecycle operations
- **`claude.go`**: Claude AI integration service
- **`session.go`**: PTY session management
- **`port_allocation.go`**: Dynamic port allocation
- **`commit_sync.go`**: Automated commit synchronization

### Handlers (`internal/handlers/`)

HTTP/WebSocket request handlers:

- **`git.go`**: Git operations API endpoints
- **`pty.go`**: WebSocket PTY session handling
- **`claude.go`**: Claude AI chat integration
- **`ports.go`**: Port management and proxy
- **`auth.go`**: Authentication handling

### Git Layer (`internal/git/`)

Git operations and abstractions:

- **`operations.go`**: Core git operation interfaces
- **`worktree_manager.go`**: Git worktree lifecycle management
- **`branch.go`**: Branch operations and validation
- **`executor/`**: Git command execution (shell, go-git, in-memory)
- **`utils.go`**: Git utilities and branch naming

## Key Features

### Git Worktree Management

- **Custom Namespace**: Uses `refs/catnip/` to isolate workspace branches
- **Dynamic State Detection**: Automatically syncs metadata with actual git state
- **Comprehensive Cleanup**: Removes branches, worktrees, and dangling references
- **Live Repository Support**: Mounts local repositories with "live" remotes

### AI Integration

- **Claude Code Support**: WebSocket-based Claude chat integration
- **Session Management**: Persistent conversation history
- **File Context**: Automatic file content inclusion in conversations

### Container Orchestration

- **Port Detection**: Automatic discovery of development servers
- **Proxy System**: Built-in reverse proxy for containerized services
- **Volume Management**: Persistent workspace and configuration storage

### Development Tools

- **Swagger Documentation**: Auto-generated API documentation at `/docs`
- **WebSocket PTY**: Real-time terminal access via WebSocket
- **Hot Reload**: Live recompilation during development

## API Structure

### RESTful Endpoints

```
/v1/git/repositories     # Repository management
/v1/git/worktrees        # Worktree operations
/v1/claude/sessions      # AI chat sessions
/v1/ports               # Port management
/v1/auth                # Authentication
```

### WebSocket Endpoints

```
/ws/pty                 # Terminal sessions
/ws/events              # Real-time event streaming
/ws/claude              # AI chat interface
```

## Building and Development

### Prerequisites

- Go 1.21+
- Docker
- `just` command runner

### Build Commands

```bash
# Build both CLI and server
just build

# Run tests
just test

# Run linter
just lint

# Generate Swagger docs
just docs

# Development server (auto-rebuild)
just dev
```

### Docker Development

```bash
# Build development image
docker build -f Dockerfile.dev -t catnip-dev .

# Run development container
docker run -e CATNIP_DEV=true -p 8080:8080 -v $(pwd):/live catnip-dev
```

## Configuration

### Environment Variables

- `CATNIP_DEV`: Enable development mode
- `CATNIP_PORT`: Server port (default: 8080)
- `WORKSPACE_DIR`: Workspace directory path
- `GIT_STATE_DIR`: Git state persistence directory

### Git Configuration

- Uses `refs/catnip/` namespace for workspace branches
- Automatically configures git credentials via GitHub CLI
- Supports both local and remote repository workflows

## Testing

The codebase includes comprehensive test coverage:

### Test Types

- **Unit Tests**: Individual component testing
- **Integration Tests**: Service interaction testing
- **Functional Tests**: End-to-end workflow testing
- **In-Memory Tests**: Fast isolated testing with mock git

### Running Tests

```bash
# All tests
go test -v ./...

# Specific package
go test -v ./internal/services

# With integration tests
RUN_INTEGRATION_TESTS=1 go test -v ./...
```

## Extension Points

### Adding New Services

1. Create service in `internal/services/`
2. Add models in `internal/models/`
3. Create handlers in `internal/handlers/`
4. Update Swagger documentation

### Custom Git Executors

Implement the `CommandExecutor` interface in `internal/git/executor/`:

- `ShellExecutor`: Shell command execution
- `GitExecutor`: go-git library integration
- `InMemoryExecutor`: Testing and mocking

### WebSocket Extensions

Add new WebSocket handlers following the pattern in `internal/handlers/pty.go`

## Security Considerations

- Git operations are sandboxed within container
- Authentication required for sensitive operations
- File system access limited to workspace directories
- Network access controlled via container networking

## Monitoring and Debugging

- **Swagger UI**: Available at `/docs` for API exploration
- **Health Check**: GET `/health` endpoint
- **Metrics**: Built-in request/response logging
- **Debug Mode**: Enable with `CATNIP_DEV=1`
