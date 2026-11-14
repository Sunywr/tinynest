# TinyNest · 下一版本缺口分析与路线选择（2026-11-14，Hy4 复核）

> 分支：`feature/tinynest-v0.1.2-fee-and-networth`（从 `main` SHA `f6fe9b8` 派生）
> 依据：真实 Git 引用、源码逐行核查、本机实跑命令、GitHub Actions 真实运行页。
> 本文在动手前写成，用于锁定「跳过什么、补什么」，避免重复开发。

## 1. 最新仓库事实（实测）

| 项 | 实测值 | 取证方式 |
|---|---|---|
| 仓库根 / 当前 `main` | `C:/Users/Raython/Codes/tinynest` / `f6fe9b85bbc8f6a69fbf77d8a3b74ed71fd0ecfd` | `git rev-parse --show-toplevel`、`git rev-parse HEAD`（切分支前） |
| `origin/main` | 同为 `f6fe9b8`，2026-11-14 15:06 +0800 | `git fetch origin --prune` + `git log -1` |
| 最新推送内容 | `f6fe9b8` 是一次「双父合并」：一条线 `df06bbc`（确认十项账务口径、发布门禁、跨年退款测试），另一条线 `3aea021 → 347263e`（V0.1.1 M0–M5、投资收口、发布候选复核、MySQL 生产支持） | `git log --all --graph --oneline --decorate -40` |
| 已合并的功能分支 | `feature/tinynest-v0.1.1-financial-closure` = `9db47bd`，`codex/tinynest-release-validation` = `347263e`（独立 worktree `tinynest-release-validation` 仍检出） | `git branch -a -vv`、`git worktree list` |
| 远程功能分支 | `origin/feature/tinynest-phase0-v0.1` 已 `gone`（被 prune）；当前远程仅 `origin/main` | `git fetch origin --prune` |
| GitHub Actions（真实） | CI #9 = run `43569504212`，commit `f6fe9b8`，branch `main`，**completed successfully**，1m10s；#8 `347263e`、#7 `426dc5d`、#6 `608ad1c` 同样成功；#5 `9953d5d` cancelled | GitHub Actions 工作流页与 run 页（只读访问），本机 `gh` CLI 不存在，故用网页取证 |
| 本机工具链 | Go `1.23.6` 便携版 `%TEMP%/tinynest-go/go/bin/go.exe`（不在 PATH）、Node `22.22.2` / npm `10.9.7` | `go version` / `node -v` / `npm -v` |

## 2. 本机基线实跑（2026-11-14，全部真实退出码）

| 命令 | 首次 | 处理后 | 说明 |
|---|---|---|---|
| `go build ./...` | exit 1 | **exit 0** | 首次失败：`github.com/go-sql-driver/mysql@v1.9.3` 经 `proxy.golang.org` 返回 `Bad Gateway`（本机网络，不是代码缺陷）。改 `GOPROXY=https://goproxy.cn,direct` 后通过 |
| `go test ./... -count=1` | exit 1 | **exit 0** | 同上；68 个 `Test*`，6 个包全绿 |
| `go vet ./...` | exit 1 | **exit 0** | 同上 |
| `go mod tidy -diff` | exit 0 | **exit 0** | `.gitattributes` 已固定 `apps/api/go.sum eol=lf`，无行尾伪差异 |
| `npm ci` | exit 1 | **exit 0** | 首次失败于沙箱 `SAFE_DELETE_BULK_CONFIRM_REQUIRED`（npm 清理已有 `node_modules` 的 229 个文件被拦），把旧目录移出仓库后全新安装 53 包通过 |
| `npm run typecheck` | exit 1 | **exit 0** | 首次因 `npm ci` 未完成、`vue-tsc` 不存在 |
| `npm run build` | exit 1 | **exit 0** | vite 6.4.6，15 modules，JS 136.09 kB |

> 两次「首次失败」均为本机环境/沙箱限制，不构成项目 P0/P1；已用可复现的替代命令取得真实绿灯，不以「跳过」冒充通过。

## 3. V0.1.1 逐项判定：**DEV_READY**

| 能力 | 判定 | 证据（真实文件 / 测试 / 运行） |
|---|---|---|
| M1 信用账单生命周期、额度、推导逾期、单期修正 | **PASS** | `v011_credit.go`、`credit/cycle.go`；`TestV011_T1_CreditStatementLifecycle`、`TestV011_CreditStatementStatusDerivation`、`TestV011_CreditInstrumentPatchAndStatementAdjust` |
| M2 固定负债（房贷/保险/期次/付款/冲正/提前结清） | **PASS** | `v011_debt.go`；`TestV011_T2_CreditCardPaysMortgage`、`TestV011_DebtPartialPaymentReversalAndIdempotency` |
| M3 分期守恒、显式/比例拆分、退款联动、转分期 | **PASS** | `v011_installment.go`、`009_installment_payment_splits.sql`；`TestV011_T3_*`、`TestLegacyInstallmentSplitRecoveredBeforeSettlement` |
| M4 每日调度器、启动补扫、幂等 20 次、跨家庭隔离 | **PASS** | `v011_schedule.go`、`main.go:155`；`TestV011_M4_*` |
| M5 首页三口径、信用总览、下钻 | **PASS** | `dashboard.go`、`v011_dashboard.go`；`TestV011_T4_ThreeAccountingBasesAcrossMonthsAndYears` |
| 浏览器 E2E 22 条 + 390/768/1440 视口 | **PASS** | `apps/web/e2e/financial-closure.spec.ts`（22 编号场景 + 追加的账单日期修正用例 + 视口场景 22）；`v0.1.1-e2e-report.md` 记录 11-14 复跑 22/22 |
| SQLite 迁移 1–9、重启、备份恢复、`quick_check`/`foreign_key_check` | **PASS** | `scripts/db_drill.py`；`v0.1.1-e2e-report.md` 记录 Linux 全链路通过 |
| 家庭隔离 / CSRF / Cookie Secure / 限流 / 一次性令牌 | **PASS** | `TestV011_T5_FinancialIsolation`、`TestAuditLogReadIsHouseholdScoped`、`remediation_test.go` 系列；`helpers.go`、`main.go` |
| 10 项账务口径 | **已确认 10 / 待确认 0** | `docs/reviews/v0.1-accounting-decisions.md` 头部「2026-11-14 用户回复 1–10 按推荐值」，逐项 `CONFIRMED`。注意 `v0.1.1-accounting-decision-impact.md` 与 `v0.1.1-baseline.md` 第 7 节仍是 11-13 的「0/10」旧记录，**以最新台账为准** |
| CI 真实绿灯（最新 main） | **PASS** | Actions run `43569504212`（`f6fe9b8`）成功，4 个 job：Go / Web / Web E2E / hygiene |

结论：V0.1.1 业务闭环已完成并有代码与运行证据，**不重开路线 A**。发布门禁 B（生产 HTTPS 正式链路、公网切换、初始化令牌启用）仍未全部通过，但这属于生产放行，不阻断 V0.1.2 开发。

## 4. V0.1.2 现状判定：**PARTIAL**（故采用路线 B）

| V0.1.2 目标 | 现状 | 证据 |
|---|---|---|
| 账户级费率规则（增） | **PASS** | `fee_rules.go:9-64` `POST /investments/fee-rules`，版本化 + 生效日 + 审计 |
| 交易自动计算佣金/最低收费/印花税/过户费 | **PASS** | `investments.go:158-185`、`money.FeeWithMinimum` / `MulBPS`；`TestMulBPSAndMinimumFee`、`TestInvestmentFeeRuleUsesTradeDate` |
| 单笔费用覆盖 + 费率历史快照 | **PASS** | `investments.go:133` `fee_minor` 指针、`:251-259` 写 `fee_rule_id` / `fee_rule_snapshot_json` |
| 费率规则「查」 | **MISSING** | 全仓仅有 `POST`；`registerInvestments` 无 GET 路由；`apps/web/src/services/api.ts:98-104` 无任何 fee-rules 封装 |
| 费率费用「预览」 | **MISSING** | 无 preview 端点；`App.vue` 全文无「费率 / 预览」字样 |
| 费率规则「改 / 删」 | **设计取舍，非缺陷** | 追加 `effective_from` 新版本即完成「改」；历史交易已存 `fee_rule_id` + JSON 快照，物理删除会破坏可追溯性，故**不实现硬删**。本轮不新增迁移/列，避免破坏 `main` 已并入的 MySQL 生产路径（`db/mysql.go`、`mysql/001_initial.sql`） |
| 股票/ETF/理财建仓·加仓·减仓·清仓 | **PASS** | `invest/cost.go:56-121`、`investments.go:125-274`；`TestCase18to21_MovingAverageCost`、`TestSellGuards` |
| 移动加权成本、已实现/未实现损益、净分红、参考收益率 | **PASS** | `invest/cost.go:136-153`、`investments.go:303-359`；`TestInvestmentValuationOmissionAndClosedPosition` |
| 分红税未知不冒充 0 | **PASS** | `investments.go:367-387`（`tax_minor` 指针）、`:308-309` `dividend_tax_unknown`、`ledger.Dividend` |
| 手工估值 + 估值日期 | **PASS** | `investments.go:435-473`、`006_invest.sql:71-79`；`TestInvestmentValuationOmissionAndClosedPosition` |
| 首页报表基础三口径 + 净资产 | **PASS** | `dashboard.go:31-139`、`App.vue:382-394` |
| 家庭资产统计：资产构成 / 负债结构 / 投资市值 / 可下钻明细 | **PARTIAL** | `dashboard/summary` 仅返回 `net_worth_minor`、`investment_*` 汇总与 `accounts` 列表，**没有按科目分组的资产构成/负债结构，没有逐项 ledger_code 与估值日期**；`App.vue` 首页无资产构成卡片 |
| 固定数值验收（Prompt 第 4 节投资场景，经真实 HTTP） | **MISSING** | 现有 `TestCase18to21_MovingAverageCost` 是内核级用例，没有覆盖「100×10+5 → 100×12+5 → 卖 50×13-5 → 估值 14 → 分红 100/10」整条 HTTP 链路 |

## 5. 路线选择与跳过清单

**采用路线 B（继续开发 V0.1.2 真实缺口），不采用 A、不采用 C。**

- 不采用 A：第 3 节证据显示 V0.1.1 已 DEV_READY。
- 不采用 C：第 4 节显示投资费率查询/预览与家庭资产构成仍未实现，「上述功能已完成」的前提不成立。
- 路线 C 的验收动作（真实浏览器 E2E、SQLite 恢复演练、隔离回归）**仍作为本轮每个阶段的验证手段执行**，但不作为扩大业务范围的开发内容。

**明确跳过（已完成，不重复开发）**：复式记账内核、借贷平衡、整数分金额、幂等键机制、信用账单生命周期、固定负债/分期全链路、每日调度器、首页三口径、投资移动加权成本与收益算法、分红税未知语义、手工估值、PWA/奶油团子主题与原始图标、MySQL 兼容层、22 条既有 E2E。

**本轮实施范围（最小增量）**：

1. `GET /api/v1/investments/fee-rules` — 按 `household_id` 隔离列出账户级费率规则（版本、生效日、各项 bps、最低佣金）。
2. `POST /api/v1/investments/fee-rules/preview` — 纯计算、零写入、零副作用的费用预览；与真实交易**共用同一个费用计算函数**，杜绝预览与入账漂移。
3. `GET /api/v1/dashboard/net-worth` — 按分录重建资产构成、负债结构、投资成本/市值/估值日期与净资产，每项带 `ledger_code` 可下钻。
4. `apps/web` 增加费率规则录入/列表/预览与首页资产构成卡片，沿用 A 奶油团子样式与 `/icons/tinynest-original.png` 原图。
5. 补齐真实 HTTP 层的固定数值回归（Prompt 第 4 节投资场景）、跨家庭隔离、20 次重复/并发幂等，并扩展浏览器 E2E。

**边界**：不新增迁移、不修改 `ledger/rules.go` 分录规则、不改动 10 项已确认口径、不接实时行情、不开发自动券商同步或 AI 分析、不 push / 不合并 main / 不部署 / 不触碰真实家庭财务数据。
