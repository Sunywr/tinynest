# TinyNest · 小家 · 开发与本地验证

**当前基线：V2 记账重置**。保留身份/权限/家庭/邮件与复式记账内核（含信用卡账单日/还款日、只读还款计划与 pfm 导入的只读证券/理财估值快照）；信用账单、分期、负债、V1 投资交易、导入等模块已退役。唯一视觉主题是 **A · 奶油团子**（见 [设计规范](design-system.md)）。

当前进度、已退役模块与未验证项见 [当前进度](project-status.md)。V1/Phase 0 报告仅作历史归档（[V0.1 实施报告](reviews/v1-implementation-report.md)、[Phase 0 执行报告](execution-report.md)）。

## 1. 工具链与 Windows 注意

| 组件 | 要求 |
| --- | --- |
| Go | `go.mod` 声明 `go 1.22`；`go mod tidy -diff` 门禁需要 Go ≥ 1.23，CI 固定 1.23.6 |
| Node / npm | Node 22（CI 固定）；仓库用 `package-lock.json`，统一 `npm ci` |
| Python | 3.11+；仅用于 `scripts/verify-brand.py` 等辅助脚本 |
| MySQL | 仅生产方言；本地默认 SQLite。MySQL 演练需要独立 MySQL 8 实例；Windows 可用隔离 TCP 实例 + `TINYNEST_TEST_MYSQL_TCP`，见 §6 |

**Go 工具链可能不在 PATH 上**（本机常见情况）：把 Go 的 `bin` 目录临时加入 `PATH` 再执行命令，例如 PowerShell：

```powershell
$env:Path = "<Go 安装目录>\bin;$env:Path"
go version
```

Playwright 配置还会读取 `TINYNEST_GO_BIN_DIR`，在未改全局 PATH 时用它把 Go 的 `bin` 目录传给 E2E 启动的 Go 进程。

**Windows 执行策略拦截 `npm.ps1`** 时，改用 `npm.cmd`：

```powershell
npm.cmd ci
npm.cmd run dev
npm.cmd run build
npm.cmd run test:e2e
```

或仅对当前进程放行：`Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`。不要为此永久降低系统执行策略。

## 2. 快速启动（本地 SQLite）

先阅读 `apps/api/.env.example`（示例配置，勿提交真实值）。本地用 SQLite 时不需要 MySQL 与邮件密钥；`TINYNEST_COOKIE_SECURE` **必须显式**设置。

```bash
# 终端 1：Go API（启动时自动应用 V2 迁移 001–014）
cd apps/api
mkdir -p data
export TINYNEST_DATA_DIR=./data
export TINYNEST_DB_PATH=./data/tinynest.sqlite3
export TINYNEST_COOKIE_SECURE=false
# 初始化令牌仅用于首次创建家庭与管理员，由部署者生成；不要提交、不要进日志
export TINYNEST_SETUP_TOKEN="$(openssl rand -hex 32)"
go run ./cmd/server        # 默认监听 127.0.0.1:8787

# 终端 2：Vue Web
cd apps/web
npm.cmd ci
npm.cmd run dev            # http://127.0.0.1:5173，Vite 把 /api 代理到 127.0.0.1:8787
```

Windows PowerShell 也可从仓库根目录启动后端（脚本会生成并复用 Git 忽略的一次性令牌文件）：

```powershell
.\scripts\start-dev-api.ps1
```

首次使用：公开前端**没有**初始化/创建管理员入口。由运维在受控环境调用一次：

```bash
curl -fsS -X POST http://127.0.0.1:8787/api/v1/setup \
  -H 'Content-Type: application/json' \
  -H "X-Setup-Token: <与后端一致的令牌>" \
  -d '{"household_name":"我的小家","username":"admin","password":"<至少8位强口令>"}'
```

成功后初始化端点即失效（再次调用返回 409）。部署环境还应设置 `TINYNEST_SETUP_DISABLED=true` 并在重启前移除令牌。

可选邮件能力：用 `mail-admin` 生成 32 字节原始密钥文件，并以 `TINYNEST_MAIL_KEY_FILE` 注入：

```bash
cd apps/api
go run ./cmd/mail-admin -action generate-key -key-file /path/to/protected/mail-key
export TINYNEST_MAIL_KEY_FILE=/path/to/protected/mail-key
```

### 本地数据库重置

V2 不做旧库原地升级。若本地库是 V1 结构（缺少 `schema_meta.baseline=v2` 或迁移账本无 `checksum` 列），启动会**拒绝**并提示按 `docs/plans/v2-cutover-runbook.md` 处理。
本地开发正确做法：停止服务后删除本地 SQLite 文件（例如 `apps/api/data/tinynest.sqlite3`，连同 `-wal`、`-shm`），下次启动由 V2 二进制从零建立基线。不要修改 `schema_meta` 或删除迁移账本来绕过守卫。

## 3. 环境变量（后端）

| 变量 | 说明 |
| --- | --- |
| `TINYNEST_LISTEN_ADDR` | 默认 `127.0.0.1:8787` |
| `TINYNEST_DATA_DIR` | 默认 `./data` |
| `TINYNEST_DB_PATH` | 默认 `<data_dir>/tinynest.sqlite3` |
| `TINYNEST_DB_DRIVER` | `sqlite`（默认）或 `mysql` |
| `TINYNEST_MYSQL_SOCKET` / `_DATABASE` / `_USER` | MySQL 模式必填；通过本机 Unix socket 连接 |
| `TINYNEST_MYSQL_PASSWORD_FILE` | MySQL 口令文件；也支持 `CREDENTIALS_DIRECTORY/mysql-password` |
| `TINYNEST_MAIL_KEY_FILE` | 可选，32 字节邮件加密密钥；也支持 `CREDENTIALS_DIRECTORY/mail-key` |
| `TINYNEST_SETUP_TOKEN` | 一次性初始化令牌；未配置时 `POST /api/v1/setup` 返回 403 `setup_disabled` |
| `TINYNEST_SETUP_DISABLED` | 严格 `true`/`false`；`true` 时初始化端点对任何请求都返回 403 |
| `TINYNEST_COOKIE_SECURE` | **必填** `true`（生产 HTTPS）/ `false`（本地纯 HTTP）；缺失或非法拒绝启动 |
| `TINYNEST_TRUSTED_PROXIES` | 可选，逗号分隔 IP/CIDR；只有这些来源的 `X-Forwarded-*` 才被采信 |
| `TINYNEST_LOGIN_MAX_FAILURES` / `_WINDOW_SECONDS` / `_LOCKOUT_SECONDS` | 可选，登录失败限流参数；留空用内置默认值 |

SQLite 文件只在后端，`.gitignore` 已排除。`GET http://127.0.0.1:8787/api/v1/healthz` 检查进程存活；`readyz` 在 SQLite 模式执行 `PRAGMA quick_check`，在 MySQL 模式执行 ping，两种模式都校验迁移版本完整。

## 4. 工程目录（V2）

```text
tinynest/
├── AGENTS.md                    # V2 操作守则（含强制阅读与安全规则）
├── README.md                    # 产品定位与当前状态
├── IMPLEMENTATION_REPORT.md     # 指向 V1 历史报告的指针
├── assets/brand/                # 用户原始团子图标（不改绘；唯一批准 SHA-256）
├── previews/                    # Phase 0 静态原型存档（虚构金额，非当前应用）
├── apps/web/                    # Vue 3 + TS + Vite + vue-router 4；PWA / SW / 五界面
│   ├── src/App.vue              # 应用壳（路由切换、底部导航、记账面板挂载）
│   ├── src/router.ts            # 路由表（/bills、/charts、/assets、/me、/me/settings、公开页）
│   ├── src/views/               # Bills / Charts / Assets / Me / Settings / Landing / Login
│   ├── src/components/          # 日组列表、记账面板、键盘、环形图、预算、管理员弹层等
│   ├── src/composables/         # session、entry form/sheet、preferences、breakpoint、toast
│   └── src/services/            # api / format / export / accountBrand
├── apps/api/                    # Go API + 运维 CLI
│   ├── cmd/server/              # 组合根：打开数据库、迁移、HTTP 服务、邮件 worker
│   ├── cmd/tinynest-admin/      # 运维 CLI（baseline-capture / purge / verify / identity-*）
│   ├── cmd/mail-admin/          # 运维 CLI（生成密钥、站点管理员、受控改密）
│   └── internal/                # api / auth / db / dbadmin / ledger / money / platform / server
├── deploy/frontend/ · backend/  # A 机 Nginx HTTPS 同源代理 / B 机私网 Nginx + systemd 模板
├── scripts/                     # 品牌校验、自检、备份、MySQL CI 演练等辅助脚本
└── docs/                        # 开发、架构、安全、部署、V2 契约/规则、验收与历史归档
    └── plans/                   # V2 总计划、模块规格、三端优化、清理手册、切换手册
```

后端包职责：`internal/api`（HTTP 处理器与路由注册）、`internal/auth`（Argon2id 与令牌）、`internal/db`（连接、迁移、分录持久化、默认分类）、`internal/dbadmin`（清理与身份导入导出）、`internal/ledger`（复式内核与业务分录）、`internal/money`（整数分）、`internal/platform/config`（API 与 CLI 共用的连接配置）、`internal/server`（healthz/readyz 与中间件骨架）。

## 5. 数据库迁移（V2）

- 每个方言当前有 **版本 1–14（001–014）**：`001_core.sql`、`002_bookkeeping.sql`、`003_category_style.sql`、`004_credit_cycle.sql`、`005_investments.sql`、`006_auth_login_locks.sql`、`007_project_budgets.sql`、`008_transaction_source_metadata.sql`、`009_credit_statement_mailboxes.sql`、`010_credit_statement_bindings.sql`、`011_statement_mailbox_providers.sql`、`012_card_statements.sql`、`013_statement_placeholders.sql`、`014_statement_details.sql`（SQLite 在 `internal/db/migrations/`，MySQL 在 `internal/db/mysql/` 同名）。文件名前 3 位是版本号。空库迁移后 39 张表，MySQL 运行时 40 张、SQLite 41 张。
- 单一迁移器 `internal/db/migrate.go` 按版本升序应用缺失版本，记录 SHA-256 校验和，校验版本连续性与已应用版本的校验和；`schema_meta` 写入 `baseline=v2`。
- SQLite 的迁移文件与账本行在同一事务；MySQL DDL 隐式提交，因此 **MySQL 迁移文件必须可重跑**。新表用 `CREATE TABLE IF NOT EXISTS` 与内联索引；需要 `ALTER TABLE` 时用 `information_schema.columns` + `PREPARE/EXECUTE` 做幂等保护（003/004 的写法）。
- `schema_meta.baseline` 缺失/不是 `v2`、出现二进制不认识的版本、校验和不一致时启动失败，中文错误指向 [新库切换手册](plans/v2-cutover-runbook.md)。**不得绕过守卫**。
- 默认分类由 `db.DefaultCategories` + `db.SeedForHousehold()` 幂等写入；不要再引入 SQL 种子文件或启动期 `UPDATE`。
- 新增迁移只能追加新版本文件，且必须补：空库全量迁移、重复迁移 no-op、校验和篡改拒绝、baseline 不符拒绝、`PRAGMA quick_check` / `foreign_key_check` 测试。
- **生产 Linux 二进制必须使用 LF 换行的嵌入迁移文件构建**。Windows 工作树若为 CRLF，直接 `go build` 会把 CRLF 写进 `//go:embed`，导致生产 `schema_migrations` 的 checksum 校验失败（2026-11-22 部署曾复现并回退）。构建前用 `git archive` 取 LF 文件，或显式把 `internal/db/migrations/*.sql` 与 `internal/db/mysql/*.sql` 归一化为 LF；不要修改生产账本或绕过守卫。仓库 `.gitattributes` 已为这些 SQL 声明 `eol=lf`。

## 6. 验证命令

```bash
# 后端
cd apps/api
go build ./...
go test ./... -count=1
go vet ./...
go mod tidy -diff            # 退出码 0 = go.mod / go.sum 与 tidy 一致
go test -race ./... -count=1 # 需要 cgo/gcc；环境不具备时标 UNVERIFIED

# 前端
cd apps/web
npm.cmd ci
npm.cmd run typecheck
npm.cmd run build
npm.cmd run test:e2e         # 自动拉起 Go + Vite；需要 Go 可执行（或用 TINYNEST_GO_BIN_DIR）

# 仓库卫生（CI 门禁同款）
git diff --check
git grep -nI -E '[[:blank:]]+$' -- '*.md' '*.go' '*.ts' '*.vue' '*.css' '*.py' '*.sql' '*.yml' '*.yaml' '*.json' '*.html' '*.js'

# 品牌原图
python3 scripts/verify-brand.py
```

### Windows 本机隔离 MySQL（TCP）

生产仍通过 Unix socket 连接；Windows 没有 Unix socket，测试用 `mysql_windows_test.go` 的 TCP shim 把 `unix` 拨号重定向到本机 TCP 实例。隔离实例必须绑定 `127.0.0.1`，每个集成测试使用独立空库（`POST /api/v1/setup` 单次有效）：

```powershell
$env:TINYNEST_TEST_MYSQL_TCP='127.0.0.1:3307'
$env:TINYNEST_TEST_MYSQL_SOCKET='tcp'  # 占位；shim 会改用 TCP
$env:TINYNEST_TEST_MYSQL_USER='tinynest_drill'
$env:TINYNEST_TEST_MYSQL_PASSWORD_FILE='<受保护的口令文件>'
$env:TINYNEST_TEST_MYSQL_DATABASE='<每个测试独立空库>'
go test ./internal/api -run '^TestBookkeepingLifecycle$' -count=1
go test ./internal/dbadmin -run '^TestMySQLPurgeDrill$' -count=1
```

- 本机已用 MySQL 8.0.46 隔离实例验证 24 条 API 测试、2 条 dbadmin drill 与 dump/restore 对账；生产 Unix socket 与 `readyz` 仍未验证。
- 2026-11-26 另在一台临时 MySQL 8.4.7 隔离实例（临时 datadir、`127.0.0.1:3307`，口令只经 `TINYNEST_TEST_MYSQL_PASSWORD_FILE` 传入，用完即停进程并删除数据目录）跑通迁移 `001→014` 全链与 `014_statement_details.sql` 重放三次（退出码 `0 0 0`、行数不变），并复跑 `TestMySQLPurgeDrill`、`TestMySQLIdentityCutoverDrill`（各自独立空库）与 11 条 `TestStatementDetail*`（每条一个空库）；机器自带服务实例未被触碰。
- `internal/api/mysql_windows_test.go` 与 `internal/dbadmin/mysql_windows_test.go` 只在 `TINYNEST_TEST_MYSQL_TCP` 设置时注册拨号器；Linux CI 仍使用 `scripts/mysql-ci-drill.sh` 的 Unix socket 路径。

- `scripts/check-phase0.sh` / `.ps1` 名字保留历史，实际执行品牌校验、Go test/vet 与前端构建；V2 的完整门禁以 `.github/workflows/ci.yml` 为准（Go build/test/race/vet/tidy、MySQL Unix socket 与备份恢复演练、Web typecheck/build、Playwright E2E、全树行尾空白检查）。
- 本机没有 MySQL / gcc 时，`go test -race`、`scripts/mysql-ci-drill.sh` 与 MySQL drill 用例会跳过或无法执行，结论标 `UNVERIFIED`；Windows 隔离 MySQL 按上一节执行。
- `scripts/db_drill.py` 是 V0.1 时期的 SQLite 迁移/备份恢复演练脚本，V2 下未重新验证；V2 的迁移与基线行为以 `internal/db/migrate_test.go` 和清理/切换手册为准。

## 7. 本地运行 `tinynest-admin`

`tinynest-admin` 是**只能在服务器本地、服务停止时执行**的运维工具，不具备也不得暴露 HTTP 接口。它与 API 共用同一套数据库环境变量，因此不会误连其他数据库。

```bash
cd apps/api
go build -o tinynest-admin ./cmd/tinynest-admin

# SQLite 本地：显式指定驱动与数据库文件；数据库必须已由 API 完成迁移
export TINYNEST_DB_DRIVER=sqlite
export TINYNEST_DB_PATH="$PWD/data/tinynest.sqlite3"

# 只读：采集基线、dry-run 清理、校验当前状态
./tinynest-admin baseline-capture --out=./data/baseline.json
./tinynest-admin purge --dry-run
./tinynest-admin verify --baseline=./data/baseline.json
```

危险操作规则：

- 先停止 API（`purge -execute` 还要求 `-yes-service-stopped` 显式确认），否则调度/邮件 worker 会边清边写。
- `-confirm` 必须与目标数据库名完全一致；SQLite 模式下 `Database.Name()` 就是数据库文件路径，必须逐字匹配。
- MySQL 口令只通过 `TINYNEST_MYSQL_PASSWORD_FILE` 或 `CREDENTIALS_DIRECTORY/mysql-password` 提供，不接受命令行密码。
- `purge` 在单事务内清理 **56 个表名（16 组）**：当前 V2 基线中 24 张存在、32 个是 V1 遗留名（按 `Missing` 跳过并在输出里列出）；保留 11 张身份表与 2 张含加密授权码的个人配置表（`statement_mailbox_configs`、`credit_statement_mailboxes`，后者不进入身份导出）。事务内断言已分类的业务表全空、身份与保留配置计数、`users` 摘要不变后才提交；未归类的新表会让 `verify` 失败而不是静默漏清。
- 执行前先备份并做隔离恢复比对；执行后立即 `verify`；旧库与备份保留 ≥30 天。完整命令、判据与回滚见 [生产清理手册](plans/v2-prod-purge-runbook.md) 与 [新库切换手册](plans/v2-cutover-runbook.md)。
- `identity-export` 输出含用户名与邮箱（及口令哈希），权限 0600，按机密保管并限期删除，不得进入仓库。

## 8. 设计与契约文档

[V2 账务规则](accounting-rules-v2.md) · [V2 API 契约](api-v2.md) · [V2 ER](er-v2.md) · [设计规范](design-system.md) · [安全底线](security.md) · [验收清单](acceptance.md)。
V2 计划与生产手册在 `docs/plans/`；历史 V0.1 文档（`plan-v0.1.md`、`docs/reviews/v0.1-*` 等）只作归档，不再代表当前实现。
