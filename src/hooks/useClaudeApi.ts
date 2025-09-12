import { useMemo } from "react";
import { claudeApi } from "@/lib/claude-api";

export function useClaudeApi() {
  return useMemo(
    () => ({
      getAllWorktreeSessionSummaries: claudeApi.getAllWorktreeSessionSummaries,
      getWorktreeLatestAssistantMessage:
        claudeApi.getWorktreeLatestAssistantMessage,
      getWorktreeLatestMessageOrError:
        claudeApi.getWorktreeLatestMessageOrError,
    }),
    [],
  );
}
