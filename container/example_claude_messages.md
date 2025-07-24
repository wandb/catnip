# Claude Messages API Examples

The new `/v1/claude/messages` endpoint provides claude CLI subprocess integration with streaming and resume support.

## Basic Usage

### Non-streaming request:

```bash
curl -X POST http://localhost:3001/v1/claude/messages \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Help me debug this Go error",
    "stream": false,
    "working_directory": "/workspace/my-project"
  }'
```

### Streaming request:

```bash
curl -X POST http://localhost:3001/v1/claude/messages \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Help me debug this Go error",
    "stream": true,
    "working_directory": "/workspace/my-project"
  }'
```

### Resume previous session:

```bash
curl -X POST http://localhost:3001/v1/claude/messages \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Continue where we left off",
    "resume": true,
    "working_directory": "/workspace/my-project"
  }'
```

## Advanced Options

### With custom system prompt and model:

```bash
curl -X POST http://localhost:3001/v1/claude/messages \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Review this code for best practices",
    "system_prompt": "You are an expert Go developer focused on clean code",
    "model": "claude-3-5-sonnet-20241022",
    "max_turns": 5,
    "working_directory": "/workspace/my-project"
  }'
```

### Default working directory:

If `working_directory` is not specified, it defaults to `/workspace/current`:

```bash
curl -X POST http://localhost:3001/v1/claude/messages \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "What files are in this project?",
    "stream": false
  }'
```

## Request Parameters

- `prompt` (required): The message/prompt to send to claude
- `stream` (optional): Boolean, whether to stream the response to client (default: false)
- `resume` (optional): Boolean, whether to resume the most recent session (default: false)
- `system_prompt` (optional): Custom system prompt override
- `model` (optional): Model to use (default: claude-3-5-sonnet-20241022)
- `max_turns` (optional): Maximum conversation turns (default: 10)
- `working_directory` (optional): Working directory for claude (default: /workspace/current)

## Streaming Behavior

- **stream: true** - Server streams JSON chunks to client as they arrive from claude CLI
- **stream: false** - Server accumulates all chunks internally and sends a single response when complete
- **Internal processing** - Always uses `stream-json` format for both input and output to claude CLI

## Resume Logic

When `resume: true` is specified:

1. The system resolves the `working_directory` (including symlinks like `/workspace/current`)
2. Converts the path to claude's project directory format (e.g., `/workspace/catnip/simba` → `-workspace-catnip-simba`)
3. Looks for the most recent `.jsonl` session file in `~/.claude/projects/-workspace-catnip-simba/`
4. Extracts the session UUID from the filename
5. Passes `--resume {session-uuid}` to the claude CLI

## Command Line Arguments Used

The subprocess wrapper calls claude CLI with these arguments:

- `claude -p` (for prompt mode)
- `--output-format stream-json` (always, for consistent streaming)
- `--input-format stream-json` (always, for consistent streaming)
- `--verbose` (required when using stream-json with -p)
- `--system-prompt "{prompt}"` (if provided)
- `--model "{model}"` (if provided)
- `--max-turns {number}` (if provided)
- `--resume {session-id}` (if resume is true and session found)

**Important**: The prompt is sent via **stdin** as JSON, not as a command argument:

```json
{ "type": "user", "message": { "role": "user", "content": "Your prompt here" } }
```

**Note**: The API always uses `stream-json` format internally. For non-streaming requests (`stream: false`), the server accumulates all streaming chunks before sending the final response to the client.
