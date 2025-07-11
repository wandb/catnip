import { FC } from "react";
import { cn } from "../lib/utils";
import { InlineDiff } from "./InlineDiff";
import { TodoDisplay } from "./TodoDisplay";
import { Terminal, FileText, Code2, Search, Globe, BookOpen, FileSearch2, LogOut } from "lucide-react";

interface ToolCallRendererProps {
  toolName: string;
  toolInput: Record<string, any>;
  result?: {
    content: string;
    isError: boolean;
  };
  showMinimal?: boolean;
}

// Tool-specific render functions
const toolRenderers: Record<string, FC<ToolCallRendererProps>> = {
  Bash: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div>
        {toolInput.command && (
          <div className="bg-zinc-900 dark:bg-zinc-950 rounded-lg p-3 font-mono text-sm">
            <div className="flex items-center gap-2 text-zinc-400 mb-2">
              <Terminal className="h-3 w-3" />
              <span className="text-xs">Terminal</span>
            </div>
            <div className="text-zinc-100">
              <span className="text-emerald-400">$</span> {toolInput.command}
            </div>
            {result && result.content && (
              <>
                <div className="h-px bg-zinc-700 my-2" />
                <div className={cn(
                  "text-xs whitespace-pre-wrap break-words overflow-auto max-h-96",
                  result.isError ? "text-red-400" : "text-zinc-300"
                )}>
                  {result.content}
                </div>
              </>
            )}
          </div>
        )}
      </div>
    );
  },

  Read: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) {
      return result && result.content ? (
        <div className="text-xs text-muted-foreground -my-0.5">
          {result.content.split('\n').length} lines read
        </div>
      ) : null;
    }
    
    return (
      <div className="space-y-2">
        {result && result.content && (
          <div className="bg-muted/50 rounded-lg p-3 font-mono text-xs whitespace-pre-wrap break-words overflow-auto max-h-96 border">
            {result.content}
          </div>
        )}
      </div>
    );
  },

  Edit: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        <InlineDiff
          oldContent={toolInput.old_string || ""}
          newContent={toolInput.new_string || ""}
          fileName={toolInput.file_path || "Unknown file"}
        />
        {result && result.isError && result.content && (
          <div className="bg-red-50 dark:bg-red-950/50 border border-red-200 dark:border-red-800 rounded-lg p-3 text-sm">
            {result.content}
          </div>
        )}
      </div>
    );
  },

  MultiEdit: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) {
      return (
        <div className="text-xs text-muted-foreground -my-0.5">
          {toolInput.edits?.length || 0} edit{toolInput.edits?.length === 1 ? '' : 's'}
        </div>
      );
    }
    
    return (
      <div className="space-y-2">
        {toolInput.edits && Array.isArray(toolInput.edits) && (
          <div className="space-y-2">
            {toolInput.edits.map((edit: any, index: number) => (
              <div key={index} className="border rounded-lg p-2">
                <div className="text-xs text-muted-foreground mb-1">Edit {index + 1}</div>
                <InlineDiff
                  oldContent={edit.old_string || ""}
                  newContent={edit.new_string || ""}
                  fileName=""
                />
              </div>
            ))}
          </div>
        )}
        {result && result.content && (
          <div className={cn(
            "text-sm p-3 rounded-lg mt-2",
            result.isError 
              ? "bg-red-50 dark:bg-red-950/50 border border-red-200 dark:border-red-800" 
              : "bg-green-50 dark:bg-green-950/50 border border-green-200 dark:border-green-800"
          )}>
            {result.content}
          </div>
        )}
      </div>
    );
  },

  Write: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        {toolInput.content && (
          <div className="bg-muted/50 rounded-lg p-3 font-mono text-xs whitespace-pre-wrap break-words overflow-auto max-h-48 border">
            {toolInput.content}
          </div>
        )}
        {result && result.content && (
          <div className={cn(
            "text-sm p-2 rounded mt-1",
            result.isError 
              ? "bg-red-50 dark:bg-red-950/50 text-red-700 dark:text-red-400" 
              : "bg-green-50 dark:bg-green-950/50 text-green-700 dark:text-green-400"
          )}>
            {result.content}
          </div>
        )}
      </div>
    );
  },

  TodoWrite: ({ toolInput, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div>
        {toolInput.todos && <TodoDisplay todos={toolInput.todos} />}
      </div>
    );
  },

  TodoRead: ({ result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div>
        {result && result.content && (
          <div className="bg-muted/50 rounded-lg p-3 text-sm">
            {result.content}
          </div>
        )}
      </div>
    );
  },

  Task: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        {toolInput.prompt && (
          <div className="text-sm text-muted-foreground bg-muted/50 rounded-lg p-3">
            {toolInput.prompt}
          </div>
        )}
        {result && result.content && (
          <div className="bg-muted/50 rounded-lg p-3 text-sm whitespace-pre-wrap break-words overflow-auto max-h-96">
            {result.content}
          </div>
        )}
      </div>
    );
  },

  WebFetch: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        {toolInput.url && (
          <div className="flex items-center gap-2 text-sm">
            <Globe className="h-4 w-4 text-muted-foreground" />
            <a href={toolInput.url} target="_blank" rel="noopener noreferrer" 
               className="text-blue-600 dark:text-blue-400 hover:underline truncate max-w-md">
              {toolInput.url}
            </a>
          </div>
        )}
        {toolInput.prompt && (
          <div className="text-sm text-muted-foreground bg-muted/50 rounded-lg p-2">
            {toolInput.prompt}
          </div>
        )}
        {result && result.content && (
          <div className="bg-muted/50 rounded-lg p-3 text-sm whitespace-pre-wrap break-words overflow-auto max-h-48">
            {result.content}
          </div>
        )}
      </div>
    );
  },

  WebSearch: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        {toolInput.query && (
          <div className="flex items-center gap-2 text-sm">
            <Search className="h-4 w-4 text-muted-foreground" />
            <span className="font-medium">"{toolInput.query}"</span>
          </div>
        )}
        {result && result.content && (
          <div className="bg-muted/50 rounded-lg p-3 text-sm whitespace-pre-wrap break-words overflow-auto max-h-48">
            {result.content}
          </div>
        )}
      </div>
    );
  },

  Glob: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        {toolInput.pattern && (
          <div className="flex items-center gap-2 text-sm">
            <FileSearch2 className="h-4 w-4 text-muted-foreground" />
            <code className="bg-muted px-2 py-0.5 rounded text-xs">{toolInput.pattern}</code>
            {toolInput.path && <span className="text-xs text-muted-foreground">in {toolInput.path}</span>}
          </div>
        )}
        {result && result.content && (
          <div className="bg-muted/50 rounded-lg p-3 font-mono text-xs whitespace-pre-wrap break-words overflow-auto max-h-48">
            {result.content}
          </div>
        )}
      </div>
    );
  },

  Grep: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        {toolInput.pattern && (
          <div className="flex items-center gap-2 text-sm">
            <Search className="h-4 w-4 text-muted-foreground" />
            <code className="bg-muted px-2 py-0.5 rounded text-xs">{toolInput.pattern}</code>
            {toolInput.include && <span className="text-xs text-muted-foreground">in {toolInput.include}</span>}
            {toolInput.path && <span className="text-xs text-muted-foreground">• {toolInput.path}</span>}
          </div>
        )}
        {result && result.content && (
          <div className="bg-muted/50 rounded-lg p-3 font-mono text-xs whitespace-pre-wrap break-words overflow-auto max-h-48">
            {result.content}
          </div>
        )}
      </div>
    );
  },

  LS: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div>
        {toolInput.path && (
          <div className="bg-zinc-900 dark:bg-zinc-950 rounded-lg p-3 font-mono text-sm">
            <div className="flex items-center gap-2 text-zinc-400 mb-2">
              <Terminal className="h-3 w-3" />
              <span className="text-xs">Terminal</span>
            </div>
            <div className="text-zinc-100">
              <span className="text-emerald-400">$</span> ls {toolInput.path}
            </div>
            {result && result.content && (
              <>
                <div className="h-px bg-zinc-700 my-2" />
                <div className={cn(
                  "text-xs whitespace-pre-wrap break-words overflow-auto max-h-96",
                  result.isError ? "text-red-400" : "text-zinc-300"
                )}>
                  {result.content}
                </div>
              </>
            )}
          </div>
        )}
      </div>
    );
  },

  NotebookRead: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        {toolInput.notebook_path && (
          <div className="flex items-center gap-2 text-sm">
            <BookOpen className="h-4 w-4 text-muted-foreground" />
            <code className="bg-muted px-2 py-0.5 rounded text-xs">{toolInput.notebook_path}</code>
            {toolInput.cell_id && <span className="text-xs text-muted-foreground">• Cell {toolInput.cell_id}</span>}
          </div>
        )}
        {result && result.content && (
          <div className="bg-muted/50 rounded-lg p-3 text-sm whitespace-pre-wrap break-words overflow-auto max-h-96">
            {result.content}
          </div>
        )}
      </div>
    );
  },

  NotebookEdit: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        {toolInput.notebook_path && (
          <div className="flex items-center gap-2 text-sm">
            <BookOpen className="h-4 w-4 text-muted-foreground" />
            <code className="bg-muted px-2 py-0.5 rounded text-xs">{toolInput.notebook_path}</code>
            {toolInput.cell_id && <span className="text-xs text-muted-foreground">• Cell {toolInput.cell_id}</span>}
          </div>
        )}
        {toolInput.new_source && (
          <div className="bg-muted/50 rounded-lg p-3 font-mono text-xs whitespace-pre-wrap break-words overflow-auto max-h-48 border">
            {toolInput.new_source}
          </div>
        )}
        {result && result.content && (
          <div className={cn(
            "text-sm p-2 rounded mt-1",
            result.isError 
              ? "bg-red-50 dark:bg-red-950/50 text-red-700 dark:text-red-400" 
              : "bg-green-50 dark:bg-green-950/50 text-green-700 dark:text-green-400"
          )}>
            {result.content}
          </div>
        )}
      </div>
    );
  },

  exit_plan_mode: ({ toolInput, result, showMinimal }) => {
    if (showMinimal) return null;
    
    return (
      <div className="space-y-2">
        {toolInput.plan && (
          <div className="space-y-2">
            <div className="flex items-center gap-2 text-sm">
              <LogOut className="h-4 w-4 text-muted-foreground" />
              <span className="font-medium">Exit Plan Mode</span>
            </div>
            <div className="bg-muted/50 rounded-lg p-3 text-sm whitespace-pre-wrap">
              {toolInput.plan}
            </div>
          </div>
        )}
        {result && result.content && (
          <div className="text-sm text-muted-foreground mt-2">
            {result.content}
          </div>
        )}
      </div>
    );
  },
};

// Default renderer for unknown tools
const DefaultRenderer: FC<ToolCallRendererProps> = ({ toolInput, result, showMinimal }) => {
  if (showMinimal) {
    return (
      <div className="text-xs text-muted-foreground -my-0.5">
        {Object.keys(toolInput).length} parameter{Object.keys(toolInput).length === 1 ? '' : 's'}
      </div>
    );
  }
  
  return (
    <div className="space-y-2">
      <div className="text-xs text-muted-foreground mb-2">Input parameters:</div>
      <div className="bg-muted/50 p-3 rounded border">
        <pre className="text-xs whitespace-pre-wrap break-words overflow-auto">
          {JSON.stringify(toolInput, null, 2)}
        </pre>
      </div>
      {result && result.content && (
        <div className={cn(
          "p-3 rounded border font-mono text-xs whitespace-pre-wrap break-words overflow-auto max-h-96 mt-2",
          result.isError ? "bg-red-50 dark:bg-red-950/50 border-red-200 dark:border-red-800" : "bg-muted/50"
        )}>
          {result.content}
        </div>
      )}
    </div>
  );
};

export function ToolCallRenderer(props: ToolCallRendererProps) {
  const Renderer = toolRenderers[props.toolName] || DefaultRenderer;
  return <Renderer {...props} />;
}