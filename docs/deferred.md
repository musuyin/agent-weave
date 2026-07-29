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

**What needs to be built when unblocked**: See `ph2-mcp.md`, `ph3-scheduled-reports.md`.
