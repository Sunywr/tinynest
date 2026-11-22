# V2 基线证据（Phase 0）

采集时间：2026-11-22（Asia/Shanghai）
工作树：`C:/Users/Raython/Codes/tinynest/.agents/worktrees/v2-bookkeeping-reset`
基线提交：`f9bf038`（`main`）→ 工作分支 `v2-bookkeeping-reset`
新增提交：`d300a4e`（运维清理 CLI，本阶段唯一代码改动）

## 1. 工具链

| 组件 | 版本 / 位置 | 状态 |
| --- | --- | --- |
| Go | go1.23.6 windows/amd64，便携版 `%TEMP%\tinynest-go\go\bin\go.exe`（不在 PATH） | 实测 |
| `GOPROXY` | 需设为 `https://goproxy.cn,direct`，否则拉取 MySQL 驱动失败 | 实测 |
| Node | v24.20.0 | 实测 |
| npm | 11.19.0；`npm` 是 `.ps1` 且被执行策略拦截，须用 `npm.cmd` 或 `Set-ExecutionPolicy -Scope Process Bypass` | 实测 |

## 2. 改动前基线（V1 代码）

| 项目 | 数值 |
| --- | --- |
| Git 跟踪文件数 | 348 |
| Go 测试函数 | 189（本次新增 `internal/dbadmin` 9 个后为 194） |
| Playwright 用例 | 88 |
| 后端路由 | 138 + 2 健康检查 = 140 |
| 数据库表 | 55（SQLite 迁移 21 个 / MySQL 版本 14 个） |
| 前端最大文件 | `apps/web/src/App.vue` 3,041 行 |

## 3. 已执行的验证

| 命令 | 结果 |
| --- | --- |
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `go test ./... -count=1` | 全绿（api 17.4s、credit 2.4s、ledger 4.4s、money 1.3s、server 2.5s、dbadmin 5.1s、invest 0.05s） |
| `go test ./internal/dbadmin/... -count=1` | 9/9 通过 |
| `git diff --check` | exit 0 |

## 4. 本阶段未验证（UNVERIFIED）

- 前端 `npm ci` / `npm run typecheck` / `npm run build` 与 88 条 E2E：本轮由前端重建任务覆盖，基线数值未单独复跑。
- 生产库 `schema_migrations` 实际版本、`households` 计数：需要 B 机侧采集，见 `docs/plans/v2-prod-purge-runbook.md`。
- 生产 MySQL 是否强制 `mysql/008_credit_mail.sql` 的列内联 `REFERENCES`：未连接生产库核对。
- iPhone 实机、A↔B 私网 TLS、生产性能样本：均未验证。

## 5. 主工作区说明

用户主工作区 `C:/Users/Raython/Codes/tinynest` 存在未提交改动（`apps/api/internal/api/stock_watch.go`、`cash_forecast.go`、`internal/db/migrations/021_stock_watch.sql`、`internal/db/mysql/014_stock_watch.sql`、视口图像等），与本计划「删除所有模块」冲突。本阶段按计划从已提交的 `main` 新建工作树开展，**未触碰主工作区**；该批改动的去留由用户决定后再合并。

## 6. 生产数据清理工具验收（本阶段交付）

新增 `apps/api/{cmd/tinynest-admin,internal/dbadmin,internal/platform/config}`：

- 44 张业务表按子表→父表顺序在单个事务内 DELETE，不关闭外键检查；
- 事务内断言「业务表全空 + 身份表计数不变 + users 摘要不变」后才提交；
- `-confirm` 必须与数据库名完全一致，`-execute` 必须同时确认服务已停止；
- `identity-export` / `identity-import` 支持 V2 新库切换。
- 单测夹具写满全部 44 张业务表与 11 张身份表，覆盖：清空、身份保留、dry-run 不变、多家庭拒绝、借贷不平衡拒绝、保留审计、重播种分类、导出导入往返、清后新增业务行被 verify 捕获。

## 7. P5 结果（V2 文档对齐阶段，2026-11-22）

### 7.1 最终清点数字（当前工作树，可逐项复核）

| 项 | 结果 | 复核位置 |
| --- | --- | --- |
| 业务路由 | 55 条（`Register` 内，按方法 + 路径）；`/api/v1/healthz`、`/api/v1/readyz` 2 条另计 | `internal/api/setup.go`；`TestRouteBudget` 断言 ≤55 |
| 数据库表 | 25 张业务表（`001_core.sql` 21 张 + `002_bookkeeping.sql` 4 张）；运行时 26 张（含 `schema_migrations`） | 两个方言的迁移文件 |
| 迁移版本 | SQLite 与 MySQL 各 2 个文件 | `internal/db/migrate.go` 嵌入清单 |
| 前端应用壳 | `App.vue` 98 行（非空行 88） | `apps/web/src/App.vue` |
| 前端最大非样式文件 | 276 行（`views/BillsView.vue`） | `apps/web/src/` |
| 后端非测试代码 | 35 文件、7,887 行（目标 ≤7,000 行，未达标） | `apps/api/`，排除 `_test.go` |
| Go 测试 | 101 个 `Test` 函数、21 个含测试的文件（22 个 `*_test.go`；其中 2 个 MySQL 演练用例在无 MySQL 时 skip） | `apps/api/**/*_test.go` |
| Playwright | 72 条用例（`financial-closure` 51 + `public-landing` 21） | `apps/web/e2e/` |
| 清理白名单 | 45 张表（42 张 v1 业务表 + 3 张成员档案/邀请），保留 11 张身份表 | `internal/dbadmin/tables.go` |

口径差异说明：计划稿写 `App.vue` 88 行（非空行口径），实际文件 98 行 = 非空行 88 + 空行 10；计划稿 P2 的 Go 测试口径为 99 个/21 文件，最终清点为 100 个 `Test` 函数（21 个含测试的文件，22 个 `*_test.go`），差异来自 P2 之后新增的测试。

**更正**：§6 记录的「44 张业务表」为当时笔误。P5 复核 `PurgeTableSet()` 实际返回 **45** 张（`PurgeGroups` 13 组去重后 45 张；`deleteOrder` 亦为 45 项），以 45 为准。

### 7.2 本阶段文档交付

- 新建：`docs/api-v2.md`（55 条路由契约）、`docs/er-v2.md`（25 张表与冲正不变量）、`docs/accounting-rules-v2.md`（分录规则与统计口径）。
- 重写/更新：`AGENTS.md`、`README.md`、`docs/project-status.md`、`docs/DEVELOPMENT.md`、`docs/design-system.md`、`docs/architecture.md`、`docs/deployment.md`、`docs/security.md`、`docs/acceptance.md`。
- 归档：`IMPLEMENTATION_REPORT.md` 内容移入 `docs/reviews/v1-implementation-report.md`，根目录仅留指针。
- 退役：删除 `docs/api-v0.1.md`、`docs/er-v0.1.md`、`docs/accounting-rules-v0.1.md`；当前文档中的链接已改指 V2 文档，V0.1 历史报告中的旧链接保持历史原样。

### 7.3 仍未验证（UNVERIFIED）

- MySQL 运行时链路：本机未启动 MySQL 跑完整 API；`scripts/mysql-ci-drill.sh` 与 2 个 MySQL drill 用例未在本环境执行。
- 生产清理执行（`baseline-capture` / `purge --execute` / `verify`）与生产新库切换（`identity-export` / `identity-import`）未执行。
- iPhone 实机 PWA、A↔B 私网 TLS、生产性能样本未验证。
- 生产备份恢复与异地副本校验未执行。

### 7.4 最终复跑（文档对齐之后，主代理执行）

| 命令 | 结果 |
| --- | --- |
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `go test ./... -count=1` | 全绿（api 3.73s、db 0.40s、dbadmin 0.49s、ledger 0.05s、money 0.05s、server 0.06s） |
| `go mod tidy -diff` | exit 0（无输出） |
| `go test -race ./...` | **未执行**：本机 Windows 无 cgo/gcc，`-race` 需 `CGO_ENABLED=1`；由 CI（ubuntu-latest）覆盖，UNVERIFIED |
| `npm run typecheck` | exit 0 |
| `npm run build` | exit 0（`dist/assets/index-*.js` 617.73 kB / gzip 204.27 kB；>500 kB 提示来自既有 `xlsx` 依赖） |
| `npx playwright test` | **72/72 通过**（financial-closure 51 + public-landing 21，约 1 分钟，串行单 worker，真实 Go + SQLite 后端） |
| `python scripts/verify-brand.py` | exit 0（两张品牌原图哈希未变） |
| `git diff --check` | exit 0；改动文件行尾空白扫描 0 处 |

本轮 E2E 发现并修复的 5 处问题（均为实现或断言与规格不一致，修复后复跑全绿）：

1. 记账面板在无历史偏好时默认选中第一个分类——会静默把真实支出记到错误分类；改为「未记录过就不预选」，完成键在未选分类时保持禁用。
2. 更正流水用例把「删除末位 + 输入 9」当成 9.09（实际为 7.09），改为清空后完整输入。
3. 搜索按钮定位与流水行 accessible name 的子串冲突，改为 `exact: true`。
4. 成员/分类面板在保存成功后由父组件关闭，导致新增行不可见；改为保存后保持面板打开并刷新列表。
5. 信用卡口径文案断言与实现措辞不一致（「仅记录欠款」→「只记录欠款」），按实现修正断言。

另：`MailSettings.vue` 移除了已退役的家庭公告表单与 `/household/mail/announcements` 调用，避免调用不存在的接口。

### 7.5 V1 模式清理演练（本机，真实 V1 schema）

生产库目前仍是 V1（`GET /api/v1/readyz` 返回 `mysql_ping_and_migrations`、`phase=v0.1`），而 `tinynest-admin` 是在 V2 代码库中构建的。为确认它能在 V1 schema 上安全运行，本机做了一次完整演练：

1. 从 `f9bf038` 建临时工作树，用旧代码的嵌入迁移生成真实的 V1 SQLite 库（迁移版本 1–20）。
2. 灌入覆盖各模块的合成数据：身份表 11 张 + 业务表 38 行（含自引用 `journal_entries.reversal_of`、`categories.parent_id`、`refunds`、`credit_payments`、`installment_payments`、`debt_payments`、`investment_*`、`schedule_due_items` 等）。
3. 用**新构建的** `tinynest-admin` 依次执行 `baseline-capture` → `purge -dry-run` → `purge -execute` → `verify -baseline`。

结果：

| 步骤 | 结果 |
| --- | --- |
| `baseline-capture` | 读取 V1 迁移版本 `[1..20]`、借贷平衡、11 张身份表计数，写出基线 JSON |
| `purge -dry-run` | 待删 41 行；自动跳过 V1 不存在的 `budgets`、`stock_watch`、`stock_quotes` 并明确列出 |
| `purge -execute` | 单事务提交成功，`✓ 清理完成并已提交` |
| `verify -baseline` | `✓ 校验通过：业务表全空，身份与权限数据与 baseline 一致` |
| 独立复核（python sqlite3） | 业务行剩余 0，`journal_lines` 0 行，借贷合计 0；身份表 11 张计数不变 |

结论：清理工具可在**当前 V1 生产 schema** 上直接运行，无需先升级数据库；V2 新增表会自动跳过。生产执行仍缺 B 机 shell/MySQL 访问，未执行，标 UNVERIFIED。

