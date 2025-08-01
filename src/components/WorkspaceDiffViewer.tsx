import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import {
  ChevronDown,
  ChevronRight,
  FileText,
  X,
  Copy,
  Check,
  Terminal,
} from "lucide-react";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { useSidebar } from "@/hooks/use-sidebar";
import { useWorktreeDiff } from "@/hooks/useWorktreeDiff";
import type { Worktree } from "@/lib/git-api";

interface WorkspaceDiffViewerProps {
  worktree: Worktree;
  onClose: () => void;
}

interface DiffLine {
  type: "context" | "add" | "remove" | "header";
  oldLineNumber?: number;
  newLineNumber?: number;
  content: string;
  isNoNewlineAtEnd?: boolean;
}

interface FileDiffStats {
  additions: number;
  deletions: number;
  totalChanges: number;
}

// Maximum lines of changes before auto-collapsing
const MAX_LINES_TO_AUTO_EXPAND = 500;

export function WorkspaceDiffViewer({
  worktree,
  onClose,
}: WorkspaceDiffViewerProps) {
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";

  const { diffStats, loading, error } = useWorktreeDiff(
    worktree.id,
    worktree.commit_hash,
    worktree.is_dirty,
  );

  const [expandedFiles, setExpandedFiles] = useState<Set<string>>(new Set());
  const [copiedLine, setCopiedLine] = useState<string | null>(null);

  // Auto-expand files when data loads, but only if under the threshold
  useEffect(() => {
    if (diffStats?.file_diffs) {
      const autoExpanded = new Set<string>();
      diffStats.file_diffs.forEach((file) => {
        const stats = calculateFileStats(file.diff_text || "");
        if (stats.totalChanges <= MAX_LINES_TO_AUTO_EXPAND) {
          autoExpanded.add(file.file_path);
        }
      });
      setExpandedFiles(autoExpanded);
    }
  }, [diffStats]);

  const toggleFileExpansion = (filePath: string) => {
    setExpandedFiles((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(filePath)) {
        newSet.delete(filePath);
      } else {
        newSet.add(filePath);
      }
      return newSet;
    });
  };

  const calculateFileStats = (diffText: string): FileDiffStats => {
    const lines = diffText.split("\n");
    let additions = 0;
    let deletions = 0;

    for (const line of lines) {
      if (line.startsWith("+") && !line.startsWith("+++")) {
        additions++;
      } else if (line.startsWith("-") && !line.startsWith("---")) {
        deletions++;
      }
    }

    return {
      additions,
      deletions,
      totalChanges: additions + deletions,
    };
  };

  const parseDiffText = (diffText: string): DiffLine[] => {
    const lines = diffText.split("\n");
    const result: DiffLine[] = [];
    let oldLineNumber = 0;
    let newLineNumber = 0;

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];

      // Parse hunk headers like @@ -1,3 +1,84 @@
      if (line.startsWith("@@")) {
        const match = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
        if (match) {
          oldLineNumber = parseInt(match[1], 10);
          newLineNumber = parseInt(match[2], 10);
        }
        result.push({
          type: "header",
          content: line,
        });
        continue;
      }

      // Skip file headers
      if (line.startsWith("---") || line.startsWith("+++")) {
        continue;
      }

      // Handle different line types
      if (line.startsWith("+")) {
        result.push({
          type: "add",
          newLineNumber: newLineNumber,
          content: line.slice(1), // Remove the + prefix
        });
        newLineNumber++;
      } else if (line.startsWith("-")) {
        result.push({
          type: "remove",
          oldLineNumber: oldLineNumber,
          content: line.slice(1), // Remove the - prefix
        });
        oldLineNumber++;
      } else {
        // Context line (unchanged)
        result.push({
          type: "context",
          oldLineNumber: oldLineNumber,
          newLineNumber: newLineNumber,
          content: line.startsWith(" ") ? line.slice(1) : line,
        });
        oldLineNumber++;
        newLineNumber++;
      }
    }

    return result;
  };

  const copyToClipboard = async (content: string, lineId: string) => {
    try {
      await navigator.clipboard.writeText(content);
      setCopiedLine(lineId);
      setTimeout(() => setCopiedLine(null), 2000);
    } catch (error) {
      console.error("Failed to copy to clipboard:", error);
    }
  };

  const getFileIcon = () => {
    return <FileText className="w-4 h-4 text-muted-foreground" />;
  };

  if (loading) {
    return (
      <div className="h-full flex items-center justify-center bg-background">
        <div className="flex items-center gap-3">
          <div className="animate-spin rounded-full h-6 w-6 border-2 border-primary border-t-transparent"></div>
          <span className="text-sm text-muted-foreground">Loading diff...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-full flex items-center justify-center bg-background">
        <div className="text-center space-y-4">
          <div className="text-destructive text-sm">Failed to load diff</div>
          <Button variant="outline" size="sm" onClick={onClose}>
            <Terminal className="w-4 h-4 mr-2" />
            Back to Claude
          </Button>
        </div>
      </div>
    );
  }

  if (!diffStats || !diffStats.file_diffs?.length) {
    return (
      <div className="h-full flex items-center justify-center bg-background">
        <div className="text-center space-y-4">
          <FileText className="w-12 h-12 mx-auto text-muted-foreground/50" />
          <div className="text-sm text-muted-foreground">
            No changes to show
          </div>
          <Button variant="outline" size="sm" onClick={onClose}>
            <Terminal className="w-4 h-4 mr-2" />
            Back to Claude
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full bg-background flex flex-col">
      {/* Compact Header - like Claude header */}
      <div className="flex-shrink-0 px-4 py-2 border-b bg-background/50 backdrop-blur-sm">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {isCollapsed && <SidebarTrigger className="h-4 w-4" />}
            <FileText className="w-4 h-4 text-muted-foreground" />
            <span className="text-sm font-medium">Diff</span>
            <span className="text-xs text-muted-foreground">
              - {diffStats.summary}
            </span>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={onClose}
            className="h-6 w-6 p-0"
          >
            <X className="w-3 h-3" />
          </Button>
        </div>
      </div>

      {/* Files */}
      <div className="flex-1 overflow-auto">
        <div className="p-4 space-y-4">
          {diffStats.file_diffs.map((file) => {
            const isExpanded = expandedFiles.has(file.file_path);
            const stats = calculateFileStats(file.diff_text || "");
            const isLargeFile = stats.totalChanges > MAX_LINES_TO_AUTO_EXPAND;

            return (
              <div
                key={file.file_path}
                className="border border-border rounded-lg overflow-hidden bg-card"
              >
                {/* Compact File Header */}
                <div
                  className="flex items-center gap-2 px-3 py-2 bg-muted/10 border-b cursor-pointer hover:bg-muted/20 transition-colors"
                  onClick={() => toggleFileExpansion(file.file_path)}
                >
                  <div className="flex items-center gap-2 flex-1 min-w-0">
                    {isExpanded ? (
                      <ChevronDown className="w-3 h-3 text-muted-foreground flex-shrink-0" />
                    ) : (
                      <ChevronRight className="w-3 h-3 text-muted-foreground flex-shrink-0" />
                    )}
                    {getFileIcon()}
                    <span className="font-mono text-xs font-medium truncate">
                      {file.file_path}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    {/* GitHub-style colored bars */}
                    {(stats.additions > 0 || stats.deletions > 0) && (
                      <div className="flex items-center gap-2">
                        <div className="flex items-center gap-px">
                          {(() => {
                            const total = stats.totalChanges;
                            const greenBars = Math.min(
                              5,
                              Math.round((stats.additions / total) * 5),
                            );
                            const redBars = Math.min(
                              5 - greenBars,
                              Math.round((stats.deletions / total) * 5),
                            );
                            const grayBars = 5 - greenBars - redBars;

                            return [
                              ...Array(greenBars)
                                .fill(0)
                                .map((_, i) => (
                                  <div
                                    key={`add-${i}`}
                                    className="w-2 h-2 bg-green-500 rounded-[1px]"
                                  />
                                )),
                              ...Array(redBars)
                                .fill(0)
                                .map((_, i) => (
                                  <div
                                    key={`del-${i}`}
                                    className="w-2 h-2 bg-red-500 rounded-[1px]"
                                  />
                                )),
                              ...Array(grayBars)
                                .fill(0)
                                .map((_, i) => (
                                  <div
                                    key={`empty-${i}`}
                                    className="w-2 h-2 bg-muted/30 rounded-[1px]"
                                  />
                                )),
                            ];
                          })()}
                        </div>
                        <div className="text-xs font-mono text-muted-foreground">
                          {stats.additions > 0 && (
                            <span className="text-green-600">
                              +{stats.additions}
                            </span>
                          )}
                          {stats.deletions > 0 && (
                            <span className="text-red-600 ml-1">
                              -{stats.deletions}
                            </span>
                          )}
                        </div>
                      </div>
                    )}
                    {isLargeFile && (
                      <span className="text-xs text-muted-foreground">
                        ({stats.totalChanges})
                      </span>
                    )}
                  </div>
                </div>

                {/* File Diff Content */}
                {isExpanded && file.diff_text && (
                  <div className="bg-background">
                    {parseDiffText(file.diff_text).map((line, lineIndex) => {
                      const lineId = `${file.file_path}-${lineIndex}`;

                      if (line.type === "header") {
                        return (
                          <div
                            key={lineIndex}
                            className="bg-muted/30 text-muted-foreground px-4 py-1 text-xs font-mono border-b"
                          >
                            {line.content}
                          </div>
                        );
                      }

                      const bgColor =
                        line.type === "add"
                          ? "bg-green-50 dark:bg-green-950/30"
                          : line.type === "remove"
                            ? "bg-red-50 dark:bg-red-950/30"
                            : "bg-background";

                      const lineNumberBg =
                        line.type === "add"
                          ? "bg-green-100 dark:bg-green-900/40"
                          : line.type === "remove"
                            ? "bg-red-100 dark:bg-red-900/40"
                            : "bg-muted/20";

                      const textColor =
                        line.type === "add"
                          ? "text-green-800 dark:text-green-200"
                          : line.type === "remove"
                            ? "text-red-800 dark:text-red-200"
                            : "text-foreground";

                      const prefix =
                        line.type === "add"
                          ? "+"
                          : line.type === "remove"
                            ? "-"
                            : " ";

                      return (
                        <div
                          key={lineIndex}
                          className={`flex hover:bg-muted/50 group ${bgColor}`}
                        >
                          {/* Line Numbers - GitHub style with matching backgrounds */}
                          <div className="flex-shrink-0 flex">
                            <div
                              className={`w-12 px-2 py-1 text-xs text-muted-foreground/60 text-right font-mono select-none ${lineNumberBg}`}
                            >
                              {line.oldLineNumber || ""}
                            </div>
                            <div
                              className={`w-12 px-2 py-1 text-xs text-muted-foreground/60 text-right font-mono select-none ${lineNumberBg}`}
                            >
                              {line.newLineNumber || ""}
                            </div>
                          </div>

                          {/* Content */}
                          <div className="flex-1 min-w-0 flex">
                            <span
                              className={`w-4 text-center text-xs font-mono select-none ${textColor}`}
                            >
                              {prefix}
                            </span>
                            <pre
                              className={`flex-1 px-2 py-1 text-xs font-mono whitespace-pre-wrap break-all ${textColor}`}
                            >
                              {line.content || " "}
                            </pre>

                            {/* Copy Button */}
                            <div className="flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-auto p-1 text-muted-foreground hover:text-foreground"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  void copyToClipboard(line.content, lineId);
                                }}
                              >
                                {copiedLine === lineId ? (
                                  <Check className="w-3 h-3" />
                                ) : (
                                  <Copy className="w-3 h-3" />
                                )}
                              </Button>
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}

                {/* Collapsed state message for large files */}
                {!isExpanded && isLargeFile && (
                  <div
                    className="p-4 text-center text-sm text-muted-foreground bg-muted/10 cursor-pointer hover:bg-muted/20 hover:text-foreground transition-colors"
                    onClick={() => toggleFileExpansion(file.file_path)}
                  >
                    Large diff ({stats.totalChanges} changes) • Click to expand
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
