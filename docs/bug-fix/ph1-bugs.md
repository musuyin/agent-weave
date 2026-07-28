# Phase 1 Bug Fix Reminders

Discovered during code review on 2026-07-28.

---

## BUG-1 (CRITICAL): Invariant A violated for multi-tool responses

**File:** `internal/agent/loop.go:199–215`

**Symptom:** When the model returns multiple `tool_use` blocks in a single response, the second and subsequent tools have `FirePre` called *after* the assistant message is already written to DB. Invariant A (`FirePre → write history → execute`) holds only for the first tool.

**Root cause:** The assistant message is persisted on the first loop iteration via `assistantPersisted` flag, but `FirePre` is called at the top of every iteration:

```go
for _, pt := range pending {
    preErr := s.chain.FirePre(ctx, &params)   // ← called each iteration

    if !assistantPersisted {                   // ← written only on first iteration
        s.persistMessage(..., assistantBlocks)
        assistantPersisted = true
    }
    // execute ...
}
```

For tools[1], [2], …[N]: history is written before `FirePre` fires — backwards.

**Fix:** Persist the assistant message once, before the loop, then run `FirePre` + execute inside the loop:

```go
// Persist assistant turn ONCE, before any hook fires.
if err := s.persistMessage(ctx, conversationID, "assistant", assistantBlocks); err != nil {
    return fmt.Errorf("persist assistant: %w", err)
}

for _, pt := range pending {
    params := hook.ToolCallParams{Name: pt.name, Params: pt.input}
    preErr := s.chain.FirePre(ctx, &params)
    // ... execute ...
    s.chain.FirePost(ctx, params, result, auditErr)
}
```

---

## BUG-2: `FirePost` receives `nil` error when a pre-hook denied the call

**File:** `internal/agent/loop.go:241`

**Symptom:** When `SecurityHook` (or any future pre-hook) blocks a tool call, `AuditHook` logs `"tool call succeeded"` instead of `"tool call failed"`. The audit trail is wrong.

**Root cause:**
```go
s.chain.FirePost(ctx, params, result, toolErr)
```
`toolErr` is `nil` when the tool was never executed (pre-hook aborted it). `preErr` is the actual error but is not passed to `FirePost`.

**Fix:**
```go
auditErr := toolErr
if preErr != nil {
    auditErr = preErr
}
s.chain.FirePost(ctx, params, result, auditErr)
```

---

## BUG-3 (CRITICAL): `buildToolParams` sends invalid JSON Schema to Anthropic API

**File:** `internal/agent/loop.go:277–297`

**Symptom:** The Anthropic API receives malformed tool schemas. The model likely cannot correctly use any tool because the input schema is a flat JSON object of zero values (`{"path":""}`) instead of a proper JSON Schema (`{"type":"object","properties":{"path":{"type":"string"}}}`).

**Root cause:** `d.InputSchema` is a zero-value Go struct. `json.Marshal` serialises it as a plain JSON object, not a JSON Schema. The spec says "Go struct → serialized to JSON Schema" but that conversion is not implemented:

```go
schemaBytes, err := json.Marshal(d.InputSchema)  // produces {"path":""} — wrong
var props map[string]any
_ = json.Unmarshal(schemaBytes, &props)           // props = {"path": ""} — not a schema
```

**Fix (option A — manual schema per tool):** Define `InputSchema` as a pre-built `map[string]any` JSON Schema directly in each tool's `init()`:

```go
InputSchema: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "path": map[string]any{
            "type":        "string",
            "description": "Absolute or relative path to the file to read.",
        },
    },
    "required": []string{"path"},
},
```

**Fix (option B — schema generation library):** Use `github.com/invopop/jsonschema` (already in `go.sum` as an indirect dep) to reflect the struct into a proper schema.

---

## DESIGN-1: `fetch_url` ignores caller's context, uses `context.Background()`

**File:** `internal/tool/builtin/fetch_url.go:39`

**Symptom:** If the agent run is cancelled (e.g. conversation deleted, server shutting down), in-flight HTTP requests from `fetch_url` are not cancelled. They run to their 30-second timeout regardless.

**Root cause:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
```
The `ctx` parameter passed to the handler is discarded.

**Fix:**
```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
```

---

## DESIGN-2: `description` struct tag on `InputSchema` has no effect

**Files:** `internal/tool/builtin/read_file.go`, `list_directory.go`, `fetch_url.go`

**Symptom:** Tool parameter descriptions are silently dropped — they never reach the Anthropic API.

**Root cause:** `description:"..."` is not a recognised Go struct tag by any standard or used library. It is ignored at runtime:

```go
InputSchema: struct {
    Path string `json:"path" description:"Absolute or relative path..."`
}{},
```

**Fix:** Once BUG-3 is resolved with proper JSON Schema construction, move descriptions into the schema's `"description"` field rather than a struct tag.

---

## DESIGN-3: Tool list duplicated between `orchestrator.md` and `buildSystemPrompt`

**Files:** `internal/prompts/orchestrator.md`, `internal/agent/loop.go:262–273`

**Symptom:** The model receives the tool list twice per request — once hardcoded in the static system prompt and once in the dynamically generated `## Tool Reference` section. This wastes tokens and will drift out of sync when tools are added or removed.

**Fix:** Remove the `## Available Tools` section from `orchestrator.md`. The dynamic section in `buildSystemPrompt` is the authoritative list and stays current automatically.

---

## DESIGN-4: `respond_to_user` tool specified but not implemented

**File:** spec `docs/ph1-tools-and-hooks.md §1.5`, `internal/tool/builtin/`

**Symptom:** The orchestrator prompt and spec reference `respond_to_user` as the tool that signals the loop to emit a `message_appended` SSE event and end the turn. The file `respond_to_user.go` does not exist, and the loop has no `message_appended` push path. The model cannot cleanly signal turn completion via tool.

**Fix:** Create `internal/tool/builtin/respond_to_user.go` that sets a sentinel in context or returns a special marker string, and handle that marker in `dispatchTools` to push `EventMessageAppended` and break the loop.

---

## MISSING-1: No tests for any Phase 1 code

**Packages:** `internal/tool`, `internal/hook`, `internal/tool/builtin`, updated `internal/agent`

The Phase 1 additions have zero test coverage. Minimum tests needed:

| Test | What it checks |
|---|---|
| `TestTruncate` | UTF-8 boundary safety, exact-limit passthrough, no-op when short |
| `TestFirePre_Abort` | First failing hook aborts chain; second hook not called |
| `TestAuditHook_KeysOnly` | Logged keys match param keys; values never appear in log output |
| `TestDispatchTools_InvariantA` | For a 2-tool response, assistant message persisted before both `FirePre` calls |
| `TestFetchURL_ContextCancellation` | Cancelled ctx aborts in-flight request (after DESIGN-1 fix) |
