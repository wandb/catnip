# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This project is called catnip and is meant to make the process of agentic coding fun and productive. It can be run as either a service on localhost, or in the cloud using CloudFlares
new [containers product](https://developers.cloudflare.com/containers). The project combines:

- **Frontend**: React/Vite SPA that uses SWC, tailwindcss, and ShadCN
- **Worker**: Hono-based Cloudflare Worker that manages container instances and WebSocket connections
- **Container**: Go application running in Docker that creates PTY sessions with bash.

### Frontend

`pnpm` is used for dependency management.

We should use [ShadCN components](https://ui.shadcn.com/docs/components) whenever most practical. New componets are added by running: `pnpm dlx shadcn@latest add button`. Styles can leverate tailwindcss and care should be taken for dark and light mode themes.

### Worker

The cloudflare worker is only relevant when we're running in production mode. In local and development mode we rely on vite. This is toggled with an env var CLOUDFLARE_DEV=true or the `"dev:cf"` script in package.json.

### Container

The container is a sandboxed coding environment. It's meant to support the most popular coding environments and developer tools. At its heart is a golang server that provides API's for running commands in the container or creating PTY's. We keep all endpoints well documented with OpenAPI specs generally using JSONRPC when possible.

## Features

Many of these have yet to be implemented, but here's the big vision:

1. ✅ PTY for full shell access via xterm
2. Credential persistence for Claude Code and the GH cli
3. Git checkout and git worktree use for editing multiple projects in parallel
4. HTTP git server for fetching changes made to different branches
5. Automatic port forwarding / proxy for services started in the container
6. Browser based MCP server that mimics the Pupeteer MCP server
7. Automatic log aggregation to make agentic debugging simpler
8. SSH server for remote vscode sessions
9. CLI for launching and syncing state with a server
10. Custom startup scripts for modifying the environment

## Reference

I've prototyped a number of these features in a folder named "reference". It could be useful to look at examples in this folder when implementing functionality.

## Development Tips

- Assume I'm running the dev server already which rebuilds go and the frontend
- You can exec in the dev container "catnip-dev", be sure to add the `--login` flag to bash
- Use shadcn theme variables as much as possible. You can add new ones in `index.css` if necessary.

## Operation Guidelines

- Don't restart the container unless explicitly asked to.

## Troubleshooting

- If you start getting no such file or directory, run `pwd` and get yourself into the root catnip directory