import { createFileRoute, Link } from "@tanstack/react-router";
import { Loader2, Plus } from "lucide-react";
import { WorkspaceCard } from "@/components/WorkspaceCard";
import { useWorktreeStore } from "@/hooks/useWorktreeStore";
import { useGitActions } from "@/hooks/useGitActions";
import { useGitState } from "@/hooks/useGitState";

function Index() {
  // Use the new SSE-driven worktree store
  const { worktrees, loading } = useWorktreeStore();

  // Still need useGitState for delete operations and other functionality
  const {
    // Functions needed by useGitActions
    addNewWorktrees,
    backgroundRefreshGitStatus,
    refreshWorktree,
    removeWorktree,
    fetchActiveSessions,
    setCheckoutLoading,
    setSyncingWorktree,
    setMergingWorktree,
  } = useGitState();
  const { deleteWorktree } = useGitActions({
    addNewWorktrees,
    backgroundRefreshGitStatus,
    refreshWorktree,
    removeWorktree,
    fetchActiveSessions,
    setCheckoutLoading,
    setSyncingWorktree,
    setMergingWorktree,
  });

  const handleDelete = (id: string, name: string) => {
    if (confirm(`Are you sure you want to delete workspace "${name}"?`)) {
      void deleteWorktree(id);
    }
  };

  return (
    <div className="container mx-auto px-4 py-16">
      <h1 className="text-3xl font-bold text-center mb-8">Workspaces</h1>
      {loading ? (
        <div className="flex justify-center">
          <Loader2 className="animate-spin" />
        </div>
      ) : (
        <div className="flex flex-wrap justify-center gap-6">
          {worktrees.map((wt) => (
            <WorkspaceCard key={wt.id} worktree={wt} onDelete={handleDelete} />
          ))}
          <Link
            to="/git"
            state={{ fromWorkspace: true } as any}
            className="w-[350px] h-[350px] border-2 border-dashed rounded-lg flex items-center justify-center text-muted-foreground hover:bg-muted"
          >
            <Plus size={64} className="opacity-50" />
          </Link>
        </div>
      )}
    </div>
  );
}

export const Route = createFileRoute("/")({
  component: Index,
});
