# TinyNest V2 验收清单

> 本文件是**可执行清单**，不是「已通过」声明。逐项执行后据实打勾；未执行项保留 `- [ ]` 并标 `UNVERIFIED`。
> 权威证据：[V2 基线证据](reviews/v2-baseline.md)（含 P5 结果）、[当前进度](project-status.md)；
> 生产清理与切换分别见 [生产清理手册](plans/v2-prod-purge-runbook.md) 与 [新库切换手册](plans/v2-cutover-runbook.md)。

## 1. 构建与静态检查

- [ ] `cd apps/api && go build ./...`
- [ ] `cd apps/api && go test ./... -count=1`（170 个 Test 函数、37 个 `*_test.go`；无 MySQL 时 2 个 drill 会 skip）
- [ ] `cd apps/api && go vet ./...`
- [ ] `cd apps/api && go mod tidy -diff`（退出码必须为 0）
- [ ] `cd apps/api && go test -race ./... -count=1`（需 cgo/gcc；不具备时标 `UNVERIFIED`）
- [ ] `cd apps/web && npm.cmd ci`（Windows 上 `npm` 被执行策略拦截时用 `npm.cmd`）
- [ ] `cd apps/web && npm.cmd run typecheck`
- [ ] `cd apps/web && npm.cmd run build`
- [ ] `cd apps/web && npm.cmd run test:e2e`（101 条用例，2 个 spec 文件）
- [ ] `git diff --check`（退出码必须为 0）
- [ ] 全树无行尾空白：`git grep -nI -E '[[:blank:]]+$' -- '*.md' '*.go' '*.ts' '*.vue' '*.css' '*.py' '*.sql' '*.yml' '*.yaml' '*.json' '*.html' '*.js'`
- [ ] `python3 scripts/verify-brand.py`（原始团子图与 Web 副本 SHA-256 一致）
- [ ] `python3 -m json.tool apps/web/public/manifest.webmanifest`

## 2. 迁移与基线

- [ ] 空库全量迁移：`TestMigrateFromZeroAppliesEveryVersion` 通过，`schema_migrations` 版本为 `[1..14]`（两个方言各 14 个迁移文件，`001`–`014`）；011 拆分把既有 QQ 配置与绑定复制进按服务商隔离的新表：`TestStatementProvidersMigrateExistingQQMailboxAndBinding` 断言升级到 `[11 12]`
- [ ] 重复执行 `Migrate()` 为 no-op：`TestMigrationsAreIdempotent`
- [ ] `013_statement_placeholders.sql` 两方言同名同义，MySQL 版可重复执行：隔离 8.4.7 实例上空库跑完 001→013（`applied_versions=13`、库内 39 张含 `schema_migrations`），已建库再 `SOURCE` 两次退出码 `0 0`，`SHOW CREATE TABLE` 显示 `PRIMARY KEY(household_id,statement_id)`、`uq_statement_placeholders_transaction`、`idx_statement_placeholders_due` 与指向 `card_statements(household_id,id)` 的 `ON DELETE CASCADE`；SQLite 空库迁移后 40 张（多 `sqlite_sequence`）
- [ ] `014_statement_details.sql` 两方言同名同义，MySQL 版可重复执行：2026-11-26 在隔离 8.4.7 实例（临时 datadir、`127.0.0.1:3307`，用完即删）空库跑完 001→014，迁移账本 `[1..14]`、`schema_meta.baseline=v2`、库内 **40 张**含 `schema_migrations`；已建库再 `SOURCE` 三次退出码 `0 0 0` 且行数不变；`information_schema` 复核 `description=text` 与 4 条外键（`households` / `accounts` / `transactions` / 复合 → `card_statements` `ON DELETE CASCADE`）、`uq_statement_details_row(household_id,account_id,row_key,seq)` 与 `uq_statement_details_transaction`；SQLite 空库迁移后 **41 张**（多 `sqlite_sequence`）。两条 dbadmin 演练（`TestMySQLPurgeDrill`、`TestMySQLIdentityCutoverDrill`，各自独立空库）与 11 条 `TestStatementDetail*` 用例（每条一个空库）在同一实例全部通过
- [ ] V1 形态的迁移账本（无 `checksum` 列）被拒绝：`TestV1ShapedLedgerIsRefusedUntouched`
- [ ] `schema_meta.baseline` 缺失或不是 `v2` 时拒绝启动：`TestBaselineGuardRejectsV1Database`、`TestBaselineGuardRequiresV2Marker`
- [ ] 迁移文件校验和被篡改时拒绝启动：`TestChecksumTamperIsDetected`
- [ ] 出现二进制不认识的版本时拒绝启动：`TestUnknownAppliedVersionIsRefused`
- [ ] 空的历史迁移账本可安全重建：`TestEmptyLegacyLedgerIsRebuilt`
- [ ] 默认分类幂等：`TestSeedForHouseholdIsIdempotent`
- [ ] `PRAGMA quick_check` 返回 `ok`、`PRAGMA foreign_key_check` 返回空（隔离临时库）
- [ ] 未触碰任何生产数据库或真实账本

## 3. 账务与安全回归

- [ ] 路由预算：`TestRouteBudget`（`Register` ≤75，实测 `Register wired 75 routes`）
- [ ] 端到端账务闭环：`TestBookkeepingLifecycle`
- [ ] 更正 = 冲正 + 替换：`TestTransactionCorrectionIsReversalPlusReplacement`
- [ ] 删除 = 只写冲正：`TestTransactionDeleteIsReversalOnly`
- [ ] 已冲正不计入统计：`TestReversedTransactionExcludedFromStats`
- [ ] 幂等重放：`TestIdempotentTransactionReplay`
- [ ] 统计与分录一致：`TestStatsConsistency`
- [ ] 预算写入与已用金额：`TestBudgetUpsertAndSpent`
- [ ] 业务日期边界：`TestBusinessDateBoundary`、`TestBusinessDateOpeningBalanceBoundaries`、`TestBusinessDateDefaultStatsPeriod`
- [ ] 家庭隔离：`TestHouseholdIsolation`、`TestHouseholdIsolationOnDetailEndpoints`、`TestHouseholdIsolationTransactionsAndStats`
- [ ] 单层分类与软删除：`TestCategoriesAreSingleLevel`、`TestDeleteCategoryRetainsHistory`、`TestCategoryMaintenance`
- [ ] 账户维护、停用/删除保留余额与分录、信用卡期初为负债：`TestAccountMaintenance`、`TestDisabledAccountRetainsBalance`、`TestDeleteAccountRetainsLedger`、`TestCreditAccountOpeningIsLiability`
- [ ] 信用卡账单日/还款日配置与校验：`TestCreditAccountCycleSettings`、`TestCreditCycleColumnsAreNullable`
- [ ] 信用卡还款计划：周期/发薪日边界、发薪日前金额、期初欠款、无发薪日/未配置、冲正更正删除、未来日期、溢缴款、两家庭隔离：`TestCreditCycleForBoundaries`、`TestNextPaydayAfterBoundaries`、`TestCreditDebtPlanBeforePayday`、`TestCreditDebtPlanMissingPaydayAndUnconfigured`、`TestCreditDebtPlanOpeningDebtIsAlreadyBilled`、`TestCreditDebtPlanReflectsReversalAndCorrection`、`TestCreditDebtPlanFutureAndOverpayment`、`TestCreditDebtPlanHouseholdIsolation`
- [ ] 投资估值快照：表结构可用、总额/账户/持仓/收益/股息/占比、家庭隔离、组合 include：`TestInvestmentSnapshotTablesAreUsable`、`TestInvestmentOverviewTotalsAndIsolation`
- [ ] 无拆分行/退款路由回归：确认 75 条路由中没有 `/refunds`、`/transactions/{id}/replace`、`/projects`、`/installments`、`/investments`、`/dashboard`、`/schedule`，也没有 V1 信用账单的还款/调整/计提/分期写路径；`GET /accounts?include=credit_debt` 与 `?include=investments` 是既有集合路由的投影，不算新增路由
- [ ] 账单邮箱与只读对账不触碰账本：`TestStatementSyncStoresBankSummaryWithoutLedgerWrites` 等 8 条 `TestStatementSync*` / `TestStatementsAreHouseholdScoped` 断言同步前后 `GET /accounts?include=credit_debt` 的 `total_debt_minor` 不变、不写 `transactions`/`journal_entries`/`journal_lines`；`/api/v1/credit-statement-mailbox*`、`/api/v1/accounts/{id}/credit-statement-{binding,preview,sync}`、`GET /api/v1/credit-statements` 这 10 条是 V2 新增的邮箱与只读汇总路由，**不代表 V1 信用账单模块已恢复**（它们不产生还款、调整、计提或分期）
- [ ] 待补明细占位写真实分录且守住双计：`TestStatementPlaceholderWritesAwaitingDetailTransaction`（借贷配平、分类「待补明细」、`occurred_on`=还款截止日、审计 `create_statement_placeholder`）、`TestStatementPlaceholderCountsFutureDeadline`（未来截止日仍进还款计划并按银行账期归期，`overdue`/`unbilled` 不被 `occurred_on` 误导）、`TestStatementPlaceholderReversalNetsThePeriodToZero`（冲正后同一日净额归零、计划不再占现金）、`TestStatementPlaceholderReplayReturnsExistingTransaction`（重复请求 409 `placeholder_exists` 且只有一笔）、`TestStatementPlaceholderRefusedWhenCycleAlreadyRecorded`（账期已有明细 → 409 `cycle_already_recorded`）、`TestStatementPlaceholderGuardsStatementState`（`cycle_not_closed` / `amount_unknown` / `due_date_unknown` 三个子用例）、`TestStatementPlaceholderDerivesDeadlineFromCardConfig`（银行未报截止日时按卡的 `due_day` 推算）、`TestStatementPlaceholderPermissionsAndScoping`（`viewer` 403、缺 `statement_id` 400、非信用卡/他家庭/他卡账单 404）、`TestStatementPlaceholderListedWithStatement`（`GET /credit-statements` 回显 `placeholder_transaction_id` / `placeholder_state`）、`TestStatementPlaceholderRowsAreHouseholdConsistent`（关联行家庭一致，SQL 层按 `household_id` 限定）。MySQL 侧同样跑通，但 `TestStatementPlaceholderGuardsStatementState` 的 3 个子用例需**每个子用例一个进程**（见 §2 的空库前提）
- [ ] 账单明细补录只把「消费」入账、逐行幂等、整批原子：`TestStatementDetailPreviewClassifiesWithoutWrites`（预览不落库，八类混合行按 消费/退款/还款或转入/费用利息/未识别 分开，表头与全空行跳过）、`TestStatementDetailImportPostsOnlyCharges`（只有消费生成一笔「账单补录」支出，`occurred_on`=银行**交易日期**而非记账日期；退款 −60、自助还款 −500、年费、负数未识别、外币、他卡行**零分录**）、`TestStatementDetailImportIsIdempotentPerLine`（重传同一文件 `created:0`+`replayed:true`；换文件只补新增那一行，且 `amount_minor` 只报本次入账的 7,770 分）、`TestStatementDetailImportRefusesAmbiguousMatch`（同日同金额两笔流水 → `held`「无法唯一对应」，不猜）、`TestStatementDetailImportHonoursStatementWindow`（账期外的消费 `held` 并写明窗口；已登记的行不重复入账）、`TestStatementDetailImportLinksUploadedFileToStatement`（`card_statement_sources.kind='xls'` + `source_digest` 同一文件只记一次）、`TestStatementDetailImportRefusesLivePlaceholderCycle`（占位未冲正 → 409 `placeholder_not_reversed` 且整批不写、占位不被自动冲正；用户冲正后同一上传成功入账）、`TestStatementDetailPermissionsAndScoping`（他家庭/未知 `statement_id` 404、现金账户 404、空行/缺卡尾 400、停用卡 404）、`TestStatementDetailImportWritesAllOrNothing`（被拒批次不留任何明细/流水/分录，他卡账单不串卡）、`TestStatementDetailRowsAreHouseholdConsistent`（跨家庭明细行插入失败）、`TestStatementDetailImportIgnoresClientCategory`（客户端提交收入/已删除/不存在的 `category_id` 一律无效，入账分类仍是「账单补录」）。清理侧：`TestPurgeClearsCardStatementTables` 断言 `statement_details` 排在 `card_statements` / `transactions` / `accounts` 之前
- [ ] 初始化安全：`TestSetupRequiresBootstrapToken`、`TestSetupDisabledWithoutConfiguredToken`、`TestSetupDisabledFlagRefusesBootstrapEvenWithValidToken`、`TestSetupConcurrentBootstrapOnlyOneSucceeds`
- [ ] Cookie `Secure` 与可信反代：`TestSessionCookieSecureInProductionProxy`、`TestSessionCookieSecureCanBeDisabledForLocalDev`、`TestSessionCookieHonorsTrustedProxyForwardedProto`、`TestSessionCookieIgnoresUntrustedForwardedProto`
- [ ] 登录失败限流：`TestLoginFailureRateLimit`、`TestLoginRateLimitScopeIsolation`、`TestLoginRateLimitConcurrentAndAuditSafe`
- [ ] 只读角色：`TestViewerSelfServiceAllowed` 与前端 H01
- [ ] 邮件：站点管理员权限、验证码、每日 3 封、队列与密钥（`mail_*_test.go` 全组）
- [ ] `go test -race ./...` 若本机无 cgo/gcc，标 `UNVERIFIED`，不得伪造通过

## 4. 前端与三端

- [ ] 应用壳 `App.vue` 为薄壳（98 行，非空 88），没有把视图/状态堆回单文件
- [ ] 唯一主题 A · 奶油团子，无第二主题与风格切换入口
- [ ] 底部 5 槽导航 + 中间凸起记账键在 <1024px 一致；≥1024px 使用左侧 226px 轨道导航
- [ ] 记账面板：支出/收入/转账、4 列分类网格、数字键盘、备注/日期/账户/成员、保存中禁用、失败重试、离线不伪造成功
- [ ] 主流程「打开记账 → 选分类 → 输入金额 → 保存」≤3 次显式点击（人工点按记录）
- [ ] 账单：按天分组与小计、搜索、日历、点行更正、长按/菜单删除（冲正）
- [ ] 图表：环形图 + 分类排行、支出/收入/投资三视图；投资视图显示总市值、成本、累计收益/率、股息/股息率、证券/理财占比、按成员收益与持仓收益排行
- [ ] 资产：现金/信用卡分组、证券/理财只读估值卡片、净资产（含投资市值并明确标注）、行内转账；信用卡可配置账单日/还款日，显示发薪日前需准备、可用现金、缺口、下次还款与账单周期；未配置/无发薪日/逾期/无欠款状态如实标注，不宣称现金足够
- [ ] 投资卡片：账户归属、市值、成本、累计收益/率、股息率、可展开持仓、最近交易快照与股息；为空账户/无持仓时如实显示
- [ ] 资产页 390×844 无横向溢出；「设置账单日」可打开编辑表单并保存；「我的 → 个人资料」保存后发薪日不丢失（E06/E07）；投资卡片/图表视图无横向溢出（E08/D06）
- [ ] 待补明细占位的界面反馈：账单已出且金额已报时才提供按钮；点击先弹确认（写明卡名、账期、金额与还款截止日），确认后成功才提示「已生成待补明细流水 …」并刷新；服务端 409 时确认层保持打开并原样显示服务器原因，不伪造成功（E11/E12，101 条 E2E 全绿）。**真实登录后的人工点按仍 `UNVERIFIED`**
- [ ] 账单明细补录的界面反馈：资产页信用卡区可打开「上传账单明细」，选择银行导出的 XLS/XLSX 后先由服务端预览逐行判定（消费/退款/还款或转入/费用利息/未识别 + 待入账笔数与金额），预览阶段不写任何流水；确认后才入账，成功提示只报**本次**入账的笔数与金额；重传同一文件不双计；卡号末四位缺失时服务器 400，界面原样显示原因而不是伪造成功（J01–J03）。390×844 弹层无横向溢出。占位未冲正时的 409 引导只有 Go 用例覆盖（`TestStatementDetailImportRefusesLivePlaceholderCycle`），**浏览器路径与真实登录后的人工点按、真实银行导出文件仍 `UNVERIFIED`**
- [ ] 我的：成员与邀请（邀请新成员、复制邀请链接、撤销邀请、审批加入申请）、分类管理、预算、导出 xlsx、邮箱与密码、管理员区（用户/审计）
- [ ] 用户管理与审计入口仅管理员可见；普通成员页面不渲染管理员区，接口返回 403（H05 / `TestMemberAccountLifecycle`）
- [ ] 家庭成员面板显示成员绑定状态；未绑定成员可生成邀请码，已绑定成员显示登录账号与角色；管理员可把本家庭已有账号直接绑定到未绑定成员，非管理员不显示邀请/审批操作
- [ ] 管理员可在家庭成员面板调整其他已加入成员的角色（管理员/成员/只读）；不能修改自己的角色（403），其余角色变更对已有会话立即生效
- [ ] 邀请链接 `/login?invite=...` 自动带入邀请码；已注册账号可通过“用邀请码加入”提交申请
- [ ] 资产页信用卡还款计划卡片有完整内边距，桌面与手机都不贴边；投资/信用卡/现金分组无横向溢出
- [ ] 「邮箱与密码」页的「邮箱与安全」卡片有完整内边距，标题与两栏表单在桌面和 390px 窄屏都不贴边
- [ ] 390×844 与 1440×900 双视口 E2E 全绿（H02/H03）
- [ ] 可点击元素热区 ≥44×44（H04）；`prefers-reduced-motion` 下无非必要动效
- [ ] PWA：Service Worker 不缓存 `/api/`；离线页无初始化入口；iPhone 实机安装标 `UNVERIFIED`

## 5. 运维 CLI 与生产执行（用户执行，当前 UNVERIFIED）

- [ ] `tinynest-admin` 与 API 共用环境变量，能连到目标库；二进制不出现在任何 HTTP 暴露路径
- [ ] `identity-export` / `identity-import` 往返：`TestIdentityExportImportRoundTrip`
- [ ] 清理表集合一致：`TestPurgeTableListsAreConsistent`；`PurgeGroups` 与 `DeleteOrder` 各 **56 个表名（16 组）**，在当前 V2 基线中 **24 张存在、32 个是 V1 遗留名**（按 `Missing` 跳过并列出）；11 张身份表 + 2 张保留的个人配置表（`statement_mailbox_configs`、`credit_statement_mailboxes`）永不进入删除清单
- [ ] 账单/绑卡/投资快照/待补明细占位可清干净且不阻塞 `DELETE FROM accounts`：`TestPurgeClearsCardStatementTables`（含 `statement_placeholders` 随账单一起清空）、`TestStatementTablesSortBeforeTheirParentsInDeleteOrder`（占位关联排在 `card_statements` 与 `transactions` 之前）；未分类表被 `Snapshot.Unclassified` 挡下：`TestV2SchemaHasNoUnclassifiedTables`
- [ ] `-household` 不缩小删除范围（多家庭会全部清空）：`TestPurgeHouseholdFlagDoesNotScopeDeletes`
- [ ] `purge --dry-run` 只读：`TestPurgeDryRunChangesNothing`
- [ ] 清理后业务表全空、身份不变：`TestPurgeEmptiesBusinessDataAndKeepsIdentity`
- [ ] 多家庭拒绝：`TestPurgeRefusesMultipleHouseholds`；借贷不平衡拒绝：`TestPurgeRefusesUnbalancedJournal`
- [ ] 可选保留审计：`TestPurgeKeepAuditPreservesHistory`；重播默认分类：`TestPurgeReseedCategoriesLeavesOnlySystemDefaults`
- [ ] `verify` 能捕获清理后新增业务行：`TestVerifyDetectsNewBusinessRow`
- [ ] MySQL 实库演练：`TestMySQLPurgeDrill`、`TestMySQLIdentityCutoverDrill`（需 `TINYNEST_TEST_MYSQL_*` 环境）；本机隔离 MySQL 8.0.46（TCP 3307）已通过，并完成 dump/restore 数据摘要一致与借贷平衡核对；生产 Unix socket / `readyz` 仍标 `UNVERIFIED`
- [ ] 生产备份 + SHA-256 校验 + 隔离恢复比对完成（`UNVERIFIED`）
- [ ] `baseline-capture` → `purge --dry-run` 人工核对 → 停服 → `purge --execute --confirm=<DBNAME> --yes-service-stopped` → `verify` → 冒烟（`UNVERIFIED`）
- [ ] 新库建基线 → 身份导出/导入 → 计数与 `users` 摘要一致 → 切换数据库配置 → `readyz` 200（`UNVERIFIED`）
- [ ] 旧库与备份保留 ≥30 天，证据写入 `docs/reviews/v2-purge-evidence.md`（`UNVERIFIED`）
- [ ] 冒烟产生的测试流水已用 `DELETE /api/v1/transactions/{id}` 冲正（`UNVERIFIED`）

## 6. 部署与环境（UNVERIFIED）

- [ ] 生产显式设置 `TINYNEST_COOKIE_SECURE=true`、初始化令牌、`TINYNEST_SETUP_DISABLED=true`、可信反代
- [ ] 首次初始化仅执行一次，之后 `/api/v1/setup` 返回 409 / 403
- [ ] B 机 loopback `/api/v1/healthz` 200，A 机 HTTPS `/api/v1/readyz` 200 且 `check` 与实际驱动一致
- [ ] A/B 双机私网 TLS、防火墙白名单、证书链实机核验
- [ ] iPhone 实机主屏 PWA、安全区、返回手势、会话过期行为
- [ ] 生产性能样本（LCP/CLS、流水分页）记录，不冒充验收通过

## 7. 发布门禁

- [ ] §1–§4 全部本地可执行项通过（无 `UNVERIFIED` 的成功声明）
- [ ] §5 生产清理与切换由用户执行并留存证据；未执行前不得宣称「已完成 V2 上线」
- [ ] §6 的实机与网络验证补齐；未补齐项保持 `UNVERIFIED`
- [ ] 文档与实际路由/表/命令一致：`docs/api-v2.md`、`docs/er-v2.md`、`docs/accounting-rules-v2.md`、`docs/DEVELOPMENT.md`
- [ ] CI 有真实绿灯（Go、Web、E2E、hygiene 四项）；未在 GitHub Actions 跑过时标 `CI_UNVERIFIED`
