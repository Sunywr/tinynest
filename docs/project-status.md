# TinyNest 当前进度

2026-11-22 · **V2 记账重置**（P0–P5）。后续任务从同步后的 `main` 开始；合并前以 `.agents/worktrees/v2-bookkeeping-reset` 工作树为准。

## 1. 状态摘要

V2 的核心目标是收敛而不是扩张：把 V1 的 140 条路由、55 张表与 3,041 行的前端单文件，收敛为手机优先的家庭记账产品。
代码、迁移、运维 CLI、前端五界面与文档已完成；生产库清理已执行并留有证据，新库切换与前后端 V2 上线已于 2026-11-22 完成。

| 项 | 状态 |
| --- | --- |
| V2 代码（后端 / 前端 / 迁移 / 运维 CLI） | 已完成，代码可清点（见 §3） |
| V2 文档（AGENTS / README / 开发 / 架构 / 安全 / 部署 / API / ER / 账务 / 验收） | 已完成（本批对齐） |
| V2 测试代码 | 已就位：Go 105 个 Test 函数、Playwright 76 条用例 |
| 生产清理（P5） | **已执行**：删除 3,975 行业务数据、身份 29 行保留，见 [清理证据](reviews/v2-purge-evidence.md) |
| 生产新库切换与身份导入（P5） | **已执行**：`tinynest_v2` + 身份导入（users 摘要一致），见 [切换证据](reviews/v2-cutover-evidence.md) |
| 生产环境、iPhone 实机、A→B 私网 TLS、性能实测 | `UNVERIFIED`（除公网 HTTP 冒烟外均未取得证据） |

## 2. 本版本交付

| 范围 | 内容 |
| --- | --- |
| 模块收敛 | 仅保留认证/权限、家庭/成员、托管邮箱、审计与复式记账内核；退役信用账单、分期、负债、到期调度、投资、股票自选、账单导入、信用账单邮件、项目、仪表盘、退款、家庭公告 |
| 后端 | V2 记账 API（分类 / 账户 / 流水 / 统计 / 预算）与统一错误结构；`internal/credit`、`internal/invest` 已删除 |
| 数据层 | 两个方言各 2 个迁移文件、单一通用迁移器、`schema_migrations` 校验和与版本连续性、`schema_meta.baseline=v2` 守卫、MySQL 可重跑 DDL、Go 常量默认分类 |
| 运维工具 | `apps/api/cmd/tinynest-admin`：`baseline-capture` / `purge` / `verify` / `identity-export` / `identity-import`，单事务清理 45 张表并保留 11 张身份表 |
| 前端 | `App.vue` 拆为壳 + `router.ts` + `views/` + `components/` + `composables/` + `services/`；Vue Router 4；照鲨鱼记账的信息架构（底部 5 槽导航、中间凸起记账键、键盘式记账面板、4 列分类网格、按天账单 + 搜索 + 日历、环形图 + 分类排行、资产与转账、预算），配色仍是唯一的 A · 奶油团子 |
| 文档 | 本文件与 `docs/plans/` 下五份 V2 计划/手册，以及 `docs/api-v2.md`、`docs/er-v2.md`、`docs/accounting-rules-v2.md` |

## 3. 度量（可在代码中逐项复核）

| 维度 | 实测值 | 复核位置 |
| --- | --- | --- |
| HTTP 业务路由 | **55** 条（`Register` 内按「方法 + 路径」计；另有 `/api/v1/healthz`、`/api/v1/readyz` 2 条在 `internal/server`，不计入 55） | `apps/api/internal/api/setup.go`；`TestRouteBudget` 断言 `≤ 55` |
| 数据库表 | **25** 张业务表 + 迁移账本 `schema_migrations`，运行时 **26** 张；SQLite 与 MySQL 表集一致 | `internal/db/migrations/001_core.sql`（21）+ `002_bookkeeping.sql`（4）；`internal/db/mysql/` 同名文件 |
| 前端应用壳 | `App.vue` **98 行**（非空行 88；脚本与模板之外只剩样式） | `apps/web/src/App.vue` |
| 前端结构 | 7 个视图、30 个组件 + 2 个组件辅助 TS、6 个 composable、4 个 service；非样式单文件最大 **276 行**（`BillsView.vue`），`styles.css` 677 行作为全局 token 单独豁免 | `apps/web/src/` |
| 前端源码规模 | 54 个 `.vue`/`.ts`/`.css` 文件、6,027 行 | `apps/web/src/` |
| 后端 Go 代码 | 35 个非测试文件、7,887 行（目标 ≤7,000 行，**未达标**，如实记录） | `apps/api/`，排除 `_test.go` |
| Go 测试 | **100** 个 `Test` 函数、**22** 个 `*_test.go` 文件（其中 21 个含测试函数；2 个为需真实 MySQL 的 drill，无 MySQL 时 skip） | `apps/api/**/*_test.go` |
| Playwright E2E | **72** 条用例（`financial-closure` 51 + `public-landing` 21），覆盖 390×844 与 1440×900 | `apps/web/e2e/` |
| 迁移 | SQLite 与 MySQL 各 2 个版本文件，不再有 V1 的 21 / 14 个版本 | `internal/db/migrate.go` 的 `//go:embed` |
| 生产清理白名单 | **45** 张表（42 张 v1 业务表 + `household_members` / `household_user_members` / `household_invites` 3 张），保留 11 张身份表 | `internal/dbadmin/tables.go` |

口径说明：以上为当前工作树的静态清点结果；本表不声称测试已在本环境执行，执行结果与生产运行态数据见 §6。
两处与计划稿数字的差异已复核：`App.vue` 计划稿写 88 行（按非空行口径），实际 98 行 = 非空行 88 + 空行 10；
Go 测试计划稿 P2 口径为 99 个/21 文件，本工作树最终清点为 100 个 `Test` 函数（21 个含测试的文件），差异来自 P2 之后新增的测试。

## 4. 已退役

| 模块 | 处理 |
| --- | --- |
| 信用账单、分期、负债、到期调度 | 路由、视图、测试、`internal/credit` 包与相关表/清理入口全部退役 |
| 投资、股票自选 | 同上；`internal/invest` 已删除 |
| 账单导入、信用账单邮件（IMAP） | 同上；`GET/POST /household/mail/announcements` 已删除 |
| 项目、仪表盘、退款 | 同上；`transaction_splits`、`refunds`、`projects` 不进入 V2 基线与新库 |
| V1 库原地升级 | 不支持；旧库按 [新库切换手册](plans/v2-cutover-runbook.md) 处理 |

## 5. 当前用户可完成的操作

登录后可用：**记账**（支出 / 收入 / 转账，4 列分类网格 + 自定义数字键盘）、**账单**（按天分组、搜索、日历、点行更正、长按或菜单删除）、**图表**（环形图 + 分类排行，支出/收入、月/年切换）、**资产与转账**（现金 / 信用卡两类账户、净资产、行内转账）、**预算**（按月单预算与进度）、**家庭成员**（成员档案与切换）、**导出**（前端生成 xlsx），以及邮箱验证、密码修改/找回、管理员邀请审批、用户与审计日志。

能力边界如实标注：V2 不提供信用卡账单、分期、利息、投资、导入、退款与家庭公告；信用卡账户只记录欠款。
账户余额由 `journal_lines` 派生，错账用冲正，不直接改余额。

## 6. UNVERIFIED 清单

- **MySQL 运行时链路**：本机未启动 MySQL 实例跑完整 API；仓库有 `scripts/mysql-ci-drill.sh` 与 `dbadmin/mysql_drill_test.go`，但未在本环境执行。
- **生产清理执行**：`purge --execute` / `verify` 未在生产或生产副本上执行，输出与耗时未知。
- **生产新库切换**：`identity-export` / `identity-import`、表数与 `baseline=v2` 的实际输出未在生产数据上核对。
- **iPhone 实机**：主屏 PWA、安全区、返回手势未在真机验证。
- **A↔B 私网 TLS**：双服务器链路只有部署模板，未实测连通与证书链。
- **生产性能**：LCP / CLS、分页与统计在真实数据量下的表现未标定。
- 本次文档对齐未重跑 Go 测试、前端构建与 Playwright；§3 的计数来自代码静态清点，执行结果以 CI 或本机复跑输出为准。

## 7. 下一步

1. 用户确认 P5 执行窗口与备份方案，按 [生产清理手册](plans/v2-prod-purge-runbook.md) 与 [新库切换手册](plans/v2-cutover-runbook.md) 执行。
2. 在具备 MySQL 与 Go 工具链的环境复跑 `go test ./...`、`go test -race ./...`、`npm run typecheck`、`npm run build`、`npm run test:e2e` 与 `scripts/mysql-ci-drill.sh`。
3. 补齐 iPhone 实机、A↔B TLS 与生产性能证据后，再把对应条目标为已验证。

## 8. 相关文档

- [V2 总计划](plans/v2-bookkeeping-reset.md) · [记账模块规格](plans/v2-bookkeeping-module-spec.md) · [三端优化计划](plans/v2-platform-optimization.md)
- [生产清理手册](plans/v2-prod-purge-runbook.md) · [新库切换手册](plans/v2-cutover-runbook.md)
- [V2 API 契约](api-v2.md) · [V2 ER](er-v2.md) · [V2 账务规则](accounting-rules-v2.md)
- [开发与本地验证](DEVELOPMENT.md) · [架构](architecture.md) · [部署](deployment.md) · [安全](security.md) · [验收](acceptance.md) · [V2 基线证据](reviews/v2-baseline.md)
