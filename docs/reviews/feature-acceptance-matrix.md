# TinyNest · 功能验收完成度矩阵（M0，基线 `a4df008`）

> 列定义：`需求 / SHA / 路由 / 前端组件 / Go API / SQL 表与迁移 / 测试 / 实测 / 状态 / 问题编号`
> 状态：`PASS`（真实代码 + 实测通过）、`PARTIAL`、`UI_ONLY`、`MISSING`、`UNVERIFIED`、`BLOCKED`
> SHA 均为 `a4df008`（除注明的历史 SHA）；「实测」为本机 2026-11-14 17:40 起真实运行结果。
> 证据路径相对仓库根；`E2E` 指 `apps/web/e2e/*.spec.ts` 中的场景编号。

## A. 公开入口与认证安全

| 需求 | SHA | 路由 | 前端组件 | Go API | SQL/迁移 | 测试 | 实测 | 状态 | 问题 |
|---|---|---|---|---|---|---|---|---|---|
| 未登录 `/` 仅品牌介绍、不显示登录表单 | a4df008 | `/` | `App.vue:600-665`（Landing，`v-if="booted && !authed && !showLogin"`） | 无（不请求私人数据） | — | `L01` `L02` | 49/49 中 L01/L02 PASS | **PASS** | — |
| 公开页无初始化页签/按钮/深链 | a4df008 | `/` `/offline.html` | `services/api.ts` 无 `api.setup`；`public/offline.html` | `POST /setup` 无 UI | — | `L01` `L15` `01 受控初始化` | 全仓 `grep` 前端 `api.setup`/`setup_token`/`首次初始化` = NONE | **PASS** | — |
| CTA 打开登录、Escape、焦点陷阱、焦点返回、防重复提交 | a4df008 | `/login` | `App.vue:149-174` `openLogin/closeLogin/onPopState`、`:637` 登录 `<form>` | `POST /auth/session` | `users` `sessions` | `L03`–`L06` `L08` `L09` `L16` | 全部 PASS | **PASS** | — |
| 已初始化后任何请求不能再建 admin | a4df008 | `POST /setup` | 无 | `setup.go:58-135`（开关→空 token→常量时间比较→`setupMu`+事务条件插入） | `households` `users` 唯一 | `TestSetupIsSingleUseAndSessionAuthEnforced` `TestSetupConcurrentBootstrapOnlyOneSucceeds` `TestSetupRequiresBootstrapToken` `TestSetupDisabledFlag*`(3) `26 初始化不可重复` `L12` | 409 / 403 / 401 契约全通过 | **PASS** | — |
| 未初始化库空/错 token 拒绝 setup | a4df008 | `POST /setup` | 无 | `setup.go:71-79` | — | `TestSetupDisabledWithoutConfiguredToken` `TestSetupRequiresBootstrapToken` | 403 `setup_disabled` / 401 | **PASS** | — |
| 运维受控初始化 + 完成后收口 | a4df008 | `POST /setup` | 无（运维 curl） | `SetupDisabled` 开关在 token 校验与 DB 读取之前 | — | `TestSetupDisabledFlagRefusesBootstrapEvenWithValidToken`（含 20 并发）`TestSetupDisabledFlagOffKeepsControlledBootstrap` | 通过；`docs/deployment.md:37-38` 记载流程 | **PASS** | — |
| 登录失败限流 / Cookie / CSRF / 可信代理 | a4df008 | `POST /auth/session` | `services/api.ts`（CSRF 头 + 401 事件） | `login_limiter.go`、`remediation` | `sessions` `audit_logs` | `TestLoginFailureRateLimit` `TestLoginRateLimitConcurrentAndAuditSafe` `TestLoginRateLimitScopeIsolation` `TestSessionCookie*`(4) | 通过 | **PASS** | — |
| 会话过期/登出清除私有状态、无重定向循环 | a4df008 | `/auth/me` | `App.vue:106-116` `clearPrivateState()`、`onSessionExpired` | 401 触发 | — | `L10` `L11` `L16` `27 API 不可用` | 通过 | **PASS** | — |
| 直接访问私有路由由服务端鉴权 | a4df008 | `/dashboard` 等 | `normalizePublicUrl()` `App.vue:117-121` | 所有 `/api/v1/*` 私有端点校验会话 | — | `L09` | 通过 | **PASS** | — |

## B. 视觉、动效与 PWA

| 需求 | SHA | 路由 | 前端组件 | Go API | SQL/迁移 | 测试 | 实测 | 状态 | 问题 |
|---|---|---|---|---|---|---|---|---|---|
| 原始糯米团子图标 hash 未变 | a4df008 | `/icons/*` | `public/icons/tinynest-original.png` | — | — | `scripts/verify-brand.py` | exit 0（2 个批准文件一致） | **PASS** | — |
| 奶油白/杏粉/暖棕主题一致 | a4df008 | — | `styles.css` tokens、`docs/design-system.md` | — | — | `L14` 三视口截图 | 三视口无横向溢出 | **PASS** | — |
| Hero/弹窗/路由过渡 + `prefers-reduced-motion` | a4df008 | — | `styles.css` `--motion-*`、`App.vue` `<Transition>`、`@media (prefers-reduced-motion: reduce)` | — | — | `L13` `L14` | 通过 | **PASS** | — |
| PWA 192/512/180 图标、manifest、SW 更新 | a4df008 | `/manifest.webmanifest` `/sw.js` | `public/sw.js` CACHE=`tinynest-shell-v0.2-landing-1`，激活清旧壳 | — | — | `L15` + `python -m json.tool manifest` | 通过 | **PASS** | — |
| SW 不缓存 `/api` 与私有财务响应 | a4df008 | `/api/v1/*` | `sw.js` `url.pathname.startsWith('/api/')` 跳过 + navigate network-first | 所有 API 响应 `Cache-Control: no-store` | — | `L15` `L02` | 通过 | **PASS** | — |
| 旧缓存升级后不显示老初始化页 | a4df008 | — | `sw.js` activate 删除所有旧 `tinynest-shell-*` | — | — | `L15` | 通过 | **PASS** | — |
| iPhone Safari 主屏安装/更新实机 | — | — | — | — | — | 无设备 | 未执行 | **UNVERIFIED** | D8 |

## C. 财务账务准确性（隔离临时 SQLite）

| 需求 | SHA | 路由 | 前端组件 | Go API | SQL/迁移 | 测试 | 实测 | 状态 | 问题 |
|---|---|---|---|---|---|---|---|---|---|
| 现金 10000 → 支出/收入/转账/拆分/退款/冲正 | a4df008 | `/transactions` `/refunds` `/transactions/{id}/reverse` | `App.vue:294-312` `createTx` | `transactions.go:70+` | `001`–`003` `journal_*` | `TestCase01_CashExpense` `TestCase02_Income` `TestCase03_Transfer` `TestCase04_29_Splits` `TestCase26_Reversal`；E2E `03`–`05` `28` `30` | 借贷平衡、12500 元、超额退款拒绝、转账不计收支全通过 | **PASS** | — |
| 信用消费 1000 → 还 600 → 退 300 → 还 100，负债 0 不重复支出 | a4df008 | `/credit-instruments` `/credit-statements` `/credit-statements/{id}/payments` | 信用账单视图 | `credit.go` `v011_credit.go` | `004_credit` `008_v011` | `TestCase05to08_CreditAndRefund` `TestV011_T1_CreditStatementLifecycle` `TestV011_CreditStatementStatusDerivation`；E2E `06`–`09` `账单日期修正` | 通过 | **PASS** | — |
| 房贷 2000（本 1500/息 500）用信用卡支付，之后还卡不重复计费 | a4df008 | `/debts` `/debts/{id}/schedule` `/debts/{id}/payments` | 分期与负债视图 | `v011_debt.go` | `005_debt` `008_v011` | `TestCase10_CreditPaysMortgage` `TestV011_T2_CreditCardPaysMortgage` `TestV011_DebtPartialPaymentReversalAndIdempotency`；E2E `10`–`12` | 通过 | **PASS** | — |
| 保险：本金 0、保费计入 5120、期次/冲正禁止本金 | a4df008 | `/debts` `/debts/{id}/schedule` `/debts/{id}/payments` | `App.vue:875` 保险选项 | `v011_debt.go:59/68/456/463/561` | `005_debt` 科目 `5120` | `TestV011_DebtContractCRUDAndValidation` `TestV011_DebtPaymentValidation` | 内核测试通过；**缺端到端固定数值断言** | **PARTIAL** | **D6** |
| 分期 3000+90 三期、部分付款守恒、退款与提前结清 | a4df008 | `/installments` `/installments/{id}/payments` `/installments/convert-from-credit` | 分期视图 | `v011_installment.go` | `008_v011` `009_installment_payment_splits` | `TestCase11to14_Installment` `TestV011_T3_InstallmentConservationAndPartialPayment` `TestV011_InstallmentSettleEarlyKeepsPaidHistory` `TestLegacyInstallmentSplitRecoveredBeforeSettlement`；E2E `13`–`19` | 通过（515 = 500 本 + 15 费） | **PASS** | — |
| 投资移动加权成本 / 固定数值 2210 / 92.50 / 1657.50 | a4df008 | `/investments/trades` `/investments/positions` | 投资视图 | `investments.go`、`internal/invest/cost.go` | `006_invest` | `TestCase18to21_MovingAverageCost` `TestV012_InvestmentFixedNumericalAcceptance` `TestV013_InvestmentBoundaryRegression`；E2E `23` `24` | 通过 | **PASS** | — |
| 费率版本/快照、费用预览与真实入账同源 | a4df008 | `/investments/fee-rules` `/fee-rules/preview` | 费率面板 | `fee_rules.go` `v012.go` `computeTradeFee` | `006_invest` | `TestV012_FeeRuleListAndPreview` `TestInvestmentFeeRuleUsesTradeDate` `TestV012_FeePreviewRejectsInvalidInput`；E2E `23` `24` | 通过 | **PASS** | — |
| 分红税 `NULL` = 未知不按 0；估值日期与收益口径非年化 | a4df008 | `/investments/dividends` `/valuations` | 分红记录面板 | `v013.go` `investments.go` | `006_invest` | `TestV013_DividendHistoryAndAnnualSummary` `TestV013_ReferenceReturnIsNotAnnualised` `TestV013_ValuationWithPastDate` `TestInvestmentValuationOmissionAndClosedPosition`；E2E `31` | 28.28% 参考收益率断言通过 | **PASS** | — |
| 自动到期：重启补扫、20 次不重复、不自动付款 | a4df008 | `/schedule/generate` `/schedule/due-items` | 首页待办 | `v011_schedule.go` | `008_v011` | `TestV011_M4_SchedulerBackfillTickConfirmAndIsolation` `TestV011_M4_NextDailyRunBoundary`；E2E `20` | 通过（合计 650，未自动付款） | **PASS** | — |
| 首页三套口径 / 净资产 / 投资收益可追溯明细 | a4df008 | `/dashboard/summary` `/dashboard/net-worth` `/dashboard/categories` `/dashboard/projects` | 首页各卡片 | `dashboard.go` `v011_dashboard.go` `v012.go` | 全部按 `household_id` 隔离 | `TestV011_T4_ThreeAccountingBasesAcrossMonthsAndYears` `TestV012_NetWorthBreakdownReconciles` `TestHouseholdIsolation`；E2E `21` `25` `29` | 通过 | **PASS** | — |
| 20 次重复/并发幂等 | a4df008 | 多写端点 | — | `Idempotency-Key` | `transactions` 唯一键 | `TestV012_IdempotentTradeReplay20Times` `TestV011_DebtPartialPaymentReversalAndIdempotency` `TestLoginRateLimitConcurrentAndAuditSafe` | 通过，重复入账 0 | **PASS** | — |
| 双家庭数据隔离 0 越权 | a4df008 | 全部私有端点 | — | SQL 层 `household_id` | 全表 | `TestHouseholdIsolation` `TestHouseholdIsolationOnDetailEndpoints` `TestV011_T5_FinancialIsolation` `TestAuditLogReadIsHouseholdScoped` `TestV012_NewEndpointsAreHouseholdScoped` `TestV013_DividendHistoryIsHouseholdScoped` | 通过 | **PASS** | — |
| 业务日 Asia/Shanghai、跨月/闰年/跨年、账单日 28/29/30/31 | a4df008 | — | — | `businessToday()`/`businessDate()` 可注入时钟 | — | `TestBusinessDate*` (3) `TestCycleFor*` (3) `TestInstallmentDueDatesClampEveryMonth` `TestCrossYearCashRefundKeepsProjectAndCashflowSeparate` `TestV011_T4_*` | 通过 | **PASS** | — |
| 整数分、复式平衡、冲正不可变 | a4df008 | — | — | `internal/money` `internal/ledger` | `002_ledger` | `TestKernelRejectsInvalidEntries` `TestDivRoundHalfUp` `TestAddOverflow` `TestParseYuan*` `db_drill.py` 借贷各 50000 | 通过 | **PASS** | — |

## D. 构建、恢复与发布

| 需求 | SHA | 路由 | 前端组件 | Go API | SQL/迁移 | 测试 | 实测 | 状态 | 问题 |
|---|---|---|---|---|---|---|---|---|---|
| Go `build/test/vet/tidy -diff` | a4df008 | — | — | — | — | 82 个 Test | 全 exit 0 | **PASS** | — |
| Web `npm ci/typecheck/build` | a4df008 | — | — | — | — | vue-tsc + vite | 全 exit 0（本机 node_modules 已装；CI 上 npm ci 12s/22s 绿） | **PASS** | — |
| ≥18 条真实浏览器 E2E | a4df008 | — | — | — | — | 49 条（31 账务 + 18 Landing） | **49/49 PASS，1.4m** | **PASS** | — |
| 390/768/1440px 三视口 | a4df008 | — | `styles.css` 媒体查询 | — | — | `22 三视口` `L14` + 6 张截图 | 无横向溢出 | **PASS** | — |
| GitHub CI 对 `a4df008` 真实全绿 | a4df008 | — | — | — | — | Actions 网页 | **无 `a4df008` 运行**；#14/#15 因 hygiene 红灯且已合并 | **UNVERIFIED / FAIL** | **D1** |
| 空库迁移、已有库升级、备份恢复一致性 | a4df008 | `/readyz` | — | Go 启动迁移 | `001`–`009` | `db_drill.py` 八步、`TestMigrationsAreIdempotent` | PASS | **PASS** | — |
| 备份产物 `foreign_key_check` + 版本/业务快照校验 | a4df008 | — | — | — | — | `scripts/backup-sqlite.py` 现仅 `quick_check` | **缺** | **PARTIAL** | **D7** |
| 生产 Cookie/反代/CSRF/限流/setup 令牌现场配置 | — | — | — | 代码已实现 | — | 无生产访问授权 | 未读取生产配置 | **UNVERIFIED** | D8 |
| 双服务器 HTTPS、私网 TLS、备份运维、`go test -race`、Linux Chromium 本分支 CI、生产 LCP/CLS、iPhone 实机 PWA | — | — | — | — | — | — | 未执行 / 无授权 | **UNVERIFIED** | D8 |

## E. 可用性（Gate D 增量目标）

| 需求 | SHA | 前端组件 | 测试 | 实测 | 状态 | 问题 |
|---|---|---|---|---|---|---|
| iPhone 快速记账路径（首页直达 + 大触控区 + 自动聚焦） | a4df008 | `App.vue` 无快捷入口，需先切「记账流水」视图 | 无 | 缺 | **MISSING** | **D3** |
| 常用分类/账户记忆（仅本地非敏感 UI 首选项） | a4df008 | 无 `localStorage` 使用 | 无 | 缺 | **MISSING** | **D4** |
| 新增记录后余额/报表刷新 | a4df008 | `App.vue:283-292` `run()` → `await loadAll()` | E2E `21` `25` | 已通过 | **PASS** | — |
| 键盘提交（Enter）与统一表单校验/加载/空状态/错误提示 | a4df008 | `App.vue:776` 记账用 `@click` 非 `<form>`，Enter 不提交；账户/信用工具未选时无前端校验 | 无 | 记账表单键盘不可提交 | **PARTIAL** | **D5** |
| 账单到期、待支付、失败与离线状态清晰 | a4df008 | 首页待办 + `27 API 不可用` + `.empty` 状态 | E2E `20` `27` | 通过 | **PASS** | — |

## F. 缺陷编号汇总（M0 基线）

| 编号 | 级别 | 内容 | 本轮处置 |
|---|---|---|---|
| **D1** | **P1** | `a4df008` 已合入 3 行尾随空格，PR #3 的 #14/#15 CI hygiene 红灯 | 修复 + 加固 CI 门禁 |
| D2 | P2 | `AGENTS.md`/`accounting-rules-v0.1.md`/`architecture.md`/`security.md` 与最新口径、迁移数、发布状态冲突 | 列为待用户决策，不擅自改写设计基线 |
| **D3** | P2 | iPhone 快速记账入口缺失 | 实施 |
| **D4** | P2 | 常用分类/账户本地首选项缺失 | 实施（仅 UI 偏好，不缓存账务） |
| **D5** | P2 | 键盘提交与统一校验缺失 | 实施 |
| **D6** | P2 | 保险端到端固定数值回归证据缺失 | 补 Go 回归测试（不开发功能） |
| **D7** | P2 | 备份产物校验不完整 | 增强 `scripts/backup-sqlite.py` + 演练 |
| D8 | UNVERIFIED | 本分支远端 CI、iPhone 实机、双机 HTTPS、生产现场、`go test -race`、生产性能样本 | 如实标注，不冒充通过 |

**基线结论**：P0=0，P1=1（D1），业务核心与上一轮体验改造全部 PASS → 不执行 Gate A/B/C 的开发，按用户指令进入 **Gate D 增量实施**。
