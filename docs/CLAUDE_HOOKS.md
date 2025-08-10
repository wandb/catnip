# Claude Hooks Integration for Improved Activity Tracking

This feature enhances Claude activity status tracking by using Claude Code's built-in hook system instead of relying on PTY output and file modification times.

## Overview

The integration works by:

1. **Hook Events**: Claude Code fires events like `UserPromptSubmit` and `Stop` during its lifecycle
2. **HTTP Endpoint**: Our catnip server receives these events via `/v1/claude/hooks`
3. **Precise Tracking**: We can now distinguish between:
   - **Active**: User just submitted a prompt (within 1 minute)
   - **Running**: Claude is processing or has recently finished (within 5 minutes)
   - **Inactive**: No recent activity (>5 minutes)

## Installation

### Prerequisites

- `jq` command-line JSON processor (usually pre-installed on most systems)
- `curl` for making HTTP requests

### Setup

**For Container Users (Recommended)**
Claude hooks are automatically installed and configured in the catnip container. No manual setup required!

**For Manual Installation**

1. Run the setup script to install the hooks:

   ```bash
   ./container/scripts/setup-claude-hooks.sh
   ```

2. The script will:
   - Create `~/.claude/catnip-activity-hook.sh`
   - Configure `~/.claude/settings.json` with hook definitions
   - Set up automatic event sending to catnip server

## How it Works

### Hook Script

The hook script (`container/scripts/claude-hooks.sh`) is called by Claude Code with JSON data via stdin containing:

- `hook_event_name`: Type of event (UserPromptSubmit, Stop, etc.)
- `cwd`: Current working directory
- `session_id`: Unique session identifier
- Additional event-specific data

### API Endpoint

The hook makes HTTP POST requests to `/v1/claude/hooks` with:

```json
{
  "event_type": "UserPromptSubmit",
  "working_directory": "/workspace/my-project"
}
```

### Activity State Logic

1. **UserPromptSubmit** → Marks session as Active immediately
2. **Stop** → Transitions from Active to Running state
3. **Timeout-based transitions** → Running to Inactive after 5+ minutes

## Benefits Over Previous Approach

- **More Accurate**: Hook events are fired exactly when Claude starts/stops
- **Real-time**: No delays from file monitoring or PTY parsing
- **Reliable**: Works even when PTY connections are closed
- **Clean**: Separates activity tracking from terminal output processing

## Configuration

### Custom Server Address

Set the `CATNIP_HOST` environment variable to use a different server:

```bash
export CATNIP_HOST=your-server:8080
```

### Debugging

To test the hook manually:

```bash
cd ~/.claude/hooks
echo '{"hook_event_name":"UserPromptSubmit","cwd":"/workspace/test","session_id":"test-123"}' | ./hook.sh
```

## Fallback Behavior

The system maintains backward compatibility:

- If no hook events are received, it falls back to PTY-based activity tracking
- Both methods can work together for maximum reliability

## Files Created

- `container/scripts/claude-hooks.sh`: The actual hook script that gets installed
- `container/scripts/setup-claude-hooks.sh`: Installation script
- `docs/CLAUDE_HOOKS.md`: This documentation
- New HTTP endpoint: `POST /v1/claude/hooks`
- Enhanced activity tracking in `ClaudeService` and `ClaudeMonitorService`
