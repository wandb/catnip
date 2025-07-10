import { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ChevronDown, ChevronRight, FileText, FilePlus, FileMinus, FileIcon, SplitSquareHorizontal, RectangleHorizontal } from 'lucide-react';
import ReactDiffViewer, { DiffMethod } from 'react-diff-viewer-continued';
import Prism from 'prismjs';

// Import common language support for syntax highlighting
import 'prismjs/components/prism-typescript';
import 'prismjs/components/prism-javascript';
import 'prismjs/components/prism-jsx';
import 'prismjs/components/prism-tsx';
import 'prismjs/components/prism-css';
import 'prismjs/components/prism-scss';
import 'prismjs/components/prism-json';
import 'prismjs/components/prism-markdown';
import 'prismjs/components/prism-yaml';
import 'prismjs/components/prism-bash';
import 'prismjs/components/prism-go';
import 'prismjs/components/prism-python';
import 'prismjs/themes/prism.css'; // Light theme
import 'prismjs/themes/prism-dark.css'; // Dark theme

interface FileDiff {
  file_path: string;
  change_type: string;
  old_content?: string;
  new_content?: string;
  diff_text?: string;
  is_expanded: boolean;
}

interface WorktreeDiffResponse {
  worktree_id: string;
  worktree_name: string;
  source_branch: string;
  fork_commit: string;
  file_diffs: FileDiff[];
  total_files: number;
  summary: string;
}

interface DiffViewerProps {
  worktreeId: string;
  isOpen: boolean;
  onClose: () => void;
}

export function DiffViewer({ worktreeId, isOpen, onClose }: DiffViewerProps) {
  const [diffData, setDiffData] = useState<WorktreeDiffResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expandedFiles, setExpandedFiles] = useState<Set<string>>(new Set());
  const [splitView, setSplitView] = useState(false);
  const [isWideScreen, setIsWideScreen] = useState(false);

  // Check screen width for split view option and set defaults
  useEffect(() => {
    const checkScreenWidth = () => {
      const isWide = window.innerWidth >= 1024; // lg breakpoint (lowered from xl)
      setIsWideScreen(isWide);
      
      // Default to split view on wide screens, unified on narrow screens
      setSplitView(isWide);
    };
    
    checkScreenWidth();
    window.addEventListener('resize', checkScreenWidth);
    return () => window.removeEventListener('resize', checkScreenWidth);
  }, []);

  const fetchDiff = async () => {
    if (!isOpen) return;
    
    setLoading(true);
    setError(null);
    
    try {
      const response = await fetch(`/v1/git/worktrees/${worktreeId}/diff`);
      if (!response.ok) {
        throw new Error(`Failed to fetch diff: ${response.statusText}`);
      }
      
      const data = await response.json();
      setDiffData(data);
      
      // Auto-expand files that should be expanded by default
      const autoExpanded = new Set<string>();
      data.file_diffs.forEach((file: FileDiff) => {
        if (file.is_expanded) {
          autoExpanded.add(file.file_path);
        }
      });
      setExpandedFiles(autoExpanded);
      
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch diff');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchDiff();
  }, [worktreeId, isOpen]);

  const toggleFileExpansion = (filePath: string) => {
    setExpandedFiles(prev => {
      const newSet = new Set(prev);
      if (newSet.has(filePath)) {
        newSet.delete(filePath);
      } else {
        newSet.add(filePath);
      }
      return newSet;
    });
  };

  const getLanguageFromFilePath = (filePath: string): string => {
    const ext = filePath.split('.').pop()?.toLowerCase();
    switch (ext) {
      case 'ts':
      case 'tsx':
        return 'typescript';
      case 'js':
      case 'jsx':
        return 'javascript';
      case 'css':
        return 'css';
      case 'scss':
      case 'sass':
        return 'scss';
      case 'json':
        return 'json';
      case 'md':
        return 'markdown';
      case 'yml':
      case 'yaml':
        return 'yaml';
      case 'sh':
      case 'bash':
        return 'bash';
      case 'go':
        return 'go';
      case 'py':
        return 'python';
      default:
        return 'javascript'; // Default fallback
    }
  };

  const highlightSyntax = (code: string, language: string) => {
    try {
      if (Prism.languages[language]) {
        return Prism.highlight(code, Prism.languages[language], language);
      }
    } catch (e) {
      // Fallback to plain text if highlighting fails
    }
    return code;
  };

  const getFileIcon = (changeType: string) => {
    if (changeType.includes('added')) {
      return <FilePlus className="w-4 h-4 text-green-600" />;
    } else if (changeType.includes('deleted')) {
      return <FileMinus className="w-4 h-4 text-red-600" />;
    } else {
      return <FileIcon className="w-4 h-4 text-blue-600" />;
    }
  };

  const getChangeTypeBadge = (changeType: string) => {
    let variant: "default" | "secondary" | "destructive" | "outline" = "default";
    let className = "";
    
    if (changeType.includes('added')) {
      variant = "secondary";
      className = "bg-green-100 text-green-800 border-green-200";
    } else if (changeType.includes('deleted')) {
      variant = "destructive";
      className = "bg-red-100 text-red-800 border-red-200";
    } else if (changeType.includes('modified')) {
      variant = "outline";
      className = "bg-blue-100 text-blue-800 border-blue-200";
    }
    
    return (
      <Badge variant={variant} className={className}>
        {changeType}
      </Badge>
    );
  };

  if (!isOpen) return null;

  return (
    <Card className="mt-4 border-l-4 border-l-blue-500">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-lg flex items-center gap-2">
            <FileText className="w-5 h-5" />
            Diff against {diffData?.source_branch || 'source branch'}
          </CardTitle>
          <div className="flex items-center gap-2">
            {isWideScreen && (
              <Button 
                variant={splitView ? "default" : "outline"} 
                size="sm"
                onClick={() => setSplitView(!splitView)}
                title={splitView ? "Switch to unified view" : "Switch to split view"}
                className={splitView ? "bg-blue-100 text-blue-800 border-blue-200 hover:bg-blue-200" : ""}
              >
                {splitView ? (
                  <RectangleHorizontal className="w-4 h-4" />
                ) : (
                  <SplitSquareHorizontal className="w-4 h-4" />
                )}
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={onClose}>
              ✕
            </Button>
          </div>
        </div>
        {diffData && (
          <p className="text-sm text-muted-foreground">
            {diffData.summary} in {diffData.worktree_name} (vs {diffData.fork_commit.slice(0, 8)})
          </p>
        )}
      </CardHeader>
      
      <CardContent>
        {loading && (
          <div className="flex items-center justify-center py-8">
            <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600"></div>
            <span className="ml-2 text-sm text-muted-foreground">Loading diff...</span>
          </div>
        )}
        
        {error && (
          <div className="text-red-600 text-sm bg-red-50 p-3 rounded border">
            {error}
          </div>
        )}
        
        {diffData && diffData.file_diffs.length === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            <FileText className="w-12 h-12 mx-auto mb-3 opacity-30" />
            <p>No changes to show</p>
          </div>
        )}
        
        {diffData && diffData.file_diffs.length > 0 && (
          <div className="space-y-3">
            {diffData.file_diffs.map((file) => {
              const isExpanded = expandedFiles.has(file.file_path);
              return (
                <div key={file.file_path} className="border rounded-lg overflow-hidden">
                  <div 
                    className="flex items-center justify-between p-3 bg-muted/30 hover:bg-muted/50 cursor-pointer transition-colors"
                    onClick={() => toggleFileExpansion(file.file_path)}
                  >
                    <div className="flex items-center gap-2 flex-1 min-w-0">
                      {isExpanded ? (
                        <ChevronDown className="w-4 h-4 text-muted-foreground flex-shrink-0" />
                      ) : (
                        <ChevronRight className="w-4 h-4 text-muted-foreground flex-shrink-0" />
                      )}
                      {getFileIcon(file.change_type)}
                      <span className="font-mono text-sm truncate" title={file.file_path}>
                        {file.file_path}
                      </span>
                    </div>
                    <div className="flex items-center gap-2 flex-shrink-0">
                      {getChangeTypeBadge(file.change_type)}
                    </div>
                  </div>
                  
                  {isExpanded && (
                    <div className="border-t">
                      {file.change_type.includes('added') && file.change_type.includes('untracked') ? (
                        // Show new file content for untracked files
                        <div className="p-3 bg-green-50 dark:bg-green-950/20">
                          <div className="text-xs text-muted-foreground mb-2">New file content:</div>
                          <pre className="text-sm font-mono whitespace-pre-wrap bg-background p-3 rounded border max-h-96 overflow-auto">
                            {file.new_content || 'No content to display'}
                          </pre>
                        </div>
                      ) : file.change_type.includes('deleted') ? (
                        // Show deletion message for deleted files
                        <div className="p-3 bg-red-50 dark:bg-red-950/20">
                          <div className="text-sm text-red-800 dark:text-red-200">
                            File was deleted
                          </div>
                        </div>
                      ) : file.old_content !== undefined && file.new_content !== undefined ? (
                        // Show ReactDiffViewer for files with old/new content
                        <div 
                          className="border-t font-mono" 
                          style={{
                            fontSize: '0.75rem',
                            lineHeight: '1.25'
                          }}
                        >
                          <ReactDiffViewer
                            oldValue={file.old_content}
                            newValue={file.new_content}
                            splitView={splitView}
                            compareMethod={DiffMethod.WORDS}
                            hideLineNumbers={false}
                            showDiffOnly={true}
                            useDarkTheme={document.documentElement.classList.contains('dark')}
                            renderContent={(source: string) => {
                              const language = getLanguageFromFilePath(file.file_path);
                              return (
                                <pre 
                                  className="font-mono text-xs leading-tight"
                                  dangerouslySetInnerHTML={{
                                    __html: highlightSyntax(source, language)
                                  }}
                                />
                              );
                            }}
                            styles={{
                              variables: {
                                light: {
                                  codeFoldGutterBackground: '#f1f3f4',
                                  codeFoldBackground: '#f1f3f4',
                                  addedBackground: '#e6ffed',
                                  addedColor: '#24292e',
                                  removedBackground: '#ffeef0',
                                  removedColor: '#24292e',
                                  wordAddedBackground: '#acf2bd',
                                  wordRemovedBackground: '#fdb8c0',
                                  addedGutterBackground: '#cdffd8',
                                  removedGutterBackground: '#fdbdbc',
                                  gutterBackground: '#f1f3f4',
                                  gutterBackgroundDark: '#f1f3f4',
                                  highlightBackground: '#fffbdd',
                                  highlightGutterBackground: '#fff5b4',
                                  // Try to control line number size specifically
                                  gutterColor: '#6b7280',
                                  lineNumberColor: '#6b7280',
                                },
                                dark: {
                                  codeFoldGutterBackground: '#21262d',
                                  codeFoldBackground: '#21262d',
                                  addedBackground: '#0d1117',
                                  addedColor: '#e6edf3',
                                  removedBackground: '#0d1117',
                                  removedColor: '#e6edf3',
                                  wordAddedBackground: '#033a16',
                                  wordRemovedBackground: '#67060c',
                                  addedGutterBackground: '#033a16',
                                  removedGutterBackground: '#67060c',
                                  gutterBackground: '#21262d',
                                  gutterBackgroundDark: '#21262d',
                                  highlightBackground: '#373e47',
                                  highlightGutterBackground: '#373e47',
                                  // Try to control line number size specifically
                                  gutterColor: '#9ca3af',
                                  lineNumberColor: '#9ca3af',
                                },
                              },
                              // Target line numbers more specifically
                              lineNumber: {
                                fontSize: '0.75rem',
                                lineHeight: '1.25',
                              },
                              gutter: {
                                fontSize: '0.75rem',
                                lineHeight: '1.25',
                                minWidth: '2.5rem',
                              },
                              codeFold: {
                                fontSize: '0.75rem',
                                fontWeight: 'normal',
                                color: document.documentElement.classList.contains('dark') ? '#9ca3af' : '#6b7280',
                                cursor: 'pointer',
                                userSelect: 'none',
                                lineHeight: '1.25',
                                '&:hover': {
                                  color: document.documentElement.classList.contains('dark') ? '#d1d5db' : '#374151',
                                },
                              },
                              codeFoldGutter: {
                                fontSize: '0.75rem',
                                fontWeight: 'normal',
                                color: document.documentElement.classList.contains('dark') ? '#9ca3af' : '#6b7280',
                                lineHeight: '1.25',
                              },
                            }}
                          />
                        </div>
                      ) : file.diff_text ? (
                        // Fallback to raw diff text if old/new content not available  
                        <div className="border-t bg-background">
                          <pre className="font-mono text-xs leading-tight whitespace-pre-wrap p-3 m-0 overflow-auto max-h-96">
                            {file.diff_text}
                          </pre>
                        </div>
                      ) : (
                        // Fallback for files without diff content
                        <div className="p-3 text-sm text-muted-foreground">
                          No diff content available
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}