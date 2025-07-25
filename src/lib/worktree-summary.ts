// Removed completion imports - no longer using AI for summary generation

export interface WorktreeSummary {
  worktreeId: string;
  title: string;
  summary: string;
  status: "pending" | "generating" | "completed" | "error";
  error?: string;
  generatedAt?: Date;
}

export interface WorktreeDiffResponse {
  worktree_id: string;
  worktree_name: string;
  source_branch: string;
  fork_commit: string;
  file_diffs: FileDiff[];
  total_files: number;
  summary: string;
}

export interface FileDiff {
  file_path: string;
  change_type: string;
  old_content?: string;
  new_content?: string;
  diff_text?: string;
  is_expanded: boolean;
}

// Function to get diff data for a worktree
export async function getWorktreeDiff(
  worktreeId: string,
): Promise<WorktreeDiffResponse | null> {
  // Create abort controller for request timeout
  const controller = new AbortController();
  const timeoutId = setTimeout(() => {
    controller.abort();
  }, 30000); // 30 seconds timeout for diff requests

  try {
    const response = await fetch(`/v1/git/worktrees/${worktreeId}/diff`, {
      signal: controller.signal,
    });

    clearTimeout(timeoutId);

    if (!response.ok) {
      return null;
    }

    return (await response.json()) as WorktreeDiffResponse;
  } catch {
    clearTimeout(timeoutId);
    return null;
  }
}

// Function to generate a summary from diff data
export async function generateWorktreeSummary(
  worktreeId: string,
): Promise<WorktreeSummary> {
  const diffData = await getWorktreeDiff(worktreeId);

  if (!diffData?.file_diffs) {
    return {
      worktreeId,
      title: "",
      summary: "",
      status: "error",
      error: "Failed to fetch diff data",
    };
  }

  try {
    // Create a concise diff summary
    const diffSummary = createDiffSummary(diffData);

    // Generate a simple title and summary based on file changes (no AI needed)
    const title = generateSimpleTitle(diffData);
    const summary = generateSimpleSummary(diffData, diffSummary);

    return {
      worktreeId,
      title,
      summary,
      status: "completed",
      generatedAt: new Date(),
    };
  } catch (error) {
    console.error(
      `Failed to generate summary for worktree ${worktreeId}:`,
      error,
    );
    return {
      worktreeId,
      title: "Failed to generate summary",
      summary: "An error occurred while generating the summary",
      status: "error",
      error: error instanceof Error ? error.message : "Unknown error",
    };
  }
}

// Create a concise diff summary for the AI
function createDiffSummary(diffData: WorktreeDiffResponse): string {
  const { file_diffs, total_files, summary } = diffData;

  let diffSummary = `Summary: ${summary}\n`;
  diffSummary += `Total files changed: ${total_files}\n\n`;

  // Group files by change type
  const added = file_diffs.filter((f) => f.change_type === "added");
  const modified = file_diffs.filter((f) => f.change_type === "modified");
  const deleted = file_diffs.filter((f) => f.change_type === "deleted");

  if (added.length > 0) {
    diffSummary += `Added files (${added.length}):\n`;
    added.slice(0, 5).forEach((f) => {
      diffSummary += `  - ${f.file_path}\n`;
    });
    if (added.length > 5) {
      diffSummary += `  ... and ${added.length - 5} more\n`;
    }
    diffSummary += "\n";
  }

  if (modified.length > 0) {
    diffSummary += `Modified files (${modified.length}):\n`;
    modified.slice(0, 5).forEach((f) => {
      diffSummary += `  - ${f.file_path}\n`;
      // Include a snippet of the diff for context
      if (f.diff_text) {
        const lines = f.diff_text.split("\n").slice(0, 3);
        lines.forEach((line) => {
          if (line.startsWith("+") || line.startsWith("-")) {
            diffSummary += `    ${line}\n`;
          }
        });
      }
    });
    if (modified.length > 5) {
      diffSummary += `  ... and ${modified.length - 5} more\n`;
    }
    diffSummary += "\n";
  }

  if (deleted.length > 0) {
    diffSummary += `Deleted files (${deleted.length}):\n`;
    deleted.slice(0, 5).forEach((f) => {
      diffSummary += `  - ${f.file_path}\n`;
    });
    if (deleted.length > 5) {
      diffSummary += `  ... and ${deleted.length - 5} more\n`;
    }
  }

  return diffSummary;
}

// Function to check if a worktree should have a summary generated
export function shouldGenerateSummary(worktree: {
  repo_id: string;
  commit_count: number;
}): boolean {
  return worktree.repo_id.startsWith("local/") && worktree.commit_count > 0;
}

// Generate a simple title based on file changes (no AI needed)
function generateSimpleTitle(diffData: WorktreeDiffResponse): string {
  const { file_diffs, worktree_name } = diffData;

  if (file_diffs.length === 0) {
    return worktree_name || "No changes";
  }

  // Group files by change type
  const added = file_diffs.filter((f) => f.change_type === "added");
  const modified = file_diffs.filter((f) => f.change_type === "modified");
  const deleted = file_diffs.filter((f) => f.change_type === "deleted");

  // Generate title based on predominant change type
  if (added.length > modified.length && added.length > deleted.length) {
    if (added.length === 1) {
      return `Add ${added[0].file_path}`;
    }
    return `Add ${added.length} files`;
  } else if (
    deleted.length > modified.length &&
    deleted.length > added.length
  ) {
    if (deleted.length === 1) {
      return `Remove ${deleted[0].file_path}`;
    }
    return `Remove ${deleted.length} files`;
  } else if (modified.length > 0) {
    if (modified.length === 1 && added.length === 0 && deleted.length === 0) {
      return `Update ${modified[0].file_path}`;
    }
    return `Update ${file_diffs.length} files`;
  }

  return worktree_name || "Changes";
}

// Generate a simple summary based on file changes (no AI needed)
function generateSimpleSummary(
  diffData: WorktreeDiffResponse,
  _diffSummary: string,
): string {
  const { file_diffs, total_files, worktree_name } = diffData;

  if (file_diffs.length === 0) {
    return "No changes in this worktree.";
  }

  let summary = `## Changes in ${worktree_name}\n\n`;

  // Group files by change type
  const added = file_diffs.filter((f) => f.change_type === "added");
  const modified = file_diffs.filter((f) => f.change_type === "modified");
  const deleted = file_diffs.filter((f) => f.change_type === "deleted");

  summary += `**${total_files} file${total_files === 1 ? "" : "s"} changed**\n\n`;

  if (added.length > 0) {
    summary += `### Added (${added.length})\n`;
    added.slice(0, 3).forEach((f) => {
      summary += `- \`${f.file_path}\`\n`;
    });
    if (added.length > 3) {
      summary += `- ... and ${added.length - 3} more\n`;
    }
    summary += "\n";
  }

  if (modified.length > 0) {
    summary += `### Modified (${modified.length})\n`;
    modified.slice(0, 3).forEach((f) => {
      summary += `- \`${f.file_path}\`\n`;
    });
    if (modified.length > 3) {
      summary += `- ... and ${modified.length - 3} more\n`;
    }
    summary += "\n";
  }

  if (deleted.length > 0) {
    summary += `### Deleted (${deleted.length})\n`;
    deleted.slice(0, 3).forEach((f) => {
      summary += `- \`${f.file_path}\`\n`;
    });
    if (deleted.length > 3) {
      summary += `- ... and ${deleted.length - 3} more\n`;
    }
  }

  return summary;
}
