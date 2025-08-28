# PTY Title Extraction

## Overview

Catnip intercepts terminal title changes from CLI tools to enable better integration with development environments like Claude Code. This is accomplished through a PTY (pseudo-terminal) wrapper system that transparently monitors escape sequences while preserving all terminal functionality.

## Architecture

### Core Components

1. **`catnip purr` command** (`container/internal/cmd/purr.go`)
   - PTY wrapper that executes commands while monitoring output
   - Intercepts terminal title escape sequences (`\x1b]0;title\x07`)
   - Logs title changes to `~/.catnip/title_events.log`
   - Preserves all terminal features (colors, cursor positioning, raw mode)

2. **Claude CLI Wrapper** (`/opt/catnip/bin/claude`)
   - Wrapper script that calls `catnip purr ~/.local/bin/claude`
   - Placed in `/opt/catnip/bin` which is first in PATH via devcontainer config
   - Ensures our wrapper is always used instead of the real Claude binary

### Title Log Format

```
timestamp|pid|cwd|title
2024-01-15T10:30:45.123Z|12345|/workspace/project|Chat started - Claude Code
```

## Implementation Details

### PTY Interception

The `purr` command creates a pseudo-terminal to run the target command:

1. Starts the command with `pty.Start(cmd)`
2. Puts stdin in raw mode to preserve terminal behavior
3. Handles window resize events (`SIGWINCH`)
4. Processes I/O concurrently:
   - Input: Raw copy from stdin to PTY
   - Output: Scans for title sequences while passing through unchanged

### Title Extraction

```go
titleRegex := regexp.MustCompile(`\x1b\]0;([^\x07]*)\x07`)
```

- Matches ANSI OSC (Operating System Command) sequence for setting window title
- Format: `ESC]0;title BEL` where ESC=`\x1b` and BEL=`\x07`
- Extracts title content and logs with timestamp, PID, and working directory

### Claude Integration

The wrapper setup ensures:

1. Claude CLI auto-updates work normally (updates `~/.local/bin/claude`)
2. Our wrapper always intercepts calls (via PATH priority)
3. No modification of the original Claude binary required
4. Title extraction works transparently for all Claude commands

## Environment Variables

- `CATNIP_TITLE_LOG`: Custom path for title log file (default: `~/.catnip/title_events.log`)
- `CATNIP_DISABLE_PTY_INTERCEPTOR`: Set to "1" or "true" to bypass interception

## Usage Examples

```bash
# Direct usage
catnip purr claude chat

# Through wrapper (automatic)
claude chat  # Actually calls: catnip purr ~/.local/bin/claude chat

# Custom log location
CATNIP_TITLE_LOG=/tmp/titles.log catnip purr some-command

# Disable interception
CATNIP_DISABLE_PTY_INTERCEPTOR=1 claude chat
```

## Benefits

1. **Transparent Operation**: No changes to user workflow or Claude CLI
2. **Full Compatibility**: Preserves all terminal features and escape sequences
3. **Update Safe**: Claude auto-updates don't affect our wrapper
4. **Development Integration**: Enables IDE features based on terminal context
5. **Debugging**: Title logs provide insight into command execution flow
