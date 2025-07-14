import { toast } from "sonner";
import { Copy, Eye } from "lucide-react";
import { useGitOperations } from "@/hooks/useGitApi";

export interface ApiError {
  error: string;
  conflict_files?: string[];
  worktree_name?: string;
}

export const handleMergeConflict = (errorData: ApiError, operation: string) => {
  if (errorData.error === "merge_conflict") {
    const worktreeName = errorData.worktree_name;
    const conflictFiles = errorData.conflict_files || [];
    const sessionId = encodeURIComponent(worktreeName || "");
    const terminalUrl = `/terminal/${sessionId}`;
    
    const conflictText = conflictFiles.length > 0 
      ? `Conflicts in: ${conflictFiles.join(", ")}`
      : "Multiple files have conflicts";

    const claudePrompt = `I have a merge conflict during a ${operation} operation. ${conflictText}. Please help me resolve these conflicts by examining the files, understanding the conflicting changes, and providing a resolution strategy.`;

    return {
      title: `Merge Conflict in ${worktreeName}`,
      description: `${conflictText}\n\nOpen terminal to resolve: ${terminalUrl}\n\nSuggested Claude prompt: "${claudePrompt}"`
    };
  }
  return null;
};

export const copyRemoteCommand = (url: string) => {
  const command = `git remote add catnip ${url} && git fetch catnip`;
  navigator.clipboard.writeText(command);
  toast.success("Command copied to clipboard");
};

export const parseGitHubUrl = (url: string) => {
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

export const createPreviewToast = (branchName: string) => {
  const previewBranch = `preview/${branchName}`;
  const command = `git checkout ${previewBranch}`;

  return (
    <div className="flex items-center gap-2 w-full">
      <div className="flex-1">
        <div className="font-medium">Preview branch created!</div>
        <div className="text-sm text-muted-foreground mt-1">
          Run: <code className="bg-muted px-1 py-0.5 rounded text-xs">{command}</code>
        </div>
      </div>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          navigator.clipboard.writeText(command);
          const button = e.currentTarget;
          const originalContent = button.innerHTML;
          button.innerHTML = '<svg class="w-4 h-4 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>';
          setTimeout(() => {
            button.innerHTML = originalContent;
          }, 1000);
        }}
        className="p-1 hover:bg-muted rounded transition-colors"
        title="Copy command to clipboard"
      >
        <Copy className="w-4 h-4" />
      </button>
    </div>
  );
};

export const createPullRequestToast = (prData: any) => {
  return (
    <div className="flex items-center gap-2 w-full">
      <div className="flex-1">
        <div className="font-medium">Pull request created!</div>
        <div className="text-sm text-muted-foreground mt-1">
          PR #{prData.number}: {prData.title}
        </div>
      </div>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          window.open(prData.url, "_blank");
        }}
        className="p-1 hover:bg-muted rounded transition-colors"
        title="Open pull request"
      >
        <Eye className="w-4 h-4" />
      </button>
    </div>
  );
};