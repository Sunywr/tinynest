# TinyNest · 小家

**已确认视觉主题：A · 奶油团子**。个人家庭记账网站项目 Phase 0 初始化包，适合 PC Web 与 iPhone PWA（非 App Store App）。

> ⚠️ 当前只有 **界面演示与基础设施骨架**；所有金额、流水、预算为虚构样本，**不可用于真正记账**。没有登录、权限、加密账务存储和真实业务 API。

## 现在就能预览

**直接打开** `previews/open-directly.html`，不需要安装任何依赖、无需启动后端；或查看 `previews/desktop-cream.png` / `previews/iphone-cream.png`。

## 快速启动 Vue + Go

```bash
# 终端1: Go API
cd apps/api
mkdir -p data
TINYNEST_DATA_DIR=./data go run ./cmd/server

# 终端2: Vue Web
cd apps/web
npm install
npm run dev
# http://127.0.0.1:5173
```

Windows PowerShell：`cd apps\api; New-Item -ItemType Directory -Force data; go run ./cmd/server`，另一个窗口 `cd apps\web; npm install; npm run dev`。

`http://127.0.0.1:8787/api/v1/healthz` 是后端存活检查；页面 `/api` 通过 Vite 代理访问它。断开后端不影响 Demo 页面。

## 工程目录

```text
tinynest/
├── AGENTS.md / plan.md / README.md
├── assets/brand/         # 用户原始团子图标（不改绘）
├── previews/             # 无依赖 HTML Demo、PC/手机截图
├── apps/web/             # Vue 3 + TS + Vite；PWA/图标/Service Worker
├── apps/api/             # Go 标准库：healthz / readyz，零业务 API
├── scripts/              # 品牌哈希核对、元数据库初始化/备份/自检
├── deploy/frontend/      # A 机 Nginx HTTPS 同源代理模板
├── deploy/backend/       # B 机私网 Nginx + systemd 模板
└── docs/                 # 设计、架构、安全、部署、Agent 提示词、验收
```

## 部署方向

A 机提供前端静态资源和 HTTPS 统一入口；B 机运行 Go 与 SQLite；A→B 经私网加密链路，B 不公开数据库/Go 端口。具体配置待用户提供域名和服务器网络信息后再落地。

## 给 Coding Agent

打开 `docs/agent-prompt.md` 并复制提示词；或直接要求 Agent 阅读 `AGENTS.md`、`plan.md`。不得跳到真实业务开发。

## 自检

Linux/macOS: `bash scripts/check-phase0.sh`；Windows：`powershell -ExecutionPolicy Bypass -File scripts/check-phase0.ps1`。安装过 npm 依赖后，运行 `cd apps/web && npm run build`。
