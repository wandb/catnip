import type { SessionCardData } from "@/types/session";

// Generate meaningful names for sessions using Claude completion API
export async function generateSessionName(session: SessionCardData): Promise<string> {
  try {
    const context = buildSessionContext(session);
    const prompt = buildNamePrompt(context);
    
    const response = await fetch('/v1/claude/completion', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        message: prompt,
        max_tokens: 50,
        system: "You are a helpful assistant that generates concise, meaningful names for coding sessions.",
      }),
    });

    if (!response.ok) {
      throw new Error(`API request failed: ${response.status}`);
    }

    const data = await response.json();
    return extractNameFromResponse(data.response);
  } catch (error) {
    console.error('Failed to generate session name:', error);
    return generateFallbackName(session);
  }
}

// Build context about the session for name generation
function buildSessionContext(session: SessionCardData): string {
  const { worktree, claudeSession, metrics } = session;
  
  const context = [
    `Branch: ${worktree.branch}`,
    `Repository: ${worktree.repo_id}`,
    `Commits: ${worktree.commit_count}`,
    `Status: ${session.status}`,
  ];

  if (metrics.turns > 0) {
    context.push(`Claude turns: ${metrics.turns}`);
  }

  if (worktree.is_dirty) {
    context.push("Has uncommitted changes");
  }

  if (worktree.commits_behind > 0) {
    context.push(`${worktree.commits_behind} commits behind`);
  }

  return context.join(', ');
}

// Build the prompt for name generation
function buildNamePrompt(context: string): string {
  return `Generate a concise, meaningful name (2-4 words) for this coding session based on the following context:

${context}

The name should be:
- Descriptive of what's being worked on
- Professional and clear
- No more than 4 words
- Use title case

Examples of good names:
- "User Auth Feature"
- "Database Migration"
- "Bug Fix Session"
- "API Refactor"

Just return the name, nothing else.`;
}

// Extract the name from Claude's response
function extractNameFromResponse(response: string): string {
  // Clean up the response - remove quotes, extra whitespace, etc.
  const cleaned = response
    .replace(/['"]/g, '') // Remove quotes
    .replace(/\n/g, ' ') // Replace newlines with spaces
    .trim();
  
  // Split into words and take first 4 to ensure brevity
  const words = cleaned.split(' ').slice(0, 4);
  
  // Capitalize each word (title case)
  return words
    .map(word => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join(' ');
}

// Generate fallback name when API fails
function generateFallbackName(session: SessionCardData): string {
  const { worktree, status } = session;
  
  // Try to create a meaningful name from available data
  if (worktree.branch.includes('feature')) {
    return 'Feature Development';
  }
  
  if (worktree.branch.includes('fix') || worktree.branch.includes('bug')) {
    return 'Bug Fix';
  }
  
  if (worktree.branch.includes('refactor')) {
    return 'Code Refactor';
  }
  
  if (status === 'progress') {
    return 'Active Session';
  }
  
  if (status === 'finished') {
    return 'Completed Work';
  }
  
  // Last resort fallback
  return worktree.branch || 'Code Session';
}

// Batch generate names for multiple sessions
export async function batchGenerateNames(sessions: SessionCardData[]): Promise<Record<string, string>> {
  const results: Record<string, string> = {};
  
  // Process in batches to avoid overwhelming the API
  const batchSize = 3;
  for (let i = 0; i < sessions.length; i += batchSize) {
    const batch = sessions.slice(i, i + batchSize);
    
    const promises = batch.map(async (session) => {
      const name = await generateSessionName(session);
      return { id: session.id, name };
    });
    
    const batchResults = await Promise.all(promises);
    batchResults.forEach(({ id, name }) => {
      results[id] = name;
    });
    
    // Small delay between batches to be respectful to the API
    if (i + batchSize < sessions.length) {
      await new Promise(resolve => setTimeout(resolve, 100));
    }
  }
  
  return results;
}