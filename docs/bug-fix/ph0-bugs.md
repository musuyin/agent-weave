# Phase 0 Bug Fix Reminders

Discovered during code review on 2026-07-27.

---

## BUG-1 (CRITICAL — 3 tests failing): `datetime(3)` breaks SQLite time scanning

**Files:** `internal/model/conversation.go`, `internal/model/message.go`, `internal/model/thread.go`

**Symptom:** `TestConversation_CreateAndList`, `TestMessage_ListOrdering`, `TestMessage_ListKeysetCursor` all return HTTP 500 with:
```
sql: Scan error on column index N, name "created_at": unsupported Scan, storing driver.Value type string into type *time.Time
```

**Root cause:** `go-sqlite3` only auto-parses `time.Time` for columns typed exactly `DATETIME` or `TIMESTAMP`. The `(3)` precision suffix in `gorm:"type:datetime(3)"` makes AutoMigrate create the column as `datetime(3)` in SQLite, which the driver treats as an unknown string type.

**Fix:** Replace `type:datetime(3)` with GORM's precision tags. MySQL still gets millisecond precision; SQLite gets a plain `datetime` column the driver can scan.

```go
// conversation.go
CreatedAt time.Time `gorm:"not null;autoCreateTime:milli"`
UpdatedAt time.Time `gorm:"not null;autoUpdateTime:milli"`

// message.go
CreatedAt time.Time `gorm:"not null;autoCreateTime:milli"`

// thread.go
CreatedAt time.Time `gorm:"not null;autoCreateTime:milli"`
UpdatedAt time.Time `gorm:"not null;autoUpdateTime:milli"`
```

---

## BUG-2: `mysqlDSN` silently omits `parseTime=true` when URL already has query params

**File:** `internal/db/db.go:54`

**Symptom:** No test failure in this repo (tests use SQLite), but production MySQL will fail to scan `time.Time` fields if the `database_url` contains any query parameter (e.g. `?charset=utf8`).

**Root cause:**
```go
if !strings.Contains(dsn, "?") {
    dsn += "?parseTime=true&charset=utf8mb4&loc=UTC"
}
```
If `?` is already present, the entire block is skipped — `parseTime=true` is never added.

**Fix:** Ensure required params are always appended:
```go
if strings.Contains(dsn, "?") {
    if !strings.Contains(dsn, "parseTime") {
        dsn += "&parseTime=true&loc=UTC"
    }
} else {
    dsn += "?parseTime=true&charset=utf8mb4&loc=UTC"
}
```

---

## DESIGN-1: Stream handler creates Hub for non-existent conversation → SSE hangs forever

**File:** `internal/handler/stream.go:24–27`

**Symptom:** `GET /api/conversations/nonexistent-id/stream` creates a Hub with `GetOrCreate`, then blocks in the channel select. `queue_drained` is never pushed so the connection never closes.

**Root cause:** `StreamHandler.Stream` calls `registry.GetOrCreate(convID)` with no prior existence check. The `Send` handler correctly calls `requireConversation` first; `Stream` does not.

**Fix:** Add a conversation existence check before `GetOrCreate`, mirroring `MessageHandler`:
```go
func (h *StreamHandler) Stream(c *gin.Context) {
    convID := c.Param("id")
    // validate conversation exists (reuse requireConversation pattern or inject MessageService)
    ...
    hub := h.registry.GetOrCreate(convID)
```

---

## DEAD-1: `sqlDBFromGorm` is defined but never called

**File:** `internal/db/db.go:60–63`

```go
// sqlDBFromGorm is a helper used by migrate.go.
func sqlDBFromGorm(db *gorm.DB) (*sql.DB, error) {
    return db.DB()
}
```

`migrate.go` does not call this. `ProvideDB` extracts `sql.DB` inline. The comment is wrong. Delete this function.

---

## MINOR-1: Double `time.Now()` in `ConversationService.Create`

**File:** `internal/service/conversation.go:39–40`

```go
CreatedAt: time.Now().UTC(),
UpdatedAt: time.Now().UTC(),
```

Two separate calls can produce microsecond-apart timestamps. Use a single `now`:
```go
now := time.Now().UTC()
conv := model.Conversation{..., CreatedAt: now, UpdatedAt: now}
```

---

## MINOR-2: Port from `config.yaml` is ignored in `main.go`

**File:** `cmd/server/main.go:53`

`getPort()` reads `$PORT` env var and hard-falls-back to `"8080"`. `cfg.Server.Port` loaded from `config.yaml` is never consulted. Either use it as the fallback or document that port is env-only.

---

## DOC-1: `docs/ph0-foundation.md` references non-existent files and a dropped field

- File list (§"File List") references `internal/auth/service.go`, `internal/api/auth.go`, `internal/api/router.go`, etc. — these don't exist. The actual package is `internal/handler/`, not `internal/api/`.
- §0.5 says `Conversation{ID, UserID, Title, CreatedAt, UpdatedAt}` but `UserID` was intentionally dropped (auth deferred). Update the model description.
