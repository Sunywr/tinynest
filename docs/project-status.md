# TinyNest 当前进度

2026-11-22 · **V2 记账重置**（P0–P5）。后续任务从同步后的 `main` 开始；合并前以 `.agents/worktrees/v2-bookkeeping-reset` 工作树为准。

## 1. 状态摘要

V2 的核心目标是收敛而不是扩张：把 V1 的 140 条路由、55 张表与 3,041 行的前端单文件，收敛为手机优先的家庭记账产品。
代码、迁移、运维 CLI、前端五界面与文档已完成；生产库清理已执行并留有证据，新库切换与前后端 V2 上线已于 2026-11-22 完成。

| 项 | 状态 |
| --- | --- |
| V2 代码（后端 / 前端 / 迁移 / 运维 CLI） | 已完成，代码可清点（见 §3） |
| 信用卡还款计划（2026-11-22 增补） | 已完成：信用卡账户账单日/还款日配置 + `GET /accounts?include=credit_debt` 只读派生；不新增路由/表，不恢复 V1 信用账单模块 |
| 信用卡还款计划生产发布（2026-11-22） | **已完成**：后端/前端 `v2-9fc30b1` 上线；生产迁移 1–4；`healthz`/`readyz` 200；静态资源哈希与前后端 release 核对通过；登录后真实账本冒烟待用户验证。见 [发布证据](reviews/v2-credit-plan-deployment-evidence.md) |
| 证券/理财估值快照（2026-11-22 增补） | 代码与本地验证完成：2 张只读表、`GET /accounts?include=investments`、资产页投资面板与图表投资视图；pfm dry-run 4 账户/30 持仓，总市值 ¥535,406.71；生产迁移与导入待用户授权 |
| V2 文档（AGENTS / README / 开发 / 架构 / 安全 / 部署 / API / ER / 账务 / 验收） | 已完成（含本次还款计划口径） |
| V2 测试代码 | 已就位：Go 117 个 Test 函数、Playwright 80 条用例 |
| 生产清理（P5） | **已执行**：删除 3,975 行业务数据、身份 29 行保留，见 [清理证据](reviews/v2-purge-evidence.md) |
| 生产新库切换与身份导入（P5） | **已执行**：`tinynest_v2` + 身份导入（users 摘要一致），见 [切换证据](reviews/v2-cutover-evidence.md) |
| iPhone 实机、A↔B 私网 TLS、生产性能 | `UNVERIFIED`（公网 HTTP 冒烟与发布健康检查已通过） |

## 2. 本版本交付

| 范围 | 内容 |
| --- | --- |
| 模块收敛 | 保留认证/权限、家庭/成员、托管邮箱、审计与复式记账内核；退役信用账单、分期、负债、到期调度、V1 投资交易、股票自选、账单导入、信用账单邮件、项目、仪表盘、退款、家庭公告；新增 pfm 导入的只读证券/理财估值快照 |
| 信用卡还款计划 | 信用卡账户可设账单日/还款日；`GET /accounts?include=credit_debt` 按当前用户发薪日、有效流水与 FIFO 还款派生「发薪日前需准备 / 可用现金 / 缺口 / 逾期 / 未出账」；不建账单表、不写分录 |
| 证券/理财估值快照 | `investment_accounts` / `investment_holdings` 保存 pfm 导入的账户、持仓、成本、市值、股息与最近交易快照；`GET /accounts?include=investments` 只读投影；资产页与图表展示投资收益；不写分录、不参与转账 |
| 后端 | V2 记账 API（分类 / 账户 / 流水 / 统计 / 预算）、信用卡还款计划派生、投资估值投影与统一错误结构；`internal/credit`、`internal/invest` 已删除 |
| 数据层 | 两个方言各 5 个迁移文件、单一通用迁移器、`schema_migrations` 校验和与版本连续性、`schema_meta.baseline=v2` 守卫、MySQL 可重跑 DDL、Go 常量默认分类 |
| 运维工具 | `apps/api/cmd/tinynest-admin`：`baseline-capture` / `purge` / `verify` / `identity-export` / `identity-import`，单事务清理 45 张表并保留 11 张身份表 |
| 前端 | `App.vue` 拆为壳 + `router.ts` + `views/` + `components/` + `composables/` + `services/`；Vue Router 4；照鲨鱼记账的信息架构（底部 5 槽导航、中间凸起记账键、键盘式记账面板、4 列分类网格、按天账单 + 搜索 + 日历、环形图 + 分类排行、资产与转账、预算），配色仍是唯一的 A · 奶油团子；资产页新增投资面板，图表页新增投资收益视图 |
| 文档 | 本文件与 `docs/plans/` 下五份 V2 计划/手册，以及 `docs/api-v2.md`、`docs/er-v2.md`、`docs/accounting-rules-v2.md` |

## 3. 度量（可在代码中逐项复核）

| 维度 | 实测值 | 复核位置 |
| --- | --- | --- |
| HTTP 业务路由 | **55** 条（`Register` 内按「方法 + 路径」计；另有 `/api/v1/healthz`、`/api/v1/readyz` 2 条在 `internal/server`，不计入 55）；本次不新增路由，还款计划/投资快照分别为 `GET /accounts?include=credit_debt`、`?include=investments` 投影 | `apps/api/internal/api/setup.go`；`TestRouteBudget` 断言 `≤ 55` |
| 数据库表 | **27** 张业务表 + 迁移账本 `schema_migrations`，运行时 **28** 张；SQLite 与 MySQL 表集一致 | `internal/db/migrations/001_core.sql`（21）+ `002_bookkeeping.sql`（4）+ `005_investments.sql`（2）；`003` / `004` 只增列/样式 |
| 前端应用壳 | `App.vue` **98 行**（非空行 88；脚本与模板之外只剩样式） | `apps/web/src/App.vue` |
| 前端结构 | 7 个视图、33 个 `.vue` 组件 + 2 个组件辅助 TS、6 个 composable、5 个 service；非样式单文件最大 **244 行**（`BillsView.vue`），`styles.css` 682 行作为全局 token 单独豁免 | `apps/web/src/` |
| 前端源码规模 | 58 个 `.vue`/`.ts`/`.css` 文件、6,049 行 | `apps/web/src/` |
| 后端 Go 代码 | 37 个非测试文件、8,044 行（目标 ≤7,000 行，**未达标**，如实记录） | `apps/api/`，排除 `_test.go` |
| Go 测试 | **117** 个 `Test` 函数、**27** 个 `*_test.go` 文件（含新增投资投影与迁移表测试；2 个需真实 MySQL 的 drill 无 MySQL 时 skip；另有 2 个 Windows TCP 拨号 shim） | `apps/api/**/*_test.go` |
| Playwright E2E | **80** 条用例（`financial-closure` 59 + `public-landing` 21），覆盖 390×844 与 1440×900；本轮 80/80 通过 | `apps/web/e2e/` |
| MySQL 方言回归（本机隔离实例） | 既有 **24 条 API 测试 + 2 条 dbadmin drill + dump/restore 对账**通过；新增投资投影用例 1 条在 MySQL 定向通过（MySQL 8.0.46，TCP 127.0.0.1:3307，临时数据目录；生产 Unix socket 未验证） | `TINYNEST_TEST_MYSQL_TCP` + `mysql_windows_test.go`；`scripts/mysql-ci-drill.sh` 对应本机 TCP 版本 |
| 迁移 | SQLite 与 MySQL 各 **5** 个版本文件（001–005），不再有 V1 的 21 / 14 个版本 | `internal/db/migrate.go` 的 `//go:embed` |
| 生产清理白名单 | **45** 张表（42 张 v1 业务表 + `household_members` / `household_user_members` / `household_invites` 3 张），保留 11 张身份表 | `internal/dbadmin/tables.go` |

口径说明：以上为 2026-11-22 当前工作树的静态清点结果。本轮已执行 `go test ./...`、`npm.cmd run typecheck`、`npm.cmd run build`、Playwright **80/80**，并在本机隔离 MySQL 8.0.46（TCP 3307）上执行既有 24 条 API 测试、2 条 dbadmin drill、dump/restore 对账与新增投资投影定向用例；生产 Unix socket、race 与生产投资导入仍未执行，见 §6。
`App.vue` 的 98 行 = 非空行 88 + 空行 10；Go 测试数量以 `^func Test` 清点，本轮新增信用卡周期/还款计划/账户配置/迁移/投资估值用例。

## 4. 已退役

| 模块 | 处理 |
| --- | --- |
| 信用账单、分期、负债、到期调度 | 路由、视图、测试、`internal/credit` 包与相关表/清理入口全部退役；本次只在 `accounts` 增加账单日/还款日两列并做只读派生，不恢复账单表或生命周期 |
| 投资、股票自选 | V1 投资交易/股票自选退役；`internal/invest` 已删除。仅保留 pfm 导入的只读证券/理财估值快照（`investment_accounts` / `investment_holdings`），不恢复买卖、转账或估值重算 |
| 账单导入、信用账单邮件（IMAP） | 同上；`GET/POST /household/mail/announcements` 已删除 |
| 项目、仪表盘、退款 | 同上；`transaction_splits`、`refunds`、`projects` 不进入 V2 基线与新库 |
| V1 库原地升级 | 不支持；旧库按 [新库切换手册](plans/v2-cutover-runbook.md) 处理 |

## 5. 当前用户可完成的操作

登录后可用：**记账**（支出 / 收入 / 转账，4 列分类网格 + 自定义数字键盘）、**账单**（按天分组、搜索、日历、点行更正、长按或菜单删除）、**图表**（环形图 + 分类排行，支出/收入/投资、月/年切换）、**资产与转账**（现金 / 信用卡两类记账账户、证券/理财只读估值账户、净资产、行内转账、信用卡账单日/还款日配置、发薪日前还款现金计划、投资收益与持仓明细）、**预算**（按月单预算与进度）、**家庭成员**（成员档案与切换）、**导出**（前端生成 xlsx），以及邮箱验证、密码修改/找回、管理员邀请审批、用户与审计日志。

能力边界如实标注：V2 不提供 V1 的完整信用账单、分期、利息、最低还款、自动扣款、投资交易、导入、退款与家庭公告；信用卡只做负债账户、账单日/还款日配置和只读还款计划；证券/理财只展示 pfm 导入的只读估值快照与当前持仓收益，不含历史成交流水、买卖、已实现收益重算。
账户余额由 `journal_lines` 派生，错账用冲正，不直接改余额；投资市值是明确标注的估值视图，不是账本余额。

## 6. UNVERIFIED 清单

- **生产 MySQL Unix socket 链路**：本机隔离 MySQL 8.0.46（TCP 3307）已跑通既有 24 条 API 测试、`TestMySQLPurgeDrill`、`TestMySQLIdentityCutoverDrill`、dump/restore 对账与新增投资投影定向用例；但生产 B 机 Unix socket、受限账号、`readyz` 的 `mysql_ping_and_migrations` 实际输出仍未验证。
- **生产投资快照迁移与导入**：`005_investments.sql` 尚未部署到生产；pfm dry-run 已核对 4 账户/30 持仓，但真实写入、备份与生产 API 验证待用户授权。
- **go test -race**：本机未找到 gcc/CGO 工具链，未执行。
- **生产清理执行**：`purge --execute` / `verify` 未在生产或生产副本上执行，输出与耗时未知。
- **生产新库切换**：`identity-export` / `identity-import`、表数与 `baseline=v2` 的实际输出未在生产数据上核对。
- **iPhone 实机**：主屏 PWA、安全区、返回手势未在真机验证。
- **信用卡还款计划登录后冒烟**：未使用真实账号口令登录；需用户登录后在「资产」页新建/配置信用卡并确认面板与编辑流程。
- **A↔B 私网 TLS**：双服务器链路只有部署模板，未实测连通与证书链。
- **生产性能**：LCP / CLS、分页与统计在真实数据量下的表现未标定。
- 2026-11-22 本轮已执行 `go test ./...`、`npm.cmd run typecheck`、`npm.cmd run build`（生成 dist）、Playwright **80/80**、本机隔离 MySQL 回归；race 与生产现场仍未执行。

## 7. 下一步

1. 用户确认 P5 执行窗口与备份方案，按 [生产清理手册](plans/v2-prod-purge-runbook.md) 与 [新库切换手册](plans/v2-cutover-runbook.md) 执行。
2. 在 B 机 Unix socket + 受限账号环境复跑 `go test ./...`、`go test -race ./...`、`npm run typecheck`、`npm run build`、`npm run test:e2e` 与 `scripts/mysql-ci-drill.sh`；本机 TCP 隔离实例已通过，生产链路仍待验证。
3. 补齐 iPhone 实机、A↔B TLS 与生产性能证据后，再把对应条目标为已验证。

## 8. 相关文档

- [V2 总计划](plans/v2-bookkeeping-reset.md) · [记账模块规格](plans/v2-bookkeeping-module-spec.md) · [三端优化计划](plans/v2-platform-optimization.md)
- [生产清理手册](plans/v2-prod-purge-runbook.md) · [新库切换手册](plans/v2-cutover-runbook.md)
- [V2 API 契约](api-v2.md) · [V2 ER](er-v2.md) · [V2 账务规则](accounting-rules-v2.md)
- [开发与本地验证](DEVELOPMENT.md) · [架构](architecture.md) · [部署](deployment.md) · [安全](security.md) · [验收](acceptance.md) · [V2 基线证据](reviews/v2-baseline.md)
