# TinyNest / 小家 · 任意 Coding Agent 规范

> **状态说明（2026-11-25 更新）**：本文件已从 V0.1 规范升级为 **V2 记账重置** 的运维手册。
> V2 删除了分期、负债、到期调度、投资交易、股票自选、账单导入、仪表盘与退款模块，
> 只保留身份/权限/家庭/邮件、复式记账内核、信用卡还款计划与 **只读的证券/理财估值快照**。该快照从 pfm 导入，
> 使用独立的 `investment_accounts` / `investment_holdings` 表，不写分录、不参与转账，也不恢复 V1 投资交易/股票自选模块。
> 当前新增的「信用卡还款计划」只是 `accounts` 上账单日/还款日配置 + 只读派生统计，不恢复 V1 的 `credit_statements` 账单表、分期或利息。
> V2 之后按需重新引入的增量，均与 V1 同名模块**不同**，不得据此认为模块已恢复：
> 迁移 007 的 `project_budgets` 只做「预算 − 关联流水」额度统计（无 V1 `projects` 表与项目路由）；
> 迁移 009–011 的账单邮箱只读 IMAP 并只存 AES-GCM 密文授权码；迁移 012 的 `card_statements` / `card_statement_sources`
> 只保存**银行报的**账期金额用于只读比对，不写分录、不参与还款计划，命名刻意避开 V1 的 `credit_statements` / `credit_statement_items`。
> 迁移 013 的 `statement_placeholders` 只是「银行账单 ↔ 待补明细占位流水」的一对一关联 + 银行自己报的账期日期：占位本身是一条**真实的信用卡支出分录**（分类「待补明细」、`occurred_on`=还款截止日），由用户点按钮确认后生成，不是 V1 的自动过账/分期/利息，明细补录后冲正即归零。
> 迁移 014 的 `statement_details` 是「银行导出的账单明细文件 ↔ 逐行判定结果」的登记表：每行明细按 `row_key`（日期+金额+卡尾+币种+摘要的哈希 + 出现序号）存一条分类与状态，只有识别为**消费**且账本没有对应流水的行，才在用户点「确认入账」后生成一笔分类「账单补录」的信用卡支出；退款、还款或转入、费用利息与未识别行只登记不记账，明细的语义判定全部在服务端完成，不做 V1 的 IMAP 自动过账、分期或利息计提。生产数据库**不做 V1 原地升级**，
> 清理与切换由用户按 [生产清理手册](docs/plans/v2-prod-purge-runbook.md) 与
> [新库切换手册](docs/plans/v2-cutover-runbook.md) 执行；未执行前相关结论一律 `UNVERIFIED`。
> Phase 0 与 V0.1 只作为历史归档，不再约束当前代码；历史交付见
> [V0.1 实施报告](docs/reviews/v1-implementation-report.md) 与 [Phase 0 执行报告](docs/execution-report.md)。

## 项目专属技能

当用户说「继续实施未完成的计划」，先读[信用卡账单导入与对账计划](docs/plans/credit-card-statement-import/plan.md)和[实施进度](docs/plans/credit-card-statement-import/实施进度.md)。按当前 checkout 核实记录，从首个可执行的未完成项继续，并更新实施进度。本计划是 V2 后续需求，当前能力以项目进度和实际代码为准。

执行 TinyNest 开发、审查、修复或发布验收前，读取并应用 [TinyNest 高级开发工程师](.agents/skills/tinynest-senior-engineer/SKILL.md)。支持原生 Skill 的 Agent 可调用 `tinynest-senior-engineer`。其他 Agent 可直接读取同一份 Markdown，并按本文件的仓库规则执行。

## 当前阶段从 main 开始

- 项目入口为 TinyNest 仓库根目录。用 `git rev-parse --show-toplevel` 和 `git worktree list` 核对实际路径；本地路径不写死进文档。
- 开工前运行 `git status --short` 和 `git fetch origin --prune`，保留已有改动。需要任务分支或独立 worktree 时，以同步后的 `main` 为起点；V2 记账重置在 `.agents/worktrees/v2-bookkeeping-reset` 工作树完成，合并前以该工作树为准。
- 以 [当前进度](docs/project-status.md) 确认范围。不自动 `push` / `merge` / 部署 / 创建发布 tag。
- 当前基线是 V2。任何「V2 已上线/已切库/已通过生产验收」的说法都必须有用户执行证据，否则标 `UNVERIFIED`。

## 强制阅读（V2 当前）

1. `README.md`（产品定位与当前状态）、`docs/project-status.md`（进度与实测数字）、`docs/DEVELOPMENT.md`（启动与验证）。
2. `docs/plans/v2-bookkeeping-reset.md`（P0–P5 总计划）、`docs/plans/v2-bookkeeping-module-spec.md`（模块与 API 契约冻结稿）、`docs/plans/v2-platform-optimization.md`（三端优化计划）。
3. `docs/accounting-rules-v2.md`（V2 账务规则与统计口径）、`docs/api-v2.md`（**75 条**业务路由契约）、`docs/er-v2.md`（**39 张**表与冲正不变量）。
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
  **例外（2026-11-22 起）**：`investment_accounts` / `investment_holdings` 是从 pfm 导入的只读估值快照，不写分录；资产页的“投资市值”及含投资的净资产是明确标注的估值视图，不是账本余额。投资账户不参与转账、不生成流水，不能据此反推账务事实。
- 一条业务记录（`transactions` 行）、其全部分录行与审计日志必须在**同一个数据库事务**内写入或同时回滚。
- 已入账分录不可变（不 UPDATE / DELETE）；纠错 = 生成借贷取反的冲正分录（`reversal_of`）+ 新建正确业务记录；删除流水同样只写冲正，不物理删除。
- 冲正沿用原 `occurred_on`，保证同一统计期间净额归零；已冲正流水不可再次冲正。
- 每笔流水只保存一个 `category_id`；分类为单层，删除/停用用软删除（`categories.active=-1`），历史分录保留引用。
- **所有财务查询必须在 SQL 层按 `household_id` 限定**，不得先全库聚合再在应用层过滤；`journal_lines` 通过 `journal_entries.household_id` 关联限定。
- 业务日期统一 Asia/Shanghai，使用可注入时钟 `API.businessToday()` / `businessDate()`；审计、会话等时间戳仍以 UTC 存储。
- 统计只认科目编码：收入 `4010:%`（贷−借）、支出 `5010:%`（借−贷），分类 id 由 `SUBSTR(account_code, 6)` 解析；转账与信用卡还款不计入收支（见 `internal/api/stats.go`）。
- 信用卡还款计划是**只读派生**：从 `journal_lines` + 当前有效 `transactions` + `users.payday` 计算，不写分录、不改余额、不缓存，也不读取 `card_statements`。查询同样必须按 `household_id` 限定。
  **唯一例外（迁移 013 起）**：关联到 `statement_placeholders` 的「待补明细」占位流水不受「截至今天」过滤约束，且按银行自己的 `period_start_on` / `period_end_on` / `due_on` 归期而不是卡的周期网格——占位的意义就是让未来的还款截止日提前占住现金。除日期之外仍不读 `card_statements` 的任何金额。
- 账单明细补录（迁移 014）只把**识别为「消费」且账本无对应流水**的行写成信用卡支出：`occurred_on` = 银行的**交易日期**（不是记账日期），分类「账单补录」，金额取银行原文。退款、还款或转入、费用利息与未识别行只登记状态与原因，不产生分录；明细的行身份是 `row_key`（日期+金额+卡尾+币种+摘要的哈希）+ 同键出现序号，`UNIQUE(household_id, account_id, row_key, seq)` 保证重复上传不双计。占位与明细同时覆盖同一账期时，确认入账以 409 `placeholder_not_reversed` 拒绝，**不自动冲掉整笔占位**——是否冲正由用户自己决定。

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
- 两个方言当前各有 **14 个迁移文件**（`internal/db/migrations/` SQLite 与 `internal/db/mysql/` MySQL 同名）：`001_core.sql`（21 张表）、`002_bookkeeping.sql`（4 张）、`003_category_style.sql`（分类颜色）、`004_credit_cycle.sql`（信用卡账单日/还款日）、`005_investments.sql`（证券/理财估值快照 2 张）、`006_auth_login_locks.sql`（登录风控）、`007_project_budgets.sql`（项目预算 2 张）、`008_transaction_source_metadata.sql`、`009_credit_statement_mailboxes.sql`、`010_credit_statement_bindings.sql`、`011_statement_mailbox_providers.sql`（按服务商隔离，含从 009/010 复制）、`012_card_statements.sql`（银行账单汇总 2 张）、`013_statement_placeholders.sql`（待补明细占位关联 1 张）、`014_statement_details.sql`（账单明细逐行登记 1 张）。
- 数据库只在后端 B 机；V2 空库迁移后共 **39 张表**（含基线标记 `schema_meta`），加迁移账本 `schema_migrations` 后 MySQL 运行时 **40 张**，SQLite 另因 `AUTOINCREMENT` 多出 `sqlite_sequence` 为 **41 张**（2026-11-26 在两方言隔离库上逐张清点）。009/010 的旧表 011 只复制不删除，因此新旧两组并存，新代码只读写 011 的两张表。
- SQLite 迁移文件与账本行在同一事务执行；MySQL DDL 隐式提交，因此 **MySQL 迁移文件必须可重复执行**。新表用 `CREATE TABLE IF NOT EXISTS` 与内联索引；确需 `ALTER TABLE` 时用 `information_schema.columns` + `PREPARE/EXECUTE` 做幂等保护（003/004 的写法），不能裸写会重复失败的 ALTER。
- 默认分类来自 Go 常量 `db.DefaultCategories` + `db.SeedForHousehold()`，不要再加 SQL 种子文件或启动期 `UPDATE`。「待补明细」不在默认分类里：由 `internal/api/credit_statement_placeholder.go` 的 `placeholderCategory()` 在写事务内按需 ensure（缺失则插入，被软删除则重新激活），不得改成种子文件或启动期 `UPDATE`。「账单补录」同理，由 `internal/api/statement_details.go` 的 `statementDetailCategory()` 在确认入账的写事务内按需 ensure。

### 生产清理 CLI（`tinynest-admin`）安全规则

- `tinynest-admin` 是**只能在服务器本地、服务停止时由人工执行**的运维工具，**绝不能通过 HTTP 暴露**，也不得被 `cmd/server` 引用。
- `purge` 只删数据行（DML），在**单事务**内按子表→父表顺序删除清理清单：`PurgeTableSet()` 与 `DeleteOrder()` 均为 **56 个表名**，其中在当前 V2 schema 中存在的只有 **24 张**，其余 32 个是 V1 遗留名（`projects`、`credit_*`、`installment_*`、`debt_*`、`investment_*`（快照除外）、`stock_*`、`schedule_*`、`statement_import_*`、`credit_mail*`），遇到按 `Missing` 跳过；事务内断言「业务表全空 + 11 张身份表 + 2 张保留的个人配置表计数不变 + `users` 摘要不变」后才提交，任一失败整体回滚。
- 账单表（`card_statements` / `card_statement_sources`）、占位关联（`statement_placeholders`，删除顺序排在 `card_statements` 与 `transactions` 之前）、明细登记（`statement_details`，同样排在 `card_statements` 与 `transactions` 之前，因为它同时引用账单、卡与流水）、绑卡关系（`statement_mailbox_card_links` / `credit_statement_bindings`）与投资估值快照按**业务数据清空**；含加密授权码的 `statement_mailbox_configs` / `credit_statement_mailboxes` 属个人凭据，列在 `PreservedConfigTables` 随身份保留、但不进入身份导出/导入。`auth_login_locks` 是安全状态，同样不清空。
- 任何新迁移加的表必须显式归类，否则 `Snapshot.Unclassified` 会在清点后列出它并让 `verify` 失败——「业务表全空」这句断言只覆盖已分类的表，正是这个守卫在防止漏清。
- 身份表白名单（11 张）不得进入删除清单：`households`、`users`、`sessions`、`pending_registrations`、`site_administrators`、`email_identities`、`email_challenges`、`mail_configuration`、`mail_preferences`、`mail_daily_usage`、`mail_recipient_daily_usage`。
- `-confirm` 必须与数据库名完全一致（SQLite 为数据库文件路径）；`-execute` 必须同时提供 `-yes-service-stopped`；MySQL 口令只能从受保护文件读取，不得出现在命令行。
- `-household=<id>` **不缩小删除范围**：`purge` 的每条 `DELETE FROM <表>` 都不带家庭条件，该开关只在检测到多家庭时确认继续并选择 `-reseed-categories` 的目标。库里有多个家庭时执行会清空全部家庭的业务行；2026-11-25 只读核对生产 `tinynest_v2` 有 **3 个家庭**，与「单家庭」设计前提不符，执行前必须由用户逐条授权并记入证据。
- 日常备份的库名必须等于服务运行时的 `TINYNEST_MYSQL_DATABASE`，并断言转储含 `schema_meta` 的 baseline 行；模板见 `deploy/backend/backup-mysql.sh.example`。库名写死成旧库时 `mysqldump` 仍退出 0，只表现为转储小一个数量级。
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
| 保留 | 只读证券/理财估值快照：`investment_accounts` / `investment_holdings`，经 `GET /accounts?include=investments` 投影；不写分录、不参与转账 |
| 保留 | 项目预算 `project-budgets`（`project_budgets` + `transaction_budget_links`，只做额度统计）、流水来源元数据 `transaction_source_metadata`、持久化登录风控 `auth_login_locks` |
| 保留 | 信用卡账单邮箱与只读对账（`/api/v1/credit-statement-mailbox*`、`/api/v1/accounts/{id}/credit-statement-{preview,sync,binding}`、`GET /api/v1/credit-statements`）：只读 IMAP 取银行发的账单汇总，密文存授权码，落 `card_statements` / `card_statement_sources` 只做只读比对，不写分录、不参与还款计划 |
| 保留 | 待补明细占位 `POST /accounts/{id}/credit-statement-placeholder`（`statement_placeholders`，迁移 013）：账单已出、明细未上传时，按银行报出的应还生成一笔「待补明细」信用卡支出（`occurred_on`=还款截止日），关联表只存账单 ↔ 流水与银行账期日期；金额由服务端取自已落库账单、不接受客户端提交，重复请求 409，明细补录后冲正归零 |
| 保留 | 账单明细补录 `POST /accounts/{id}/statement-detail-preview` + `POST /accounts/{id}/statement-detail-import`（`statement_details`，迁移 014）：读取银行导出的 XLS/XLSX 明细，逐行按 `row_key`（日期+金额+卡尾+币种+摘要+出现序号）登记分类与状态；只有识别为「消费」且账本无对应流水的行在用户点「确认入账」后生成一笔「账单补录」信用卡支出（`occurred_on`=银行交易日期），退款/还款或转入/费用利息/未识别只登记不记账，重复上传同一行不双计；账期还有未冲正的占位时拒绝入账（409 `placeholder_not_reversed`） |
| 保留 | 前端五界面：账单 / 记账面板 / 图表 / 资产 / 我的；账户行内转账；前端 xlsx 导出；PWA |
| 退役 | V1 信用账单（`credit_statements` / `credit_statement_items` 及其还款、调整、计提）、分期、负债、到期调度、**V1 投资交易**、股票自选、V1 账单导入（`statement_import_*`）、V1 信用账单邮件（`credit_mailbox*` / IMAP 自动过账）、V1 项目（`projects` 表）、仪表盘、退款、家庭公告 |

退役模块的路由、视图与表结构已删除；`internal/credit`、`internal/invest` 包不存在。不要以「历史兼容」为名恢复它们。信用卡还款计划只保留 `accounts` 两列配置与 `internal/api/credit_debt.go` 的只读派生，不构成恢复 V1 信用账单模块。这些例外都只是数据或人工确认的记账：只读投资估值快照是 pfm 导入的展示数据，不提供买卖、转账、估值重算或已实现收益账务；`card_statements` 是银行报的账期金额，只用于与账本派生值比对差异，本身不产生分录——用户显式点击「生成待补流水」才由 `statement_placeholders` 关联一笔占位支出；`statement_details` 是明细文件的逐行登记表，入账动作由用户点「确认入账」触发、金额与日期一律取银行原文，解析、分类、匹配、幂等全在服务端完成，浏览器不提交金额。三者都不是 V1 的 IMAP 自动过账、分期或利息计提，也不等于恢复 `statement_import_*`。009/010 留下的 `credit_statement_mailboxes` / `credit_statement_bindings` 是 011 拆分前的旧结构，011 复制数据但不删表，新代码不再读写。

## Phase 0（历史，仅存档，不再约束当前代码）

- Phase 0 交付静态奶油团子原型、Vue 与 Go 工程骨架、PWA 资源、健康检查与双服务器部署模板；`docs/execution-report.md` 为验收归档。
- 「仅健康检查、Demo 金额全虚构、不开发记账业务」等约束已被 V0.1/V2 相继取代。
- `previews/*` 为 Phase 0 静态原型，金额虚构，不代表当前 `apps/web`。

## 一句话任务（当前）

维护 V2 家庭记账：先补失败测试再修缺陷，保持前后端构建与账务回归全绿，遵守 V2 基线/冲正/家庭隔离/显式安全配置规则，如实标注未执行的生产清理、切换与实机验收项。
