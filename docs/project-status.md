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
| V2 测试代码 | 已就位：Go 126 个 Test 函数、Playwright 85 条用例 |
| 生产清理（P5） | **已执行**：删除 3,975 行业务数据、身份 29 行保留，见 [清理证据](reviews/v2-purge-evidence.md) |
| 生产新库切换与身份导入（P5） | **已执行**：`tinynest_v2` + 身份导入（users 摘要一致），见 [切换证据](reviews/v2-cutover-evidence.md) |
| 银行账单汇总对账 | **已完成并发布**：只读 IMAP 读取浦发/中信账单汇总，授权码 AES-GCM 密文入库；`card_statements` / `card_statement_sources` 按账期与来源摘要幂等落库，`GET /credit-statements` 返回银行金额、同周期账本消费与差额；只读比对，不写分录、不参与还款计划。生产前端已切 `v2-332a69b`（界面来自 `91b47c9`），登录后真实邮箱同步待用户验收。见 [发布证据](reviews/v2-statement-ui-deployment-evidence.md) |
| 清理工具归类修复（2026-11-25） | **已完成并在生产取证**：账单/绑卡/投资快照进清理清单，两张含密文邮箱配置表进 `PreservedConfigTables`；B 机先装 `v2-332a69b/tinynest-admin`，再用 `1a143e2` 重建的 CLI 装到 `releases/v2-1a143e2/`（SHA-256 `2e9cf99f…`，`current` 未指向它），`purge -dry-run` 实测迁移 `[1..12]`、22 张存在 / 32 个 V1 名跳过、身份 51 行 + 保留 3 行、待清理业务 95,176 行、无未分类告警。多家庭库现在会被明确拒绝并说明删除**不按家庭过滤**（生产实测 3 个家庭，退出码 1），`-confirm` 与库名不符、缺 `-yes-service-stopped` 也都实测退出 1。`purge -execute` 未执行 |
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
| 后端 | V2 记账 API（分类 / 账户 / 流水 / 统计 / 预算 / 项目预算）、信用卡还款计划派生、投资估值投影、账单邮箱与同步、统一错误结构；`internal/credit`、`internal/invest` 已删除 |
| 数据层 | 两个方言各 12 个迁移文件（001–012）、单一通用迁移器、`schema_migrations` 校验和与版本连续性、`schema_meta.baseline=v2` 守卫、MySQL 可重跑 DDL、Go 常量默认分类 |
| 运维工具 | `apps/api/cmd/tinynest-admin`：`baseline-capture` / `purge` / `verify` / `identity-export` / `identity-import`，单事务清理 54 个表名（V2 中存在 22 张），保留 11 张身份表与 2 张含密文的个人配置表 |
| 前端 | `App.vue` 拆为壳 + `router.ts` + `views/` + `components/` + `composables/` + `services/`；Vue Router 4；照鲨鱼记账的信息架构（底部 5 槽导航、中间凸起记账键、键盘式记账面板、4 列分类网格、按天账单 + 搜索 + 日历、环形图 + 分类排行、资产与转账、预算），配色仍是唯一的 A · 奶油团子；资产页新增投资面板，图表页新增投资收益视图 |
| 文档 | 本文件与 `docs/plans/` 下五份 V2 计划/手册，以及 `docs/api-v2.md`、`docs/er-v2.md`、`docs/accounting-rules-v2.md` |

## 3. 度量（可在代码中逐项复核）

> 本节在 2026-11-25 于候选 `91b47c9` + 本轮工作树重新逐项清点；此前的 2026-11-22 数字已被账单邮箱、绑卡、同步与银行汇总四项能力取代。

| 维度 | 实测值 | 复核位置 |
| --- | --- | --- |
| HTTP 业务路由 | **72** 条（`Register` 内按「方法 + 路径」计，含账单邮箱 5 + 绑卡 2 + 预览/同步 2 + 账单列表 1 共 10 条；另有 `/api/v1/healthz`、`/api/v1/readyz` 2 条在 `internal/server`，不计入 72）；还款计划/投资快照仍为 `GET /accounts?include=credit_debt`、`?include=investments` 投影而非新路由 | `apps/api/internal/api/setup.go`、`credit_statements.go`；`TestRouteBudget` 断言 `≤ 72` |
| 数据库表 | 空库迁移后 **37** 张（含 `schema_meta`），加迁移账本 `schema_migrations` MySQL 运行时 **38** 张，SQLite 另因 `AUTOINCREMENT` 多出 `sqlite_sequence` 为 **39** 张；两方言业务表集一致 | `internal/db/migrations/`（001 计 21、002 计 4、005 计 2、006 计 1、007 计 2、008 计 1、009–012 计 6）；明细见 `docs/er-v2.md` §1 |
| 迁移 | SQLite 与 MySQL 各 **12** 个版本文件（001–012），不再有 V1 的 21 / 14 个版本；011 复制 009/010 数据但保留旧表 | `internal/db/migrations/`、`internal/db/mysql/`；`internal/db/migrate.go` 的 `//go:embed` |
| 前端应用壳 | `App.vue` **117 行**（非空行 107；脚本与模板之外只剩样式） | `apps/web/src/App.vue` |
| 前端结构 | 9 个视图、36 个 `.vue` 组件 + 3 个组件辅助 TS、6 个 composable、6 个 service；非样式单文件最大 **485 行**（`services/api.ts`），其后 `MembersEditor.vue` 402、`ChartsView.vue` 343、`StatementMailboxSettings.vue` 323；`styles.css` 782 行作为全局 token 单独豁免 | `apps/web/src/` |
| 前端源码规模 | 65 个 `.vue`/`.ts`/`.css` 文件、8,481 行 | `apps/web/src/` |
| 后端 Go 代码 | 50 个非测试文件、11,408 行（目标 ≤7,000 行，**未达标**，如实记录） | `apps/api/`，排除 `_test.go` |
| Go 测试 | **148** 个 `Test` 函数、**35** 个 `*_test.go` 文件（含投资投影、账号绑定、迁移表、账单同步/绑定/邮箱与清理归类测试；2 条需真实 MySQL 的 drill 无 MySQL 时 skip；另有 2 个 Windows TCP 拨号 shim） | `apps/api/**/*_test.go`，以 `^func Test` 清点 |
| Playwright E2E | **96** 条用例（`financial-closure` + `public-landing`，2 个 spec 文件），覆盖 390×844 与 1440×900；本轮 96/96 通过 | `apps/web/e2e/`，`npx playwright test --list` |
| MySQL 方言回归（本机隔离实例） | 既有 API 测试 + 2 条 dbadmin drill + dump/restore 对账通过；账单相关 11 条（迁移幂等、012 DDL 重跑、8 条同步、绑定/邮箱/预览各 1 条）在隔离实例逐库单跑全绿（MySQL Community 8.4.7，TCP `127.0.0.1:3307`，临时数据目录；生产 Unix socket 未验证） | `TINYNEST_TEST_MYSQL_TCP` + `mysql_windows_test.go`；证据见 `docs/plans/credit-card-statement-import/实施进度.md` |
| 生产清理清单 | **54** 个表名，其中当前 V2 schema 中存在 **22** 张（其余 32 个为 `projects`、`credit_*`、`installment_*`、`debt_*`、`stock_*`、`schedule_*`、`statement_import_*`、`credit_mail*` 等 V1 遗留名，按 `Missing` 跳过）；保留 **11** 张身份表 + **2** 张含加密授权码的个人配置表 `statement_mailbox_configs` / `credit_statement_mailboxes` | `internal/dbadmin/tables.go`（`PurgeGroups` / `IdentityTables` / `PreservedConfigTables` / `DeleteOrder`） |

口径说明：以上为 2026-11-25 当前工作树的静态清点结果。本轮已执行 `go test ./... -count=1`、`go vet ./...`、`go mod tidy`、`go build ./...`、`npm.cmd run typecheck`、`npm.cmd run build`、Playwright **96/96**，并在本机隔离 MySQL 8.4.7（TCP 3307）上重跑 `TestMySQLPurgeDrill` 与 `TestMySQLIdentityCutoverDrill`；生产 Unix socket、race 与生产投资导入仍未执行，见 §6。
`App.vue` 的 117 行 = 非空行 107 + 空行 10；Go 测试数量以 `^func Test` 清点，含本轮新增的账单表清理归类测试。

## 4. 已退役

| 模块 | 处理 |
| --- | --- |
| 信用账单、分期、负债、到期调度 | 路由、视图、测试、`internal/credit` 包与相关表/清理入口全部退役；本次只在 `accounts` 增加账单日/还款日两列并做只读派生，不恢复账单表或生命周期 |
| 投资、股票自选 | V1 投资交易/股票自选退役；`internal/invest` 已删除。仅保留 pfm 导入的只读证券/理财估值快照（`investment_accounts` / `investment_holdings`），不恢复买卖、转账或估值重算 |
| 账单导入、信用账单邮件（IMAP） | 同上；`GET/POST /household/mail/announcements` 已删除 |
| 项目、仪表盘、退款 | 同上；`transaction_splits`、`refunds`、`projects` 不进入 V2 基线与新库 |
| V1 库原地升级 | 不支持；旧库按 [新库切换手册](plans/v2-cutover-runbook.md) 处理 |

## 5. 当前用户可完成的操作

登录后可用：**记账**（支出 / 收入 / 转账，4 列分类网格 + 自定义数字键盘）、**账单**（按天分组、搜索、日历、点行更正、长按或菜单删除）、**图表**（环形图 + 分类排行，支出/收入/投资、月/年切换）、**资产与转账**（现金 / 信用卡两类记账账户、证券/理财只读估值账户、净资产、行内转账、信用卡账单日/还款日配置、发薪日前还款现金计划、投资收益与持仓明细）、**预算**（按月单预算与进度）、**家庭成员**（邀请注册或已有账号加入、审批、成员档案与当前成员切换）、**导出**（前端生成 xlsx），以及邮箱验证、密码修改/找回、用户与审计日志。

能力边界如实标注：V2 不提供 V1 的完整信用账单、分期、利息、最低还款、自动扣款、投资交易、导入、退款与家庭公告；信用卡只做负债账户、账单日/还款日配置和只读还款计划；证券/理财只展示 pfm 导入的只读估值快照与当前持仓收益，不含历史成交流水、买卖、已实现收益重算。
账户余额由 `journal_lines` 派生，错账用冲正，不直接改余额；投资市值是明确标注的估值视图，不是账本余额。

## 6. UNVERIFIED 清单

- **生产 MySQL Unix socket 链路**：本机隔离 MySQL 8.4.7（TCP 3307）已跑通既有 API 测试、`TestMySQLPurgeDrill`、`TestMySQLIdentityCutoverDrill`、dump/restore 对账与投资投影定向用例；2026-11-25 公网 `/api/v1/readyz` 实测 `check=mysql_ping_and_migrations`、`status=ready`，B 机 socket 与受限账号在运行时已证实。仍 `UNVERIFIED` 的是在 B 机现场直接执行 `go test ./...`（本机产物用 Go 1.25.2 交叉编译，CI 用 1.23.6）。
- **生产投资快照**：已完成迁移 1–5、4 账户/30 持仓导入，生产 API 与 390×844 真实页面验证通过；iPhone 实机与生产性能仍未复测。
- **生产成员绑定与真实邀请闭环**：sunywr 已绑定爸爸；`admin` 与妈妈仍未绑定，尚未在真实生产完成邀请注册→邮箱验证→审批全链路；需用户确认剩余归属。
- **工资导入后的完整资产**：只导入了工资收入，未导入 pfm 支出/转账；工资账户已停用，资产页净资产不包含这批仅收入余额。
- **go test -race**：本机未找到 gcc/CGO 工具链，未执行；CI 的 Go API job 已在 `332a69b` 跑通 `-race`，结论是「CI 已执行、本机未执行」。
- **生产清理执行**：`purge -dry-run` 已在 B 机 `tinynest_v2` 只读取证（见发布证据），`baseline-capture` / `purge --execute` / `verify` 未执行，输出与耗时未知；[清理手册](plans/v2-prod-purge-runbook.md) 正文的「45 张清理表 / 11 张身份表」尚未按 54/22 + 11 + 2 改写，照做前必须先修订。
- **生产备份可恢复性**：定时备份已改为导出运行时库 `tinynest_v2` 并写 SHA-256 sidecar，摘要校验通过；2026-11-25 已在 B 机同实例的隔离库里恢复该转储并按[部署指南](deployment.md) §5 逐项比对通过（迁移版本、表数、`baseline`、11 张身份计数、2 张保留配置、`users` 摘要、账本与业务行数全部一致）。仍 `UNVERIFIED` 的是 **B 机之外的第二份异地副本**：没有第二份拷贝就无法做异地摘要校验，磁盘当前 `/dev/vda2` 40G、**96% 已用、1.7G 可用**，保留期与水位策略需用户定口径（模板刻意不删历史转储）。
- **生产新库切换**：`identity-export` / `identity-import`、表数与 `baseline=v2` 的实际输出未在生产数据上核对。
- **iPhone 实机**：主屏 PWA、安全区、返回手势未在真机验证。
- **登录后冒烟（信用卡还款计划与银行汇总）**：未使用真实账号口令登录，按 [部署指南](deployment.md) §9 不向 Agent 索取口令；需用户登录后在「资产」页确认汇总并排展示、在「我的 → 账单邮箱」执行一次真实同步。
- **生产性能**：LCP / CLS、分页与统计在真实数据量下的表现未标定。
- 2026-11-22 本轮已执行 `go test ./...`、`npm.cmd run typecheck`、`npm.cmd run build`（生成 dist）、Playwright **82/82**、本机隔离 MySQL 回归；生产现场仍未完整执行。

## 7. 下一步

1. 用户登录生产站点做一次实机验收：资产页银行汇总并排展示与差额、「我的 → 账单邮箱」同步一条真实邮件、信用卡账单日/还款日面板；同时确认剩余两个账号（`admin` 与妈妈）的归属。
2. 补齐备份可恢复性证据的**剩余一半**：隔离恢复逐项比对已于 2026-11-25 通过，还差**第二份异地副本**——把某个 `tinynest_v2-*.sql` 传到 B 机之外的位置，取回后比对 SHA-256，并确定保留期与磁盘水位策略（`/dev/vda2` 已用 96%，模板本身不删历史文件）。
3. 修订[生产清理手册](plans/v2-prod-purge-runbook.md)的表数口径（54 个表名 / 22 张存在 + 11 身份 + 2 保留配置），再由用户按 `baseline-capture` → 备份 → `purge --execute` → `verify` 逐条授权执行。
4. 处置本轮记在[发布证据](reviews/v2-statement-ui-deployment-evidence.md) §8 的凭据暴露：按其中步骤轮换 B 机 MySQL 运行账号口令，并更新受保护凭据文件。
5. 补齐 iPhone 实机与生产性能证据后，再把对应条目标为已验证。

## 8. 相关文档

- [V2 总计划](plans/v2-bookkeeping-reset.md) · [记账模块规格](plans/v2-bookkeeping-module-spec.md) · [三端优化计划](plans/v2-platform-optimization.md)
- [生产清理手册](plans/v2-prod-purge-runbook.md) · [新库切换手册](plans/v2-cutover-runbook.md)
- [V2 API 契约](api-v2.md) · [V2 ER](er-v2.md) · [V2 账务规则](accounting-rules-v2.md)
- [开发与本地验证](DEVELOPMENT.md) · [架构](architecture.md) · [部署](deployment.md) · [安全](security.md) · [验收](acceptance.md) · [V2 基线证据](reviews/v2-baseline.md)
