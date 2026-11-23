# TinyNest V2 验收清单

> 本文件是**可执行清单**，不是「已通过」声明。逐项执行后据实打勾；未执行项保留 `- [ ]` 并标 `UNVERIFIED`。
> 权威证据：[V2 基线证据](reviews/v2-baseline.md)（含 P5 结果）、[当前进度](project-status.md)；
> 生产清理与切换分别见 [生产清理手册](plans/v2-prod-purge-runbook.md) 与 [新库切换手册](plans/v2-cutover-runbook.md)。

## 1. 构建与静态检查

- [ ] `cd apps/api && go build ./...`
- [ ] `cd apps/api && go test ./... -count=1`（115 个 Test 函数；无 MySQL 时 2 个 drill 会 skip）
- [ ] `cd apps/api && go vet ./...`
- [ ] `cd apps/api && go mod tidy -diff`（退出码必须为 0）
- [ ] `cd apps/api && go test -race ./... -count=1`（需 cgo/gcc；不具备时标 `UNVERIFIED`）
- [ ] `cd apps/web && npm.cmd ci`（Windows 上 `npm` 被执行策略拦截时用 `npm.cmd`）
- [ ] `cd apps/web && npm.cmd run typecheck`
- [ ] `cd apps/web && npm.cmd run build`
- [ ] `cd apps/web && npm.cmd run test:e2e`（78 条用例）
- [ ] `git diff --check`（退出码必须为 0）
- [ ] 全树无行尾空白：`git grep -nI -E '[[:blank:]]+$' -- '*.md' '*.go' '*.ts' '*.vue' '*.css' '*.py' '*.sql' '*.yml' '*.yaml' '*.json' '*.html' '*.js'`
- [ ] `python3 scripts/verify-brand.py`（原始团子图与 Web 副本 SHA-256 一致）
- [ ] `python3 -m json.tool apps/web/public/manifest.webmanifest`

## 2. 迁移与基线

- [ ] 空库全量迁移：`TestMigrateFromZeroAppliesBothVersions` 通过，`schema_migrations` 版本为 1、2
- [ ] 重复执行 `Migrate()` 为 no-op：`TestMigrationsAreIdempotent`
- [ ] V1 形态的迁移账本（无 `checksum` 列）被拒绝：`TestV1ShapedLedgerIsRefusedUntouched`
- [ ] `schema_meta.baseline` 缺失或不是 `v2` 时拒绝启动：`TestBaselineGuardRejectsV1Database`、`TestBaselineGuardRequiresV2Marker`
- [ ] 迁移文件校验和被篡改时拒绝启动：`TestChecksumTamperIsDetected`
- [ ] 出现二进制不认识的版本时拒绝启动：`TestUnknownAppliedVersionIsRefused`
- [ ] 空的历史迁移账本可安全重建：`TestEmptyLegacyLedgerIsRebuilt`
- [ ] 默认分类幂等：`TestSeedForHouseholdIsIdempotent`
- [ ] `PRAGMA quick_check` 返回 `ok`、`PRAGMA foreign_key_check` 返回空（隔离临时库）
- [ ] 未触碰任何生产数据库或真实账本

## 3. 账务与安全回归

- [ ] 路由预算：`TestRouteBudget`（`Register` ≤55）
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
- [ ] 无拆分行/退款路由回归：确认 55 条路由中没有 `/refunds`、`/transactions/{id}/replace`、`/projects`、`/credit-*`、`/installments`、`/investments`、`/dashboard`、`/schedule`；`GET /accounts?include=credit_debt` 与 `?include=investments` 是既有集合路由的投影，不算新增路由
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
- [ ] 清理表集合一致：`TestPurgeTableListsAreConsistent`；45 张清理表、11 张身份表
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
