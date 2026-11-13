# TinyNest · 小家 · 开发与本地验证

**已确认视觉主题：A · 奶油团子**（见 `docs/design-system.md`）。当前仓库处于 **V0.1 家庭账本开发基线**：`apps/web` 已连接真实后端 API，可完成登录、记账、退款、信用账单、分期与投资记录。

> **状态声明**：V0.1 主体已实现并有自动化测试覆盖，但**发布门禁尚未全部通过**（iPhone 实机 PWA、双服务器 HTTPS 私网链路、前端 E2E、10 项账务口径确认）。**暂不建议录入真实家庭财务数据，暂不部署生产**。原始 Phase 0 计划见 `plan.md`（历史存档）。

## 静态原型（历史存档，虚构数据）

`previews/open-directly.html` 与 `previews/desktop-cream.png` / `previews/iphone-cream.png` 是 **Phase 0 存档的静态 Demo**，金额全部为虚构样本，**不代表当前 `apps/web`**。它们不需要安装依赖、无需启动后端，仅用于回顾视觉风格。

## 当前 V0.1 快速启动

先阅读 `apps/api/.env.example`（示例配置，勿提交真实值）。后端**必须显式**设置会话 Cookie 安全开关，否则拒绝启动。

```bash
# 终端 1：Go API（启动即执行 7 个版本迁移）
cd apps/api
mkdir -p data
# 本地纯 HTTP 开发：显式关闭 Secure；初始化令牌由部署者生成，仅用一次
export TINYNEST_DATA_DIR=./data
export TINYNEST_DB_PATH=./data/tinynest.sqlite3
export TINYNEST_COOKIE_SECURE=false
export TINYNEST_SETUP_TOKEN="$(openssl rand -hex 32)"   # Windows 可用 PowerShell 生成随机串
go run ./cmd/server

# 终端 2：Vue Web
cd apps/web
npm ci
npm run dev        # http://127.0.0.1:5173
```

Windows PowerShell 5.1 / PowerShell 7：

从仓库根目录启动后端：

```powershell
.\scripts\start-dev-api.ps1
```

首次运行会使用系统加密随机数生成 32 字节，编码为 64 位小写十六进制令牌，保存在 Git 忽略的 `apps/api/data/.env.setup-token`。以后启动会读取同一文件，并把令牌作为 `TINYNEST_SETUP_TOKEN` 传给 Go 后端。若启动前已设置该环境变量，脚本优先使用显式值。

在另一个 PowerShell 终端从仓库根目录执行以下命令，将默认令牌复制到剪贴板：

```powershell
Set-Clipboard -Value (Get-Content -Raw .\apps\api\data\.env.setup-token).Trim()
```

另一个终端按上面的 Vue Web 步骤启动前端，打开 http://127.0.0.1:5173 。初始化成功后，数据库中的初始化状态会关闭该入口，重启后端也无需再次初始化。生产部署按部署文档单独配置令牌。

首次使用：打开页面 →「首次初始化」→ 填入**与后端 `TINYNEST_SETUP_TOKEN` 完全一致**的一次性令牌、家庭名称、用户名与密码 → 提交（仅一次）→ 登录。令牌不写入浏览器代码、不进仓库、不写日志。

环境变量（全部在后端配置）：

| 变量 | 说明 |
|---|---|
| `TINYNEST_LISTEN_ADDR` | 默认 `127.0.0.1:8787` |
| `TINYNEST_DATA_DIR` | 默认 `./data` |
| `TINYNEST_DB_PATH` | 默认 `<data_dir>/tinynest.sqlite3` |
| `TINYNEST_SETUP_TOKEN` | 一次性 Bootstrap Token；**未配置时 `/api/v1/setup` 返回 403** |
| `TINYNEST_COOKIE_SECURE` | **必填** `true`（生产 HTTPS）/ `false`（本地纯 HTTP）；缺失拒绝启动 |
| `TINYNEST_TRUSTED_PROXIES` | 可选，逗号分隔 IP/CIDR；只有这些来源的 `X-Forwarded-*` 才被采信 |
| `TINYNEST_LOGIN_MAX_FAILURES` / `_WINDOW_SECONDS` / `_LOCKOUT_SECONDS` | 可选，登录失败限流参数，留空用内置默认值 |

数据库文件只在后端，`.gitignore` 已排除。健康检查：`GET http://127.0.0.1:8787/api/v1/healthz`；`readyz` 会做真实 SQLite `quick_check` 与迁移版本校验。前端 `/api` 经 Vite 代理访问后端。

## 工程目录（当前）

```text
tinynest/
├── AGENTS.md                    # 当前 V0.1 守则（含 Phase 0 历史说明）
├── plan.md / .plan/              # 历史阶段计划（保留原文）
├── assets/brand/                 # 用户原始团子图标（不改绘）
├── previews/                     # Phase 0 静态原型存档（虚构金额）
├── apps/web/                     # Vue 3 + TS + Vite；真实 API 界面 / PWA / SW
├── apps/api/                     # Go + SQLite：健康检查 + V0.1 账本 API + 7 版本迁移
├── scripts/                      # 品牌哈希核对、备份、自检、可选元表预初始化
├── deploy/frontend/ · backend/  # A 机 Nginx HTTPS 同源代理 / B 机私网 Nginx + systemd 模板
├── docs/                         # 设计、架构、安全、部署、账务规则、API 契约
└── docs/reviews/                 # 合并前审查、修复报告、发布门禁、口径决策记录
```

## 部署方向

A 机提供前端静态资源和 HTTPS 统一入口；B 机运行 Go 与 SQLite；A→B 经私网加密链路，B 不公开数据库/Go 端口。生产必须设置 `TINYNEST_COOKIE_SECURE=true`、可信反代列表与一次性初始化令牌。详见 `docs/deployment.md`、`docs/architecture.md`、`deploy/*`。**上线前仍需完成发布门禁**（见 `docs/reviews/v0.1-release-gates.md`）。

## 验证命令

```bash
cd apps/api
go test ./... -count=1
go vet ./...
go mod tidy -diff            # 退出码 0 表示 go.mod / go.sum 与 tidy 一致
go build ./...

cd apps/web
npm ci
npm run typecheck
npm run build
```

或用 `bash scripts/check-phase0.sh`（脚本名保留历史，实际执行全仓自检）/ Windows `powershell -ExecutionPolicy Bypass -File scripts/check-phase0.ps1`。CI 工作流见 `.github/workflows/ci.yml`（首次绿灯需真正在 GitHub Actions 跑过）。

设计文档：`docs/accounting-rules-v0.1.md`（账务规则，第 8 节 10 项**待确认**）、`docs/er-v0.1.md`（ER）、`docs/api-v0.1.md`（API 契约含状态列）、`docs/plan-v0.1.md`（V0.1 实施计划）、`IMPLEMENTATION_REPORT.md`（交付与未竟事项）。
