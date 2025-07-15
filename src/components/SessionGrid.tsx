import { useState } from "react";
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { Button } from "@/components/ui/button";
import { RefreshCw, Plus } from "lucide-react";
import { SessionCard } from "./SessionCard";
import type { SessionCardData } from "@/types/session";

interface SessionGridProps {
  sessions: SessionCardData[];
  onRefresh: () => void;
  onError: (error: { open: boolean; title: string; description: string }) => void;
}

// Grid container with drag-and-drop functionality
export function SessionGrid({ sessions, onRefresh, onError }: SessionGridProps) {
  const [activeSession, setActiveSession] = useState<SessionCardData | null>(null);
  const [sessionPositions, setSessionPositions] = useState<Record<string, { x: number; y: number }>>({});

  // Configure drag sensors
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8, // 8px of movement before drag starts
      },
    })
  );

  // Handle drag start
  const handleDragStart = (event: DragStartEvent) => {
    const session = sessions.find(s => s.id === event.active.id);
    setActiveSession(session || null);
  };

  // Handle drag end - update positions
  const handleDragEnd = (event: DragEndEvent) => {
    const { active, delta } = event;
    
    if (delta.x !== 0 || delta.y !== 0) {
      updateSessionPosition(active.id as string, delta);
    }
    
    setActiveSession(null);
  };

  // Update session position locally
  const updateSessionPosition = (sessionId: string, delta: { x: number; y: number }) => {
    setSessionPositions(prev => ({
      ...prev,
      [sessionId]: {
        x: (prev[sessionId]?.x || 0) + delta.x,
        y: (prev[sessionId]?.y || 0) + delta.y,
      }
    }));
  };

  // Get effective position for a session
  const getSessionPosition = (session: SessionCardData) => {
    return sessionPositions[session.id] || session.position;
  };

  return (
    <div className="space-y-4">
      {/* Header with actions */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <h2 className="text-xl font-semibold">Sessions</h2>
          <Button
            variant="outline"
            size="sm"
            onClick={onRefresh}
            className="gap-2"
          >
            <RefreshCw size={16} />
            Refresh
          </Button>
        </div>
        
        <Button size="sm" className="gap-2">
          <Plus size={16} />
          New Session
        </Button>
      </div>

      {/* Drag and drop context */}
      <DndContext
        sensors={sensors}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
      >
        {/* Session cards grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {sessions.map(session => (
            <SessionCard
              key={session.id}
              session={session}
              position={getSessionPosition(session)}
              onError={onError}
            />
          ))}
        </div>

        {/* Drag overlay */}
        <DragOverlay>
          {activeSession && (
            <SessionCard
              session={activeSession}
              position={getSessionPosition(activeSession)}
              onError={onError}
              isDragging={true}
            />
          )}
        </DragOverlay>
      </DndContext>

      {/* Empty state */}
      {sessions.length === 0 && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No sessions found</p>
          <Button className="mt-4 gap-2">
            <Plus size={16} />
            Create your first session
          </Button>
        </div>
      )}
    </div>
  );
}