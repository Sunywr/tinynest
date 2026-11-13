# 运行与部署指引（服务器 A/B 完全独立）

## 本地开发（Windows/Linux/macOS）

1. 前端免安装预览：双击 `previews/open-directly.html`（资源路径需保留整个项目目录）。
2. Go API（Terminal 1）：
   ```bash
   cd apps/api
   mkdir -p data
   go test ./... && go run ./cmd/server
   ```
   Windows PowerShell 可用 `New-Item -Type Directory -Force data`。
3. Vue Web（Terminal 2）：
   ```bash
   cd apps/web
   npm install
   npm run dev
   ```
   浏览器访问 `http://127.0.0.1:5173`；Vite 将 `/api` 转发到 Go `127.0.0.1:8787`。
4. 构建：`cd apps/web && npm run build`、`cd apps/api && go build -o tinynest-api ./cmd/server`。正式版 PWA 需要 HTTPS（localhost 本地测试除外）。

**注意**：`npm install` 依赖访问可信 npm 源；包内不包含 node_modules 和 npm lock，Agent 首次安装后需提交 `package-lock.json`，此后使用 `npm ci` 实现可复现构建。

## 服务器 A（前端）

- Linux，安装 Nginx，配置正式域名证书。
- 只上传 `apps/web/dist/` 到 `/srv/tinynest/web/dist/`；**不上传 B 数据库或后端 .env**。
- 将 `deploy/frontend/nginx.conf.example` 复制到正式 Nginx 配置，替换 PUBLIC_DOMAIN、B 私网 IP 和内部 CA 路径。
- `sudo nginx -t` 通过后再 reload。

## 服务器 B（后端）

- Linux 建立非 root `tinynest` 服务账号、`/var/lib/tinynest` mode 0700、定期备份目录（系统权限配置由运维人员执行）。
- `cd apps/api && go build -o tinynest-api ./cmd/server`，放置 `/opt/tinynest/api/tinynest-api`。
- `python3 scripts/init_sqlite.py --db /var/lib/tinynest/tinynest.sqlite3`（在 B 执行，确保所有权归服务用户）。
- 审核并安装 `deploy/backend/tinynest-api.service.example` 与 `deploy/backend/nginx.conf.example`；替换私网监听地址、证书、白名单 IP；做防火墙 ACL。
- **首次初始化（FIX-01）**：在 B 机生成强随机令牌（如 `openssl rand -hex 32`），仅通过 systemd 的 `Environment=` 或受保护的 env 文件注入 `TINYNEST_SETUP_TOKEN`；**不要写进仓库、日志或前端**。未配置令牌时 `POST /api/v1/setup` 返回 `403 setup_disabled`（fail closed）。管理员在初始化页面填入该令牌提交一次，成功后令牌即失效（再次调用返回 `409`）。切勿以「Go 侧看到 `127.0.0.1`」作为安全依据——反代会让公网请求看起来像本机。
- 验证 B 的 loopback `curl -fsS http://127.0.0.1:8787/api/v1/healthz`，然后在 A 验证 `curl -fsS https://YOUR_PUBLIC_DOMAIN/api/v1/healthz`。
- 使用 `scripts/backup-sqlite.py` 形成一致性备份并验证备份文件；不要在公网开放 `8787`、`8443`（8443 仅私网）。

## iPhone 安装

完成 HTTPS 部署后，在 iPhone Safari 打开 TinyNest 域名 -> 分享 -> 添加到主屏幕 -> 如有“作为 Web App 打开”选项则开启 -> 添加。PWA 不是 App Store 原生应用。

## 上线前仍需用户提供

正式域名/两台服务器操作系统/是否已有 VPN 或私网/证书获取方式/更新策略/备份保留周期与存储位置。暂不索取任何账号、密码或私钥。
