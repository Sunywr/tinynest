# TinyNest V2 生产清理手册（Purge Runbook）

> 状态：P5 执行口径（P4 交付）｜编写日期：2026-11-22｜执行人：生产维护者（用户）
> 重要声明：本手册中的命令未在文档编写环境执行；任何「验证通过」结论必须由生产执行时的输出与证据文件支撑，未执行前一律 UNVERIFIED。
> 关联文档：[总计划](v2-bookkeeping-reset.md)、[记账模块规格](v2-bookkeeping-module-spec.md)、[新库切换手册](v2-cutover-runbook.md)
> 适用范围：v1 生产库（MySQL 或 SQLite）的业务数据清理；目标是在不删除身份数据的前提下清空业务数据，并提供可验证、可回滚的执行路径。

## 1. 目标、边界与清理语义

- 目标：生产库业务数据清空，仅保留 11 张身份表数据；清理前后身份计数与迁移版本一致，`verify` 全绿。
- 边界：生产清理由用户执行（决策①）；我们交付工具与手册，不接触生产凭据、不代执行、不保存生产数据。
- 清理语义：只删除数据行（DML），不执行 DDL。原因：MySQL 的 `TRUNCATE` / `DROP` 会隐式提交，无法满足「单事务执行、失败整体回滚」；v1 库清空后保留空表结构，V2 的 27 张表目标由新库基线承担（见切换手册）。
- 不变量：11 张身份表数据完整；`journal_lines` 借贷合计在清理前采集、清理后归零并可核对；迁移版本不变；备份与旧库保留 ≥30 天。

## 2. 安全不变量与前置条件

| 编号 | 不变量 | 说明 |
| --- | --- | --- |
| S1 | 凭据只走文件 | 只使用 `TINYNEST_MYSQL_PASSWORD_FILE` / `CREDENTIALS_DIRECTORY`；不接受命令行密码；日志不得打印连接串与凭据 |
| S2 | 服务必须停止 | `purge --execute` 必须带 `--yes-service-stopped`；执行前确认应用进程已停止、健康检查不可达 |
| S3 | 库名必须二次确认 | `--confirm=<DBNAME>` 必须与当前连接的数据库名完全一致，否则中止 |
| S4 | 单事务 | 全部删除在同一事务内完成；任一步失败整体回滚，不得留下半清理状态 |
| S5 | 身份表白名单 | 11 张身份表永不进入删除清单；`verify` 与人工核对双重确认 |
| S6 | 先备份后清理 | 备份、SHA-256 校验、隔离恢复比对全部通过后才允许 `--execute` |
| S7 | 证据留痕 | 执行人、时间、备份 SHA-256、前后计数写入 `docs/reviews/v2-purge-evidence.md` |
| S8 | 保留期 | 旧库与备份保留 ≥30 天；期间不得删除任何回滚材料 |

前置条件：

- 已确认执行窗口与值守人；生产访问权限由用户持有。
- 已确认数据库名、字符集与驱动（SQLite / MySQL）。
- 已备份 32 字节 `mail-key` / `credit-mail-key` 文件并登记保管人（丢失后 `mail_configuration.password_cipher` 与 `mail_queue.payload` 不可解）。
- 已在证据文件中预留记录位置，并确认基线文件、备份文件、dry-run 输出的归档目录。

## 3. 运维 CLI 接口（最终实现口径）

```text
tinynest-admin [--driver sqlite|mysql] <command>

baseline-capture --out=<path>        清理前采集身份表计数、迁移版本、借贷合计
purge --dry-run                      只打印将删除的表与行数
purge --execute --confirm=<DBNAME> --yes-service-stopped [--household=<id>] [-reseed-categories]
verify --baseline=<path>             只读校验
identity-export --out=<path>         导出 11 张身份表为 JSONL
identity-import --in=<path>          导入 JSONL 到已建基线的库
```

参数说明：

| 命令 / 参数 | 说明 |
| --- | --- |
| `--driver sqlite` / `--driver mysql` | 按部署驱动选择；默认值由 CLI 决定，执行前显式写出 |
| `baseline-capture --out=<path>` | 采集身份表计数、迁移版本、`journal_lines` 借贷合计，生成基线文件；不修改数据库 |
| `purge --dry-run` | 只读；打印将清理的表与行数，供人工核对；不得写库 |
| `purge --execute` | 执行清理；必须同时提供 `--confirm=<DBNAME>` 与 `--yes-service-stopped` |
| `--household=<id>` | 多家庭库必须显式指定清理范围；未指定时如检测到多个家庭应中止并提示 |
| `-reseed-categories` | 清理后重新播种默认分类。**默认关闭**：关闭时「所有业务表为空」的校验最严格；只有需要清理后继续使用 v1 界面时才开启，且开启后须用 `verify -allow-seeded-categories` 校验 |
| `verify --baseline=<path>` | 只读校验；对比基线，确认身份计数未变、业务表已清空、迁移版本一致 |
| `identity-export --out=<path>` | 导出 11 张身份表为 JSONL；含用户名与邮箱，按最小权限保管 |
| `identity-import --in=<path>` | 导入 JSONL；目标库必须已建立 V2 基线（`schema_meta.baseline=v2`） |

示例（占位符需按生产实际替换，执行前逐条核对）：

```bash
tinynest-admin --driver mysql baseline-capture --out=<备份目录>/baseline.json
tinynest-admin --driver mysql purge --dry-run
tinynest-admin --driver mysql purge --execute --confirm=<数据库名> --yes-service-stopped
tinynest-admin --driver mysql verify --baseline=<备份目录>/baseline.json
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

### 4.2 清理分组（42 张表）

| 分组 | 张数 | 表名 | 说明 |
| --- | --- | --- | --- |
| 核心结构 | 4 | `categories`、`projects`、`accounts`、`ledger_accounts` | `categories` 默认清理为空（加 `-reseed-categories` 可重新播种）；`accounts` 清空后需先建账户才能记账（预期） |
| 账本 | 3 | `journal_entries`、`journal_lines`、`account_opening_balances` | 分录清空；借贷合计在清理前已采集 |
| 记账数据 | 4 | `transactions`、`transaction_splits`、`refunds`、`budgets` | 流水、拆分、退款与月度预算清空；`budgets` 只在 V2 库存在，v1 库自动跳过 |
| 信用 | 7 | `credit_instruments`、`credit_statements`、`credit_statement_items`、`credit_payments`、`credit_instrument_changes`、`credit_statement_adjustments`、`credit_statement_provisions` | 信用模块整体退役 |
| 分期 | 4 | `installment_contracts`、`installment_schedule_items`、`installment_payments`、`installment_adjustments` | 分期模块整体退役 |
| 负债 | 3 | `debt_contracts`、`debt_schedule_items`、`debt_payments` | 负债模块整体退役 |
| 调度 | 1 | `schedule_due_items` | 到期调度退役 |
| 投资 | 6 | `investment_assets`、`investment_fee_rules`、`investment_trades`、`investment_dividends`、`investment_valuations`、`investment_positions` | 投资模块整体退役 |
| 信用账单邮件 | 3 | `credit_mailboxes`、`credit_mail_bindings`、`credit_mail_previews` | 信用邮件账单退役 |
| 账单导入 | 2 | `statement_import_rows`、`statement_import_bindings` | 账单导入退役 |
| 股票 | 1 | `stock_watch` | 股票关注退役 |
| 待发邮件 | 1 | `mail_queue` | 待发邮件清空；站点邮件配置保留 |
| 运营审计 | 3 | `audit_logs`、`schedule_runs`、`stock_quotes` | 清理 v1 审计数据；V2 保留 `audit_logs` 结构承载新审计 |

### 4.3 附加清理项：成员档案与邀请（3 张，需确认）

| 表 | 说明 |
| --- | --- |
| `household_members` | 成员档案；V2 继续提供家庭成员功能时按「清空后重建」处理，不删除结构 |
| `household_user_members` | 用户与成员关联；同上 |
| `household_invites` | 历史邀请记录；邀请机制保留，历史数据可清空 |

口径说明：§4.2 的 13 个分组表名合计 42 张，加上本节 3 张，`tinynest-admin purge` 的白名单共 **45 张表**。执行时以 `purge --dry-run` 实测清单为准：当前 schema 中不存在的表会被明确列为「已跳过」，不会静默漏清。若保留家庭成员功能，这 3 张表按清空重建处理，需要用户在 dry-run 核对时明确确认；未确认前 `-execute` 不得执行。

### 4.4 清理后行为说明

- `categories` 默认清理为空（严格口径）。若需清理后继续使用 v1 界面，加 `-reseed-categories` 重新播种默认分类，并用 `verify -allow-seeded-categories` 校验；不加则需手工创建分类。
- `accounts` 清空后必须先建账户才能记账，属预期行为，不是故障。
- `mail_queue` 清空后待发邮件丢失，属预期。
- 加密配置依赖 32 字节 `mail-key` / `credit-mail-key`：必须单独备份这两个文件，否则 `mail_configuration.password_cipher` 与 `mail_queue.payload` 不可解。
- 清理只删数据，不执行 DDL；v1 空表结构保留，便于回滚与切换前导出。

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

```bash
# 口令由受限凭据文件提供，不出现在命令行
mysqldump \
  --defaults-extra-file=<凭据文件> \
  --single-transaction \
  --no-tablespaces \
  --set-gtid-purged=OFF \
  --routines \
  --events \
  --triggers \
  --order-by-primary \
  <数据库名> > <备份目录>/<数据库名>-<时间戳>.sql
sha256sum <备份目录>/<数据库名>-<时间戳>.sql | tee <备份目录>/<数据库名>-<时间戳>.sql.sha256
sha256sum -c <备份目录>/<数据库名>-<时间戳>.sql.sha256
```

要求：备份文件与 SHA-256 文件分开归档；SHA-256 记入证据文件；校验失败不得继续。

### 步骤 3：恢复到隔离实例并比对

```bash
mysql --defaults-extra-file=<隔离实例凭据文件> <隔离库名> < <备份文件>
```

| 检查项 | 命令或方式 | 期望 |
| --- | --- | --- |
| 迁移版本 | `SELECT COUNT(*) AS versions, MAX(version) AS latest FROM schema_migrations;` | 与基线记录一致 |
| 身份计数 | 对 §4.1 的 11 张表逐一 `COUNT(*)` | 与基线一致 |
| 借贷合计 | `SELECT SUM(debit_minor) AS debit_total, SUM(credit_minor) AS credit_total FROM journal_lines;` | `debit_total = credit_total` |

比对通过后才进入下一步；任一项不一致必须停止并排查备份或实例问题。

### 步骤 4：采集基线

```bash
tinynest-admin --driver mysql baseline-capture --out=<备份目录>/baseline.json
```

基线文件记录：身份表计数、迁移版本、`journal_lines` 借贷合计、采集时间。基线文件必须另存副本，`verify` 失败排查时不得覆盖原基线。

### 步骤 5：dry-run 人工核对

```bash
tinynest-admin --driver mysql purge --dry-run
```

人工核对清单：

- [ ] 输出中的表清单与 §4.2、§4.3 一致，无遗漏、无多余表。
- [ ] 输出包含每张表的预计删除行数。
- [ ] 11 张身份表均未出现在删除清单中。
- [ ] 多家庭库时使用 `--household=<id>` 且核对范围正确。
- [ ] 输出为只读行为，核对期间数据库无写入（服务已停）。

### 步骤 6：执行清理

```bash
tinynest-admin --driver mysql purge --execute --confirm=<数据库名> --yes-service-stopped
```

要求：

- 命令在单事务内执行；出现任何错误即整体回滚，记录错误输出后排查。
- 执行期间禁止重试并发清理；一次执行结束后再判断状态。
- 执行完成后立即执行 `verify`，不要先启动服务。

### 步骤 7：校验

```bash
tinynest-admin --driver mysql verify --baseline=<备份目录>/baseline.json
```

`verify` 必须确认：

- 11 张身份表计数与迁移版本与基线一致。
- §4.2、§4.3 的清理表业务数据为空（身份表与保留结构除外）。
- 退出码为 0；任一不一致即视为失败，不得进入下一步。

### 步骤 8：启动 API 并检查就绪

- 启动 API 服务。
- `GET /readyz` 返回 200；`GET /healthz` 返回 200。
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
| `purge --execute` 执行失败 | 单事务已整体回滚；核对 `verify` 输出与数据库状态，修复后重新执行 |
| 清理已提交但需要回滚 | 停止服务，从备份恢复数据库，校验身份计数与迁移版本，再启动 v1 服务 |
| 清理后发现数据异常 | 立即停止服务；以备份为准恢复；保留现场日志与 dry-run/verify 输出 |
| 密钥文件丢失 | 无法从数据库恢复密文；只能重新配置 SMTP / IMAP（标注为不可自动恢复） |
| 回滚完成后 | 重新执行 `baseline-capture` 与 `verify`，确认恢复到清理前状态，并记录回滚原因与时间 |

回滚原则：已提交后的回滚等于从备份恢复，不做逆向补写；旧库与备份至少保留 30 天。

## 7. 边缘情况

| 场景 | 判定信号 | 处置 |
| --- | --- | --- |
| 清理中断 | CLI 非零退出或连接中断 | 单事务自动回滚；确认 `verify` 与计数未变后重跑；不得在未知状态下重复执行 |
| `--confirm` 不符 | CLI 报错并中止 | 核对当前连接数据库名与实际库名；修正后重新 dry-run，不得绕过确认 |
| 多家庭库未指定 `--household` | CLI 检测到多个家庭并中止 | 先确认清理范围，dry-run 加 `--household=<id>`，核对输出后执行 |
| 服务未停 | 缺少 `--yes-service-stopped` 或检测到活动写入 | 中止执行；停服并确认后再继续 |
| 停 API 后线上 502 | loopback 反代单元声明了 `Requires=<API 单元>`，被级联停止 | 清理后同时启动 API 与反代两个单元；不要只启动 API |
| 无分类 | 清理后分类为空（默认即如此） | 需手工创建分类；或在清理时加 `-reseed-categories` 重新播种默认分类 |
| 无账户 | 清理后账户为空 | 预期行为；先创建账户再记账 |
| 密钥丢失 | 启动后 SMTP / IMAP 解密失败 | 备份缺失时无法恢复，只能重新配置；属不可自动回滚项 |
| 基线文件丢失 | `verify` 无法执行 | 清理前必须另存副本；若已提交且基线丢失，只能以备份恢复后重新采集 |
| 备份校验失败 | `sha256sum -c` 不通过 | 立即停止，重新备份；不得使用无效备份继续 |
| 身份表被误列入清理 | dry-run 输出出现身份表 | 中止并修复配置；不得执行 `--execute` |

## 8. 证据与验收

`docs/reviews/v2-purge-evidence.md` 必须记录：

| 字段 | 内容 |
| --- | --- |
| 执行人 / 时间 | 执行人姓名或工号、开始与结束时间（含时区） |
| 备份 | 备份文件名、SHA-256、归档位置、校验结果 |
| 隔离恢复 | 隔离实例比对结果（迁移版本、身份计数、借贷合计） |
| 基线 | 基线文件名、采集时间、身份计数与借贷合计 |
| dry-run | 输出摘要、确认人、确认时间 |
| execute | 完整命令参数（不含凭据）、退出码、耗时 |
| verify | 完整输出、退出码 |
| 前后计数 | 11 张身份表清理前后 `COUNT(*)` 对照 |
| 冒烟 | `/readyz`、登录、`/auth/me`、成员切换结果 |
| 回滚点 | 旧库与备份位置、保留到期时间 |
| 结论 | 通过 / 失败、遗留问题与 UNVERIFIED 项 |

验收清单：

- [ ] 备份校验通过，隔离恢复比对通过。
- [ ] `baseline-capture` 完成且基线文件已归档。
- [ ] `purge --dry-run` 人工核对通过，身份表不在清单中。
- [ ] `purge --execute` 退出码为 0。
- [ ] `verify` 退出码为 0，身份计数与迁移版本前后一致。
- [ ] `GET /readyz` 返回 200。
- [ ] 管理员登录、`/auth/me` 与 `POST /household/members/current` 通过。
- [ ] 备份与旧库保留 ≥30 天并登记。
- [ ] 证据文件完整。

## 9. UNVERIFIED 清单

- 全部命令未在本次文档编写环境执行，UNVERIFIED。
- 备份、隔离恢复、dry-run、执行、verify 的实际输出与耗时 UNVERIFIED。
- 生产数据库实际表名、驱动与版本以生产部署的实际环境为准，本手册按任务口径列举，未做生产核对 UNVERIFIED。
- 密钥备份文件的实际保管方式与责任 UNVERIFIED，需用户确认。
- 旧库与备份保留 ≥30 天的执行情况 UNVERIFIED。
