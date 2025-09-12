import { gitApi } from "./git-api";

export interface ClaudeSessionSummary {
  worktreePath: string;
  turnCount: number;
  header?: string;
  isActive: boolean;
  lastSessionId?: string;
  lastCost?: number;
  lastDuration?: number;
  lastTotalInputTokens?: number;
  lastTotalOutputTokens?: number;
  sessionStartTime?: string;
  sessionEndTime?: string;
  currentSessionId?: string;
  allSessions?: Array<{
    sessionId: string;
    lastModified: string;
    startTime?: string;
    endTime?: string;
    isActive: boolean;
  }>;
}

export interface ClaudeMessageOrError {
  content: string;
  isError: boolean;
}

export const claudeApi = {
  async getAllWorktreeSessionSummaries(): Promise<
    Record<string, ClaudeSessionSummary>
  > {
    return gitApi.fetchClaudeSessions();
  },

  async getWorktreeLatestAssistantMessage(
    worktreePath: string,
  ): Promise<string> {
    return gitApi.fetchWorktreeLatestAssistantMessage(worktreePath);
  },

  async getWorktreeLatestMessageOrError(
    worktreePath: string,
  ): Promise<ClaudeMessageOrError> {
    return gitApi.fetchWorktreeLatestMessageOrError(worktreePath);
  },
};
