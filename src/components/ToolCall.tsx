import { useState } from 'react'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { ChevronDown, ChevronRight, Code2, Terminal, FileText, Edit3, CheckSquare, Search, Globe, FileSearch2, BookOpen, LogOut } from 'lucide-react'
import { cn } from '../lib/utils'
import { ToolCallRenderer } from './ToolCallRenderer'

interface ToolCallProps {
  toolId: string
  toolName: string
  toolInput: Record<string, any>
  result?: {
    content: string
    isError: boolean
  }
  defaultExpanded?: boolean
  fullWidth?: boolean
}

type ExpansionState = 'minimal' | 'full'

const TOOL_ICONS: Record<string, React.ComponentType<{ className?: string }>> = {
  Bash: Terminal,
  Read: FileText,
  Edit: Edit3,
  Write: FileText,
  TodoWrite: CheckSquare,
  TodoRead: CheckSquare,
  Task: Search,
  WebFetch: Globe,
  WebSearch: Search,
  Glob: FileSearch2,
  Grep: Search,
  LS: FileText,
  NotebookRead: BookOpen,
  NotebookEdit: BookOpen,
  MultiEdit: Edit3,
  exit_plan_mode: LogOut,
  default: Code2
}

// Helper functions to extract display info
function getFilePath(toolInput: Record<string, any>): string | null {
  return toolInput.file_path || toolInput.path || toolInput.notebook_path || null
}

function getCommand(toolInput: Record<string, any>): string | null {
  return toolInput.command || null
}

function getDescription(toolInput: Record<string, any>, toolName: string): string | null {
  // For specific tools, generate descriptions based on input
  if (toolName === 'Write' && toolInput.content) {
    return `${toolInput.content.split('\n').length} lines`
  }
  if (toolName === 'TodoWrite' && toolInput.todos) {
    return `${toolInput.todos.length} todo item${toolInput.todos.length === 1 ? '' : 's'}`
  }
  return toolInput.description || null
}

// Component to conditionally render tool content
function ToolCallContent({ toolName, toolInput, result, expansionState }: {
  toolName: string;
  toolInput: Record<string, any>;
  result?: { content: string; isError: boolean };
  expansionState: ExpansionState;
}) {
  // Tools that return null in minimal view
  const noMinimalContent = ['Bash', 'LS', 'Edit', 'TodoRead', 'WebFetch', 'WebSearch', 'Glob', 'Grep', 'NotebookRead', 'NotebookEdit', 'exit_plan_mode'];
  
  // Don't render CardContent if in minimal view and tool has no minimal content
  if (expansionState === 'minimal' && noMinimalContent.includes(toolName)) {
    return null;
  }
  
  return (
    <div className={cn(expansionState === 'minimal' ? "px-3 pb-1" : "p-3 pt-0")}>
      <ToolCallRenderer
        toolName={toolName}
        toolInput={toolInput}
        result={expansionState === 'full' ? result : undefined}
        showMinimal={expansionState === 'minimal'}
      />
    </div>
  );
}

export function ToolCall({ toolName, toolInput, result, defaultExpanded = false, fullWidth = false }: ToolCallProps) {
  const [expansionState, setExpansionState] = useState<ExpansionState>(
    defaultExpanded ? 'minimal' : 'minimal'
  )
  const Icon = TOOL_ICONS[toolName] || TOOL_ICONS.default
  
  const filePath = getFilePath(toolInput)
  const command = getCommand(toolInput)
  const description = getDescription(toolInput, toolName)

  const handleToggle = () => {
    setExpansionState(expansionState === 'minimal' ? 'full' : 'minimal')
  }

  const getChevronIcon = () => {
    return expansionState === 'minimal' 
      ? <ChevronRight className="h-3 w-3" />
      : <ChevronDown className="h-3 w-3" />
  }

  return (
    <div className={cn(
      "rounded-xl border shadow-sm border-l-4 transition-all",
      result ? (
        result.isError 
          ? "border-red-500 bg-red-50/50 dark:bg-red-950/20" 
          : "border-green-500 bg-green-50/50 dark:bg-green-950/20"
      ) : "border-orange-200 bg-orange-50/50 dark:border-orange-800 dark:bg-orange-950/20",
      fullWidth && "w-full"
    )}>
      <div className={cn(
        "flex items-center gap-2",
        expansionState === 'minimal' ? "py-1.5 px-3" : "p-3"
      )}>
        <Icon className={cn(
          "h-4 w-4",
          result ? (
            result.isError 
              ? "text-red-600 dark:text-red-400" 
              : "text-green-600 dark:text-green-400"
          ) : "text-orange-600 dark:text-orange-400"
        )} />
        <span className="font-mono font-semibold text-sm">{toolName}</span>
        
        {/* Display file path for file operations */}
        {filePath && (
          <span className="text-xs text-muted-foreground font-mono bg-muted px-2 py-1 rounded">
            {filePath}
          </span>
        )}
        
        {/* Display command for Bash calls */}
        {command && expansionState === 'minimal' && (
          <pre className="text-xs text-muted-foreground font-mono bg-muted px-2 py-1 rounded max-w-md truncate">
            {command}
          </pre>
        )}
        
        {/* Display description inline */}
        {description && expansionState === 'minimal' && (
          <span className="text-xs text-muted-foreground">
            {description}
          </span>
        )}
        
        {/* Result status indicator - only show for errors */}
        {result && result.isError && (
          <Badge 
            variant="destructive"
            className="text-xs"
          >
            Failed
          </Badge>
        )}
        
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto h-6 w-6 p-0"
          onClick={handleToggle}
        >
          {getChevronIcon()}
        </Button>
      </div>
      
      <ToolCallContent
        toolName={toolName}
        toolInput={toolInput}
        result={result}
        expansionState={expansionState}
      />
    </div>
  )
}