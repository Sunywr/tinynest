# 安全与备份底线（V2）

> 个人账务属于敏感数据。本文件是**最低安全要求**，不是「已通过生产验收」声明；未执行项按 `UNVERIFIED` 记录。

## 1. 传输与网络边界

- 公网只暴露 A 机 443；A→B 走私网加密链路 + 后端 TLS（内部 CA 验证）。B 的 Go 服务只监听 loopback，B 私网 Nginx 只放行 A 的私网地址。
- 浏览器不直连后端、数据库或私网地址；API 禁止开放 CORS；前端只请求同源相对路径 `/api/v1/*`。
- 不在前端 JS 包中包含私钥、口令、数据库 URL、后端内网地址或一次性初始化令牌。

## 2. 已实现的安全机制

| 机制 | 实现位置 / 说明 |
| --- | --- |
| 口令哈希 | Argon2id（`internal/auth`）；登录失败限流按用户名 + IP，带冷却与 `Retry-After` |
| 会话 | `tn_session` HttpOnly Cookie，SameSite=Lax，7 天过期；`Secure` 由 `TINYNEST_COOKIE_SECURE` 显式决定（缺失或非法拒绝启动） |
| CSRF | 所有非 GET/HEAD 写请求必须带 `X-CSRF-Token`，与当前会话的 `csrf_token` 比较 |
| 角色 | `admin` / `member` / `viewer`；`viewer` 写请求 403（登出、改自己资料、改自己密码除外） |
| 初始化 | `POST /api/v1/setup` 需要一次性 `X-Setup-Token`；未配置令牌 fail closed；`TINYNEST_SETUP_DISABLED=true` 后一律 403；令牌常量时间比较、不落日志 |
| 可信反代 | 只有 `TINYNEST_TRUSTED_PROXIES` 中的来源，其 `X-Forwarded-For` / `X-Forwarded-Proto` 才被采信 |
| 家庭隔离 | 所有财务查询在 SQL 层按 `household_id` 限定；跨家庭 ID 等同不存在 |
| 审计 | 记账、预算、账户、成员、邀请、审批、用户与邮件配置写操作写 `audit_logs`（与业务同事务） |
| 邮件密钥 | 32 字节原始密钥文件；SMTP 口令与队列正文 AES-GCM 加密；验证码只存 HMAC 摘要；同一收件邮箱每天最多 3 封 |
| 响应 | 所有 API 响应 `Cache-Control: no-store`、`X-Content-Type-Options: nosniff`；错误不泄露 SQL/路径/堆栈 |
| 数据库 | 代码创建数据目录权限 0700；部署侧要求 SQLite 文件与备份为 0600；MySQL 通过本机 Unix socket + 仅 DML 的运行账号 |
| 服务端 HTTP | `ReadHeaderTimeout` 5s、`ReadTimeout` 10s、`WriteTimeout` 30s、`IdleTimeout` 60s；请求体大小限制 |

**不可作为安全依据**：

- `RemoteAddr == 127.0.0.1`：Nginx 反代会让公网请求看起来像本机。
- `r.TLS != nil`：Nginx 终止 TLS 后 Go 侧恒为 HTTP，不能用来判断请求是否安全。
- 客户端伪造的 `X-Forwarded-*`：只有配置在 `TINYNEST_TRUSTED_PROXIES` 的来源才被采信。

## 3. 必须显式配置的部署项

| 变量 | 要求 |
| --- | --- |
| `TINYNEST_SETUP_TOKEN` | 一次性初始化令牌；首次部署由运维在 B 机生成并注入，成功后移除 |
| `TINYNEST_SETUP_DISABLED` | 初始化完成后设为 `true`；严格 `true`/`false` |
| `TINYNEST_COOKIE_SECURE` | 生产 HTTPS 必须 `true`；本地纯 HTTP 才允许 `false` |
| `TINYNEST_TRUSTED_PROXIES` | 只填实际反代来源 IP/CIDR；不得 `0.0.0.0/0` |
| `TINYNEST_LOGIN_MAX_FAILURES` / `_WINDOW_SECONDS` / `_LOCKOUT_SECONDS` | 可选；留空用内置默认值 |
| `TINYNEST_MAIL_KEY_FILE` / `CREDENTIALS_DIRECTORY/mail-key` | 可选邮件功能；恰好 32 字节，文件权限 0600 |
| `TINYNEST_MYSQL_PASSWORD_FILE` / `CREDENTIALS_DIRECTORY/mysql-password` | MySQL 口令只能来自文件，不接受命令行参数 |

## 4. 运维工具安全规则（`tinynest-admin` / `mail-admin`）

- 两个工具都**只在服务器本地、服务停止时由人工执行**，不提供也不得增加 HTTP 入口；不得放进 Nginx 可访问目录。
- `tinynest-admin purge` 只做 DML，单事务清理 **55 个表名（当前 V2 基线中 23 张存在、32 个 V1 遗留名跳过）**；事务内断言已分类的业务表全空、11 张身份表与 2 张保留配置表计数不变、`users` 摘要不变后才提交。
- `-household=<id>` **不缩小删除范围**：每条 `DELETE` 都不带家庭条件，库里有多家庭时一次执行会清空全部家庭的业务行；CLI 在检测到多家庭时要求显式确认。
- `-confirm` 必须与数据库名完全一致（SQLite 为数据库文件路径）；`-execute` 必须同时带 `-yes-service-stopped`；执行前先备份并做隔离恢复比对，执行后立即 `verify`。
- 用户、会话、口令哈希、邮箱身份、站点管理员、SMTP 配置与邮件用量属于身份数据，`purge` 不删除；含 AES-GCM 密文授权码的 `statement_mailbox_configs` / `credit_statement_mailboxes` 同样保留，但**不进入身份导出/导入**，切换新库时是否单独搬运由执行人判断。身份导出文件含用户名与邮箱（及口令哈希），权限 0600、按机密保管并限期删除，不得进入仓库。
- 生产清理与切换由用户执行，我们只交付工具与手册，不接触生产凭据；证据与回滚要求见 [生产清理手册](plans/v2-prod-purge-runbook.md) 与 [新库切换手册](plans/v2-cutover-runbook.md)。

## 5. 数据库、备份与恢复

- 数据只存在 B 机；备份文件保护等级同数据库，另外加密并保留至少一份异地离线副本。
- SQLite 使用在线一致性备份（含 WAL 中已提交数据），不要直接复制运行中的数据库文件；MySQL 使用一致性导出。
- 恢复演练只在隔离环境执行：停 API、复制备份为新数据库文件、限制权限 0600、`PRAGMA quick_check` + `PRAGMA foreign_key_check`、启动后核对 `readyz`、身份计数与账务汇总；生产恢复需单独审批。
- 恢复后若数据库不是 V2 基线（`schema_meta.baseline` 缺失或不为 `v2`），V2 二进制会拒绝启动；不要通过改元数据绕过。
- 邮件密钥与数据库分开备份：数据库备份不含解密能力；密钥丢失无法从数据库恢复 SMTP 口令与队列正文。
- 旧库与备份在清理/切换后保留 ≥30 天，期间不得删除回滚材料。

## 6. Service Worker 与前端

- Service Worker 不缓存 `/api/` 响应，只缓存公开应用壳；敏感 HTML 不得进入 SW 缓存。
- 会话过期或 401 时清空内存私有状态并回到公开页；公开页不渲染任何私人金额。
- 离线时明确提示「网络不可用，未保存」，不伪造成功；V2 不做离线补记账队列。

## 7. 安全验收（未全部完成）

- [ ] 双服务器实际 HTTPS 与 A→B 私网 TLS 证书链、白名单与防火墙经实机核验（`UNVERIFIED`）。
- [ ] iPhone 实机安装 PWA，确认安全区、会话与退出行为（`UNVERIFIED`）。
- [ ] 生产清理与切库按手册执行并留存证据（`UNVERIFIED`，用户执行）。
- [ ] 生产备份恢复演练与异地副本校验通过（`UNVERIFIED`，用户执行）。
- [ ] 生产登录限流、Cookie `Secure`、`setup_disabled` 与可信反代配置在真实链路验证（`UNVERIFIED`）。
- 本地可执行项（Go 测试、前端构建/E2E、迁移与基线拒绝测试）以 CI 输出为准；未执行时不得宣称通过。
