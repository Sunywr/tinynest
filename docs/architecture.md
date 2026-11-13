# TinyNest 架构 / 部署边界（Phase 0 骨架 + V0.1 账本主体）

> 本文件描述**当前**架构。Phase 0 的拓扑与双服务器边界继续有效；后端能力已随 V0.1 从「仅健康检查」升级为完整账本 API。历史阶段计划见 `plan.md`（已标注为历史存档）。

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
- 域名、VPN IP、证书和用户身份策略均待用户确认；模板里的 `YOUR_*` 仅占位。

### 当前实现（V0.1）
- `apps/web`：Vue 3 / TypeScript / Vite + PWA，独立 `dist` 静态发布；**已连接真实后端 API**，不再展示虚构金额。`previews/*` 仅为 Phase 0 静态原型存档。
- `apps/api`：Go 模块化单体，`net/http` + 纯 Go 驱动 `modernc.org/sqlite`；除 `healthz` / `readyz` 外提供登录、账户/分类/项目、交易与复式分录、退款、信用账单、分期、投资、首页统计等真实业务 API。完整路由与状态见 `docs/api-v0.1.md`。
- 数据库：启动时按序执行 `001`–`007` 共 7 个版本迁移（逐版本单事务、`CREATE TABLE IF NOT EXISTS` + `INSERT OR IGNORE`，重跑幂等）；启用 WAL / `foreign_keys=ON` / `busy_timeout=5000`；余额由 `journal_lines` 重建。`readyz` 为真实 `PRAGMA quick_check` + 迁移版本校验。业务模型见 `docs/er-v0.1.md`。
- 认证与安全：Argon2id 口令、HttpOnly / SameSite=Lax 会话 Cookie（`Secure` 由 `TINYNEST_COOKIE_SECURE` 显式配置）、写操作 CSRF、审计日志、登录失败限流、`/api/v1/setup` 一次性 Bootstrap Token。所有财务查询在 SQL 层按 `household_id` 过滤。详见 `docs/reviews/v0.1-pr-remediation-report.md` 与 `docs/security.md`。
- `scripts/init_sqlite.py`：仅创建 `schema_migrations` 元表的**可选预初始化工具**；业务表由 Go 进程启动时自动迁移生成，不要把它当成完整账本 schema 的初始化脚本。
- `scripts/backup-sqlite.py`：SQLite 在线一致性备份；恢复演练在隔离库验证过（见审查与修复报告）。

### 尚未完成 / UNVERIFIED
- 多成员登录与细粒度权限、每日自动调度器（当前仅手动 `POST /schedule/generate`）、S3/S4/S5 部分端点（见 `IMPLEMENTATION_REPORT.md` 第 8 节）。
- 生产 HTTPS、A→B 私网 TLS 链路的双服务器实机连通性；iPhone Safari 实机 PWA；前端 E2E；10 项账务口径确认。**发布门禁未通过，暂不录入真实家庭财务数据、暂不部署生产。**
