# TinyNest / 小家 · 任意 Coding Agent 规范

> **状态说明（2026-11-22）**：本文件已从 V0.1 规范升级为 **V2 记账重置** 的运维手册。
> V2 删除了信用账单、分期、负债、到期调度、投资、股票自选、账单导入、信用账单邮件、项目、仪表盘与退款模块，
> 只保留身份/权限/家庭/邮件、复式记账内核与 V2 记账 API。当前新增的「信用卡还款计划」只是 `accounts` 上
> 账单日/还款日配置 + 只读派生统计，不恢复 V1 的 `credit_statements` 账单表、分期或利息。生产数据库**不做 V1 原地升级**，
> 清理与切换由用户按 [生产清理手册](docs/plans/v2-prod-purge-runbook.md) 与
> [新库切换手册](docs/plans/v2-cutover-runbook.md) 执行；未执行前相关结论一律 `UNVERIFIED`。
> Phase 0 与 V0.1 只作为历史归档，不再约束当前代码；历史交付见
> [V0.1 实施报告](docs/reviews/v1-implementation-report.md) 与 [Phase 0 执行报告](docs/execution-report.md)。

## 项目专属技能

执行 TinyNest 开发、审查、修复或发布验收前，读取并应用 [TinyNest 高级开发工程师](.agents/skills/tinynest-senior-engineer/SKILL.md)。支持原生 Skill 的 Agent 可调用 `tinynest-senior-engineer`。其他 Agent 可直接读取同一份 Markdown，并按本文件的仓库规则执行。

## 当前阶段从 main 开始

- 项目入口为 TinyNest 仓库根目录。用 `git rev-parse --show-toplevel` 和 `git worktree list` 核对实际路径；本地路径不写死进文档。
- 开工前运行 `git status --short` 和 `git fetch origin --prune`，保留已有改动。需要任务分支或独立 worktree 时，以同步后的 `main` 为起点；V2 记账重置在 `.agents/worktrees/v2-bookkeeping-reset` 工作树完成，合并前以该工作树为准。
- 以 [当前进度](docs/project-status.md) 确认范围。不自动 `push` / `merge` / 部署 / 创建发布 tag。
- 当前基线是 V2。任何「V2 已上线/已切库/已通过生产验收」的说法都必须有用户执行证据，否则标 `UNVERIFIED`。

## 强制阅读（V2 当前）

1. `README.md`（产品定位与当前状态）、`docs/project-status.md`（进度与实测数字）、`docs/DEVELOPMENT.md`（启动与验证）。
2. `docs/plans/v2-bookkeeping-reset.md`（P0–P5 总计划）、`docs/plans/v2-bookkeeping-module-spec.md`（模块与 API 契约冻结稿）、`docs/plans/v2-platform-optimization.md`（三端优化计划）。
3. `docs/accounting-rules-v2.md`（V2 账务规则与统计口径）、`docs/api-v2.md`（55 条业务路由契约）、`docs/er-v2.md`（25 张业务表与冲正不变量）。
4. `docs/plans/v2-prod-purge-runbook.md`（生产清理）、`docs/plans/v2-cutover-runbook.md`（新库切换）；执行前逐条核对。
5. `docs/architecture.md`（双服务器边界）、`docs/security.md`（安全底线）、`docs/deployment.md`（部署与迁移）、`docs/acceptance.md`（验收清单）。
6. 仍然有效：`docs/design-system.md`（**A · 奶油团子**，唯一主题，不得提供其他风格）、`assets/brand/README.md`（唯一批准原图 SHA-256，严禁改绘/重新生成图标）、`docs/reviews/v2-baseline.md`（V2 基线与 P5 结果）。
7. 历史归档（只读参考，不代表当前状态）：`docs/reviews/v1-implementation-report.md`、`docs/execution-report.md`、`docs/reviews/` 下 2026-11 及更早的 V0.1 审查记录。

## V2 当前守则

### 流程与边界

- 先检查 `git status` 与现有文件，禁止覆盖用户已有改动；不自动 `push` / `merge` / 部署 / 创建发布 tag。
- 不引入新的框架或中间件栈：前端 Vue 3 + TypeScript + Vite，唯一新增依赖是 `vue-router` 4；后端 Go + SQLite/MySQL；部署保持双服务器模板与 PWA。不引入 UI 组件库、状态管理库、图表库或缓存/消息队列。
- 修缺陷前**先补失败测试**，再修复，最后跑完整回归；不得降低验收门槛换取合并。
- 必须执行能执行的验证；环境不具备时写明 `UNVERIFIED`，不得冒领通过。

### 账务不变量（不得破坏）

- 金额一律整数分（`*_minor`），禁止浮点参与计算；API 与数据库都不存元。
- `journal_entries` / `journal_lines` 是唯一账务事实来源；账户余额、统计与净资产一律从分录聚合重建，禁止直接改余额。
- 一条业务记录（`transactions` 行）、其全部分录行与审计日志必须在**同一个数据库事务**内写入或同时回滚。
- 已入账分录不可变（不 UPDATE / DELETE）；纠错 = 生成借贷取反的冲正分录（`reversal_of`）+ 新建正确业务记录；删除流水同样只写冲正，不物理删除。
- 冲正沿用原 `occurred_on`，保证同一统计期间净额归零；已冲正流水不可再次冲正。
- 每笔流水只保存一个 `category_id`；分类为单层，删除/停用用软删除（`categories.active=-1`），历史分录保留引用。
- **所有财务查询必须在 SQL 层按 `household_id` 限定**，不得先全库聚合再在应用层过滤；`journal_lines` 通过 `journal_entries.household_id` 关联限定。
- 业务日期统一 Asia/Shanghai，使用可注入时钟 `API.businessToday()` / `businessDate()`；审计、会话等时间戳仍以 UTC 存储。
- 统计只认科目编码：收入 `4010:%`（贷−借）、支出 `5010:%`（借−贷），分类 id 由 `SUBSTR(account_code, 6)` 解析；转账与信用卡还款不计入收支（见 `internal/api/stats.go`）。
- 信用卡还款计划是**只读派生**：从 `journal_lines` + 当前有效 `transactions` + `users.payday` 计算，不写分录、不改余额、不缓存；不新增账单表。查询同样必须按 `household_id` 限定。

### 安全配置（必须显式）

- `TINYNEST_SETUP_TOKEN`（一次性初始化令牌；未配置时 `POST /api/v1/setup` 返回 403 `setup_disabled`）。
- `TINYNEST_SETUP_DISABLED`（初始化完成后必须设为 `true` 收口；严格只接受 `true`/`false`，非法值拒绝启动）。
- `TINYNEST_COOKIE_SECURE`（必须显式 `true`/`false`，缺失或非法拒绝启动）。
- `TINYNEST_TRUSTED_PROXIES`（可信反代来源；只有这些来源的 `X-Forwarded-*` 才被采信）。
- 登录/注册失败限流参数 `TINYNEST_LOGIN_MAX_FAILURES`、`TINYNEST_LOGIN_WINDOW_SECONDS`、`TINYNEST_LOGIN_LOCKOUT_SECONDS`。
- 邮件加密密钥：`TINYNEST_MAIL_KEY_FILE` 或 systemd `CREDENTIALS_DIRECTORY/mail-key`，必须是恰好 32 字节的原始二进制文件。
- **禁止**依赖 `r.TLS != nil`、`RemoteAddr == 127.0.0.1` 或客户端可伪造的 `X-Forwarded-*` 判断请求是否安全（Nginx 反代会让公网请求看起来像本机）。
- 真实服务器地址、域名、证书、口令、数据库文件、一次性令牌不得提交到 git，也不得进入浏览器 JS。

### V2 基线与迁移

- V2 是一次**基线重置**：不支持 V1 旧库原地升级。数据库必须由 V2 二进制在空库上建立 `schema_meta.baseline=v2`；旧库按切换手册新建库 + 导入身份数据，禁止把 V2 二进制指向旧库。
- **不得绕过 baseline 守卫**：`internal/db/migrate.go` 在 `schema_meta.baseline` 缺失或不为 `v2`、出现不认识的迁移版本、校验和不一致时拒绝启动。不要通过改守卫、改 `schema_meta`、删迁移账本等方式强行启动。
- 两个方言当前各有 4 个迁移文件：`001_core.sql`（21 张表）、`002_bookkeeping.sql`（4 张表）、`003_category_style.sql`（分类颜色）与 `004_credit_cycle.sql`（信用卡账单日/还款日），嵌入在 `internal/db/migrations/`（SQLite）与 `internal/db/mysql/`（MySQL）。
- SQLite 迁移文件与账本行在同一事务执行；MySQL DDL 隐式提交，因此 **MySQL 迁移文件必须可重复执行**。新表用 `CREATE TABLE IF NOT EXISTS` 与内联索引；确需 `ALTER TABLE` 时用 `information_schema.columns` + `PREPARE/EXECUTE` 做幂等保护（003/004 的写法），不能裸写会重复失败的 ALTER。
- 默认分类来自 Go 常量 `db.DefaultCategories` + `db.SeedForHousehold()`，不要再加 SQL 种子文件或启动期 `UPDATE`。
- 数据库只在后端 B 机；V2 运行时 26 张表（25 张业务表 + 迁移账本 `schema_migrations`）。

### 生产清理 CLI（`tinynest-admin`）安全规则

- `tinynest-admin` 是**只能在服务器本地、服务停止时由人工执行**的运维工具，**绝不能通过 HTTP 暴露**，也不得被 `cmd/server` 引用。
- `purge` 只删数据行（DML），在**单事务**内按子表→父表顺序删除 45 张清理表；事务内断言「业务表全空 + 11 张身份表计数不变 + `users` 摘要不变」后才提交，任一失败整体回滚。
- 身份表白名单（11 张）不得进入删除清单：`households`、`users`、`sessions`、`pending_registrations`、`site_administrators`、`email_identities`、`email_challenges`、`mail_configuration`、`mail_preferences`、`mail_daily_usage`、`mail_recipient_daily_usage`。
- `-confirm` 必须与数据库名完全一致（SQLite 为数据库文件路径）；`-execute` 必须同时提供 `-yes-service-stopped`；MySQL 口令只能从受保护文件读取，不得出现在命令行。
- 执行前必须完成备份、SHA-256 校验、隔离恢复比对与 `baseline-capture`；执行后必须 `verify`，并把执行人、时间、前后计数写入证据文件；旧库与备份保留 ≥30 天。
- 具体命令、判据与回滚以 [生产清理手册](docs/plans/v2-prod-purge-runbook.md) 与 [新库切换手册](docs/plans/v2-cutover-runbook.md) 为准，本文不复述。

### 前端与产品诚实性

- `apps/web` 只请求同源相对路径 `/api/v1/*`；开发代理 `5173 -> 127.0.0.1:8787`；生产由 A 机 Nginx 转到 B 机私网加密链路。
- Service Worker 不缓存 `/api/` 响应；`previews/*` 仅是 Phase 0 静态原型（虚构金额），不得描述为当前应用界面。
- 未开发能力必须在界面清晰提示「未开发」，**不得伪造保存成功**；离线时明确提示「网络不可用，未保存」，V2 不做离线补记账队列。
- **不得把「按默认值实现」写成「用户已确认」**；不得把计划目标写成实测结果。

## V2 模块范围

| 状态 | 模块 |
| --- | --- |
| 保留 | 初始化与认证、会话/CSRF、家庭成员与邀请审批、管理员与审计日志、托管邮箱 SMTP 与邮箱验证/找回、站点管理员 |
| 保留 | 复式记账内核、`categories`、`accounts`（`cash` / `credit`，含可空账单日/还款日配置）、`transactions`（支出/收入/转账）、`stats`（summary / categories / daily / monthly）、`budgets`、信用卡还款计划（`GET /accounts?include=credit_debt` 只读派生） |
| 保留 | 前端五界面：账单 / 记账面板 / 图表 / 资产 / 我的；账户行内转账；前端 xlsx 导出；PWA |
| 退役 | 信用账单、分期、负债、到期调度、投资、股票自选、账单导入、信用账单邮件（IMAP）、项目、仪表盘、退款、家庭公告 |

退役模块的路由、视图、测试与表结构已删除；`internal/credit`、`internal/invest` 包不存在。不要以「历史兼容」为名恢复它们。信用卡还款计划只保留 `accounts` 两列配置与 `internal/api/credit_debt.go` 的只读派生，不构成恢复 V1 信用账单模块。

## Phase 0（历史，仅存档，不再约束当前代码）

- Phase 0 交付静态奶油团子原型、Vue 与 Go 工程骨架、PWA 资源、健康检查与双服务器部署模板；`docs/execution-report.md` 为验收归档。
- 「仅健康检查、Demo 金额全虚构、不开发记账业务」等约束已被 V0.1/V2 相继取代。
- `previews/*` 为 Phase 0 静态原型，金额虚构，不代表当前 `apps/web`。

## 一句话任务（当前）

维护 V2 家庭记账：先补失败测试再修缺陷，保持前后端构建与账务回归全绿，遵守 V2 基线/冲正/家庭隔离/显式安全配置规则，如实标注未执行的生产清理、切换与实机验收项。
