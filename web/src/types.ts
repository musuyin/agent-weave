export interface Conversation {
  ID: string
  Title: string
  CreatedAt: string
  UpdatedAt: string
}

export interface ContentBlock {
  type: 'text' | 'tool_use' | 'tool_result'
  // text
  text?: string
  // tool_use
  id?: string
  name?: string
  input?: Record<string, unknown>
  // tool_result
  tool_use_id?: string
  content?: string
  is_error?: boolean
}

export interface Message {
  ID: string
  ConversationID: string
  Role: 'user' | 'assistant'
  AgentID: string | null
  Compacted: boolean
  Content: ContentBlock[]
  CreatedAt: string
}

export interface Skill {
  ID: string
  Name: string
  Description: string
  Body: string
  IsSystem: boolean
  CreatedAt: string
  UpdatedAt: string
}

export interface Agent {
  ID: string
  Name: string
  Description: string
  Prompt: string
  IsSystem: boolean
  CreatedAt: string
  UpdatedAt: string
}

// SSE event payloads
export interface AgentStartPayload { conversation_id: string }
export interface BlockStartPayload { block_id: string; block_type: 'text' | 'tool_use'; index: number }
export interface BlockDeltaPayload { block_id: string; text: string; index: number }
export interface BlockStopPayload  { block_id: string; index: number }
export interface ThreadStatusPayload { thread_id: string; status: 'running' | 'done' | 'error' | 'cancelled'; agent_name: string }

export interface SSEEvent {
  type: string
  data: unknown
}
