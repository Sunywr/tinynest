# TinyNest · Feature × Evidence 完成度矩阵（2026-11-14）

> 基线：`main` = `f6fe9b8`；本轮分支：`feature/tinynest-v0.1.2-fee-and-networth`
> 状态：`PASS` 真实 API + SQLite + 自动化测试通过 / `PARTIAL` 部分实现 / `UI_ONLY` 仅前端 / `MISSING` 代码确实没有 / `UNVERIFIED` 环境不具备或未执行
> 证据一律指向真实文件、路由、测试名或可复现命令；README/旧报告的描述不作为完成证据。
> 版本标记：`[基线]` = 本次开工前已存在；`[本轮]` = 本分支新增。每个阶段结束后更新本表。

## A. 工程与数据底座

| # | 功能 | 状态 | 证据 |
|---|---|---|---|
| A1 | Go 模块化单体 + Vue3/TS/Vite + SQLite | PASS `[基线]` | `apps/api/go.mod`、`apps/web/package.json`、`apps/api/internal/db/db.go` |
| A2 | SQLite 迁移 1–9（append-only） | PASS `[基线]` | `migrations/001..009`；`TestMigrationsAreIdempotent`、`scripts/db_drill.py` |
| A3 | MySQL 生产旁路（不改 SQLite 默认） | PASS `[基线]` | `db/mysql.go`、`db/mysql/001_initial.sql`、`IsMySQL()` 分支；CI #8 `347263e` 成功 |
| A4 | 空库迁移 / 重启 / 在线备份恢复 / `quick_check` / `foreign_key_check` | PASS `[基线]` | `scripts/db_drill.py`；`v0.1.1-e2e-report.md` 记录 Linux 演练 |
| A5 | CI 工作流 + 最新 main 真实绿灯 | PASS `[基线]` | `.github/workflows/ci.yml` 4 job；Actions #9 run `43569504212`（`f6fe9b8`）success |
| A6 | Go build / test / vet / tidy -diff | PASS `[基线]` | 本机 2026-11-14 四项 exit 0，68 个 `Test*` |
| A7 | Web npm ci / typecheck / build | PASS `[基线]` | 本机 2026-11-14 三项 exit 0 |
| A8 | 品牌原图与 A 奶油团子主题 | PASS `[基线]` | `assets/brand/original-mochi-1024.png`（未改动）、`docs/design-system.md`、`apps/web/src/styles.css` |
| A9 | 前端零 Mock 冒充 | PASS `[基线]` | `src/mock/demo.ts` 为遗留文件，`grep -rn mock src --include=*.ts/*.vue` 无引用；`api.ts` 仅相对 `/api/v1` |

## B. 财务内核与安全（V0.1 / V0.1.1）

| # | 功能 | 状态 | 证据 |
|---|---|---|---|
| B1 | 整数分金额、定点份额、无浮点 | PASS `[基线]` | `money/money.go`、`invest/cost.go:36-54` 用 `big.Int` |
| B2 | 复式借贷平衡 + 业务行与分录同事务 | PASS `[基线]` | `ledger/entry.go`、`db.SaveEntry`、`db.WithTx` |
| B3 | `household_id` SQL 层隔离 | PASS `[基线]` | `db/db.go:252-298`；`TestV011_T5_FinancialIsolation`、`TestAuditLogReadIsHouseholdScoped` |
| B4 | 幂等键 / 并发 20 次不重复入账 | PASS `[基线]` | `v011.go` `scopedKey`、`v011_schedule.go` INSERT OR IGNORE；`TestV011_M4_*` / `TestSetupConcurrentBootstrapOnlyOneSucceeds` |
| B5 | 一次性 Bootstrap Token / CSRF / 会话 | PASS `[基线]` | `setup.go`、`helpers.go:226+`；`TestSetupRequiresBootstrapToken` 等 |
| B6 | 显式 Cookie Secure / 可信反代 / 登录限流 | PASS `[基线]` | `helpers.go:183-192`、`main.go:93-108`、`login_limiter.go`；`TestSessionCookie*`、`TestLogin*` |
| B7 | Asia/Shanghai 业务日 + 可注入时钟 | PASS `[基线]` | `helpers.go:101-119`；`TestBusinessDate*` |
| B8 | 信用账单出账/状态推导/额度/还款/退款 | PASS `[基线]` | `v011_credit.go`、`credit.go`；`TestV011_T1_*`、`TestV011_Credit*` |
| B9 | 房贷/保险/固定负债、信用卡付房贷不重复计费用 | PASS `[基线]` | `v011_debt.go`、`ledger.DebtPayment`；`TestV011_T2_CreditCardPaysMortgage` |
| B10 | 分期守恒、按比例手续费、退款联动、转分期 | PASS `[基线]` | `v011_installment.go`、`009_installment_payment_splits.sql`；`TestV011_T3_*`、`TestLegacyInstallmentSplitRecoveredBeforeSettlement` |
| B11 | 每日调度器 / 待办只生成不改账 | PASS `[基线]` | `v011_schedule.go`、`main.go:155`；`TestV011_M4_*` |
| B12 | 首页三口径 + 信用总览 + 下钻 | PASS `[基线]` | `dashboard.go`、`v011_dashboard.go`；`TestV011_T4_*` |
| B13 | 10 项账务口径确认 | PASS `[基线]` | `docs/reviews/v0.1-accounting-decisions.md` 十项 `CONFIRMED` |
| B14 | 浏览器 E2E + 390/768/1440 视口 | PASS `[基线 22 条 → 本轮 25 条]` | `apps/web/e2e/financial-closure.spec.ts`；本机 Windows Edge 真实 Go API + 独立 SQLite，**25/25 PASS（31.1s，exit 0）**；视口场景 #22 三宽度无横向溢出 |
| B15 | 生产 HTTPS 正式链路 / iPhone 实机 / 公网切换 | UNVERIFIED `[基线]` | `v0.1-release-gates.md` 第 74–92 节；本轮不部署 |

## C. V0.1.2 投资与家庭资产（本轮重点）

| # | 功能 | 状态 | 证据 |
|---|---|---|---|
| C1 | 账户级费率规则创建（版本 + 生效日） | PASS `[基线]` | `fee_rules.go:9-64`；`TestInvestmentFeeRuleUsesTradeDate` |
| C2 | 交易自动计费（佣金/最低/印花税/过户费） | PASS `[基线]` | `investments.go:158-185`、`money.FeeWithMinimum`；`TestMulBPSAndMinimumFee` |
| C3 | 单笔费用覆盖 + 费率快照 | PASS `[基线]` | `investments.go:133,251-259`、`006_invest.sql:41-42` |
| C4 | 建仓/加仓/减仓/清仓 + 移动加权平均 | PASS `[基线]` | `invest/cost.go:56-121`、`investments.go:125-274`；`TestCase18to21_MovingAverageCost`、`TestSellGuards` |
| C5 | 已实现/未实现损益、净分红、参考收益率（非年化） | PASS `[基线]` | `invest/cost.go:136-153`、`investments.go:303-359`、`dashboard.go:130-136` |
| C6 | 分红税未知显示为未知（不冒充 0） | PASS `[基线]` | `investments.go:367-387,308-309`；`v0.1-release-gates.md` B14 |
| C7 | 手工估值 + 估值日期 + 无估值资产 | PASS `[基线]` | `investments.go:435-473`；`TestInvestmentValuationOmissionAndClosedPosition` |
| C8 | 费率规则列表（查） | **PASS `[本轮]`** | `v012.go:handleListFeeRules`，路由 `GET /investments/fee-rules`；`TestV012_FeeRuleListAndPreview` 断言 2 个版本、字段齐全、按账户过滤；`TestV012_NewEndpointsAreHouseholdScoped` 断言跨家庭不可见 |
| C9 | 费率费用预览（与真实计费同源） | **PASS `[本轮]`** | `v012.go:computeTradeFee` 被 `handlePreviewFeeRule` 与 `investments.go:handleCreateTrade` **共同调用**；`TestV012_FeeRuleListAndPreview` 断言预览 520 / 620 / v2 420 且实际入账费用相等；`TestV012_FeePreviewRejectsInvalidInput` 5 个非法输入均 400 |
| C10 | 费率「改」 | PASS（追加版本语义）`[基线]` | 同 `account_id` 再 POST 一条新 `effective_from` 即 `version+1`，`feeSnapshot` 按生效日取最新；`TestV012_FeeRuleListAndPreview` 验证 v2 生效日切换 |
| C11 | 费率「删」 | **MISSING（有意不做）** | 历史交易存 `fee_rule_id` + 快照，硬删破坏可追溯；见 `next-version-gap-analysis.md` 第 4 节。前端 `App.vue` 明确提示「不提供硬删除」 |
| C12 | 前端费率录入/列表/预览 | **PASS `[本轮]`** | `api.ts` 新增 `feeRules/createFeeRule/previewFee/netWorth`；`App.vue` 投资页费率面板 + 交易费用预览块；E2E #23/#24 真实浏览器验证 ¥5.20 预览与 ¥1,005.20 入账成本 |
| C13 | 家庭资产构成 / 负债结构 / 估值日期 / 逐项 ledger_code 下钻 | **PASS `[本轮]`** | `v012.go:handleNetWorth`，`GET /dashboard/net-worth`；`TestV012_NetWorthBreakdownReconciles` 断言资产合计 1,500,000、负债合计 600,000、净资产 900,000 与 `summary` 一致，估值市值 120,000、日期 2026-11-14、逐项 `ledger_code`；前端首页卡片 + E2E #25 |
| C14 | Prompt 第 4 节投资固定数值（真实 HTTP 全链路） | **PASS `[本轮]`** | `TestV012_InvestmentFixedNumericalAcceptance`：成本 221000、净入账 64500、结转 55250、已实现 9250、余 150 股成本 165750、未实现 44250、净分红 9000、总收益 62500；未知税额保持 `tax_unknown=true` |
| C15 | V0.1.2 新端点的跨家庭隔离 / 幂等 20 次 | **PASS `[本轮]`** | `TestV012_NewEndpointsAreHouseholdScoped`（同库两家庭，列表/预览/创建/净资产 0 越权）；`TestV012_IdempotentTradeReplay20Times`（20 顺序 + 20 并发，仅 1 笔交易、余额 899500） |
| C16 | 前端交易费用留空即自动计费（修复覆盖缺陷） | **PASS `[本轮]`** | 原 `App.vue` 默认 `fee:'0'` 并总是传 `fee_minor`，使费率规则失效；本轮改为留空不传字段，E2E #24 断言自动计费 520 分 |

## D. 未验证与环境限制

| # | 项 | 状态 | 说明 |
|---|---|---|---|
| D1 | `go test -race` | UNVERIFIED | 本机缺 gcc / cgo；Linux CI Go job 未启用 race |
| D2 | GitHub Actions 本轮分支的远端运行 | UNVERIFIED | 未获 push 授权，不推送；基线 main 的 CI #9（run `43569504212`）已真实绿灯，但本分支提交尚未触发任何远端运行 |
| D3 | iPhone Safari 主屏 PWA 实机 | UNVERIFIED | 11-14 用户确认过隔离环境，正式链路未验；本轮无设备 |
| D4 | 双服务器 HTTPS 正式公网切换、生产令牌启用 | UNVERIFIED | 本轮不部署 |
| D5 | 性能 / 压力基准 | UNVERIFIED | 无基准套件；本轮只做正确性回归 |

## E. 缺陷台账（审计阶段）

| 级别 | 项 | 说明 |
|---|---|---|
| P0 | 0 | — |
| P1 | 0 | — |
| P2 | 1 | `investments.go:310` 估值子查询未显式带 `household_id`（父查询已隔离，实际不串账，形式不符守则）；本轮新增查询均显式隔离 |
| P2 | 2 | `v0.1.1-accounting-decision-impact.md` / `v0.1.1-baseline.md` 第 7 节仍写「0/10」，与最新台账「10/10」冲突（历史审查记录，未擅自改写） |
| P2 | 3 | `README.md`、`docs/DEVELOPMENT.md`、`docs/project-status.md` 仍写「7 版迁移 / E2E 未验证 / 口径待确认」，与代码不符（本轮只更新 `docs/api-v0.1.md` 的口径注记与新增路由） |
| P2 | 4 | 本机默认 Go 模块代理不可达、Go 不在 PATH、`npm ci` 受沙箱批量删除保护、环境 `HTTP_PROXY` 拦截本地 E2E 探测 —— 环境限制，非代码缺陷；解决方式见测试报告 |
| P2 | 5 | 分期合同创建分录的 `occurred_on` 取 `first_due_on`（既有行为），跨月时「截至某日」资产负债快照与「消费发生日」口径不同；涉及已确认口径，**本轮不擅自修改**，列为待用户决策 |
| P2 | 6 | `apps/web/test-results/.last-run.json` 被 git 跟踪（E2E 每次运行会改写/删除），与 `.gitignore` 的 `test-results/` 意图冲突；本轮已恢复原文件，建议后续 PR `git rm --cached` |

**阶段门禁汇总**：阶段 1（后端）6 个新测试 + 74 个 Go 全量测试 PASS；阶段 2（前端）typecheck/build PASS；阶段 3（浏览器）25/25 PASS；阶段 4（数据可靠性）`db_drill.py` PASS；最终全量回归 7 项命令 exit 0。**P0=0 / P1=0**。
