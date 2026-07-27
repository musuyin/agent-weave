# ph7 — Context Compression + Long-term Memory (上下文压缩+长期记忆)

**Goal**: Handle long conversations without hitting token limits; persist learnings across sessions.
**Prerequisite**: ph1 complete (agent loop). All other phases optional.

---

## 7.1 Context Compression (`internal/agent/compressor.go`)

Triggered when the accumulated token count approaches the model's context limit.

```go
type Compressor struct {
    aiClient *anthropic.Client
    db       *gorm.DB
    log      *slog.Logger
}

// MaybeCompress checks token usage and compresses if needed.
// Called at step 6 of the agent loop (after each round).
// Returns true if compression occurred.
func (c *Compressor) MaybeCompress(ctx context.Context, convID string, messages []anthropic.MessageParam) ([]anthropic.MessageParam, bool, error)
```

Compression strategy:
1. Count tokens in current message history (use `accMsg.Usage` from the API response)
2. Threshold: compress when `input_tokens > model_context_limit * 0.8` (e.g. > 160K for 200K context)
3. Identify a "safe tail" — keep the last N messages intact (default: last 20)
4. Summarize the head (everything before the safe tail) via a separate Anthropic API call:
   ```
   System: "Summarize the following conversation history concisely, preserving key decisions, facts, and tool results."
   ```
5. Replace the head with a single synthetic `user` message: `"[Conversation summary: ...]"`
6. Update the in-memory message slice (DB is NOT rewritten — compression is ephemeral per session)

## 7.2 Long-term Memory (`internal/memory/`)

Memory is stored as Markdown files in a per-user directory: `memory/{user_id}/`.

### 7.2.1 MEMORY.md Index

`memory/{user_id}/MEMORY.md` — index file, one line per memory entry:
```
- [Title](filename.md) — one-line description
```

Loaded in system prompt layer 5 each round:
```go
func LoadMemoryIndex(userID string) (string, error)
    // returns contents of MEMORY.md, or empty string if absent
```

### 7.2.2 Memory Read Tool

```go
// read_memory tool: load a specific memory file by slug
type ReadMemoryParams struct {
    Name string `json:"name"`  // slug from MEMORY.md index
}
```

### 7.2.3 Memory Write Tool

```go
// save_memory tool: create or update a memory file
type SaveMemoryParams struct {
    Name    string `json:"name"`     // kebab-case slug, also the filename
    Title   string `json:"title"`
    Content string `json:"content"`  // markdown body
    Type    string `json:"type"`     // user | feedback | project | reference
}
```

Handler:
1. Write `memory/{user_id}/{name}.md` with frontmatter + content
2. Upsert the entry in `MEMORY.md` index

### 7.2.4 System Prompt Layer 5

Dynamic layer rebuilt each round:
```go
func (s *Service) buildMemoryLayer(userID string) string {
    index, _ := memory.LoadMemoryIndex(userID)
    if index == "" {
        return ""
    }
    return "## Long-term Memory Index\n\n" + index
}
```

## 7.3 Agent Loop Update

**Step 3** (rebuild dynamic prompt) gains layer 5:
```go
systemPrompt = buildStaticLayers() +
    "\n---\n" +
    buildMemoryLayer(userID) +         // layer 5 — new
    buildDynamicContext(convID, db)    // layer 6
```

**Step 6** (compression — previously no-op):
```go
messages, compressed, err := compressor.MaybeCompress(ctx, convID, messages)
if compressed {
    slog.Info("context compressed", "conv_id", convID)
}
```

## 7.4 Oncall Alert Fast-Analysis

Pure prompt capability — no new tools needed.

Add to system prompt layer 1 (orchestrator instructions):
```
When given an oncall alert or error log:
1. Identify the service, error type, and timestamp
2. Search relevant MCP sources (GitHub, Jira) for related recent changes
3. Produce a structured triage: impact, likely cause, immediate mitigation steps
```

## 7.5 Files to Create/Modify

**New:**
- `internal/agent/compressor.go`
- `internal/memory/memory.go` — index read/write
- `internal/tool/builtin/read_memory.go`
- `internal/tool/builtin/save_memory.go`

**Modified:**
- `internal/agent/loop.go` — add step 6 compression, layer 5 in prompt builder
- `prompts/orchestrator.md` — add oncall triage instructions

---

## Verification

```bash
# 1. Context compression:
# Send 100+ messages to accumulate tokens
# After threshold: slog shows "context compressed"
# Subsequent messages still work correctly

# 2. Memory write:
# Agent calls save_memory → memory/{user_id}/test.md created
# MEMORY.md updated with new entry
# Next session: layer 5 includes the new entry

# 3. Memory read:
# Agent calls read_memory with slug → returns file content
```
