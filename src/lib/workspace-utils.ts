import type { Worktree } from "@/lib/git-api";

export function getWorkspaceTitle(worktree: Worktree): string {
  // Priority 1: Use PR title if available
  if (worktree.pull_request_title) {
    return worktree.pull_request_title;
  }

  // Priority 2: Use session title if available
  if (worktree.session_title?.title) {
    return worktree.session_title.title;
  }

  // Priority 3: Use latest user prompt if available (requires backend support)
  // Note: This field doesn't exist yet but would come from ~/.claude.json history
  if ((worktree as any).latest_user_prompt) {
    return (worktree as any).latest_user_prompt;
  }

  // Priority 4: Use workspace name
  const workspaceName = worktree.name.split("/")[1] || worktree.name;
  return workspaceName;
}

export function getWorktreeStatus(worktree: Worktree) {
  // Use the claude_activity_state to determine the status
  switch (worktree.claude_activity_state) {
    case "active":
      return { color: "bg-gray-500", label: "active" };
    case "running":
      return { color: "bg-green-500 animate-pulse", label: "running" };
    case "inactive":
    default:
      return {
        color: "border-2 border-gray-400 bg-transparent",
        label: "inactive",
      };
  }
}

export function getStatusIndicatorClasses(worktree?: Worktree): string {
  if (!worktree)
    return "h-2 w-2 border border-gray-400 bg-transparent rounded-full flex-shrink-0";

  switch (worktree.claude_activity_state) {
    case "active":
      return "h-2 w-2 bg-green-500 rounded-full animate-pulse flex-shrink-0";
    case "running":
      return "h-2 w-2 bg-gray-500 rounded-full flex-shrink-0";
    case "inactive":
    default:
      return "h-2 w-2 border border-gray-400 bg-transparent rounded-full flex-shrink-0";
  }
}
