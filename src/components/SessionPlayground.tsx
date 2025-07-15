import { useState } from "react";
import { SessionCard } from "./SessionCard";
import { SessionGrid } from "./SessionGrid";
import { useSessionData } from "@/hooks/useSessionData";
import { ErrorAlert } from "./ErrorAlert";
import { LoadingSpinner } from "./LoadingSpinner";
import type { SessionCardData } from "@/types/session";

// Main playground component that orchestrates the session cards
export function SessionPlayground() {
  const { sessions, loading, error, refreshSessions } = useSessionData();
  const [errorAlert, setErrorAlert] = useState({
    open: false,
    title: "",
    description: "",
  });

  // Show loading state
  if (loading) {
    return <LoadingSpinner />;
  }

  // Show error state
  if (error) {
    return (
      <ErrorAlert
        open={true}
        onOpenChange={() => {}}
        title="Failed to load sessions"
        description={error}
      />
    );
  }

  // Transform session data for cards
  const sessionCards = transformSessionsToCards(sessions);

  return (
    <>
      <SessionGrid 
        sessions={sessionCards}
        onRefresh={refreshSessions}
        onError={setErrorAlert}
      />
      
      {/* Error Alert */}
      <ErrorAlert
        open={errorAlert.open}
        onOpenChange={(open) => setErrorAlert(prev => ({ ...prev, open }))}
        title={errorAlert.title}
        description={errorAlert.description}
      />
    </>
  );
}

// Transform raw session data into card format
function transformSessionsToCards(sessions: any[]): SessionCardData[] {
  return sessions.map(session => ({
    id: session.id,
    name: session.generatedName || session.name || "Unnamed Session",
    worktree: session.worktree,
    claudeSession: session.claudeSession,
    status: determineSessionStatus(session),
    metrics: {
      cost: session.claudeSession?.lastCost || 0,
      turns: session.claudeSession?.turnCount || 0,
      tokens: session.claudeSession?.tokenCount || 0,
      duration: calculateDuration(session.claudeSession),
    },
    position: session.position || { x: 0, y: 0 },
  }));
}

// Determine the current status of a session
function determineSessionStatus(session: any): "progress" | "finished" | "errored" | "needs_input" {
  if (!session.claudeSession) return "finished";
  
  if (session.claudeSession.isActive) {
    return "progress";
  }
  
  if (session.claudeSession.hasError) {
    return "errored";
  }
  
  if (session.claudeSession.waitingForInput) {
    return "needs_input";
  }
  
  return "finished";
}

// Calculate session duration in minutes
function calculateDuration(claudeSession: any): number {
  if (!claudeSession?.sessionStartTime) return 0;
  
  const start = new Date(claudeSession.sessionStartTime);
  const end = claudeSession.sessionEndTime 
    ? new Date(claudeSession.sessionEndTime)
    : new Date();
  
  return Math.round((end.getTime() - start.getTime()) / (1000 * 60));
}