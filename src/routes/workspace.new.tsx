import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { NewWorkspace } from "@/components/NewWorkspace";
import { NewWorkspaceDialog } from "@/components/NewWorkspaceDialog";
import { useMediaQuery } from "@/hooks/use-media-query";

function NewWorkspacePage() {
  const navigate = useNavigate();
  const [dialogOpen, setDialogOpen] = useState(false);
  const isDesktop = useMediaQuery("(min-width: 768px)");

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
    // TODO: Implement workspace creation API call
    console.log("Creating workspace:", { repoId, branch, prompt });

    // For now, just navigate back to workspace list
    // In a real implementation, this would:
    // 1. Create the workspace via API
    // 2. Navigate to the new workspace
    // 3. Start the Claude session with the prompt
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
