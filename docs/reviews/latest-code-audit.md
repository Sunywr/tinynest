# TinyNest · 最新代码验收报告（2026-11-14 第三轮 / M0）

| 项 | 实测值 |
|---|---|
| 仓库 | `git@github.com:Sunywr/tinynest.git`（`https://github.com/Sunywr/tinynest.git`） |
| 最新 `main` / `origin/main` | **`91ef72eee2378ff09514e8f2bd5a2d5f961dee26`**（2026-11-14 16:47 +0800，「添加 TinyNest 手动验收与核对清单及相关实施计划文档」） |
| 本轮分支 | `feature/tinynest-public-landing-motion`，从同步后的 `main` = `91ef72e` 派生 |
| 审计时工作树 | `git status --short` 仅 `?? docs/plans/landing-20261114/`（用户本轮三份输入文档，未跟踪、原样保留） |
| 执行日期 | 2026-11-14 16:57–17:05 GMT+8 |
| 环境 | Windows / Go `1.23.6`（便携版，不在 PATH）/ Node `22.22.2` / npm `10.9.7` / Python `3.13.14` / 系统 Microsoft Edge |
| 数据 | 全部独立临时 SQLite；**未读取或修改生产数据库与真实家庭财务数据** |

## 1. Git 事实与相对上一轮的差异

- `git fetch --all --prune` 成功，远程仅 `origin/main`；历史分支 `feature/tinynest-v0.1.2-fee-and-networth`=`b11793b`、`feature/tinynest-v0.1.3-preflight`=`91ef72e`、`codex/tinynest-release-validation`=`347263e`（独立 worktree）。
- 相对上一轮交付（`5ccc551`）新增 2 个提交：`6fbe857`（PR #2 合并，已含 V0.1.2 全部成果）、`91ef72e`（用户新增 `docs/plans/2026-11-14/` 四份文档，共 +329 行，无源码改动）。
- **`91ef72e` 不含任何前后端代码变更**，因此业务能力与上一轮验收一致；本轮改动起点是完整 V0.1.1 + V0.1.2 基线。

## 2. GitHub Actions 真实结果（只读网页取证，本机无 `gh`）

| run | 事件 / 提交 | 结论 | 失败点 |
|---|---|---|---|
| #9 `43569504212` | push `f6fe9b8` | **Success**（4 job 全绿） | — |
| #10 `43573074289` / #11 `43573131991` | PR #2 分支 `b11793b` | Failure | `Repository hygiene` exit 2（历史尾随空格） |
| #12 `43573205012` | push `6fbe857`（PR #2 合并） | **Failure** | `Repository hygiene` `#step:3:29` exit 2 |
| #13 `43577502534` | push **`91ef72e`（最新 main）** | **Failure** | `Repository hygiene` `#step:3:29` exit 2 |

**根因（本机复现）**：`.github/workflows/ci.yml:95` 的 `git diff --check`。`git diff --check 5ccc551..91ef72e` 输出 6 条 `trailing whitespace` 并 **exit 2**：

```
docs/plans/2026-11-14/TINYNEST-ACCEPTANCE-AND-NEXT-PLAN.md:3,4,5
docs/plans/2026-11-14/WORKBUDDY-HY4-LATEST-AUDIT-AND-IMPLEMENT.md:3,4,5
```

这两份文档由 `91ef72e` 提交（各 3 行 Markdown 硬换行尾随空格）。Go/Web/E2E 三个 job 在 #13 均未标记失败。**这是 P1 工程门禁缺陷（CI 红灯却已合并 main），本轮 M1 顺带修复**。注意：本轮新输入 `docs/plans/landing-20261114/01-NEXT-STEPS-PLAN.md` 也有 3 行同类尾随空格，当前未跟踪、不纳入提交；若将来纳入版本控制必须先清理。

## 3. 本机基线实跑（真实退出码）

| 命令 | 退出码 | 结果 |
|---|---|---|
| `go build ./...` | 0 | — |
| `go test ./... -count=1` | 0 | **80 个 `Test*`**，6 个包全绿 |
| `go vet ./...` | 0 | — |
| `go mod tidy -diff` | 0 | — |
| `npm ci --no-audit --no-fund` | 0 | 53 packages（旧 `node_modules` 移出仓库后全新安装，规避沙箱批量删除保护） |
| `npm run typecheck` | 0 | `vue-tsc --noEmit` |
| `npm run build` | 0 | vite 6.4.6，15 modules，JS 145.92 kB / CSS 15.44 kB |
| `npm run test:e2e` | 0 | **31/31 PASS（36.1s）**，Edge + 真实 Go API + 独立 SQLite |
| `python scripts/db_drill.py --api-dir apps/api` | 0 | 八步 PASS：迁移 `[1..9]`、重复迁移 no-op、借贷平衡、备份恢复、快照一致、`quick_check=ok`、`foreign_key_check=0` |
| `python scripts/verify-brand.py` | 0 | 原始团子图标 SHA-256 一致 |
| `python -m json.tool apps/web/public/manifest.webmanifest` | 0 | PWA 清单合法 |
| `git diff --check`（工作树） | 0 | 工作树干净 |
| `git diff --check 5ccc551..91ef72e` | **2** | 复现 CI 红灯 |

Go 需 `GOPROXY=https://goproxy.cn,direct`；E2E 需 `unset HTTP_PROXY*` + `NO_PROXY=127.0.0.1,localhost`（环境代理会拦截本地就绪探测）。均为本机环境限制，非代码缺陷。

## 4. 完成度矩阵（源码 + 测试证据）

| 模块 | 状态 | 源码 / API / 迁移 | 测试证据 |
|---|---|---|---|
| 复式记账、整数分、事务、冲正 | **PASS** | `money/money.go`、`ledger/entry.go:62-92`、`db/db.go:202-240` | `TestKernelRejectsInvalidEntries`、`TestCase26_Reversal`、`db_drill.py` 借贷各 50,000 |
| 收支/账户/项目/退款 | **PASS** | `transactions.go:15-21`、`setup.go:26-33`、`003_transactions.sql` | `TestV01EndToEnd`、`TestCase01..04`、E2E #03–#05/#28 |
| 信用账单（出账/逾期/额度/核对） | **PASS** | `credit.go:18-27`、`v011_credit.go:15-18`、`008_v011.sql` | `TestV011_T1_*`、`TestV011_CreditStatementStatusDerivation`、E2E #06–#09 |
| 房贷/保险/分期/负债 | **PASS** | `v011_debt.go:15-22`、`v011_installment.go:17-21`、`009_*.sql` | `TestV011_T2_*`、`TestV011_T3_*`、E2E #10–#19 |
| 每日调度与待办幂等 | **PASS** | `v011_schedule.go:78-140,204-242`、`main.go:186` | `TestV011_M4_*`、E2E #20 |
| 首页三口径 / 净资产 / 下钻 | **PASS** | `dashboard.go:31-139`、`v011_dashboard.go`、`v012.go:handleNetWorth` | `TestV011_T4_*`、`TestV012_NetWorthBreakdownReconciles`、E2E #21/#25/#29 |
| 投资费率/快照/成本/收益/分红/估值 | **PASS** | `fee_rules.go`、`v012.go:computeTradeFee`、`invest/cost.go`、`v013.go`、`006_invest.sql` | `TestV012_*`（6）、`TestV013_*`（5）、E2E #23/#24/#31 |
| 家庭隔离 / Cookie / 代理 / 限流 / CSRF | **PASS** | `db/db.go:252-298`、`helpers.go:183-192,226-256`、`main.go:113-132`、`login_limiter.go` | `TestHouseholdIsolation*`、`TestSessionCookie*`、`TestLogin*`、`TestSetup*` |
| SQLite 迁移 1–9 / 备份恢复 | **PASS** | `migrations/001..009`、`scripts/db_drill.py` | `TestMigrationsAreIdempotent`、八步演练 |
| 品牌 A 奶油团子 + 原始图标 | **PASS** | `docs/design-system.md`、`assets/brand/original-mochi-1024.png`、`apps/web/public/icons/tinynest-original.png` | `verify-brand.py` exit 0 |
| **公开初始化入口（本轮 P0）** | **MISSING（需移除）** | `App.vue:25,106-110,447-465` 仍有 `authForm.mode='setup'`、"首次初始化"页签、`setup_token` 输入框；`api.ts:39-40` 仍有 `api.setup()` | 无「公开页看不到 setup」的断言；E2E #01 反而通过 UI 初始化 |
| **Go setup API 安全性** | **PASS（部分）** | `setup.go:58-132`：空 token→403、错 token→401、`setupMu`+事务内 `COUNT(households)`+条件 INSERT→409；不依赖 `RemoteAddr` | `TestSetupRequiresBootstrapToken`、`TestSetupDisabledWithoutConfiguredToken`、`TestSetupConcurrentBootstrapOnlyOneSucceeds`、E2E #01/#26。**缺** `TINYNEST_SETUP_DISABLED` 受控开关 |
| **未登录产品介绍页** | **MISSING** | `App.vue:448-465` 未登录直接显示登录/初始化卡片 | 无 Landing 相关 E2E |
| **统一动效 / prefers-reduced-motion** | **PARTIAL** | `styles.css` 仅 `@media(prefers-reduced-motion: reduce){*{transition:none!important}}`，无统一 tokens、无入场/呼吸/路由过渡 | 无动效 E2E |
| **PWA 缓存更新** | **PARTIAL** | `sw.js` CACHE=`tinynest-shell-v0.1-2`；跳过 `/api/`、navigate network-first、assets cache-first；`offline.html` 无初始化入口 | 无 SW 升级 E2E；CACHE 版本未随界面改版更新 |
| **CI 对最新 SHA 绿灯** | **FAIL（P1）** | `.github/workflows/ci.yml` | #12/#13 Failure（见第 2 节） |
| iPhone Safari 实机 PWA / 双服务器 HTTPS / 性能基准 | **UNVERIFIED** | `docs/deployment.md`、`deploy/*` | 无设备/服务器；LCP/CLS 未测量 |

## 5. 本轮任务的现状结论

1. **业务账务无 P0 缺陷**：80 个 Go 测试、31 条 E2E、八步数据库演练全绿，可继续前台改造（符合计划第 6 节：若发现财务 P0 才阻断）。
2. **生产初始化「前端入口」必须收口**：`App.vue` 与 `api.ts` 仍完整暴露初始化 UI/调用路径；Go 侧已有 403/401/409 与并发保护，但缺 `TINYNEST_SETUP_DISABLED` 受控开关。用户告知「生产已初始化」属环境事实，**未读取生产配置或实际 API 前不标「已现场验证」**。
3. **Landing / 动效 / PWA 更新 / 16 条新 E2E 均为真实缺口**，需按 M1→M5 增量实施，不重建项目、不重复实现账务能力。
4. **CI 红灯 P1 需在本分支修复**：清理 `91ef72e` 两份文档的 6 行尾随空格，并复验 `git diff --check origin/main...HEAD` = 0。

## 6. 路由与前端结构（M2 实施依据）

- 应用为**单页无路由库**：`App.vue` 用 `view: View` 内部状态切换 7 个驾驶舱视图；`boot()` 调 `api.me()` 决定 `authed`；`loadAll()` 仅在登录成功后请求私有数据。
- 登录/初始化是同一张 `.authcard`（`App.vue:448-465`），含 `authtabs` 两个按钮。
- `api.ts:14-32` 的 `request()` 统一 `credentials:'same-origin'`、`cache:'no-store'`，错误抛 `ApiError`；**401 目前不会触发全局登出**，会话过期需在本轮补。
- PWA：`main.ts` 仅 PROD 注册 `/sw.js`；`sw.js` 缓存公开壳，`/api/` 直接放行；`index.html` 已含 `viewport-fit=cover` 与 apple meta。
- 现有 E2E `financial-closure.spec.ts` 的 `ensureLogin()` 与场景 #01/#02 **依赖旧 UI**（"首次初始化"按钮、直接可见的用户名输入框），M2 改造必须同步更新，否则 31 条回归会断。

## 7. UNVERIFIED / 边界

- `go test -race`（缺 gcc）、本分支远端 CI（未推送）、Linux Chromium、iPhone 实机 PWA、双服务器 HTTPS 生产链路、生产令牌运维、LCP/CLS 基准：均 **UNVERIFIED**，不冒充通过。
- 本轮不修改任何账务规则、口径常量或迁移；不改原始图标字节；不推送、不合并 main、不部署、不触碰真实财务数据。
