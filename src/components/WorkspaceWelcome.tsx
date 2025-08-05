import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { GitBranch, FileCode, FolderOpen, Plus } from "lucide-react";
import { NewWorkspaceDialog } from "@/components/NewWorkspaceDialog";

export function WorkspaceWelcome() {
  const [newWorkspaceOpen, setNewWorkspaceOpen] = useState(false);
  const [showTemplateFirst, setShowTemplateFirst] = useState(false);

  const handleNewFromTemplate = () => {
    setShowTemplateFirst(true);
    setNewWorkspaceOpen(true);
  };

  const handleNewFromRepo = () => {
    setShowTemplateFirst(false);
    setNewWorkspaceOpen(true);
  };

  return (
    <div className="flex h-screen items-center justify-center p-8">
      <div className="max-w-4xl w-full space-y-8">
        <div className="text-center space-y-4">
          <h1 className="text-4xl font-bold">Welcome to Catnip</h1>
          <p className="text-xl text-muted-foreground">
            Get started by creating a new workspace
          </p>
        </div>

        <div className="grid gap-6 md:grid-cols-2">
          <Card
            className="hover:border-primary transition-colors cursor-pointer"
            onClick={handleNewFromRepo}
          >
            <CardHeader>
              <div className="flex items-center gap-3">
                <div className="p-3 bg-primary/10 rounded-lg">
                  <GitBranch className="w-6 h-6 text-primary" />
                </div>
                <div>
                  <CardTitle>Clone Repository</CardTitle>
                  <CardDescription>
                    Start from an existing Git repository
                  </CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Clone a repository from GitHub or connect to a local Git
                repository to begin working on your project.
              </p>
            </CardContent>
          </Card>

          <Card
            className="hover:border-primary transition-colors cursor-pointer"
            onClick={handleNewFromTemplate}
          >
            <CardHeader>
              <div className="flex items-center gap-3">
                <div className="p-3 bg-primary/10 rounded-lg">
                  <FileCode className="w-6 h-6 text-primary" />
                </div>
                <div>
                  <CardTitle>Start from Template</CardTitle>
                  <CardDescription>
                    Create a new project from a template
                  </CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Choose from React, Vue, Next.js, Express, FastAPI, and more
                pre-configured templates to jumpstart your project.
              </p>
            </CardContent>
          </Card>
        </div>

        <div className="flex justify-center">
          <Card className="w-full max-w-2xl border-dashed">
            <CardHeader>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <FolderOpen className="w-5 h-5 text-muted-foreground" />
                  <div>
                    <CardTitle className="text-lg">Recent Workspaces</CardTitle>
                    <CardDescription>
                      No recent workspaces found
                    </CardDescription>
                  </div>
                </div>
                <Button onClick={handleNewFromRepo} size="sm" variant="outline">
                  <Plus className="w-4 h-4 mr-2" />
                  New Workspace
                </Button>
              </div>
            </CardHeader>
          </Card>
        </div>
      </div>

      <NewWorkspaceDialog
        open={newWorkspaceOpen}
        onOpenChange={setNewWorkspaceOpen}
        showTemplateFirst={showTemplateFirst}
      />
    </div>
  );
}
