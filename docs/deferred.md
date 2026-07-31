# Deferred Items — Cannot Implement Now

These items are blocked on external dependencies or setup that is not yet available.

---

## 1. OIDC Authentication (`internal/auth/`)

**Blocked by**: Company OIDC IDP registration — need `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_PROVIDER_URL`.

**What needs to be built when unblocked**:
- `internal/auth/service.go` — `Service` struct with PKCE state map + `time.AfterFunc` 10-min TTL
  - `StartOIDCFlow() (redirectURL string, err error)`
  - `CompleteOIDCCallback(ctx, state, code string) (*UserClaims, error)`
  - `SetSessionCookie(c *gin.Context, claims *UserClaims)` — hand-rolled HMAC-SHA256, httponly + secure + SameSite=Lax
  - `ClearSessionCookie(c *gin.Context)`
  - `ValidateSession(c *gin.Context) (*UserClaims, error)`
- `internal/auth/middleware.go` — `AuthMiddleware`, `GetCurrentUser`
- Auth routes: `GET /auth/oidc/login`, `GET /auth/oidc/callback`, `POST /auth/logout`
- Session cookie: JSON-marshal claims → HMAC-SHA256(SESSION_SECRET) → base64url, format `base64(json).base64(hmac)`
- Wire providers: `ProvideAuthService(ctx, cfg)`

**Config keys needed** (currently absent from `config.yaml`):
- `oidc_provider_url`
- `oidc_client_id`
- `oidc_client_secret`
- `session_secret`
- `base_url`

**Temporary bypass in place**: all API routes currently have no auth. Add `AuthMiddleware` to the authenticated route group in `internal/handler/router.go` once OIDC is ready.

**Architecture invariants to preserve**:
- PKCE state: in-memory `map[string]PKCEState` only (no DB, no Redis)
- `time.AfterFunc` 10-min TTL cleanup — no background goroutine needed
- No users table; claims are the identity
- logout only clears cookie; no JWT blacklist

---

## 2. MCP OAuth Token Refresh (Phase 2)

**Blocked by**: Internal MCP server OAuth credentials.

**Current workaround**: MCP servers are authenticated via a static `Authorization: Bearer <PAT>` header in `config.yaml` (`mcp.servers[*].headers`). This is sufficient for a single shared PAT but incompatible with per-user OAuth flows. When a PAT expires, update `config.yaml` and restart the server.

**What needs to be built when unblocked**: See `ph2-mcp.md` section 2.4 — `TokenManager`, `mcp_tokens` DB table, per-server `OAuthConfig`, OIDC re-auth error surfaced as SSE event.

---

## 3. Company GitHub + Jira MCP Servers (Phase 2, 3)

**Blocked by**: Network access + MCP server URLs for `github.tools.sap`, `github.wdf.sap.corp`, Jira.

**What needs to be built when unblocked**: See `ph2-mcp.md`, `ph3-manual-reports.md`.

---

## 4. Phase 5 — File Operations + Approval (文件操作+审批)

**Skipped** — not needed for the current learning-project scope.

**Design spec** (preserved here for reference):

Every conversation gets an isolated sandbox at `sandbox/{conversation_id}/`. All file tool handlers validate the path at the handler layer (not hook layer) using `filepath.Clean` + `filepath.Rel` so the check cannot be bypassed even if hooks are skipped (invariant G).

**Tools to build**:
- `create_file` — sandbox-validate, trigger ApprovalHook, `os.MkdirAll` + `os.WriteFile`
- `edit_file` — sandbox-validate, trigger ApprovalHook, read → replace first occurrence → write; push diff as `message_appended` SSE event

**ApprovalHook** (`internal/hook/approval_hook.go`): fires in PRE_TOOL_USE for high-risk tools.
1. Write approval record to DB (status=`pending`) — **fail-closed**: if DB write fails, reject the tool call
2. Push `approval_requested` SSE event
3. Block on a per-approval channel (120 s timeout → auto-reject)

Decision endpoint: `POST /api/conversations/:id/approvals/:block_id` with `{"decision":"approved"|"rejected"}` — writes DB first, then signals channel (invariant F).

**Batch approval**: ≥2 file writes in one round → single merged approval bubble; subsequent calls in the same round skip via a context flag.

**AuditHook enhancement**: add structured DB logging (`audit_logs` table) — `ToolName`, `ParamKeys []string` (keys only, never values, invariant H), `Success`, `ErrorMessage`.

**Migration needed**: `000010_create_audit_logs.up.sql` (or adjust number to follow current sequence).

---

## 5. Phase 6 — Command-Driven Visualization (命令驱动可视化)

**Skipped** — not needed for the current learning-project scope. Requires Docker daemon.

**Design spec** (preserved here for reference):

**`deploy_app` tool** (`internal/tool/builtin/deploy_app.go`): 8-step Docker deploy — validate sandbox path → build image (`nginx:alpine`, copy files) → stop/remove existing container for conversation → run new container (bridge network, random host port) → wait for healthy → record in DB → return preview URL.

High-risk tool — triggers ApprovalHook (same as file writes).

**Reverse proxy** (`GET /preview/:conv_id/*path`): looks up host port from DB, proxies via `httputil.NewSingleHostReverseProxy`. Route is outside the auth group (public for local user).

**Migration needed**: `deployments` table (conversation_id, container_id, image_tag, host_port, sandbox_path, deployed_at).

**Docker client**: `docker/docker/client` — add provider to wire graph.

**Agent prompt addition**: instruct orchestrator to generate self-contained HTML+ECharts, write via `create_file`, then call `deploy_app`.
