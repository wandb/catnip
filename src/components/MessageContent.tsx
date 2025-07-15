import type { TranscriptMessage } from '../lib/transcript-types'
import { isToolCall, isToolResult, isTextContent } from '../lib/transcript-utils'
import { ToolCall } from './ToolCall'
import { ToolResult } from './ToolResult'
import { TextContent } from './TextContent'

interface MessageContentProps {
  message: TranscriptMessage
  allMessages?: TranscriptMessage[]
  showOnlyText?: boolean
  showOnlyTools?: boolean
}

// Helper function to find tool results for a given tool call
function findToolResult(toolId: string, allMessages: TranscriptMessage[]): { content: string; isError: boolean } | undefined {
  for (const msg of allMessages) {
    if (msg.type === 'user' && msg.message?.content && Array.isArray(msg.message.content)) {
      for (const item of msg.message.content) {
        if (item.type === 'tool_result' && item.tool_use_id === toolId) {
          return {
            content: item.content || '',
            isError: item.is_error || false
          }
        }
      }
    }
  }
  return undefined
}

export function MessageContent({ message, allMessages = [], showOnlyText = false, showOnlyTools = false }: MessageContentProps) {
  // Handle assistant messages with structured content
  if (message.type === 'assistant' && message.message?.content && Array.isArray(message.message.content)) {
    return (
      <div className="space-y-2">
        {message.message.content.map((item, index) => {
          if (isTextContent(item)) {
            if (showOnlyTools) return null
            return <TextContent key={index} content={item.text || ''} />
          }
          
          if (isToolCall(item)) {
            if (showOnlyText) return null
            const result = findToolResult(item.id || '', allMessages)
            return (
              <ToolCall
                key={item.id || index}
                toolId={item.id || ''}
                toolName={item.name || ''}
                toolInput={item.input || {}}
                result={result}
              />
            )
          }
          
          return null
        })}
      </div>
    )
  }

  // Handle user messages with message.content (tool results) - only if not filtered out
  if (message.type === 'user' && message.message?.content && Array.isArray(message.message.content)) {
    return (
      <div className="space-y-2">
        {message.message.content.map((item, index) => {
          // User messages have tool_result items with different structure
          if (item.type === 'tool_result') {
            if (showOnlyText) return null
            return (
              <ToolResult
                key={item.tool_use_id || index}
                toolUseId={item.tool_use_id || ''}
                content={item.content || ''}
                isError={item.is_error || false}
              />
            )
          }
          
          // Handle text content in user messages
          if (isTextContent(item)) {
            if (showOnlyTools) return null
            return <TextContent key={index} content={item.text || ''} />
          }
          
          return null
        })}
      </div>
    )
  }

  // Handle user messages (typically tool results) - fallback to content property
  if (message.content && Array.isArray(message.content)) {
    return (
      <div className="space-y-2">
        {message.content.map((item, index) => {
          if (isToolResult(item)) {
            return (
              <ToolResult
                key={item.tool_use_id || index}
                toolUseId={item.tool_use_id}
                content={item.content}
                isError={item.is_error || false}
              />
            )
          }
          
          return null
        })}
      </div>
    )
  }

  // Handle simple string content (fallback)
  if (typeof message.message?.content === 'string') {
    return <TextContent content={message.message.content} />
  }

  if (typeof message.content === 'string') {
    return <TextContent content={message.content} />
  }

  return (
    <div className="text-muted-foreground italic">
      No content available
    </div>
  )
}