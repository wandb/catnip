import { TranscriptMessage, TranscriptSession, ParsedTranscript } from './transcript-types'

export function parseTranscript(session: TranscriptSession): ParsedTranscript {
  const { messages } = session
  
  // Sort messages by timestamp
  const sortedMessages = [...messages].sort((a, b) => 
    new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
  )
  
  // Create a map for quick parent-child lookup
  const messageTree = new Map<string, TranscriptMessage[]>()
  const rootMessages: TranscriptMessage[] = []
  
  // Build the tree structure
  for (const message of sortedMessages) {
    if (message.parentUuid) {
      // This message has a parent
      if (!messageTree.has(message.parentUuid)) {
        messageTree.set(message.parentUuid, [])
      }
      messageTree.get(message.parentUuid)!.push(message)
    } else {
      // This is a root message
      rootMessages.push(message)
    }
  }
  
  return {
    messages: sortedMessages,
    messageTree,
    rootMessages
  }
}

export function getChildMessages(messageId: string, messageTree: Map<string, TranscriptMessage[]>): TranscriptMessage[] {
  return messageTree.get(messageId) || []
}

export function formatTimestamp(timestamp: string): string {
  const date = new Date(timestamp)
  return date.toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  })
}

export function getMessageDepth(message: TranscriptMessage, messages: TranscriptMessage[]): number {
  let depth = 0
  let currentParentUuid = message.parentUuid
  
  while (currentParentUuid) {
    depth++
    const parent = messages.find(m => m.uuid === currentParentUuid)
    currentParentUuid = parent?.parentUuid
  }
  
  return depth
}

export function isToolCall(content: any): boolean {
  return content.type === 'tool_use'
}

export function isToolResult(content: any): boolean {
  return content.type === 'tool_result'
}

export function isTextContent(content: any): boolean {
  return content.type === 'text'
}