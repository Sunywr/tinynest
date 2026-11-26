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
| 证券/理财估值快照（2026-11-22 增补） | **已完成并发布**：2 张只读表、`GET /accounts?include=investments`、资产页投资面板与图表投资视图；生产迁移 1–5，导入 pfm 4 账户/30 持仓（总市值 ¥535,406.71），生产 API 与 390×844 真实页面验证通过。见 [发布证据](reviews/v2-investment-snapshot-deployment-evidence.md) |
| 家庭成员邀请重做（2026-11-22） | **已完成并发布**：家庭成员面板统一邀请新成员、绑定本家庭已有账号、审批加入；登录页支持邀请链接与已有账号加入；后端增加绑定已有账号能力（不新增路由）。生产 `v2-c1f3d5e`；`sunywr` 已绑定爸爸，妈妈仍待邀请。见 [发布证据](reviews/v2-family-invite-deployment-evidence.md) |
| 家庭成员角色管理（2026-11-23） | **已完成并发布**：管理员可在家庭成员面板把其他已加入账号设为管理员/成员/只读；不能修改自己的角色（403）；复用 `PATCH /admin/users/{id}/role`，不新增路由；用户管理与审计入口仅管理员可见。生产 `v2-ce68a8e`。见 [发布证据](reviews/v2-member-role-deployment-evidence.md) |
| 邮箱与安全卡片留白（2026-11-23） | **已完成并发布**：`MailSettings` 外层卡片增加内边距，桌面与 390px 窄屏标题和表单都不再贴边；E2E G08 与生产 computed padding 核对通过。生产 `v2-ce68a8e` |
| 工资导入与 sunywr 绑定（2026-11-22） | **已完成**：从 pfm 导入 140 条工资收入（¥1,851,169.91，仅收入、排除预支/纯奖金），工资账户导入后停用，资产页净资产不变；`sunywr` 已绑定 **爸爸**。见 [导入证据](reviews/v2-salary-import-evidence.md) |
| V2 文档（AGENTS / README / 开发 / 架构 / 安全 / 部署 / API / ER / 账务 / 验收） | 已完成（含还款计划与投资估值快照口径） |
| V2 测试代码 | 已就位：Go 170 个 `Test` 函数 / 37 个测试文件、Playwright 101 条用例（2026-11-26 在当前工作树清点） |
| 生产清理（P5） | **已执行**：删除 3,975 行业务数据、身份 29 行保留，见 [清理证据](reviews/v2-purge-evidence.md) |
| 生产新库切换与身份导入（P5） | **已执行**：`tinynest_v2` + 身份导入（users 摘要一致），见 [切换证据](reviews/v2-cutover-evidence.md) |
| 银行账单汇总对账 | **已完成并发布**：只读 IMAP 读取浦发/中信账单汇总，授权码 AES-GCM 密文入库；`card_statements` / `card_statement_sources` 按账期与来源摘要幂等落库，`GET /credit-statements` 返回银行金额、同周期账本消费与差额；只读比对，不写分录、不参与还款计划。生产前后端已于 2026-11-26 切 `v2-9376f47`（此前前端为 `v2-332a69b`，界面来自 `91b47c9`），登录后真实邮箱同步待用户验收。见 [发布证据](reviews/v2-statement-ui-deployment-evidence.md) |
| 账单明细补录（2026-11-26） | **代码与验证已完成，生产未部署、未实机验收（`UNVERIFIED`）**：`POST /accounts/{id}/statement-detail-preview`（只分析、零写入）与 `…/statement-detail-import`（只有判定为「消费」的行按银行**交易日期**生成一笔「账单补录」信用卡支出）为第 74–75 条路由；迁移 `014_statement_details` 逐行登记四类判定与 `row_key`+`seq`，重传同一文件 `created:0`、同一行永不双计；占位未冲正时整批 409 `placeholder_not_reversed`，服务端不自动冲正；分类固定「账单补录」由服务端决定，客户端提交 `category_id` 无效。前端资产页信用卡区新增「上传账单明细」。本地门禁全绿（Go 170 测试、E2E 101/101、隔离 MySQL 8.4.7 演练） |
| 待补明细占位（2026-11-26） | **已完成并发布**：账单已出、明细未上传时，按银行报出的应还生成一笔「待补明细」信用卡支出，`occurred_on` = 还款截止日，使还款计划的现金缺口提前占住；金额只取自服务端已落库账单，重复请求 409，明细补录后删除即冲正归零。迁移 `013_statement_placeholders`、路由 `POST /accounts/{id}/credit-statement-placeholder`（第 73 条）。生产 `v2-9376f47`（2026-11-26 15:39 CST，迁移 `[1..13]`、39 张表、账本计数不变）。见 [发布证据](reviews/v2-statement-ui-deployment-evidence.md) §10 |
| 清理工具归类修复（2026-11-25） | **已完成并在生产取证**：账单/绑卡/投资快照进清理清单，两张含密文邮箱配置表进 `PreservedConfigTables`；B 机先装 `v2-332a69b/tinynest-admin`，再用 `1a143e2` 重建的 CLI 装到 `releases/v2-1a143e2/`（SHA-256 `2e9cf99f…`，`current` 未指向它），`purge -dry-run` 实测迁移 `[1..12]`、22 张存在 / 32 个 V1 名跳过、身份 51 行 + 保留 3 行、待清理业务 95,176 行、无未分类告警。多家庭库现在会被明确拒绝并说明删除**不按家庭过滤**（生产实测 3 个家庭，退出码 1），`-confirm` 与库名不符、缺 `-yes-service-stopped` 也都实测退出 1。`purge -execute` 未执行。（2026-11-26 部署 `v2-9376f47` 后同一 CLI 复核为迁移 `[1..13]`、23 张存在 / 待清理业务仍 95,176 行，见[发布证据](reviews/v2-statement-ui-deployment-evidence.md) §10） |
| 生产定时备份修复（2026-11-25） | **已完成并证明可恢复**：备份脚本原写死 V1 库 `tinynest`，11-23 起每日转储仅 83,850 字节、不含生产账本；现导出 `tinynest_v2` 并强制校验 baseline 行 + 写 SHA-256 sidecar，实测新转储 12,859,119 字节、`transactions`/`journal_lines` 齐全。B 机运维脚本已在同日替换为仓库模板 [deploy/backend/backup-mysql.sh.example](../deploy/backend/backup-mysql.sh.example)（模板会比对 `tinynest-api` 运行时库名，不一致即退出 1；本身不删历史文件），备份 drop-in 补齐 `BACKUP_DIR` 与三个 `TINYNEST_MYSQL_*` 变量并把 `Description=` 移到 `[Unit]` 段。**隔离恢复逐项比对已执行通过**：导入 10.661s、隔离库 38 张表、`baseline=v2`、迁移 `[1..12]`，与生产基线 0 项差异（身份 51、保留配置 3、业务 95,176、借=贷=1,311,336,623、`users_digest` 相同），核对后删除隔离库。仍缺**第二份异地副本**，该项 `UNVERIFIED` |
| 生产链路补证（2026-11-25） | **已验证**：公网 `/api/v1/readyz` 实测 `check=mysql_ping_and_migrations`、`status=ready`（B 机 Unix socket + 受限账号真实生效）；`/api/v1/healthz` 与 `/api/v1/accounts` 匿名 401 走通 Nginx → SSH 隧道 → B 机 TLS 的转发链路，此前的「A↔B 只有部署模板」不再成立 |
| iPhone 实机、生产性能 | `UNVERIFIED`（公网 HTTP 冒烟、发布健康检查与 A→B 转发链路已通过） |

## 2. 本版本交付

| 范围 | 内容 |
| --- | --- |
| 模块收敛 | 保留认证/权限、家庭/成员、托管邮箱、审计与复式记账内核；退役 V1 信用账单、分期、负债、到期调度、V1 投资交易、股票自选、V1 账单导入、V1 信用账单邮件（IMAP 自动过账）、V1 项目（`projects`）、仪表盘、退款、家庭公告；新增 pfm 导入的只读证券/理财估值快照、`project_budgets` 额度统计与只读的银行账单汇总对账 |
| 信用卡还款计划 | 信用卡账户可设账单日/还款日；`GET /accounts?include=credit_debt` 按当前用户发薪日、有效流水与 FIFO 还款派生「发薪日前需准备 / 可用现金 / 缺口 / 逾期 / 未出账」；不写分录、不读取 `card_statements` |
| 证券/理财估值快照 | `investment_accounts` / `investment_holdings` 保存 pfm 导入的账户、持仓、成本、市值、股息与最近交易快照；`GET /accounts?include=investments` 只读投影；资产页与图表展示投资收益；不写分录、不参与转账 |
| 银行账单汇总对账 | 只读 IMAP 读取浦发/中信账单汇总，授权码 AES-GCM 密文入库；`card_statements` / `card_statement_sources` 按账期与来源摘要幂等落库，`GET /credit-statements` 返回银行金额、同周期账本消费与差额；只读比对，不写分录、不参与还款计划 |
| 待补明细占位（本轮新增） | 账单已出但用户不上传明细时，`POST /accounts/{id}/credit-statement-placeholder` 按银行报出的应还生成一笔真实的「待补明细」信用卡支出，`occurred_on` = 还款截止日；`statement_placeholders`（迁移 013）保存账单 ↔ 流水关联与银行自己的账期，使还款计划把它归入实际覆盖的账期而不是下一期；重复请求 409 不双计，补录明细后冲正即归零。**不恢复 V1 的分期/利息/自动过账**：金额取自银行已报汇总，不自动过账明细 |
| 账单明细补录（本轮新增） | 资产页可对已出账的信用卡上传银行导出的 XLS/XLSX：`POST /accounts/{id}/statement-detail-preview` 由服务端逐行判定 消费 / 退款 / 还款或转入 / 费用利息 / 未识别 并回显待入账笔数与金额（**不落库**）；`…/statement-detail-import` 只把「消费」且账本尚无对应流水的行按银行的**交易日期**生成一笔「账单补录」信用卡支出（借 `5010:statement_detail` / 贷 `2010:<卡>`）。`statement_details`（迁移 014）逐行登记判定、状态与 `row_key`+`seq`，幂等粒度是**行**：重传同一文件 `created:0`、`replayed:true`，同一行永远只可能产生一笔支出；分类由服务端固定，客户端无法指定；占期外、无法唯一对应、判定为非消费的行只登记不入账。**不是 V1 的 IMAP 自动过账、分期或利息**：每笔入账都来自用户显式确认的一次批量补录 |
| 后端 | V2 记账 API（分类 / 账户 / 流水 / 统计 / 预算 / 项目预算）、信用卡还款计划派生、投资估值投影、账单邮箱与同步、统一错误结构；`internal/credit`、`internal/invest` 已删除 |
| 数据层 | 两个方言各 14 个迁移文件（001–014）、单一通用迁移器、`schema_migrations` 校验和与版本连续性、`schema_meta.baseline=v2` 守卫、MySQL 可重跑 DDL、Go 常量默认分类 |
| 运维工具 | `apps/api/cmd/tinynest-admin`：`baseline-capture` / `purge` / `verify` / `identity-export` / `identity-import`，单事务清理 56 个表名（V2 中存在 24 张），保留 11 张身份表与 2 张含密文的个人配置表 |
| 前端 | `App.vue` 拆为壳 + `router.ts` + `views/` + `components/` + `composables/` + `services/`；Vue Router 4；照鲨鱼记账的信息架构（底部 5 槽导航、中间凸起记账键、键盘式记账面板、4 列分类网格、按天账单 + 搜索 + 日历、环形图 + 分类排行、资产与转账、预算），配色仍是唯一的 A · 奶油团子；资产页新增投资面板，图表页新增投资收益视图；资产页还款计划卡片可对已出账的银行账单发起「生成待补流水」与「上传账单明细」，两者都先弹服务端预览/确认（卡名、账期、金额、笔数与逐行判定），确认后成功才提示本次入账结果，服务端拒绝时原样显示原因、不伪造成功 |
| 文档 | 本文件与 `docs/plans/` 下五份 V2 计划/手册，以及 `docs/api-v2.md`、`docs/er-v2.md`、`docs/accounting-rules-v2.md` |

## 3. 度量（可在代码中逐项复核）

> 本节在 2026-11-25 于候选 `91b47c9` + 本轮工作树重新逐项清点；此前的 2026-11-22 数字已被账单邮箱、绑卡、同步与银行汇总四项能力取代。
> 2026-11-26 在 `31d4061`（`main`，工作树干净、与 `origin/main` 同步）复核：路由 72、迁移各 12 个文件、空库 SQLite 37 张具名表 + `sqlite_sequence` = 38、清理清单 16 组 / 54 个表名 / 22 张存在 / 32 个 V1 遗留名、E2E 96 条、Go 测试 148 → 149（`1a143e2` 新增 `TestPurgeHouseholdFlagDoesNotScopeDeletes`）——**该行的旧值即当时口径，不要与下表混用**。
> 下表是 2026-11-26 加入「账单明细补录」（迁移 `014`、路由 2 条、11 条 Go 测试、3 条 E2E）之后在当前工作树重新逐项实测的结果；上一行口径（含迁移 `013`）为 73 条路由、38/39/40 张表、55 个清理表名、159 个 Go 测试、98 条 E2E。

| 维度 | 实测值 | 复核位置 |
| --- | --- | --- |
| HTTP 业务路由 | **75** 条（`Register` 内按「方法 + 路径」计，含账单邮箱 5 + 绑卡 2 + 预览/同步 2 + 账单列表 1 + 待补明细占位 1 + 明细预览/入账 2 共 13 条；另有 `/api/v1/healthz`、`/api/v1/readyz` 2 条在 `internal/server`，不计入 75）；还款计划/投资快照仍为 `GET /accounts?include=credit_debt`、`?include=investments` 投影而非新路由 | `apps/api/internal/api/setup.go`、`credit_statements.go`、`credit_statement_placeholder.go`、`statement_details.go`；`TestRouteBudget` 断言 `≤ 75`，实测 `Register wired 75 routes (budget 75)` |
| 数据库表 | 空库迁移后 **39** 张（含 `schema_meta`），加迁移账本 `schema_migrations` MySQL 运行时 **40** 张，SQLite 另因 `AUTOINCREMENT` 多出 `sqlite_sequence` 为 **41** 张；两方言业务表集一致 | 2026-11-26 逐张清点：SQLite 临时库 41 项、隔离 MySQL 8.4.7 `information_schema.tables` 40 项；`internal/db/migrations/`（001 计 21、002 计 4、005 计 2、006 计 1、007 计 2、008 计 1、009–012 计 6、013 计 1、014 计 1）；明细见 `docs/er-v2.md` §1 |
| 迁移 | SQLite 与 MySQL 各 **14** 个版本文件（001–014），不再有 V1 的 21 / 14 个版本；011 复制 009/010 数据但保留旧表；`014_statement_details.sql` 的 MySQL 版在隔离实例上重跑三次退出码 `0 0 0`、行数不变 | `internal/db/migrations/`、`internal/db/mysql/`；`internal/db/migrate.go` 的 `//go:embed`；`TestMigrationsAreIdempotent` |
| 前端应用壳 | `App.vue` **117 行**（非空行 107；脚本与模板之外只剩样式） | `apps/web/src/App.vue` |
| 前端结构 | 9 个视图、37 个 `.vue` 组件 + 3 个组件辅助 TS、6 个 composable、7 个 service；非样式单文件最大 **576 行**（`services/api.ts`），其后 `MembersEditor.vue` 402、`ChartsView.vue` 343、`StatementMailboxSettings.vue` 323、`StatementDetailImport.vue` 243；`styles.css` 782 行作为全局 token 单独豁免 | `apps/web/src/` |
| 前端源码规模 | 67 个 `.vue`/`.ts`/`.css` 文件、8,997 行 | `apps/web/src/` |
| 后端 Go 代码 | 52 个非测试文件、12,742 行（目标 ≤7,000 行，**未达标**，如实记录）；本轮 `internal/api/statement_details.go` 914 行、`statement_details_test.go` 585 行 | `apps/api/`，排除 `_test.go` |
| Go 测试 | **170** 个 `Test` 函数、**37** 个 `*_test.go` 文件（含投资投影、账号绑定、迁移表、账单同步/绑定/邮箱与清理归类测试、10 条 `TestStatementPlaceholder*`，以及本轮新增的 11 条 `TestStatementDetail*`；2 条需真实 MySQL 的 drill 无 MySQL 时 skip；另有 2 个 Windows TCP 拨号 shim）。`1a143e2` 为多家庭删除范围新增 `TestPurgeHouseholdFlagDoesNotScopeDeletes`（148 → 149），占位测试使计数到 159，本轮明细补录 11 条到 170 | `apps/api/**/*_test.go`，以 `^func Test` 清点；2026-11-26 `go test ./... -count=1 -v` 全绿：顶层 167 PASS + 3 SKIP = 170，另有 24 条子测试，0 FAIL |
| Playwright E2E | **101** 条用例（`financial-closure` + `public-landing`，2 个 spec 文件），覆盖 390×844 与 1440×900；本轮 101/101 通过（新增 J01 预览不写流水、J02 确认入账只记消费且重传不双计、J03 缺卡号末四位时不伪造成功） | `apps/web/e2e/`，`npx playwright test --list` |
| MySQL 方言回归（本机隔离实例） | 既有 API 测试 + 2 条 dbadmin drill + dump/restore 对账通过；账单相关用例在隔离实例逐库单跑全绿。本轮新增：`014_statement_details.sql` 空库全链 `001→014` + 重放三次、`information_schema` 复核（`description=text`、4 条外键含复合 → `card_statements ON DELETE CASCADE`、两个唯一键）、`TestMySQLPurgeDrill` 与 `TestMySQLIdentityCutoverDrill`（各自独立空库）、11 条 `TestStatementDetail*`（每条一个空库）。**每个测试/子用例必须独占一个空库**——`POST /api/v1/setup` 单次有效，共用库会让第二条起全部失败（生产 Unix socket 未验证） | `TINYNEST_TEST_MYSQL_TCP` + `mysql_windows_test.go`；证据见 `docs/plans/credit-card-statement-import/实施进度.md` |
| 生产清理清单 | **56** 个表名，其中当前 V2 schema 中存在 **24** 张（其余 32 个为 `projects`、`credit_*`、`installment_*`、`debt_*`、`stock_*`、`schedule_*`、`statement_import_*`、`credit_mail*` 等 V1 遗留名，按 `Missing` 跳过）；保留 **11** 张身份表 + **2** 张含加密授权码的个人配置表 `statement_mailbox_configs` / `credit_statement_mailboxes` | `internal/dbadmin/tables.go`（`PurgeGroups` / `IdentityTables` / `PreservedConfigTables` / `DeleteOrder`）；2026-11-26 以 `PurgeTableSet()` + `MissingPurgeTables()` 实测 56 / 32 / 24；`TestPurgeClearsCardStatementTables` 断言 `statement_details` 排在 `card_statements` / `transactions` / `accounts` 之前 |

口径说明：以上为 2026-11-26 当前工作树（含迁移 `013`/`014`、待补明细占位与账单明细补录）的逐项实测结果。本轮已执行 `go build ./...`、`go vet ./...`、`go mod tidy -diff`、`go test ./... -count=1`（167 PASS + 3 SKIP，0 FAIL）、`npm.cmd run typecheck`、`npm.cmd run build`、Playwright **101/101**，并在本机临时 MySQL 8.4.7 隔离实例（TCP 3307，用完已停进程并删除数据目录）上跑通 `001→014` 全链与 014 重放三次、`TestMySQLPurgeDrill`、`TestMySQLIdentityCutoverDrill` 与 11 条 `TestStatementDetail*`；生产 Unix socket、`go test -race`、生产部署与真实账号点按仍未执行，见 §6。
`App.vue` 的 117 行 = 非空行 107 + 空行 10；Go 测试数量以 `^func Test` 清点，含本轮新增的 11 条明细补录测试。

## 4. 已退役

| 模块 | 处理 |
| --- | --- |
| 信用账单、分期、负债、到期调度 | 路由、视图、测试、`internal/credit` 包与相关表/清理入口全部退役；本次只在 `accounts` 增加账单日/还款日两列并做只读派生，不恢复账单表或生命周期 |
| 投资、股票自选 | V1 投资交易/股票自选退役；`internal/invest` 已删除。仅保留 pfm 导入的只读证券/理财估值快照（`investment_accounts` / `investment_holdings`），不恢复买卖、转账或估值重算 |
| 账单导入、信用账单邮件（IMAP） | 同上；`GET/POST /household/mail/announcements` 已删除 |
| 项目、仪表盘、退款 | 同上；`transaction_splits`、`refunds`、`projects` 不进入 V2 基线与新库 |
| V1 库原地升级 | 不支持；旧库按 [新库切换手册](plans/v2-cutover-runbook.md) 处理 |

## 5. 当前用户可完成的操作

登录后可用：**记账**（支出 / 收入 / 转账，4 列分类网格 + 自定义数字键盘）、**账单**（按天分组、搜索、日历、点行更正、长按或菜单删除）、**图表**（环形图 + 分类排行，支出/收入/投资、月/年切换）、**资产与转账**（现金 / 信用卡两类记账账户、证券/理财只读估值账户、净资产、行内转账、信用卡账单日/还款日配置、发薪日前还款现金计划、对已出账账单「生成待补流水」与「上传账单明细」补录消费、投资收益与持仓明细）、**预算**（按月单预算与进度）、**家庭成员**（邀请注册或已有账号加入、审批、成员档案与当前成员切换）、**导出**（前端生成 xlsx），以及邮箱验证、密码修改/找回、用户与审计日志。

能力边界如实标注：V2 不提供 V1 的完整信用账单、分期、利息、最低还款、自动扣款、投资交易、导入、退款与家庭公告；信用卡只做负债账户、账单日/还款日配置和只读还款计划；证券/理财只展示 pfm 导入的只读估值快照与当前持仓收益，不含历史成交流水、买卖、已实现收益重算。
账户余额由 `journal_lines` 派生，错账用冲正，不直接改余额；投资市值是明确标注的估值视图，不是账本余额。

## 6. UNVERIFIED 清单

- **生产 MySQL Unix socket 链路**：本机隔离 MySQL 8.4.7（TCP 3307）已跑通既有 API 测试、`TestMySQLPurgeDrill`、`TestMySQLIdentityCutoverDrill`、dump/restore 对账与投资投影定向用例；2026-11-25 公网 `/api/v1/readyz` 实测 `check=mysql_ping_and_migrations`、`status=ready`，B 机 socket 与受限账号在运行时已证实。仍 `UNVERIFIED` 的是在 B 机现场直接执行 `go test ./...`（本机产物用 Go 1.25.2 交叉编译，CI 用 1.23.6）。
- **生产投资快照**：已完成迁移 1–5、4 账户/30 持仓导入，生产 API 与 390×844 真实页面验证通过；iPhone 实机与生产性能仍未复测。
- **生产成员绑定与真实邀请闭环**：sunywr 已绑定爸爸；`admin` 与妈妈仍未绑定，尚未在真实生产完成邀请注册→邮箱验证→审批全链路；需用户确认剩余归属。
- **工资导入后的完整资产**：只导入了工资收入，未导入 pfm 支出/转账；工资账户已停用，资产页净资产不包含这批仅收入余额。
- **go test -race**：本机未找到 gcc/CGO 工具链，未执行；CI 的 Go API job 已在 `332a69b` 跑通 `-race`，结论是「CI 已执行、本机未执行」。
- **生产清理执行**：`purge -dry-run` 与 `baseline-capture` 已在 B 机 `tinynest_v2` **只读**执行（前者两次行数逐值一致，后者采的生产基线用于隔离恢复比对，见发布证据）；`purge --execute` 与 `verify` 未执行，输出与耗时未知。[清理手册](plans/v2-prod-purge-runbook.md) §4.3 已按 **56 个表名 / 24 张存在 + 11 身份 + 2 保留配置**改写（2026-11-26 加入迁移 `014` 后在代码上复核，`PurgeGroups` 16 组、`DeleteOrder` 56 项）；剩下的只是**生产执行本身**需用户逐条授权，且生产有 **3 个家庭**、`-household` 不缩小范围。
- **生产备份可恢复性**：定时备份已改为导出运行时库 `tinynest_v2` 并写 SHA-256 sidecar，摘要校验通过；2026-11-25 已在 B 机同实例的隔离库里恢复该转储并按[部署指南](deployment.md) §5 逐项比对通过（迁移版本、表数、`baseline`、11 张身份计数、2 张保留配置、`users` 摘要、账本与业务行数全部一致）。仍 `UNVERIFIED` 的是 **B 机之外的第二份异地副本**：没有第二份拷贝就无法做异地摘要校验，磁盘当前 `/dev/vda2` 40G、**96% 已用、1.7G 可用**，保留期与水位策略需用户定口径（模板刻意不删历史转储）。
- **生产新库切换**：`identity-export` / `identity-import`、表数与 `baseline=v2` 的实际输出未在生产数据上核对。
- **iPhone 实机**：主屏 PWA、安全区、返回手势未在真机验证。
- **登录后冒烟（信用卡还款计划、银行汇总、待补明细占位与账单明细补录）**：未使用真实账号口令登录，按 [部署指南](deployment.md) §9 不向 Agent 索取口令；需用户登录后在「资产」页确认汇总并排展示、执行一次「生成待补流水」并观察确认弹层 → 待还占位 → 删除后归零显示「已冲正」，用**真实银行导出文件**走一次「上传账单明细」的预览 → 确认 → 逐行判定与本次入账笔数，以及在「我的 → 账单邮箱」执行一次真实同步。
- **账单明细补录的生产部署**：迁移 `014` 与两条新路由尚未上生产（生产仍为 `v2-9376f47`、迁移 `[1..13]`、39 张运行时表）；上线后需实测 `statement_details` 建表、`[1..14]`、账本计数不变与真实文件补录，当前一律 `UNVERIFIED`。
- **生产性能**：LCP / CLS、分页与统计在真实数据量下的表现未标定。
- 2026-11-22 本轮已执行 `go test ./...`、`npm.cmd run typecheck`、`npm.cmd run build`（生成 dist）、Playwright **82/82**、本机隔离 MySQL 回归；生产现场仍未完整执行。

## 7. 下一步

1. 用户登录生产站点做一次实机验收：资产页银行汇总并排展示与差额、「我的 → 账单邮箱」同步一条真实邮件、信用卡账单日/还款日面板、对已出账账单点一次「生成待补流水」并观察冲正归零，以及用**真实银行导出文件**走一次「上传账单明细」（先看预览的四类判定与待入账笔数，确认后核对只入账消费、重传不双计、补录的「账单补录」支出随后改到真实分类）；同时确认剩余两个账号（`admin` 与妈妈）的归属。
2. 部署迁移 `014` 与两条明细补录路由到生产（待用户执行），并核对运行时 40 张表、迁移 `[1..14]`、账本与 `users` 摘要逐值不变。
3. 补齐备份可恢复性证据的**剩余一半**：隔离恢复逐项比对已于 2026-11-25 通过，还差**第二份异地副本**——把某个 `tinynest_v2-*.sql` 传到 B 机之外的位置，取回后比对 SHA-256，并确定保留期与磁盘水位策略（`/dev/vda2` 已用 96%，模板本身不删历史文件）。
4. ~~修订[生产清理手册](plans/v2-prod-purge-runbook.md)的表数口径~~ 已完成（`45b4092` 归类修复 + `332a69b` 文档口径，2026-11-26 在 `31d4061` 复核与代码逐值一致：55 个表名 / 23 张存在 + 11 身份 + 2 保留配置；同日加入迁移 `014` 后已再改为 **56 / 24**）。剩下的只有**生产执行**：由用户按 `baseline-capture` → 备份 → `purge --execute` → `verify` 逐条授权（停服、`-confirm` 与库名完全一致），并先定 3 个家庭是否全部清空。
5. 处置本轮记在[发布证据](reviews/v2-statement-ui-deployment-evidence.md) §8 的凭据暴露：按其中步骤轮换 B 机 MySQL 运行账号口令，并更新受保护凭据文件。
6. 补齐 iPhone 实机与生产性能证据后，再把对应条目标为已验证。

## 8. 相关文档

- [V2 总计划](plans/v2-bookkeeping-reset.md) · [记账模块规格](plans/v2-bookkeeping-module-spec.md) · [三端优化计划](plans/v2-platform-optimization.md)
- [生产清理手册](plans/v2-prod-purge-runbook.md) · [新库切换手册](plans/v2-cutover-runbook.md)
- [V2 API 契约](api-v2.md) · [V2 ER](er-v2.md) · [V2 账务规则](accounting-rules-v2.md)
- [开发与本地验证](DEVELOPMENT.md) · [架构](architecture.md) · [部署](deployment.md) · [安全](security.md) · [验收](acceptance.md) · [V2 基线证据](reviews/v2-baseline.md)
