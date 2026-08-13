# Phase 8 — Docker Sandbox

## Goal

Provide OS-level isolation for agent file and command operations. Each conversation
gets its own Docker container with a mounted workspace directory. The container is
created lazily on the first file/command tool call and stopped when the server shuts
down.

---

## Scope

Tools that run inside Docker:

| Tool | Description |
|---|---|
| `read_file` | Read a file at a path relative to `/workspace` |
| `list_directory` | List a directory relative to `/workspace` |
| `write_file` | Write content to a file, creating parent dirs |
| `run_command` | Execute a shell command inside the sandbox |

Tools that stay on host (no sandbox benefit):

- MCP tools — call external network services
- `fetch_url` — validated HTTP GET, no filesystem access
- `dispatch_to_agent` — fan-out control flow, no I/O

---

## Architecture

```
internal/sandbox/
  container.go   Container — exec / read / write / list operations
  manager.go     Manager — per-conversation container lifecycle + tool registration
  provider.go    Wire provider (ProvideManager)
```

`Manager` is injected into `agent.ProvideAgentService`. After the Service is
constructed, `ProvideAgentService` calls `sandboxMgr.RegisterTools(conversationIDFromCtx)`,
which overwrites the empty stubs that previously lived in the builtin files.

---

## Container Lifecycle

1. **Lazy create** — `Manager.Ensure(ctx, convID)` is called the first time a tool
   needs the sandbox. It creates the host workspace dir, calls `ContainerCreate` +
   `ContainerStart`, and caches the `*Container` keyed by conversation ID.
2. **Persistent** — the same container is reused for the entire conversation. File
   writes in one tool call are visible to subsequent tool calls and `run_command`.
3. **Shutdown** — `Manager.Close()` (called via Wire cleanup func) stops all running
   containers. `AutoRemove: true` on the container ensures Docker removes the
   container filesystem once stopped.

---

## Container Configuration

```
Image:      cfg.Sandbox.Image    (default: "ubuntu:24.04")
Cmd:        ["sleep", "infinity"]
WorkingDir: /workspace
Binds:      {baseDir}/{convID}:/workspace
Memory:     256 MB
AutoRemove: true
```

Host workspace is at `{cfg.Sandbox.WorkspaceDir}/{convID}/` (default: `./sandbox/{convID}/`).
Files written by the agent persist on the host after the container is removed.

---

## Path Validation (Invariant I)

All paths are validated in `container.go` before any Docker call:

```go
resolved := filepath.Join("/workspace", filepath.Clean(path))
// resolved must equal "/workspace" or start with "/workspace/"
```

An absolute path or a traversal like `../../etc/passwd` is rejected with an error
result returned to the model — never a panic or host filesystem access.

---

## Tool Registration

`Manager.RegisterTools` calls `tool.Register` for all four tools. Since `tool.Register`
overwrites the map entry, any prior stub registration is replaced. The builtin files
(`read_file.go`, `list_directory.go`, `write_file.go`, `run_command.go`) contain only
comments; they carry no `init()` function.

---

## Config

```yaml
sandbox:
  image: "ubuntu:24.04"   # Docker image for sandbox containers
  workspace_dir: "./sandbox"  # host-side base directory for workspace mounts
```

Both fields are optional. Defaults are applied in `config.Load()`.

---

## Wire Integration

`wire.go`:
```go
sandbox.ProvideManager,   // added to wire.Build
```

`ProvideAgentService` signature gains `sandboxMgr *sandbox.Manager` as a new parameter.
`wire ./cmd/server/` regenerates `wire_gen.go` with:
```go
sandboxManager, cleanup3, err := sandbox.ProvideManager(ctx, configConfig, slogger)
service2 := agent.ProvideAgentService(..., sandboxManager)
// cleanup3 added to the returned cleanup closure
```

---

## Invariants Added

| # | Invariant |
|---|---|
| I | Sandbox paths validated before every Docker call — no traversal possible |
| J | Container created once per conversation; never recreated during its lifetime |
| K | Tool results from sandbox tools capped at 16 KB via `tool.Truncate` |

---

## Verification

```bash
# Prerequisites: Docker running, ubuntu:24.04 image available
docker pull ubuntu:24.04

# Start server
cd server && go run ./cmd/server/

# In the web UI — create a conversation, then send:
# "Write a Python script at hello.py that prints 'Hello from sandbox', then run it."

# Verify while server is running:
docker ps                            # shows sandbox-<convID[:8]> container
ls sandbox/<convID>/                 # shows hello.py on the host
# Agent response includes "Hello from sandbox"

# After stopping the server:
docker ps                            # container is gone (AutoRemove)

# Tests (no Docker required):
go test ./test/... -race             # all existing tests pass
```
