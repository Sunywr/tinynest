# 安全 / 备份底线（V0.1 当前）

- 个人账务属于敏感数据：使用 HTTPS；不在前端 JS 包中包含私钥、口令、数据库 URL、后端内网地址或一次性初始化令牌。
- 后端 Go 默认监听 loopback；只允许 A 机沿加密私网访问 B 机 Nginx；API 禁止 CORS `*`。
- **已实现**：Argon2id 口令、HttpOnly/`Secure`/SameSite=Lax 会话、写操作 CSRF、登录失败限流（有界冷却）、审计日志写入、所有财务查询 SQL 层 `household_id` 隔离、`/api/v1/setup` 一次性 Bootstrap Token。
- **不可作为安全依据**：`RemoteAddr == 127.0.0.1`（反代会让公网请求看起来像本机）、`r.TLS != nil`（Nginx 终止 TLS 后 Go 侧恒为 HTTP）、客户端伪造的 `X-Forwarded-*`（只有 `TINYNEST_TRUSTED_PROXIES` 中的来源才被采信）。
- 生产必须显式设置 `TINYNEST_COOKIE_SECURE=true` 与 `TINYNEST_SETUP_TOKEN`；未设置前者服务拒绝启动，未设置后者初始化端点 fail closed。详见 `docs/reviews/v0.1-pr-remediation-report.md` 与 `docs/api-v0.1.md` 第 1.1/1.2 节。
- 后端 SQLite 文件权限 0600，宿主目录 0700；备份文件保护同等级，另外加密并保留至少一份异地离线副本。数据与备份不要放到 A 机发布目录。
- `python3 scripts/init_sqlite.py --db ...` 仅幂等创建 `schema_migrations` 元表（可选），**业务表由 Go 启动时 7 版本迁移生成**；使用 `python3 scripts/backup-sqlite.py --db ... --out-dir ...` 在线一致备份（包含 WAL 中已提交数据），不要直接 cp 正在运行中的 SQLite DB 当备份。
- 恢复：在**隔离环境**中停 API、复制备份为新的数据库文件、限制权限 0600、`PRAGMA quick_check` + `foreign_key_check`、重启后验证；真实生产恢复需单独执行经审批的操作。备份→隔离库恢复演练已在审查与本轮修复中实测通过，但仓库内仍缺可自动执行的演练脚本。
- Service Worker 不缓存 `/api/` 响应，默认只缓存公开应用壳；敏感 HTML 不得进入 SW 缓存。
- CI/Agent 不自动 deploy 到公网；上线前按实际域名和 IP 人工核验安全组/防火墙/证书链/备份与恢复，并完成 `docs/reviews/v0.1-release-gates.md` 的生产发布门禁。
