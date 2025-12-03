# Interactive PTY Capture Tool

**Interactive** command-line tool to capture Claude sessions for Xcode previews. You can actually use Claude normally while recording!

## Features

✅ **Fully Interactive** - Use Claude naturally, see everything in real-time
✅ **Uses Your Config** - Reads your real ~/.claude configuration
✅ **Correct Dimensions** - Records at exact portrait (65x15) or landscape (120x30) size
✅ **Simple** - Just type, interact, and press Ctrl+C when done
✅ **Proper Cleanup** - Handles Ctrl+C gracefully without hanging
✅ **Perfect for Previews** - Optimized for iOS TerminalView rendering

## Quick Start

```bash
cd container

# Build
go build -o capture-pty cmd/capture-pty/main.go

# Capture (portrait mode - default)
./capture-pty

# Capture (landscape mode)
./capture-pty -landscape

# Custom output file
./capture-pty -output my-session.json
```

## How It Works

1. **Starts Claude** in a PTY with proper dimensions (65x15 portrait or 120x30 landscape)
2. **Uses Your Config** - Reads your real ~/.claude credentials and settings
3. **Interactive Mode** - You can type, navigate, use Claude normally
4. **Records Everything** - All output is captured with accurate timestamps
5. **Press Ctrl+C** - Gracefully stops, kills Claude, and saves JSON file

## Usage Example

```bash
$ ./capture-pty -output portrait-capture.json

🎬 Interactive PTY Capture Tool
📝 Output file: portrait-capture.json
📐 Dimensions: 65x15 (portrait)

✅ Using claude at: /Users/you/.claude/local/claude
✅ Using your real ~/.claude config
✅ Set terminal size to 65x15

🎮 Interactive Mode - You can now use Claude normally!
   • Type commands, interact with the TUI
   • Everything you see is being recorded
   • Press Ctrl+C when done to save

[Claude starts, you interact normally...]
[Go through onboarding, send messages, etc.]
[Press Ctrl+C when done]

🛑 Recording stopped

💾 Saving capture to portrait-capture.json...
✅ PTY capture saved successfully!
📊 Summary:
   - Dimensions: 65x15 (portrait)
   - Total bytes: 15234
   - Events: 42
   - Duration: 45.23s

🎯 To use in Xcode:
   1. cp portrait-capture.json ../xcode/catnip/PTYCapture/
   2. Add to Xcode project (if not already)
   3. Rebuild and view canvas!
```

## Options

```
-output string
    Output JSON file (default "pty-capture.json")

-landscape
    Use landscape dimensions (120x30) instead of portrait (65x15)
```

## Terminal Dimensions

The tool sets the correct terminal size for your target device:

- **Portrait (default)**: 65 cols × 15 rows
  - Matches `TerminalController.minCols` and `minRows`
  - Perfect for iPhone portrait mode

- **Landscape**: 120 cols × 30 rows
  - Wider layout for landscape orientation
  - Use with `-landscape` flag

## Tips

### Capture Ready Prompt

```bash
./capture-pty -output ready-prompt.json
# Claude will start in code mode (already authenticated)
# 1. Wait for the ">" prompt to appear
# 2. Maybe type a quick message to see response
# 3. Press Ctrl+C to save
```

### Capture Code Mode

```bash
./capture-pty -output code-mode.json
# 1. Claude starts (already authenticated)
# 2. Type a prompt: "help me fix this bug"
# 3. See Claude's response
# 4. Press Ctrl+C
```

### Capture Plan Mode

```bash
./capture-pty -output plan-mode.json
# 1. Claude starts
# 2. Press Shift+Tab to enter plan mode
# 3. Type a planning request
# 4. Press Ctrl+C
```

### Capture Landscape

```bash
./capture-pty -landscape -output landscape.json
# Same as above but with 120x30 dimensions
```

## What Gets Captured

- ✅ All terminal output (ANSI colors, cursor movements, etc.)
- ✅ Accurate timestamps for replay
- ✅ Proper terminal dimensions
- ❌ Your keyboard input (for privacy - only output is recorded)

## Key Features

- **Real Environment** - Uses your actual ~/.claude config and credentials
- **Proper Cleanup** - Ctrl+C kills Claude process and exits cleanly (no hanging!)
- **No Test Isolation** - Direct connection to Claude API, just like normal usage

## Output Format

JSON format compatible with Swift's `MockPTYDataSource`:

```json
{
  "captureDate": "2025-12-03T12:00:00Z",
  "totalBytes": 15234,
  "durationSeconds": 45.23,
  "events": [
    {
      "timestampMs": 0,
      "data": [27, 91, 50, 74, ...]
    }
  ]
}
```
