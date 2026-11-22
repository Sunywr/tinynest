# 运行与部署指引（服务器 A/B 完全独立）

> 适用版本：**V2 记账重置**。数据层为 V2 基线：两个方言各 5 个迁移文件、运行时 28 张表；不支持 V1 旧库原地升级。
> 生产清理与新库切换由用户执行，步骤与证据要求以 [生产清理手册](plans/v2-prod-purge-runbook.md) 与 [新库切换手册](plans/v2-cutover-runbook.md) 为准。
> 本地开发见 [DEVELOPMENT.md](DEVELOPMENT.md)，架构与边界见 [architecture.md](architecture.md)，安全底线见 [security.md](security.md)。

## 1. 本地开发（Windows/Linux/macOS）

1. 工具链、Go 不在 PATH、Windows `npm.cmd` 等注意事项见 [DEVELOPMENT.md](DEVELOPMENT.md) §1。
2. 后端：
   ```bash
   cd apps/api
   mkdir -p data
   TINYNEST_DATA_DIR=./data TINYNEST_DB_PATH=./data/tinynest.sqlite3 \
   TINYNEST_COOKIE_SECURE=false TINYNEST_SETUP_TOKEN=<一次性令牌> \
   go run ./cmd/server
   ```
3. 前端：
   ```bash
   cd apps/web
   npm.cmd ci
   npm.cmd run dev          # http://127.0.0.1:5173，/api 代理到 127.0.0.1:8787
   ```
4. 构建：`cd apps/web && npm.cmd run build`、`cd apps/api && go build -o tinynest-api ./cmd/server`。正式版 PWA 需要 HTTPS（localhost 除外）。
5. 本地库重置：停止服务后删除本地 SQLite 文件（含 `-wal`、`-shm`），由 V2 二进制重建基线；不要修改 `schema_meta` 或迁移账本绕过基线守卫。

## 2. 服务器 A（前端）

- Linux + Nginx + 正式域名证书。
- 只上传 `apps/web/dist/` 到 `/srv/tinynest/web/dist/`；**不上传 B 数据库、密钥或后端 env**。
- 复制 `deploy/frontend/nginx.conf.example`，替换 PUBLIC_DOMAIN、B 私网地址与内部 CA 路径；`sudo nginx -t` 通过后 reload。
- A 不存数据库、不存 `mail-key`、不存 MySQL 口令。

## 3. 服务器 B（后端）

- Linux 建立非 root `tinynest` 服务账号；`/var/lib/tinynest` 权限 0700；备份目录另行加固。
- 构建并放置两个二进制：
  ```bash
  cd apps/api
  go build -o tinynest-api ./cmd/server
  go build -o tinynest-admin ./cmd/tinynest-admin
  go build -o tinynest-mail-admin ./cmd/mail-admin
  ```
  `tinynest-api` 放 `/opt/tinynest/api/`；`tinynest-admin` 与 `tinynest-mail-admin` 只放受控运维目录，**不得由 Nginx 或任何 HTTP 入口暴露**。
- 审核并安装 `deploy/backend/tinynest-api.service.example` 与 `deploy/backend/nginx.conf.example`；替换私网监听地址、证书、白名单；配置防火墙 ACL。
- 数据库路径（SQLite）或 MySQL Unix socket 配置通过 systemd `Environment=` / `EnvironmentFile` 注入，文件权限 0600，属主为服务账号。
- **首次初始化（受控运维流程，公网无 UI 入口）**：在 B 机生成强随机令牌（如 `openssl rand -hex 32`），只通过 systemd 环境注入 `TINYNEST_SETUP_TOKEN`，不要写进仓库、日志、URL 或前端。启动 API 后由运维在受控环境调用一次：
  ```bash
  curl -fsS -X POST http://127.0.0.1:8787/api/v1/setup \
    -H 'Content-Type: application/json' \
    -H "X-Setup-Token: <TOKEN>" \
    -d '{"household_name":"我的小家","username":"admin","password":"<强口令>"}'
  ```
  成功后再次调用返回 409。公开前端不提供初始化/创建管理员页面。
- **初始化完成后立即收口**：从服务环境移除 `TINYNEST_SETUP_TOKEN`，设置 `TINYNEST_SETUP_DISABLED=true` 并重启；此后 `POST /api/v1/setup` 对所有请求（含有效令牌）一律返回 403 `setup_disabled`。该开关是附加保护，不替代数据库单次创建约束。
- 验证：B 机 loopback `curl -fsS http://127.0.0.1:8787/api/v1/healthz`，A 机 `curl -fsS https://<站点域名>/api/v1/healthz` 与 `/api/v1/readyz`。不要公网开放 `8787` / `8443`（8443 仅私网）。
- 首次启动会让 V2 二进制在**空库**上建立 `schema_meta.baseline=v2` 并写入默认分类。若数据库不是 V2 基线，启动会拒绝并提示按切换手册处理；不要绕过守卫。

## 4. 数据库迁移与基线

- 迁移文件：SQLite `apps/api/internal/db/migrations/001_core.sql`、`002_bookkeeping.sql`、`003_category_style.sql`、`004_credit_cycle.sql`；MySQL `apps/api/internal/db/mysql/` 下同名文件。文件名前 3 位是版本号。
- 单一迁移器 `internal/db/migrate.go` 按版本升序应用，记录 SHA-256 校验和，校验版本连续性与已应用版本校验和；`schema_meta` 写入 `baseline=v2`。
- SQLite 迁移与账本行在同一事务；MySQL DDL 隐式提交，因此每个 MySQL 迁移文件必须可重跑。新表用 `CREATE TABLE IF NOT EXISTS` 与内联索引；`ALTER TABLE` 用 `information_schema.columns` + `PREPARE/EXECUTE` 做幂等保护（003/004 的写法）。
- 迁移只在版本推进时执行一次；不存在 V1 时期每次启动执行 `UPDATE categories ...` 的副作用；默认分类由 Go 代码幂等写入。
- 生产 Linux 二进制必须用 **LF** 的嵌入迁移文件构建。Windows CRLF 工作树直接 `go build` 会使 `schema_migrations` 的 checksum 不匹配并拒绝启动；用 `git archive` 取 LF 文件或归一化两套 `internal/db/**/*.sql` 后再交叉编译，不要修改生产账本或守卫。
- 不支持 V1 旧库原地升级；旧库按 [新库切换手册](plans/v2-cutover-runbook.md) 新建库并导入 11 张身份表，旧库保留为回滚点 ≥30 天。

## 5. MySQL 8

本地开发默认 SQLite；生产通过 B 机本机 Unix socket 使用独立 MySQL 数据库与受限账号。

| 变量 | 说明 |
| --- | --- |
| `TINYNEST_DB_DRIVER=mysql` | 启用 MySQL 模式 |
| `TINYNEST_MYSQL_SOCKET` | MySQL Unix socket 路径 |
| `TINYNEST_MYSQL_DATABASE` | 数据库名（`purge -confirm` 必须与它完全一致） |
| `TINYNEST_MYSQL_USER` | 运行账号（运行时只需 DML 权限） |
| `TINYNEST_MYSQL_PASSWORD_FILE` | 口令文件；也支持 systemd `CREDENTIALS_DIRECTORY/mysql-password` |

- 首次启动在**空库**上创建 V2 基线（`schema_meta.baseline=v2`），之后 `readyz` 返回 `mysql_ping_and_migrations`。
- 建立基线可使用迁移账号（含 DDL 权限）；基线建立并核对后，运行服务切换为仅 DML 的受限账号。
- **不得让 V2 二进制连接 V1 旧库**：旧库会被基线守卫拒绝；生产切换按 [新库切换手册](plans/v2-cutover-runbook.md) 执行。
- 备份使用 MySQL 一致性导出；恢复演练必须在隔离实例执行，不接触生产账本。

### MySQL 备份恢复与异地副本验收

使用运维批准的备份与专用隔离 MySQL 实例；隔离实例版本与源端兼容，凭据只具备隔离库权限。演练记录只保留备份时间、摘要、恢复结果与经授权的汇总值。

1. **确认备份可用**：取得备份文件与 SHA-256；比较异地副本下载后的 SHA-256；记录备份时间、校验时间与故障域。加密备份先校验密文再在隔离环境解密。
2. **恢复到空库**：核对目标实例与目标库属于本次演练；标准单库 SQL 转储可使用下列命令（客户端配置指定隔离实例）。
3. **核对恢复结果**：比较备份时记录的迁移版本、11 张身份表计数、`users` 摘要、会计分录数与借贷合计；单一「借贷平衡」不足以证明恢复完整。
4. **启动隔离 API**：使用该演练库与只在隔离环境有效的配置，确认 `readyz` 正常，并以授权方式读取账户、流水与统计核对余额；记录恢复耗时，按数据保留策略清理临时库与解密产物。

```sh
mysql --defaults-extra-file=/secure/drill-client.cnf --database=tinynest_restore < /secure/backup.sql
mysql --defaults-extra-file=/secure/drill-client.cnf --database=tinynest_restore <<'SQL'
SELECT version, description FROM schema_migrations ORDER BY version;
SELECT `value` FROM schema_meta WHERE meta_key='baseline';
SELECT COUNT(*) AS users FROM users;
SELECT COUNT(*) AS entries FROM journal_entries;
SELECT COUNT(*) AS lines,
       COALESCE(SUM(debit_minor),0) AS debit_minor,
       COALESCE(SUM(credit_minor),0) AS credit_minor
FROM journal_lines;
SQL
```

证据分别记录原始备份校验、异地副本校验、隔离恢复、账务比对与 API 验收结果；未执行项记为 `UNVERIFIED`。合成数据演练只验证流程与工具，生产备份由运维单独验收。

## 6. 备份、清理与切换

- 日常备份：SQLite 使用在线一致性备份（含 WAL 中已提交数据），不要直接复制运行中的数据库文件；MySQL 使用一致性导出。备份加密并保留至少一份异地离线副本，恢复密钥独立保管。
- 生产清理（清空业务数据、保留 11 张身份表）：先备份 → 隔离恢复比对 → `tinynest-admin baseline-capture` → `purge --dry-run` 人工核对 → 停服后 `purge --execute --confirm=<DBNAME> --yes-service-stopped` → `verify` → 启动服务冒烟。完整判据、边缘情况与回滚见 [生产清理手册](plans/v2-prod-purge-runbook.md)。
- 新库切换（迁移 11 张身份表到 V2 新库）：旧库备份 → 新库建基线 → 停旧服务 → `identity-export` → `identity-import` → 计数与 `users` 摘要比对 → 切换 `TINYNEST_MYSQL_DATABASE` 与账号 → `readyz` 与冒烟 → 旧库保留 ≥30 天。见 [新库切换手册](plans/v2-cutover-runbook.md)。
- `tinynest-admin` 只能以服务账号在服务器本地执行；**绝不能通过 HTTP 暴露，也不能被挂在 Nginx location 下**。MySQL 口令只从受保护文件读取，不出现在命令行与日志。

## 7. 邮件与密钥

- 邮件加密密钥（SMTP 口令与待发邮件正文）是**恰好 32 字节的原始二进制文件**；通过 systemd `LoadCredential=mail-key:/etc/tinynest/credentials/mail-key` 或 `TINYNEST_MAIL_KEY_FILE` 注入。
- 生成密钥（B 机本地执行，不可覆盖已有密钥）：
  ```sh
  install -d -m 0700 /etc/tinynest/credentials
  /opt/tinynest/api/tinynest-mail-admin -action generate-key -key-file /etc/tinynest/credentials/mail-key
  chmod 0600 /etc/tinynest/credentials/mail-key
  ```
- 密钥必须单独备份并指定保管人；数据库备份不包含解密能力。**不要直接替换密钥**，否则 `mail_configuration.password_cipher` 与 `mail_queue.payload` 无法读取。
- V1 的信用卡账单 IMAP 模块与 `credit-mail-key` 随信用模块在 V2 退役：当前代码只读取一个 `mail-key`，不存在 `TINYNEST_CREDIT_MAIL_KEY_FILE`。切换手册仍要求备份旧库密钥，是因为旧库密文不可逆，与 V2 运行时无关。
- SMTP 配置由站点管理员在「设置 → 系统邮件」动态保存；测试通过后才切换配置，密码不回显。邮件能力细节见 [托管邮箱 SMTP 接入](feishu-mail.md)（该文中的 V0.1 迁移版本号已过时，以本文件 §4 为准）。

## 8. iPhone 安装

完成 HTTPS 部署后，在 iPhone Safari 打开站点 → 分享 → 添加到主屏幕 → 如有「作为 Web App 打开」选项则开启 → 添加。PWA 不是 App Store 原生应用；安装图标使用站点根目录的原版奶油团子 Logo（`/apple-touch-icon.png`，180×180）。若旧主屏快捷方式图标未更新，删除后重新添加。

## 9. 上线前仍需用户提供

正式域名、两台服务器操作系统、是否已有 VPN/私网、证书获取方式、更新策略、备份保留周期与存储位置。不要索取或记录任何账号、密码或私钥。

## 10. 未验证（UNVERIFIED）

- 本文命令未在本次文档编写环境执行；双服务器实际连通、证书链、防火墙与 iPhone 实机安装未验证。
- 生产 MySQL 运行时链路（含 `readyz` 的 `mysql_ping_and_migrations` 实际输出）未在本环境执行；本机隔离 MySQL 8.0.46（TCP 3307）已通过迁移、核心 API、`TestMySQLPurgeDrill`、`TestMySQLIdentityCutoverDrill` 与 dump/restore 对账，但不能替代生产 B 机 Unix socket + 受限账号链路。
- 生产清理与新库切换未执行；实际表名、驱动版本与耗时以生产输出为准。
- 生产备份/恢复演练与异地副本校验未执行。
