import type { Worktree } from "@/lib/git-api";

// Session status enum
export type SessionStatus = "progress" | "finished" | "errored" | "needs_input";

// Session metrics interface
export interface SessionMetrics {
  cost: number;
  turns: number;
  tokens: number;
  duration: number; // in minutes
}

// Card position for drag and drop
export interface CardPosition {
  x: number;
  y: number;
}

// Claude session data (matches existing interface)
export interface ClaudeSession {
  sessionStartTime?: string | Date;
  sessionEndTime?: string | Date;
  isActive: boolean;
  turnCount: number;
  lastCost: number;
  tokenCount?: number;
  hasError?: boolean;
  waitingForInput?: boolean;
}

// Main session card data interface
export interface SessionCardData {
  id: string;
  name: string;
  worktree: Worktree;
  claudeSession?: ClaudeSession;
  status: SessionStatus;
  metrics: SessionMetrics;
  position: CardPosition;
  generatedName?: string; // AI-generated meaningful name
}