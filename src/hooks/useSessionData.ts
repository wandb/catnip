import { useState, useEffect } from "react";
import { useGitState } from "./useGitState";
import type { SessionCardData } from "@/types/session";

// Hook to manage session data combining worktrees and Claude sessions
export function useSessionData() {
  const [sessions, setSessions] = useState<SessionCardData[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  const {
    worktrees,
    claudeSessions,
    loading: gitLoading,
    refreshAll,
  } = useGitState();

  // Transform and combine data when dependencies change
  useEffect(() => {
    if (gitLoading) {
      setLoading(true);
      return;
    }

    try {
      const combinedSessions = combineWorktreesAndSessions(worktrees, claudeSessions);
      setSessions(combinedSessions);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load sessions");
    } finally {
      setLoading(false);
    }
  }, [worktrees, claudeSessions, gitLoading]);

  // Refresh sessions data
  const refreshSessions = () => {
    refreshAll();
  };

  return {
    sessions,
    loading,
    error,
    refreshSessions,
  };
}

// Combine worktrees with their Claude sessions
function combineWorktreesAndSessions(
  worktrees: any[],
  claudeSessions: Record<string, any>
): SessionCardData[] {
  return worktrees.map(worktree => {
    const claudeSession = claudeSessions[worktree.path];
    
    return {
      id: worktree.id,
      name: generateDisplayName(worktree, claudeSession),
      worktree,
      claudeSession,
      status: determineStatus(worktree, claudeSession),
      metrics: calculateMetrics(claudeSession),
      position: { x: 0, y: 0 }, // Default position
      generatedName: claudeSession?.generatedName,
    };
  });
}

// Generate a display name for the session
function generateDisplayName(worktree: any, claudeSession?: any): string {
  // Use generated name if available
  if (claudeSession?.generatedName) {
    return claudeSession.generatedName;
  }
  
  // Use worktree name or branch as fallback
  return worktree.name || worktree.branch || "Unnamed Session";
}

// Determine session status based on worktree and Claude session state
function determineStatus(worktree: any, claudeSession?: any): SessionCardData["status"] {
  if (!claudeSession) {
    return "finished";
  }
  
  if (claudeSession.isActive) {
    return "progress";
  }
  
  if (claudeSession.hasError) {
    return "errored";
  }
  
  if (claudeSession.waitingForInput) {
    return "needs_input";
  }
  
  return "finished";
}

// Calculate session metrics from Claude session data
function calculateMetrics(claudeSession?: any): SessionCardData["metrics"] {
  if (!claudeSession) {
    return {
      cost: 0,
      turns: 0,
      tokens: 0,
      duration: 0,
    };
  }
  
  return {
    cost: claudeSession.lastCost || 0,
    turns: claudeSession.turnCount || 0,
    tokens: claudeSession.tokenCount || 0,
    duration: calculateDuration(claudeSession.sessionStartTime, claudeSession.sessionEndTime),
  };
}

// Calculate duration in minutes
function calculateDuration(startTime?: string | Date, endTime?: string | Date): number {
  if (!startTime) return 0;
  
  const start = new Date(startTime);
  const end = endTime ? new Date(endTime) : new Date();
  
  return Math.round((end.getTime() - start.getTime()) / (1000 * 60));
}