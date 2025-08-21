import { useState, useEffect, useMemo, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  RepoSelectorPill,
  BranchSelectorPill,
} from "@/components/PillSelector";
import { useAppStore } from "@/stores/appStore";
import { gitApi } from "@/lib/git-api";
import type { Repository } from "@/lib/git-api";

interface NewWorkspaceProps {
  onClose: () => void;
  onSubmit: (repoId: string, branch: string, prompt: string) => void;
}

export function NewWorkspace({ onClose, onSubmit }: NewWorkspaceProps) {
  const [prompt, setPrompt] = useState("");
  const [selectedRepo, setSelectedRepo] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("");
  const [availableBranches, setAvailableBranches] = useState<string[]>([]);
  const [githubRepos, setGithubRepos] = useState<Repository[]>([]);
  const [githubLoading, setGithubLoading] = useState(true);
  const hasFetchedGithubRepos = useRef(false);

  const getRepositoryById = useAppStore((state) => state.getRepositoryById);
  const getWorktreesList = useAppStore((state) => state.getWorktreesList);

  // Get available repositories
  const repositories = useAppStore((state) => state.repositories);
  const availableRepositories = useMemo(() => {
    return Array.from(repositories.values()).filter((repo) => repo.available);
  }, [repositories]);

  console.log("All repositories:", repositories);
  console.log("Available repositories:", availableRepositories);

  // Fetch GitHub repositories on component mount
  useEffect(() => {
    if (hasFetchedGithubRepos.current) return;

    const fetchGithubRepos = async () => {
      try {
        hasFetchedGithubRepos.current = true;
        setGithubLoading(true);
        const repos = await gitApi.fetchRepositories();
        setGithubRepos(repos);
        console.log("Fetched GitHub repos:", repos);
      } catch (error) {
        console.error("Failed to fetch GitHub repositories:", error);
        hasFetchedGithubRepos.current = false; // Reset on error so we can retry
      } finally {
        setGithubLoading(false);
      }
    };

    void fetchGithubRepos();
  }, []);

  // Find the repository with the most workspaces and get its default branch
  useEffect(() => {
    console.log(
      "useEffect running, availableRepositories.length:",
      availableRepositories.length,
    );
    if (availableRepositories.length === 0) {
      console.log("No available repositories, skipping default selection");
      return;
    }

    // Count workspaces per repository
    const worktrees = getWorktreesList();
    const repoWorkspaceCount: Record<string, number> = {};

    worktrees.forEach((worktree) => {
      repoWorkspaceCount[worktree.repo_id] =
        (repoWorkspaceCount[worktree.repo_id] || 0) + 1;
    });

    // Find repo with most workspaces, or first available repo if none have workspaces
    let mostUsedRepo = availableRepositories[0];
    let maxCount = 0;

    for (const repo of availableRepositories) {
      const count = repoWorkspaceCount[repo.id] || 0;
      if (count > maxCount) {
        maxCount = count;
        mostUsedRepo = repo;
      }
    }

    setSelectedRepo(mostUsedRepo.id);

    // Set default branch (prefer "main", then "master", then first available)
    const defaultBranch = mostUsedRepo.default_branch || "main";
    setSelectedBranch(defaultBranch);

    // For now, we'll mock some common branches - in a real implementation,
    // this would fetch from the repository
    const mockBranches = [defaultBranch, "main", "master", "develop", "dev"];
    const uniqueBranches = [...new Set(mockBranches)];
    setAvailableBranches(uniqueBranches);

    console.log(
      "Selected repo:",
      mostUsedRepo.id,
      "Selected branch:",
      defaultBranch,
    );
  }, [availableRepositories, getWorktreesList]);

  // Update branches when repository changes
  useEffect(() => {
    if (!selectedRepo) return;

    const repo = getRepositoryById(selectedRepo);
    if (!repo) return;

    // Mock branches for the selected repository
    const defaultBranch = repo.default_branch || "main";
    const mockBranches = [defaultBranch, "main", "master", "develop", "dev"];
    const uniqueBranches = [...new Set(mockBranches)];
    setAvailableBranches(uniqueBranches);

    // Set branch to default if not already set or if current selection isn't available
    if (!selectedBranch || !uniqueBranches.includes(selectedBranch)) {
      setSelectedBranch(defaultBranch);
    }
  }, [selectedRepo, getRepositoryById, selectedBranch]);

  const handleSubmit = () => {
    if (!selectedRepo || !selectedBranch || !prompt.trim()) return;
    onSubmit(selectedRepo, selectedBranch, prompt);
  };

  const selectedRepository = getRepositoryById(selectedRepo);

  return (
    <div className="min-h-screen bg-background flex flex-col">
      {/* Header */}
      <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="p-4 flex items-center gap-3">
          <Button variant="ghost" size="sm" onClick={onClose} className="p-2">
            <span className="text-lg font-bold">‹</span>
          </Button>
          <div className="flex-1">
            <h1 className="text-lg font-semibold">New Workspace</h1>
            <p className="text-sm text-muted-foreground">
              Choose repository and describe your task
            </p>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 p-4 space-y-4">
        {/* Repository and Branch Selectors */}
        <div className="flex gap-2 flex-wrap">
          <RepoSelectorPill
            value={selectedRepo}
            onValueChange={setSelectedRepo}
            repositories={availableRepositories}
            githubRepositories={githubRepos}
            loading={githubLoading}
          />
          <BranchSelectorPill
            value={selectedBranch}
            onValueChange={setSelectedBranch}
            branches={availableBranches}
            defaultBranch={selectedRepository?.default_branch}
          />
        </div>

        {/* Task Description */}
        <div className="space-y-2">
          <Textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Describe your task..."
            className="min-h-[120px]"
          />
        </div>

        {/* Submit Button */}
        <Button
          onClick={handleSubmit}
          disabled={!selectedRepo || !selectedBranch || !prompt.trim()}
          className="w-full"
        >
          Create Workspace
        </Button>
      </div>
    </div>
  );
}
