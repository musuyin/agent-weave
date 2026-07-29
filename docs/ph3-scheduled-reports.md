# ph3 — Manual Reports (手动报告)

**Goal**: On-demand work summaries triggered by a single API call. The caller hits an endpoint; the server sends a fixed prompt to the agent, which uses the GitHub MCP tools to query activity and write a report into a dedicated conversation.
**Prerequisite**: ph2 complete (MCP connected to GitHub).

No scheduler, no cron, no background jobs.

---

## 3.1 API

```
POST /api/reports/:type/run
```

`type` is one of: `daily` | `weekly`

**Response**: `202 Accepted` with the conversation ID the report is being written into.

```json
{ "conversation_id": "uuid" }
```

The caller then subscribes to `GET /api/conversations/:id/stream` to watch the agent write the report in real time, exactly like a normal conversation.

---

## 3.2 Fixed Conversations

Each report type writes into a dedicated conversation identified by a well-known title:

| type | title |
|---|---|
| `daily` | `"daily-report"` |
| `weekly` | `"weekly-report"` |

Helper used by the report handler:

```go
// GetOrCreateReportConversation returns the ID of the conversation with the given
// title, creating it if absent.
func GetOrCreateReportConversation(ctx context.Context, db *gorm.DB, title string) (string, error)
```

On first call the conversation is created. On subsequent calls the same conversation is reused and the new report is appended as additional messages.

---

## 3.3 Report Prompts

The handler saves a user message into the conversation and then calls `agent.Service.Run` — the same path as a normal `POST /conversations/:id/messages`. The agent uses whichever GitHub MCP tools it sees fit.

### Daily prompt

```
Generate a daily work report for today.
Use the GitHub MCP tools to find:
- Commits pushed today (filter out bot authors whose login contains "serviceuser" or "[bot]")
- Pull requests merged today
Group results by repository and format as a Markdown summary.
```

### Weekly prompt

```
Generate a weekly work report for this week (Monday to today).
Use the GitHub MCP tools to find:
- Commits pushed this week (filter bot authors)
- Pull requests opened and merged this week
Group results by repository and format as a copyable plain-text bullet list.
```

---

## 3.4 Files to Create/Modify

**New:**
- `internal/handler/report.go` — `ReportHandler` with `Run` method
- `internal/service/report.go` — `ReportService` with `GetOrCreateReportConversation` + prompt builder

**Modified:**
- `internal/handler/router.go` — add `POST /api/reports/:type/run`
- `cmd/server/wire.go` + `wire_gen.go` — add `ProvideReportService`, `NewReportHandler`

No new DB migrations needed — report conversations are plain `conversations` rows.

---

## Verification

```bash
# Start the server
go run ./cmd/server/

# Trigger a daily report
curl -X POST http://localhost:8080/api/reports/daily/run
# → {"conversation_id":"<uuid>"}

# Watch it stream
curl -N http://localhost:8080/api/conversations/<uuid>/stream
# Expect: agent_start, block_start/delta/stop for the GitHub tool calls, final text report, round_done
```
