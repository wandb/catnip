import { useXTerminalConnection } from "@/hooks/useXTerminalConnection";
import type { Worktree } from "@/lib/git-api";

interface WorkspaceTerminalProps {
  worktree: Worktree;
  terminalId?: string;
  isFocused?: boolean;
}

export function WorkspaceTerminal({
  worktree,
  terminalId = "default",
  isFocused = true,
}: WorkspaceTerminalProps) {
  const {
    ref,
    error,
    isReadOnly,
    shakeReadOnlyBadge,
    handlePromoteRequest,
    isConnected,
    isConnecting,
    handleRetryConnection,
    terminalContainerRef,
    handleTerminalFocus,
    isTerminalFocused: _isTerminalFocused,
  } = useXTerminalConnection({
    worktree,
    terminalId,
    isFocused,
  });

  return (
    <div
      ref={terminalContainerRef}
      className="h-full w-full bg-black relative"
      onMouseDown={handleTerminalFocus}
      onFocus={handleTerminalFocus}
      tabIndex={-1}
    >
      {/* Connection status, error, and read-only indicators in upper right */}
      <div className="absolute top-2 right-2 z-10 flex flex-col gap-1 items-end">
        {/* Error indicator */}
        {error && (
          <div className="bg-red-600/20 border border-red-500/50 text-red-300 px-3 py-2 rounded-md text-xs font-medium backdrop-blur-sm flex flex-col gap-2 max-w-xs">
            <div className="font-semibold">{error.title}</div>
            <div className="text-xs text-red-200">{error.message}</div>
            <button
              onClick={handleRetryConnection}
              className="self-end bg-red-600/30 hover:bg-red-600/50 border border-red-500/50 hover:border-red-500/70 text-red-200 px-2 py-1 rounded text-xs transition-all duration-200"
            >
              🔄 Retry
            </button>
          </div>
        )}

        {/* Connection status indicator - show connecting or disconnected state */}
        {!isConnected && !error && isConnecting && (
          <div className="bg-amber-600/20 border border-amber-500/50 text-amber-300 px-2 py-1 rounded-md text-xs font-medium backdrop-blur-sm">
            🔄 Connecting...
          </div>
        )}

        {/* Disconnected status indicator */}
        {!isConnected && !error && !isConnecting && (
          <div
            className="bg-red-600/20 border border-red-500/50 text-red-300 px-2 py-1 rounded-md text-xs font-medium backdrop-blur-sm cursor-pointer hover:bg-red-600/30 hover:border-red-500/70 transition-all duration-200"
            onClick={handleRetryConnection}
          >
            ⚠️ Disconnected - Click to retry
          </div>
        )}

        {/* Read-only indicator - only show when connected */}
        {isConnected && isReadOnly && (
          <div
            className={`bg-yellow-600/20 border border-yellow-500/50 text-yellow-300 px-2 py-1 rounded-md text-xs font-medium backdrop-blur-sm cursor-pointer hover:bg-yellow-600/30 hover:border-yellow-500/70 transition-all duration-200 ${
              shakeReadOnlyBadge ? "animate-pulse animate-bounce" : ""
            }`}
            onClick={handlePromoteRequest}
            title="Click to request write access"
          >
            👁️ Read Only
          </div>
        )}
      </div>

      {/* Terminal with minimal padding */}
      <div ref={ref} className="h-full w-full p-2" />
    </div>
  );
}
