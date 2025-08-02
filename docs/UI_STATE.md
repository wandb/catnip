# UI State Management

This document explains the state management architecture in Catnip, how the frontend syncs with the backend through Server-Sent Events (SSE), and how to add new events to the system.

## Overview

Catnip uses a modern state management approach with **Zustand** for React state management and **Server-Sent Events (SSE)** for real-time updates from the backend. This eliminates the need for polling and provides instant UI updates when backend state changes.

## Architecture

### Frontend State Management (Zustand)

- **Store Location**: `src/stores/appStore.ts`
- **State Management**: Zustand with subscriptions middleware
- **Connection**: Auto-connecting SSE client with reconnection logic
- **Update Pattern**: Event-driven updates via SSE messages

### Backend Event System

- **Handler**: `container/internal/handlers/events.go`
- **Endpoint**: `/v1/events` (SSE endpoint)
- **Event Broadcasting**: In-memory pub/sub system for multiple clients
- **Heartbeat**: 5-second ping to maintain connection

## Event System

### Event Types

All events follow the pattern:

```typescript
interface AppEvent {
  type: EventType;
  payload: PayloadType;
}
```

Current event types:

#### Port Events

- `port:opened` - New service detected on a port
- `port:closed` - Service stopped on a port

#### Git Events

- `git:dirty` - Repository has uncommitted changes
- `git:clean` - Repository is clean

#### Process Events

- `process:started` - New process started
- `process:stopped` - Process terminated

#### System Events

- `container:status` - Container status change
- `heartbeat` - Keep-alive ping every 5 seconds

### Event Flow

1. **Backend Event**: Something happens (port opens, git changes, etc.)
2. **Event Emission**: Handler calls `eventsHandler.EmitXXX(...)` method
3. **SSE Broadcast**: Event is sent to all connected clients via SSE
4. **Frontend Update**: Zustand store receives event and updates state
5. **UI Refresh**: React components automatically re-render

## Adding New Events

### 1. Define Types (Frontend)

Add to `src/types/events.ts`:

```typescript
export interface MyNewEvent {
  type: 'my:new_event';
  payload: {
    id: string;
    data: any;
  };
}

// Add to union type
export type AppEvent =
  | PortOpenedEvent
  | PortClosedEvent
  | MyNewEvent  // <- Add here
  | ...;
```

### 2. Define Types (Backend)

Add to `container/internal/handlers/events.go`:

```go
const (
    // ... existing events
    MyNewEvent EventType = "my:new_event"
)

type MyNewEventPayload struct {
    ID   string `json:"id"`
    Data string `json:"data"`
}
```

### 3. Add State Handling (Frontend)

Update `src/stores/appStore.ts`:

```typescript
interface AppState {
  // ... existing state
  myNewData: Map<string, any>;

  // ... existing methods
}

// In the store implementation:
handleEvent: (event: AppEvent) => {
  switch (event.type) {
    // ... existing cases
    case "my:new_event":
      const newData = new Map(get().myNewData);
      newData.set(event.payload.id, event.payload.data);
      set({ myNewData: newData });
      break;
  }
};
```

### 4. Add Emission Method (Backend)

Add to `container/internal/handlers/events.go`:

```go
func (h *EventsHandler) EmitMyNewEvent(id string, data string) {
    h.broadcastEvent(AppEvent{
        Type: MyNewEvent,
        Payload: MyNewEventPayload{
            ID:   id,
            Data: data,
        },
    })
}
```

### 5. Emit Events from Backend Services

Call from any service when the event occurs:

```go
// In some service method
eventsHandler.EmitMyNewEvent("example-id", "example-data")
```

### 6. Use in React Components

```typescript
import { useAppStore } from '@/stores/appStore';

function MyComponent() {
  const { myNewData } = useAppStore();

  return (
    <div>
      {Array.from(myNewData.entries()).map(([id, data]) => (
        <div key={id}>{data}</div>
      ))}
    </div>
  );
}
```

## Connection Management

### Auto-reconnection

The SSE client automatically reconnects if the connection is lost:

- **Retry Delay**: 3 seconds
- **Error Handling**: Exponential backoff (future enhancement)
- **Status Indicator**: Connection status shown in navbar

### Heartbeat System

- **Interval**: 5 seconds
- **Purpose**: Detect dead connections and maintain NAT traversal
- **Payload**: Timestamp and uptime information

## Performance Considerations

### Zustand Benefits

- **Selective Updates**: Only subscribed components re-render
- **Minimal Overhead**: < 1KB bundle size
- **Type Safety**: Full TypeScript support
- **DevTools**: Redux DevTools compatible

### SSE Advantages

- **Real-time**: Instant updates vs polling delays
- **Efficient**: One persistent connection vs multiple HTTP requests
- **Reliable**: Built-in reconnection and error handling
- **Scalable**: Server can handle many concurrent connections

## Debugging

### Frontend Debugging

```typescript
// Enable detailed logging in store
console.log("SSE Event:", event);
```

### Backend Debugging

```go
// Add logging in events handler
log.Printf("Broadcasting event: %+v", event)
```

### Network Debugging

- Check Network tab in browser DevTools
- Look for `/v1/events` EventSource connection
- Verify SSE messages are being received

## Best Practices

1. **Event Granularity**: Keep events focused and specific
2. **Payload Size**: Keep payloads small for performance
3. **Error Handling**: Always handle event parsing errors
4. **Type Safety**: Use TypeScript interfaces for all events
5. **State Normalization**: Use Maps for keyed data collections
6. **Selective Updates**: Only update state when actually changed

## Migration from Polling

To migrate a component from polling to SSE:

1. Remove `useEffect` with `setInterval`
2. Remove local state management
3. Use Zustand store selectors
4. Ensure backend emits relevant events
5. Test real-time updates

## PTY Session Management

### TUI Buffer Replay System

For PTY sessions with TUI (Terminal User Interface) applications like Claude Code, we implement smart buffer replay to prevent UI state corruption on reconnection:

#### Problem

TUI applications use ANSI escape sequences to control terminal display. When a WebSocket reconnects, replaying the entire output buffer can cause duplicate UI elements (e.g., double input boxes) because the buffer contains both the final state and all intermediate drawing commands.

#### Solution

We detect when applications enter **alternate screen buffer mode** using ANSI sequence `\x1b[?1049h`:

```go
// In PTY output monitoring
if bytes.Contains(buf[:n], []byte("\x1b[?1049h")) {
    // Mark position where TUI content begins
    session.AlternateScreenActive = true
    session.LastNonTUIBufferSize = len(session.outputBuffer)
}
```

#### Buffer Replay Logic

- **For TUI sessions**: Only replay content up to alternate screen entry point
- **For regular terminals**: Replay entire buffer as before
- **Post-replay**: Send `Ctrl+L` to trigger TUI refresh

This ensures clean reconnections without duplicate interface elements while preserving command history that occurred before TUI activation.

## Future Enhancements

- **Event Filtering**: Client-side event filtering
- **Event Replay**: Replay missed events on reconnection
- **Event Persistence**: Store events for offline clients
- **Event Batching**: Batch multiple events for efficiency
- **Rate Limiting**: Prevent event spam
- **Event Compression**: Compress large payloads

## Type Synchronization

Currently, event types are manually synchronized between Go and TypeScript. Future improvements:

1. **Schema Generation**: Generate TypeScript from Go structs
2. **OpenAPI Events**: Document events in OpenAPI spec
3. **Code Generation**: Auto-generate event handlers
4. **Runtime Validation**: Validate event payloads at runtime
