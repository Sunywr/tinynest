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
- **首次初始化（受控运维流程，公网无 UI 入口）**：在 B 机生成强随机令牌（如 `openssl rand -hex 32`），仅通过 systemd 的 `Environment=` 或受保护的 env 文件注入 `TINYNEST_SETUP_TOKEN`；**不要写进仓库、日志、URL 或前端**。未配置令牌时 `POST /api/v1/setup` 返回 `403 setup_disabled`（fail closed）。在受控环境（B 机 loopback 或私网）用一次 HTTP 调用完成初始化，例如：`curl -fsS -X POST http://127.0.0.1:8787/api/v1/setup -H 'Content-Type: application/json' -H "X-Setup-Token: <TOKEN>" -d '{"household_name":"我的小家","username":"admin","password":"<强口令>"}'`；成功后再次调用返回 `409`。**公开前端不提供初始化/创建管理员页面**，普通访客只能登录。
- **初始化完成后立即收口**：从服务环境移除 `TINYNEST_SETUP_TOKEN`，并设置 `TINYNEST_SETUP_DISABLED=true` 后重启 API；此后 `POST /api/v1/setup` 对所有请求（含有效令牌）一律返回 `403 setup_disabled`。该开关是附加保护，不替代数据库单次创建约束。切勿以「Go 侧看到 `127.0.0.1`」作为安全依据——反代会让公网请求看起来像本机。
- 验证 B 的 loopback `curl -fsS http://127.0.0.1:8787/api/v1/healthz`，然后在 A 验证 `curl -fsS https://YOUR_PUBLIC_DOMAIN/api/v1/healthz`。
- 使用 `scripts/backup-sqlite.py` 形成一致性备份并验证备份文件；不要在公网开放 `8787`、`8443`（8443 仅私网）。

## iPhone 安装

完成 HTTPS 部署后，在 iPhone Safari 打开 TinyNest 域名 -> 分享 -> 添加到主屏幕 -> 如有“作为 Web App 打开”选项则开启 -> 添加。PWA 不是 App Store 原生应用。安装图标使用站点根目录的原版奶油团子 Logo（`/apple-touch-icon.png`，180×180）。若已添加的图标仍为灰色「小」，删除旧主屏幕快捷方式，再从 Safari 重新添加。

## 上线前仍需用户提供

正式域名/两台服务器操作系统/是否已有 VPN 或私网/证书获取方式/更新策略/备份保留周期与存储位置。暂不索取任何账号、密码或私钥。

## 生产 MySQL 8

本地开发默认使用 SQLite。生产通过后端本机 Unix socket 使用独立 MySQL 数据库和受限账号。设置 `TINYNEST_DB_DRIVER=mysql`，并配置 `TINYNEST_MYSQL_SOCKET`、`TINYNEST_MYSQL_DATABASE` 和 `TINYNEST_MYSQL_USER`。密码通过 systemd `LoadCredential=mysql-password:/path/to/password-file` 注入，或设置 `TINYNEST_MYSQL_PASSWORD_FILE` 指向服务可读的受保护文件。

首次启动应用会创建 MySQL 专用基线结构。切换已有环境前暂停写入并备份。旧 SQLite 含业务数据时需要先迁移并逐表核验，不可直接更换驱动。就绪接口在 MySQL 模式返回 `mysql_ping_and_migrations`。把备份定时任务同步切换为 MySQL 一致性导出，并用独立库验证恢复。

首次创建家庭和管理员同样通过后端的一次性初始化令牌由运维在受控环境调用 `POST /api/v1/setup` 完成（公开前端无初始化入口），完成后设置 `TINYNEST_SETUP_DISABLED=true` 收口。

### MySQL 生产备份恢复与异地副本验收

使用运维批准的备份和专用隔离 MySQL 实例。隔离实例版本应与源端兼容，凭据只具备隔离库权限。演练记录只保留备份时间、摘要、恢复结果和经授权的汇总值。备份、凭据与逐笔账务留在受控位置。

1. **确认备份可用。** 从备份作业取得已完成的备份及其 SHA-256。比较异地副本下载后的 SHA-256 与原始值，留存备份时间、校验时间与副本所在故障域。若备份经过加密，先对同一层的密文校验，再在隔离环境解密及解压。
2. **恢复到空库。** 核对目标实例标识和目标库，确认它们属于本次演练。检查备份格式与内嵌数据库切换指令是否符合隔离目标。标准单库 SQL 转储可使用下面的命令，受保护的客户端配置文件须明确指定隔离实例地址与账号。
3. **核对恢复结果。** 比较备份时记录的迁移版本、账户及分录计数、借贷合计、选定账户余额以及合同和持仓快照。单独检查借贷平衡不能证明数据恢复完整。如原备份没有对应快照，记录该限制，并在下一次备份补齐同一快照的证据。
4. **启动隔离 API。** 使用该演练库和只在隔离环境有效的配置，确认就绪检查正常，并以授权的测试方式读取账户、合同和持仓，核对余额与汇总。记录恢复耗时。按数据保留策略清理临时库和解密产物。

```sh
mysql --defaults-extra-file=/secure/drill-client.cnf --database=tinynest_restore < /secure/backup.sql
mysql --defaults-extra-file=/secure/drill-client.cnf --database=tinynest_restore <<'SQL'
SELECT version FROM schema_migrations ORDER BY version;
SELECT COUNT(*) AS accounts FROM accounts;
SELECT COUNT(*) AS entries FROM journal_entries;
SELECT COUNT(*) AS lines,
       COALESCE(SUM(debit_minor),0) AS debit_minor,
       COALESCE(SUM(credit_minor),0) AS credit_minor
FROM journal_lines;
SQL
```

证据应分别记录原始备份校验、异地副本校验、隔离恢复、账务比对及 API 验收的结果。未执行的项目记为 UNVERIFIED。合成数据演练可用于验证流程和工具，生产备份由运维单独验收。

## 信用卡账单 IMAP 邮箱

信用卡账单邮箱使用独立的 IMAP 加密密钥。后端通过 systemd
`LoadCredential=credit-mail-key:/etc/tinynest/credentials/credit-mail-key` 加载，
也可通过 `TINYNEST_CREDIT_MAIL_KEY_FILE` 指定受保护的 32 字节随机密钥文件。
保留密钥的受限备份，后续部署沿用同一密钥。
家庭管理员在「信用账单 → 账单邮箱」测试并保存配置。后端须能出站访问邮箱的 TCP 993 端口。

## 托管邮箱 SMTP 服务

邮件动态配置、站点管理员授予、密钥备份与上线步骤见 [托管邮箱 SMTP 接入](feishu-mail.md)。代码已部署生产，真实 SMTP 待管理员在设置页配置并测试。
