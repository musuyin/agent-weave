# Frontend Integration Guide

This document describes every backend API endpoint, request/response shape, and SSE protocol that the frontend must implement against. All routes are prefixed with `/api`. The backend runs on the port configured in `config.yaml` (default `8080`).

---

## Data Types

### Conversation

```ts
interface Conversation {
  ID:        string    // UUID
  Title:     string
  CreatedAt: string    // ISO 8601 with milliseconds
  UpdatedAt: string
}
```

### Message

```ts
interface ContentBlock {
  type: "text" | "tool_use" | "tool_result"
  // text
  text?: string
  // tool_use
  id?: string
  name?: string
  input?: object
  // tool_result
  tool_use_id?: string
  content?: string
  is_error?: boolean
}

interface Message {
  ID:             string
  ConversationID: string
  Role:           "user" | "assistant"
  AgentID:        string | null   // null = orchestrator
  Compacted:      boolean
  Content:        ContentBlock[]
  CreatedAt:      string          // ISO 8601 with nanoseconds — use as pagination cursor
}
```

### Skill

```ts
interface Skill {
  ID:          string
  Name:        string
  Description: string
  Body:        string
  IsSystem:    boolean
  CreatedAt:   string
  UpdatedAt:   string
}
```

### Agent

```ts
interface Agent {
  ID:          string
  Name:        string
  Description: string
  Prompt:      string
  IsSystem:    boolean
  CreatedAt:   string
  UpdatedAt:   string
}
```

### Error

All error responses use:

```ts
{ error: string }
```

---

## Conversations

### List conversations

```
GET /api/conversations
→ 200  Conversation[]     // ordered newest-first, max 50
```

### Create conversation

```
POST /api/conversations
Body (optional): { "title": string }
→ 201  Conversation

// Empty body is allowed — title defaults to "New conversation"
```

---

## Messages

### List messages (keyset pagination)

```
GET /api/conversations/:id/messages
  ?after_created_at=<RFC3339Nano>   // both params required together
  ?after_id=<uuid>
→ 200  Message[]    // up to 50, ordered oldest-first
→ 404  if conversation not found
→ 400  if cursor params are malformed or after_id doesn't belong to this conversation
```

**Pagination pattern:** On first load send no query params — you get the 50 most recent messages. To page backwards, take the `CreatedAt` and `ID` of the oldest message in the current page and pass them as `after_created_at` / `after_id`.

`CreatedAt` must be formatted as RFC3339Nano (e.g. `2026-08-03T09:00:00.123456789Z`). Use `new Date(msg.CreatedAt).toISOString()` — JavaScript's ISO string matches what the backend expects.

### Send message

```
POST /api/conversations/:id/messages
Body: { "content": string }    // required, non-empty
→ 202  Message                 // the saved user message
→ 400  if content missing
→ 404  if conversation not found
```

The agent loop starts **asynchronously** after this returns. Open the SSE stream (or have it already open) to receive the agent's response. The 202 body is only the user's own message.

---

## SSE Stream

```
GET /api/conversations/:id/stream
→ text/event-stream
→ 404  if conversation not found
```

**Open this connection before or immediately after calling POST `/messages`.** The agent runs in the background; if the stream is not open, events queue in the Hub (cap 256). If the queue fills, older events (token deltas) are dropped to make room — terminal signals are never dropped.

### Wire format

Each event is a standard SSE frame:

```
event: <EventType>
data: <JSON-encoded SSEEvent>

```

The `data` field is always a JSON object of the shape `{ "type": string, "data": ... }`. Parse `data` as JSON, then dispatch on `type`.

### Event types

#### `agent_start`

```ts
{ type: "agent_start", data: { conversation_id: string } }
```

Agent loop has started. Create a "thinking" indicator.

---

#### `block_start`

```ts
{ type: "block_start", data: { block_id: string, block_type: "text" | "tool_use", index: number } }
```

A new content block begins. Use `block_id` as the key for this block in your UI state. `index` is the Anthropic SDK's block index within the current LLM response (not globally unique across rounds).

---

#### `block_delta`

```ts
{ type: "block_delta", data: { block_id: string, text: string, index: number } }
```

Only emitted for `text` blocks. Append `text` to the block identified by `block_id`.

---

#### `block_stop`

```ts
{ type: "block_stop", data: { block_id: string, index: number } }
```

The block is complete. Finalize its rendering.

---

#### `message_appended`

```ts
{ type: "message_appended", data: any }
```

A new independent message was appended to the conversation (e.g. an approval bubble, a diff view). Fetch the latest messages via the list endpoint to get the full message content. Do **not** use the `data` field for rendering — treat this as a cache invalidation signal.

---

#### `thread_status`

```ts
{ type: "thread_status", data: { thread_id: string, status: "running" | "done" | "error" | "cancelled", agent_name: string } }
```

A subagent thread changed state. Use to render a "subagent activity" indicator.

---

#### `round_done`

```ts
{ type: "round_done", data: null }
```

The orchestrator has finished one full round (all tool calls resolved, subagents done). Clear any streaming/thinking state. **Do not close the SSE connection yet** — `queue_drained` follows.

---

#### `queue_drained`

```ts
{ type: "queue_drained", data: null }
```

The Hub is empty and will be torn down. **Close the SSE connection here.** After this point the backend deletes the Hub for this conversation.

---

### SSE lifecycle

```
frontend                        backend
  |                               |
  |-- GET /stream --------------> | (Hub created or reused)
  |                               |
  |-- POST /messages -----------> | (agent goroutine launched)
  |                               |
  |<-- agent_start --------------|
  |<-- block_start  -------------|
  |<-- block_delta  ------------- | (many)
  |<-- block_stop   -------------|
  |<-- round_done   -------------|
  |<-- queue_drained ------------|
  |                               |
  | (close connection)            | (Hub deleted)
```

If the agent runs multiple rounds (tool calls), `block_start/delta/stop` repeat for each round; `round_done` / `queue_drained` are emitted only once at the very end.

### EventSource usage (browser)

```ts
const es = new EventSource(`/api/conversations/${convId}/stream`)

es.addEventListener('agent_start',    e => { /* ... */ })
es.addEventListener('block_start',    e => { const d = JSON.parse(e.data); /* ... */ })
es.addEventListener('block_delta',    e => { const d = JSON.parse(e.data); /* ... */ })
es.addEventListener('block_stop',     e => { const d = JSON.parse(e.data); /* ... */ })
es.addEventListener('message_appended', e => { /* refetch messages */ })
es.addEventListener('thread_status',  e => { const d = JSON.parse(e.data); /* ... */ })
es.addEventListener('round_done',     e => { /* clear thinking indicator */ })
es.addEventListener('queue_drained',  e => { es.close() })

es.onerror = () => { /* reconnect logic */ }
```

Note: `e.data` in the `message` event handler is the raw string from the SSE frame. Because the backend serialises the full `SSEEvent` object as the data field, `JSON.parse(e.data)` gives `{ type, data }` — use the inner `.data` property for the payload.

---

## Skills

```
GET  /api/skills             → 200  Skill[]
GET  /api/skills/:id         → 200  Skill   | 404
POST /api/skills             → 201  Skill   | 400
  Body: { name: string, description?: string, body: string }
PUT  /api/skills/:id         → 200  Skill   | 400 | 404
  Body: { name: string, description?: string, body: string }
DELETE /api/skills/:id       → 204          | 400 (system skill) | 404
```

System skills (`IsSystem: true`) cannot be updated or deleted — the server returns 400 with `"error": "system record is read-only"`. Hide or disable edit/delete controls for these in the UI.

---

## Agents

```
GET  /api/agents             → 200  Agent[]
GET  /api/agents/:id         → 200  Agent  | 404
POST /api/agents             → 201  Agent  | 400
  Body: { name: string, description?: string, prompt: string }
PUT  /api/agents/:id         → 200  Agent  | 400 | 404
  Body: { name: string, description?: string, prompt: string }
DELETE /api/agents/:id       → 204         | 400 (system agent) | 404
```

System agents are read-only — same 400 guard as skills.

### Agent skill loadout

```
GET    /api/agents/:id/skills              → 200  Skill[]
POST   /api/agents/:id/skills              → 200
  Body: { skill_id: string }
DELETE /api/agents/:id/skills/:skillId     → 204 | 404
```

### Conversation agent membership

Subagents available in a conversation are listed here. The orchestrator's system prompt is dynamically built from this list — adding an agent makes it available for `dispatch_to_agent`.

```
GET    /api/conversations/:id/agents            → 200  Agent[]
POST   /api/conversations/:id/agents            → 200
  Body: { agent_id: string }
DELETE /api/conversations/:id/agents/:agentId   → 204 | 404
```

---

## Reports

```
POST /api/reports/:type/run
  type: "daily" | "weekly"
→ 202  { conversation_id: string }
→ 400  if type is unknown
```

Triggers a report generation run. The response contains the `conversation_id` of the (pre-created) report conversation. Navigate to that conversation and open its SSE stream to receive the output. The report conversations have stable IDs across server restarts — their titles are `"Daily Report"` and `"Weekly Report"`.

---

## Thread Cancellation

```
DELETE /api/conversations/:id/threads
→ 204   // all non-terminal threads cancelled
→ 400   // missing conversation id
→ 500   // DB error
```

Call this when the user wants to stop a running agent session. After calling this, the agent loop will detect the cancellation on its next iteration and terminate.

---

## Health Check

```
GET /health
→ 200  { status: "ok" }
```

---

## Error handling summary

| HTTP status | Meaning |
|---|---|
| 400 | Bad request body, invalid cursor, or system-record mutation |
| 404 | Resource not found |
| 202 | Agent started asynchronously — watch SSE for output |
| 204 | Success with no body |
| 500 | Unexpected server error — show generic error to user |

---

## Implementation notes

- **Do not parse `Content` blocks beyond `type: "text"` for display.** `tool_use` and `tool_result` blocks are the agent's internal plumbing. Render them collapsed or as a "used tool X" indicator.
- **Compacted messages** (`Compacted: true`) are never returned by the list endpoint — the filter is applied server-side. You do not need to handle them.
- **AgentID on messages**: `null` means the orchestrator produced the message. A non-null UUID identifies which subagent produced it — use this to label subagent replies differently in the UI.
- **SSE reconnect**: `EventSource` reconnects automatically on network error. The Hub persists in the backend as long as the conversation is active, so reconnecting mid-stream will pick up from the next event in the buffer.
- **Streaming bubble approach**: On `block_start` with `block_type: "text"`, create a new empty bubble for the current `block_id`. On each `block_delta`, append text to that bubble. On `block_stop`, finalize. On `round_done`, clear the streaming state. Multiple text blocks can appear in one round (e.g. reasoning + response).
