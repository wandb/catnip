import { useState } from 'react'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { Card, CardContent, CardHeader, CardTitle } from './ui/card'
import { ChevronDown, ChevronRight, CheckCircle, XCircle } from 'lucide-react'
import { cn } from '../lib/utils'

interface ToolResultProps {
  toolUseId: string
  content: string
  isError: boolean
}

export function ToolResult({ toolUseId, content, isError }: ToolResultProps) {
  const [isExpanded, setIsExpanded] = useState(false)
  
  // Try to detect if content is likely truncated
  const isTruncated = content.length > 1000 || content.includes('...')
  
  // Handle empty content
  const isEmpty = !content || content.trim().length === 0
  const displayContent = isEmpty ? (isError ? 'Command failed with no output' : 'Command completed successfully') : content
  
  return (
    <Card className={cn(
      "border-l-4",
      isError 
        ? "border-red-500 bg-red-50/50 dark:bg-red-950/20" 
        : "border-green-500 bg-green-50/50 dark:bg-green-950/20"
    )}>
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center gap-2 text-sm">
          {isError ? (
            <XCircle className="h-4 w-4 text-red-600 dark:text-red-400" />
          ) : (
            <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
          )}
          <span>Tool Result</span>
          {isError && (
            <Badge variant="destructive" className="text-xs">
              Error
            </Badge>
          )}
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto h-6 w-6 p-0"
            onClick={() => setIsExpanded(!isExpanded)}
          >
            {isExpanded ? (
              <ChevronDown className="h-3 w-3" />
            ) : (
              <ChevronRight className="h-3 w-3" />
            )}
          </Button>
        </CardTitle>
      </CardHeader>
      
      {isExpanded && (
        <CardContent className="pt-0">
          <div className={cn(
            "p-3 rounded border font-mono text-xs whitespace-pre-wrap break-words overflow-auto max-h-96",
            isError ? "bg-red-50 dark:bg-red-950/50" : "bg-muted/50",
            isEmpty && "italic text-muted-foreground"
          )}>
            {displayContent}
          </div>
          {isTruncated && (
            <div className="mt-2 text-xs text-muted-foreground">
              Content may be truncated
            </div>
          )}
        </CardContent>
      )}
      
      {!isExpanded && (
        <CardContent className="pt-0">
          <div className={cn(
            "text-xs text-muted-foreground",
            isEmpty && "italic"
          )}>
            {isEmpty ? displayContent : (
              <>
                {content.slice(0, 100)}
                {content.length > 100 && '...'}
              </>
            )}
          </div>
        </CardContent>
      )}
    </Card>
  )
}