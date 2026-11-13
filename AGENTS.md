# TinyNest / 小家 · 任意 Coding Agent 规范

> **状态说明（2026-11-13）**：本文件原为 **Phase 0** 规范。自 `feature/tinynest-phase0-v0.1` 起，项目已进入 **V0.1 家庭账本主体** 阶段：复式记账内核、真实 SQLite 持久化、登录/会话/CSRF/审计、真实 API 前端界面均已实现。
> 凡标注「Phase 0（历史）」的条目只描述历史阶段，**不再约束当前代码**。当前开发请以「V0.1 当前守则」和 V0.1 设计/审查文档为准。原始历史计划（`plan.md`、`.plan/*`）保留原文，不擅自改写。

## 当前阶段从 main 开始

- 项目入口为 TinyNest 仓库根目录。本机主工作区为 `D:/Code/tinynest`，远程为 `Sunywr/tinynest`。用 `git rev-parse --show-toplevel` 和 `git worktree list` 核对路径。
- 开工前运行 `git status --short` 和 `git fetch origin --prune`。保留已有改动。在干净的主工作区运行 `git switch main` 和 `git merge --ff-only origin/main`。需要任务分支或独立 worktree 时，以同步后的 `main` 为起点。
- 以 [当前进度](docs/project-status.md) 确认范围。PR #1 的功能分支与 `tinynest-pr-fix` 工作树用于本次合并，后续任务从 `main` 接手。

## 强制阅读（V0.1 当前）

1. `README.md`（产品定位与当前状态）、`docs/DEVELOPMENT.md`（启动与验证）。
2. `docs/accounting-rules-v0.1.md`（账务规则；**第 8 节 10 项口径仍待用户确认**）。
3. `docs/api-v0.1.md`（API 契约；每个路由标注 `IMPLEMENTED` / `PARTIAL` / `PLANNED`）。
4. `docs/er-v0.1.md`、`IMPLEMENTATION_REPORT.md`（第 8/9 节：未完成与待确认项）。
5. `docs/reviews/`（合并前审查、修复报告、发布门禁、账务口径决策记录）。
6. 仍然有效：`docs/design-system.md`（**A · 奶油团子**，唯一主题，不得提供其他风格）、`assets/brand/README.md`（唯一批准原图 SHA-256，严禁改绘/重新生成图标）、`docs/architecture.md`（双服务器、同源 `/api`、数据库仅 B 机）。

## V0.1 当前守则

- 先检查 `git status` 与现有文件，禁止覆盖用户已有改动；不自动 `push` / `merge` / 部署 / 创建发布 tag。
- 架构固定：Vue 3 + TypeScript + Vite 前端、Go + SQLite 后端、双服务器模板、PWA；不引入新的框架或中间件栈。
- 金额一律整数分；余额由 `journal_entries` / `journal_lines` 重建，禁止直接改余额；已入账分录不可变，纠错用冲正（`reversal_of`）。
- **所有财务查询必须在 SQL 层按 `household_id` 限定**，不得先全库聚合再过滤（见 `docs/reviews/v0.1-pr-remediation-report.md` FIX-02）。
- 业务日期统一 Asia/Shanghai，使用可注入时钟 `API.businessToday()` / `businessDate()`；审计与会话时间戳仍以 UTC 存储（FIX-03）。
- 安全配置必须显式：`TINYNEST_SETUP_TOKEN`（一次性初始化令牌）、`TINYNEST_COOKIE_SECURE`（必须显式 `true`/`false`）、`TINYNEST_TRUSTED_PROXIES`（可信反代来源）、登录失败限流参数。**禁止**依赖 `r.TLS != nil` 或 `RemoteAddr == 127.0.0.1` 判断请求是否安全（Nginx 反代会让公网请求看起来像本机）。
- 修缺陷前**先补失败测试**，再修复，最后跑完整回归；不得降低验收门槛来换取合并。
- `apps/web` 只请求相对路径 `/api/v1/*`；开发代理 `5173 -> 127.0.0.1:8787`；生产由 A 机 Nginx 转到 B 机私网加密链路。
- 真实服务器地址、域名、证书、口令、数据库文件、一次性令牌不得提交到 git，也不得进入浏览器 JS。
- 未开发能力必须在界面清晰提示「未开发」，不得伪造保存成功；**不得把「按默认值实现」写成「用户已确认」**。
- 必须执行能执行的验证；环境不具备时写明 `UNVERIFIED`，不得冒领通过。
- 当前是 **V0.1 开发基线**：发布门禁未通过（iPhone 实机、双服务器 HTTPS、前端 E2E、10 项账务口径确认），**不得录入真实家庭财务数据、不得部署生产**。

## Phase 0（历史，仅存档，不再约束当前代码）

- 原守则「仅实现 Phase 0，禁止账号/授权、流水 CRUD、报表真实计算」「Go 后端仅提供健康检查」「Demo 页面金额一律虚构」「完成 Phase 0 后停止等待真实需求」等，均已被 V0.1 计划取代。
- `plan.md` 与 `.plan/*` 为历史阶段计划，保留原文；`docs/execution-report.md` 为 Phase 0 验收归档。
- `previews/*` 为 Phase 0 静态原型（虚构金额），不代表当前 `apps/web` 的真实 API 界面。

## 一句话任务（当前）

维护 V0.1 家庭账本开发基线：先补失败测试再修缺陷，保持前后端构建与账务回归全绿，如实标注未完成模块与待确认口径；发布门禁未通过前不冒充发布、不上生产。
