# Catnip Development Container Feature

An agentic coding environment that transforms your development workflow with AI-powered assistance.

## Overview

Catnip is a comprehensive development environment featuring:

- **Intelligent Code Assistant**: AI-powered coding companion for enhanced productivity
- **Sandboxed Development**: Secure, isolated development environment
- **Modern Tech Stack**: React frontend with Go backend and Cloudflare Worker support
- **Seamless Integration**: Works out-of-the-box with GitHub Codespaces and VS Code

## Quick Start

Add this feature to your `.devcontainer/devcontainer.json`:

```json
{
  "features": {
    "ghcr.io/wandb/catnip:1": {}
  }
}
```

## Configuration Options

The feature supports several configuration options:

```json
{
  "features": {
    "ghcr.io/wandb/catnip:1": {
      "version": "latest",
      "enableAutoStart": true,
      "port": "3000"
    }
  }
}
```

### Available Options

- `version` (string): Specify the catnip version to install (default: `latest`)
- `enableAutoStart` (boolean): Automatically start catnip services on container start (default: `true`)
- `port` (string): Default port for the web interface (default: `3000`)

## What's Included

This feature installs and configures:

- Catnip binary and CLI tools
- Required dependencies and runtime environment
- Service auto-start configuration
- Development tools and utilities

## Usage

Once installed, you can:

1. **Start catnip**: The service starts automatically, or run `catnip` manually
2. **Access web interface**: Navigate to `http://localhost:3000` (or your configured port)
3. **Use CLI tools**: Access catnip commands directly from the terminal

## Requirements

- Docker or Podman with devcontainer support
- VS Code with Dev Containers extension or GitHub Codespaces
- Internet connection for initial setup

## Support

For issues or questions:

- [GitHub Issues](https://github.com/wandb/catnip/issues)
- [Documentation](https://github.com/wandb/catnip/docs)

---

Built with ❤️ for developers who want AI-powered coding assistance in their containers.
