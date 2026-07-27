# ph3 — Scheduled Reports (定时日报)

**Goal**: Automated daily/weekly work summaries written into dedicated conversations.
**Prerequisite**: ph2 complete (MCP connected to GitHub + Jira).

---

## 3.1 Scheduler Setup (`internal/scheduler/scheduler.go`)

```go
type Scheduler struct {
    cron    *cron.Cron
    db      *gorm.DB
    agent   *agent.Service
    log     *slog.Logger
}

func ProvideScheduler(db *gorm.DB, agentSvc *agent.Service, log *slog.Logger) *Scheduler

// Start registers all jobs and starts the cron runner.
// Called from main() after HTTP server is started.
func (s *Scheduler) Start()

// Stop gracefully stops the cron runner (waits for running jobs to finish).
func (s *Scheduler) Stop()
```

Cron runner: `cron.New(cron.WithSeconds())` for full schedule flexibility.

## 3.2 Daily GitHub Report

**Schedule**: `0 9 * * *` (09:00 local time every weekday — adjust if needed)

**Job** (`internal/scheduler/daily_report.go`):

```go
func (s *Scheduler) runDailyReport(ctx context.Context)
```

Steps:
1. Compute date range: yesterday 00:00:00 → 23:59:59
2. Query **both** GitHub MCP instances for:
   - Commits authored by the current user in that range
   - PRs merged by the current user in that range
3. **Bot filter**: skip any commit/PR where author login contains `serviceuser` or `[bot]`
4. Group results by repository
5. Format as structured Markdown report
6. Write to the fixed **"daily-report" conversation** (created on first run, reused thereafter; pinned in frontend)
7. Trigger `agent.Service.Run` with the report as a user message so SSE pushes it live

**Fixed conversation**: identified by `user_id + title = "daily-report"`. Created via upsert on startup if absent.

## 3.3 Weekly Draft

**Schedule**: `0 17 * * 5` (17:00 every Friday)

**Job** (`internal/scheduler/weekly_draft.go`):

Steps:
1. Compute date range: Monday 00:00 → Friday 17:00 of the current week
2. Query both GitHub instances for own commits + merged PRs in range
3. Query Jira MCP for issues closed by the user this week
4. Format as copyable plain text (bullet list per project)
5. Write to the fixed **"weekly-draft" conversation**

## 3.4 Sprint Board Snapshot

**Trigger**: once at server startup (not cron)

**Job** (`internal/scheduler/sprint_board.go`):

Steps:
1. Query Jira MCP for current Sprint issues assigned to the authenticated user
2. For each issue: fetch linked PR status from GitHub MCP
3. Format as a Markdown table (issue key, summary, status, PR link, PR status)
4. Write to the fixed **"sprint-board" conversation**

## 3.5 Fixed Conversation Management

Helper used by all three jobs:

```go
// GetOrCreateFixedConversation returns the ID of a conversation with the given
// title for the system user, creating it if absent.
// These conversations are "pinned" — the frontend should display them at the top.
func GetOrCreateFixedConversation(ctx context.Context, db *gorm.DB, title string) (string, error)
```

Fixed conversation titles:
- `"daily-report"`
- `"weekly-draft"`
- `"sprint-board"`

Frontend distinguishes pinned conversations by a `pinned BOOLEAN` column (or by well-known title prefix).

**DB migration** (if adding `pinned` column): `000005_add_pinned_to_conversations.up.sql`

## 3.6 Files to Create/Modify

**New:**
- `internal/scheduler/scheduler.go`
- `internal/scheduler/daily_report.go`
- `internal/scheduler/weekly_draft.go`
- `internal/scheduler/sprint_board.go`
- `internal/scheduler/fixed_conv.go`
- `db/migrations/000005_add_pinned_to_conversations.{up,down}.sql` _(if pinned column needed)_

**Modified:**
- `cmd/server/main.go` — start/stop scheduler
- `cmd/server/wire.go` + `wire_gen.go` — add `ProvideScheduler`

---

## Verification

```bash
# Force-run daily report immediately (add a test-trigger HTTP endpoint or CLI flag):
curl -X POST /api/admin/trigger/daily-report

# Verify:
# 1. "daily-report" conversation created in DB
# 2. SSE event stream shows message_appended with report content
# 3. Bot commits (login contains "serviceuser" or "[bot]") are absent from output
```
