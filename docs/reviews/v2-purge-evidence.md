# V2 生产数据清理证据（2026-11-22 执行）

本文件记录一次**已实际执行**的生产清理。所有数字来自 `tinynest-admin` 的输出与执行后的独立 SQL 复核。
本文不含服务器地址、域名、口令或令牌；备份与基线文件留在受控位置。

## 1. 结论

| 项目 | 结果 |
| --- | --- |
| 执行时间 | 2026-11-22 15:21 CST（07:21 UTC） |
| 执行对象 | 生产 MySQL 8.0.46，数据库 `tinynest`，迁移版本 1–13（V1 schema） |
| 清理方式 | `tinynest-admin purge -execute`，单事务，未停外键检查 |
| 删除业务行 | **3,975 行**（44 张清理表；`budgets`/`stock_watch`/`stock_quotes` 在 V1 不存在，自动跳过并明确列出） |
| 清理后业务行 | **0**（工具断言 + 独立 SQL 逐表复核） |
| 分录 | 清理前 568 条分录 / 1,136 行、借贷各 34,036,109；清理后 0 条 / 0 行 |
| 身份与权限 | **完整保留**：households 1、users 2、sessions 19、site_administrators 1、email_identities 1、mail_configuration 1、mail_preferences 0、mail_daily_usage 2、mail_recipient_daily_usage 1，合计 29 行 |
| users 摘要 | 清理前后 SHA-256 一致（`c2f570c1…`），账号、口令哈希、角色未被触碰 |
| 引用完整性 | 孤儿分录行、孤儿用户、孤儿会话、孤儿站点管理员 4 项检查全部为 0 |
| 服务状态 | API 与 loopback 反代均已启动；`readyz` = `mysql_ping_and_migrations` ready；公开首页 200；`/auth/me` 401、错误口令登录 401（证明账号表可读且未损坏） |

## 2. 备份与恢复验证

| 项目 | 值 |
| --- | --- |
| 备份文件 | `/var/backups/tinynest/mysql/pre-v2-purge-<UTC 时间戳>.sql`（约 1.9 MB） |
| SHA-256 | `175d23741f83f97ad231bb968b9ae4dc20ce5087e1bd6737ba59e4173faf7e90` |
| 转储内容 | 54 个 `CREATE TABLE`，以 `-- Dump completed` 结束 |
| **恢复演练** | 在一台与生产隔离的主机上用一次性 `mysqld`（独立 datadir、`--skip-networking`）恢复该转储 |
| 演练结果 | 恢复后 54 张表；**逐表行数的规范化校验和与生产完全一致**（`360106b78060031367906b758dee8f72`）；分录借贷合计同为 34,036,109；users 两行内容一致 |
| 演练后处理 | 一次性实例已关闭并删除，转储副本从临时目录删除 |

结论：备份可恢复，且恢复内容与清理前生产数据逐表一致。

## 3. 执行顺序（实际）

1. 读取服务单元配置，确认数据库名、账号、socket 与凭据文件位置。
2. 生成全量转储 + SHA-256，并在隔离主机完成恢复验证。
3. 停止 API 单元（观察：loopback 反代单元因 `Requires` 被级联停止）。
4. `baseline-capture`：记录迁移版本、身份计数、借贷合计与 users 摘要。
5. `purge -dry-run`：核对将删除的表与行数（合计 3,975）。
6. `purge -execute -confirm=<数据库名> -yes-service-stopped`：提交。
7. `verify -baseline`：业务表全空、身份计数与摘要一致。
8. 独立 SQL 复核：逐表计数合计 0；身份表计数不变；4 项引用完整性检查为 0。
9. 启动 API 与反代两个单元；验证 `healthz` / `readyz` / 未认证 401 / 错误口令 401 / 公开首页 200。

证据文件（受控位置，未入库）：`baseline.json`、`purge-result.json`、`verify.json`、转储及其 `.sha256`。

## 4. 清理后的状态与影响

- **分类为空**：按严格口径未重新播种默认分类。维护期若仍需用 V1 界面记账，需先创建分类，或重新执行一次带 `-reseed-categories` 的清理并用 `verify -allow-seeded-categories` 校验。
- **账户为空**：需先建账户才能记账，属预期。
- **家庭成员档案、邀请、审计日志已清空**；用户下次登录时由 `POST /api/v1/household/members/current` 自动重建成员档案与绑定。
- **会话保留**：19 条会话未失效，已登录设备无需重新登录。
- **运维二进制**：`tinynest-admin` 已安装在 B 机 `/usr/local/bin/`（0755，root 所有），供后续 V2 切换使用；它不是 HTTP 服务，也不被 API 进程引用。
- 执行期间创建的临时凭据配置文件与转储副本已删除。

## 5. 未验证（UNVERIFIED）

- 未用真实账号口令完成登录与记账冒烟（无口令，不应索取）；仅以 401/200 响应与身份计数证明账号数据完好。
- V2 新库切换（`identity-export` / `identity-import`）尚未执行。
- iPhone 实机、A↔B 隧道重建后的长时间稳定性、生产性能样本未验证。
