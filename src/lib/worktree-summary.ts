import { getCompletion, createCompletionRequest } from './completion';

export interface WorktreeSummary {
  worktreeId: string;
  title: string;
  summary: string;
  status: 'pending' | 'generating' | 'completed' | 'error';
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
export async function getWorktreeDiff(worktreeId: string): Promise<WorktreeDiffResponse | null> {
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
    
    return await response.json() as WorktreeDiffResponse;
  } catch {
    clearTimeout(timeoutId);
    return null;
  }
}

// Function to generate a summary from diff data
export async function generateWorktreeSummary(worktreeId: string): Promise<WorktreeSummary> {
  const diffData = await getWorktreeDiff(worktreeId);
  
  if (!diffData) {
    return {
      worktreeId,
      title: 'Failed to generate summary',
      summary: 'Unable to fetch diff data',
      status: 'error',
      error: 'Failed to fetch diff data'
    };
  }

  try {
    // Create a concise diff summary for the AI
    const diffSummary = createDiffSummary(diffData);
    
    // Generate PR title with timeout and error handling
    const titleRequest = createCompletionRequest({
      message: `Generate a concise, descriptive pull request title for these changes. The title should be 10 words or less and clearly indicate what was changed or added. Focus on the main feature or fix.

Changes in ${diffData.worktree_name}:
${diffSummary}

Return only the title, no additional text or quotes.`,
      maxTokens: 50,
      system: 'You are a helpful assistant that generates concise, professional pull request titles based on code changes.'
    });

    // Generate PR body/summary with timeout and error handling
    const summaryRequest = createCompletionRequest({
      message: `Generate a comprehensive pull request description for these changes. Include:
1. A brief overview of what was changed
2. Any especially notable implementation details
3. Use bullet points for clarity

Changes in ${diffData.worktree_name}:
${diffSummary}

Format the response as a proper pull request description with clear sections.`,
      maxTokens: 500,
      system: 'You are a helpful assistant that generates professional pull request descriptions based on code changes. Focus on being clear, concise, and informative.'
    });

    // Generate both title and summary with Promise.allSettled for better error handling
    const [titleResult, summaryResult] = await Promise.allSettled([
      getCompletion({ request: titleRequest, cacheKey: `title-${worktreeId}` }),
      getCompletion({ request: summaryRequest, cacheKey: `summary-${worktreeId}` })
    ]);

    // Handle results with fallbacks
    let title = diffData.worktree_name || 'Changes';
    let summary = '';

    if (titleResult.status === 'fulfilled' && titleResult.value?.response) {
      title = titleResult.value.response.trim();
    }

    if (summaryResult.status === 'fulfilled' && summaryResult.value?.response) {
      summary = summaryResult.value.response.trim();
    }

    return {
      worktreeId,
      title,
      summary,
      status: 'completed',
      generatedAt: new Date()
    };
  } catch (error) {
    console.error(`Failed to generate summary for worktree ${worktreeId}:`, error);
    return {
      worktreeId,
      title: 'Failed to generate summary',
      summary: 'An error occurred while generating the summary',
      status: 'error',
      error: error instanceof Error ? error.message : 'Unknown error'
    };
  }
}

// Create a concise diff summary for the AI
function createDiffSummary(diffData: WorktreeDiffResponse): string {
  const { file_diffs, total_files, summary } = diffData;
  
  let diffSummary = `Summary: ${summary}\n`;
  diffSummary += `Total files changed: ${total_files}\n\n`;
  
  // Group files by change type
  const added = file_diffs.filter(f => f.change_type === 'added');
  const modified = file_diffs.filter(f => f.change_type === 'modified');
  const deleted = file_diffs.filter(f => f.change_type === 'deleted');
  
  if (added.length > 0) {
    diffSummary += `Added files (${added.length}):\n`;
    added.slice(0, 5).forEach(f => {
      diffSummary += `  - ${f.file_path}\n`;
    });
    if (added.length > 5) {
      diffSummary += `  ... and ${added.length - 5} more\n`;
    }
    diffSummary += '\n';
  }
  
  if (modified.length > 0) {
    diffSummary += `Modified files (${modified.length}):\n`;
    modified.slice(0, 5).forEach(f => {
      diffSummary += `  - ${f.file_path}\n`;
      // Include a snippet of the diff for context
      if (f.diff_text) {
        const lines = f.diff_text.split('\n').slice(0, 3);
        lines.forEach(line => {
          if (line.startsWith('+') || line.startsWith('-')) {
            diffSummary += `    ${line}\n`;
          }
        });
      }
    });
    if (modified.length > 5) {
      diffSummary += `  ... and ${modified.length - 5} more\n`;
    }
    diffSummary += '\n';
  }
  
  if (deleted.length > 0) {
    diffSummary += `Deleted files (${deleted.length}):\n`;
    deleted.slice(0, 5).forEach(f => {
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
  return worktree.repo_id.startsWith("local/") && worktree.commit_count > 1;
} 