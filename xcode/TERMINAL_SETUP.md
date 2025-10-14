# SwiftTerm Terminal Integration Setup

This guide explains how to complete the SwiftTerm integration for landscape terminal mode.

## What's Been Implemented

1. **PTYWebSocketManager** (`Services/PTYWebSocketManager.swift`)
   - WebSocket connection manager matching the backend PTY protocol
   - Handles all control messages (resize, input, focus, ready, prompt)
   - Automatic reconnection with exponential backoff
   - Full compatibility with the `/v1/pty` WebSocket endpoint

2. **TerminalView** (`Components/TerminalView.swift`)
   - SwiftUI wrapper for SwiftTerm's `LocalProcessTerminalView`
   - Integrates with PTYWebSocketManager
   - Handles terminal resize, input, and output streaming
   - Connection status indicator

3. **WorkspaceDetailView Integration** (`Views/WorkspaceDetailView.swift`)
   - Landscape orientation detection using size classes
   - Automatic switch between normal UI (portrait) and terminal (landscape)
   - WebSocket URL conversion (https → wss)

## Setup Instructions

### 1. Add SwiftTerm Package Dependency

Since manual editing of `.pbxproj` files is risky, please add SwiftTerm via Xcode:

1. Open `xcode/catnip.xcodeproj` in Xcode
2. Select the project in the navigator (top-level "catnip")
3. Select the "catnip" target
4. Go to the "Package Dependencies" tab
5. Click the "+" button
6. Enter the SwiftTerm repository URL:
   ```
   https://github.com/migueldeicaza/SwiftTerm
   ```
7. Select "Up to Next Major Version" with version `2.0.0` (or latest)
8. Click "Add Package"
9. Ensure "SwiftTerm" is checked for the "catnip" target
10. Click "Add Package"

### 2. Build and Test

1. Build the project (⌘B)
2. Run on a device or simulator
3. Navigate to a workspace
4. Rotate to landscape mode → Terminal should appear!
5. Rotate back to portrait → Normal workspace view returns

## How It Works

### Orientation Detection

The app uses SwiftUI's `horizontalSizeClass` and `verticalSizeClass` environment values:

```swift
// Landscape detection
let isLandscape = verticalSizeClass == .compact ||
    (horizontalSizeClass == .regular && verticalSizeClass == .compact)
```

### WebSocket Protocol

The terminal communicates with the backend using the same protocol as xterm.js:

**Control Messages (JSON, text frames):**

- `ready` - Client ready, triggers buffer replay
- `resize` - Terminal dimensions changed (cols/rows)
- `input` - User keyboard input
- `focus` - Terminal focus state
- `prompt` - Inject prompt into PTY

**PTY Output (binary frames):**

- Raw terminal output sent as binary data
- Fed directly to SwiftTerm for rendering

**Server Messages (JSON, text frames):**

- `read-only` - Terminal is in read-only mode
- `buffer-complete` - Buffer replay finished
- `buffer-size` - Buffered terminal dimensions

### Connection Flow

1. User rotates to landscape
2. `TerminalView` appears and creates `TerminalController`
3. `TerminalController` initializes SwiftTerm view
4. WebSocket connects to `wss://catnip.run/v1/pty?session={workspaceId}&agent=claude`
5. After connection, sends `ready` signal
6. Backend sends buffered output (if any)
7. Terminal becomes interactive
8. All user input is sent via WebSocket `input` messages
9. All PTY output is rendered in SwiftTerm

## Features

✅ Full terminal emulation with SwiftTerm
✅ WebSocket PTY connection matching backend protocol
✅ Automatic orientation detection
✅ Connection status indicator
✅ Automatic reconnection
✅ Terminal resize handling
✅ Works with Claude agent sessions

## Customization

### Change Terminal Font

Edit `TerminalView.swift`:

```swift
terminalView.font = UIFont.monospacedSystemFont(ofSize: 14, weight: .medium)
```

### Change WebSocket URL

For local development, edit `WorkspaceDetailView.swift`:

```swift
private var websocketBaseURL: String {
    return "ws://localhost:8080"  // Local dev
}
```

### Adjust Landscape Detection

Edit the `updateOrientation()` function in `WorkspaceDetailView.swift` to customize when the terminal appears.

## Troubleshooting

**Terminal doesn't appear in landscape:**

- Check that orientation is enabled in Info.plist
- Verify size class detection in console logs (`📱 Orientation changed`)

**WebSocket won't connect:**

- Check the WebSocket URL is correct (wss:// not https://)
- Verify backend is running and accessible
- Check console for connection errors (`❌ WebSocket receive error`)

**Terminal is black/blank:**

- Ensure SwiftTerm package is properly linked
- Check for errors in `TerminalController` initialization
- Verify data is being received (`📨 Received control message`)

## Architecture Diagram

```
┌─────────────────────────────────────┐
│   WorkspaceDetailView               │
│  ┌──────────────┐ ┌───────────────┐ │
│  │   Portrait   │ │  Landscape    │ │
│  │   (Normal    │ │  (Terminal)   │ │
│  │    UI)       │ │               │ │
│  └──────────────┘ └───────┬───────┘ │
└────────────────────────────┼─────────┘
                             │
                    ┌────────▼────────┐
                    │  TerminalView   │
                    │  (SwiftUI)      │
                    └────────┬────────┘
                             │
          ┌──────────────────┴──────────────────┐
          │                                     │
    ┌─────▼──────┐                    ┌────────▼────────┐
    │ SwiftTerm  │                    │ PTYWebSocket    │
    │ Terminal   │                    │ Manager         │
    │ (UI)       │                    │                 │
    └────────────┘                    └────────┬────────┘
                                               │
                                       ┌───────▼────────┐
                                       │  WebSocket     │
                                       │  wss://...     │
                                       └───────┬────────┘
                                               │
                                    ┌──────────▼─────────┐
                                    │  Backend PTY       │
                                    │  /v1/pty           │
                                    │  (pty.go)          │
                                    └────────────────────┘
```

## Next Steps

Consider adding:

- [ ] Terminal toolbar with copy/paste buttons
- [ ] Font size adjustment
- [ ] Color scheme selector
- [ ] Save terminal history
- [ ] Multi-window terminal support on iPad
