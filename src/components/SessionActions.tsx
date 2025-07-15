import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  MoreHorizontal,
  Terminal,
  GitMerge,
  Eye,
  RefreshCw,
  Trash2,
  Sparkles,
  FileText,
} from "lucide-react";
import type { SessionCardData } from "@/types/session";
import { generateSessionName } from "@/lib/session-names";

interface SessionActionsProps {
  session: SessionCardData;
  onError: (error: { open: boolean; title: string; description: string }) => void;
}

// Dropdown menu with all session actions
export function SessionActions({ session, onError }: SessionActionsProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
          <MoreHorizontal className="w-4 h-4" />
        </Button>
      </DropdownMenuTrigger>
      
      <DropdownMenuContent align="end" className="w-48">
        {/* Terminal actions */}
        <DropdownMenuItem asChild>
          <Link
            to="/terminal/$sessionId"
            params={{ sessionId: session.worktree.name }}
            className="flex items-center gap-2 w-full"
          >
            <Terminal className="w-4 h-4" />
            Open Terminal
          </Link>
        </DropdownMenuItem>
        
        <DropdownMenuItem asChild>
          <Link
            to="/terminal/$sessionId"
            params={{ sessionId: session.worktree.name }}
            search={{ agent: "claude" }}
            className="flex items-center gap-2 w-full"
          >
            <Sparkles className="w-4 h-4" />
            Vibe with Claude
          </Link>
        </DropdownMenuItem>
        
        <DropdownMenuSeparator />
        
        {/* Git actions */}
        {session.worktree.commit_count > 0 && (
          <>
            <DropdownMenuItem onClick={() => handleViewDiff(session)}>
              <FileText className="w-4 h-4" />
              View Diff
            </DropdownMenuItem>
            
            <DropdownMenuItem onClick={() => handleCreatePR(session, onError)}>
              <GitMerge className="w-4 h-4" />
              Create PR
            </DropdownMenuItem>
            
            <DropdownMenuItem onClick={() => handleMerge(session, onError)}>
              <GitMerge className="w-4 h-4" />
              Merge to Main
            </DropdownMenuItem>
          </>
        )}
        
        {/* Preview action for non-local repos */}
        {!session.worktree.repo_id.startsWith("local/") && (
          <DropdownMenuItem onClick={() => handleCreatePreview(session, onError)}>
            <Eye className="w-4 h-4" />
            Create Preview
          </DropdownMenuItem>
        )}
        
        <DropdownMenuSeparator />
        
        {/* Management actions */}
        <DropdownMenuItem onClick={() => handleSync(session, onError)}>
          <RefreshCw className="w-4 h-4" />
          Sync with {session.worktree.source_branch}
        </DropdownMenuItem>
        
        <DropdownMenuItem onClick={() => handleRegenerateName(session, onError)}>
          <Sparkles className="w-4 h-4" />
          Regenerate Name
        </DropdownMenuItem>
        
        <DropdownMenuSeparator />
        
        <DropdownMenuItem 
          onClick={() => handleDelete(session, onError)}
          className="text-red-600 focus:text-red-600"
        >
          <Trash2 className="w-4 h-4" />
          Delete Session
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

// Action handlers - keep them small and focused
function handleViewDiff(session: SessionCardData) {
  // TODO: Implement diff viewing
  console.log("View diff for session:", session.id);
}

function handleCreatePR(session: SessionCardData, _onError: SessionActionsProps["onError"]) {
  // TODO: Implement PR creation
  console.log("Create PR for session:", session.id);
}

function handleMerge(session: SessionCardData, _onError: SessionActionsProps["onError"]) {
  // TODO: Implement merge
  console.log("Merge session:", session.id);
}

function handleCreatePreview(session: SessionCardData, _onError: SessionActionsProps["onError"]) {
  // TODO: Implement preview creation
  console.log("Create preview for session:", session.id);
}

function handleSync(session: SessionCardData, _onError: SessionActionsProps["onError"]) {
  // TODO: Implement sync
  console.log("Sync session:", session.id);
}

async function handleRegenerateName(session: SessionCardData, onError: SessionActionsProps["onError"]) {
  try {
    const newName = await generateSessionName(session);
    
    // TODO: Update the session name in the backend/state
    console.log("Generated new name:", newName, "for session:", session.id);
    
    // For now, just show success - will integrate with backend later
    // toast.success(`Generated new name: "${newName}"`);
    
  } catch (error) {
    onError({
      open: true,
      title: "Name Generation Failed",
      description: "Failed to generate a new name for this session. Please try again.",
    });
  }
}

function handleDelete(session: SessionCardData, _onError: SessionActionsProps["onError"]) {
  // TODO: Implement deletion with confirmation
  console.log("Delete session:", session.id);
}