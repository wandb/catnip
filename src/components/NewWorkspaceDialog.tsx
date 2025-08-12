import { useState, useEffect, useRef } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { BranchDisplay } from "@/components/BranchDisplay";
import { RepoSelector } from "@/components/RepoSelector";
import { TemplateSelector } from "@/components/TemplateSelector";
import { Loader2, GitBranch, FileCode, X } from "lucide-react";
import { type LocalRepository, gitApi } from "@/lib/git-api";
import { useAppStore } from "@/stores/appStore";
import { useGitApi } from "@/hooks/useGitApi";
import type { ProjectTemplate } from "@/types/template";

interface NewWorkspaceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialRepoUrl?: string;
  initialBranch?: string;
  showTemplateFirst?: boolean;
}

export function NewWorkspaceDialog({
  open,
  onOpenChange,
  initialRepoUrl,
  initialBranch,
  showTemplateFirst = false,
}: NewWorkspaceDialogProps) {
  const { gitStatus } = useAppStore();
  const { checkoutRepository } = useGitApi();
  const navigate = useNavigate();

  const [githubUrl, setGithubUrl] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("");
  const [selectedRepoBranches, setSelectedRepoBranches] = useState<string[]>(
    [],
  );
  const [branchesLoading, setBranchesLoading] = useState(false);
  const [checkoutLoading, setCheckoutLoading] = useState(false);
  const [githubRepos, setGithubRepos] = useState<any[]>([]);
  const [currentGithubRepos, setCurrentGithubRepos] = useState<
    Record<string, LocalRepository>
  >({});
  const [error, setError] = useState<string | null>(null);
  const hasFetchedRepos = useRef(false);
  const [activeTab, setActiveTab] = useState<"repository" | "template">(
    showTemplateFirst ? "template" : "repository",
  );
  const [selectedTemplate, setSelectedTemplate] =
    useState<ProjectTemplate | null>(null);
  const [projectName, setProjectName] = useState("");

  // Reset form when dialog opens/closes
  useEffect(() => {
    if (!open) {
      setGithubUrl("");
      setSelectedBranch("");
      setSelectedRepoBranches([]);
      setCheckoutLoading(false);
      setError(null);
      setSelectedTemplate(null);
      setProjectName("");
      setActiveTab(showTemplateFirst ? "template" : "repository");
    } else if (open) {
      // Set active tab when dialog opens
      setActiveTab(showTemplateFirst ? "template" : "repository");

      if (initialRepoUrl) {
        // Set initial values when dialog opens with pre-selected repo
        setGithubUrl(initialRepoUrl);
        // Don't set initialBranch - let handleRepoChange determine the correct default branch
        // Immediately fetch branches for the initial repo
        void handleRepoChange(initialRepoUrl);
      }
    }
  }, [open, initialRepoUrl, initialBranch, showTemplateFirst]);

  // Fetch GitHub repositories when dialog opens (only once)
  useEffect(() => {
    if (open && !hasFetchedRepos.current) {
      const fetchGithubRepos = async () => {
        try {
          hasFetchedRepos.current = true;
          const repos = await gitApi.fetchRepositories();
          setGithubRepos(repos);

          const repositories = useAppStore.getState().getRepositoriesList();
          setCurrentGithubRepos(
            repositories.reduce(
              (acc, repo) => {
                acc[repo.id] = repo;
                return acc;
              },
              {} as Record<string, LocalRepository>,
            ),
          );
        } catch (error) {
          console.error("Failed to load repositories:", error);
          hasFetchedRepos.current = false; // Reset on error so we can retry
        }
      };

      void fetchGithubRepos();
    }
  }, [open]);

  // Handle template creation
  const handleTemplateCreate = async () => {
    if (!selectedTemplate || !projectName.trim()) return;

    setCheckoutLoading(true);
    setError(null);
    try {
      // Validate project name
      const cleanName = projectName.trim().replace(/[^a-zA-Z0-9-_]/g, "-");
      if (cleanName !== projectName.trim()) {
        setError(
          "Project name can only contain letters, numbers, hyphens, and underscores",
        );
        return false;
      }

      // Create workspace from template
      const result = await gitApi.createFromTemplate(
        selectedTemplate.id,
        cleanName,
        (error: Error) => setError(error.message),
      );

      if (result.success) {
        // Close the dialog
        onOpenChange(false);

        // Navigate to the specific project workspace using the worktree name
        if (result.worktreeName) {
          // Split the worktree name (format: "projectName/workspaceName")
          const parts = result.worktreeName.split("/");
          if (parts.length >= 2) {
            window.location.href = `/workspace/${parts[0]}/${parts[1]}`;
          } else {
            // Fallback: use the project name as both project and workspace
            window.location.href = `/workspace/${cleanName}/${cleanName}`;
          }
        } else {
          // Fallback: use the project name as both project and workspace
          window.location.href = `/workspace/${cleanName}/${cleanName}`;
        }
      }

      return result.success;
    } catch (error) {
      console.error("Error creating from template:", error);
      setError(
        error instanceof Error
          ? error.message
          : "Failed to create from template",
      );
      return false;
    } finally {
      setCheckoutLoading(false);
    }
  };

  // Handle checkout functionality
  const handleCheckout = async (url: string, branch?: string) => {
    if (!url || !branch) return false;

    setCheckoutLoading(true);
    setError(null);
    try {
      let result: { success: boolean; worktreeName?: string };
      let repoId: string;

      // Check if this is a local repository (starts with "local/")
      if (url.startsWith("local/")) {
        // For local repos, extract the repo name
        const repoName = url.split("/")[1];
        repoId = `local/${repoName}`;
        result = await checkoutRepository("local", repoName, branch);
      } else {
        // For GitHub URLs, parse the org and repo name
        // Updated regex to handle URLs with or without .git suffix
        let match = url.match(
          /github\.com[/:]([\w.-]+)\/([\w.-]+?)(?:\.git)?(?:\/)?$/,
        );
        if (!match) {
          // Try without protocol
          match = url.match(/^([\w.-]+)\/([\w.-]+)$/);
        }

        if (match) {
          const org = match[1];
          const repo = match[2];
          repoId = `${org}/${repo}`;
          result = await checkoutRepository(org, repo, branch);
        } else {
          setError(
            "Invalid GitHub URL format. Please use a valid GitHub URL or org/repo format.",
          );
          return false;
        }
      }

      if (result.success) {
        onOpenChange(false);

        // Navigate to the newly created workspace
        if (result.worktreeName) {
          // Split the worktree name (format: "projectName/workspaceName")
          const parts = result.worktreeName.split("/");
          if (parts.length >= 2) {
            void navigate({
              to: "/workspace/$project/$workspace",
              params: {
                project: parts[0],
                workspace: parts[1],
              },
            });
          }
        }
      } else {
        setError(
          "Failed to create workspace. Please check the repository URL and try again.",
        );
      }

      return result.success;
    } catch (error) {
      console.error("Error creating workspace:", error);
      setError(
        error instanceof Error ? error.message : "An unexpected error occurred",
      );
      return false;
    } finally {
      setCheckoutLoading(false);
    }
  };

  // Handle repo selection change - fetch branches for the selected repo
  const handleRepoChange = async (url: string) => {
    setGithubUrl(url);
    // Don't clear selected branch - we'll validate it after fetching branches
    // This preserves the user's branch selection when switching between repos
    // that might have the same branch names (e.g., main, master)
    setSelectedRepoBranches([]);

    if (!url) return;

    setBranchesLoading(true);
    setError(null);
    try {
      // Check if this is a current repository (already checked out)
      const repositories = gitStatus.repositories as
        | Record<string, LocalRepository>
        | undefined;
      const currentRepo = Object.values(repositories ?? {}).find(
        (repo: LocalRepository) =>
          (repo.id.startsWith("local/") ? repo.id : repo.url) === url,
      );

      let branches: string[] = [];
      let repoId: string;

      if (currentRepo) {
        // For checked out repos, use the repository ID
        repoId = currentRepo.id;
        branches = await gitApi.fetchBranches(repoId);

        // Only set default branch if no branch is currently selected
        // This respects user selections while providing a sensible default
        if (!selectedBranch) {
          if (
            currentRepo.default_branch &&
            branches.includes(currentRepo.default_branch)
          ) {
            setSelectedBranch(currentRepo.default_branch);
          } else if (branches.length > 0) {
            // Look for common default branch names first
            const defaultCandidate = branches.find(
              (branch) => branch === "main" || branch === "master",
            );
            setSelectedBranch(defaultCandidate || branches[0]);
          }
        } else {
          // Verify the selected branch still exists in the new repo's branches
          if (!branches.includes(selectedBranch)) {
            // Selected branch doesn't exist in this repo, reset to default
            if (
              currentRepo.default_branch &&
              branches.includes(currentRepo.default_branch)
            ) {
              setSelectedBranch(currentRepo.default_branch);
            } else if (branches.length > 0) {
              const defaultCandidate = branches.find(
                (branch) => branch === "main" || branch === "master",
              );
              setSelectedBranch(defaultCandidate || branches[0]);
            } else {
              setSelectedBranch("");
            }
          }
        }
      } else {
        // For remote GitHub repos that haven't been checked out yet
        let repoPath = "";
        if (url.startsWith("https://github.com/")) {
          // Extract org/repo from full GitHub URL
          const match = url.match(/github\.com\/([^/]+\/[^/]+)/);
          if (match) {
            repoPath = match[1].replace(/\.git$/, "");
          }
        } else if (url.includes("/") && !url.startsWith("local/")) {
          // Already in org/repo format
          repoPath = url;
        }

        if (repoPath) {
          repoId = repoPath;
          branches = await gitApi.fetchBranches(repoId);

          // For remote repos, only set default if no branch is selected
          if (!selectedBranch && branches.length > 0) {
            // Look for common default branch names
            const defaultBranch = branches.find(
              (branch) => branch === "main" || branch === "master",
            );
            setSelectedBranch(defaultBranch || branches[0]);
          } else if (selectedBranch && !branches.includes(selectedBranch)) {
            // Selected branch doesn't exist in this repo, reset to default
            if (branches.length > 0) {
              const defaultBranch = branches.find(
                (branch) => branch === "main" || branch === "master",
              );
              setSelectedBranch(defaultBranch || branches[0]);
            } else {
              setSelectedBranch("");
            }
          }
        }
      }

      setSelectedRepoBranches(branches);
    } catch (error) {
      console.error("Failed to fetch branches:", error);
      setSelectedRepoBranches([]);
      setError("Failed to fetch branches. Please check the repository URL.");
    } finally {
      setBranchesLoading(false);
    }
  };

  const getCurrentBranch = () => {
    const repositories = gitStatus.repositories as
      | Record<string, LocalRepository>
      | undefined;
    const currentRepo = Object.values(repositories ?? {}).find(
      (repo: LocalRepository) =>
        (repo.id.startsWith("local/") ? repo.id : repo.url) === githubUrl,
    );

    if (currentRepo?.id.startsWith("local/")) {
      // For local repos, get the current branch from worktrees
      const worktrees = useAppStore.getState().getWorktreesList();
      const repoWorktrees = worktrees.filter(
        (wt: any) => wt.repo_id === currentRepo.id,
      );
      return repoWorktrees.length > 0
        ? repoWorktrees[0].source_branch
        : undefined;
    }
    return currentRepo?.default_branch;
  };

  const getDefaultBranch = () => {
    const repositories = gitStatus.repositories as
      | Record<string, LocalRepository>
      | undefined;
    const currentRepo = Object.values(repositories ?? {}).find(
      (repo: LocalRepository) =>
        (repo.id.startsWith("local/") ? repo.id : repo.url) === githubUrl,
    );
    return currentRepo?.default_branch;
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>Create New Workspace</DialogTitle>
          <DialogDescription>
            Start from a template or clone an existing repository
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
            {error}
          </div>
        )}

        <Tabs
          value={activeTab}
          onValueChange={(v) => setActiveTab(v as "repository" | "template")}
        >
          <TabsList className="grid w-full grid-cols-2 mb-2.5">
            <TabsTrigger value="repository">
              <GitBranch className="w-4 h-4 mr-2" />
              Repository
            </TabsTrigger>
            <TabsTrigger value="template">
              <FileCode className="w-4 h-4 mr-2" />
              Template
            </TabsTrigger>
          </TabsList>

          <TabsContent value="repository" className="space-y-4">
            <div className="flex gap-2 w-full overflow-hidden">
              <div className="flex-1 min-w-0 max-w-[320px]">
                <RepoSelector
                  value={githubUrl}
                  onValueChange={handleRepoChange}
                  repositories={githubRepos}
                  currentRepositories={currentGithubRepos}
                  loading={false}
                />
              </div>
              <div className="w-32 flex-shrink-0">
                <BranchDisplay
                  value={selectedBranch || "main"}
                  onValueChange={setSelectedBranch}
                  branches={selectedRepoBranches}
                  currentBranch={getCurrentBranch()}
                  defaultBranch={getDefaultBranch()}
                  loading={branchesLoading}
                />
              </div>
            </div>

            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={checkoutLoading}
              >
                Cancel
              </Button>
              <Button
                onClick={() => handleCheckout(githubUrl, selectedBranch)}
                disabled={!githubUrl || !selectedBranch || checkoutLoading}
              >
                {checkoutLoading ? (
                  <Loader2 className="animate-spin mr-2 h-4 w-4" />
                ) : null}
                Create Workspace
              </Button>
            </div>
          </TabsContent>

          <TabsContent value="template" className="space-y-4">
            {!selectedTemplate ? (
              <TemplateSelector onSelectTemplate={setSelectedTemplate} />
            ) : (
              <>
                <div className="flex items-center gap-2 p-4 border rounded-md bg-muted/50">
                  <span className="text-2xl">{selectedTemplate.icon}</span>
                  <div className="flex-1">
                    <h4 className="font-semibold">{selectedTemplate.name}</h4>
                    <p className="text-sm text-muted-foreground">
                      {selectedTemplate.description}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setSelectedTemplate(null);
                      setProjectName("");
                    }}
                    className="h-8 w-8 p-0"
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="project-name">Project Name</Label>
                  <Input
                    id="project-name"
                    placeholder="my-awesome-project"
                    value={projectName}
                    onChange={(e) => setProjectName(e.target.value)}
                    onKeyDown={(e) => {
                      if (
                        e.key === "Enter" &&
                        selectedTemplate &&
                        projectName.trim()
                      ) {
                        void handleTemplateCreate();
                      }
                    }}
                  />
                  <p className="text-xs text-muted-foreground">
                    This will be the name of your git repository
                  </p>
                </div>
              </>
            )}

            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={checkoutLoading}
              >
                Cancel
              </Button>
              <Button
                onClick={handleTemplateCreate}
                disabled={
                  !selectedTemplate || !projectName.trim() || checkoutLoading
                }
              >
                {checkoutLoading ? (
                  <Loader2 className="animate-spin mr-2 h-4 w-4" />
                ) : null}
                Create from Template
              </Button>
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
