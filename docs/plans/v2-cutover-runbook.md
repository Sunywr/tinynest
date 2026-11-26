# TinyNest V2 新库切换手册（Cutover Runbook）

> 状态：P5 执行口径（P4 交付）｜编写日期：2026-11-22｜执行人：生产维护者（用户）
> 重要声明：本手册中的命令未在文档编写环境执行；执行结果以生产侧输出与证据文件为准，未执行前一律 UNVERIFIED。
> 关联文档：[总计划](v2-bookkeeping-reset.md)、[记账模块规格](v2-bookkeeping-module-spec.md)、[生产清理手册](v2-prod-purge-runbook.md)
> 适用范围：从 v1 旧库切换到 V2 新库；只迁移 11 张身份表数据，业务数据按 V2 重置。

## 1. 目标与不变量

- 目标：旧库停写 → 导出身份数据 → 导入 V2 新库 → 切换数据库配置与二进制 → 冒烟通过，旧库保留为回滚点。
- 不变量：
  - 旧库在停服后不再写入，作为完整备份与回滚点保留 ≥30 天。
  - 新库由 V2 二进制从零建立基线（`schema_meta.baseline=v2`），不导入 `schema_migrations`。
  - 11 张身份表的计数与摘要在新旧库一致。
  - 加密密钥沿用同一 32 字节文件，否则 SMTP / IMAP 密文不可解。
- 边界：切换由用户执行；我们提供工具与手册，不接触生产凭据。

## 2. 前置条件

| 条件 | 要求 | 状态 |
| --- | --- | --- |
| 新二进制 | 已构建并通过 Go 测试、契约测试与迁移测试；确认不支持 v1 库原地升级 | UNVERIFIED |
| 旧库备份 | 已按 §3 步骤 1 完成全量备份并通过 SHA-256 校验 | UNVERIFIED |
| 新库 | 已创建空库；字符集与排序规则按部署标准 | UNVERIFIED |
| 受限账号 | 迁移账号（建基线用）与运行账号（仅 DML）已创建，来源范围收紧 | UNVERIFIED |
| 凭据注入 | `TINYNEST_MYSQL_PASSWORD_FILE` / `CREDENTIALS_DIRECTORY` 已就绪，口令不落文档与命令行 | UNVERIFIED |
| 加密密钥 | 与旧环境相同的 32 字节 `mail-key` / `credit-mail-key` 文件已就位并登记 | UNVERIFIED |
| 维护窗口 | 已确认停服窗口、值守人与回滚决策人 | UNVERIFIED |
| 回滚方案 | 旧二进制与旧库配置可恢复，回滚步骤已评审 | UNVERIFIED |

## 3. 切换步骤

### 步骤 1：旧环境全量备份

```bash
mysqldump \
  --defaults-extra-file=<旧库凭据文件> \
  --single-transaction \
  --no-tablespaces \
  --set-gtid-purged=OFF \
  --routines \
  --events \
  --triggers \
  --order-by-primary \
  <旧数据库名> > <备份目录>/<旧数据库名>-<时间戳>.sql
sha256sum <备份文件> | tee <备份文件>.sha256
sha256sum -c <备份文件>.sha256
```

要求：备份文件、SHA-256 文件与恢复命令写入证据；校验失败不得继续。

### 步骤 2：创建新库与受限账号

```sql
-- 口令从密码管理器注入，不写入文档与命令历史
CREATE DATABASE <新库名> CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;

CREATE USER '<迁移账号>'@'<应用来源>' IDENTIFIED BY '<强口令>';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, INDEX, REFERENCES
  ON `<新库名>`.* TO '<迁移账号>'@'<应用来源>';

CREATE USER '<运行账号>'@'<应用来源>' IDENTIFIED BY '<强口令>';
GRANT SELECT, INSERT, UPDATE, DELETE ON `<新库名>`.* TO '<运行账号>'@'<应用来源>';
```

说明：

- 迁移账号只在建立 V2 基线时使用；基线建立并核对后，应用切换为运行账号。
- `<应用来源>` 按实际部署收紧，不得使用任意主机通配。
- 若部署使用 SQLite，本节对应改为准备新的 SQLite 数据库文件路径（本地开发删除 `apps/api/data/tinynest.sqlite3` 后由二进制重建）。

### 步骤 3：新二进制首次启动，建立 V2 基线

```bash
TINYNEST_MYSQL_DATABASE=<新库名> TINYNEST_MYSQL_USER=<迁移账号> <新二进制路径>
```

首次启动会执行 V2 迁移，建立新表并写入 `schema_meta` 的 `baseline=v2`。核对后停止进程，进入后续步骤。

核对项：

```sql
SELECT `value` FROM schema_meta WHERE `meta_key` = 'baseline';  -- 期望 v2（列名是 `meta_key`，写成 `key` 实测报 `ERROR 1054 (42S22): Unknown column 'key' in 'where clause'`）
SELECT COUNT(*) AS table_count FROM information_schema.tables WHERE table_schema = '<新库名>';
```

期望：`baseline=v2`；迁移版本为 `[1..14]`；新库表数符合当前 V2 基线 **39 张业务表**——MySQL 运行时 **40 张**（另加迁移器创建的 `schema_migrations`），SQLite **41 张**（再因 `AUTOINCREMENT` 多出 `sqlite_sequence`）；启动日志无迁移错误与解密错误。若新库非空或 baseline 与二进制不符，启动应被拒绝——此时不得绕过守卫，必须处理或重建新库。

> 口径修正（2026-11-26）：本段此前写「27 张业务表、运行时 28 张」，那是迁移 006–012 加入之前的 V2 目标值。**按旧数判定会把建立成功的新库误读成缺表**，进而去「补建」或重建本该保留的基线。逐表清单以 [ER](../er-v2.md) §1（001 计 21、002 计 4、005 计 2、006 计 1、007 计 2、008 计 1、009–012 计 6、013 计 1、014 计 1，合计 39）为准，模块规格 §6 的 27/28 为冻结时的历史目标。
>
> **二进制必须内嵌当前迁移集**：只有包含 `013`/`014` 的二进制才建得出 `statement_placeholders` 与 `statement_details`。用旧二进制建库、再按本节核对会缺 1–2 张表；正确做法是升级二进制后重启，让启动期迁移补上，而不是手工建表、改 `schema_meta` 或改 baseline 守卫。反过来，内嵌新迁移的二进制在旧库上会被基线守卫拒绝启动，回滚必须同时切回旧库与旧二进制。

### 步骤 4：停止旧服务

- 停止旧 API 服务，确认旧库没有新写入。
- 记录停服时间，维护窗口从此刻开始计算。
- 停服后旧库只读用于导出，不得再启动旧二进制写数据。

### 步骤 5：导出 11 张身份表

```bash
tinynest-admin --driver mysql identity-export --out=<导出目录>/identity-<时间戳>.jsonl
sha256sum <导出目录>/identity-<时间戳>.jsonl | tee <导出目录>/identity-<时间戳>.jsonl.sha256
```

说明：

- 使用旧库连接配置执行导出；导出内容为 11 张身份表的 JSONL。
- 导出文件包含用户名与邮箱，按最小权限保管并限期删除；不得进入代码仓库。
- 导出后核对各表行数与文件大小，并记录到证据。

### 步骤 6：导入 V2 新库

```bash
# 使用新库连接配置执行导入；目标库必须已建立 V2 基线
tinynest-admin --driver mysql identity-import --in=<导出目录>/identity-<时间戳>.jsonl
```

要求：

- 目标库 `schema_meta.baseline=v2` 且身份表为空。
- 导入使用的账号需要写入权限；导入完成后应用切换为运行账号。
- 若导入中断，以工具事务行为为准（UNVERIFIED）；原则是不得在有残留行的新库上继续导入，应清理目标库身份表或重建新库基线后重做，并记录处置。

### 步骤 7：计数与摘要比对

在新库执行 §4 的比对 SQL，并与旧库结果逐行对比：

- 11 张身份表 `COUNT(*)` 完全一致。
- `users` 摘要一致（MySQL 用 `CRC32` 摘要，SQLite 用长度与首尾 ID 摘要）。
- 新库身份表之外无业务数据；旧库 `schema_migrations` 不参与比对，由新二进制重建。
- 若选择不迁移会话（强制用户重新登录），`sessions` 的差异属于预期口径，必须在证据中明确记录并经用户确认；默认口径为完整迁移 11 张表。

### 步骤 8：切换数据库配置

- 更新部署配置：`TINYNEST_MYSQL_DATABASE=<新库名>`、`TINYNEST_MYSQL_USER=<运行账号>`。
- 凭据文件改为运行账号口令；保持同一 32 字节加密密钥文件与路径。
- 确认旧库配置不再被引用；确认没有双写或回退到旧库的定时任务。
- 该步骤只改配置，不启动服务；启动在步骤 9。

### 步骤 9：启动冒烟

```bash
# 就绪检查
curl -sS -o /dev/null -w '%{http_code}\n' <站点地址>/api/v1/readyz    # 期望 200（裸 /readyz 是 404）

# 登录：请求体放 600 权限文件，避免口令进入命令行历史
curl -sS -c <临时 Cookie 文件> -H 'Content-Type: application/json' \
  -X POST <站点地址>/api/v1/auth/session --data-binary @<登录请求文件>

# 当前用户
curl -sS -b <临时 Cookie 文件> <站点地址>/api/v1/auth/me

# 当前家庭成员切换
curl -sS -b <临时 Cookie 文件> -X POST <站点地址>/api/v1/household/members/current \
  -H 'Content-Type: application/json' --data-binary @<成员请求文件>

# 记一笔（使用幂等键）
curl -sS -b <临时 Cookie 文件> -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: <唯一键>' -X POST <站点地址>/api/v1/transactions \
  --data-binary @<记账请求文件>

# 统计
curl -sS -b <临时 Cookie 文件> '<站点地址>/api/v1/stats/summary?month=<YYYY-MM>'
```

| 冒烟项 | 期望结果 |
| --- | --- |
| `GET /api/v1/readyz` | 200（裸 `/readyz` 实测 404，不算通过） |
| 登录 `POST /api/v1/auth/session` | 200，Set-Cookie 正常 |
| `GET /api/v1/auth/me` | 返回导入的用户与家庭 |
| `POST /api/v1/household/members/current` | 200，当前成员切换成功 |
| 记一笔 `POST /api/v1/transactions` | 201 / 200，流水与统计可见 |
| `GET /api/v1/stats/summary` | 金额与该笔流水一致 |
| 服务日志 | 无迁移错误、无解密错误、无 5xx |

冒烟产生的测试流水必须当场用 `DELETE /api/v1/transactions/{id}` 冲正，并在证据中记录；不得把测试数据留在生产账本。

### 步骤 10：旧库保留为回滚点

- 旧库与旧库备份保留 ≥30 天，期间不删除、不覆盖。
- 记录旧库位置、备份 SHA-256、保留到期时间与回滚决策人。
- 观察期内监控 `readyz`、登录、记账与统计；异常时按 §6 回滚。

## 4. 比对 SQL

### 4.1 11 张身份表计数（MySQL 与 SQLite 通用）

```sql
SELECT 'households' AS table_name, COUNT(*) AS n FROM households
UNION ALL SELECT 'users', COUNT(*) FROM users
UNION ALL SELECT 'sessions', COUNT(*) FROM sessions
UNION ALL SELECT 'pending_registrations', COUNT(*) FROM pending_registrations
UNION ALL SELECT 'site_administrators', COUNT(*) FROM site_administrators
UNION ALL SELECT 'email_identities', COUNT(*) FROM email_identities
UNION ALL SELECT 'email_challenges', COUNT(*) FROM email_challenges
UNION ALL SELECT 'mail_configuration', COUNT(*) FROM mail_configuration
UNION ALL SELECT 'mail_preferences', COUNT(*) FROM mail_preferences
UNION ALL SELECT 'mail_daily_usage', COUNT(*) FROM mail_daily_usage
UNION ALL SELECT 'mail_recipient_daily_usage', COUNT(*) FROM mail_recipient_daily_usage;
```

新旧库各执行一次，逐行对比 `n` 必须完全一致。

### 4.2 `users` 摘要校验（MySQL）

```sql
SELECT
  COUNT(*) AS user_count,
  SUM(CHAR_LENGTH(id)) AS id_len,
  SUM(CHAR_LENGTH(username)) AS username_len,
  BIT_XOR(CRC32(CONCAT_WS('|', id, username, role, status))) AS users_digest
FROM users;
```

### 4.3 `users` 摘要校验（SQLite）

SQLite 默认不提供 `CRC32` 扩展函数，改用无需扩展的摘要组合：

```sql
SELECT
  COUNT(*) AS user_count,
  TOTAL(LENGTH(id)) AS id_len,
  TOTAL(LENGTH(username)) AS username_len,
  MIN(id) AS min_id,
  MAX(id) AS max_id
FROM users;
```

说明：摘要用于快速发现差异，若摘要不一致，再按 `id` 逐行抽样比对定位；不得只凭摘要判断导入成功。

### 4.4 迁移与基线核对（仅新库）

```sql
SELECT `value` FROM schema_meta WHERE `meta_key` = 'baseline';  -- 期望 v2（列名是 `meta_key`，写成 `key` 实测报 `ERROR 1054 (42S22): Unknown column 'key' in 'where clause'`）
SELECT COUNT(*) AS versions FROM schema_migrations;        -- 由新二进制重建，不要求与旧库一致
```

## 5. 注意与约束

- V2 基线为重置：旧库不兼容新二进制。旧二进制也不能直接打开 V2 新库；回滚必须同时切回旧库与旧二进制。
- 身份表列集合必须一致：导出 / 导入前核对 11 张表的列集合；列集合不一致时导入应失败，需先修 V2 迁移或改用工具支持的显式映射（若支持，UNVERIFIED）。
- 加密密钥沿用同一 32 字节文件：新旧环境使用同一 `mail-key` / `credit-mail-key`，否则 `mail_configuration.password_cipher` 等密文不可解；切换前确认密钥文件内容一致（比对 SHA-256，不打印内容）。
- `schema_migrations` 不迁移：新库由新二进制重建 V2 版本序列；不得把旧库迁移记录复制到新库。
- 切换窗口内禁止双写：停服后旧库只读；新库在冒烟前不得被其他实例连接。
- 会话口径：默认完整迁移 `sessions`；若改为强制重新登录，必须记录为预期差异并通知用户。

## 6. 回滚

回滚 = 切回旧库 + 旧二进制：

1. 停止 V2 服务。
2. 部署配置改回旧库：`TINYNEST_MYSQL_DATABASE=<旧库名>`、`TINYNEST_MYSQL_USER=<旧账号>`，凭据文件恢复为旧账号口令，密钥文件保持原路径。
3. 启动旧二进制。
4. `GET /api/v1/readyz` 返回 200；管理员登录、`/auth/me`、家庭成员与统计冒烟通过。
5. 记录回滚原因、时间、操作人与验证结果；新库保留备查，暂不删除。

回滚风险：

- 旧库在停服后未写入，回滚本身不丢旧数据。
- 若切换成功后已在新库产生新账务，回滚会丢失新库数据；此时必须先评估新库数据处置（导出 / 人工记录），再决定是否回滚。
- 回滚后旧库继续保留 ≥30 天；新库与备份按证据要求归档，不得立即删除。

## 7. 证据与验收

证据写入 `docs/reviews/v2-purge-evidence.md` 或独立的切换记录，字段包括：执行人、时间线（停服 / 导出 / 导入 / 切换 / 冒烟 / 恢复服务）、旧库备份 SHA-256、identity JSONL SHA-256、11 张表前后计数、`users` 摘要、`readyz` 与冒烟结果、旧库保留到期时间、回滚决策与回滚点。

验收清单：

- [ ] 旧环境全量备份完成并通过 SHA-256 校验。
- [ ] 新库已创建，迁移账号与运行账号权限符合最小权限原则。
- [ ] 新二进制首次启动建立 `baseline=v2`，表数与模块规格一致。
- [ ] 旧服务已停止，停服时间已记录。
- [ ] identity JSONL 已导出并校验 SHA-256。
- [ ] identity JSONL 已导入新库，身份表计数与 `users` 摘要一致。
- [ ] 配置已切换到 `TINYNEST_MYSQL_DATABASE` 与 `TINYNEST_MYSQL_USER`。
- [ ] `GET /api/v1/readyz` 200；登录、`/auth/me`、`/household/members/current`、记一笔、统计冒烟通过。
- [ ] 冒烟测试流水已冲正并记录。
- [ ] 旧库保留 ≥30 天并登记回滚点。
- [ ] 证据文件完整，UNVERIFIED 项已标注。

## 8. UNVERIFIED 清单

- 全部命令与 SQL 未在本次文档编写环境执行，UNVERIFIED。
- 新二进制版本、新库表数与 `baseline=v2` 的实际输出 UNVERIFIED。
- 11 张身份表的实际列集合是否完全一致，未在生产数据上核对，UNVERIFIED。
- 导入工具在中断场景下的事务行为 UNVERIFIED。
- 备份、导出、导入、冒烟的实际耗时与结果 UNVERIFIED。
- iPhone 实机与异地副本的切换后验收 UNVERIFIED，需用户后续补齐。
