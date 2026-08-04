import type { Conversation, Message, Skill, Agent } from '@/types'

const BASE = '/api'

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

// Conversations
export const listConversations = () => req<Conversation[]>('GET', '/conversations')
export const createConversation = (title?: string) =>
  req<Conversation>('POST', '/conversations', title ? { title } : {})

// Messages
export const listMessages = (convId: string, cursor?: { after_created_at: string; after_id: string }) => {
  const qs = cursor
    ? `?after_created_at=${encodeURIComponent(cursor.after_created_at)}&after_id=${cursor.after_id}`
    : ''
  return req<Message[]>('GET', `/conversations/${convId}/messages${qs}`)
}
export const sendMessage = (convId: string, content: string) =>
  req<Message>('POST', `/conversations/${convId}/messages`, { content })

// Cancel threads
export const cancelThreads = (convId: string) =>
  req<void>('DELETE', `/conversations/${convId}/threads`)

// Skills
export const listSkills  = ()                        => req<Skill[]>('GET', '/skills')
export const getSkill    = (id: string)              => req<Skill>('GET', `/skills/${id}`)
export const createSkill = (b: { name: string; description?: string; body: string }) =>
  req<Skill>('POST', '/skills', b)
export const updateSkill = (id: string, b: { name: string; description?: string; body: string }) =>
  req<Skill>('PUT', `/skills/${id}`, b)
export const deleteSkill = (id: string) => req<void>('DELETE', `/skills/${id}`)

// Agents
export const listAgents  = ()                          => req<Agent[]>('GET', '/agents')
export const getAgent    = (id: string)                => req<Agent>('GET', `/agents/${id}`)
export const createAgent = (b: { name: string; description?: string; prompt: string }) =>
  req<Agent>('POST', '/agents', b)
export const updateAgent = (id: string, b: { name: string; description?: string; prompt: string }) =>
  req<Agent>('PUT', `/agents/${id}`, b)
export const deleteAgent = (id: string) => req<void>('DELETE', `/agents/${id}`)

// Agent skill loadout
export const listAgentSkills  = (agentId: string)                => req<Skill[]>('GET', `/agents/${agentId}/skills`)
export const addAgentSkill    = (agentId: string, skillId: string) => req<void>('POST', `/agents/${agentId}/skills`, { skill_id: skillId })
export const removeAgentSkill = (agentId: string, skillId: string) => req<void>('DELETE', `/agents/${agentId}/skills/${skillId}`)

// Conversation agents
export const listConvAgents  = (convId: string)                  => req<Agent[]>('GET', `/conversations/${convId}/agents`)
export const addConvAgent    = (convId: string, agentId: string) => req<void>('POST', `/conversations/${convId}/agents`, { agent_id: agentId })
export const removeConvAgent = (convId: string, agentId: string) => req<void>('DELETE', `/conversations/${convId}/agents/${agentId}`)

// Reports
export const runReport = (type: 'daily' | 'weekly') =>
  req<{ conversation_id: string }>('POST', `/reports/${type}/run`)
