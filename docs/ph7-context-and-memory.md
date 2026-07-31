# ph7 — Context Compaction (Memory Management)

**Goal**: Keep the active context window small as conversations grow, using a
**pinned head + compacted middle + live tail** model.

Status: **complete** (context compaction implemented; long-term memory deferred).

---

## Implemented: Context Compaction

### Strategy

```
[first 4 messages  — pinned head, never dropped]
[1 synthetic "summary" message — compacted middle]
[last 10 messages  — live tail]
```

Triggered when `loadHistory` returns ≥ 40 non-compacted messages.

### Flow

```
loadHistory()
  └─ query DB: conversation_id = X AND compacted = false, ORDER BY created_at ASC, id ASC
  └─ maybeCompact(msgs)
       ├─ len(msgs) < 40 → return as-is
       └─ len(msgs) >= 40:
            head   = msgs[:4]
            tail   = msgs[len-10:]
            middle = msgs[4 : len-10]
            compactMessages(middle, tail)
              ├─ buildCompactionPrompt(middle, tail)   ← both slices sent to LLM
              ├─ LLM call → summaryText
              ├─ INSERT summary message (role=user, compacted=false, created_at = last_middle.created_at + 1ms)
              └─ UPDATE messages SET compacted=1 WHERE id IN (middle IDs)
            return [head... summary tail...]
```

### Compaction prompt technique

The LLM receives **both** `OLD MESSAGES` (candidates for compression) **and**
`RECENT MESSAGES` (the live tail, for context only). This lets the model drop
content that has already been resolved or superseded — producing tighter summaries
than sending only the old messages.

The summary is prepended with `[Context summary]\n` so the agent loop can
distinguish it from regular user messages.

### DB change

`messages.compacted TINYINT(1) DEFAULT 0`

- `compacted = 1`: excluded from `loadHistory` forever
- `compacted = 0`: included (both regular messages and the synthetic summary)
- Migration: `db/migrations/000009_add_compacted_to_messages.up.sql`

### Parameters (`internal/agent/compaction.go`)

```go
const (
    CompactThreshold = 40  // trigger when non-compacted history >= this
    PinnedHead       = 4   // messages always kept at the front
    LiveTail         = 10  // messages always kept at the end
)
```

### Non-fatal design

If the compaction LLM call fails (network error, empty response, DB write failure),
`loadHistory` logs a warning at `WARN` level and falls through with the original
full history. The agent loop is **never blocked** by a compaction failure.

### Files

| File | Role |
|---|---|
| `db/migrations/000009_add_compacted_to_messages.up.sql` | Adds `compacted` column |
| `internal/model/repository/message.go` | `Compacted bool` field on `Message` |
| `internal/agent/compaction.go` | `maybeCompact`, `compactMessages`, `renderMessages`, constants |
| `internal/agent/history.go` | `compacted = false` filter + `maybeCompact` call in `loadHistory` |
| `test/agent/compaction_test.go` | Split invariants + DB filter tests |

---

## Deferred: Long-term Memory

Memory tools (`save_memory`, `read_memory`) and `MEMORY.md` index injection into
the system prompt (layer 5 of the six-layer prompt) are deferred — context
compaction covers the primary token-cost concern for this learning project.

When implemented, the design is:
- `MEMORY.md` lives in the project root; each line is a one-line summary + file pointer
- System prompt layer 5 injects the full `MEMORY.md` content each round (dynamic layer)
- `save_memory` tool: append to `MEMORY.md` + write the memory file
- `read_memory` tool: load a specific memory file by name (progressive disclosure)
