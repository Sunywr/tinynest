# TinyNest · 2026-11-14 最新源码现场审计（Hy4 第二轮）

| 项 | 实测值 |
|---|---|
| 仓库 | `git@github.com:Sunywr/tinynest.git`（`https://github.com/Sunywr/tinynest.git`） |
| 审计基线 | `origin/main` = `6fbe857628cb6b7640b6ad9110ce95f80fdcde2b`（2026-11-14 15:53 +0800，Merge pull request #2） |
| 本轮工作分支 | `feature/tinynest-v0.1.3-preflight`，从 `origin/main` 派生，起点同 `6fbe857` |
| 本地 `main` | `f6fe9b8`（落后 `origin/main` 3 个提交；**本轮未切换、未快进、未合并**） |
| 审计时 HEAD | `6fbe857`；`git merge-base HEAD origin/main` = `6fbe857` |
| 工作树 | `git status --porcelain=v1` 仅 `?? docs/plans/2026-11-14/`（用户提供的 4 份输入文档，原样保留、未纳入提交） |
| 执行日期 | 2026-11-14 16:02–16:12 GMT+8 |
| 测试环境 | Windows / Go `1.23.6`（便携版，**不在 PATH**）/ Node `22.22.2` / npm `10.9.7` / Python `3.13.14` / 系统 Microsoft Edge |
| 数据 | 全部独立临时 SQLite；未读取或修改任何真实家庭财务数据 |

## 1. Git 事实（实测，不采信旧报告）

- `git fetch --all --prune`：删除已失效的 `origin/feature/tinynest-v0.1.2-fee-and-networth` 跟踪引用；`f6fe9b8..6fbe857 main -> origin/main`。
- 远程仅剩 `origin/main`；本地 `feature/tinynest-v0.1.1-financial-closure`=`9db47bd`、`codex/tinynest-release-validation`=`347263e`（独立 worktree）、`feature/tinynest-phase0-v0.1`=`df2a9b3` 均为历史分支，成果已并入 main。
- PR #2（`https://github.com/Sunywr/tinynest/pull/2`）状态 **Merged**，合并 2 个提交（`6d3c439` V0.1.2 实现、`b11793b` 文档）到 main，合并提交 `6fbe857`。
- 本轮读取的源码 = `origin/main` 的树；`git diff --stat HEAD origin/main` 为空，确认工作区代码与最新基线一致。

## 2. GitHub Actions 真实结果（只读访问运行页，本机 `gh` 未安装）

| 运行 | 事件 / 提交 | 结论 | 证据 |
|---|---|---|---|
| CI #9 run `43569504212` | push `f6fe9b8`（合并前 main） | **Success**，4 job 全绿，1m10s | Actions 运行页 |
| CI #10 run `43573074289` | push `b11793b`（PR #2 分支） | **Failure**，48s；`Repository hygiene` 退出码 2 | 运行页注解 `#step:3:29`，其余 Go/Web/E2E job 成功 |
| CI #11 run `43573131991` | pull_request #2，head `b11793b` | **Failure**，1m1s；`Repository hygiene` 退出码 2 | 运行页注解 `#step:3:31`，Go API job `succeeded ... 21s` |
| 合并提交 `6fbe857` | push main | **无可见运行记录**（Actions workflow 列表最新仍为 #9） | workflow 页 9 runs |

**根因（本机复现）**：`.github/workflows/ci.yml:95` 的 `git diff --check on this PR's changes` 检查 PR 分支相对 base 的差异。`b11793b` 新增的 `docs/plans/TINYNEST-PROGRESS-AND-NEXT-STEPS.md` 第 3/4/5/31/34/44 行含 Markdown 硬换行尾随空格，本机 `git diff --check f6fe9b8...b11793b` 与 `git diff --check 6d3c439..b11793b` 均输出 6 条 `trailing whitespace` 并 **exit 2**，与远端一致。这不是应用代码缺陷，但属于**已被合并的 CI 红灯（P1 工程门禁缺陷）**，本轮修复。

> CI #9 的成功不能替代最新 main `6fbe857` 的绿灯；本轮分支未推送，任何远端验证仍需用户授权。

## 3. 本机实跑（全部真实退出码）

| 命令 | 退出码 | 结果 |
|---|---|---|
| `go build ./...` | 0 | — |
| `go test ./... -count=1` | 0 | 74 个 `Test*`；6 个包全绿（api 6.666s 等） |
| `go vet ./...` | 0 | — |
| `go mod tidy -diff` | 0 | `.gitattributes` 固定 `go.sum` LF |
| `npm run typecheck` | 0 | `vue-tsc --noEmit` |
| `npm run build` | 0 | vite 6.4.6，15 modules，JS 143.45 kB / CSS 15.44 kB |
| `npm run test:e2e` | 0 | **25/25 PASS（44.2s）**，Edge + 真实 Go API + 独立 SQLite，含 390/768/1440 视口 |
| `python scripts/db_drill.py --api-dir apps/api` | 0 | 空库迁移 `[1..9]`、重启余额 50000、在线备份→新库恢复、`quick_check=ok`、`foreign_key_check=0` |
| `python scripts/verify-brand.py` | 0 | 原图与 Web 副本 SHA-256 一致，**原始图标未改** |
| `python -m json.tool apps/web/public/manifest.webmanifest` | 0 | PWA 清单 JSON 合法 |
| `git diff --check`（工作树） | 0 | 工作树无尾随空白 |
| `git diff --check f6fe9b8...b11793b` | **2** | 复现 CI 红灯（见第 2 节） |
| 敏感文件 / `.workbuddy` 扫描 | 0 | `git ls-files` 无 `.env`/`.db`/`.sqlite3`/`data|backups`/agent notes |

Go 需 `GOPROXY=https://goproxy.cn,direct`（默认 `proxy.golang.org` 下载 `go-sql-driver/mysql` 返回 Bad Gateway）；E2E 需 `unset HTTP_PROXY*` + `NO_PROXY=127.0.0.1,localhost`（环境代理会拦截本地就绪探测，表现为 Vite 60s 超时）。两者均为本机环境限制，非代码缺陷。

## 4. 逐模块状态（详见 `v0.1.1-acceptance-matrix.md`）

| 编号 | 模块 | 判定 |
|---|---|---|
| A0 | 工程安全（token / Cookie / 代理 / 限流 / 隔离 / 敏感扫描） | **PASS**（CI hygiene 单独列为 D1） |
| A1 | 收支、账户、项目、退款冲正 | **PASS** |
| A2 | 信用账单（出账/逾期/额度/还款/核对） | **PASS** |
| A3 | 分期与固定负债（房贷/保险/期次/拆分/冲正） | **PASS** |
| A4 | 到期调度（自动/补扫/幂等/隔离） | **PASS** |
| A5 | 首页三套口径 + 下钻 | **PASS** |
| A6 | 投资（费率/成本/买卖/分红/估值/收益） | **PARTIAL** —— 核心闭环 PASS，但缺分红历史列表与年度汇总（D2） |
| A7 | PWA / E2E（25 条 + 三视口） | **PARTIAL** —— 自动化 PASS；iPhone 实机 UNVERIFIED；手动清单尚有未覆盖项（D5） |
| A8 | 发布可靠性（迁移/备份恢复/生产配置/CI） | **PARTIAL** —— 迁移与备份恢复 PASS；**CI 红灯 D1**；HTTPS 双机/生产令牌 UNVERIFIED |

**V0.1.1 判定：`V0.1.1_DEV_READY`（PASS）** —— 门禁 A 所有关键账务路径有真实测试，P0=0/P1=0（D1 属 CI 工程门禁，不属账务错账）。
**V0.1.2 判定：核心投资与报表已实现并合并（PR #2），但存在 D2–D4 细项缺口。**

## 5. 路线判定：**路径 C（验收加固）为主，附带最小缺口补丁**

依据用户三条选择规则：V0.1.1 已通过完整验收（第 4 节 + CI #9 + 74 测试 + 25 E2E），V0.1.2 的投资费率、持仓成本、买卖收益、分红与家庭财务统计**已在 `6d3c439` 实现并合并**，因此不重开路径 A/B 的业务开发。本轮：
- **不重复开发**：复式内核、信用/分期/负债、调度、移动加权成本、费率自动计费与快照、首页三口径、奶油团子主题与原始图标。
- **只补真实缺口/缺陷**：D1 CI 红灯、D2 分红历史列表（只读端点）、D3 收益率口径说明与断言、D4 估值过期提示、D5 浏览器 E2E 覆盖、D6 备份恢复演练增强、手动验收清单逐项核对。

## 6. 缺陷清单（审计阶段）

| 级别 | 编号 | 问题 | 证据 |
|---|---|---|---|
| P1 | D1 | PR #2 两次 CI `Repository hygiene` 红灯已合并 main | run `43573074289` / `43573131991`；本机 `git diff --check` exit 2 |
| P1 | D2 | `GET /investments/dividends` 缺失，分红无法按历史/年度核对 | `investments.go:22` 仅 POST；`IMPLEMENTATION_REPORT.md:235` S6 未实现项 |
| P2 | D3 | 参考收益率 28.28% 无专项断言，前端未写明分母「累计含费买入成本」 | `invest/cost.go:148`、`App.vue` 持仓行 |
| P2 | D4 | 估值过期未标注「估值待更新」 | `investments.go:308` 仅 `last_valued_on` |
| P2 | D5 | 手动清单若干项无自动化证据（断网报错、匿名抢注、超额退款、5 号→次月 4 号、转账不计收支、PWA 静态） | `MANUAL-ACCEPTANCE-CHECKLIST.md` |
| P2 | D6 | `db_drill.py` 未覆盖「已有库升级重跑」与「恢复后业务汇总比对」 | `scripts/db_drill.py` 六步 |
| P2 | D7 | 文档口径冲突：`accounting-rules-v0.1.md` 第 8 节 / `AGENTS.md:15,34` / `IMPLEMENTATION_REPORT.md:247` 仍写「10 项待确认」，权威台账 `v0.1-accounting-decisions.md` 已记录 2026-11-14 用户确认 1–10 全部推荐值（10 `CONFIRMED`） | 三份文档原文对照 |

D7 **不擅自改写设计基线/规范原文**，只在报告中如实列出冲突并提请用户决策（符合「不得把按默认值实现写成用户已确认」，也符合「用户已明确确认的应标 CONFIRMED」）。

## 7. 10 项账务口径（如实）

| 来源 | 状态 |
|---|---|
| `docs/reviews/v0.1-accounting-decisions.md`（权威台账，头部记录 2026-11-14 用户回复「1–10 按推荐值」） | **10 项 `CONFIRMED` / 0 待确认** |
| `docs/accounting-rules-v0.1.md` 第 8 节、`AGENTS.md`、`IMPLEMENTATION_REPORT.md` 第 9 节 | 仍写「待确认」，属 D7 文档不一致 |

本轮**不修改任何账务规则、分录逻辑或口径常量**。

## 8. UNVERIFIED / 环境不具备（不得标 PASS）

- `go test -race`：本机缺 gcc/cgo。
- 本轮分支的 GitHub Actions 远端运行：未推送。
- 合并提交 `6fbe857` 的 CI：无可见运行记录。
- iPhone Safari 主屏 PWA 实机、双服务器 HTTPS 正式链路、生产令牌启用/轮换、公网切换：无设备/服务器，本轮不部署。
- 性能/压力基准：无基准套件。
- 正式备份定时任务与异地副本、季度恢复演练：未验。

**结论**：`AUDIT_COMPLETE` + `V0.1.1_DEV_READY`；`V0.1.2_DEV_READY` 需修复 D1/D2 并复验；`PRODUCTION_READY` = **NO**。
