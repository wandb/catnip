# 🐱 Catnip CLI (catctrl)

Modern containerized coding environment management tool.

## Installation

### Quick Install (Latest Version)
```bash
curl -sfL https://raw.githubusercontent.com/wandb/catnip/main/public/install.sh | sh
```

### Install Specific Version
```bash
curl -sfL https://raw.githubusercontent.com/wandb/catnip/main/public/install.sh | sh -s -- --version v0.1.0
```

### Custom Installation Directory
```bash
curl -sfL https://raw.githubusercontent.com/wandb/catnip/main/public/install.sh | INSTALL_DIR=/usr/local/bin sh
```

## Usage

```bash
# Start interactive container environment
catctrl run

# Start with custom session name
catctrl run --name my-project

# Get help
catctrl --help
```

## Features

- **🐳 Containerized Development**: Isolated, reproducible coding environments
- **📟 Interactive TUI**: Real-time logs, port detection, and status monitoring  
- **🔧 Git Integration**: Automatic repository management with worktrees
- **🌐 Port Forwarding**: Automatic detection and proxy for development servers
- **🤖 Claude Code Support**: Seamless integration with AI coding assistance
- **⚡ Hot Reload**: Live recompilation and restart capabilities

## Requirements

- **Docker**: Required for container management
- **Git**: For repository operations
- **Bash/Zsh**: Compatible shell environment

## Supported Platforms

- **Linux**: amd64, arm64, armv6, armv7
- **macOS**: amd64 (Intel), arm64 (Apple Silicon)  
- **FreeBSD**: amd64, arm64

> **Note**: Windows is not currently supported. Use WSL2 on Windows.

## Development

This CLI is part of the larger [Catnip project](https://github.com/wandb/catnip).

### Building from Source

```bash
# Clone the repository
git clone https://github.com/wandb/catnip.git
cd catnip/container

# Build CLI
go build -o catctrl ./cmd/cli/main.go

# Build server
go build -o catnip ./cmd/server/main.go
```

### Running Tests

```bash
go test -v ./...
```

### Linting

```bash
golangci-lint run
```

## Architecture

- **CLI (`catctrl`)**: Command-line interface for container management
- **Server (`catnip`)**: HTTP server with WebSocket support for PTY sessions
- **Container**: Docker-based development environment with pre-installed tools

## Configuration

The CLI automatically manages:
- Container lifecycle (start/stop/cleanup)
- Volume persistence for settings and state
- Network configuration for port forwarding
- Git credential management

## Troubleshooting

### Docker Issues
```bash
# Check Docker is running
docker info

# Verify permissions (Linux)
sudo usermod -aG docker $USER
```

### Port Conflicts
```bash
# Check what's using port 8080
lsof -i :8080

# Use different port
catctrl run --port 8081
```

### Container Cleanup
```bash
# Stop all catnip containers
docker stop $(docker ps -q --filter ancestor=catnip)

# Remove catnip containers
docker rm $(docker ps -aq --filter ancestor=catnip)
```

## Support

- 📖 [Documentation](https://github.com/wandb/catnip)
- 🐛 [Issues](https://github.com/wandb/catnip/issues)
- 💬 [Discussions](https://github.com/wandb/catnip/discussions)

## License

See [LICENSE](../LICENSE) file in the root of the repository.