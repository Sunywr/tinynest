# Phase 0 架构 / 部署边界

```text
PC 浏览器 / iPhone Safari 或主屏 PWA
        |
        | HTTPS 443 (单一公开域名)
        v
[服务器 A · 前端] Nginx / SPA dist / PWA 静态资源
        |
        | 同源请求 /api/v1/* -> 私网 VPN/overlay + TLS (服务端证书验证)
        v
[服务器 B · 后端] 私网 Nginx TLS:8443 -> 127.0.0.1:8787 Go API
                                               |
                                               + /var/lib/tinynest/tinynest.sqlite3
                                               + /var/backups/tinynest/ （加密、异地）
```

### 数据与边界
- 所有数据库表/文件**只在 B**；A 不存库、不存密钥、只做静态分发和 API 反向代理。
- 浏览器不会直连 B 或 SQLite。无 CORS 开放；仅 A 同源 `/api` 代理。
- B 的 Go 服务仅 `127.0.0.1:8787` 监听；B 私网 Nginx 8443 由防火墙允许 A 私网 IP，禁止公网。
- A→B 不可裸 HTTP 跨公网；优先 WireGuard/Tailscale 类加密隧道 + 后端 Nginx TLS + CA 验证。若没有可用内网，停止部署，先处理安全网络。
- 未来域名、VPN IP、证书和用户身份策略均待用户确认；模板里的 YOUR_* 仅占位。

### 开发和交付
- `apps/web`：Vue 3/Vite + PWA，独立 dist 静态发布。
- `apps/api`：Go stdlib，仅 healthz/readyz，独立二进制发布。
- `scripts/init_sqlite.py`：Phase 0 的 SQLite 元数据初始化脚本，保留可用。
- **V0.1 更新**：Go 后端已通过纯 Go 驱动 `modernc.org/sqlite` 连接 SQLite，启用 WAL / `foreign_keys=ON` / `busy_timeout`，7 个版本迁移在启动时按序执行、单事务提交；`readyz` 已升级为真实 `PRAGMA quick_check` + 迁移版本校验。业务模型见 `docs/er-v0.1.md`。
- V0.1 已实现单家庭管理员登录（Argon2id）、HttpOnly/Secure/SameSite 会话、写操作 CSRF 校验与审计日志；多成员登录与细粒度权限仍待后续版本。生产 HTTPS、A→B 私网 TLS 链路与实机验证**尚未执行**，模板中的 `YOUR_*` 仍为占位符。
