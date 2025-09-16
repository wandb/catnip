# Catnip Development Container Feature

**The developer environment that's like catnip for agentic programming.**

Perfect for getting the most out of coding agents like Claude Code. Catnip helps you stay organized, run many agents in parallel, and operate agents on the go with our mobile interface.

## Overview

Catnip transforms your devcontainer into a multi-agent coding workspace featuring:

- **🔒 Isolated Sandbox**: Safe environment where AI assistants can run commands without risk
- **🧑‍💻 Worktree Management**: Parallel development with automatic Git worktree handling
- **📱 Mobile Interface**: Access and monitor your codespaces from anywhere via [catnip.run](https://catnip.run)
- **💻 Full Terminal Access**: Multiple web-based terminals plus SSH support
- **👀 Live Preview**: Built-in proxy with automatic port detection for web services
- **🌐 Universal Access**: Works with VS Code, Cursor, and other remote development tools

## Quick Start

Add this feature to your `.devcontainer/devcontainer.json` and include port forwarding:

```json
{
  "features": {
    "ghcr.io/wandb/catnip/feature:1": {}
  },
  "forwardPorts": [6369]
}
```

> [!TIP]
> Why 6369 you might ask? Type C A T S on your telephone keypad and you'll find the answer 🤓

## Configuration Options

The feature supports several configuration options:

```json
{
  "features": {
    "ghcr.io/wandb/catnip/feature:1": {
      "ensureSsh": true,
      "installCatnip": true,
      "installClaude": true,
      "installGh": true,
      "debug": false
    }
  }
}
```

### Available Options

- `ensureSsh` (boolean): Install and run openssh-server and openssh-client if missing (default: `true`)
- `installCatnip` (boolean): Install the catnip binary and services (default: `true`)
- `installClaude` (boolean): Install the Claude CLI for AI assistance (default: `true`)
- `installGh` (boolean): Install GitHub CLI for seamless Git integration (default: `true`)
- `debug` (boolean): Enable debug mode for enhanced logging and troubleshooting (default: `false`)

## What's Included

This feature automatically installs and configures:

- **Catnip binary and services**: Core catnip application with auto-start configuration
- **Claude CLI**: Official Anthropic CLI for AI assistance (if `installClaude: true`)
- **GitHub CLI**: Official GitHub CLI for seamless Git operations (if `installGh: true`)
- **SSH server/client**: Secure shell access for remote development (if `ensureSsh: true`)
- **VS Code extensions**:
  - `anthropic.claude-code` - Official Claude Code extension
  - `wandb.catnip-sidebar` - Catnip sidebar integration

## Usage

Once your devcontainer starts:

1. **Access the web interface**: Navigate to `http://localhost:6369` or click the cat logo in the VS Code sidebar
2. **Use from mobile**: Visit [catnip.run](https://catnip.run), login with GitHub, and we'll connect you to your codespace
3. **SSH access**: Use the configured SSH connection for remote development with Cursor or other editors
4. **CLI tools**: Access `catnip`, `claude`, and `gh` commands directly from any terminal

## Mobile Access

The mobile interface at [catnip.run](https://catnip.run) provides full access to your codespace:

- **Automatic startup**: We'll start your codespace if it's not running
- **Secure access**: Uses encrypted, temporary credentials that expire after 24 hours
- **Real-time sync**: Monitor multiple agents working in parallel
- **Touch-friendly**: Optimized interface for coding on mobile devices

## Environment Setup

Catnip automatically looks for a `setup.sh` file in your repository root and runs it during workspace creation. Perfect for installing dependencies:

```bash
#!/bin/bash
# Example setup.sh for AI/ML projects
pip install -r requirements.txt
npm install
# Pre-load common AI/ML dependencies
pip install openai anthropic chromadb langchain
```

## Git Integration

Catnip creates isolated Git worktrees for each workspace, enabling:

- **Parallel development**: Multiple AI agents working simultaneously without conflicts
- **Automatic commits**: Changes are committed as agents make them
- **Clean branches**: Nice feature branch names are maintained alongside worktree refs
- **Easy review**: Check out feature branches locally to review AI-generated changes

## Requirements

- GitHub Codespaces or local devcontainer support
- VS Code with Dev Containers extension (for local development)
- Internet connection for setup and mobile access

## Support

For questions or issues:

- [GitHub Repository](https://github.com/wandb/catnip)
- [GitHub Issues](https://github.com/wandb/catnip/issues)
- [Main Documentation](https://github.com/wandb/catnip/blob/main/README.md)

---

**Made with ❤️ by the [Weights & Biases](https://wandb.ai) team**
