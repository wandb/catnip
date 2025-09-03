import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { useAppStore } from "@/stores/appStore";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { PullRequestDialog } from "@/components/PullRequestDialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  AlertTriangle,
  Bot,
  Eye,
  GitMerge,
  MoreHorizontal,
  RefreshCw,
  Terminal,
  Trash2,
  Code2,
  GitBranch,
} from "lucide-react";
import {
  type Worktree,
  type PullRequestInfo,
  type LocalRepository,
} from "@/lib/git-api";
// ConflictStatus type moved - conflicts now tracked directly on worktree.has_conflicts

type Mode = "workspace" | "worktree";

interface WorkspaceActionsProps {
  mode: Mode;
  worktree: Worktree;
  repository?: LocalRepository;

  // Optional props for worktree mode
  mergeConflicts?: Record<string, any>; // ConflictStatus type removed
  prStatus?: PullRequestInfo;
  isSyncing?: boolean;
  isMerging?: boolean;

  // Callbacks
  onDelete?: (id: string, name: string) => void;
  onSync?: (id: string) => void;
  onMerge?: (id: string, name: string) => void;
  onCreatePreview?: (id: string, branch: string) => void;
  onOpenPrDialog?: (worktreeId: string, branchName: string) => void;

  // Additional callbacks for worktree mode
  onConfirmDelete?: (
    id: string,
    name: string,
    isDirty: boolean,
    commitCount: number,
  ) => void;
}

export function WorkspaceActions({
  mode,
  worktree,
  repository,
  mergeConflicts = {},
  prStatus,
  isSyncing = false,
  isMerging = false,
  onDelete,
  onSync,
  onMerge,
  onCreatePreview,
  onOpenPrDialog,
  onConfirmDelete,
}: WorkspaceActionsProps) {
  const sshEnabled = useAppStore((state) => state.sshEnabled);
  const [showSshDialog, setShowSshDialog] = useState(false);
  const [showGitDialog, setShowGitDialog] = useState(false);
  const [prDialogOpen, setPrDialogOpen] = useState(false);

  const handleDeleteClick = (e?: React.MouseEvent) => {
    if (e) {
      e.preventDefault();
      e.stopPropagation();
    }

    if (mode === "workspace" && onDelete) {
      onDelete(worktree.id, worktree.name);
    } else if (mode === "worktree" && onConfirmDelete) {
      onConfirmDelete(
        worktree.id,
        worktree.name,
        worktree.is_dirty,
        worktree.commit_count,
      );
    }
  };

  const handleOpenInCursor = () => {
    const workspacePath = worktree.path.startsWith("/workspace")
      ? worktree.path
      : `/workspace`;
    const url = `cursor://vscode-remote/ssh-remote+catnip${workspacePath}`;
    console.log("Opening Cursor with URL:", url);
    window.location.href = url;
  };

  const handleOpenInVSCode = () => {
    const workspacePath = worktree.path.startsWith("/workspace")
      ? worktree.path
      : `/workspace`;
    window.location.href = `vscode://vscode-remote/ssh-remote+catnip${workspacePath}`;
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={mode === "workspace" ? "h-8 w-8 p-0" : ""}
            onClick={
              mode === "workspace"
                ? (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                  }
                : undefined
            }
          >
            <MoreHorizontal size={16} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {/* Sync actions - available in both workspace and worktree modes for local repos */}
          {worktree.repo_id.startsWith("local/") &&
            worktree.source_branch &&
            onSync && (
              <>
                <DropdownMenuItem
                  onClick={() => onSync(worktree.id)}
                  disabled={isSyncing}
                >
                  {isSyncing ? (
                    <RefreshCw size={16} className="animate-spin" />
                  ) : (
                    <RefreshCw size={16} />
                  )}
                  Sync with {worktree.source_branch}
                  {isSyncing && (
                    <span className="ml-2 text-xs">Syncing...</span>
                  )}
                </DropdownMenuItem>

                {worktree.has_conflicts && (
                  <DropdownMenuItem asChild>
                    <Link
                      to="/terminal/$sessionId"
                      params={{ sessionId: worktree.name }}
                      search={{
                        agent: "claude",
                        prompt:
                          "Please help me resolve these conflicts and complete the rebase. Ask me for confirmation before completing the rebase.",
                      }}
                      className="flex items-center gap-2 text-blue-600"
                    >
                      <Bot size={16} />
                      Auto Resolve Conflicts
                    </Link>
                  </DropdownMenuItem>
                )}

                <DropdownMenuSeparator />
              </>
            )}

          {/* Open in... submenu */}
          <DropdownMenuSub>
            <DropdownMenuSubTrigger className="flex items-center gap-2">
              <Code2 size={16} />
              Open in...
            </DropdownMenuSubTrigger>
            <DropdownMenuPortal>
              <DropdownMenuSubContent>
                {mode !== "workspace" && (
                  <DropdownMenuItem asChild>
                    <Link
                      to="/terminal/$sessionId"
                      params={{ sessionId: worktree.name }}
                      search={{ agent: "claude" }}
                      className="flex items-center gap-2"
                    >
                      <img
                        src="/anthropic.png"
                        alt="Claude"
                        className="w-4 h-4"
                      />
                      Claude
                    </Link>
                  </DropdownMenuItem>
                )}

                {sshEnabled && (
                  <>
                    <DropdownMenuItem
                      onClick={handleOpenInCursor}
                      className="flex items-center gap-2"
                    >
                      <Code2 size={16} />
                      Cursor
                    </DropdownMenuItem>

                    <DropdownMenuItem
                      onClick={handleOpenInVSCode}
                      className="flex items-center gap-2"
                    >
                      <Code2 size={16} />
                      VS Code
                    </DropdownMenuItem>
                  </>
                )}

                {mode !== "workspace" && (
                  <DropdownMenuItem asChild>
                    <Link
                      to="/terminal/$sessionId"
                      params={{ sessionId: worktree.name }}
                      className="flex items-center gap-2"
                    >
                      <Terminal size={16} />
                      Web Terminal
                    </Link>
                  </DropdownMenuItem>
                )}

                {sshEnabled && (
                  <DropdownMenuItem
                    onClick={() => setShowSshDialog(true)}
                    className="flex items-center gap-2"
                  >
                    <Terminal size={16} />
                    SSH
                  </DropdownMenuItem>
                )}

                {!worktree.repo_id.startsWith("local/") && (
                  <DropdownMenuItem
                    onClick={() => setShowGitDialog(true)}
                    className="flex items-center gap-2"
                  >
                    <GitBranch size={16} />
                    Git
                  </DropdownMenuItem>
                )}
              </DropdownMenuSubContent>
            </DropdownMenuPortal>
          </DropdownMenuSub>

          {/* Create PR action - available in both modes */}
          <DropdownMenuItem
            onClick={() => {
              if (onOpenPrDialog) {
                // Use legacy callback if provided
                onOpenPrDialog(worktree.id, worktree.branch);
              } else {
                // Use integrated dialog
                setPrDialogOpen(true);
              }
            }}
            className={
              !worktree.commit_count || worktree.commit_count === 0
                ? "text-muted-foreground"
                : "text-green-600"
            }
            disabled={!worktree.commit_count || worktree.commit_count === 0}
            title={
              !worktree.commit_count || worktree.commit_count === 0
                ? "No commits in this worktree"
                : worktree.pull_request_url
                  ? "Update existing pull request on GitHub"
                  : "Create new pull request on GitHub"
            }
          >
            <GitMerge size={16} />
            {worktree.pull_request_url
              ? "Update PR (GitHub)"
              : "Create PR (GitHub)"}
          </DropdownMenuItem>

          {/* Worktree-specific actions continued */}
          {mode === "worktree" && (
            <>
              {worktree.repo_id.startsWith("local/") && (
                <DropdownMenuItem
                  onClick={() =>
                    onCreatePreview?.(worktree.id, worktree.branch)
                  }
                  className="text-purple-600"
                >
                  <Eye size={16} />
                  Create Preview
                </DropdownMenuItem>
              )}

              {worktree.repo_id.startsWith("local/") &&
                worktree.commit_count > 0 && (
                  <DropdownMenuItem
                    onClick={() => onMerge?.(worktree.id, worktree.name)}
                    disabled={isMerging}
                    className={
                      mergeConflicts[worktree.id]?.has_conflicts
                        ? "text-red-600"
                        : "text-blue-600"
                    }
                  >
                    {isMerging ? (
                      <RefreshCw size={16} className="animate-spin" />
                    ) : mergeConflicts[worktree.id]?.has_conflicts ? (
                      <AlertTriangle size={16} />
                    ) : (
                      <GitMerge size={16} />
                    )}
                    {isMerging
                      ? `Merging ${worktree.commit_count} commits...`
                      : mergeConflicts[worktree.id]?.has_conflicts
                        ? `Merge ${worktree.commit_count} commits (conflicts)`
                        : `Merge ${worktree.commit_count} commits`}
                  </DropdownMenuItem>
                )}
            </>
          )}

          {mode === "worktree" && <DropdownMenuSeparator />}

          {/* Delete action */}
          {(onDelete || onConfirmDelete) && (
            <DropdownMenuItem
              onClick={handleDeleteClick}
              className="text-red-600"
              variant="destructive"
            >
              <Trash2 size={16} />
              Delete {mode === "workspace" ? "Workspace" : "Worktree"}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      {/* SSH Dialog */}
      <Dialog open={showSshDialog} onOpenChange={setShowSshDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>SSH Connection</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <p className="text-muted-foreground">
              Use this command to connect via SSH:
            </p>
            <div className="bg-muted p-3 rounded-md font-mono text-sm">
              WORKDIR=
              {worktree.path.startsWith("/workspace")
                ? worktree.path
                : "/workspace"}{" "}
              ssh catnip
            </div>
            <p className="text-xs text-muted-foreground">
              Make sure you have SSH access configured to the catnip container.
            </p>
          </div>
        </DialogContent>
      </Dialog>

      {/* Git Clone Dialog */}
      <Dialog open={showGitDialog} onOpenChange={setShowGitDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Clone Repository</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <p className="text-muted-foreground">
              Use this command to clone this repository locally:
            </p>
            <div className="bg-muted p-3 rounded-md font-mono text-sm">
              git clone -o catnip http://localhost:6369/
              {worktree.repo_id.split("/").pop()}.git
            </div>
            <p className="text-xs text-muted-foreground">
              This will clone the repository with 'catnip' as the remote name,
              allowing you to work locally and push changes back to this
              environment.
            </p>
          </div>
        </DialogContent>
      </Dialog>

      {/* Pull Request Dialog */}
      {!onOpenPrDialog && (
        <PullRequestDialog
          open={prDialogOpen}
          onOpenChange={setPrDialogOpen}
          worktree={worktree}
          repository={repository}
          prStatus={prStatus}
          summary={undefined} // TODO: Pass WorktreeSummary if available
          onRefreshPrStatuses={async () => {
            // TODO: Implement PR status refresh
            console.log("Refreshing PR statuses...");
          }}
        />
      )}
    </>
  );
}
