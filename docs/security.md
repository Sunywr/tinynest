# 安全/备份底线（仅规划模板）

- 个人账务属于敏感数据：使用 HTTPS；不在前端 JS 包中包含私钥、口令、数据库 URL、后端内网地址等。
- 后端 Go 默认监听 loopback；只允许 A 机沿加密私网访问 B 机 Nginx；API 禁止 CORS *；正式业务上线前必须设计认证、会话管理、CSRF 防护、限流和权限。
- 后端 SQLite 文件权限 0600，宿主目录 0700；备份文件保护同等级，另外加密并保留至少一份异地离线副本。数据与备份不要放到 A 机发布目录。
- `python3 scripts/init_sqlite.py --db ...` 是幂等元数据初始化；使用 `python3 scripts/backup-sqlite.py --db ... --out-dir ...` 在线一致备份（包含 WAL 中已提交数据），不要直接 cp 正在运行中的 SQLite DB 当备份。
- 恢复：在**隔离环境**中停 API、复制备份为新的数据库文件、限制权限 0600、`PRAGMA quick_check`、重启后验证；真实生产恢复需单独执行经审批的操作，防止覆盖现有数据。
- Service Worker 不缓存 `/api/` 响应，默认只缓存公开应用壳；未来一旦加入登录或敏感 HTML，必须重新审计 SW 缓存策略。
- CI/Agent 不自动 deploy 到公网；上线前按实际域名和 IP 人工核验安全组/防火墙/证书链/备份与恢复。
