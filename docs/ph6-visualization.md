# ph6 — Command-Driven Visualization (命令驱动可视化)

**Goal**: Agent generates HTML/JS charts and serves them as live previews via Docker containers.
**Prerequisite**: ph5 complete (file ops + sandbox). Docker daemon available locally.

---

## 6.1 `deploy_app` Tool (`internal/tool/builtin/deploy_app.go`)

```go
type DeployAppParams struct {
    Path string `json:"path"`  // path within sandbox to the HTML/JS app directory
    Port int    `json:"port"`  // internal container port (default 80)
}
```

8-step deploy sequence using `docker/docker/client`:

```
1. Validate sandbox path (handler-layer check, same as file ops)
2. Build Docker image from sandbox directory
   - Base image: nginx:alpine
   - Copy files into /usr/share/nginx/html/
   - Tag: agentweave/{conversation_id}:{timestamp}
3. Stop + remove any existing container for this conversation
4. Run new container:
   - Name: agentweave-{conversation_id}
   - Port binding: host random port → container port
   - Network: bridge (isolated from host network beyond the bound port)
   - Labels: conversation_id, deployed_at
5. Wait for container healthy (poll /health or just wait 2s)
6. Record deployment in DB: conversation_id, container_id, host_port, path
7. Return preview URL: /preview/{conversation_id}/{path}
```

High-risk tool — triggers `ApprovalHook` (same as file writes).

## 6.2 Reverse Proxy (`internal/api/preview.go`)

```go
// Route: GET /preview/:conv_id/*path
// No auth required for the preview route (Docker container serves its own content).
// Proxies to the container's bound host port.
func PreviewHandler(db *gorm.DB) gin.HandlerFunc
```

Implementation:
```go
func (h *PreviewHandler) ServeHTTP(c *gin.Context) {
    convID := c.Param("conv_id")
    // Look up host port for this conversation's deployed container
    deployment := // query DB
    target := &url.URL{
        Scheme: "http",
        Host:   fmt.Sprintf("localhost:%d", deployment.HostPort),
    }
    proxy := httputil.NewSingleHostReverseProxy(target)
    proxy.ServeHTTP(c.Writer, c.Request)
}
```

Route added to router (no auth group — previews are public for the local user):
```
GET /preview/:conv_id/*path
```

## 6.3 DB Migration

`000007_create_deployments.up.sql`:

```sql
CREATE TABLE deployments (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY,
    conversation_id VARCHAR(36)  NOT NULL,
    container_id    VARCHAR(128) NOT NULL,
    image_tag       VARCHAR(255) NOT NULL,
    host_port       INT          NOT NULL,
    sandbox_path    VARCHAR(500) NOT NULL,
    deployed_at     DATETIME(3)  NOT NULL,
    INDEX idx_deployments_conversation (conversation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

## 6.4 Agent Prompt for Visualization

The system prompt (layer 1 or layer 4) instructs the orchestrator how to use `deploy_app`:

```
When asked to visualize data:
1. Understand the data structure from the user's message or files
2. Choose the appropriate ECharts chart type
3. Generate a self-contained HTML file with embedded ECharts JS
   (use CDN: https://cdn.jsdelivr.net/npm/echarts/dist/echarts.min.js)
4. Write it to sandbox using create_file
5. Call deploy_app with the sandbox path
6. Return the preview URL to the user
```

## 6.5 Container Lifecycle

```go
// CleanupContainer stops and removes the container for a conversation.
// Called when: conversation deleted, new deploy requested, server shutdown.
func CleanupContainer(ctx context.Context, dockerClient *client.Client, convID string) error
```

On `deploy_app` step 3, the existing container for the conversation (if any) is stopped+removed before starting the new one.

On server shutdown: gracefully stop all managed containers (best-effort; containers survive Docker daemon restart anyway).

## 6.6 Files to Create/Modify

**New:**
- `internal/tool/builtin/deploy_app.go`
- `internal/api/preview.go`
- `db/migrations/000007_create_deployments.{up,down}.sql`

**Modified:**
- `internal/api/router.go` — add `/preview/:conv_id/*path` route (outside auth group)
- `cmd/server/wire.go` + `wire_gen.go` — add Docker client provider

---

## Verification

```bash
# 1. Ask agent: "Plot a bar chart of [1,2,3,4,5] with labels A-E"
# Expected: agent calls create_file → approval → deploy_app → approval
# → SSE message_appended with preview URL
# → curl /preview/{conv_id}/index.html → proxied nginx response

# 2. Second deploy to same conversation → old container removed, new one starts

# 3. Path traversal in deploy_app path → rejected at handler layer
```
