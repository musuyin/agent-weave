# Analysis: SSE — How It Is Built, How It Works, and How Multiple Events Are Sent

---

## 1. SSE vs WebSocket

| | SSE | WebSocket |
|---|---|---|
| Direction | Server → client only (unidirectional) | Full-duplex (both directions) |
| Protocol | Plain HTTP/1.1 (`text/event-stream`) | Upgraded TCP (`ws://`) |
| Reconnection | Browser reconnects automatically | Manual reconnect logic required |
| Framing | Text frames (`data:`, `event:`, `id:`) | Binary or text frames, no built-in schema |
| Proxy/firewall friendliness | High — it's just HTTP | Lower — requires upgrade handshake |
| When to use | Push-only streams (AI token stream, notifications) | Chat, collaborative editing, bidirectional real-time |

For an AI agent that pushes token deltas to the browser, SSE is the right fit: the browser never needs to send data back on the same connection.

---

## 2. SSE wire format

An SSE response is a long-lived HTTP response with `Content-Type: text/event-stream`.
Each event is a block of text lines followed by a blank line:

```
event: block_delta\n
data: {"type":"block_delta","data":{"block_id":"b1","text":"Hello","index":0}}\n
\n
event: round_done\n
data: {"type":"round_done"}\n
\n
```

- `event:` sets the event name (maps to `addEventListener('block_delta', ...)` on the frontend).
- `data:` is the payload. Multiple `data:` lines are concatenated.
- The blank line signals end-of-event.

---

## 3. How this codebase builds SSE

### 3a. Event types and payloads (`agent/sse.go`)

```go
type EventType string

const (
    EventAgentStart      EventType = "agent_start"
    EventBlockStart      EventType = "block_start"
    EventBlockDelta      EventType = "block_delta"   // token streaming
    EventBlockStop       EventType = "block_stop"
    EventMessageAppended EventType = "message_appended"
    EventApprovalReq     EventType = "approval_requested"
    EventThreadStatus    EventType = "thread_status"
    EventRoundDone       EventType = "round_done"
    EventQueueDrained    EventType = "queue_drained"  // signals stream end
)

type SSEEvent struct {
    Type EventType `json:"type"`
    Data any       `json:"data,omitempty"`
}
```

Each event type has a corresponding typed payload struct (e.g. `BlockDeltaData`).

### 3b. Hub — in-memory channel per conversation (`agent/sse.go`)

```go
type Hub struct {
    ch     chan SSEEvent   // buffered channel, cap 256
    mu     sync.Mutex
    closed bool
    log    *slog.Logger
}
```

The agent loop calls `hub.Push(event)` to enqueue events.
The HTTP handler reads from `hub.Chan()` and writes them to the response.

**Buffer-full strategy:** if all 256 slots are taken, `Push` drains the channel (discards pending
events) before writing the new one. This guarantees that terminal signals like `round_done` and
`queue_drained` are never lost — they would be invisible to the client otherwise.

### 3c. HubRegistry — one Hub per active conversation

```go
type HubRegistry struct {
    mu   sync.RWMutex
    hubs map[string]*Hub
}
```

`GetOrCreate(convID)` returns the existing Hub or creates a fresh one.
`Delete(convID)` closes and removes it after the stream ends.

This decouples the agent (writer) from the HTTP handler (reader): the agent doesn't know or care
whether a client is currently connected.

### 3d. HTTP handler — streaming the channel to the client (`handler/stream.go`)

```go
func (h *StreamHandler) Stream(c *gin.Context) {
    convID := c.Param("id")
    // ... existence check ...

    hub := h.registry.GetOrCreate(convID)

    c.Header("Cache-Control", "no-cache")
    c.Header("X-Accel-Buffering", "no")   // disables nginx buffering
    c.Header("Content-Type", "text/event-stream")

    c.Stream(func(w io.Writer) bool {
        select {
        case event, ok := <-hub.Chan():
            if !ok {
                return false           // channel closed → stop
            }
            data, _ := json.Marshal(event)
            c.SSEvent(string(event.Type), string(data))
            if event.Type == agent.EventQueueDrained {
                h.registry.Delete(convID)
                return false           // all events sent → stop
            }
            return true                // keep streaming
        case <-c.Request.Context().Done():
            return false               // client disconnected → stop
        }
    })

    h.registry.Delete(convID)
}
```

`c.Stream` is Gin's helper: it calls the callback in a loop, flushing after each call.
Returning `true` continues the loop; `false` closes the response.

---

## 4. How multiple events are sent

The callback passed to `c.Stream` is called repeatedly (it is a loop, not a one-shot call).
Each invocation blocks on the `select` until either:

- a new event arrives from `hub.Chan()` → serialize, write SSE frame, return `true` (continue)
- `EventQueueDrained` arrives → write final frame, return `false` (stop)
- client disconnects → return `false` (stop)

So the sequence for one AI response looks like:

```
agent loop                     Hub.ch (buffer)              HTTP handler (c.Stream loop)
──────────────────────────────────────────────────────────────────────────────────────
Push(agent_start)          →   [agent_start]            →   write "event: agent_start\ndata:...\n\n"
Push(block_start)          →   [block_start]            →   write "event: block_start\ndata:...\n\n"
Push(block_delta, "He")    →   [block_delta]            →   write "event: block_delta\ndata:...\n\n"
Push(block_delta, "llo")   →   [block_delta]            →   write "event: block_delta\ndata:...\n\n"
Push(block_stop)           →   [block_stop]             →   write "event: block_stop\ndata:...\n\n"
Push(round_done)           →   [round_done]             →   write "event: round_done\ndata:...\n\n"
Push(queue_drained)        →   [queue_drained]          →   write + return false → response ends
```

Gin calls `http.Flusher.Flush()` after each frame, so the browser receives each event immediately
without waiting for the response to close.

---

## 5. Frontend consumption (pattern)

```js
const es = new EventSource(`/api/conversations/${id}/stream`);

es.addEventListener('block_delta', (e) => {
    const payload = JSON.parse(e.data);   // SSEEvent wrapper
    const delta = JSON.parse(payload.data); // BlockDeltaData
    appendToken(delta.text);
});

es.addEventListener('queue_drained', () => {
    es.close();
});
```

Each `event:` name maps to a separate `addEventListener`, so different event types can be handled
independently without a top-level type-switch.

---

## Summary

| Concept | Implementation |
|---|---|
| Event definitions | `EventType` constants + typed `Data` structs in `agent/sse.go` |
| Producer→consumer decoupling | Buffered `chan SSEEvent` inside `Hub` |
| One hub per conversation | `HubRegistry` (map + RWMutex) |
| Sending multiple events | `c.Stream` loop — each callback invocation = one SSE frame flushed immediately |
| Stream termination | `EventQueueDrained` → handler returns `false`, response closes |
| Client disconnect | `c.Request.Context().Done()` → handler returns `false` cleanly |
