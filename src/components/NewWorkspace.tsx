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

  // Fetch GitHub repositories on component mount
  useEffect(() => {
    if (hasFetchedGithubRepos.current) return;

    const fetchGithubRepos = async () => {
      try {
        hasFetchedGithubRepos.current = true;
        setGithubLoading(true);
        const repos = await gitApi.fetchRepositories();
        setGithubRepos(repos);
      } catch (error) {
        console.error("Failed to fetch GitHub repositories:", error);
        hasFetchedGithubRepos.current = false; // Reset on error so we can retry
      } finally {
        setGithubLoading(false);
      }
    };

    void fetchGithubRepos();
  }, []);

  // Find the repository of the most recently updated workspace
  useEffect(() => {
    if (availableRepositories.length === 0) {
      return;
    }

    // Get workspaces sorted by most recent access/creation
    const worktrees = getWorktreesList()
      .filter((worktree) => {
        const repository = getRepositoryById(worktree.repo_id);
        return repository && repository.available;
      })
      .sort((a, b) => {
        const aAccessed = new Date(a.last_accessed || a.created_at).getTime();
        const bAccessed = new Date(b.last_accessed || b.created_at).getTime();
        return bAccessed - aAccessed; // Most recent first
      });

    // Get the repository of the most recently updated workspace, or first available repo if no workspaces
    let defaultRepo = availableRepositories[0];

    if (worktrees.length > 0) {
      const mostRecentWorktree = worktrees[0];
      const mostRecentRepo = getRepositoryById(mostRecentWorktree.repo_id);
      if (mostRecentRepo && mostRecentRepo.available) {
        defaultRepo = mostRecentRepo;
      }
    }

    setSelectedRepo(defaultRepo.id);

    // Set default branch (prefer "main", then "master", then first available)
    const defaultBranch = defaultRepo.default_branch || "main";
    setSelectedBranch(defaultBranch);

    // For now, we'll mock some common branches - in a real implementation,
    // this would fetch from the repository
    const mockBranches = [defaultBranch, "main", "master", "develop", "dev"];
    const uniqueBranches = [...new Set(mockBranches)];
    setAvailableBranches(uniqueBranches);
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
