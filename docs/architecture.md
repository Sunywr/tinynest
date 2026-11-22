# TinyNest 架构（V2）

> 本文件描述**当前**架构。双服务器拓扑与数据边界沿用 Phase 0/V0.1；V2 把后端收敛为记账 API + 身份/家庭/邮件，并改为单一迁移器与 V2 基线。
> 历史交付见 [V0.1 实施报告](reviews/v1-implementation-report.md) 与 [Phase 0 执行报告](execution-report.md)。

## 1. 拓扑

```text
PC 浏览器 / iPhone Safari 或主屏 PWA
        |
        | HTTPS 443（单一公开域名）
        v
[服务器 A · 前端] Nginx / SPA dist / PWA 静态资源
        |
        | 同源请求 /api/v1/* -> 私网 VPN/overlay + TLS（服务端证书验证）
        v
[服务器 B · 后端] 私网 Nginx TLS:8443 -> 127.0.0.1:8787 Go API
                                               |
                                               + SQLite 文件
                                                 或本机 Unix socket 的 MySQL 8
                                               + 加密备份目录（异地保留）
```

- 所有数据库与密钥**只在 B**；A 不存库、不存密钥，只做静态分发与 API 反向代理。
- 浏览器不直连 B、不直连数据库；没有开放 CORS，只有 A 同源 `/api` 代理。
- B 的 Go 服务仅监听 `127.0.0.1:8787`；B 私网 Nginx 8443 只允许 A 的私网地址，禁止公网。
- A→B 不允许裸 HTTP 跨公网；优先加密隧道 + 后端 TLS + 内部 CA 验证。没有可用内网时停止部署。
- 域名、私网地址、证书与账号策略由部署环境决定；仓库模板只含占位符。

## 2. 组件与代码边界

| 组件 | 位置 | 职责 |
| --- | --- | --- |
| Web 客户端 | `apps/web/` | Vue 3 + TypeScript + Vite + vue-router 4；PWA；只请求相对路径 `/api/v1/*` |
| API 服务 | `apps/api/cmd/server` | 组合根：打开数据库 → 执行迁移 → 注入 readiness → 注册路由 → 启动邮件 worker |
| 业务包 | `internal/api` | 55 条业务路由、认证/CSRF/限流中间件、记账、统计、预算、信用卡还款计划派生、身份/家庭/邮件 handler |
| 账务内核 | `internal/ledger`、`internal/money` | 借贷平衡校验、科目编码、业务分录、冲正、整数分金额 |
| 数据层 | `internal/db` | SQLite/MySQL 连接、单一迁移器、分录持久化、默认分类 |
| 运维工具 | `cmd/tinynest-admin`、`internal/dbadmin` | 生产清理、校验、身份导出导入；与本文件描述的数据访问共用 `internal/platform/config` |
| 邮件运维工具 | `cmd/mail-admin` | 生成邮件密钥、授予/撤销站点管理员、受控改密（仅本地执行） |
| 健康检查 | `internal/server` | `/api/v1/healthz`、`/api/v1/readyz`，以及 HTTP 超时与中间件骨架 |

`internal/server` 的 `Options.Register` 由组合根注入业务路由，因此 `healthz`/`readyz` 不计入 55 条业务路由。

## 3. 前端

- `App.vue` 是约 98 行的应用壳：会话引导、公开/私有路由切换、底部导航与记账面板挂载。
- `router.ts` 使用 vue-router 4 的 history 模式；公开页 `/`、`/login`，私有页 `/bills`、`/charts`、`/assets`、`/me`、`/me/settings`，未知路径重定向到 `/`。
- `views/` 五个业务界面 + 设置/公开页；`components/` 拆分日组列表、记账面板、数字键盘、分类网格、环形图、预算、管理员弹层等；`composables/` 管理会话、记账草稿、偏好与断点；`services/` 只负责请求、格式化与前端 xlsx 导出。
- 视觉上保持唯一主题 A · 奶油团子，信息架构照鲨鱼记账；390×844 与 1440×900 是必测视口。
- Service Worker 只缓存公开应用壳，显式跳过 `/api/`；离线时明确提示，不伪造保存。

## 4. 后端与数据层

- Go 模块化单体：`net/http` + `modernc.org/sqlite`（纯 Go、无 CGO）或 `go-sql-driver/mysql`；SQLite 启用 WAL、`foreign_keys=ON`、`busy_timeout=5000`、单连接单写。
- SQLite 与 MySQL 使用同一套迁移器（`internal/db/migrate.go`）与同一套业务代码；方言差异只在 SQL 语法分支（upsert、`FOR UPDATE`、DDL）。
- 两个方言当前各有 4 个迁移文件：`001_core.sql`（21 张表：身份/家庭/邮件/审计/复式内核/`schema_meta`）、`002_bookkeeping.sql`（4 张表：分类/账户/流水/预算）、`003_category_style.sql`（分类颜色）、`004_credit_cycle.sql`（信用卡账单日/还款日配置列）。运行时另有代码创建的 `schema_migrations`，共 26 张。
- 迁移器按文件名版本升序应用，记录 SHA-256 校验和并校验连续性；SQLite 整文件单事务，MySQL 逐条执行且文件必须可重跑。
- **基线守卫**：`schema_meta.baseline` 缺失或不是 `v2`、出现不认识的版本、校验和不一致时启动直接失败，中文错误指向 [新库切换手册](plans/v2-cutover-runbook.md)。V2 不支持 V1 旧库原地升级。
- 默认分类由 Go 常量 `db.DefaultCategories` + `db.SeedForHousehold()` 幂等写入；不再有 SQL 种子文件与启动期 `UPDATE`。
- `readyz` 为真实检查：SQLite 执行 `PRAGMA quick_check`，MySQL 执行 ping，两者都校验迁移版本完整。

## 5. 账务不变量

- 金额一律整数分；`journal_entries` / `journal_lines` 是唯一事实来源，余额与统计从分录重建，页面不存余额。
- 一条流水、其分录与审计日志在同一事务提交；已入账分录不可变，纠错用冲正（`reversal_of`），删除流水同样是冲正。
- 所有财务查询在 SQL 层按 `household_id` 限定；`journal_lines` 通过 `journal_entries` 继承家庭范围。
- 统计只从科目编码派生：收入 `4010:%`（贷−借）、支出 `5010:%`（借−贷）；转账与信用卡还款不计入收支。
- 信用卡还款计划是只读派生：`GET /accounts?include=credit_debt` 从 `journal_lines` + 当前有效 `transactions` + `users.payday` 计算，不写分录、不建账单表、不缓存；账户上的 `statement_day` / `due_day` 只是配置。
- 业务日期 Asia/Shanghai（可注入时钟），审计/会话时间戳 UTC。

## 6. 运维工具边界

- `tinynest-admin` 与 `cmd/server` 是两个独立二进制：`dbadmin` 包**不被 API 进程导入**，因此清理/导入能力不可能通过 HTTP 触发。
- `tinynest-admin` 与 API 共用 `internal/platform/config` 的环境变量与口令文件规则，保证指向同一数据库；MySQL 口令只能从受保护文件读取。
- 生产清理只做 DML 且单事务提交；新库切换用 `identity-export` / `identity-import` 迁移 11 张身份表。执行由用户按 [生产清理手册](plans/v2-prod-purge-runbook.md) 与 [新库切换手册](plans/v2-cutover-runbook.md) 完成。
- `mail-admin` 同样只在服务器本地执行，不提供 HTTP 入口。

## 7. 模块范围

| 状态 | 内容 |
| --- | --- |
| 保留 | 初始化、认证/会话/CSRF、家庭与成员、邀请与审批、管理员与审计、托管邮箱（SMTP + 邮箱验证/找回 + 发信队列）、站点管理员 |
| 保留 | 复式内核、分类、账户（`cash`/`credit`，含账单日/还款日配置）、流水（支出/收入/转账）、统计、预算、信用卡还款计划（只读派生）、前端五界面、PWA、前端 xlsx 导出 |
| 退役 | 信用账单、分期、负债、到期调度、投资、股票自选、账单导入、信用账单邮件（IMAP）、项目、仪表盘、退款、家庭公告 |

## 8. 尚未验证

- 生产 MySQL Unix socket 运行时链路（本机隔离 TCP MySQL 8.0.46 已验证迁移、核心 API、purge/cutover 与 dump/restore；生产 B 机 Unix socket、受限账号与 `readyz` 仍未验证）。
- 生产库清理与切库执行：以 [当前进度](project-status.md) 与 `docs/reviews/` 证据文件为准；本环境未复验。
- 双服务器 A↔B 私网 TLS 实际连通、证书链与防火墙。
- iPhone 实机 PWA（安装、安全区、返回手势）与生产性能（LCP/CLS、分页在真实数据量下的表现）。
