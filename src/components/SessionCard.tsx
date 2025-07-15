import { Link } from "@tanstack/react-router";
import { useDraggable } from "@dnd-kit/core";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { 
  Terminal, 
  GitMerge, 
  Eye, 
  MoreHorizontal,
  Loader2,
  CheckCircle,
  XCircle,
  AlertCircle,
  DollarSign,
  Clock,
  MessageSquare,
  Hash
} from "lucide-react";
import { SessionActions } from "./SessionActions";
import type { SessionCardData } from "@/types/session";

interface SessionCardProps {
  session: SessionCardData;
  position: { x: number; y: number };
  onError: (error: { open: boolean; title: string; description: string }) => void;
  isDragging?: boolean;
}

// Main session card component with drag-and-drop
export function SessionCard({ session, position, onError, isDragging = false }: SessionCardProps) {
  const { attributes, listeners, setNodeRef, transform } = useDraggable({
    id: session.id,
  });

  // Calculate transform for dragging
  const style = transform ? {
    transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
  } : undefined;

  return (
    <Card
      ref={setNodeRef}
      style={style}
      className={`cursor-grab active:cursor-grabbing transition-shadow ${
        isDragging ? "shadow-lg rotate-2" : "hover:shadow-md"
      }`}
      {...attributes}
      {...listeners}
    >
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex-1">
            <CardTitle className="text-lg font-semibold line-clamp-1">
              {session.name}
            </CardTitle>
            <SessionStatus status={session.status} />
          </div>
          <SessionActions session={session} onError={onError} />
        </div>
      </CardHeader>
      
      <CardContent className="space-y-3">
        {/* Worktree info */}
        <WorktreeInfo worktree={session.worktree} />
        
        {/* Session metrics */}
        <SessionMetrics metrics={session.metrics} />
        
        {/* Quick actions */}
        <QuickActions session={session} />
      </CardContent>
    </Card>
  );
}

// Status indicator with appropriate styling
function SessionStatus({ status }: { status: SessionCardData["status"] }) {
  const statusConfig = {
    progress: {
      icon: <Loader2 className="w-3 h-3 animate-spin" />,
      label: "In Progress",
      variant: "secondary" as const,
      className: "text-blue-600 bg-blue-100 border-blue-200"
    },
    finished: {
      icon: <CheckCircle className="w-3 h-3" />,
      label: "Finished",
      variant: "secondary" as const,
      className: "text-green-600 bg-green-100 border-green-200"
    },
    errored: {
      icon: <XCircle className="w-3 h-3" />,
      label: "Error",
      variant: "destructive" as const,
      className: "text-red-600 bg-red-100 border-red-200"
    },
    needs_input: {
      icon: <AlertCircle className="w-3 h-3" />,
      label: "Needs Input",
      variant: "secondary" as const,
      className: "text-orange-600 bg-orange-100 border-orange-200"
    }
  };

  const config = statusConfig[status];

  return (
    <Badge variant={config.variant} className={`text-xs gap-1 ${config.className}`}>
      {config.icon}
      {config.label}
    </Badge>
  );
}

// Worktree information display
function WorktreeInfo({ worktree }: { worktree: SessionCardData["worktree"] }) {
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <Badge variant="outline" className="text-xs">
          {worktree.branch}
        </Badge>
        {worktree.is_dirty && (
          <Badge variant="destructive" className="text-xs">
            Dirty
          </Badge>
        )}
      </div>
      
      <div className="text-xs text-muted-foreground">
        {worktree.commit_count} commit{worktree.commit_count !== 1 ? 's' : ''}
        {worktree.commits_behind > 0 && (
          <span className="text-orange-600 ml-2">
            {worktree.commits_behind} behind
          </span>
        )}
      </div>
    </div>
  );
}

// Session metrics display
function SessionMetrics({ metrics }: { metrics: SessionCardData["metrics"] }) {
  return (
    <div className="grid grid-cols-2 gap-2 text-xs">
      <MetricItem
        icon={<DollarSign className="w-3 h-3" />}
        label="Cost"
        value={`$${metrics.cost.toFixed(4)}`}
      />
      <MetricItem
        icon={<MessageSquare className="w-3 h-3" />}
        label="Turns"
        value={metrics.turns.toString()}
      />
      <MetricItem
        icon={<Hash className="w-3 h-3" />}
        label="Tokens"
        value={metrics.tokens.toLocaleString()}
      />
      <MetricItem
        icon={<Clock className="w-3 h-3" />}
        label="Duration"
        value={`${metrics.duration}m`}
      />
    </div>
  );
}

// Individual metric item
function MetricItem({ icon, label, value }: { 
  icon: React.ReactNode; 
  label: string; 
  value: string; 
}) {
  return (
    <div className="flex items-center gap-1 text-muted-foreground">
      {icon}
      <span>{value}</span>
    </div>
  );
}

// Quick action buttons
function QuickActions({ session }: { session: SessionCardData }) {
  return (
    <div className="flex items-center gap-2">
      <Link
        to="/terminal/$sessionId"
        params={{ sessionId: session.worktree.name }}
        search={{ agent: "claude" }}
      >
        <Button variant="outline" size="sm" className="gap-1">
          <Terminal className="w-3 h-3" />
          Vibe
        </Button>
      </Link>
      
      {session.worktree.commit_count > 0 && (
        <Button variant="outline" size="sm" className="gap-1">
          <GitMerge className="w-3 h-3" />
          PR
        </Button>
      )}
    </div>
  );
}