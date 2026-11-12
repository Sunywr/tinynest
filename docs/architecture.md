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
- `scripts/init_sqlite.py`：SQLite 元数据初始化；**Go 现在未连接 SQLite 数据库**，后续须实现驱动/迁移工具后才能提供真实数据库读写。
- 暂不确定正式身份认证、家庭成员权限、记账表和安全存储；任何未来敏感功能上线路径必须补完认证、授权、CSRF/会话等安全设计。
