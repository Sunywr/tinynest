# TinyNest V2 · API 契约

> 依据：`apps/api/internal/api/setup.go` 的 `Register` 路由注册、各 handler 实现与 `internal/api/*_test.go`。
> 契约范围：**72 条业务路由**（按「方法 + 路径」计，在 `91b47c9` 上以 `TestRouteBudget` 实测）；另有 `/api/v1/healthz`、`/api/v1/readyz` 两条健康检查由 `internal/server` 注册，不计入 72。
> 路由总数由 `TestRouteBudget` 冻结为 ≤72；包括注册及登录使用的图形验证码入口。
> 信用卡还款计划与投资估值不是新路由，而是 `GET /accounts?include=credit_debt,investments` 的可选投影；`include` 接受空值、`credit_debt`、`investments` 或逗号组合。
> 账单邮箱、绑卡、预览、同步与只读对账共 10 条路由见 §11，全部只读账本、不写分录。

## 1. 通用约定

| 项 | 约定 |
| --- | --- |
| 前缀 | 全部业务接口位于 `/api/v1`；前端只请求同源相对路径 `/api/v1/*` |
| 传输 | JSON；响应 `Content-Type: application/json; charset=utf-8`，所有响应 `Cache-Control: no-store`、`X-Content-Type-Options: nosniff` |
| 金额 | 整数分（`*_minor`）；禁止 JSON 浮点；V2 不用 `currency` 做多币种，默认 `CNY` |
| 日期 | `occurred_on` 为 `YYYY-MM-DD`；业务日期按 Asia/Shanghai；时间戳字段为 RFC3339 UTC |
| 认证 | HttpOnly Cookie `tn_session`（SameSite=Lax，`Secure` 由 `TINYNEST_COOKIE_SECURE` 显式控制） |
| CSRF | 会话内的非 GET/HEAD 请求必须带 `X-CSRF-Token`，值来自 `POST /auth/session` 或 `GET /auth/me` 的 `csrf_token`。例外：公开端点（`/setup`、登录、注册、加入、`/auth/mail/*`）与 `DELETE /auth/session` 不要求 CSRF |
| 角色 | `admin`（管理员）/ `member`（成员）/ `viewer`（只读）。`viewer` 的写请求除登出、改自己资料、改自己密码外一律 403 |
| 错误结构 | `{"error":{"code":"...","message":"中文说明"}}`；不泄露 SQL、路径、堆栈或密钥 |
| 分页 | 仅 `GET /transactions` 使用 cursor 分页（`limit` 1–200，默认 50，返回 `next_cursor`） |
| 幂等 | 仅 `POST /transactions` 与 `POST /accounts/transfers` 读取 `Idempotency-Key`（详见 §13）；账单同步用唯一约束与配置版本去重 |
| 家庭隔离 | 所有财务查询在 SQL 层按 `household_id` 限定；跨家庭 ID 等同于不存在（404 / 业务 400），不返回他人数据 |
| 审计 | 记账、预算、账户、成员、邀请、审批、用户与邮件配置等写操作在业务事务内写 `audit_logs`；分类的新建/改名/排序不写审计，分类删除写审计；只读查询不写审计 |

### 1.1 错误码

| HTTP | code | 含义 |
| --- | --- | --- |
| 400 | `invalid_request` / `invalid_amount` / `unsupported_statement_card` | 请求格式或业务校验失败（含金额非正整数分、账单卡名缺少四位卡号尾号或银行不在支持范围） |
| 401 | `unauthorized` | 未登录、会话过期或口令错误 |
| 403 | `forbidden` / `setup_disabled` / `verification_required` | 角色不足、初始化入口关闭、需邮箱验证 |
| 404 | `not_found` | 家庭内不存在（含不属于本家庭的 ID） |
| 409 | `conflict` / `member_in_use` / `verification_failed` / `card_already_bound` | 已初始化、已被引用、已冲正、邮箱配置版本过期、重复验证失败、信用卡已绑定其他账单邮箱 |
| 429 | `rate_limited` / `mail_quota` / `email_daily_limit` | 登录/注册/邮件限流；`Retry-After` 仅在登录限流时返回 |
| 500 | `internal_error` | 未预期错误（通用中文文案） |
| 502 | `mail_test_failed` / `mailbox_test_failed` / `statement_preview_failed` / `statement_sync_failed` | SMTP 测试发送失败（旧配置保持生效）；账单邮箱连接或 IMAP 读取失败 |
| 503 | `mail_unavailable` | 邮件未启用、加密密钥缺失或账单邮箱授权码不可用 |

业务路由的实际状态码在 §2–§12 逐条标注；成功响应的常见状态码为 200 / 201 / 202 / 204。

## 2. 初始化与认证（9 条）

| 方法 | 路径 | 鉴权 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- | --- |
| POST | `/setup` | 一次性 `X-Setup-Token`（公开端点） | `household_name`、`username`（≥3）、`password`（≥8） | 201 `{household_id,user_id,status:"initialized"}`；同时写入默认分类 | 401 令牌无效、403 `setup_disabled`（未配置令牌或 `TINYNEST_SETUP_DISABLED=true`）、409 已初始化、400 |
| GET | `/auth/captcha` | 公开 | 无 | 200 `{id,image}`，PNG data URL | 429 |
| POST | `/auth/session` | 公开（验证码与密码） | `username`、`password`、`captcha{id,answer}` | 200 `{username,csrf_token,status:"ok"}` + `Set-Cookie: tn_session`（7 天） | 400 `captcha_invalid`、401、429 `rate_limited` / `account_locked` / `account_locked_today` |
| DELETE | `/auth/session` | 公开（读取 Cookie；无会话也返回成功） | 无 | 200 `{status:"logged_out"}`，存在会话时被吊销 | — |
| GET | `/auth/me` | 会话 | 无 | 200 `{member_id,member_name,user_id,household_id,username,csrf_token,role,email_verified,email?,avatar?,birthday?,payday?}` | 401 |
| PATCH | `/auth/me/profile` | 会话 | `name?`（1–50 字）、`avatar?`（≤8 字符或 64×64 JPEG data URL，Base64 ≤32 KiB）、`birthday?`（`YYYY-MM-DD`，1900-01-01 至业务今天）、`payday?`（1–31）、`email?`（只能与原值相同，改邮箱须走验证） | 200 `{status:"ok",updated:[字段名]}` | 400、401、403 CSRF |
| POST | `/auth/me/password` | 会话 | `current_password`、`new_password`（8–256）、`proof{challenge_id,code}` | 200 `{status:"ok"}`；撤销全部会话并排入安全通知邮件 | 400 `invalid_request` / `verification_failed`、409 |
| POST | `/auth/register` | 公开（注册限流） | `username`（3–40 位小写字母/数字/`._-`）、`password`（至少 8 字符）、`captcha{id,answer}` | 201 `{username,csrf_token,status:"ok"}` 并建立会话。带 `email` 和 `proof` 的旧版注册仍返回 202。 | 400、409 用户名已使用、429 |
| POST | `/auth/join` | 会话与 CSRF（已验证邮箱）；旧版待审批账号可用凭据 | `code`（邀请码）；旧版附 `username`、`password` | 202 `{status:"pending"}` | 400 邀请码无效/已用、401、403、409、429 |

说明：

- 图形验证码为 6 位数字，2 分钟内一次有效，绑定请求来源。答错或过期时获取新验证码。
- 同一账号连续 5 次密码错误触发一次锁定。上海时间同日第 1–4 次分别锁 5、10、15、20 分钟，第 5 次锁至次日零点。成功登录清零当前连续错误，保留当日锁定轮数。
- `POST /setup` 只在空库可用；令牌比较在固定长度 SHA-256 摘要上做常量时间比较。初始化后令牌不能再次创建管理员。
- 登录成功会清除该用户名的失败计数，但不清 IP 维度计数；失败限流返回 429 与 `Retry-After`。
- 错误凭据统一返回 401「用户名或密码错误」，不区分账号是否存在。

## 3. 邮箱与站点邮件（8 条）

| 方法 | 路径 | 鉴权 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- | --- |
| POST | `/auth/mail/challenge` | 公开 | `email`、`purpose`（`register` / `reset`） | 202 `{status:"accepted",challenge_id}`；`reset` 对未知邮箱同样返回 accepted | 400、429 `rate_limited` / `email_daily_limit` / `mail_quota`、503 `mail_unavailable` |
| POST | `/auth/mail/reset` | 公开 | `email`、`new_password`（12–256）、`proof` | 200 `{status:"ok"}`；修改口令、撤销全部会话、排入安全通知 | 400 `invalid_request` / `verification_failed` |
| GET | `/auth/me/mail` | 会话 | 无 | 200 `{email,verified,announcements,site_admin,mail_available}` | 401 |
| POST | `/auth/me/mail/preferences` | 会话 | `announcements`（布尔，邮件订阅偏好） | 200 `{status:"ok"}` | 400、401 |
| POST | `/auth/me/mail/challenge` | 会话 | `email`、`purpose`（`bind` / `old_email` / `password`）、`current_password` | 202 `{status:"accepted",challenge_id}` | 400 `invalid_request` / `email_unverified`、429、503 |
| POST | `/auth/me/mail/confirm` | 会话 | `email`、`current_password`、`proof`、`old_proof`（更换已验证邮箱时必填） | 200 `{status:"verified"}`；绑定/换绑邮箱并向旧邮箱发送安全通知 | 400 / 409 `verification_failed` |
| GET | `/site/mail` | 站点管理员 | 无 | 200 `{last_test:{result,at},config:{enabled,host,port,username,from,name,daily_limit,announcement_limit},password_configured,version,used_today,announcements_today,key_ready}` | 401、403 |
| POST | `/site/mail` | 站点管理员 | `config{...}`、`password`（空字符串=保留原密码）、`current_password`、`test_recipient`、`version` | 200 `{status:"smtp_accepted",version}`；测试通过后才保存 | 400、403、409 `conflict`（版本过期）、429、502 `mail_test_failed`、503 `mail_unavailable` |

说明：

- 验证码 10 分钟有效、60 秒可重发、最多尝试 5 次；同一收件邮箱每天最多 3 封投递尝试（按 Asia/Shanghai 零点重置）。
- 站点管理员由 `site_administrators` 表决定，普通家庭管理员无权配置 SMTP。
- 家庭公告发送能力在 V2 退役：不存在 `GET/POST /household/mail/announcements` 路由；保留的 `announcements` 字段只表示邮件订阅偏好。

## 4. 家庭与成员（9 条）

新账号登录后，`/auth/me` 返回空的 `household_id`。通过邮箱验证可启用密码找回，并可创建家庭或申请加入。审批通过后，会话下次请求即可获得家庭权限。

| 方法 | 路径 | 鉴权 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- | --- |
| POST | `/household/invitations` | 会话（`viewer` 写请求 403；`bind_existing` 需 admin） | 普通邀请：`member_id` 与 `member_name` 二选一、`username` 可空（指定登录名）；已有账号绑定：`bind_existing:true` + `member_id` + `username` | 201 `{code}`（明文邀请码只返回一次，落库保存哈希）；绑定成功 200 `{status:"bound",member_id,username}` | 400、403、404 档案已绑定/不可用、409 已有账号或成员已绑定 |
| GET | `/household/current` | 会话 | 无 | 200 `{id,name,currency,timezone}` | 401、403、404 |
| POST | `/household/current` | 会话与 CSRF（已验证邮箱且未加入家庭） | `name`（最多 100 字） | 201 `{household_id}`，创建者为家庭管理员 | 400、401、403、409 |
| PATCH | `/household/current` | 家庭管理员与 CSRF | `id`、`name`（去除首尾空白后 1–100 字） | 200 `{id,name}` | 400、401、403、409 当前家庭已变化 |
| DELETE | `/household/current?id={id}` | 家庭管理员与 CSRF | 当前家庭 ID | 200 `{status:"archived"}`，保留历史账本，解绑成员并撤销未决邀请 | 401、403、409 当前家庭已变化 |
| GET | `/household/members` | 会话 | 无 | 200 `{items:[{id,name,status,bound,user_id?,username?,role?,user_status?}],total}`（排除已删除；`bound` 表示已绑定登录账号，`user_status` 为绑定账号状态） | 401 |
| POST | `/household/members` | 会话 | `name`（1–100 字） | 201 `{id,name,status:"active"}` | 400 |
| DELETE | `/household/members/{id}` | 会话 | 无 | 200 `{id,status:"deleted"}`（软删除） | 404、409 `member_in_use`（已绑定登录、被邀请或被账务引用） |
| POST | `/household/members/current` | 会话 | 空对象 `{}` 即可 | 200 `{id,name,status}`；缺失时自动创建并绑定当前用户 | 401 |

说明：邀请默认 48 小时有效；审批通过后为申请人创建 `member` 账号并绑定成员档案。邀请接口在路由层只限制 `viewer` 写操作；`bind_existing` 变体仅管理员可用，用于把本家庭已有账号绑定到未绑定成员，并撤销该成员的未决邀请。

## 5. 管理员（11 条，均需 `role=admin`）

| 方法 | 路径 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/admin/invitations` | 无 | 200 `{items:[{id,member_name,target_username,expires_at}]}`（`id` 为邀请令牌哈希） | 401、403 |
| DELETE | `/admin/invitations/{id}` | 无 | 204 无响应体；同时使未决申请失效 | 403、404 |
| GET | `/admin/join-applications` | 无 | 200 `{items:[{id,username,member_name}]}` | 403 |
| POST | `/admin/join-applications/{id}` | `approve`（布尔，必填） | 200 `{status:"decided"}`；同意时创建账号并绑定成员 | 400、403、409 已处理/过期/已绑定 |
| GET | `/admin/users` | 无 | 200 `{items:[{id,username,role,status,member_name?,email?,avatar?,birthday?,payday?}]}` | 403 |
| PATCH | `/admin/users/{id}/role` | `role`（`admin` / `member` / `viewer`） | 200 `{id,role}`；可修改本家庭其他启用账号的角色，角色对已有会话立即生效 | 400、403（不能修改自己的角色）、404、409 `last_admin`（并发降级时保底，不能没有管理员） |
| POST | `/admin/users/{id}/disable` | 无 | 200 `{id,status:"disabled"}`；撤销该用户全部会话 | 403、404 |
| PATCH | `/admin/users/{id}/profile` | 同 §2 资料字段（目标仅限本家庭 `member` / `viewer`） | 200 `{status:"ok",updated:[...]}` | 400、403、404 |
| POST | `/admin/users/{id}/password` | 无（不读取请求体） | **固定 403** `{code:"verification_required"}`：管理员不能重置密码，成员须走邮箱找回 | 403 |
| DELETE | `/admin/users/{id}` | 无 | 200 `{id,status:"deleted"}`；删除会话、成员绑定与邮件身份，保留成员档案与历史账务 | 403、404（管理员账号不可删除） |
| GET | `/admin/audit-logs` | 查询 `limit`（1–100，默认 50） | 200 `{items:[{id,actor,action,entity,entity_id,created_at}]}`（不含 payload） | 400、403 |

## 6. 分类（4 条）

鉴权：会话；`viewer` 只允许 GET，写请求 403。

| 方法 | 路径 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/categories` | 查询 `kind?`（`expense` / `income`） | 200 `{items:[{id,kind,name,emoji,color,sort,active}]}`，按 `kind,sort,name,id`；包含已停用（`active=0`），排除已删除（`active=-1`） | 400 |
| POST | `/categories` | `kind`（`expense` / `income`）、`name`（1–50 字）、`emoji`（图标键或 emoji，≤16 字符且无控制字符，可空）、`color?`（`1`–`8` 调色板键或空）、`sort?` | 201 `{id,kind,name,emoji,color,sort,active:1}` | 400 |
| PATCH | `/categories/{id}` | `name?` / `emoji?` / `color?` / `sort?` / `active?`（至少一项） | 200 `{id}` | 400、404 |
| DELETE | `/categories/{id}` | 无 | 200 `{id,status:"deleted"}`；软删除（`active=-1`），历史分录仍可解析分类 | 404 |

说明：分类为**单层**，每笔流水只有一个 `category_id`；改分类名不会回写历史流水，历史流水只保存 `category_id`。

## 7. 账户（7 条）

鉴权：会话；`viewer` 只允许 GET。

| 方法 | 路径 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/accounts` | 查询 `include?`（空、`credit_debt`、`investments` 或逗号组合） | 200 `{items:[{id,kind,subtype,name,owner_member_id,status,note,statement_day,due_day,balance_minor}], credit_debt?, investments?}`（排除 `status=deleted`；逐账户由分录重建余额；`credit_debt` 追加还款计划投影；`investments` 追加只读估值快照） | 400（`include` 非法）、401 |
| POST | `/accounts` | `kind`（`cash` / `credit`）、`name`（1–100 字）、`subtype?`、`owner_member_id?`、`note?`、`opening_minor?`（≥0）、`statement_day?` / `due_day?`（1–31，仅 `credit`，必须同时提供） | 201 `{id,name,kind,ledger_code,opening_minor,balance_minor,statement_day,due_day}`；创建时同步写 `ledger_accounts`，期初余额写分录 | 400 `invalid_request` / `invalid_amount` |
| GET | `/accounts/{id}` | 无 | 200 `{id,kind,subtype,name,owner_member_id,status,note,statement_day,due_day,ledger_code,balance_minor}` | 404 |
| PATCH | `/accounts/{id}` | `name?` / `subtype?` / `owner_member_id?` / `note?` / `status?`（`active` / `disabled`）/ `statement_day?` / `due_day?`（1–31 或显式 `null`；合并后两字段必须同时有值或同时为空，仅 `credit`） | 200 `{id}` | 400、404 |
| DELETE | `/accounts/{id}` | 无 | 200 `{id,status:"deleted"}`；软删除，历史分录与余额保留 | 404 |
| GET | `/accounts/{id}/journal` | 无 | 200 `{items:[{id,occurred_on,kind,reversal_of,change_minor}],limit:100}`；最多最近 100 条，`change_minor` 正数=借方净增 | 404 |
| POST | `/accounts/transfers` | `from_account_id`、`to_account_id`、`amount_minor`（>0）、`occurred_on`、`note?`；可选 `Idempotency-Key` | 201 `{id,entry_id,status:"posted"}`；重复幂等键返回 200 `{id,status:"posted",replayed:true}` | 400 `invalid_amount` / `invalid_request`（同账户、信用卡作为转出账户被拒）、500 |

说明：`cash -> cash` 为普通转账（净资产不变）；`cash -> credit` 按**信用卡还款**入账（借贷记 `2010` / 贷 `1010`，不重复计支出）；`credit -> cash` 被拒绝。

`statement_day` 是账单截止日（日），`due_day` 是每月还款日（日）。账单周期起点由上一账单截止日次日推导；还款日取严格晚于账单截止日的第一个 `due_day`，跨月自动顺延。两列均可空；空表示「未配置」，不参与发薪日前现金计算。修改配置不会改写历史分录或账单，只会让派生计划按当前配置重算。

### 7.1 信用卡还款计划投影

`GET /accounts?include=credit_debt` 追加只读字段。所有金额为整数分；没有发薪日或没有任何已配置卡时，汇总金额为 `null`（不是 0）。单张未配置卡的 `cash_needed_minor` 为 `null`，但其欠款仍计入 `total_debt_minor`。

| 字段 | 说明 |
| --- | --- |
| `today` | 当前 Asia/Shanghai 业务日 |
| `payday` / `next_payday` / `days_until_payday` | 当前用户 `users.payday`、严格晚于今天的下一个发薪日、相差天数；未设置时为 `null` |
| `complete` | 发薪日已设置且所有信用卡都配置了账单日/还款日 |
| `unconfigured_cards` | 未配置的信用卡数量 |
| `cash_available_minor` | 启用中现金账户截至今天的余额合计 |
| `cash_needed_minor` | 到期日 ≤ 下次发薪日的未还金额合计（含逾期）；无发薪日或没有任何已配置卡时为 `null` |
| `cash_gap_minor` | `max(0, cash_needed_minor − cash_available_minor)`；无发薪日或没有任何已配置卡时为 `null` |
| `total_debt_minor` | 所有信用卡未还金额合计（含未出账与逾期，不含溢缴款） |
| `overdue_minor` / `unbilled_minor` | 已配置卡的逾期 / 未出账金额合计 |
| `cards[]` | 未删除信用卡列表；字段见下 |

每张卡字段：`account_id`、`name`、`status`、`configured`、`statement_day`、`due_day`、`total_debt_minor`、`unbilled_minor`、`overdue_minor`、`credit_balance_minor`、`cash_needed_minor`、`next_due_on`、`next_due_minor`、`next_due_period_start_on`、`next_due_period_end_on`、`current_cycle`（`{start_on,end_on,due_on}` 或 `null`）。

- 还款按到期日从早到晚 FIFO 冲抵；还款多于欠款形成 `credit_balance_minor` 溢缴款。
- 期初欠款视为在账户创建时已出账，按创建日之前最近的账单截止日归期。
- 已冲正/已删除/已更正的旧流水不计入；统计从 `journal_lines` + 当前有效 `transactions` 派生，不写分录、不缓存。

### 7.2 证券/理财估值投影

`GET /accounts?include=investments` 追加 `investments` 对象。它是 pfm 导入的只读快照，不写分录、不参与转账；`accounts.items` 仍只含 `cash` / `credit`。

顶层字段：`total_cost_minor`、`total_holdings_value_minor`、`total_cash_minor`、`total_market_value_minor`（= 持仓 + 现金）、`total_unrealized_pnl_minor`、`total_dividends_minor`、`total_return_minor`（= 未实现 + 股息）、`total_return_bp`、`total_dividend_yield_bp`、`holding_count`、`account_count`。

`accounts[]`：`id`、`kind`（`security` / `wealth`）、`name`、`institution`、`owner_member_id`、`owner_name`、`cash_minor`、`holdings_value_minor`、`market_value_minor`、`cost_minor`、`unrealized_pnl_minor`、`dividends_minor`、`return_minor`、`return_bp`、`dividend_yield_bp`、`holding_count`、`holdings[]`。

`holdings[]`：`id`、`symbol`、`name`、`asset_class`、`asset_label`、`quantity_micros`、`price_micros`、`cost_minor`、`market_value_minor`、`unrealized_pnl_minor`、`return_bp`、`dividends_minor`、`total_return_minor`、`total_return_bp`、`dividend_yield_bp`、`last_trade_on?`、`last_trade_pnl_minor`、`last_trade_return_bp`、`weight_bp`。

另返回 `by_owner[]`、`by_kind[]`、`by_asset_class[]`（含 `share_bp`/`weight_bp`）与按累计收益排序的 `top_holdings[]`。收益率均为基点，分母为对应成本；`last_trade_on` 来自 pfm 最近一次交易快照，不是完整买卖流水。

## 8. 流水（5 条）

鉴权：会话；`viewer` 只允许 GET。

| 方法 | 路径 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/transactions` | 查询 `from?`、`to?`（`YYYY-MM-DD`，含边界）、`type?`（`expense` / `income` / `transfer`）、`category_id?`、`account_id?`（匹配转出或转入）、`q?`（备注/分类名服务端 LIKE）、`cursor?`、`limit?`（1–200，默认 50） | 200 `{items:[流水行],next_cursor: string\|null}`；按 `occurred_on DESC, created_at DESC, id DESC` | 400 参数非法或游标无效 |
| POST | `/transactions` | `type`、`amount_minor`（>0）、`occurred_on`、`category_id?`、`account_id?`、`to_account_id?`、`member_id?`、`note?`；可选 `Idempotency-Key` | 201 `{id,entry_id,status:"posted"}`；幂等重放 200 `{id,status:"posted",replayed:true}` | 400 `invalid_amount` / `invalid_request`（分类方向不匹配、账户停用、收入必须记现金账户等） |
| GET | `/transactions/{id}` | 无 | 200 流水行 + `journal_lines:[{account,debit_minor,credit_minor}]` | 404 |
| PATCH | `/transactions/{id}` | 部分字段：`amount_minor?`、`occurred_on?`、`category_id?`、`account_id?`、`to_account_id?`、`member_id?`、`note?`；`type` 若传入必须与原值相同 | 200 `{id,replaces,entry_id,status:"posted"}`：原流水冲正并生成新流水 | 400（含「更正不支持修改流水类型」）、404、409 `conflict`（已冲正） |
| DELETE | `/transactions/{id}` | 无 | 200 `{id,status:"reversed"}`：只写冲正分录，不物理删除 | 404、409（重复删除） |

流水行字段：

| 字段 | 说明 |
| --- | --- |
| `id` | 流水 ID（`tx_` 前缀） |
| `type` | `expense` / `income` / `transfer` |
| `amount_minor` | 正整数分 |
| `currency` | V2 固定 `CNY` |
| `occurred_on` | 业务日期 `YYYY-MM-DD` |
| `category_id` / `category_name` / `category_emoji` / `category_color` | 支出/收入分类与调色板键；转账为 `null` |
| `account_id` / `account_name` | 转出账户或支出/收入账户 |
| `to_account_id` / `to_account_name` | 转入账户；非转账为 `null` |
| `member_id` / `note` | 可空 |
| `status` | `posted` / `reversed` |
| `entry_id` / `created_at` | 对应分录 ID 与创建时间（UTC RFC3339） |

说明：`type=transfer` 时若 `to_account_id` 是 `credit` 账户，按**信用卡还款**入账（见 [账务规则](accounting-rules-v2.md) §3 规则 5），因此转账列表里会同时出现普通账户互转与还款流水。

## 9. 统计（4 条）

统计一律由 `journal_lines` 派生，家庭范围在 SQL 内限定；转账与信用卡还款不计入收支。

| 方法 | 路径 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/stats/summary` | 查询 `from?`、`to?`；缺省为本业务月首日至今天 | 200 `{from,to,income_minor,expense_minor,balance_minor}`；`balance = income − expense` | 400（日期非法或 `from > to`） |
| GET | `/stats/categories` | 查询 `from?`、`to?`、`kind?`（默认 `expense`） | 200 `{from,to,kind,items:[{category_id,name,emoji,color,amount_minor,share_bp}]}`；按金额降序，净额为零的分类不返回 | 400 |
| GET | `/stats/daily` | 查询 `month?`（`YYYY-MM`，默认当前业务月） | 200 `{month,days:[{day,expense_minor,income_minor}]}`，按日升序，仅返回有净额的日子 | 400 |
| GET | `/stats/monthly` | 查询 `to?`（`YYYY-MM`，默认当前业务月）、`months?`（1–120，默认 6） | 200 `{months:[{month,income_minor,expense_minor}]}`，含无数据月份（补 0） | 400 |

口径定义见 [V2 账务规则](accounting-rules-v2.md) §6：收入科目 `4010:%`（贷−借），支出科目 `5010:%`（借−贷），分类 ID 为 `SUBSTR(account_code, 6)`；`share_bp` 为金额占同口径合计的万分比整数（向下取整）。

## 10. 预算（5 条）

| 方法 | 路径 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/budgets/{month}` | 路径 `month` 为 `YYYY-MM` | 200 `{month,amount_minor:number\|null,spent_minor}`；未设置时 `amount_minor` 为 `null`（不是 0） | 400 |
| PUT | `/budgets/{month}` | `amount_minor`（≥0 整数分） | 200 `{month,amount_minor,spent_minor}`；按 `(household_id,month)` 覆盖写入并写审计 | 400、500 |

`spent_minor` 为该月支出科目净额（借−贷），与账单页预算进度使用同一口径。

| 方法 | 路径 | 请求 | 成功 |
| --- | --- | --- | --- |
| GET | `/project-budgets` | 无 | `{items:[{id,name,amount_minor,spent_minor}]}` |
| POST | `/project-budgets` | `name`、`amount_minor`（正整数分） | `{id}` |
| PUT | `/project-budgets/{id}` | `name`、`amount_minor`（正整数分） | `{id}` |

支出可通过 `budget_id` 关联一个家庭内项目。更正时省略该字段保留关联，传空字符串清除关联。仅提交 `budget_id` 时只更新关联和审计记录。项目实际支出按关联的有效分录跨月累计，同时仍计入发生当月的月度预算。

## 11. 信用卡账单邮箱与对账（10 条）

鉴权：会话，且当前用户 `role` ∈ `admin` / `member`（`viewer` 与已被降级的账号在写路径上返回 403 `forbidden`「当前账号不能修改此邮箱配置」）。所有路径都接受查询参数 `provider`，取值 `qq`（默认，含空值）或 `163`，其他值 400 `invalid_request`「请选择 QQ 或 163 邮箱」。

邮箱配置按 `(household_id, user_id, provider)` 隔离，`version` 是乐观锁：读取返回当前版本，写入必须回传同一版本，成功后自增。授权码以 AES-GCM 密文存 `auth_code_cipher`（密钥来自 `TINYNEST_MAIL_KEY_FILE`），响应只回显掩码邮箱，**任何接口都不返回明文邮箱或授权码**；IMAP 全程只读（不删信、不改旗标）。

| 方法 | 路径 | 请求 | 响应 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/credit-statement-mailbox` | 查询 `provider?` | 200 `{provider,enabled,masked_email?,version,key_ready}`（未配置时 `enabled:false`、`version:0`、无 `masked_email`；`key_ready` 表示 32 字节密钥就绪） | 400、401 |
| PUT | `/credit-statement-mailbox` | `email`、`auth_code`、`version`（必填，≥0；0 表示首次创建）；请求体上限 2 KiB | 200 同上，`version` 为 `旧值+1`；**先用凭据试连 IMAP 成功才落库**；邮箱地址变更时删除该邮箱在本服务商下的全部绑卡 | 400 `invalid_request`（地址与服务商域名不一致）、409 `conflict`（版本过期）、403、429 `rate_limited`、502 `mailbox_test_failed`、503 `mail_unavailable`（密钥未配置/不可用） |
| DELETE | `/credit-statement-mailbox` | `version`（必填，≥0）；请求体上限 256 B | 200 同上，`enabled:false`、`version` 为 `旧值+1`（清空密文而非删除行，保留版本单调性） | 400、401、403、409、503 |
| POST | `/credit-statement-mailbox/test` | 未保存测试：`email`、`auth_code`；已保存测试：仅 `version`（≥1，此时 `email`/`auth_code` 必须为空） | 200 `{status:"connected"}`（仅证明能列出 INBOX，不读取任何邮件正文） | 400、409、429（按用户 + IP 限流，失败计数会延长冷却）、502 `mailbox_test_failed`、503 `mail_unavailable` |
| GET | `/credit-statement-mailbox/cards` | 查询 `provider?` | 200 `{items:[{id,name,status,bound,taken}]}`，本家庭 `kind=credit` 且（`active`，或 `disabled` 但仍绑定给当前用户与本服务商）；`bound` 属于当前用户+服务商，`taken` 属于他人 | 400、401 |
| PUT | `/accounts/{id}/credit-statement-binding` | 查询 `provider?`、`version`（必填 ≥1，须等于当前邮箱配置版本）；无请求体 | 204 无响应体；写审计 `bind_statement_card` | 400 `invalid_request`（缺版本或卡非 `active`）、404 `not_found`、409 `card_already_bound` / `conflict`、403、503 |
| DELETE | `/accounts/{id}/credit-statement-binding` | 同上 | 204 无响应体（重复解绑幂等）；写审计 `unbind_statement_card`；被他人绑定视为 404 | 400、404、409 `conflict`、403、503 |
| POST | `/accounts/{id}/credit-statement-preview` | 查询 `provider?`、`version`（必填 ≥1） | 200 `{items:[{received_on?,statement_on?,due_on?,amount_due_minor?,needs_review?}],since:"YYYY-MM-DD"}`，`since` 为业务今天前 30 天；只读 IMAP 搜索并把解析结果**只回显、不落库** | 400 `unsupported_statement_card`、404（该卡未绑定给当前邮箱）、409、429 `rate_limited`、502 `statement_preview_failed`、503 |
| POST | `/accounts/{id}/credit-statement-sync` | 同上，`since` 为业务今天前 45 天 | 200 `{since,created,linked,conflicts,needs_review,items:[{statement_on,due_on,result}]}`，`result` ∈ `created`/`linked`/`conflict`/`needs_review` | 同预览 |
| GET | `/credit-statements` | 无 | 200 `{items:[{id,account_id,account_name,period_start_on,period_end_on,due_on,currency,amount_due_minor,status,review_reason,source_count,ledger_charge_minor,difference_minor}]}`，按 `period_end_on` 降序；`ledger_charge_minor`/`difference_minor` 在无账期起点时为 `null` | 401 |

约束与口径：

- 卡片识别：`credit` 账户名须同时命中支持的银行（浦发 `spdb`、中信 `citic`）且以 `（1234）`/`(1234)` 结尾，否则 400 `unsupported_statement_card`；命中多家用 `&&` 互斥判定，两家都提到即视为不支持。
- 绑定的读取门槛：卡 `kind='credit' AND status='active'`，绑定行 `(household_id, account_id)` 唯一（一卡一邮箱一服务商），且绑定人的 `users.status='active'` 且 `role IN ('admin','member')`；不满足即 404，绝不跨家庭或跨用户读取。
- 网络访问前的全部校验（绑定、卡名、配置版本、解密授权码）在 `prepareStatementRead` 内完成；预览、同步与连接测试共用容量为 4 的并发闸 `mailboxProbeSlots`，超出一律 429。
- 读取之后、报告之前会**重新校验**邮箱版本与卡名（`readStatementMails`）：期间禁用邮箱（版本自增）或改名都会得到 409 `conflict`，不返回可能被错配的结果。
- 落库只写 `card_statements` / `card_statement_sources`（见 [数据模型](er-v2.md) §6.4），不写 `transactions` / `journal_entries`，也不被还款计划读取。同一 `(household_id, account_id, period_end_on)` 已存在时**保留首次记录的金额**，只有金额不一致才在 `review_reason=''` 时写入「银行账单金额与已记录金额不一致，请核对」并计为 `conflict`。
- 来源幂等：`digest = SHA-256("email", provider, user_id, IMAP UID, account_id, statement_on, due_on, 卡尾号, amount_due, "CNY")`，配合 `UNIQUE(household_id, digest)`；同一封邮件重放只增加不重复，且不新增审计。
- 审计只在 `created + conflicts > 0` 时写一条 `sync_card_statement`（`entity="card_statement"`，`entity_id` 为卡账户 ID，payload 含三种计数）。
- `ledger_charge_minor` 由同一账期内该卡的**有效**支出分录聚合（`e.status='posted' AND e.reversal_of IS NULL AND biz_type='expense'` 且 `t.status='posted' AND t.type='expense'`），`difference_minor = amount_due_minor − ledger_charge_minor`；两者仅作只读比对，任何非零差额都不触发自动过账。

## 12. 健康检查（2 条，不计入 72）

| 方法 | 路径 | 鉴权 | 响应 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/healthz` | 公开 | 200 `{app:"tinynest-api",status:"ok",phase:"v0.1"}`；`phase` 是历史标签，当前值仍为 `v0.1` | 非 GET → 405 `{status:"method_not_allowed"}` |
| GET | `/api/v1/readyz` | 公开 | 200 `{status:"ready",phase:"v0.1",check:"sqlite_quick_check_and_migrations"}`（SQLite）或 `check:"mysql_ping_and_migrations"`（MySQL） | 503 `{status:"not_ready",reason:"data_dir_unavailable"}` / `{reason:"database_unavailable"}`；非 GET → 405 |

`readyz` 的数据库检查为组合根注入：SQLite 执行 `PRAGMA quick_check`，MySQL 执行 ping；两者都校验迁移版本完整。

## 13. 幂等与家庭隔离规则

### 13.1 幂等

- 生效范围：`POST /transactions`、`POST /accounts/transfers`。其余写接口（PATCH/DELETE、分类/账户/预算、成员、邀请、管理员操作）**不**读取幂等键。
- 请求头：`Idempotency-Key: <调用方生成的唯一字符串>`；省略即不做去重。
- 存储：键写入 `transactions.idempotency_key`（唯一约束）与 `journal_entries.idempotency_key`（唯一约束）。
- 重放：同一家庭内已提交过该键时，直接返回 200 `{id,status:"posted",replayed:true}`，不重复记账；事务失败后还会再查一次，覆盖并发请求已提交同键的情况。
- 幂等键不跨家庭复用：回放查询按 `(idempotency_key, household_id)` 限定，其他家庭看不到该流水。
- 账单同步不使用 `Idempotency-Key`：去重靠数据库唯一约束 `(household_id, account_id, period_end_on)` 与来源 `UNIQUE(household_id, digest)`，以及邮箱配置 `version` 的乐观锁，因此重复点击同步不会产生第二行账单或第二条来源。

### 13.2 家庭隔离与只读角色

- 所有财务 SQL 都以 `WHERE household_id=?` 限定；`journal_lines` 通过 `JOIN journal_entries ON entry_id` 继承家庭范围，不存在「先全库聚合再过滤」的查询。
- 跨家庭 ID 按不存在处理（404 或业务 400），不返回他人余额、流水或成员。
- `viewer` 角色：允许 GET 与登出、改自己资料、改自己密码；其他写操作 403「只读账号不能修改家庭账本」。
- CSRF 校验在会话内写请求上先于业务逻辑；缺失或错误返回 403 `forbidden`。公开端点与登出不经过该中间件（公开端点由各自的令牌/验证码/限流保护）。
