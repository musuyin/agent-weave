# Frontend (Vue 3)

## Commands

```bash
pnpm install
pnpm dev
pnpm build
pnpm test
```

## Tech stack

| 层 | 选型 |
|---|---|
| 框架 | Vue 3（`<script setup>` + Composition API）|
| UI 库 | Naive UI 或 Element Plus |
| 全局状态 | Pinia |
| 服务端状态 | TanStack Query for Vue |
| 图表 | ECharts（`vue-echarts`）|
| 构建 | Vite |

## SSE Streaming

通过 SSE 接收后端 Agent 输出，事件协议见 `server/CLAUDE.md` SSE Event Protocol 一节。

关键前端行为：
- `agent_start`：创建流式气泡
- `block_start/delta/stop`：以 block_id 追加/更新气泡内容
- `message_appended`：插入独立新消息（审批气泡、diff 展示）
- `round_done` / `queue_drained`：清理 streaming 状态并断开连接；丢失会导致界面卡在"思考中"
- 消息以 ID 为 map key，子 Agent 回复消息 ID 由后端独立生成，不会与用户消息 ID 重复

## Approval UI

审批气泡通过 `message_appended` 事件推送，作为独立消息渲染。用户操作后前端调用审批 API，后端处理顺序：先写 DB → 再 signal channel。

## Visualization

`deploy_app` 工具部署的可视化页面通过 `/preview/{conv_id}/{path}` 反代访问，前端用 `<iframe>` 嵌入，刷新时追加 `?t=timestamp` 参数强制重载。
