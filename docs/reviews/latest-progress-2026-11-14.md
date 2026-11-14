# TinyNest · 2026-11-14 最新进度审计报告（Hy4 / 吴八哥）

> 审计对象：`main` = `f6fe9b85bbc8f6a69fbf77d8a3b74ed71fd0ecfd`（2026-11-14 15:06 +0800）
> 审计方式：`git fetch --all --prune` → 分支/提交图 → 源码逐文件 → 本机真实构建/测试 → GitHub Actions 网页取证
> 工作分支：`feature/tinynest-v0.1.2-fee-and-networth`（本地，未推送）
> 用户提供的 `docs/plans/WORKBUDDY-HY4-NEXT-IMPLEMENTATION-PROMPT.md` 与 `docs/plans/TINYNEST-PROGRESS-AND-NEXT-STEPS.md` 为未跟踪文件，全程原样保留，未改动、未纳入提交。

## 1. 结论摘要

| 项 | 结论 |
|---|---|
| V0.1.1 | **DEV_READY**（业务闭环、测试、E2E、CI、迁移、口径确认均有证据） |
| V0.1.2 | **PARTIAL**（投资内核完成；费率查询/预览、前端费率 UI、家庭资产构成统计缺失） |
| 本轮路线 | **B** — 只补 V0.1.2 真实缺口，详见 `next-version-gap-analysis.md` |
| 生产放行 | **NO** — 发布门禁 B 未全部通过；不得录入真实家庭财务数据 |

## 2. 最新推送在哪里、新增了什么

`f6fe9b8` 是 `main` 上的一次双父合并：

```
*   f6fe9b8 (main, origin/main)  Merge branches 'main' and 'main'
|\
| * 347263e  Support MySQL for TinyNest production
| * 426dc5d  Close investment and audit release checks
| * 608ad1c  Remove trailing whitespace from release plan
| * 9953d5d  Validate TinyNest release candidate and confirmed accounting rules
| * 9db47bd  Complete TinyNest V0.1.1 local financial regression
| * 3dfa96c  feat(v0.1.1): frontend for M1-M5 ...
| * bab3017  M5 dashboard closure
| * 7715706  M4 automatic daily due-item scheduler
| * e91bdd4  M3 installment integrity
| * 0011324  M2 fixed liabilities
| * a52ba02  M1 credit statement lifecycle
| * 3aea021  docs(v0.1.1): M0 baseline, migration 008
* | df06bbc  feat: 更新项目状态和发布门禁，确认十项账务口径，添加退款跨年测试
|/
* df2a9b3 (feature/tinynest-phase0-v0.1)
```

相对上一轮报告（`v0.1.1-implementation-matrix.md`，11-13）新增：

- **迁移**：`008_v011.sql` → `009_installment_payment_splits.sql`（从已入账分录回填历史付款拆分）；另增 `internal/db/mysql/001_initial.sql` 与 `seed.sql`。
- **后端**：`v011_*.go` 全套、`fee_rules.go`、`v011_dashboard.go`、`v011_schedule.go`、`db/mysql.go`、`IsMySQL()` 分支、`accounting_decisions_test.go`、`v011_isolation_test.go`。
- **投资收口**：费率生效日/追加版本、费用快照、未估值资产与清仓收益、审计查询的家庭隔离。
- **前端**：`App.vue` 的 M1–M5 界面（信用、负债、分期、待办、首页信用总览）、持仓页红利税未知提示、离线壳。
- **测试**：68 个 Go `Test*`；E2E 22 条 + 账单日期修正追加用例；`TestCrossYearCashRefundKeepsProjectAndCashflowSeparate`、`TestInvestmentFeeRuleUsesTradeDate`、`TestInvestmentValuationOmissionAndClosedPosition`、`TestLegacyInstallmentSplitRecoveredBeforeSettlement` 等。
- **CI**：`.github/workflows/ci.yml` 4 个 job；Actions #6~#9 真实成功，其中 #9 对应最新 `f6fe9b8`。
- **口径**：`docs/reviews/v0.1-accounting-decisions.md` 头部记录 2026-11-14 用户回复「1–10 按推荐值」，十项全部 `CONFIRMED`。

## 3. 安全修复是否仍保留（读源码，不只读旧报告）

| 项 | 结论 | 代码位置 |
|---|---|---|
| 一次性 Bootstrap Token、空令牌 fail closed | **保留** | `setup.go:46-62`、`:95-125` |
| 所有财务 SQL 按 `household_id` 隔离 | **保留** | `db/db.go:252-298`（`Balance` / `Aggregate` / `Cashflow` 均在 WHERE 带 household）、`v011_debt.go`、`v011_installment.go` |
| Asia/Shanghai 业务日 + 可注入时钟 | **保留** | `helpers.go:101-119`、`dashboard.go:16-27` |
| Cookie Secure 显式配置、可信反代 | **保留** | `helpers.go:183-192`、`cmd/server/main.go:93-108`（缺失即退出） |
| 登录失败限流 | **保留** | `login_limiter.go`；`TestLoginFailureRateLimit` 等 3 个回归 |
| 整数分 / 复式借贷平衡 / 单事务 | **保留** | `money/money.go`、`ledger/entry.go`、`db.SaveEntry` |
| 已入账不可硬删，用冲正 | **保留** | `transactions.go` `/reverse`、`v011_debt.go` payment reverse |

> 审计中发现的形式问题（P2，非越权）：`investments.go:310` 的估值子查询按 `asset_id` 关联而未直接带 `household_id`；`asset_id` 全局唯一且父查询已隔离，实际不串账，但不符合「每条 SQL 显式限定 household」的字面守则。本轮新增的统计端点全部显式带 `household_id`，并把此项列入 P2 台账。

## 4. 真实执行记录（本机，2026-11-14）

| 命令 | 退出码 | 备注 |
|---|---|---|
| `git fetch origin --prune` | 0 | 远程仅剩 `origin/main` |
| `go build ./...`（`GOPROXY=https://goproxy.cn,direct`） | 0 | 默认 `proxy.golang.org` 下载 mysql 驱动 Bad Gateway，改源后通过 |
| `go test ./... -count=1` | 0 | 68 个 `Test*`；api 8.940s / credit 2.005s / invest 0.067s / ledger 2.843s / money 5.186s / server 4.233s |
| `go vet ./...` | 0 | — |
| `go mod tidy -diff` | 0 | `.gitattributes` 固定 `go.sum` LF |
| `npm ci --no-audit --no-fund` | 0 | 53 packages；旧 `node_modules` 移出仓库以避开沙箱批量删除保护 |
| `npm run typecheck` | 0 | `vue-tsc --noEmit` |
| `npm run build` | 0 | vite 6.4.6，15 modules，JS 136.09 kB / CSS 15.44 kB |

**GitHub Actions（真实，非本地等价）**：run `43569504212` / CI #9，commit `f6fe9b8`，branch `main`，2026-11-14，`completed successfully`，1m10s，4 个 job 全绿（Go API 21s / Web 13s / Web E2E 49s / hygiene 4s）。另有 #8 `347263e`、#7 `426dc5d`、#6 `608ad1c` 成功；#5 `9953d5d` cancelled（被后续推送取代）。本机 `gh` CLI 不存在，结论来自只读访问 Actions 网页，不杜撰 job 日志细节。

## 5. 与两份交接文档的差异修正

1. `TINYNEST-PROGRESS-AND-NEXT-STEPS.md` 第 1 节把全表标为 `UNVERIFIED`，并称「本次未能读取 GitHub 最新 main」——本轮已用 Git 远程引用 + Actions 网页覆盖为实测值。
2. `WORKBUDDY-HY4-NEXT-IMPLEMENTATION-PROMPT.md` 第 3 节说「10 项口径没有用户确认的继续 `PENDING_USER_CONFIRMATION`」——最新台账明确十项已确认，故本轮**不改动任何口径**，只补配置管理与报表能力。
3. 两份文档列的 V0.1.2 待办中，「投资成本/收益/分红/估值」实际已由 `426dc5d` 完成，本轮跳过，只补剩余缺口。
4. `docs/reviews/v0.1.1-accounting-decision-impact.md` 与 `v0.1.1-baseline.md` 第 7 节仍写「0 确认 / 10 待确认」，与最新台账冲突，列入 P2 文档一致性缺陷，本轮不擅自改写历史审查记录。
5. `README.md` / `docs/DEVELOPMENT.md` / `docs/project-status.md` 仍写「7 版迁移、E2E 未验证、口径待确认」，与代码事实不符，列入 P2 文档债务。

## 6. 环境限制（如实标注）

- 本机 `gh` 未安装，无法读取 PR 列表与 job 原始日志；Actions 状态以公开运行页为准。
- 本机 Go 不在 PATH，且默认模块代理不可达；已用便携 Go + 可用代理完成等价验证。
- `go test -race` 本机缺 gcc，`UNVERIFIED`（Linux CI 的 Go job 不含 race）。
- iPhone Safari 实机 PWA、正式双服务器 HTTPS 公网切换、生产初始化令牌启用：**UNVERIFIED / 未完成**，本轮不部署、不触碰生产。
