# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

```
server/   Go backend (Gin + GORM + Wire, Go 1.25)
web/      Vue 3 frontend (Vite)
docs/     Spec, design decisions, and per-phase implementation plans
```

Infrastructure: MySQL (persistence) + Docker (visualization containers). No Redis.

## Docs

- Full spec: `docs/项目重写说明.md`
- Per-phase plans: `docs/ph0-foundation.md` … `docs/ph7-context-and-memory.md`
- Deferred items: `docs/deferred.md`
- Overall roadmap: `docs/development-plan.md`

## Doc-sync rule

**After every implementation session, update the relevant `docs/ph*.md` to reflect the actual code.**
If the implementation diverges from the plan, fix the doc. Stale docs are worse than no docs.

## Implementation roadmap

```
Phase 0  基础骨架        ✓ complete
Phase 1  工具+Hook
Phase 2  MCP 接入
Phase 3  定时日报
Phase 4  子 Agent 调度
Phase 5  文件操作+审批
Phase 6  命令驱动可视化
Phase 7  上下文压缩+长期记忆
```

When working in `server/` or `web/`, open Claude Code from that subdirectory to load the relevant CLAUDE.md automatically.
