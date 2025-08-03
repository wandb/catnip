import { useState, useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { BranchSelector } from "@/components/BranchSelector";
import { RepoSelector } from "@/components/RepoSelector";
import { Loader2 } from "lucide-react";
import { type LocalRepository, gitApi } from "@/lib/git-api";
import { useAppStore } from "@/stores/appStore";
import { useGitApi } from "@/hooks/useGitApi";

interface NewWorkspaceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialRepoUrl?: string;
  initialBranch?: string;
}

export function NewWorkspaceDialog({
  open,
  onOpenChange,
  initialRepoUrl,
  initialBranch,
}: NewWorkspaceDialogProps) {
  const { gitStatus } = useAppStore();
  const { checkoutRepository } = useGitApi();

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

  // Reset form when dialog opens/closes
  useEffect(() => {
    if (!open) {
      setGithubUrl("");
      setSelectedBranch("");
      setSelectedRepoBranches([]);
      setCheckoutLoading(false);
      setError(null);
    } else if (open && initialRepoUrl) {
      // Set initial values when dialog opens with pre-selected repo
      setGithubUrl(initialRepoUrl);
      if (initialBranch) {
        setSelectedBranch(initialBranch);
      }
    }
  }, [open, initialRepoUrl, initialBranch]);

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

  // Handle checkout functionality
  const handleCheckout = async (url: string, branch?: string) => {
    if (!url || !branch) return false;

    setCheckoutLoading(true);
    setError(null);
    try {
      let success = false;

      // Check if this is a local repository (starts with "local/")
      if (url.startsWith("local/")) {
        // For local repos, extract the repo name
        const repoName = url.split("/")[1];
        success = await checkoutRepository("local", repoName, branch);
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
          success = await checkoutRepository(org, repo, branch);
        } else {
          setError(
            "Invalid GitHub URL format. Please use a valid GitHub URL or org/repo format.",
          );
          return false;
        }
      }

      if (success) {
        onOpenChange(false);
      } else {
        setError(
          "Failed to create workspace. Please check the repository URL and try again.",
        );
      }

      return success;
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
    setSelectedBranch("");
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

        // Set default branch as selected for current repos
        if (
          currentRepo.default_branch &&
          branches.includes(currentRepo.default_branch)
        ) {
          setSelectedBranch(currentRepo.default_branch);
        } else if (branches.length > 0) {
          setSelectedBranch(branches[0]);
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

          // For remote repos, prioritize default branches (main/master)
          if (branches.length > 0) {
            // Look for common default branch names
            const defaultBranch = branches.find(
              (branch) => branch === "main" || branch === "master",
            );
            setSelectedBranch(defaultBranch || branches[0]);
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
            Select a repository and branch to create a new workspace
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          {error && (
            <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
              {error}
            </div>
          )}
          <RepoSelector
            value={githubUrl}
            onValueChange={handleRepoChange}
            repositories={githubRepos}
            currentRepositories={currentGithubRepos}
            loading={false}
          />

          <BranchSelector
            value={selectedBranch}
            onValueChange={setSelectedBranch}
            branches={selectedRepoBranches}
            currentBranch={getCurrentBranch()}
            defaultBranch={getDefaultBranch()}
            disabled={false}
            loading={branchesLoading}
          />

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
        </div>
      </DialogContent>
    </Dialog>
  );
}
