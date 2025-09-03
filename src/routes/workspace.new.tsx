import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { NewWorkspace } from "@/components/NewWorkspace";
import { NewWorkspaceDialog } from "@/components/NewWorkspaceDialog";
import { useMediaQuery } from "@/hooks/use-media-query";
import { useGitApi } from "@/hooks/useGitApi";

function NewWorkspacePage() {
  const navigate = useNavigate();
  const [dialogOpen, setDialogOpen] = useState(false);
  const isDesktop = useMediaQuery("(min-width: 768px)");
  const { checkoutRepository } = useGitApi();

  // Handle navigation back to workspace list
  const handleClose = () => {
    void navigate({
      to: "/workspace",
    });
  };

  // Handle workspace creation
  const handleCreateWorkspace = async (
    repoId: string,
    branch: string,
    prompt: string,
  ) => {
    console.log("Creating workspace:", { repoId, branch, prompt });
    console.log("Repository ID type:", typeof repoId, "length:", repoId.length);
    console.log("Branch type:", typeof branch, "length:", branch.length);

    try {
      let result: { success: boolean; worktreeName?: string };

      // Check if this is a local repository (starts with "local/")
      if (repoId.startsWith("local/")) {
        // For local repos, extract the repo name
        const repoName = repoId.split("/")[1];
        result = await checkoutRepository("local", repoName, branch);
      } else {
        // For GitHub URLs, parse the org and repo name
        console.log("Parsing GitHub repository URL:", repoId);

        let org = "";
        let repo = "";

        // Try different GitHub URL formats
        if (repoId.includes("github.com")) {
          // Handle full GitHub URLs: https://github.com/owner/repo or git@github.com:owner/repo
          const match = repoId.match(
            /github\.com[/:]([\w.-]+)\/([\w.-]+?)(?:\.git)?(?:\/)?$/,
          );
          if (match) {
            org = match[1];
            repo = match[2];
            console.log("Parsed from GitHub URL:", { org, repo, repoId });
          } else {
            console.error("Failed to parse GitHub URL:", repoId);
            handleClose();
            return;
          }
        } else if (repoId.includes("/")) {
          // Handle org/repo format
          const match = repoId.match(/^([\w.-]+)\/([\w.-]+)$/);
          if (match) {
            org = match[1];
            repo = match[2];
            console.log("Parsed from org/repo format:", { org, repo, repoId });
          } else {
            console.error("Failed to parse org/repo format:", repoId);
            handleClose();
            return;
          }
        } else {
          console.error("Unknown repository format:", repoId);
          handleClose();
          return;
        }

        if (org && repo) {
          console.log("Creating workspace with:", { org, repo, branch });
          result = await checkoutRepository(org, repo, branch);
        } else {
          console.error("Failed to extract org/repo from:", repoId);
          handleClose();
          return;
        }
      }

      if (result.success && result.worktreeName) {
        console.log("Workspace created successfully:", result.worktreeName);
        // Wait a moment for the workspace to be fully set up
        setTimeout(() => {
          // Navigate to the newly created workspace with the prompt
          const parts = result.worktreeName!.split("/");
          console.log("Navigating to workspace parts:", parts);
          if (parts.length >= 2) {
            const navParams = {
              to: "/workspace/$project/$workspace" as const,
              params: {
                project: parts[0],
                workspace: parts[1],
              },
              search: {
                prompt: prompt.trim(),
              },
            };
            console.log("Navigation params:", navParams);
            void navigate(navParams);
          }
        }, 1000); // 1 second delay to let backend finish setup
        return;
      } else {
        console.error("Workspace creation failed:", result);
      }
    } catch (error) {
      console.error("Failed to create workspace:", error);
    }

    // Fallback: navigate back to workspace list
    handleClose();
  };

  // For desktop, show dialog overlay; for mobile, show full screen
  useEffect(() => {
    if (isDesktop) {
      setDialogOpen(true);
    }
  }, [isDesktop]);

  // Handle dialog close on desktop
  const handleDialogClose = (open: boolean) => {
    setDialogOpen(open);
    if (!open) {
      handleClose();
    }
  };

  if (isDesktop) {
    // Desktop: Show dialog overlay
    return (
      <div className="h-screen w-full bg-background">
        {/* Background content - could show workspace list or main view */}
        <div className="h-full w-full flex items-center justify-center text-muted-foreground">
          <div className="text-center space-y-2">
            <h2 className="text-xl font-semibold">Create New Workspace</h2>
            <p className="text-sm">
              Choose a template or repository to get started
            </p>
          </div>
        </div>

        {/* New Workspace Dialog */}
        <NewWorkspaceDialog
          open={dialogOpen}
          onOpenChange={handleDialogClose}
          showTemplateFirst={false}
        />
      </div>
    );
  }

  // Mobile: Show full screen new workspace UI
  return (
    <NewWorkspace onClose={handleClose} onSubmit={handleCreateWorkspace} />
  );
}

export const Route = createFileRoute("/workspace/new")({
  component: NewWorkspacePage,
});
