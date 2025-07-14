import { useState, useCallback } from "react";
import { GitStatus, Worktree, Repository } from "@/types/git";

export const useGitState = () => {
  const [gitStatus, setGitStatus] = useState<GitStatus>({});
  const [worktrees, setWorktrees] = useState<Worktree[]>([]);
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [repoBranches, setRepoBranches] = useState<Record<string, string[]>>({});
  const [claudeSessions, setClaudeSessions] = useState<Record<string, any>>({});
  const [activeSessions, setActiveSessions] = useState<Record<string, any>>({});
  const [syncConflicts, setSyncConflicts] = useState<Record<string, any>>({});
  const [mergeConflicts, setMergeConflicts] = useState<Record<string, any>>({});
  const [loading, setLoading] = useState(false);
  const [reposLoading, setReposLoading] = useState(false);
  const [openDiffWorktreeId, setOpenDiffWorktreeId] = useState<string | null>(null);

  const [confirmDialog, setConfirmDialog] = useState<{
    open: boolean;
    title: string;
    description: string;
    onConfirm: () => void;
    variant?: "default" | "destructive";
  }>({
    open: false,
    title: "",
    description: "",
    onConfirm: () => {},
  });

  const [errorAlert, setErrorAlert] = useState<{
    open: boolean;
    title: string;
    description: string;
  }>({
    open: false,
    title: "",
    description: "",
  });

  const [prDialog, setPrDialog] = useState<{
    open: boolean;
    worktreeId: string;
    branchName: string;
    title: string;
    description: string;
  }>({
    open: false,
    worktreeId: "",
    branchName: "",
    title: "",
    description: "",
  });

  const showConfirmDialog = useCallback((config: {
    title: string;
    description: string;
    onConfirm: () => void;
    variant?: "default" | "destructive";
  }) => {
    setConfirmDialog({
      open: true,
      ...config,
    });
  }, []);

  const showErrorAlert = useCallback((title: string, description: string) => {
    setErrorAlert({
      open: true,
      title,
      description,
    });
  }, []);

  const openPrDialog = useCallback((worktreeId: string, branchName: string) => {
    setPrDialog({
      open: true,
      worktreeId,
      branchName,
      title: `Pull request from ${branchName}`,
      description: `Automated pull request created from worktree ${branchName}`,
    });
  }, []);

  const closePrDialog = useCallback(() => {
    setPrDialog({
      open: false,
      worktreeId: "",
      branchName: "",
      title: "",
      description: "",
    });
  }, []);

  const toggleDiff = useCallback((worktreeId: string) => {
    setOpenDiffWorktreeId(prev => prev === worktreeId ? null : worktreeId);
  }, []);

  return {
    // State
    gitStatus,
    worktrees,
    repositories,
    repoBranches,
    claudeSessions,
    activeSessions,
    syncConflicts,
    mergeConflicts,
    loading,
    reposLoading,
    openDiffWorktreeId,
    confirmDialog,
    errorAlert,
    prDialog,
    
    // Setters
    setGitStatus,
    setWorktrees,
    setRepositories,
    setRepoBranches,
    setClaudeSessions,
    setActiveSessions,
    setSyncConflicts,
    setMergeConflicts,
    setLoading,
    setReposLoading,
    setConfirmDialog,
    setErrorAlert,
    setPrDialog,
    
    // Actions
    showConfirmDialog,
    showErrorAlert,
    openPrDialog,
    closePrDialog,
    toggleDiff,
  };
};