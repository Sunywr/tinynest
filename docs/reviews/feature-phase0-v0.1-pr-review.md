# TinyNest · `feature/tinynest-phase0-v0.1` 合并前专项审查报告

> **本文件已更新**：下方**第 0 节是修复后复核结论，取代第 1 节**。第 1～8 节为 2026-11-13 首次审查的原始记录，完整保留作为证据链，其 `DRAFT_PR_ONLY` 结论已被第 0 节取代。

## 0. 修复后复核（2026-11-13，PR 阻断缺陷修复轮）

| 项目 | 内容 |
|---|---|
| 复核角色 | 高级开发工程师（吴八哥）· WorkBuddy Hy4 |
| 依据 | `tinynest-v01-pr-remediation-prompt.md`（严格执行 FIX-01～FIX-08、M4、第四组重新验收） |
| 基线 SHA | `origin/main` `69535438f3dc38c6a4226b7367cc1a3cc40e0cd2`、`origin/feature/tinynest-phase0-v0.1` `1d0e4016dd030bd4a9ac3f8c6304d1bcf88a23dc`，**两分支 SHA 与首次审查时一致、未变** |
| 工作树状态 | 首次审查报告写入 `main` 工作树且未提交；切分支前已把 3 个未提交文件备份到仓库外 `D:/Code/tinynest-uncommitted-backup-20261113-1223/`。实际修复全部在独立 Worktree `D:/Code/tinynest-pr-fix`（本地 `feature/tinynest-phase0-v0.1` 跟踪 origin 同名分支）内完成，**`main` 源码未改动**，改动均为 Worktree 内本地未提交状态 |
| 复核边界 | 未 merge、未 push、未创建或合并 PR、未部署生产、未访问或修改真实 SQLite / 财务数据 |

### 0.1 复核结论

| 判定 | 首次审查 | 修复后复核 |
|---|---|---|
| 合并至 `main` 的工程门禁 | `DRAFT_PR_ONLY` | **`MERGE_READY`**（仅作为「V0.1 开发基线」合并，附条件，见 0.4） |
| 上公网 / 录入真实家庭数据 | 否 | **否（NO）** |

**理由**：P1-01～P1-06 六项阻断缺陷已按「失败测试 → 修复 → 回归」逐项修复并有 17 个定向用例 PASS；前后端构建、vet、tidy、前端三命令全绿；隔离库 18 项演练证实 7 版本迁移、幂等重跑与备份恢复；无敏感数据、无破坏性迁移；文档已真实反映未实现功能。CI 工作流已建立但 GitHub Actions 真实绿灯仍 `CI_UNVERIFIED`，按修复 Prompt 允许列为待办。

### 0.2 P1 逐项复核

| 编号 | 首次审查问题 | 复核结果 | 证据 |
|---|---|---|---|
| P1-01 | `/setup` 未授权抢注 admin | **已修复** | 一次性 `X-Setup-Token`、常量时间比较、未配置 fail closed 403、初始化后 409、并发单次 201；4 个 setup 测试 + 真实二进制端到端 PASS |
| P1-02 | 缺 `household_id` 隔离 | **已修复** | `db.go:246/261/279` 聚合 SQL 下推；`credit.go:533/555` 到期任务按父合同过滤且 `biz_key` 家庭作用域；2 个隔离测试 PASS |
| P1-03 | 业务日期 UTC 错期 | **已修复** | `helpers.go:99/112/115` 可注入 Clock + UTC+8 业务日；期初、冲正、定时任务、默认区间全部改用；6 个边界子用例 PASS |
| P1-04 | TLS 终止后 Cookie 无 Secure | **已修复** | `helpers.go:183` `secureCookie` 不依赖 `r.TLS`，只采信 `TINYNEST_TRUSTED_PROXIES`；服务启动强制显式 `TINYNEST_COOKIE_SECURE`；4 个 Cookie 测试 PASS |
| P1-05 | 登录无限流 | **已修复** | 新增 `login_limiter.go`（两级 bucket、有界冷却、429 + `Retry-After`、并发 TOCTOU 修复、审计脱敏）；3 个限流测试 PASS |
| P1-06 | 文档与实现冲突 | **已修复** | `AGENTS.md`、`plan.md`、`DEVELOPMENT.md`、`architecture.md`、`api-v0.1.md`（逐路由状态）、`README.md`、`IMPLEMENTATION_REPORT.md`、`security.md`、`deployment.md`、`agent-prompt.md`、`acceptance.md`、`migrations/README.md` 全部同步 |

### 0.3 P2 与 M 清单复核

| 编号 | 结果 |
|---|---|
| P2-01 `go.mod`/`go.sum` | **PASS**：`go mod tidy`，`go mod tidy -diff` exit 0 |
| P2-02 8 处尾随空白 | **PASS**：删除重复计划中的一份，权威版 4 处改列表写法；最终 tree 尾随空白 0，`git diff --check origin/main` exit 0 |
| P2-03 重复计划 | **PASS**：`cmp` 确认字节相同（各 43392 B）后删除 `.plan/tinynest-v0.1-implementation-plan.md`，引用改指 `docs/plan-v0.1.md` |
| P2-04 本地过程记录 | **PASS**：`git rm --cached .workbuddy/memory/2026-11-12.md`（磁盘文件保留），`.gitignore` 新增 `.workbuddy/` |
| P2-05 无 CI | **PARTIAL**：`.github/workflows/ci.yml` 已建立并静态校验（PyYAML + 7 条必需命令 + `git diff --check` 范围检查）；**GitHub Actions 真实绿灯 `CI_UNVERIFIED`** |
| P2-06 systemd 模板 | **PASS**：V0.1 描述、`TINYNEST_DB_PATH`、`EnvironmentFile` 与安全/限流变量声明 |
| P2-07 `latestMigration = 7` | **PASS**：删除常量，`db.EmbeddedVersions()` 从 embed FS 派生，`readyz` 逐版本校验；`seedMigration()` 去掉 `007_seed.sql` 硬编码 |
| P2-08 `init_sqlite.py` 误导 | **PASS**：docstring / 输出改述，脚本名保留历史并加说明 |
| P2-09 误导性测试名 | **PASS**：改名并补齐 setup 鉴权断言 |
| M1/M2/M3/M5/M6/M7/M8/M10/M12 | **PASS / PARTIAL**（M10 见 P2-05）；逐项证据见 `docs/reviews/v0.1-pr-remediation-report.md` 第 2～9 节 |
| M4 10 项口径 | **按 Prompt 要求保持 `PENDING_USER_CONFIRMATION`（0 确认 / 10 待确认）**，台账见 `docs/reviews/v0.1-accounting-decisions.md`；**不假装确认** |
| M9 S3～S7 / M11 实机 HTTPS E2E | **仍未实现 / UNVERIFIED**，如实披露，未因本轮安全修复宣称完成 |

### 0.4 复核门禁口径差异（需复核者决策）

首次审查把 M4（口径确认）、M9（S3～S7 补齐）、M11（实机/HTTPS/E2E）列为**合并前阻断**；本次修复 Prompt 的最终判定标准把这三类明确划归 `PRODUCTION_READY`，`MERGE_READY` 只要求「P0/P1 安全与隔离缺陷修复并测试通过、构建绿、无破坏性迁移、无敏感数据、文档真实反映未实现功能」，并允许「CI 未执行则按实际策略列待办」。

- 按用户「严格执行修复 Prompt」的指令，本复核采用修复 Prompt 的划分 → **`MERGE_READY`（工程门禁）**，且明确标注为「V0.1 开发基线」，**不是**生产发布。
- **若复核时决定沿用首次审查更严的 M4/M9/M11 合并阻断口径，结论应回落为 `DRAFT_PR_ONLY`。**
- 无论采用哪套口径，**可上线并录入真实家庭数据均为 `NO`**。

### 0.5 仍然存在的缺口（复核后如实保留）

- `CI_UNVERIFIED`：CI 文件已建立，真实绿灯需 PR 创建后由 GitHub Actions 产出。
- `UNVERIFIED`：`go test -race`（本机缺 gcc/cgo）、iPhone Safari 实机 PWA、A→B 双服务器 HTTPS 私网链路、前端 E2E、390/768/1440 真实浏览器视觉回归。
- `NOT_IMPLEMENTED`：S3 账单状态自动流转/额度、S4 房贷保险 HTTP API、S5 费率规则写入 API、S6 独立统计端点、S7 每日自动调度器与审计查询端点、统一 `Idempotency-Key`。
- 10 项账务口径 0 确认 / 10 待确认；正式启用真实数据前必须取得用户确认。
- 已知问题与运维待办见 `IMPLEMENTATION_REPORT.md` 第 9 节、`docs/reviews/v0.1-release-gates.md` 门禁 B。

### 0.6 复核证据文件

| 文件 | 内容 |
|---|---|
| `docs/reviews/v0.1-pr-remediation-report.md` | 缺陷编号、根因、修复文件:行号、RED→GREEN 证据、完整回归矩阵 |
| `docs/reviews/v0.1-accounting-decisions.md` | 10 项口径逐项：当前代码行为 / 代码位置 / 推荐值 / 影响 / `PENDING_USER_CONFIRMATION` |
| `docs/reviews/v0.1-release-gates.md` | 门禁 A（合并 main）与门禁 B（生产发布）逐项状态 |
| `docs/reviews/v0.1-pr-description.md` | Draft PR 标题、摘要、剩余缺口、测试证据、待确认事项 |
| `docs/acceptance.md` | V0.1 可执行验收清单 |

> 复核结论仅代表本地工程门禁评估。**未执行 merge / push / PR 创建或合并 / 生产部署**，等待用户复核。

---

## 首次审查元信息（历史存档，2026-11-13）

| 项目 | 内容 |
|---|---|
| 审查日期 | 2026-11-13（GMT+8） |
| 审查角色 | 高级开发工程师（吴八哥）· PR 合并门禁审查 |
| 审查依据 | `.plan/tinynest-pr-merge-gate-prompt.md`（完整执行第 1～4 节） |
| 目标分支 | `main` |
| 审查方式 | **只读**。用 `git archive` 导出 feature 分支到系统临时目录做隔离验证，未检出分支、未改动 `main` 工作树源码 |
| 审查边界 | 未执行 `merge` / `rebase` / `reset --hard` / `clean -fd` / `push`；未创建或合并 PR；未部署真实服务器；未修改任何真实 SQLite 数据 |
| 报告落盘位置 | `docs/reviews/feature-phase0-v0.1-pr-review.md`（首次审查时写入 `main` 工作树，**未提交、未推送**；本轮修复已把副本复制进独立 Worktree `D:/Code/tinynest-pr-fix` 并追加第 0 节复核结论，两处均未提交） |

> 本报告只记录有命令输出、文件行号或可复现实验支撑的结论。所有凭据类值一律脱敏为 `<redacted>`。

---

## 1. 结论（首次审查历史记录，已被第 0 节取代）

### `DRAFT_PR_ONLY`（历史结论，已失效） —— 首次审查时判定「不可以直接合并到 `main`」

一句话理由：前后端构建、账务内核单测、7 版本迁移与备份恢复演练均实测通过，且无凭据泄露、无合并冲突；但 `/api/v1/setup` 未认证即可抢注管理员、多家庭数据隔离缺失、10 项账务口径尚未确认、S3～S7 多项核心模块未实现，且 iPhone 实机 / 双服务器 HTTPS / 前端 E2E 为 `UNVERIFIED`，不满足 `MERGE_READY` 的「无 P0/P1 阻断 + 主要业务路径完整」条件。

### 四选一判定对照

| 结论 | 是否成立 | 依据 |
|---|---|---|
| `MERGE_READY` | 否 | 存在 6 项 P1；S3/S4/S5/S6/S7 未完整实现；实机与 HTTPS 关键检查为 `UNVERIFIED`。门禁明确「关键测试不可运行不可标记 MERGE_READY」 |
| `DRAFT_PR_ONLY` | **是（首次审查结论，已被第 0 节取代）** | 精确匹配门禁定义「核心模块未完整实现」「存在非紧急 P1 问题」；同时差异可展示、构建与账务核心测试通过，适合 Draft PR 讨论 |
| `BLOCKED` | 条件性 | 无已证实凭据泄露、无严重错账、迁移可恢复，故不整体判 BLOCKED。**但若按门禁第 3 节「越权访问」字面判定 P1-01（未认证创建 admin 并可永久锁死合法所有者）为 P0，或本次 PR 计划立即上公网 / 录入真实财务数据，则必须升级为 `BLOCKED`**，先修复再谈合并 |
| `UNVERIFIED` | 部分项 | 作为整体结论不适用（关键构建/账务测试已跑通）；但 iPhone PWA 实机、双服务器 HTTPS 私网链路、前端 E2E、CI 状态、真实浏览器视觉回归逐项标记 `UNVERIFIED` |

### 关于范围的澄清（非缺陷，但必须在 PR 中说明）

本分支**并非纯 Phase 0**：5 个提交中包含 `feat(ledger): implement V0.1 double-entry kernel, SQLite migrations and API` 与 `feat(web): replace mock demo with real ledger UI`，实际交付了完整 V0.1 复式记账主体（48 文件、+10354 行）。而 `AGENTS.md` 第 12 行仍写「仅实现 Phase 0。禁止账号/授权、流水 CRUD、报表真实计算」，`plan.md` 第 3 节「拒绝过度开发」同样冻结在 Phase 0。

审查认定：这是**范围规范未随代码更新**，不是「违规开发」——`README.md` 已诚实标注「V0.1 主体已实现 · 发布门禁尚未全部通过」「暂不建议录入真实家庭财务数据」，`IMPLEMENTATION_REPORT.md` 第 8/9 节逐项列明未完成与待确认项，**不存在以完整 V0.1 冒充发布的问题**。但规范文件冲突会误导后续维护者，列作 P1-06。

---

## 2. 比较基线

| 项 | 值 |
|---|---|
| `origin/main` | `69535438f3dc38c6a4226b7367cc1a3cc40e0cd2`（`更新README`） |
| `origin/feature/tinynest-phase0-v0.1` | `1d0e4016dd030bd4a9ac3f8c6304d1bcf88a23dc`（`docs: 添加2026-11-12的Phase 0验收和V0.1实施记录`） |
| merge-base | `69535438f3dc38c6a4226b7367cc1a3cc40e0cd2`（等于 main 头） |
| 提交数 | feature 领先 **5**，main 领先 **0**（线性可快进，本分支不落后 main） |
| `git diff --stat` | **48 files changed, 10354 insertions(+), 103 deletions(-)** |
| `git diff --name-status` | 37 个 `A` + 11 个 `M`，**0 个 `D`**（无误删文件） |
| `git merge-tree --write-tree` | exit 0，输出树 `4b21be7d472c6cd30729a6535203d543ab86ed53` → **无合并冲突** |
| `git diff --check` | **exit 2**，8 处 trailing whitespace，全部位于两份内容重复的计划文档第 3～6 行 |
| 大体积/冗余产物 | `.plan/tinynest-v0.1-implementation-plan.md` 与 `docs/plan-v0.1.md` **SHA-256 完全相同**（`bd0e16da...`），各 1031 行重复入库；`package-lock.json` 1513 行属正常；无 `dist/`、`node_modules/`、数据库或构建产物入库 |

### 5 个提交（新→旧）

```text
1d0e401 docs: 添加2026-11-12的Phase 0验收和V0.1实施记录
6ef72d6 docs: add V0.1 design baselines, plans and implementation report
54da1bd feat(web): replace mock demo with real ledger UI
5b939ae feat(ledger): implement V0.1 double-entry kernel, SQLite migrations and API
dc7b7b6 fix(phase0): unblock Vue production build and refresh PWA metadata
```

### 受影响模块（真实差异，非仅看文档）

`apps/api/internal/{money,ledger,invest,credit,db,auth,api,server}`、`apps/api/cmd/server`、`apps/api/go.mod`/`go.sum`、`apps/web/src/{App.vue,services/api.ts}`、`apps/web/{package-lock.json,vite.config.ts,index.html}`、PWA `manifest.webmanifest`/`sw.js`、`README.md`、`docs/{architecture,execution-report,DEVELOPMENT,plan-v0.1,accounting-rules-v0.1,api-v0.1,er-v0.1}.md`、`IMPLEMENTATION_REPORT.md`、`.workbuddy/memory/2026-11-12.md`。

---

## 3. 功能完成矩阵

标记含义：`PASS` 实测通过 / `PARTIAL` 部分实现 / `UI_ONLY` 仅界面 / `UNVERIFIED` 环境不具备、无法验证 / `MISSING` 明确未实现。

| 范围 | 标记 | 源文件 / 证据 |
|---|---|---|
| **Phase 0 工程基础**（品牌、构建、PWA 骨架、双服务器模板） | **PASS** | `scripts/verify-brand.py` exit 0（原图与 Web 副本 SHA-256 一致）；`npm ci`+`typecheck`+`build` 全绿；manifest 192/512/180 图标尺寸实测正确；`sw.js` 显式排除 `/api/`，仅缓存公开壳 |
| **S0 设计基线 / 账务口径** | **PARTIAL** | `docs/accounting-rules-v0.1.md`、`docs/plan-v0.1.md`、`docs/er-v0.1.md`、`docs/api-v0.1.md` 齐备；但 `IMPLEMENTATION_REPORT.md:243` 明确 **10 项口径「待确认」**，代码按建议默认值实现，不得视为正式业务规则 |
| **S1 分类 / 项目 / 账户 / 期初余额** | **PASS** | `setup.go:150-422`；`api_test.go:136-148` 期初 1,000,000 分余额由分录重建成功；HTTP 冒烟真实进程建家庭后默认分类 21 个 |
| **S2 交易与复式记账内核** | **PASS** | `money/money.go`（int64 分、big.Int 防溢出、无浮点）、`ledger/{entry,rules,subjects}.go`（借贷平衡、冲正不删原分录、拆分和=总额）、`transactions.go`；`rules_test.go` 用例 1-14/18-23/26/27/29 全通过；`api_test.go` 覆盖拆分不匹配时业务行+分录同时回滚、冲正恢复余额 |
| **S3 信用账单** | **PARTIAL** | `credit.go`（账单自动归集、部分还款、对账）、`credit/cycle.go`（月末钳制）；`IMPLEMENTATION_REPORT.md:230` 列明未实现：账单状态自动流转（unbilled/billed/overdue）、额度已用/可用/使用率、`convert-to-installment` HTTP 端点 |
| **S4 退款 / 分期 / 债务** | **PARTIAL** | `transactions.go:359-479`（部分退款、超退拒绝）、`credit.go:400-514`（分期期次幂等生成、付款）；`IMPLEMENTATION_REPORT.md:231` 列明未实现：房贷/贷款/保险 HTTP CRUD 与期次付款、提前结清、分期退款后期次调整 |
| **S5 投资** | **PARTIAL** | `invest/cost.go`（移动加权平均、big.Int 比例、清仓无残差、红利税 NULL=`unknown`）、`investments.go`；`cost_test.go` 用例 18-23 通过；`IMPLEMENTATION_REPORT.md:232` 列明未实现：费率规则写入 API、无份额金额模式 |
| **S6 首页三套口径** | **PASS（带隔离缺陷）** | `dashboard.go:30-135` 收支 / 现金流 / 到期负债分离，排除投资损益与期初注资；`api_test.go:317-329` 断言 income=800000 且 cashflow≠expense。**但 5 项聚合缺 household 过滤，见 P1-02** |
| **S7 发布门禁 / 运维 / 安全加固** | **MISSING** | `IMPLEMENTATION_REPORT.md:234` 列明：登录失败限流、真正的每日调度器（当前仅手动 `POST /schedule/generate`）、审计全覆盖、备份自动演练、`docs/release-v0.1.md` 均未实现 |
| **认证 / CSRF / 会话 / 审计** | **PARTIAL** | `auth/auth.go`（Argon2id、常量时间比较、token 仅存 SHA-256）、`helpers.go:81-107`（requireAuth + 写操作 CSRF）、`api_test.go:345-368`（401 / 403 通过）。缺 setup 保护、限流、Secure Cookie，见 P1-01/P1-04/P1-05 |
| **前端真实 API 界面** | **PASS（构建）/ UNVERIFIED（交互）** | `App.vue`（真实 API 表单、无 mock 数据、`App.vue:538` 明确标注未开发能力且不伪造保存成功）、`services/api.ts`（仅相对 `/api/v1/*`、`no-store`、整数分格式化）；无 E2E，真实浏览器 390/768/1440 视觉未实测 |
| **数据库迁移 / 备份恢复** | **PASS** | 空库 7 版本 → 33 表；已有库重跑不丢数据；真实 Go 生成库备份→隔离恢复 `quick_check=ok`、`foreign_key_check=[]`（第 4 节） |
| **PWA 实机 / 双服务器 HTTPS** | **UNVERIFIED** | manifest/SW/图标/模板静态检查通过；无 iPhone、无 A/B 双机，`deploy/*` 全为 `YOUR_*` 占位符 |
| **CI / GitHub Actions** | **MISSING** | `git ls-tree` 确认 feature 分支**无任何 `.github/workflows`**；`gh` CLI 未安装，故无 CI 检查状态可查 |
| **无真实记账业务（Phase 0 约束）** | **N/A（范围已变更）** | 该约束属于已被 V0.1 取代的 Phase 0 规范；README/实现报告已如实披露，不计缺陷 |

---

## 4. 实际测试结果

审查在隔离快照 `C:\Users\lenovo\AppData\Local\Temp\tinynest-review-UuuR5i\feature`（88 文件）与独立临时数据库上执行。环境：`go 1.25.2 windows/amd64`、`node v22.22.2`、`npm 10.9.7`、`Python 3.13.14`、`git 2.55.0`；`sqlite3` CLI 缺失（用 Python `sqlite3` 模块替代）；`gh` 未安装。

| # | 命令 / 操作 | 退出码 | 结果 |
|---|---|---|---|
| 1 | `git status --short` | 0 | 工作区仅 `?? .plan/tinynest-pr-merge-gate-prompt.md`，无已修改跟踪文件 |
| 2 | `git fetch origin --prune` | 0 | 远程跟踪引用更新，未覆盖工作树文件 |
| 3 | `git rev-parse` / `git log` / `diff --stat` / `--name-status` | 0 | 见第 2 节；5 提交、48 文件、无删除 |
| 4 | `git diff --check origin/main...origin/feature` | **2** | 8 处尾随空白（两份 plan 文档第 3-6 行） |
| 5 | `git merge-tree --write-tree origin/main origin/feature` | 0 | 无冲突 |
| 6 | `git log --oneline origin/feature..origin/main` / `rev-list --count` | 0 | main 领先 0，feature 领先 5 |
| 7 | `npm ci --no-audit --no-fund` | 0 | **全新安装 50 包，未用缓存** |
| 8 | `npm run typecheck`（`vue-tsc --noEmit`） | 0 | 类型检查 0 错误 |
| 9 | `npm run build`（`vue-tsc --noEmit && vite build`） | 0 | 15 模块；`dist/index.html` 1.16 kB、CSS 15.14 kB、JS 112.34 kB，597 ms |
| 10 | `go build ./...`（默认 `proxy.golang.org`） | **1** | **环境障碍**：`golang.org/x/crypto@v0.31.0`、`modernc.org/sqlite@v1.34.5` 下载返回 `502 Bad Gateway` |
| 11 | `go test ./... -count=1`（同上） | **1** | 5 个纯算法包 `ok`（credit/invest/ledger/money/server），4 个依赖包 `setup failed` |
| 12 | `go vet ./...`（同上） | **1** | 同上网络原因 |
| 13 | **换可信镜像 `GOPROXY=https://goproxy.cn,direct` 重跑** `go build ./...` | **0** | 10 个依赖下载完成，编译通过 |
| 14 | `go test ./... -count=1`（镜像重跑） | **0** | 9 包全部 `ok`（api / credit / invest / ledger / money / server 有测试；cmd/server / auth / db 无测试文件） |
| 15 | `go test ./... -count=1 -v` 用例统计 | 0 | 逐项 PASS：`TestCase01_CashExpense`、`TestCase02_Income`、`TestCase03_Transfer`、`TestCase04_29_Splits`、`TestCase05to08_CreditAndRefund`、`TestCase10_CreditPaysMortgage`、`TestCase11to14_Installment`、`TestCase18_ConvertToInstallment`、`TestInvestmentPostings`、`TestCase26_Reversal`、`TestKernelRejectsInvalidEntries`、`TestCase18to21_MovingAverageCost`、`TestCase22_Dividend`、`TestCase23_Valuation`、`TestSellGuards`、`TestParseYuanAndFormat`、`TestDivRoundHalfUp`、`TestMulBPSAndMinimumFee`、`TestAddOverflow`、`TestCycleRollingToNextMonth`、`TestInstallmentDueDatesClampEveryMonth`、`TestHealth`、`TestReadyDirectory`、`TestMethod`、`TestV01EndToEnd`、`TestSetupIsSingleUseAndRequiresAuth`、`TestMigrationsAreIdempotent`；**0 FAIL** |
| 16 | `go vet ./...`（镜像重跑） | 0 | 无告警 |
| 17 | `go mod tidy -diff`（镜像重跑） | **1** | **依赖清单不一致**：`go.mod` 把 `golang.org/x/crypto`、`modernc.org/sqlite` 两个直接依赖误标 `// indirect`；`go.sum` 缺 20 条测试依赖校验和（pprof / x/mod / x/tools / cc/v4 / ccgo/v4 / fileutil / gc/v2 / opt / sortutil / strutil / token 等），并多出 `golang.org/x/sys v0.22.0` 条目 |
| 18 | `python scripts/verify-brand.py` | 0 | 两份原图 SHA-256 校验通过（与 `assets/brand/README.md` 批准值一致） |
| 19 | PWA 静态检查 | 0 | `manifest.webmanifest` 合法；`standalone` / `start_url=/` / `scope=/` / 名称正确；图标实测 192x192、512x512、180x180、32x32；`sw.js` 对 `/api/` 直接 return，仅缓存公开壳；响应式断点 900px / 560px |
| 20 | 迁移演练 A（Python `executescript` 逐文件执行 7 个迁移） | 0 | 空库 → **33 张表**，`schema_migrations` 版本 `[1,2,3,4,5,6,7]`；在已有 1 家庭/1 用户的库上重跑全部迁移，用户数 1→1、家庭数 1→1 **数据保留**，seed 幂等生成 21 分类 |
| 21 | 备份恢复演练 A | 0 | `scripts/backup-sqlite.py` exit 0（`sqlite3.Connection.backup()` + `quick_check` + chmod 0600）；隔离恢复库 `quick_check=ok`、用户 1、家庭名完整、33 表 → **PASS** |
| 22 | **真实 Go 二进制启动 + HTTP 冒烟**（临时库、`127.0.0.1:18787`） | 0 | 启动日志 `applied_migrations="[1 2 3 4 5 6 7]"` → 证实 **Go 侧 `Migrate` 多语句执行成功、空库全量迁移可用**；`healthz 200 {"phase":"v0.1"}`；`readyz 200 {"check":"sqlite_quick_check_and_migrations"}` |
| 23 | 备份恢复演练 B（对第 22 步真实 Go 生成的 WAL 库） | 0 | 源库 7 迁移/1 家庭/1 用户/21 分类/`quick_check=ok`；备份脚本 exit 0；恢复库 `quick_check=ok`、7/1/1/21 全部一致、`foreign_key_check=[]` → **PASS** |
| 24 | 安全扫描：`git log --all --name-only` + `git ls-tree` + `git grep` | 0 | **全历史无** `.env` / `.pem` / `.key` / `.p12` / `.sqlite3` / `.db` / 证书 / 凭据 / 私人流水文件；无硬编码密钥、无云厂商 token、无默认管理员口令；前端源码无真实域名/IP/数据库路径（仅 package-lock 的 npm registry URL） |
| 25 | `git grep` 家庭隔离/口令模式 + SQL 逐条核对 | 0 | 绝大多数查询带 `household_id=?`；发现 3 处缺失（P1-02） |
| 26 | CI 检查 | — | `git ls-tree` 无任何 `.github/workflows`；`gh` 未安装 → `MISSING / UNVERIFIED` |

**未验证项（明确 `UNVERIFIED`，不得宣称通过）**：iPhone Safari「添加到主屏幕」实机、独立窗口/启动画面/安全区；生产 HTTPS 与 A→B 私网 TLS 链路实测；前端 E2E（无 Vitest/Playwright）；真实浏览器 390/768/1440px 视觉；大数据量分页/性能压测；GitHub Actions 检查状态。

---

## 5. P0 / P1 缺陷

### P1-01（可升级 P0）`/api/v1/setup` 未认证即可抢注管理员并永久锁死合法所有者

- **文件/行号**：`apps/api/internal/api/setup.go:17`（`mux.HandleFunc("POST /api/v1/setup", a.handleSetup)`，**未套 `a.route`/`requireAuth`**）、`setup.go:39-86`、`setup.go:61-66`（仅检查 `COUNT(*) FROM households > 0`）；反证：`docs/api-v0.1.md:18` 声称「除 `/auth/session`、`/healthz`、`/readyz` 外均需认证」，且全文**未把 `/setup` 记为公开端点**。
- **重现步骤**（第 4 节第 22 步真实进程）：
  1. 启动服务（空库），不带任何 Cookie、不带 CSRF：`POST /api/v1/setup` + `{"household_name":"<redacted>","username":"<redacted>","password":"<redacted>"}`；
  2. 再以合法所有者身份发同样请求。
- **预期**：首次初始化应受保护（一次性引导令牌 / 仅 loopback 或私网白名单 / 安装后自动禁用）；未授权调用返回 401/403。
- **实际**：第一次 `HTTP/1.1 201 Created`，返回 `household_id` 与 `user_id`（admin 已建立）；第二次 `HTTP 409`。`api_test.go:44-49` 亦在**未登录**状态下调用 `/setup` 并期望 201，构成代码级铁证。
- **影响**：任何能访问 `/api` 的人可在管理员之前抢先建立 admin 账号，随后登录即可读写全部家庭财务数据；合法所有者被 409 永久挡住，普通用户无法自助恢复，只能手工改库。公开仓库 + 公开 `docs/api-v0.1.md` 使接口可被枚举；且登录无失败限流（P1-05）放大风险。
- **最小修复**：① 引入一次性 `TINYNEST_SETUP_TOKEN`（环境变量，启动时生成并只打印一次）并在 `handleSetup` 校验；或 ② 限制 `/setup` 仅 `127.0.0.1` / 私网来源；③ 初始化成功后彻底禁用端点（不只是 409）；④ 在 `docs/api-v0.1.md` 如实登记公开端点清单。
- **等级说明**：设计上属「首次安装引导」，且 README 已禁止生产部署，故本轮判 P1、整体结论 `DRAFT_PR_ONLY`；**若计划公网部署或录入真实数据，必须按门禁第 3 节「越权访问」升级 P0 → `BLOCKED`**。

### P1-02 多家庭数据隔离缺失：dashboard 五项核心指标与定时投影为全库汇总

- **文件/行号**：
  - `apps/api/internal/db/db.go:186-195` `Balance()`、`db.go:198-209` `Aggregate()`、`db.go:214-226` `Cashflow()` —— 三条 SQL 只按 `account_code LIKE ?` / 日期过滤，**未 JOIN `journal_entries.household_id`，也无 `e.household_id=?`**。
  - `apps/api/internal/api/dashboard.go:40-68` —— `income_minor`、`expense_minor`、`cashflow_minor`、`assets`、`liabilities`（进而 `net_worth_minor`）全部经上述三个函数计算；同文件 `:72-99` 的其他查询反而都带 `household_id=?`，口径不一致。
  - `apps/api/internal/api/credit.go:527-528` —— `handleGenerateDueItems` 的 `SELECT ... FROM installment_schedule_items/debt_schedule_items WHERE status='pending' AND due_on<=?` 无 household 过滤，插入 `schedule_due_items` 时却写入 `p.HouseholdID`（`credit.go:549-551`）。
- **重现步骤**：在同一库中构造两个家庭各自的分录/期次，分别登录调用 `GET /api/v1/dashboard/summary` 与 `POST /api/v1/schedule/generate`，比对返回值。
- **预期**：各家庭只看到自己的收入/支出/现金流/净资产；定时任务只投影本家庭期次。
- **实际**：LIKE 前缀（如 `4010:%`、`1%`、`2%`）会匹配所有家庭的分录，汇总为全库数值；定时任务会把别家期次投影为自家 pending 项，并因 `biz_key` 全局唯一而令对方后续无法生成。
- **影响**：当前 `handleSetup` 强制单家庭（`COUNT(*)>0` 即 409），**单家庭下不会真的串账**，但隔离边界在代码层不存在；`IMPLEMENTATION_REPORT.md:235` 与 `.workbuddy/memory/2026-11-12.md` 均自认「多家庭隔离未实现」。一旦支持多家庭、数据导入或存量库合并，立即产生跨户数据泄露与错账。
- **最小修复**：给 `Aggregate` / `Cashflow` / `Balance` 增加 `householdID` 参数，SQL 改为 `JOIN journal_entries e ... WHERE e.household_id=?`，过滤下推到 SQL（**不要**全表捞出后在应用层过滤）；`handleGenerateDueItems` 的查询按 `contract_id IN (SELECT id FROM ... WHERE household_id=?)` 收窄；补两家庭并存的回归测试。

### P1-03 账务口径 10 项待确认 + 期初余额业务日使用 UTC 导致跨日错期

- **文件/行号**：`IMPLEMENTATION_REPORT.md:243`（10 项口径：成员登录范围、现金账户范围、账单周期、分期确认时点、手续费分摊、移动加权平均成本法、手工估值、房贷每期拆分、收益率口径、退款跨月）；`IMPLEMENTATION_REPORT.md:246`（已知问题 1）；`setup.go:385`（`OccurredOn: todayUTC()`）、`helpers.go:51`（`todayUTC()` 取 UTC 日期）对比 `dashboard.go:17`（业务日固定 `Asia/Shanghai`，UTC+8）。
- **预期**：所有业务日期按 Asia/Shanghai；正式账务规则须经用户确认后固化为回归用例。
- **实际**：在 UTC+8 的 00:00–08:00 之间创建带期初余额的账户，分录日期会比业务日早一天，可能落入上一会计期间，影响当期现金流/净资产；口径按建议默认值实现但**不得视为已确认**。
- **影响**：附件 2.2 要求「首页收支、实际现金流、本期应还、投资收益和净资产必须来自真实明细」——数值来源正确且有测试，但**期间归属与口径合法性未闭环**，不能宣称账务正确性达标。
- **最小修复**：统一业务日期函数（复用 `shanghaiToday()`），期初/冲正/退款日期全部改用它；把 10 项口径逐条确认并写入 `docs/accounting-rules-v0.1.md` 后补边界用例（跨日、跨月、退款跨月）。

### P1-04 TLS 终止场景下会话 Cookie 缺少 `Secure`

- **文件/行号**：`setup.go:122-126`（`Secure: r.TLS != nil`）；`deploy/frontend/nginx.conf.example` 与 `deploy/backend/nginx.conf.example`（A 机 443 终止 TLS 后以 **`proxy_pass http://127.0.0.1:8787`** 明文转发到 Go）。
- **重现步骤**：第 4 节第 22 步真实进程登录后读取 `Set-Cookie`。
- **预期**：`HttpOnly; Secure; SameSite=Lax`（生产 HTTPS）。
- **实际**：`Set-Cookie: tn_session=<redacted>; Path=/; Expires=...; HttpOnly; SameSite=Lax` —— **无 `Secure`**。因 Go 侧收到的始终是 HTTP，`r.TLS` 恒为 nil；Nginx 虽设置 `X-Forwarded-Proto https`，Go 代码未读取该头。
- **影响**：会话 Cookie 在协议降级/中间人场景可被携带，配合同源代理的公开入口放大会话劫持风险；与 `docs/architecture.md`「HttpOnly/Secure/SameSite 会话」描述不符。
- **最小修复**：依据可信代理设置的 `X-Forwarded-Proto`（并限制只信任 A 机私网 IP）决定 `Secure`，或生产环境固定 `Secure: true`；补断言 `Set-Cookie` 属性的集成测试。

### P1-05 登录无失败限流 / 无锁定

- **文件/行号**：`setup.go:88-128` `handleLogin`（仅校验用户名/口令与 Argon2id，无失败计数、无延迟、无锁定）；`IMPLEMENTATION_REPORT.md:234` 自认 S7「登录失败限流」未实现；`docs/api-v0.1.md:27` 却写「登录，建会话；失败限流」（文档承诺未兑现）。
- **影响**：可无限次在线猜测口令；与 P1-01 叠加后，抢注与暴力破解组合风险显著。
- **最小修复**：按 `username`+来源 IP 做失败计数与指数退避/临时锁定，记录审计事件；补「连续 N 次失败后返回 429」的自动化测试。

### P1-06 范围规范与 API 契约和真实代码冲突

- `AGENTS.md:12` / `plan.md:58` 仍写「仅实现 Phase 0，禁止账号/授权、流水 CRUD、报表真实计算」，而代码已实现完整账本——后续 Agent/维护者按规范会得出错误结论。
- `docs/DEVELOPMENT.md:5` 写「当前只有界面演示与基础设施骨架…没有登录、权限…不可用于真正记账」，同文件末尾又写「V0.1 真实账本（当前分支）前端已连接真实后端，不再展示虚构金额」，**同一文档自相矛盾**。
- `docs/architecture.md:26` 仍写 `apps/api：Go stdlib，仅 healthz/readyz`，而同文件 `:28` 新增段落已说明 V0.1 接入 SQLite 与完整 API。
- `docs/api-v0.1.md` 作为「API 契约」列出大量未实现端点（`/household/current`、`/household/members`、`PATCH /categories`、`/projects/{id}` 等）、宣称「所有写操作可带 Idempotency-Key」「登录失败限流」，与实现不符（`IMPLEMENTATION_REPORT.md:235` 自认部分写接口未接幂等键）。
- **最小修复**：在合并前一次性同步 `AGENTS.md`/`plan.md`/`DEVELOPMENT.md`/`architecture.md` 到 V0.1 现状（或明确标注「Phase 0 历史规范，已被 V0.1 计划取代」），并给 `docs/api-v0.1.md` 增加「已实现 / 计划中」状态列。

### P2（非阻断，合并前建议清理）

| 编号 | 问题 | 位置 / 证据 |
|---|---|---|
| P2-01 | `go.mod`/`go.sum` 与 `go mod tidy` 不一致，两个直接依赖误标 `// indirect`，go.sum 缺 20 条校验和 | `go mod tidy -diff` exit 1（第 4 节第 17 步） |
| P2-02 | `git diff --check` 8 处尾随空白 | 两份 plan 文档 `:3-6`，exit 2 |
| P2-03 | 两份计划文档字节完全重复（各 1031 行，SHA-256 相同） | `.plan/tinynest-v0.1-implementation-plan.md` 与 `docs/plan-v0.1.md` |
| P2-04 | 本地工作区过程记录进入公开 PR（含本机路径、镜像、开发过程；无真实密钥） | `.workbuddy/memory/2026-11-12.md`（+13 行） |
| P2-05 | 无 CI 工作流，合并门禁不可自动复现 | `git ls-tree` 无 `.github/`；`gh` 未安装 |
| P2-06 | systemd 模板仍标 `TinyNest Phase 0 Go API`，且未声明 `TINYNEST_DB_PATH` | `deploy/backend/tinynest-api.service.example:3` |
| P2-07 | `latestMigration = 7` 硬编码，与迁移文件数量耦合，新增迁移易漏改导致 `readyz` 误判 | `apps/api/cmd/server/main.go:18` |
| P2-08 | `scripts/init_sqlite.py` 仍为 Phase 0 版（仅建 `schema_migrations`，打印「no business tables」），与 V0.1 的 7 版本迁移并存易误导 | `scripts/init_sqlite.py`；`check-phase0.sh` 名称亦停留在 Phase 0 |
| P2-09 | `TestSetupIsSingleUseAndRequiresAuth` 名称声称验证 setup 鉴权，实际只测重复 setup(409) 与未登录读(401)，**未覆盖 P1-01** | `api_test.go:345-368` |

---

## 6. 合并风险

| 类别 | 评估 |
|---|---|
| **数据库迁移** | **低**。7 个 `CREATE TABLE IF NOT EXISTS` + `INSERT OR IGNORE` 逐版本单事务，`Migrate` 幂等；实测空库全量、已有库重跑不丢数据、真实二进制启动日志 `[1..7]`；无破坏性 `DROP`/`ALTER`，回滚路径可用备份恢复。**但无迁移回滚脚本**，且 `latestMigration` 硬编码（P2-07） |
| **备份 / 恢复** | **低（已实测补证）**。`backup-sqlite.py` 使用 SQLite 在线 backup API + `quick_check` + chmod 0600；本报告在两套隔离库（Python 生成 / 真实 Go WAL 库）完成备份→恢复→数据一致性比对，`foreign_key_check=[]`。原 `IMPLEMENTATION_REPORT.md:219` 记录的「未做恢复演练」本轮已由审查者补做，但**仓库内仍无可自动执行的恢复演练脚本**（P2 / S7 待办） |
| **兼容性** | **低**。main 领先 0，merge-base 即 main 头，`merge-tree` exit 0，线性快进无冲突；无删除文件；`package-lock.json` 与 `go.sum` 随代码入库（但后者不干净，P2-01） |
| **数据安全** | **中高**。无凭据泄露、无敏感文件入库；但 P1-01 未授权初始化、P1-02 隔离缺失、P1-04 Cookie 无 Secure、P1-05 无限流。SQL 全部参数绑定（`?` 占位），`decode` 限制 1 MiB，`writeErr` 不泄露 SQL/路径/堆栈；`writeJSON` 强制 `no-store`；`sw.js` 与 Nginx 均排除 `/api` 缓存；Go 仅监听 `127.0.0.1`，Nginx 模板含 `allow/deny` 与 `proxy_ssl_verify on` —— 静态安全设计整体扎实，风险集中在上述 4 项 |
| **前后端部署** | **中**。模板同源 `/api/` 代理路径正确（A 机 `/api/` → B 机 8443 → Go 8787，原样转发），SPA `try_files ... /index.html`、`sw.js`/`index.html` no-cache 齐备；但全为 `YOUR_*` 占位符、未上线，A→B 私网 TLS 与 HTTPS 实机连通性 `UNVERIFIED`；`deploy/backend/*.example` 的 `proxy_set_header X-Forwarded-Proto https` 未被 Go 用于 Cookie 判定（P1-04） |
| **PWA** | **中**。manifest/SW/图标/主题色/apple-touch-icon/viewport 静态检查全通过，SW 明确不缓存 `/api`；但 iPhone Safari 实机「添加到主屏幕」、独立窗口、启动画面、安全区 `UNVERIFIED`，按门禁不得宣称通过 |
| **PR 质量** | **中**。README 的「V0.1 主体已实现 · 发布门禁尚未全部通过」与实际范围一致，未冒充完整发布，符合门禁要求；但规范文档冲突（P1-06）、重复文档与尾随空白（P2）、内部记忆入库（P2-04）降低可维护性 |

---

## 7. 合并前必修清单（逐项可验证、可勾选）

**阻断项（全部完成前不合并）**

- [ ] **M1** 为 `/api/v1/setup` 增加保护（一次性令牌 / 仅 loopback+私网 / 初始化后禁用三选一或组合），并更新 `docs/api-v0.1.md` 公开端点清单；验证：未授权 `POST /setup` 返回 401/403，持令牌首次 201，二次 409/404。
- [ ] **M2** 为 `db.Aggregate` / `db.Cashflow` / `db.Balance` 增加 `householdID` 参数并把过滤下推到 SQL；收窄 `handleGenerateDueItems` 的期次查询；验证：新建两家庭并存测试，`/dashboard/summary` 与 `/schedule/generate` 互不串数据。
- [ ] **M3** 统一业务日期为 Asia/Shanghai（期初/冲正/退款不再用 `todayUTC()`）；验证：跨 UTC 日边界的期初余额落在正确业务期间。
- [ ] **M4** 确认 `docs/accounting-rules-v0.1.md` 第 8 节 10 项口径，并把结论固化为回归测试；验证：口径文档标注「已确认」，对应测试用例存在且通过。
- [ ] **M5** 同步 `AGENTS.md`、`plan.md`、`docs/DEVELOPMENT.md`、`docs/architecture.md`、`docs/api-v0.1.md` 到 V0.1 现状（或明确标注历史规范）；验证：全文不再出现「仅 healthz/readyz」「没有登录」等与代码冲突的表述。
- [ ] **M6** 在 PR 描述中明确标注「工程基础 + V0.1 主体，发布门禁未通过，不得录入真实财务数据、不得部署生产」；验证：PR 描述含该声明且标题不含「V0.1 正式发布」。

**发布门禁项（升级为可合并前补齐；若继续作为 V0.2 待办，须在 PR 中显式列出）**

- [ ] **M7** 登录失败限流与锁定；验证：连续失败返回 429 的自动化测试。
- [ ] **M8** 修复生产会话 Cookie `Secure`（信任 `X-Forwarded-Proto` 或固定开启）；验证：`Set-Cookie` 属性断言测试。
- [ ] **M9** 补齐 S3/S4/S5 未实现端点，或明确列入 V0.2 并在 PR 清单披露；验证：`docs/api-v0.1.md` 状态列与实现一致。
- [ ] **M10** 引入 CI（至少 `go test ./... && go vet ./... && go mod tidy -diff` + `npm ci && npm run build`）；验证：GitHub Actions 对 PR 有绿色检查。
- [ ] **M11** 完成 iPhone PWA 实机、双服务器 HTTPS 私网链路、前端 E2E 与 390/768/1440 视觉实测，并附证据；验证：实现报告「未测试」清单逐项勾销。
- [ ] **M12** 清理：`go mod tidy`（消除 `go.mod`/`go.sum` diff）、`git diff --check` 归零、删除重复的 `.plan/tinynest-v0.1-implementation-plan.md`、把 `.workbuddy/memory/` 排除出 PR、更新 systemd 模板描述。

**建议合并方式**：修复 M1–M6 后以 **Draft PR** 展示，待 M7–M12 至少 M10/M11 完成后再 `squash merge` 或常规合并到 `main`；保持 main 可直接构建。

---

## 8. PR 建议

**建议标题**

```text
[Draft] feat(ledger): V0.1 家庭账本主体（工程基础，发布门禁未通过，暂不合并）
```

> 明确是**工程基础 + 部分 V0.1 功能**，不是「V0.1 正式发布」。禁止使用「V0.1 发布完成 / 可直接用于真实记账」类表述。

**建议描述（可直接粘贴）**

```markdown
## 本次变更
在 Phase 0 骨架上交付 V0.1 家庭账本主体：
- 复式记账内核（整数分、借贷平衡、冲正不删原分录）、7 版本 SQLite 迁移与真实持久化
- 登录/会话/CSRF/审计、账户与分类、收支与转账、退款、信用账单、分期、投资、首页三套口径
- Vue 前端改为全真实 API 界面；PWA manifest / SW / 图标更新

## 状态声明（重要）
本 PR 是**工程基础 + V0.1 主体实现**，**不是 V0.1 正式发布**：
- 发布门禁未通过：iPhone 实机、双服务器 HTTPS 私网链路、前端 E2E、真实浏览器视觉回归均未验证
- 未完成：S3 账单状态流转/额度、S4 房贷保险 API、S5 费率写入 API、S7 登录限流/每日调度器/审计全覆盖
- 10 项账务口径待用户确认；多家庭隔离未实现（当前强制单家庭）
- **暂不建议录入真实家庭财务数据，暂不部署生产**

完整审查：docs/reviews/feature-phase0-v0.1-pr-review.md
实施与未竟事项：IMPLEMENTATION_REPORT.md 第 7/8/9 节

## 已验证
go build / go test ./... / go vet（GOPROXY=goproxy.cn）、npm ci / typecheck / build、
空库 7 迁移、已有库重跑不丢数据、备份→隔离恢复一致性、真实进程 healthz/readyz。

## 合并前 checklist
见 docs/reviews/feature-phase0-v0.1-pr-review.md 第 7 节 M1–M12。
```

**PR checklist（贴入 PR）**

- [ ] 明确标注「仅工程基础 / 部分 V0.1」，未冒充正式发布
- [ ] `/setup` 已加保护且文档如实登记公开端点
- [ ] 所有财务查询强制带 `household_id`
- [ ] 业务日期统一 Asia/Shanghai，10 项口径已确认
- [ ] `AGENTS.md` / `plan.md` / `DEVELOPMENT.md` / `architecture.md` / `api-v0.1.md` 与代码一致
- [ ] `go test ./... && go vet ./... && go mod tidy -diff` 全绿
- [ ] `npm ci && npm run typecheck && npm run build` 全绿
- [ ] `git diff --check` 无告警；无重复文档与内部记忆文件
- [ ] CI 工作流已接入并对本 PR 通过
- [ ] 备份恢复演练在隔离库通过（本次审查已实测，需沉淀为脚本）
- [ ] iPhone PWA 实机 / HTTPS 双服务器 / E2E 已完成或明确列入 V0.2
- [ ] 未合并、未推送、未部署（本轮审查已完成只读边界确认）

---

## 9. 下一步：优先修复 3 个问题及对应自动化测试

| 优先级 | 问题 | 最小修复 | 对应自动化测试（建议新增） |
|---|---|---|---|
| **1** | **P1-01 `/setup` 未授权抢注** | 一次性 `TINYNEST_SETUP_TOKEN` + 私网来源限制 + 初始化后禁用端点 | `api` 包新增 `TestSetupRequiresBootstrapToken`：无令牌 → 401/403；错误令牌 → 403；正确令牌首次 → 201、二次 → 409/404；并断言 `users` 表仅 1 条 admin。同步修正 `TestSetupIsSingleUseAndRequiresAuth` 的命名与覆盖 |
| **2** | **P1-02 家庭数据隔离缺失** | `Aggregate`/`Cashflow`/`Balance` 增加 `householdID` 并在 SQL 层 JOIN `journal_entries` 过滤；`handleGenerateDueItems` 按本家庭期次收窄 | 新增 `TestHouseholdIsolation`：在同一 SQLite 库插入两个家庭各自的分录与分期期次，分别以两套会话调用 `/dashboard/summary`（断言 income/expense/cashflow/net_worth 各自独立）与 `/schedule/generate`（断言只生成自家 pending 且 `biz_key` 不互相占用）；补一条 SQL 断言「聚合查询计划含 `household_id` 过滤」 |
| **3** | **P1-03 业务日期与口径未闭环** | 复用 `shanghaiToday()` 替换 `todayUTC()`；确认 10 项口径并写入设计文档 | 新增 `TestCase30_BusinessDateBoundary`：把时钟注入/固定到 UTC 00:00–08:00 区间创建带期初余额账户，断言分录 `occurred_on` 等于 Asia/Shanghai 当日、且不落入上期汇总；为 10 项口径各补 1 个边界用例（退款跨月、分期手续费按期、账单跨月钳制等） |

补充建议：把上述测试与 `go vet`、`go mod tidy -diff`、`npm run build` 一起接入 GitHub Actions（M10），使后续 PR 的门禁结论不再依赖人工取证。

---

## 附录：审查执行边界声明

- 仅执行只读 git 查询、`git archive` 导出到系统临时目录、隔离目录内的构建/测试、临时端口 `127.0.0.1:18787` 的本地 HTTP 冒烟、隔离临时 SQLite 库的备份恢复演练。
- 未执行 `merge` / `rebase` / `reset --hard` / `clean -fd` / `push`；未创建或合并 PR；未修改 `main` 分支任何源码；未部署到真实服务器；未改动任何真实 SQLite 数据；未修改远程分支。
- 审查前工作区仅有未跟踪的 `.plan/tinynest-pr-merge-gate-prompt.md`；审查后新增本报告文件（未提交）。`git fetch origin --prune` 仅更新远程跟踪引用，未覆盖工作树文件。
- 报告中出现的所有会话令牌、CSRF 令牌、演练口令均已脱敏。
