import { useXTerminalConnection } from "@/hooks/useXTerminalConnection";
import { ErrorDisplay } from "@/components/ErrorDisplay";
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
  const { ref, error, isReadOnly, shakeReadOnlyBadge, handlePromoteRequest } =
    useXTerminalConnection({
      worktree,
      terminalId,
      isFocused,
    });

  // Show error display if there's an error
  if (error) {
    return (
      <div className="h-full w-full bg-background flex items-center justify-center">
        <ErrorDisplay
          title={error.title}
          message={error.message}
          onRetry={() => {
            setError(null);
            window.location.reload();
          }}
        />
      </div>
    );
  }

  return (
    <div className="h-full w-full bg-black relative">
      {/* Read-only indicator */}
      {isReadOnly && (
        <div
          className={`absolute top-2 right-2 z-10 bg-yellow-600/20 border border-yellow-500/50 text-yellow-300 px-2 py-1 rounded-md text-xs font-medium backdrop-blur-sm cursor-pointer hover:bg-yellow-600/30 hover:border-yellow-500/70 transition-all duration-200 ${
            shakeReadOnlyBadge ? "animate-pulse animate-bounce" : ""
          }`}
          onClick={handlePromoteRequest}
          title="Click to request write access"
        >
          👁️ Read Only
        </div>
      )}
      {/* Terminal with minimal padding */}
      <div ref={ref} className="h-full w-full p-2" />
    </div>
  );
}
