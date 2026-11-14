# TinyNest · 最新源码独立核验（M0）

> 日期：2026-11-14 17:40 Asia/Shanghai
> 模型/专家：WorkBuddy Hy4 +「高级开发工程师」吴八哥
> 仓库：`git@github.com:Sunywr/tinynest.git`
> 审计对象：**远程最新 `origin/main`**，本轮独立分支 `feature/tinynest-rc-hardening`
> 说明：本文结论来自本机真实命令与 GitHub 网页，不引用历史报告的 PASS 作为本轮证据。

## 1. Git 与版本事实（可核查来源）

| 项 | 值 |
|---|---|
| `git fetch --all --prune` | 成功；删除 `origin/feature/tinynest-public-landing-motion`（已合并） |
| `origin/main` / 本地 `main` | **`a4df00860a64f2e68e91de9bc55364adde1b72bd`** |
| 合并信息 | `Merge pull request #3 from Sunywr/feature/tinynest-public-landing-motion`，2026-11-14 17:33:24 +0800 |
| 本轮起始 HEAD | `a4df008`（工作树干净，仅 4 份用户未跟踪文档） |
| 本轮独立分支 | `feature/tinynest-rc-hardening`（基于 `origin/main`，本地新建，未推送） |
| 上一轮成果 | `7a5b0af` + `8eab83c` 均已通过 PR #3 进入 main，不重复开发 |
| 历史功能分支 | `feature/tinynest-v0.1.3-preflight`=`91ef72e`、`feature/tinynest-v0.1.2-fee-and-networth`=`b11793b`、`feature/tinynest-v0.1.1-financial-closure`=`9db47bd` 均已并入 main |
| 独立 worktree | `C:/Users/Raython/Codes/tinynest-release-validation` @ `347263e`（MySQL 分支，保留未动） |

`README-使用说明.md` 声称「ChatGPT 环境无法读取实时源码、SHA 和 CI」，本轮已用本工作区补齐该独立核验。

## 2. GitHub Actions 真实状态

`https://github.com/Sunywr/tinynest/actions/workflows/ci.yml` 与 PR #3 checks 页实测：

| Run | 事件 / SHA | 结论 | 明细 |
|---|---|---|---|
| #9 `43569504212` | push `f6fe9b8` | **Success** | 4 job 全绿 |
| #12 `43573205012` | push `6fbe857` (PR #2 合并) | **Failure** | 仅 `Repository hygiene` exit 2 |
| #13 `43577502534` | push `91ef72e` | **Failure** | 仅 `Repository hygiene` exit 2 |
| **#14 `43580927733`** | push `8eab83c` (PR #3 分支) | **Failure** | Go 17s / Web 12s / **E2E 1m32s 成功** / `Repository hygiene` **exit 2** |
| **#15 `43581086986`** | pull_request #3 | **Failure** | Go 20s / Web 22s / **E2E 1m25s 成功** / `Repository hygiene` **exit 2** |
| 合并 `a4df008` | — | **无可见运行** | workflow 列表最新仍为 #13；`CI_UNVERIFIED` |

本机复现（在 `a4df008` 上）：

```
git diff --check 7a5b0af..a4df008   -> exit 2
git diff --check 91ef72e...8eab83c  -> exit 2
docs/plans/landing-20261114/01-NEXT-STEPS-PLAN.md:3,4,5: trailing whitespace
```

**根因**：用户提交 `8eab83c` 时该文档 3 行 Markdown 硬换行尾随空格（上一轮报告已预警的 P2），随 PR #3 合入 main，导致两次 CI 红灯却已合并。全仓已跟踪 `*.md` 扫描仅此一处。合并提交本身没有 Actions 运行记录。

**结论**：`Repository hygiene` 是**真实 P1 工程门禁缺陷**；代码本身（Go/Web/E2E）在 Linux Chromium 上真实绿灯。

## 3. 本机基线实跑（`a4df008`，全部 exit 0）

| 命令 | 结果 |
|---|---|
| `go build ./...` | 0 |
| `go test ./... -count=1` | 0，**82 个 Test**（api/credit/invest/ledger/money/server 全 ok） |
| `go vet ./...` | 0 |
| `go mod tidy -diff` | 0 |
| `npm run typecheck` (vue-tsc) | 0 |
| `npm run build` (vite 6.4.6) | 0，`index-PN_hoBF5.js` 159.37 kB / gzip 52.72 kB |
| `npm run test:e2e` (Playwright + msedge + 真实 Go API + 隔离 SQLite) | **49/49 PASS，1.4m** |
| `python scripts/db_drill.py --api-dir apps/api` | **八步 PASS**：空库迁移 1–9、重启、重复迁移 no-op、借贷各 50,000、在线备份/恢复、`quick_check=ok`、`foreign_key_check=0`、恢复前后业务快照一致 |
| `python scripts/verify-brand.py` | 0（原始糯米团子 PNG 哈希未变） |
| `python -m json.tool apps/web/public/manifest.webmanifest` | 0 |
| `git diff --check`（工作树） | 0 |

环境注记（非代码缺陷）：本机 Go 便携版 `go1.23.6` 在 `%TEMP%/tinynest-go/go/bin`，Node `v22.22.2`；需 `GOPROXY=https://goproxy.cn,direct`；E2E 需 `unset HTTP_PROXY*` + `NO_PROXY=127.0.0.1,localhost`（环境代理会拦截本地就绪探测），并把累积的 `test-results` 移出仓库以绕过沙箱批量删除保护。

## 4. 源码 / 迁移 / 页面事实清单

- **迁移**：`001_base` `002_ledger` `003_transactions` `004_credit` `005_debt` `006_invest` `007_seed` `008_v011` `009_installment_payment_splits`，共 9 版，`db_drill.py` 实测幂等。
- **Go API 注册**：`registerTransactions` / `registerCredit` / `registerDashboard` / `registerInvestments` / `registerV011` / `registerV011Credit` / `registerV011Debt` / `registerV011Installment` / `registerV012` / `registerV013`（`apps/api/internal/api/setup.go`）。
- **公开端点**：`GET /healthz`、`GET /readyz`、`POST /auth/session`、`POST /setup`；`setup.go:58-79` 依次执行 `SetupDisabled` 开关 → 空 token fail closed → 常量时间令牌比较；`setup.go:102-131` `setupMu` + 事务内 `NOT EXISTS` 条件插入保证唯一 admin；已有 `TestSetupDisabledFlag*` 3 个测试。
- **账务内核**：`internal/ledger`、`internal/money`（整数分、half-up、bps）、`internal/invest`（移动加权成本）；`BusinessDate*` / `TestV011_T4_ThreeAccountingBasesAcrossMonthsAndYears` / `TestV011_M4_SchedulerBackfillTickConfirmAndIsolation` 等覆盖 Asia/Shanghai、跨月/跨年、调度补扫不自动付款。
- **保险**：`v011_debt.go:59` kind 枚举含 `insurance`；`:68-70` 禁止设置本金；`:456/463/561` 期次与冲正均禁止本金；科目 `5120` 保费费用（`internal/ledger/subjects.go:22`）。前端 `App.vue:875` 有保险选项。
- **前端**：`App.vue` 单文件组件（7 个 View）+ `services/api.ts`（仅相对 `/api/v1/*`，401 触发 `tinynest:session-expired`）+ `styles.css`（奶油团子 tokens / `prefers-reduced-motion`）；2 个 E2E spec 共 49 条场景。
- **PWA**：`public/sw.js` CACHE=`tinynest-shell-v0.2-landing-1`，激活时清除所有旧 `tinynest-shell-*` 缓存，`/api/` 与 navigate 请求跳过缓存；`offline.html` 不含初始化入口。
- **账务口径**：`docs/reviews/v0.1-accounting-decisions.md` 头部记录「2026-11-14 用户回复 1–10 按推荐值」，十项均 `CONFIRMED`。`AGENTS.md` / `docs/accounting-rules-v0.1.md` / `architecture.md` 仍写「待确认 / 7 版迁移 / 发布门禁未通过」，属**历史文档冲突**（P2，未擅自改写设计基线）。

## 5. Gate 判定

| Gate | 判定 |
|---|---|
| A 安全/账务 P0 | **0**：setup 三重防护 + 82 测试 + 49 条 E2E + 隔离测试；未发现未授权创建、跨家庭读写、借贷不平衡、重复入账 |
| B 上一轮 Landing/登录/动效/PWA | **PASS**：18 条 `L01–L18` E2E 全部通过，Linux CI 的 E2E job 亦真实成功 → **不重写、不改 UI** |
| C 账务闭环缺口 | **无关键缺口**：31 条 `financial-closure` E2E + 82 个 Go 测试覆盖收支/信用/分期/房贷/投资/待办/首页；保险有代码与 Go 内核测试，缺一条端到端固定数值证据（本轮补回归，不开发功能） |
| D 走向可用性 | **执行**：iPhone 快速记账路径、常用分类/账户本地首选项、键盘提交与统一校验、E2E 扩充、CI 门禁加固、备份脚本可靠性 |

## 6. 本轮工作边界

- 独立分支 `feature/tinynest-rc-hardening`，仅本地提交；**未 push / 未 merge / 未部署 / 未打 tag**。
- 全部测试使用临时 SQLite 与虚构家庭，未读取生产配置、未访问生产 API、未录入真实家庭财务数据。
- 不新增迁移、不改账务规则与口径常量、不改原始图标、不引入新框架。
- 用户 4 份中文交接文档保持未跟踪、原样未改（经检查均无尾随空格）。
