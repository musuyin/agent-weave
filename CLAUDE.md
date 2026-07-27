# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

```
server/   Go backend (Gin + GORM + Wire, Go 1.25)
web/      Vue 3 frontend (Vite)
docs/     Spec and design documents
```

Infrastructure: MySQL (persistence) + Docker (visualization containers). No Redis.

Full spec: `docs/项目重写说明.md`

Implementation roadmap: 阶段 0（基础骨架）→ 1（工具+Hook）→ 2（MCP）→ 3（定时日报）→ 4（子 Agent 调度）→ 5（文件操作+审批）→ 6（命令驱动可视化）→ 7（上下文压缩+长期记忆）

When working in `server/` or `web/`, open Claude Code from that subdirectory to load the relevant CLAUDE.md automatically.
