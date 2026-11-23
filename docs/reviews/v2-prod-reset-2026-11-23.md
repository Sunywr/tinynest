# 生产数据重置证据（2026-11-23）

本文件记录一次**已实际执行**的生产数据重置。执行对象为 TinyNest 生产 MySQL 库
`tinynest_v2`（迁移版本 1–6，含 `006_auth_login_locks`）。本文不包含服务器地址、
域名、口令或令牌；备份与脚本留在受控位置。

## 1. 结论

| 项目 | 结果 |
| --- | --- |
| 执行时间 | 2026-11-23 20:15–20:17 CST |
| 用户要求 | 删除所有记账数据、所有资产、所有家庭；只保留用户 |
| 确认口径 | 删除所有真实家庭及其业务数据；保留系统保留行 `onboarding`（「账号开通」）承载用户外键，3 个用户迁入 |
| 删除业务行 | **807 行**（16 张业务表，清理前逐表计数见 §3） |
| 清理后业务行 | **0**（事务内断言 + 执行后独立 SQL 逐表复核） |
| 家庭 | 2 → 1（仅保留系统行 `onboarding`，不是真实家庭） |
| 用户 | 3 → 3；`id/username/password_hash/role/status` 摘要前后一致 |
| 外键孤儿 | 用户、会话均为 0 |
| 服务状态 | API 与 loopback 反代均 active；`readyz` 200，`healthz` 200，未认证 `/auth/me` 401 |
| 会话冒烟 | 为 `admin` 临时建会话后 `GET /auth/me` 返回 200 且 `household_id=""`；临时会话已删除（剩余 0） |

## 2. 备份

| 项目 | 值 |
| --- | --- |
| 文件 | `/home/ubuntu/tinynest-reset-20261123/tinynest_v2-pre-reset-20261123T1215Z.sql` |
| 大小 | 191,457 字节 |
| SHA-256 | `8ccf53c3404cd5cd8ea84f0a4ff9deddec511f206b846f4578afcee0fbb4b09d` |
| 内容 | 29 个 `CREATE TABLE`，以 `-- Dump completed` 结束；`sha256sum -c` 通过 |
| 隔离演练 | 已恢复到临时库并逐表比对；重置脚本在临时库执行通过；临时库已删除 |
| 保留 | 备份与演练脚本保留 ≥30 天，期间不删除 |

## 3. 清理前计数

| 表 | 行数 |
| --- | ---: |
| `transactions` | 140 |
| `journal_entries` | 140 |
| `journal_lines` | 280 |
| `accounts` | 13 |
| `categories` | 21 |
| `ledger_accounts` | 13 |
| `investment_accounts` | 4 |
| `investment_holdings` | 30 |
| `household_members` | 2 |
| `household_user_members` | 1 |
| `household_invites` | 0 |
| `budgets` | 0 |
| `account_opening_balances` | 0 |
| `audit_logs` | 160 |
| `mail_queue` | 3 |
| `auth_login_locks` | 0 |

身份侧：`users` 3、`households` 2、`sessions` 25、`email_identities` 2、
`site_administrators` 1、`mail_configuration` 1。

## 4. 执行方式

1. 停止 `tinynest-api.service`；loopback 反代因 `Requires` 级联停止，8787/18444 端口关闭。
2. 用 `mysqldump --single-transaction` 生成全量备份并校验 SHA-256。
3. 把备份恢复到临时库，执行同一份重置脚本；全部计数符合预期后删除临时库。
4. 故障注入：把脚本中的用户数断言故意改错，确认事务回滚（流水 140、用户 3、家庭 2 原样保留）。
5. 在生产库执行同一脚本，单事务内按「子表 → 父表」删除：
   - `journal_lines` → `account_opening_balances` → `transactions` → `journal_entries`（先解除 `reversal_of` 自引用）
   - `budgets` → `investment_holdings` → `investment_accounts` → `accounts` → `categories` → `ledger_accounts`
   - `household_user_members` → `household_members` → `household_invites`
   - `audit_logs` / `mail_queue` / `auth_login_locks`
   - 补齐 `onboarding` 行 → 把用户迁入 → 删除其余家庭
6. 事务内断言：16 张业务表为 0、用户数 3、家庭数 1、用户全部属于 `onboarding`、无外键孤儿；任一不满足即报错退出，连接断开后整体回滚。断言通过后提交。
7. 独立 SQL 复核 + 重启 API 与反代 + 就绪检查。

## 5. 清理后状态

- 业务表全部为 0；`ledger_accounts` 与 `categories` 也清空（符合“所有记账数据/资产”口径）。
- `households` 仅剩 `onboarding`（名称「账号开通」）。该行是 `users.household_id` 外键的保留行，
  新版 onboarding 流程把它视为「尚未归属家庭」，`/auth/me` 返回空家庭。
- 3 个用户全部迁入 `onboarding`，登录凭据、角色、状态未改动：
  - `admin`（role=admin）
  - `liuj`（role=member）
  - `sunywr`（role=member）
- 保留的身份/邮件数据：`sessions` 25（未强制退出登录）、`email_identities` 2、
  `email_challenges` 4、`site_administrators` 1、`mail_configuration` 1、邮件用量表。
- 服务：`readyz` 200、`healthz` 200、未认证 `/auth/me` 401、`applied_migrations=[]`。

## 6. 影响与待办

- **`admin` 没有已验证邮箱**，而新版 `canOnboard` 要求邮箱已验证后才能创建/加入家庭。
  因此 `admin` 登录后会停在 onboarding，需要先绑定并验证邮箱才能创建新家庭。
  `liuj`、`sunywr` 已有已验证邮箱，可直接创建或凭邀请加入家庭。
- 用户重新创建家庭时会重新播种默认分类；当前分类为空属预期。
- 现有会话仍有效，用户无需重新登录；如需强制重新登录，可另行清理 `sessions`。

## 7. UNVERIFIED

- 未用真实口令完成登录与记账冒烟（无口令，不应索取）；以临时会话验证 `/auth/me`
  可读到 `admin`，并以 `readyz`、`healthz`、身份计数与用户摘要证明账号数据完好。
- 未做浏览器端 onboarding 实机走查（需要真实账号口令与邮箱验证码）。
- 备份仅在本机临时库恢复验证，未在独立主机做二次恢复演练。
