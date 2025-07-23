import { toast } from "sonner";
import { Copy } from "lucide-react";

export const getRelativeTime = (date: string | Date) => {
  const now = new Date();
  const then = new Date(date);
  const diffMs = now.getTime() - then.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return "just now";
  if (diffMins < 60)
    return `${diffMins} minute${diffMins !== 1 ? "s" : ""} ago`;
  if (diffHours < 24)
    return `${diffHours} hour${diffHours !== 1 ? "s" : ""} ago`;
  return `${diffDays} day${diffDays !== 1 ? "s" : ""} ago`;
};

export const getDuration = (
  startDate: string | Date,
  endDate: string | Date,
) => {
  const start = new Date(startDate);
  const end = new Date(endDate);
  const diffMs = end.getTime() - start.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);

  if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? "s" : ""}`;
  if (diffHours < 24) {
    return `${diffHours} hour${diffHours !== 1 ? "s" : ""} ${diffMins % 60} minute${
      diffMins % 60 !== 1 ? "s" : ""
    }`;
  }
  return `${Math.floor(diffHours / 24)} day${Math.floor(diffHours / 24) !== 1 ? "s" : ""}`;
};

export const copyRemoteCommand = (url: string) => {
  const command = `git remote add catnip ${url} && git fetch catnip`;
  void navigator.clipboard.writeText(command);
  toast.success("Command copied to clipboard");
};

export const showPreviewToast = (branchName: string) => {
  const command = `git checkout preview/${branchName}`;

  toast.success(
    <div className="flex items-center gap-2 w-full">
      <div className="flex-1">
        <div className="font-medium">Preview branch created!</div>
        <div className="text-sm text-muted-foreground mt-1">
          Run:{" "}
          <code className="bg-muted px-1 py-0.5 rounded text-xs">
            {command}
          </code>
        </div>
      </div>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          void navigator.clipboard.writeText(command);

          // Show brief success feedback
          const button = e.currentTarget;
          const originalContent = button.innerHTML;
          button.innerHTML =
            '<svg class="w-4 h-4 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>';

          setTimeout(() => {
            button.innerHTML = originalContent;
          }, 1000);
        }}
        className="p-1 hover:bg-muted rounded transition-colors"
        title="Copy command to clipboard"
      >
        <Copy className="w-4 h-4" />
      </button>
    </div>,
    {
      duration: 8000,
    },
  );
};

export const parseGitUrl = (url: string) => {
  if (url.startsWith("local/")) {
    const parts = url.split("/");
    if (parts.length >= 2) {
      return { type: "local", org: parts[0], repo: parts[1] };
    }
  } else if (url.startsWith("https://github.com/")) {
    const urlParts = url.replace("https://github.com/", "").split("/");
    if (urlParts.length >= 2) {
      const [org, repoWithGit] = urlParts;
      const repo = repoWithGit.replace(/\.git$/, "");
      return { type: "github", org, repo };
    }
  }
  return null;
};

export const createMergeConflictPrompt = (
  operation: string,
  conflictFiles: string[],
) => {
  const conflictText =
    conflictFiles.length > 0
      ? `Conflicts in: ${conflictFiles.join(", ")}`
      : "Multiple files have conflicts";

  return `I have a merge conflict during a ${operation} operation. ${conflictText}. Please help me resolve these conflicts by examining the files, understanding the conflicting changes, and providing a resolution strategy.`;
};

/**
 * Extracts a clean display name from a branch name by removing the catnip/ prefix
 * Examples:
 * - "catnip/felix" -> "felix"
 * - "catnip/fuzzy-luna" -> "fuzzy-luna"
 * - "main" -> "main" (unchanged)
 * - "feature/something" -> "feature/something" (unchanged)
 */
export const extractDisplayName = (branchName: string): string => {
  if (branchName.startsWith("catnip/")) {
    return branchName.replace("catnip/", "");
  }
  return branchName;
};
