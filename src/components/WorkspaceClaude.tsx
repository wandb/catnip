import { useXTerminalConnection } from "@/hooks/useXTerminalConnection";
import { ErrorDisplay } from "@/components/ErrorDisplay";
import type { Worktree } from "@/lib/git-api";

interface WorkspaceClaudeProps {
  worktree: Worktree;
  isFocused: boolean;
}

export function WorkspaceClaude({ worktree, isFocused }: WorkspaceClaudeProps) {
  const { ref, error, isReadOnly, shakeReadOnlyBadge, handlePromoteRequest } =
    useXTerminalConnection({
      worktree,
      isFocused,
      agent: "claude",
      enableAdvancedBuffering: true,
    });

  // Show error display if there's an error
  if (error) {
    return (
      <div className="h-full w-full bg-background flex items-center justify-center">
        <ErrorDisplay
          title={error.title}
          message={error.message}
          onRetry={() => {
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
          className={`absolute top-4 right-4 z-10 bg-yellow-600/20 border border-yellow-500/50 text-yellow-300 px-3 py-1 rounded-md text-sm font-medium backdrop-blur-sm cursor-pointer hover:bg-yellow-600/30 hover:border-yellow-500/70 transition-all duration-200 ${
            shakeReadOnlyBadge ? "animate-pulse animate-bounce" : ""
          }`}
          onClick={handlePromoteRequest}
          title="Click to request write access"
        >
          👁️ Read Only
        </div>
      )}
      {/* Terminal */}
      <div className="h-full w-full p-4">
        <div ref={ref} className="h-full w-full" />
      </div>
    </div>
  );
}
