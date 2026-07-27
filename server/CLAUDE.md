# Backend (Go)

## Commands

```bash
go build ./...
go test ./...
go test ./path/to/package -run TestFunctionName
wire ./...                   # regenerate DI wiring
golangci-lint run ./...

# DB migrations
migrate -path db/migrations -database "mysql://..." up
migrate -path db/migrations -database "mysql://..." down 1
```

## Tech stack

| 层 | 选型 |
|---|---|
| HTTP | `gin` |
| ORM | `gorm` (MySQL) |
| DB 迁移 | `golang-migrate` |
| 依赖注入 | `google/wire`（编译期，生成 `wire_gen.go`）|
| AI SDK | `anthropics/anthropic-sdk-go` |
| OIDC 认证 | `coreos/go-oidc/v3` + `golang.org/x/oauth2` |
| MCP Client | `mark3labs/mcp-go`（stdio/SSE/HTTP 三种传输）|
| Docker | `docker/docker/client` |
| 定时任务 | `robfig/cron/v3` |
| 日志 | `slog` |
| 配置 | `kelseyhightower/envconfig`（环境变量驱动）|

## Architecture: Agent Loop

系统核心是一个 orchestrator 驱动的 while 循环：

```
每轮循环：
  1. 检查是否被中止
  2. 消费子 Agent 完成事件，注入摘要到消息历史
  3. 重算动态 system prompt（每轮）
  4. 调用 LLM API（流式）
  5. 处理响应：end_turn → 等待子 Agent 或结束；tool_use → 执行工具
  6. 必要时压缩上下文
```

### System Prompt 六层结构

静态层（loop 启动时构建一次）：
- 层 1：核心指令（orchestrator 角色定义，从 `.md` 文件读取）
- 层 2：工具列表（走 SDK tools 参数，不写入 prompt）
- 层 3：Skill 元数据（仅名称+描述，内容按需 `load_skill` 加载）
- 层 4：项目级指令（`AGENTHUB.md`）

动态层（每轮重算）：
- 层 5：长期记忆索引（`MEMORY.md` 摘要列表）
- 层 6：动态上下文（最近消息摘要 + 可用 Agent 列表 + 当前任务图状态）

层 6 查询任务图时必须绕过 GORM identity map 缓存（`db.Session(&gorm.Session{NewDB: true})`），直接读 DB。

### 关键执行顺序约束

**工具调用顺序（最重要）**：
```
✓ 正确：fire PRE_TOOL_USE hook → 写消息历史 → 执行工具
✗ 错误：写消息历史 → fire PRE_TOOL_USE hook → 执行工具
```
hook 可能修改工具入参；若先写历史，LLM 下轮会看到不一致的入参，产生幻觉。

**子 Thread 状态**：延迟启动前必须先标记为"运行中"，否则调度器误判无活跃任务。

**取消会话 Threads**：每个 Thread 用独立短事务，不共用长事务（orchestrator finally 块持有行锁）。

**子 Agent 消息 ID**：必须独立生成，不能复用触发该 Thread 的用户消息 ID（前端以 ID 为 map key）。

## Hook System

```
PRE_TOOL_USE（同步串行，可中止，可改参）：
  SecurityHook → ApprovalHook

POST_TOOL_USE（异步 goroutine，纯观察）：
  AuditHook
```

**审批流程顺序**：
```
✓ 正确：先写 DB（approved/rejected）→ 再 signal channel
✗ 错误：先 signal channel → 再写 DB（主循环恢复后 DB 仍是 pending）
```

审批落库失败：fail-closed，直接拒绝执行。

批量审批：同一轮 ≥2 个文件写操作合并为一次审批，通过后通过 context flag 放行后续单个操作。

审计日志只记录工具入参的 key，不记录 value（value 可能含文件内容、密钥）。

## MCP Tool Routing

Loop 启动时枚举所有 MCP 服务器工具，建 `toolName → MCPClient` 路由表。

两个内网 GitHub 实例需加前缀避免工具名冲突：
- `github.tools.sap`（orgs: hci, common-service-infrastructure）
- `github.wdf.sap.corp`（orgs: DBaaS, hanadatalake, delphi）

MCP OAuth token：优先从 DB 取缓存（提前 30s 判过期）→ Client Credentials 自动换 → OIDC 需用户重新授权。

## SSE Event Protocol

| 事件 | 说明 |
|---|---|
| `agent_start` | 开始输出 |
| `block_start/delta/stop` | Block 生命周期，携带 block_id |
| `message_appended` | 独立新消息（审批气泡、diff 展示必须用此事件，不能用 `block_start`）|
| `approval_requested` | 审批请求 |
| `thread_status` | 子 Thread 状态变更 |
| `round_done` | 本轮完成（信号性，队列满时清空再推，不能丢弃）|
| `queue_drained` | 队列清空，前端断开 SSE |

## Security Boundaries

- 文件操作 handler 层自行校验路径在 `sandbox/{conversation_id}/` 内（`filepath.Clean` + `filepath.Rel`），不依赖上层 hook。
- 子 Agent 无工具调用权限。
- 审批落库失败 fail-closed。
- 审计日志不记录工具入参 value。

## DB Schema Constraints

- `messages.content`：JSON 数组，存有序 ContentBlock（text/tool_use/approval/code 等）
- `threads.blocked_by`：JSON 数组，存依赖的 thread_id 列表
- `threads.agent_id = 'orchestrator'`：标识主 Agent thread
- `mcp_tokens`：唯一约束 `(user_id, server_id)`
- 消息分页：`(created_at, id)` 复合游标；游标 `id` 必须校验属于当前会话

## Auth (OIDC PKCE only)

```
/ → 未登录 → 重定向公司 IDP
/auth/oidc/callback → 完成 PKCE → 写 session cookie（httponly, secure）
```

- PKCE state 存进程内 `map[string]PKCEState`，`time.AfterFunc` 10 分钟 TTL 清理
- 无 users 表（claims 即身份），或极简 preferences 表
- logout 只清 cookie，无 JWT 黑名单
