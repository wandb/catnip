import { createFileRoute, Link } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { WorkspaceCard } from "@/components/WorkspaceCard";
import { useGitState } from "@/hooks/useGitState";
import { useGitActions } from "@/hooks/useGitActions";

function Index() {
  const { worktrees, loading } = useGitState();
  const { deleteWorktree } = useGitActions();

  const handleDelete = (id: string, name: string) => {
    if (confirm(`Are you sure you want to delete workspace "${name}"?`)) {
      void deleteWorktree(id);
    }
  };

  return (
    <div className="container mx-auto px-4 py-16">
      <h1 className="text-3xl font-bold text-center mb-8">Workspaces</h1>
      {loading ? (
        <div className="flex justify-center">Loading...</div>
      ) : (
        <div className="flex flex-wrap justify-center gap-6">
          {worktrees.map((wt) => (
            <WorkspaceCard key={wt.id} worktree={wt} onDelete={handleDelete} />
          ))}
          <Link
            to="/git"
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
