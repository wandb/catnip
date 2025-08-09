# Testing Auto-Promotion Based on Tab Focus

## Summary of Changes

We've implemented automatic promotion of terminals from read-only to writeable based on tab focus:

### Frontend Changes:

1. **WorkspaceTerminal.tsx** and **terminal.$sessionId.tsx**:
   - Added `sendFocusState` function to send focus state to backend
   - Added event listeners for window focus, blur, and visibility change
   - Sends initial focus state when connection is established
   - Sends focus updates whenever tab gains or loses focus

### Backend Changes (pty.go):

1. **ConnectionInfo struct**:
   - Added `IsFocused` boolean field to track focus state

2. **handleFocusChange method**:
   - Updates focus state for the connection
   - Auto-promotes focused connections from read-only to write access
   - Demotes the current write connection when another gains focus
   - Sends read-only status updates to affected connections

3. **Control message handling**:
   - Added "focus" message type handling in the WebSocket message loop

## How It Works:

1. When a tab gains focus, the frontend sends `{ type: "focus", focused: true }`
2. Backend receives the focus event and:
   - If the focused connection is read-only, it promotes it to write access
   - The previous write connection is demoted to read-only
   - Both connections receive status updates
3. The UI updates the read-only badge accordingly

## Testing:

To test this feature:

1. Open the same terminal session in multiple tabs
2. Switch between tabs and observe that:
   - The focused tab automatically gets write access
   - The previously focused tab becomes read-only
   - The read-only badge appears/disappears automatically

The manual promotion (clicking the badge) is still available as a fallback.
