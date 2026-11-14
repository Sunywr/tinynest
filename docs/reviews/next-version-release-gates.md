# TinyNest · 发布门禁（V0.1.2 本轮复核，2026-11-14）

> 两套门禁严格分离：**门禁 A = 开发/合并工程门禁**，**门禁 B = 上公网 / 录入真实家庭财务数据的生产门禁**。
> 本轮结论：**门禁 A = DEV_READY（有条件）**；**门禁 B = NO**。
> 本轮未 push、未合并 main、未创建发布 tag、未部署生产、未使用真实家庭财务数据。

## 门禁 A · 开发基线与 PR 准备（本轮）

| # | 要求 | 状态 | 证据 |
|---|---|---|---|
| A1 | P0 / P1 缺陷为 0 | **PASS** | 新增 6 个失败测试→实现→回归；`latest-feature-evidence-matrix.md` 第 E 节 P0=0 / P1=0 |
| A2 | Go build / test / vet / tidy -diff 全 exit 0 | **PASS** | 74 个 `Test*`，四项退出码 0（`next-version-test-report.md` 2.4） |
| A3 | Web npm ci / typecheck / build 全 exit 0 | **PASS** | 53 包、vite 6.4.6、JS 143.45 kB；退出码 0 |
| A4 | 真实浏览器 E2E ≥12 条且全通过 | **PASS** | **25/25 PASS（31.1s，exit 0）**，Windows Edge + 真实 Go API + 独立 SQLite |
| A5 | 390 / 768 / 1440px 无操作阻断 | **PASS** | E2E #22 三视口横向溢出 ≤1px，截图已更新 |
| A6 | SQLite 空库迁移 / 升级重跑 / 备份恢复 / `quick_check` / `foreign_key_check` | **PASS** | `scripts/db_drill.py` DRILL PASS，迁移 `[1..9]`，原库与恢复库均 `quick_check=ok`、`foreign_key_check=0` |
| A7 | 幂等并发 / 重复 20 次不重复入账 | **PASS** | `TestV012_IdempotentTradeReplay20Times`（40 次调用仅 1 笔）；既有 `TestV011_M4_*` 20 次待办幂等 |
| A8 | 跨家庭越权 0 | **PASS** | `TestV011_T5_FinancialIsolation`、`TestV012_NewEndpointsAreHouseholdScoped` |
| A9 | 家庭隔离下推到 SQL、金额整数分、借贷平衡、单事务 | **PASS** | `db/db.go`、`ledger/entry.go`、`v012.go` 新查询全部显式 `household_id` |
| A10 | 10 项账务口径如实记录、未被擅自修改 | **PASS** | `v0.1-accounting-decisions.md` 十项 `CONFIRMED`；本轮**未修改任何账务规则/分录逻辑**，仅新增只读与配置管理端点 |
| A11 | 文档真实反映未实现能力 | **PASS（本轮新增部分）** | `docs/api-v0.1.md` 已把新端点改为 IMPLEMENTED 并保留 PLANNED 项；矩阵标注 C11「硬删有意不做」 |
| A12 | `git diff --check` / 敏感文件扫描 / `.workbuddy` 未入库 | **PASS** | 均 exit 0，`git ls-files` 无 `.env`/`.db`/`.sqlite3`/`.workbuddy` |
| A13 | 本分支 GitHub Actions 真实绿灯 | **`CI_UNVERIFIED`** | 未获 push 授权、未推送；基线 `main` 的 CI #9（run `43569504212`，commit `f6fe9b8`）真实成功，但不能替代本分支运行 |
| A14 | `go test -race` | **UNVERIFIED** | 本机缺 gcc / cgo |

**门禁 A 结论：`DEV_READY`（有条件）** —— 允许作为 PR 提交评审；若团队要求「CI 绿灯先于合并」，则本分支状态为 `DRAFT_PR_ONLY`，需用户授权 push 后由 Actions 产出真实绿灯再合并。

## 门禁 B · 生产放行（本轮不通过）

| # | 要求 | 状态 | 说明 |
|---|---|---|---|
| B1 | 10 项口径确认 + 边界回归 | **PASS（口径）/ PARTIAL（回归）** | 十项 `CONFIRMED`；分期手续费、退款跨年、无估值资产、清仓收益等已有回归，但「分期创建分录日期 = 首期到期日」的跨月口径仍需用户明确决策 |
| B2 | 完整必要业务路径 | **PASS（开发范围）** | V0.1.1 闭环 + V0.1.2 投资费率/收益/资产统计已补齐（详见矩阵） |
| B3 | 正式双服务器 HTTPS / 可信反代 / 主机名校验实测 | **UNVERIFIED** | 隔离上游 TLS 与回环预检为既有记录；正式公网虚拟主机仍待切换 |
| B4 | 生产 Cookie Secure / 初始化令牌运维闭环 | **UNVERIFIED** | 代码已强制显式配置；生产环境首次初始化接口在预检中返回 403，令牌启用与轮换未验 |
| B5 | iPhone Safari 主屏 PWA 实机验收 | **UNVERIFIED** | 无设备/证书；本轮未做 |
| B6 | A→B 私网链路、防火墙、systemd 隧道生产形态 | **UNVERIFIED** | 本轮不部署 |
| B7 | 生产备份定时任务 + 异地副本 + 季度恢复演练 | **PARTIAL** | `scripts/db_drill.py` 隔离演练 PASS；正式定时任务与异地副本未验 |
| B8 | 生产形态安全回归与审计抽查 | **PARTIAL** | 审计查询已有家庭隔离；生产抽查未做 |
| B9 | 本分支远端 CI 全绿 | **`CI_UNVERIFIED`** | 同 A13 |
| B10 | `go test -race` 在具备 gcc 的环境通过 | **UNVERIFIED** | 同 A14 |
| B11 | 性能 / 压力基准 | **UNVERIFIED** | 无基准套件 |
| B12 | 已知问题关闭 | **未关闭** | 见 `latest-feature-evidence-matrix.md` 第 E 节 P2 六项（含文档一致性、既有分期口径、`.last-run.json` 跟踪） |

**门禁 B 结论：`NO` / 禁止部署生产、禁止录入真实家庭财务数据。**

## 两套门禁关系

```
本轮功能分支（本地 commit，未推送）
   │
   ├─ 门禁 A ── DEV_READY（有条件）──> 可提交 PR 评审
   │              └─ 需用户授权 push 后 CI 真实绿灯（A13）
   │
   └─ 门禁 B ── NO ──> 必须补齐 B3～B12 才可 PRODUCTION_READY
```

任何「V0.1.2 已发布 / 可直接录入真实账目」的表述在当前状态下都是虚假声明。本轮所有验证数据均为独立临时库中的测试数据。

## 本轮 Git 边界确认

- 工作分支 `feature/tinynest-v0.1.2-fee-and-networth`，从 `main` `f6fe9b8` 派生。
- **push：NO · merge main：NO · 发布 tag：NO · 生产部署：NO**。
- 用户提供的两份 `docs/plans/*-PROMPT/STEPS.md` 为未跟踪文件，全程原样保留、未纳入提交。
- 另一 worktree 的 `codex/tinynest-release-validation`（`347263e`）未被改动。
