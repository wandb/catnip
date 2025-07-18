import { toast } from "sonner";
import { gitApi } from "@/lib/git-api";
import { parseGitUrl, showPreviewToast } from "@/lib/git-utils";

interface GitStateActions {
  addNewWorktrees: () => Promise<any[]>;
  backgroundRefreshGitStatus: () => Promise<void>;
  refreshWorktree: (
    id: string,
    options?: { includeDiffs?: boolean },
  ) => Promise<void>;
  removeWorktree: (id: string) => void;
  fetchActiveSessions: () => Promise<void>;
  setCheckoutLoading: (loading: boolean) => void;
  setSyncingWorktree: (id: string, syncing: boolean) => void;
  setMergingWorktree: (id: string, merging: boolean) => void;
}

export function useGitActions(gitStateActions: GitStateActions) {
  const {
    addNewWorktrees,
    backgroundRefreshGitStatus,
    refreshWorktree,
    removeWorktree,
    fetchActiveSessions,
    setCheckoutLoading,
    setSyncingWorktree,
    setMergingWorktree,
  } = gitStateActions;

  const checkoutRepository = async (
    url: string,
    setErrorAlert: (alert: {
      open: boolean;
      title: string;
      description: string;
    }) => void,
  ) => {
    setCheckoutLoading(true);
    try {
      const parsedUrl = parseGitUrl(url);
      if (!parsedUrl) {
        setErrorAlert({
          open: true,
          title: "Invalid URL",
          description: `Unknown repository URL format: ${url}`,
        });
        return false;
      }

      const { org, repo } = parsedUrl;
      const response = await fetch(`/v1/git/checkout/${org}/${repo}`, {
        method: "POST",
      });

      if (response.ok) {
        setCheckoutLoading(false);
        const message =
          parsedUrl.type === "local"
            ? "Local repository checked out successfully"
            : "Repository checked out successfully";
        toast.success(message);
        Promise.all([addNewWorktrees(), backgroundRefreshGitStatus()]).catch(
          console.error,
        );
        return true;
      } else {
        const errorData = (await response.json()) as { error?: string };
        console.error("Failed to checkout repository:", errorData);
        setErrorAlert({
          open: true,
          title: "Checkout Failed",
          description: `Failed to checkout repository: ${errorData.error ?? "Unknown error"}`,
        });
        return false;
      }
    } catch (error) {
      console.error("Failed to checkout repository:", error);
      setErrorAlert({
        open: true,
        title: "Checkout Failed",
        description: `Failed to checkout repository: ${String(error)}`,
      });
      return false;
    } finally {
      setCheckoutLoading(false);
    }
  };

  const deleteWorktree = async (id: string) => {
    try {
      await gitApi.deleteWorktree(id);
      removeWorktree(id);
      await fetchActiveSessions();
    } catch (error) {
      console.error("Failed to delete worktree:", error);
    }
  };

  const syncWorktree = async (
    id: string,
    setErrorAlert: (alert: {
      open: boolean;
      title: string;
      description: string;
    }) => void,
  ) => {
    setSyncingWorktree(id, true);
    try {
      const success = await gitApi.syncWorktree(id, { setErrorAlert });
      setSyncingWorktree(id, false);
      if (success) {
        refreshWorktree(id, { includeDiffs: true }).catch(console.error);
        return true;
      }
      return false;
    } catch (error) {
      console.error("Failed to sync worktree:", error);
      return false;
    } finally {
      setSyncingWorktree(id, false);
    }
  };

  const mergeWorktreeToMain = async (
    id: string,
    worktreeName: string,
    setErrorAlert: (alert: {
      open: boolean;
      title: string;
      description: string;
    }) => void,
    squash = true,
    autoCleanup = true,
  ) => {
    setMergingWorktree(id, true);
    try {
      const success = await gitApi.mergeWorktree(
        id,
        worktreeName,
        squash,
        {
          setErrorAlert,
        },
        autoCleanup,
      );
      setMergingWorktree(id, false);
      if (success) {
        Promise.all([
          refreshWorktree(id, { includeDiffs: true }),
          backgroundRefreshGitStatus(),
        ]).catch(console.error);
        return true;
      }
      return false;
    } catch (error) {
      console.error("Failed to merge worktree:", error);
      return false;
    } finally {
      setMergingWorktree(id, false);
    }
  };

  const createWorktreePreview = async (
    id: string,
    branchName: string,
    setErrorAlert: (alert: {
      open: boolean;
      title: string;
      description: string;
    }) => void,
  ) => {
    const success = await gitApi.createWorktreePreview(id, { setErrorAlert });
    if (success) {
      showPreviewToast(branchName);
    }
    return success;
  };

  return {
    checkoutRepository,
    deleteWorktree,
    syncWorktree,
    mergeWorktreeToMain,
    createWorktreePreview,
  };
}
