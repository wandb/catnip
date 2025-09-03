# Mock Server for Catnip Frontend Development

This mock server allows the frontend to run independently without the actual Catnip backend container.

## Features

- Implements all `/v1/*` API endpoints from `container/docs/swagger.json`
- Provides Server-Sent Events (SSE) endpoint at `/v1/events` with mock data
- Simulates real-time events (heartbeat, port changes, git status, worktree updates)
- Supports both streaming and non-streaming Claude message responses
- Includes git diff, PR, sync, merge, and other advanced git operations
- Provides mock worktrees with proper repository linkage for app navigation

## Usage

### Quick Start

Run the frontend with mock server:

```bash
pnpm run dev:mock
```

This command will:

1. Start Vite dev server on `$PORT` (defaults to 5173)
2. Start the mock server on `$PORT + 1` (defaults to 5174)
3. Configure proxy to redirect `/v1/*` requests to the mock server

In the Catnip environment, it automatically uses the `PORT` environment variable:

```bash
# Uses PORT from environment (e.g., in Catnip container)
PORT=6369 pnpm run dev:mock  # Vite on 8080, mock server on 8081
```

### Run Separately

Start mock server only:

```bash
pnpm run mock:server
```

Start Vite with mock proxy enabled:

```bash
pnpm run dev:with-mock
```

### Configuration

- **Automatic Port Configuration**: When using `pnpm run dev:mock`:
  - Vite runs on `$PORT` (or 5173 if not set)
  - Mock server runs on `$PORT + 1` (or 5174 if not set)
- **Manual Configuration**: When running separately:

  ```bash
  MOCK_PORT=3002 pnpm run mock:server
  VITE_PORT=5173 MOCK_PORT=3002 VITE_USE_MOCK=true vite
  ```

- **Vite Proxy**: When `VITE_USE_MOCK=true`, Vite will proxy all `/v1/*` requests to the mock server on `$MOCK_PORT`

## Endpoints

The mock server implements the following endpoint groups:

- **Auth** (`/v1/auth/github/*`): GitHub authentication flow
- **Claude** (`/v1/claude/*`): Claude Code sessions, settings, messages, todos
- **Git** (`/v1/git/*`): Worktrees, branches, status, checkout
- **Ports** (`/v1/ports/*`): Port management and mappings
- **Sessions** (`/v1/sessions/*`): PTY session management
- **Events** (`/v1/events`): Server-Sent Events stream
- **Notifications** (`/v1/notifications`): System notifications
- **Upload** (`/v1/upload`): File upload endpoint

## SSE Events

The `/v1/events` endpoint provides real-time updates via Server-Sent Events:

- **Heartbeat**: Every 5 seconds with timestamp and uptime
- **Container Status**: Initial connection status
- **Port Events**: When ports are opened/closed
- **Git Events**: Repository status changes (clean/dirty)
- **Process Events**: Process start/stop notifications
- **Random Events**: Simulated activity every 15 seconds

## Mock Data

The server provides realistic mock data including:

- Active worktrees with git status
- Claude session summaries with metrics
- Port mappings for common development servers
- GitHub repository listings

## Customization

Edit `mock-server.js` to:

- Modify mock data responses
- Add new endpoints
- Change event simulation behavior
- Adjust timing intervals

## Troubleshooting

- If port 3001 is in use, set `MOCK_PORT` to a different port
- Check console output for unhandled routes
- SSE events can be tested at `http://localhost:3001/v1/events`
