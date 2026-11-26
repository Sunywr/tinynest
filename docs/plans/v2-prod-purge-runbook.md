# TinyNest V2 生产清理手册（Purge Runbook）

> 状态：P5 执行口径（P4 交付）｜编写日期：2026-11-22｜表数与 CLI 口径修订：2026-11-25（按代码 `PurgeGroups` 与生产只读核对）｜执行人：生产维护者（用户）
> 重要声明：本手册中的命令未在文档编写环境执行；任何「验证通过」结论必须由生产执行时的输出与证据文件支撑，未执行前一律 UNVERIFIED。只读核对结果（表名、行数、迁移版本）见 §9 与[发布证据](../reviews/v2-statement-ui-deployment-evidence.md)。
> 关联文档：[总计划](v2-bookkeeping-reset.md)、[记账模块规格](v2-bookkeeping-module-spec.md)、[新库切换手册](v2-cutover-runbook.md)、[备份脚本模板](../../deploy/backend/backup-mysql.sh.example)
> 适用范围：生产库（MySQL 或 SQLite）的业务数据清理，既覆盖仍是 V1 结构的旧库，也覆盖已建 `baseline=v2` 的当前库；目标是在不删除身份数据的前提下清空业务数据，并提供可验证、可回滚的执行路径。

## 1. 目标、边界与清理语义

- 目标：生产库业务数据清空，保留 11 张身份表与 2 张含加密授权码的个人配置表（§4.1、§4.2）；清理前后这些表的计数与迁移版本一致，`verify` 全绿。
- 边界：生产清理由用户执行（决策①）；我们交付工具与手册，不接触生产凭据、不代执行、不保存生产数据。
- 清理语义：只删除数据行（DML），不执行 DDL。原因：MySQL 的 `TRUNCATE` / `DROP` 会隐式提交，无法满足「单事务执行、失败整体回滚」；v1 库清空后保留空表结构，V2 的基线目标由新库承担（空库迁移后 39 张表，MySQL 运行时含 `schema_migrations` 为 40 张，SQLite 另因 `AUTOINCREMENT` 多出 `sqlite_sequence` 为 41 张；见切换手册）。
- 不变量：11 张身份表数据完整；`journal_lines` 借贷合计在清理前采集、清理后归零并可核对；迁移版本不变；备份与旧库保留 ≥30 天。

## 2. 安全不变量与前置条件

| 编号 | 不变量 | 说明 |
| --- | --- | --- |
| S1 | 凭据只走文件 | 只使用 `TINYNEST_MYSQL_PASSWORD_FILE` / `CREDENTIALS_DIRECTORY`；不接受命令行密码；日志不得打印连接串与凭据 |
| S2 | 服务必须停止 | `purge -execute` 必须带 `-yes-service-stopped`；执行前确认应用进程已停止、健康检查不可达 |
| S3 | 库名必须二次确认 | `-confirm=<DBNAME>` 必须与当前连接的数据库名完全一致，否则中止 |
| S4 | 单事务 | 全部删除在同一事务内完成；任一步失败整体回滚，不得留下半清理状态 |
| S5 | 身份表白名单 | 11 张身份表与 2 张保留的个人配置表永不进入删除清单；`verify` 与人工核对双重确认 |
| S6 | 先备份后清理 | 备份、SHA-256 校验、隔离恢复比对全部通过后才允许 `-execute`；备份必须导出**运行时库**（见步骤 2 的库名断言） |
| S7 | 证据留痕 | 执行人、时间、备份 SHA-256、前后计数写入 `docs/reviews/v2-purge-evidence.md`（用 `-json-out` 落盘 dry-run 与 verify 的 JSON） |
| S8 | 保留期 | 旧库与备份保留 ≥30 天；期间不得删除任何回滚材料 |
| S9 | 删除不按家庭过滤 | `purge` 的每条 `DELETE FROM <表>` 都是无条件全表删除；`-household=<id>` **不会**把清理限制在该家庭，只用于在检测到多家庭时确认继续并选择重播种目标。库里有多个家庭时，全部家庭的业务行都会被清空 |

前置条件：

- 已确认执行窗口与值守人；生产访问权限由用户持有。
- 已确认数据库名、字符集与驱动（SQLite / MySQL）。库名以服务的 `TINYNEST_MYSQL_DATABASE` 为准，不采信文档或脚本里的历史值。
- **已核对本工具适用前提**：`purge` 的设计前提是「生产库里只有一个家庭」。2026-11-25 只读核对生产 `tinynest_v2` 现有 **3 个家庭**，属于 S9 描述的多家庭情形：不加 `-household` 时 CLI 直接拒绝，加上后删除仍覆盖全部 3 个家庭。若只想清理其中一个家庭，本工具做不到，需另定方案并在证据文件中记录。
- 已备份 32 字节 `mail-key` / `credit-mail-key` 文件并登记保管人（丢失后 `mail_configuration.password_cipher` 与 `mail_queue.payload`、`statement_mailbox_configs` 的授权码密文不可解）。
- 已在证据文件中预留记录位置，并确认基线文件、备份文件、dry-run 输出的归档目录。

## 3. 运维 CLI 接口（最终实现口径）

```text
tinynest-admin [-driver sqlite|mysql] <command> [flags]

baseline-capture   -out=<path>                       采集清理前的身份计数、迁移版本与借贷合计
purge              [-dry-run]                        只打印将删除的表与行数
purge              -execute -confirm=<db> -yes-service-stopped [flags]   执行清理
verify             [-baseline=<path>]                只读校验：业务表全空且身份数据未变
identity-export    -out=<path>                       导出身份表为 JSON 行
identity-import    -in=<path> [-dry-run]             把身份数据导入空的 V2 基线库
seed-categories    [-household=<id>]                 重播默认分类并补齐图标与颜色
```

参数说明：

| 命令 / 参数 | 说明 |
| --- | --- |
| `-driver sqlite` / `-driver mysql` | 按部署驱动选择；默认值由 CLI 决定，执行前显式写出 |
| `baseline-capture -out=<path>` | 采集身份表计数、保留配置表计数、迁移版本、`journal_lines` 借贷合计与 `users` 摘要，生成基线文件；不修改数据库。出现未分类表时打印 ⚠ 提示 |
| `purge -dry-run` | 只读；打印将清理的表与行数、当前 schema 中不存在的表、以及多家庭警告；不得写库 |
| `purge -execute` | 执行清理；必须同时提供 `-confirm=<DBNAME>` 与 `-yes-service-stopped` |
| `-household=<id>` | 库中存在多个家庭时**必须**给出，否则 CLI 拒绝执行。注意它不改变删除范围：所有 `DELETE` 都不带家庭条件，见 S9；该 ID 只决定 `-reseed-categories` 之后重播种哪些家庭 |
| `-keep-audit` | 保留 `audit_logs` 与 `schedule_runs`（其余照清）。**与 `verify` 不兼容**：`verify` 没有对应的放宽开关，保留审计行后「业务表全空」断言必定失败（`TestPurgeKeepAuditPreservesHistory` 把这一行为固定为预期）。因此本次切换**不要**使用该开关，否则拿不到 §7 要求的全绿证据 |
| `-reseed-categories` | 清理后重新播种默认分类。**默认关闭**：关闭时「所有业务表为空」的校验最严格；只有需要清理后继续使用界面时才开启，且开启后须用 `verify -allow-seeded-categories` 校验 |
| `-allow-unbalanced` | 分录已不平衡（或缺行、单条分录借贷不等）时仍继续。默认拒绝，避免用清理掩盖既有账务缺陷 |
| `-json-out=<path>` | 把本次 `PurgeResult` / 快照 JSON 写入该路径作为证据；建议 dry-run、execute、verify 各存一份 |
| `verify -baseline=<path>` | 只读校验；对比基线，确认身份与保留配置计数未变、业务表已清空、迁移版本一致 |
| `identity-export -out=<path>` | 导出 11 张身份表为 JSONL；含用户名与口令哈希，按机密保管（CLI 会提示 0600） |
| `identity-import -in=<path>` | 导入 JSONL；目标库必须已建立 V2 基线（`schema_meta.baseline=v2`）。保留配置表不在导出范围内，是否单独搬运由执行人判断 |

示例（占位符需按生产实际替换，执行前逐条核对）：

```bash
tinynest-admin -driver mysql baseline-capture -out=<备份目录>/baseline.json
tinynest-admin -driver mysql purge -dry-run -json-out=<归档目录>/dry-run.json
tinynest-admin -driver mysql purge -execute -confirm=<数据库名> -yes-service-stopped -json-out=<归档目录>/execute.json
tinynest-admin -driver mysql verify -baseline=<备份目录>/baseline.json -json-out=<归档目录>/verify.json
```

## 4. 清理分组与保留清单

### 4.1 保留的 11 张身份表

| 表 | 作用 |
| --- | --- |
| `households` | 家庭 |
| `users` | 用户 |
| `sessions` | 会话 |
| `pending_registrations` | 待审批注册 |
| `site_administrators` | 站点管理员 |
| `email_identities` | 邮箱身份 |
| `email_challenges` | 邮箱挑战 |
| `mail_configuration` | 站点邮件配置（SMTP） |
| `mail_preferences` | 邮件偏好 |
| `mail_daily_usage` | 邮件日用量 |
| `mail_recipient_daily_usage` | 收件人日用量 |

### 4.2 保留的个人配置与元数据表

| 表 | 处理 | 说明 |
| --- | --- | --- |
| `statement_mailbox_configs` | 保留，不进入身份导出 | 迁移 011 的账单邮箱配置，含 AES-GCM 密文授权码，需 32 字节 `mail-key` 才能解 |
| `credit_statement_mailboxes` | 保留，不进入身份导出 | 迁移 009 拆分前的旧邮箱表，011 复制数据但不删表；新代码不再读写 |
| `auth_login_locks` | 保留 | 登录风控是安全状态，清空等于放松爆破防护 |
| `schema_meta`、`schema_migrations` | 保留 | 基线标记与迁移账本；清理不得触碰，否则 `internal/db/migrate.go` 的基线守卫拒绝启动 |

口径来源：`PreservedConfigTables`（2 张）与 `IdentityTables`（11 张）在代码里分开定义，`identity-export` 只导出后者。切换新库时是否单独搬运邮箱授权码由执行人判断并写入证据；`sqlite_sequence` 仅在 SQLite 出现，同样不计入业务表。

### 4.3 清理分组（16 组、56 个表名）

`internal/dbadmin/tables.go` 的 `PurgeGroups` 是唯一事实来源。下表逐组照抄代码，「存在」列是 2026-11-26 在当前工作树（V2 基线 + 迁移 `013`/`014`，39 张表）上以 `PurgeTableSet()` + `MissingPurgeTables()` 实测的命中数，† 标记的表名在当前 schema 中已不存在。

| 分组 | 表名 | 存在 |
| --- | --- | --- |
| 结构与账本 | `categories`、`accounts`、`ledger_accounts`、`projects`† | 3/4 |
| 复式分录 | `journal_entries`、`journal_lines`、`account_opening_balances` | 3/3 |
| 记账数据 | `transactions`、`budgets`、`project_budgets`、`transaction_budget_links`、`transaction_source_metadata`、`transaction_splits`†、`refunds`† | 5/7 |
| 信用账单 | `credit_instruments`†、`credit_statements`†、`credit_statement_items`†、`credit_payments`†、`credit_instrument_changes`†、`credit_statement_adjustments`†、`credit_statement_provisions`† | 0/7 |
| 银行账单与绑卡 | `statement_placeholders`、`statement_details`、`card_statement_sources`、`card_statements`、`statement_mailbox_card_links`、`credit_statement_bindings` | 6/6 |
| 分期 | `installment_contracts`†、`installment_schedule_items`†、`installment_payments`†、`installment_adjustments`† | 0/4 |
| 负债 | `debt_contracts`†、`debt_schedule_items`†、`debt_payments`† | 0/3 |
| 到期调度 | `schedule_due_items`† | 0/1 |
| 投资 | `investment_assets`†、`investment_fee_rules`†、`investment_trades`†、`investment_dividends`†、`investment_valuations`†、`investment_positions`† | 0/6 |
| 投资估值快照 | `investment_holdings`、`investment_accounts` | 2/2 |
| 信用卡账单邮件 | `credit_mailboxes`†、`credit_mail_bindings`†、`credit_mail_previews`† | 0/3 |
| 账单导入 | `statement_import_rows`†、`statement_import_bindings`† | 0/2 |
| 股票自选 | `stock_watch`† | 0/1 |
| 待发邮件 | `mail_queue` | 1/1 |
| 运营与审计 | `audit_logs`、`schedule_runs`†、`stock_quotes`† | 1/3 |
| 成员档案与邀请 | `household_members`、`household_user_members`、`household_invites` | 3/3 |

当前 V2 基线中真实被清理的 24 张表：`categories`、`accounts`、`ledger_accounts`、`journal_entries`、`journal_lines`、`account_opening_balances`、`transactions`、`budgets`、`project_budgets`、`transaction_budget_links`、`transaction_source_metadata`、`statement_placeholders`、`statement_details`、`card_statement_sources`、`card_statements`、`statement_mailbox_card_links`、`credit_statement_bindings`、`investment_holdings`、`investment_accounts`、`mail_queue`、`audit_logs`、`household_members`、`household_user_members`、`household_invites`。

其余 32 个表名是 V1 模块遗留（分期、负债、到期调度、V1 投资交易与股票、V1 信用账单与账单导入等），在 V2 空库上不存在，执行时逐项列为「已跳过」。注意「银行账单与绑卡」和「投资估值快照」两组是 V2 之后重新引入的**只读**数据，与 V1 同名模块不是一回事，本轮按用户口径计入清理范围：账单+绑卡+投资快照清理，邮箱配置保留。

口径说明：56 个表名 = 24 张存在 + 32 个不存在，另有 11 张身份表 + 2 张保留配置表 + `auth_login_locks` + `schema_meta` 不计入清理，合计覆盖 V2 基线的 39 张表。删除顺序由 `DeleteOrder()` 固定为子表先于父表，外键全程不禁用，因此「删干净」本身就是没留孤儿行的证明；`statement_placeholders` 与 `statement_details` 同时引用账单与流水，因此排在本组最前、且早于 `transactions`、`accounts` 与分录表。执行时一律以 `purge -dry-run` 的实际输出为准：新迁移加的表若未归类，会出现在 `Unclassified` 警告里并让 `verify` 失败，不会静默漏清。成员档案（`household_members`、`household_user_members`、`household_invites`）在代码里已列入清理组，依据是「用户下次登录由 `/household/members/current` 自动重建档案」；这不是用户单独确认过的口径，执行人若要求保留成员档案，必须在 dry-run 核对阶段明确提出并改用其他方案，否则按本节口径清空。

### 4.4 清理后行为说明

- `categories` 默认清理为空（严格口径）。若需清理后继续使用 V2 界面，加 `-reseed-categories` 重新播种默认分类，并用 `verify -allow-seeded-categories` 校验；不加则需手工创建分类。
- `accounts` 清空后必须先建账户才能记账，属预期行为，不是故障。
- `mail_queue` 清空后待发邮件丢失，属预期。
- `investment_accounts` / `investment_holdings` 是从 pfm 导入的只读估值快照，清空后资产页的投资市值与含投资的净资产显示为空，需重新导入才有值；它们不参与分录，清理不影响账本余额。
- `card_statements` / `card_statement_sources` 是银行报的账期汇总，清空后账单页的并排比对回到「无银行数据」，需重新同步邮箱才有值。
- `statement_placeholders`（013）只是「银行账单 ↔ 待补明细占位流水」的关联：它排在 `card_statements` 与 `transactions` 之前删除，因此清理后不会留下悬空关联。由占位产生的那笔「待补明细」支出连同其分录随 `transactions` / `journal_*` 一起清空，`categories` 里的「待补明细」分类也随分类清理消失（清理后该分类不会被重建，除非再次生成占位或手工创建）。
- `statement_details`（014）逐行登记上传过的银行账单明细：它与占位关联同批排在 `card_statements`、`transactions`、`accounts` 之前删除，因此不会留下指向已删流水的行。由补录产生的每笔「账单补录」支出随 `transactions` / `journal_*` 一起清空，`categories` 里的「账单补录」分类同样随分类清理消失；`row_key`+`seq` 的历史一并清除，所以清理后重新上传同一份文件会被当成首次入账（这是预期，不是重复计账缺陷）。
- 加密配置依赖 32 字节 `mail-key`：`mail_configuration.password_cipher`、`mail_queue.payload` 与保留的 `statement_mailbox_configs.auth_code_cipher` 都用它加解密，密钥丢失即不可解。`credit-mail-key` 属已退役的 V1 信用账单模块，仅在读取旧库密文时才需要。
- 清理只删数据，不执行 DDL；当前 schema 中不存在的 V1 遗留表名不产生任何语句，旧库的空表结构原样保留，便于回滚与切换前导出。

## 5. 执行流程

### 步骤 0：准备与确认

- 确认执行窗口、值守人、数据库名与驱动。
- 准备归档目录：备份文件、SHA-256 文件、基线文件、dry-run 输出、verify 输出。
- 确认 32 字节密钥文件已备份并登记保管人。

### 步骤 1：停止服务

- 停止 API 服务（按生产部署方式执行；具体停服命令由运维环境决定）。
- 确认服务不可达后继续；不得在服务运行中执行清理。
- **实测注意（2026-11-22）**：B 机的 loopback TLS 反代单元声明了 `Requires=<API 单元>`，因此停在 API 会**级联停掉反代**，18444 端口随之消失、A 机隧道转发报 `SSL_ERROR_SYSCALL`。清理完成后必须**同时启动两个单元**（例如 `systemctl start <API 单元> <反代单元>`），再验证 `readyz` 与公开站点；只启动 API 会让线上一直 502。

### 步骤 2：全量备份

备份对象必须是**服务运行时正在用的那个库**，不是历史库名。2026-11-25 核对到生产定时备份连续三天导出已退役的 V1 库而不报错，原因就是把库名写死在运维脚本里。因此优先直接使用随仓库发布的 [deploy/backend/backup-mysql.sh.example](../../deploy/backend/backup-mysql.sh.example)（它会比对服务的 `TINYNEST_MYSQL_DATABASE`、断言转储里含 `schema_meta` 的 baseline 行、写 `.sha256` sidecar 并自检）。

**2026-11-25 起生产已在跑这份模板**：B 机运维脚本已替换为该文件（`sh -n` 通过，`0750 root:root`，原脚本留有 `.bak`），备份 service 的 drop-in 补上了 `BACKUP_DIR` 与三个 `TINYNEST_MYSQL_*` 变量，并把手柄描述从「SQLite backup」改为 MySQL；`Description=` 必须写在 `[Unit]` 段，写在 `[Service]` 段会被 systemd 静默忽略。替换后现场 `systemctl start` 三次均 `Result=success`、`ExecMainStatus=0`，产出 12,859,119 字节的 `tinynest_v2-*.sql` + sidecar，`sha256sum -c` 为 OK。库名不一致的负例也在现场验证过：脚本在 `mysqldump` 之前退出 1 且不写出任何转储。下面这段手工命令只作为模板不可用时的兜底口径保留。

```bash
# 口令由受限凭据文件提供，不出现在命令行
mysqldump \
  --protocol=SOCKET --socket=<MySQL Unix socket> \
  --user=<运行账号> \
  --default-character-set=utf8mb4 \
  --single-transaction \
  --no-tablespaces \
  --set-gtid-purged=OFF \
  --skip-add-drop-table \
  <运行时数据库名> > <备份目录>/<运行时数据库名>-<时间戳>.sql
sha256sum <备份目录>/<运行时数据库名>-<时间戳>.sql | tee <备份目录>/<运行时数据库名>-<时间戳>.sql.sha256
sha256sum -c <备份目录>/<运行时数据库名>-<时间戳>.sql.sha256
```

要求：

- `--single-transaction` 必需：运行账号只有 DML 权限，缺省加锁会报 `Got error: 1044: Access denied ... when doing LOCK TABLES`（实测）。
- 手工执行时同样要断言转储含 `INSERT INTO \`schema_meta\` VALUES ('baseline','v2');`；导错库或空库时 `mysqldump` 会正常退出 0，只表现为转储体积小一个数量级。
- 备份文件与 SHA-256 文件分开归档；SHA-256 记入证据文件；校验失败不得继续。
- 备份脚本若由 systemd 定时执行，核对上一轮的实际产物（文件大小与时间戳），不要只看 unit 的 `SUCCESS`。

### 步骤 3：恢复到隔离实例并比对

转储不含 `CREATE DATABASE`（未加 `--databases`），所以先在**隔离实例**上建一个明确命名的空库再导入，禁止指向运行时库：

```bash
mysql --defaults-extra-file=<隔离实例凭据文件> -e "CREATE DATABASE IF NOT EXISTS <隔离库名> CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"
mysql --defaults-extra-file=<隔离实例凭据文件> <隔离库名> < <备份文件>
```

导入后用同一份运维 CLI 对隔离库采基线，与清理前采集的生产基线逐字段比对，比手工 `COUNT(*)` 更难漏项：

```bash
TINYNEST_MYSQL_DATABASE=<隔离库名> tinynest-admin -driver mysql baseline-capture -out=<归档目录>/restored-baseline.json
```

| 检查项 | 命令或方式 | 期望 |
| --- | --- | --- |
| V2 基线标记 | `SELECT value FROM schema_meta WHERE meta_key='baseline';` | `v2` |
| 迁移版本 | `SELECT version FROM schema_migrations ORDER BY version;` | 与基线记录逐值一致（当前为 1–13） |
| 表数 | `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='<隔离库名>';` | 与生产基线一致（内嵌 `001`–`014` 的二进制为 MySQL 运行时 **40 张**，含 `schema_migrations`；生产若仍停在 `013` 则是 39 张，一律以生产实际 `schema_migrations` 版本为准） |
| 身份计数 | §4.1 的 11 张表逐一 `COUNT(*)`，或直接比对 `restored-baseline.json` 的 `identity_tables` | 与基线一致 |
| 保留配置计数 | §4.2 的 2 张表 `COUNT(*)` | 与基线一致 |
| `users` 摘要 | 比对 `users_digest` | 完全相同（口令哈希、角色、状态的 SHA-256） |
| 账本规模 | `journal_entries` / `journal_lines` / `transactions` 的 `COUNT(*)` | 与基线逐值一致 |
| 借贷合计 | `SELECT SUM(debit_minor), SUM(credit_minor) FROM journal_lines;` | 两值相等，且与基线的**具体数字**一致 |
| 业务数据在不在 | `SELECT COUNT(*) FROM card_statements;`、`FROM budgets;` | 与基线一致（用于证明导的是有数据的库，不是空壳） |

单一「借贷平衡」不足以证明恢复完整：空库也平衡。必须比对具体数字与摘要。比对通过后才进入下一步；任一项不一致必须停止并排查备份或实例问题。演练结束后按数据保留策略删除隔离库并记录，不在隔离库上启动 API 之外的写操作。

### 步骤 4：采集基线

```bash
tinynest-admin -driver mysql baseline-capture -out=<备份目录>/baseline.json
```

基线文件记录：身份表计数、迁移版本、`journal_lines` 借贷合计、采集时间。基线文件必须另存副本，`verify` 失败排查时不得覆盖原基线。

### 步骤 5：dry-run 人工核对

```bash
tinynest-admin -driver mysql purge -dry-run -json-out=<归档目录>/dry-run.json
```

多家庭库（含 2026-11-25 实测的生产 `tinynest_v2`，3 个家庭）不加 `-household` 时该命令直接**拒绝**并返回错误，此时不会打印任何表清单；确认要清空全部家庭后，改用 `purge -dry-run -household=<id>` 重跑取证——这一步仍然只读，`Purge` 在 `DryRun` 分支返回前不进入事务。

人工核对清单：

- [ ] 输出中的表清单与 §4.3 的 56 个表名一致，「已跳过」列表与 §4.3 的 † 项一致，无遗漏、无多余表。
- [ ] 输出包含每张表的预计删除行数，且与步骤 2 转储、`baseline-capture` 的计数互相印证。
- [ ] 11 张身份表与 §4.2 的 2 张保留配置表均未出现在删除清单中。
- [ ] 未出现「未分类表」警告；出现即说明有新迁移的表未归类，先补归类再继续。
- [ ] 多家庭库已按 S9 核对：`-household=<id>` **不缩小**删除范围，输出中的警告确认所有家庭的业务行都会被清空；若这不是预期结果，停止执行并另定方案。
- [ ] 输出为只读行为，核对期间数据库无写入（服务已停），可再跑一次 dry-run 比对逐值相同。

### 步骤 6：执行清理

```bash
tinynest-admin -driver mysql purge -execute -confirm=<数据库名> -yes-service-stopped
```

要求：

- 命令在单事务内执行；出现任何错误即整体回滚，记录错误输出后排查。
- 执行期间禁止重试并发清理；一次执行结束后再判断状态。
- 执行完成后立即执行 `verify`，不要先启动服务。

### 步骤 7：校验

```bash
tinynest-admin -driver mysql verify -baseline=<备份目录>/baseline.json
```

`verify` 必须确认：

- 11 张身份表计数与迁移版本与基线一致。
- §4.2、§4.3 的清理表业务数据为空（身份表与保留结构除外）。
- 退出码为 0；任一不一致即视为失败，不得进入下一步。

### 步骤 8：启动 API 并检查就绪

- 启动 API 服务。
- `GET /api/v1/readyz` 返回 200；`GET /api/v1/healthz` 返回 200。两条健康路由注册在 `/api/v1` 前缀下（`internal/server/server.go`），2026-11-25 在生产实测裸 `/readyz` 与 `/healthz` 均为 **404**，把它们当作就绪信号会误判。
- 只启动 API 还不够：B 机的 loopback 反代是 `Requires=<API 单元>` 的独立单元，必须一并启动，再用 A 机 HTTPS 侧复核 `/api/v1/readyz`。
- 检查启动日志无迁移错误、无解密错误。

### 步骤 9：管理员登录与家庭成员冒烟

- 管理员登录：`POST /api/v1/auth/session`，确认返回 200 并拿到会话 Cookie。
- 访问 `GET /api/v1/auth/me`，确认用户与家庭信息正确。
- 调用 `POST /api/v1/household/members/current`，确认当前成员切换正常。
- 若家庭成员表按清空重建处理，确认成员列表为空且可以重新创建成员。

### 步骤 10：保留备份与旧库

- 备份文件、SHA-256 文件、基线文件、dry-run 与 verify 输出归档。
- 旧库与备份保留 ≥30 天；期间不得删除任何回滚材料。
- 清理后的数据库在观察期内保持只读检查，确认无异常写入。

### 步骤 11：记录证据

将执行人、时间、备份 SHA-256、前后计数、verify 输出、`readyz` 结果写入 `docs/reviews/v2-purge-evidence.md`（证据字段见 §7）。

## 6. 回滚

| 情形 | 处置 |
| --- | --- |
| `purge -execute` 执行失败 | 单事务已整体回滚；核对 `verify` 输出与数据库状态，修复后重新执行 |
| 清理已提交但需要回滚 | 停止服务，从备份恢复数据库，校验身份计数与迁移版本，再启动 v1 服务 |
| 清理后发现数据异常 | 立即停止服务；以备份为准恢复；保留现场日志与 dry-run/verify 输出 |
| 密钥文件丢失 | 无法从数据库恢复密文；只能重新配置 SMTP / IMAP（标注为不可自动恢复） |
| 回滚完成后 | 重新执行 `baseline-capture` 与 `verify`，确认恢复到清理前状态，并记录回滚原因与时间 |

回滚原则：已提交后的回滚等于从备份恢复，不做逆向补写；旧库与备份至少保留 30 天。

## 7. 边缘情况

| 场景 | 判定信号 | 处置 |
| --- | --- | --- |
| 清理中断 | CLI 非零退出或连接中断 | 单事务自动回滚；确认 `verify` 与计数未变后重跑；不得在未知状态下重复执行 |
| `-confirm` 不符 | CLI 报错并中止 | 核对当前连接数据库名与实际库名；修正后重新 dry-run，不得绕过确认 |
| 多家庭库未指定 `-household` | CLI 检测到多个家庭并中止，不打印表清单 | 先按 S9 判定范围：`-household=<id>` 不缩小删除范围，加上后所有家庭的业务行都会被清空。要保留任何家庭就停止并另定方案 |
| 服务未停 | 缺少 `-yes-service-stopped` 或检测到活动写入 | 中止执行；停服并确认后再继续 |
| 停 API 后线上 502 | loopback 反代单元声明了 `Requires=<API 单元>`，被级联停止 | 清理后同时启动 API 与反代两个单元；不要只启动 API |
| 无分类 | 清理后分类为空（默认即如此） | 需手工创建分类；或在清理时加 `-reseed-categories` 重新播种默认分类 |
| 无账户 | 清理后账户为空 | 预期行为；先创建账户再记账 |
| 密钥丢失 | 启动后 SMTP / IMAP 解密失败 | 备份缺失时无法恢复，只能重新配置；属不可自动回滚项 |
| 基线文件丢失 | `verify` 无法执行 | 清理前必须另存副本；若已提交且基线丢失，只能以备份恢复后重新采集 |
| 备份校验失败 | `sha256sum -c` 不通过 | 立即停止，重新备份；不得使用无效备份继续 |
| 身份表被误列入清理 | dry-run 输出出现身份表 | 中止并修复配置；不得执行 `-execute` |

## 8. 证据与验收

`docs/reviews/v2-purge-evidence.md` 必须记录：

| 字段 | 内容 |
| --- | --- |
| 执行人 / 时间 | 执行人姓名或工号、开始与结束时间（含时区） |
| 备份 | 备份文件名、SHA-256、归档位置、校验结果 |
| 隔离恢复 | 隔离实例比对结果（迁移版本、身份计数、借贷合计） |
| 基线 | 基线文件名、采集时间、身份计数与借贷合计 |
| dry-run | `-json-out` 落盘的 JSON、表数与行数摘要、多家庭警告原文、确认人与确认时间 |
| execute | 完整命令参数（不含凭据）、退出码、耗时 |
| verify | 完整输出、`-json-out` 落盘的 JSON、退出码 |
| 前后计数 | 11 张身份表 + 2 张保留配置表清理前后 `COUNT(*)` 对照，`users` 摘要（`users_digest`）前后一致 |
| 家庭数 | 清理前后 `households` 计数与本次确认的删除范围（S9：全部家庭） |
| 冒烟 | `/api/v1/readyz`、登录、`/api/v1/auth/me`、成员切换结果 |
| 回滚点 | 旧库与备份位置、保留到期时间 |
| 结论 | 通过 / 失败、遗留问题与 UNVERIFIED 项 |

验收清单：

- [ ] 备份校验通过，隔离恢复比对通过，且备份库名与服务运行时 `TINYNEST_MYSQL_DATABASE` 一致。
- [ ] `baseline-capture` 完成且基线文件已归档，无「未分类表」警告。
- [ ] `purge -dry-run` 人工核对通过，身份表与保留配置表不在清单中，多家庭警告已被确认。
- [ ] `purge -execute` 退出码为 0。
- [ ] `verify` 退出码为 0，身份计数与迁移版本前后一致。
- [ ] `GET /api/v1/readyz` 返回 200（裸 `/readyz` 返回 404，不算通过）。
- [ ] 管理员登录、`/auth/me` 与 `POST /household/members/current` 通过。
- [ ] 备份与旧库保留 ≥30 天并登记。
- [ ] 证据文件完整。

## 9. UNVERIFIED 清单

- 表名与分组口径已三次核对，三个口径不要混用：**2026-11-25 生产只读**（迁移 `[1..12]`、37 张表）时 `PurgeGroups`/`DeleteOrder` 共 54 个表名，其中 22 张存在、32 个 V1 遗留名，生产 `tinynest_v2` 的两次只读 dry-run 输出与此逐值一致（见[发布证据](../reviews/v2-statement-ui-deployment-evidence.md)）；**2026-11-26 加入迁移 `013` 后**在当前工作树重测为 55 个表名 / 23 张存在 / 32 个遗留名，新增的那张是 `statement_placeholders`；**同一天加入迁移 `014` 后**再测为 **56 个表名 / 24 张存在 / 32 个遗留名**，新增的那张是 `statement_details`。**生产库要先跑内嵌该迁移的新二进制（启动时自动补建）才会出现这些表**，否则它们和 V1 遗留名一样按「表不存在」跳过，不影响其余清理。手册正文仍未在生产现场逐条执行。
- 生产 `purge -execute`、`verify`、`baseline-capture` 的实际输出与耗时 UNVERIFIED（属破坏性操作，未获授权执行）。
- 步骤 2 的备份已在 2026-11-25 修复并产出真实转储（导出运行时库 + 基线行断言 + SHA-256 sidecar）。**步骤 3 的隔离恢复逐项比对已在 2026-11-25 生产执行通过**：把 `tinynest_v2-20261125T085500Z.sql`（12,859,119 字节，`sha256sum -c` OK）导入同实例上明确命名的隔离库，导入耗时 10.661s，隔离库 38 张表、`baseline=v2`、迁移 `[1..12]`，用 `tinynest-admin baseline-capture` 与生产基线逐字段比对得 **0 项差异**（身份 51 行、保留配置 3 行、业务 95,176 行、`journal_entries` 12,391 / `journal_lines` 24,782 / 借=贷=1,311,336,623、`users_digest` 相同、迁移版本与家庭数相同），核对后已 `DROP` 隔离库。（2026-11-26 部署 `v2-9376f47` 后运行时为 39 张表、迁移 `[1..13]`；该演练输入仍是 013 之前的转储，故记录值为 38 与 `[1..12]`。）同一份模板随后作为生产运维脚本接管定时备份，并实测到「库名与运行时不一致即中止、不写出转储」这条守卫（退出码 1）。仍 UNVERIFIED 的是：转储尚无第二份**异地**副本。
- `purge` 的三道负面守卫在生产只读实测均为退出码 1：`-confirm` 与运行时库名不符（`-confirm 必须与数据库名完全一致（"tinynest_v2"），收到 "tinynest"`）、`-execute` 缺 `-yes-service-stopped`、多家庭库未加 `-household`（新文案明确 `the deletes are not household-scoped`）。三次运行前后 `transactions` 12,330 / `journal_entries` 12,391 / `journal_lines` 24,782 逐值未变。
- 密钥备份文件的实际保管方式与责任 UNVERIFIED，需用户确认。
- 旧库与备份保留 ≥30 天的执行情况 UNVERIFIED。
- 生产库现有 3 个家庭（2026-11-25 只读核对），与本工具「单家庭」设计前提不符：`-household` 只确认不缩范围，见 S9。是否只清其中一个家庭需用户决定，UNVERIFIED。
