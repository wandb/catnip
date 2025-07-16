# Catnip Project Summary

## Overview
Catnip is an agentic coding platform designed to make coding fun and productive. It can run locally or in the cloud using Cloudflare containers and consists of three main components:

- **Frontend**: React/Vite SPA with SWC, Tailwind CSS, and ShadCN components
- **Worker**: Hono-based Cloudflare Worker for container management and WebSocket connections
- **Container**: Go application in Docker providing PTY sessions and bash access

## Key Technologies
- **Frontend**: React, Vite, pnpm, ShadCN UI, Tailwind CSS
- **Backend**: Go, Docker, OpenAPI/Swagger, JSONRPC
- **Infrastructure**: Cloudflare Workers, WebSockets

## Architecture
```
Frontend (React/Vite) ↔ Worker (Cloudflare) ↔ Container (Go/Docker)
```

## Current Features
- ✅ PTY shell access via xterm
- Port detection and preview system with iframe support
- Automatic reverse proxy for services
- Real-time port monitoring
- Claude Completion API integration

## Planned Features
- Credential persistence for Claude Code and GitHub CLI
- Git worktree support for parallel project editing
- HTTP git server for branch changes
- Port forwarding/proxy automation
- Browser-based MCP server (Puppeteer-like)
- Log aggregation for debugging
- SSH server for remote VS Code
- CLI for server state management
- Custom startup scripts

## Development Environment
- Uses `pnpm` for dependency management
- Dev server auto-rebuilds Go and frontend
- Container name: `catnip-dev`
- Development port: `localhost:8080`
- Build command: `just build` (in container directory)
- Swagger docs: `just swagger`

## Directory Structure
- `src/` - Frontend React application
- `container/` - Go Docker application
- `worker/` - Cloudflare Worker
- `docs/` - Project documentation
- `reference/` - Feature prototypes

## API Integration
Includes Claude Completion API with:
- POST `/v1/claude/completion` endpoint
- Support for context, system prompts, and token limits
- Usage tracking and error handling
- Requires `ANTHROPIC_API_KEY` environment variable

## Development Guidelines
- Use ShadCN components when practical
- Follow existing code patterns and conventions
- Don't restart containers unless explicitly asked
- Use theme variables for consistent styling
- Prefer editing existing files over creating new ones