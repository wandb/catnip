import { createFileRoute, Link } from "@tanstack/react-router";
import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Search, FileText } from "lucide-react";
import { ErrorDisplay } from "@/components/ErrorDisplay";

interface Session {
  sessionId: string;
  worktreePath: string;
  header?: string;
  turnCount: number;
  startTime?: string | null;
  endTime?: string | null;
  isActive: boolean;
  lastCost?: number;
  lastDuration?: number;
  provider: "Claude" | "Gemini";
}

function TranscriptIndex() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void fetchSessions();
  }, []);

  const fetchSessions = async () => {
    try {
      const [claudeResponse] = await Promise.all([
        fetch("/v1/claude/sessions"),
      ]);

      if (!claudeResponse.ok) {
        throw new Error(
          `Failed to fetch Claude sessions: ${claudeResponse.statusText}`,
        );
      }

      const claudeData = await claudeResponse.json();

      // Transform the data into a flat array of sessions
      const sessionsList: Session[] = [];

      Object.entries(claudeData).forEach(
        ([worktreePath, summary]: [string, any]) => {
          // Add sessions from allSessions if available
          if (summary.allSessions && Array.isArray(summary.allSessions)) {
            summary.allSessions.forEach((session: any) => {
              sessionsList.push({
                sessionId: session.sessionId,
                worktreePath,
                header: summary.header,
                turnCount: summary.turnCount || 0,
                startTime: session.startTime,
                endTime: session.endTime,
                isActive: session.isActive || false,
                lastCost: summary.lastCost,
                lastDuration: summary.lastDuration,
                provider: "Claude",
              });
            });
          } else if (summary.currentSessionId || summary.lastSessionId) {
            // Add a single session entry for workspaces without allSessions
            sessionsList.push({
              sessionId: summary.currentSessionId || summary.lastSessionId,
              worktreePath,
              header: summary.header,
              turnCount: summary.turnCount || 0,
              startTime: summary.sessionStartTime,
              endTime: summary.sessionEndTime,
              isActive: summary.isActive || false,
              lastCost: summary.lastCost,
              lastDuration: summary.lastDuration,
              provider: "Claude",
            });
          }
        },
      );

      // Sort by most recent activity first
      sessionsList.sort((a, b) => {
        const aTime = a.endTime || a.startTime || "";
        const bTime = b.endTime || b.startTime || "";
        return bTime.localeCompare(aTime);
      });

      setSessions(sessionsList);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch sessions");
    } finally {
      setLoading(false);
    }
  };

  const filteredSessions = sessions.filter(
    (session) =>
      session.sessionId.toLowerCase().includes(searchTerm.toLowerCase()) ||
      session.worktreePath.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (session.header &&
        session.header.toLowerCase().includes(searchTerm.toLowerCase())),
  );

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <div className="max-w-4xl mx-auto">
          <div className="flex items-center justify-center py-8">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-2">Loading sessions...</span>
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="container mx-auto px-4 py-8">
        <div className="max-w-4xl mx-auto">
          <ErrorDisplay
            title="Failed to Load Sessions"
            message={error}
            onRetry={() => void fetchSessions()}
            retryLabel="Try Again"
          />
        </div>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-8">
      <div className="max-w-4xl mx-auto space-y-6">
        <div>
          <h1 className="text-3xl font-bold mb-2">Session Transcripts</h1>
          <p className="text-muted-foreground">
            Browse and view detailed transcripts of Claude and Gemini coding
            sessions
          </p>
        </div>

        <div className="relative">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search sessions..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="pl-10"
          />
        </div>

        <div className="space-y-4">
          {filteredSessions.length === 0 ? (
            <Card>
              <CardContent className="p-6 text-center">
                <div className="text-muted-foreground">
                  {searchTerm
                    ? "No sessions match your search."
                    : "No sessions available."}
                </div>
              </CardContent>
            </Card>
          ) : (
            filteredSessions.map((session) => (
              <Card
                key={session.sessionId}
                className="hover:shadow-md transition-shadow"
              >
                <CardHeader>
                  <CardTitle className="flex items-center justify-between">
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <FileText className="h-5 w-5" />
                        <span className="font-mono text-sm">
                          {session.worktreePath}
                        </span>
                        {session.isActive && (
                          <Badge variant="default" className="ml-2">
                            Active
                          </Badge>
                        )}
                        <Badge
                          variant={
                            session.provider === "Claude"
                              ? "secondary"
                              : "outline"
                          }
                          className="ml-2"
                        >
                          {session.provider}
                        </Badge>
                      </div>
                      {session.header && (
                        <p className="text-sm text-muted-foreground mt-1 line-clamp-1">
                          {session.header}
                        </p>
                      )}
                    </div>
                    <Link
                      to="/transcript/$sessionId"
                      params={{ sessionId: session.sessionId }}
                    >
                      <Button size="sm">View Transcript</Button>
                    </Link>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center gap-4 text-sm text-muted-foreground">
                    <Badge variant="secondary">{session.turnCount} turns</Badge>
                    {session.startTime && (
                      <span>
                        Started: {new Date(session.startTime).toLocaleString()}
                      </span>
                    )}
                    {session.endTime && (
                      <span>
                        Ended: {new Date(session.endTime).toLocaleString()}
                      </span>
                    )}
                    {session.lastDuration && (
                      <span>
                        Duration: {Math.round(session.lastDuration / 1000)}s
                      </span>
                    )}
                    {session.lastCost !== undefined && session.lastCost > 0 && (
                      <span>Cost: ${session.lastCost.toFixed(4)}</span>
                    )}
                  </div>
                </CardContent>
              </Card>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

export const Route = createFileRoute("/transcript/")({
  component: TranscriptIndex,
});
