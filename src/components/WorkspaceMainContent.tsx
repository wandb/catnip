import { useEffect, useCallback, useState } from "react";
import { Helmet } from "react-helmet-async";
import { WorkspaceTerminal } from "@/components/WorkspaceTerminal";
import { WorkspaceClaude } from "@/components/WorkspaceClaude";
import { WorkspaceDiffViewer } from "@/components/WorkspaceDiffViewer";
import { PortPreview } from "@/components/PortPreview";
import { Button } from "@/components/ui/button";
import { SidebarTrigger } from "@/components/ui/sidebar";
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from "@/components/ui/resizable";
import { useSidebar } from "@/hooks/use-sidebar";
import { useWorktreeDiff } from "@/hooks/useWorktreeDiff";
import { Eye, ChevronDown, ChevronUp, Plus, X } from "lucide-react";
import type { Worktree, LocalRepository } from "@/lib/git-api";

interface TerminalTab {
  id: string;
  name: string;
}

interface WorkspaceMainContentProps {
  worktree: Worktree;
  repository: LocalRepository;
  showDiffView: boolean;
  setShowDiffView: (showDiff: boolean) => void;
  showPortPreview: number | null;
  setShowPortPreview: (port: number | null) => void;
  selectedFile?: string;
  setSelectedFile?: (file: string | undefined) => void;
}

export function WorkspaceMainContent({
  worktree,
  repository: _repository,
  showDiffView,
  setShowDiffView,
  showPortPreview,
  setShowPortPreview,
  selectedFile,
  setSelectedFile,
}: WorkspaceMainContentProps) {
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";
  const [isTerminalMinimized, setIsTerminalMinimized] = useState(false);
  const [previousTerminalSize, setPreviousTerminalSize] = useState(30);
  const [terminalSize, setTerminalSize] = useState(30);

  // Centralized focus detection for the entire workspace
  const [isFocused, setIsFocused] = useState(true);

  // Centralized focus detection effect
  useEffect(() => {
    const handleFocus = () => {
      setIsFocused(true);
    };

    const handleBlur = () => {
      setIsFocused(false);
    };

    const handleVisibilityChange = () => {
      setIsFocused(!document.hidden);
    };

    window.addEventListener("focus", handleFocus);
    window.addEventListener("blur", handleBlur);
    document.addEventListener("visibilitychange", handleVisibilityChange);

    // Send initial focus state after a brief delay to avoid race conditions during connection setup
    const focusTimer = setTimeout(() => {
      setIsFocused(document.hasFocus() && !document.hidden);
    }, 100);

    return () => {
      clearTimeout(focusTimer);
      window.removeEventListener("focus", handleFocus);
      window.removeEventListener("blur", handleBlur);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, []);

  // Terminal tabs state
  const [terminals, setTerminals] = useState<TerminalTab[]>([
    { id: "default", name: "Terminal 1" },
  ]);
  const [activeTerminal, setActiveTerminal] = useState("default");

  // Terminal management functions
  const addTerminal = useCallback(() => {
    if (terminals.length >= 3) return;

    const newId = `terminal-${terminals.length + 1}`;
    const newTerminal = {
      id: newId,
      name: `Terminal ${terminals.length + 1}`,
    };

    setTerminals((prev) => [...prev, newTerminal]);
    setActiveTerminal(newId);
  }, [terminals.length]);

  const removeTerminal = useCallback(
    (terminalId: string) => {
      if (terminals.length <= 1) return; // Always keep at least one terminal

      setTerminals((prev) => prev.filter((t) => t.id !== terminalId));

      // If removing active terminal, switch to first remaining terminal
      if (activeTerminal === terminalId) {
        const remaining = terminals.filter((t) => t.id !== terminalId);
        setActiveTerminal(remaining[0]?.id || "default");
      }
    },
    [terminals, activeTerminal],
  );

  // Get diff stats to check if we have changes
  const { diffStats } = useWorktreeDiff(
    worktree.id,
    worktree.commit_hash,
    worktree.is_dirty,
  );

  const hasChanges =
    diffStats && diffStats.file_diffs && diffStats.file_diffs.length > 0;

  // Track panel sizes
  const handlePanelLayout = useCallback(
    (sizes: number[]) => {
      // The terminal panel is the second panel (index 1)
      if (sizes[1] && !isTerminalMinimized) {
        setTerminalSize(sizes[1]);
      }
    },
    [isTerminalMinimized],
  );

  // Handle minimize/expand toggle
  const toggleTerminalMinimized = useCallback(() => {
    if (!isTerminalMinimized) {
      // Save current size before minimizing
      setPreviousTerminalSize(terminalSize);
    }
    setIsTerminalMinimized(!isTerminalMinimized);
  }, [isTerminalMinimized, terminalSize]);

  // Generate page title based on session title
  const pageTitle = worktree.session_title?.title
    ? `${worktree.session_title.title} - Catnip`
    : `${worktree.name} - Catnip`;

  return (
    <div className="flex flex-1 flex-col h-screen overflow-hidden">
      <Helmet>
        <title>{pageTitle}</title>
      </Helmet>
      <ResizablePanelGroup
        direction="vertical"
        className="h-full"
        onLayout={handlePanelLayout}
        key={isTerminalMinimized ? "minimized" : "expanded"}
      >
        {/* Main Content Area - Claude Session or Diff View */}
        <ResizablePanel
          defaultSize={isTerminalMinimized ? 95 : 100 - previousTerminalSize}
          minSize={30}
        >
          <div className="h-full bg-muted/50 overflow-hidden">
            {showPortPreview ? (
              <div className="h-full">
                <PortPreview
                  port={showPortPreview}
                  onClose={() => setShowPortPreview(null)}
                />
              </div>
            ) : showDiffView ? (
              <div className="h-full">
                <WorkspaceDiffViewer
                  worktree={worktree}
                  selectedFile={selectedFile}
                  onClose={() => {
                    setShowDiffView(false);
                    setSelectedFile?.(undefined);
                  }}
                />
              </div>
            ) : (
              <ResizablePanelGroup direction="vertical" className="h-full">
                <ResizablePanel defaultSize={100} minSize={100}>
                  <div className="h-full flex flex-col">
                    <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          {isCollapsed && (
                            <SidebarTrigger className="h-4 w-4" />
                          )}
                          <img
                            src="/anthropic.png"
                            alt="Claude"
                            className="w-4 h-4"
                          />
                          <span className="text-sm font-medium">Claude</span>
                          {worktree.session_title && (
                            <span className="text-xs text-muted-foreground">
                              - {worktree.session_title.title}
                            </span>
                          )}
                        </div>
                        {/* Eyeball toggle - only show if we have changes */}
                        {hasChanges && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setShowDiffView(true)}
                            className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground"
                            title="View diff"
                          >
                            <Eye className="w-3 h-3" />
                          </Button>
                        )}
                      </div>
                    </div>
                    <div className="flex-1 overflow-hidden">
                      <WorkspaceClaude
                        worktree={worktree}
                        isFocused={isFocused}
                      />
                    </div>
                  </div>
                </ResizablePanel>
              </ResizablePanelGroup>
            )}
          </div>
        </ResizablePanel>

        {/* Resizable Handle */}
        {!isTerminalMinimized && (
          <ResizableHandle className="bg-border hover:bg-accent transition-colors cursor-row-resize" />
        )}

        {/* Terminal */}
        <ResizablePanel
          defaultSize={isTerminalMinimized ? 5 : previousTerminalSize}
          minSize={isTerminalMinimized ? 0 : 15}
          maxSize={isTerminalMinimized ? 5 : 70}
          collapsible={true}
        >
          <div className="flex flex-col bg-muted/50 overflow-hidden h-full">
            <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm flex-shrink-0">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 flex-1 min-w-0">
                  {isCollapsed && <SidebarTrigger className="h-4 w-4" />}

                  {/* Terminal Tabs */}
                  <div className="flex items-center gap-1 flex-1 min-w-0">
                    {terminals.map((terminal) => (
                      <div
                        key={terminal.id}
                        className={`flex items-center gap-1 px-2 py-1 rounded-md text-xs cursor-pointer transition-colors ${
                          activeTerminal === terminal.id
                            ? "bg-background text-foreground border border-border"
                            : "text-muted-foreground hover:text-foreground hover:bg-background/50"
                        }`}
                        onClick={() => setActiveTerminal(terminal.id)}
                      >
                        <span className="truncate max-w-[80px]">
                          {terminal.name}
                        </span>
                        {terminals.length > 1 && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              removeTerminal(terminal.id);
                            }}
                            className="ml-1 p-0.5 rounded hover:bg-destructive/20 hover:text-destructive transition-colors"
                            title={`Close ${terminal.name}`}
                          >
                            <X className="w-2.5 h-2.5" />
                          </button>
                        )}
                      </div>
                    ))}

                    {/* Add Terminal Button */}
                    {terminals.length < 3 && (
                      <button
                        onClick={addTerminal}
                        className="flex items-center justify-center w-6 h-6 rounded-md text-muted-foreground hover:text-foreground hover:bg-background/50 transition-colors"
                        title="Add new terminal (max 3)"
                      >
                        <Plus className="w-3 h-3" />
                      </button>
                    )}
                  </div>

                  <span className="text-xs text-muted-foreground ml-2">
                    {worktree.path}
                  </span>
                </div>

                <Button
                  variant="ghost"
                  size="sm"
                  onClick={toggleTerminalMinimized}
                  className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground ml-2"
                  title={
                    isTerminalMinimized
                      ? "Expand terminal"
                      : "Minimize terminal"
                  }
                >
                  {isTerminalMinimized ? (
                    <ChevronUp className="w-3 h-3" />
                  ) : (
                    <ChevronDown className="w-3 h-3" />
                  )}
                </Button>
              </div>
            </div>
            {!isTerminalMinimized && (
              <div className="flex-1 min-h-0 pb-2 bg-black relative">
                {terminals.map((terminal) => (
                  <div
                    key={terminal.id}
                    className={`absolute inset-0 ${
                      activeTerminal === terminal.id ? "block" : "hidden"
                    }`}
                  >
                    <WorkspaceTerminal
                      worktree={worktree}
                      terminalId={terminal.id}
                      isFocused={isFocused}
                    />
                  </div>
                ))}
              </div>
            )}
          </div>
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  );
}
