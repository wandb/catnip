export interface TranscriptMessage {
  cwd: string
  isMeta: boolean
  isSidechain: boolean
  sessionId: string
  timestamp: string
  type: 'assistant' | 'user'
  userType: string
  uuid: string
  parentUuid?: string
  version: string
  message?: ClaudeMessage
  content?: UserContent[] | string
}

export interface ClaudeMessage {
  content: ContentItem[] | string
  id?: string
  model?: string
  role: 'assistant' | 'user'
  stop_reason?: string | null
  stop_sequence?: string | null
  type?: 'message'
  usage?: TokenUsage
}

export interface TokenUsage {
  cache_creation_input_tokens?: number
  cache_read_input_tokens?: number
  input_tokens: number
  output_tokens: number
  service_tier: string
}

export interface ContentItem {
  type: 'text' | 'tool_use' | 'tool_result'
  text?: string
  id?: string
  name?: string
  input?: Record<string, any>
  // For tool_result items
  content?: string
  tool_use_id?: string
  is_error?: boolean
}

export interface UserContent {
  type: 'tool_result'
  content: string
  tool_use_id: string
  is_error?: boolean
}

export interface UserPrompt {
  display: string
  pastedContents: Record<string, any>
}

export interface TranscriptSession {
  messages: TranscriptMessage[]
  userPrompts: UserPrompt[]
}

export interface ParsedTranscript {
  messages: TranscriptMessage[]
  messageTree: Map<string, TranscriptMessage[]>
  rootMessages: TranscriptMessage[]
}