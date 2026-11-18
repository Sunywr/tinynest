# TinyNest V0.1 · API 契约（含实现状态）

> 依据 V0.1 实施计划第 14 节与**当前实际注册路由**。前缀统一 `/api/v1`。
> **状态定义**：`IMPLEMENTED` = 已注册且有自动化回归；`PARTIAL` = 已注册但能力/口径未完整；`PLANNED` = 仅设计、未注册路由。
> 凡标 `PLANNED` 的端点尚未注册。当前状态以 `apps/api/internal/api/*.go` 的路由注册和自动化回归为准。

## 1. 通用约定

| 项 | 约定 | 状态 |
|---|---|---|
| 传输 | JSON；`Content-Type: application/json; charset=utf-8` | IMPLEMENTED |
| 金额 | 一律 **整数分**（`*_minor`）；禁止 JSON 浮点小数 | IMPLEMENTED |
| 数量/价格 | `quantity_scaled` + `quantity_scale`；`price_scaled` + `price_scale` | IMPLEMENTED |
| 日期 | `YYYY-MM-DD`（**Asia/Shanghai 业务日**）；时间戳 RFC3339 UTC | IMPLEMENTED（FIX-03 后统一可注入时钟） |
| 分页 | 部分列表端点支持分页；**并非所有列表都实现** | PARTIAL |
| 幂等 | `Idempotency-Key` **仅部分写接口**接入（`transactions`、`refunds`、部分信用/投资写入） | PARTIAL — 不得宣称「所有写操作统一幂等」 |
| 错误 | `{"error":{"code":"...","message":"..."}}`；不泄露 SQL / 路径 / 堆栈 | IMPLEMENTED |
| 缓存 | 所有 `/api/v1/*` 响应强制 `Cache-Control: no-store` | IMPLEMENTED |
| 认证 | Cookie Session（HttpOnly / `Secure` 由 `TINYNEST_COOKIE_SECURE` 显式配置 / SameSite=Lax）+ 写操作 `X-CSRF-Token` | IMPLEMENTED |
| 数据隔离 | 所有财务查询在 **SQL 层**按 `household_id` 限定 | IMPLEMENTED（FIX-02） |
| 登录保护 | 失败限流（IP + 用户名，有界冷却）→ `429` + `Retry-After` | IMPLEMENTED（FIX-05） |

标准错误码：`unauthorized` / `forbidden` / `setup_disabled` / `rate_limited` / `invalid_amount` / `unbalanced_entry` / `idempotency_conflict` / `refund_exceeds_original` / `split_sum_mismatch` / `schedule_sum_mismatch` / `duplicate_biz_key` / `not_found` / `conflict` / `internal_error`。

### 1.1 公开端点与安全边界（FIX-01 后）

| 端点 | 状态 | 认证 | 说明 |
|---|---|---|---|
| `GET /healthz` | IMPLEMENTED | 无 | 仅进程存活 |
| `GET /readyz` | IMPLEMENTED | 无 | SQLite `PRAGMA quick_check` 或 MySQL ping，并校验迁移版本 |
| `POST /auth/session` | IMPLEMENTED | 无（口令校验 + 失败限流） | 成功后签发会话 Cookie |
| `POST /setup` | IMPLEMENTED（运维专用，无公开 UI） | **一次性 `X-Setup-Token`** | 仅空库可调用一次，创建首个家庭与唯一 admin；`TINYNEST_SETUP_DISABLED=true` 时全量拒绝 |

- Bootstrap Token 由部署者在后端主机通过 `TINYNEST_SETUP_TOKEN` 主动设置；**未配置时端点返回 403 `setup_disabled`（fail closed）**，绝不隐式开放。
- **生产初始化完成后**：设置 `TINYNEST_SETUP_DISABLED=true`，端点在令牌校验与任何数据库读取之前即返回 403 `setup_disabled`，即使令牌外泄也无法调用。该开关只是附加保护，不替代单次创建约束与令牌校验；取值严格为 `true`/`false`，非法值服务拒绝启动。
- 令牌比较在固定长度 SHA-256 摘要上做常量时间比较；不写入日志、不进入仓库、不进入浏览器打包代码。
- 已初始化后（含并发请求）一律返回 409 `conflict`，令牌不可再用于新增 admin。
- **不能**用 `RemoteAddr == 127.0.0.1` 代替令牌：Nginx 反代会让公网请求在 Go 侧显示为本机。可在 Nginx 额外限制初始化入口，但不可替代令牌。
- **公开前端不再提供任何初始化/创建管理员入口**。首次部署流程：在 B 机生成随机令牌 → 注入服务环境 → 启动 API → 由运维在受控环境用一次 `curl`/HTTP 客户端携带 `X-Setup-Token` 调用 `POST /api/v1/setup` → 确认 `201` → 从环境移除令牌并设置 `TINYNEST_SETUP_DISABLED=true`。普通访客只能登录。

### 1.2 生产 Cookie 与可信反代（FIX-04 后）

`r.TLS` 在 Go 侧恒为 nil（Nginx 终止 TLS 后明文转发），因此**不再用于判定安全属性**。`Secure` 由 `TINYNEST_COOKIE_SECURE=true|false` 显式决定；未设置或取值非法时服务拒绝启动。若采用 `X-Forwarded-Proto` 辅助判定，只有 `TINYNEST_TRUSTED_PROXIES` 中配置的来源才被采信，任意客户端伪造的请求头无效。

## 2. 接口分组（逐路由状态）

### `/auth` · `/setup`

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| POST | `/setup` | IMPLEMENTED | 需 `X-Setup-Token`，仅一次 |
| POST | `/auth/session` | IMPLEMENTED | 登录、建会话；含失败限流 |
| DELETE | `/auth/session` | IMPLEMENTED | 登出、撤销会话 |
| GET | `/auth/me` | IMPLEMENTED | 当前用户、角色、household_id 与 csrf_token；已绑定且启用的成员档案返回 member_id 和 member_name，否则为空。已设置的 avatar、birthday、payday、email 随响应返回 |
| PATCH | `/auth/me/profile` | IMPLEMENTED | 更新自己的姓名和可选资料，返回 `status` 和 `updated` 字段列表 |
| POST | `/auth/me/password` | IMPLEMENTED | 提交 `current_password` 和 `new_password` 修改自己的密码；额外要求邮箱验证 proof，成功撤销全部会话 |
| POST | `/auth/register` · `/auth/join` | IMPLEMENTED | 注册登录名、凭邀请码申请加入家庭。申请由家庭管理员审批 |

### `/household` · `/categories` · `/projects`（S1）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/categories` | IMPLEMENTED | 分类列表 / 新增（两级，level<=2） |
| PATCH | `/categories/{id}` | IMPLEMENTED | 传入任一字段：`name`（非空）、`sort`（整数）、`active`（布尔值）；仅更新本家庭分类。停用后不可用于新流水，历史关联保留 |
| DELETE | `/categories/{id}` | IMPLEMENTED | 从管理列表和记账选项移除，保留历史关联。有子分类时返回 409，重复删除返回 200 |
| GET/POST | `/projects` | IMPLEMENTED | 项目列表 / 新建 |
| GET | `/projects/{id}/summary` | IMPLEMENTED | 累计收支、退款、净支出、分类构成与最近 100 条拆分流水 |
| GET | `/projects/{id}` · PATCH `/projects/{id}` | IMPLEMENTED | 项目详情与编辑。支持名称、起止日期、说明和启用或归档状态；日期或说明传空字符串可清除 |
| DELETE | `/projects/{id}` | IMPLEMENTED | 从管理列表和记账选项移除，保留历史关联和项目汇总。重复删除返回 200 |
| GET/POST | `/household/members` | IMPLEMENTED | 本家庭成员列表与新增；`name` 为 1 至 100 个字符，成员档案不创建登录账号 |
| DELETE | `/household/members/{id}` | IMPLEMENTED | 删除未关联档案并记录审计，重复删除成功；有关联登录、邀请历史或账务记录时返回 409 |
| POST | `/household/members/current` | IMPLEMENTED | 返回当前可记账账号绑定的成员档案。尚未绑定的旧账号按用户名创建并绑定自己的档案。重复调用返回同一档案 |
| GET | `/household/current` | IMPLEMENTED | 当前会话所属家庭的名称、币种和业务时区 |
| POST | `/household/invitations` | IMPLEMENTED | 为现有或新建成员档案签发邀请 |

### `/accounts`（S1）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/accounts` | IMPLEMENTED | 现金/证券/理财账户；创建可带 `opening_balance_minor` |
| GET | `/accounts/{id}/balance` | IMPLEMENTED | 由分录重建余额 |
| GET/PATCH | `/accounts/{id}` | IMPLEMENTED | 账户详情与资料编辑（名称、子类型、归属成员、备注、启用状态），余额由分录计算 |
| DELETE | `/accounts/{id}` | IMPLEMENTED | 移出家庭账户列表、首页账户列表和记账选项。保留分录、余额与报表影响，重复删除返回相同状态 |
| GET | `/ledger/accounts` | PLANNED | 完整科目表。现有账户、账户分录和净资产接口支持已公开的账户与科目下钻 |
| GET | `/accounts/{id}/journal` | IMPLEMENTED | 本家庭账户最近 100 笔分录净变动，含期初余额和冲正 |

### `/transactions` · `/refunds`（S2 / S4）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/transactions` | IMPLEMENTED | 流水筛选与游标分页 / 支出·收入·转账（`splits[]` 合计=总额） |
| GET | `/transactions/{id}` | IMPLEMENTED | 详情（业务 + 分录影响） |
| DELETE | `/transactions/{id}` | IMPLEMENTED | 删除普通流水，保留账务审计并撤销原日影响，重复删除返回 204 |
| POST | `/transactions/{id}/replace` | IMPLEMENTED | 同一事务内冲正原流水并入账更正记录，请求体同新建 |
| POST | `/refunds` | IMPLEMENTED | 全额/部分退款、超额拒绝 |
| GET | `/refunds/by-transaction/{id}` | IMPLEMENTED | 累计退款、净支出、剩余可退 |

`GET /transactions` 接受 `from`、`to`（含边界，YYYY-MM-DD）、`type`（expense、income、transfer）及 `limit`（1–200，默认 200）。响应含 `items`、本页条数 `total` 和 `next_cursor`。下一页保持原筛选条件，将 `next_cursor` 传为 `cursor`；值为 `null` 时已到末页。流水按业务日期、创建时间与 ID 降序排列。列表每项还返回 `category_names`（拆分类别去重名称）和 `refunded_minor`（仍有效退款合计，整数分）；已撤销退款不计入。界面据此区分未退款、部分退款和全额退款。

流水详情同时返回付款方式、账户、信用工具、成员、商户、备注及 `splits[]`，用于预填复制或修改。复制提交时走新建接口，默认使用上海当日。更正后的原记录仍以 `reversed` 表示，并在界面显示为「已更正」。修改支持 `Idempotency-Key`，重复提交同一流水的同一键会返回已创建的记录。已有退款、合同关联，或原信用账单已还款或已转分期时，删除和修改会拒绝并保留原记录。

`POST /transactions` 请求示例（信用消费 1000.00，拆两行）：

```json
{
  "type": "expense", "amount_minor": 100000, "currency": "CNY",
  "occurred_on": "2026-11-12", "payment_kind": "credit",
  "credit_instrument_id": "ci_01", "merchant": "超市",
  "splits": [
    {"category_id":"cat_food","amount_minor":60000,"project_id":null},
    {"category_id":"cat_daily","amount_minor":40000,"project_id":"prj_01"}
  ]
}
```

分录：借 `X_food` 60000 + `X_daily` 40000 / 贷 `L_ci_01` 100000。

### `/credit-instruments` · `/credit-statements`（S3）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/credit-instruments` | IMPLEMENTED | 信用卡、花呗、美团、抖音及自定义工具。列表返回 `issuer` |
| GET | `/credit-statements` | IMPLEMENTED | 账单列表（自动归集） |
| POST | `/credit-statements/{id}/payments` | IMPLEMENTED | 部分/全额还款 |
| POST | `/credit-statements/{id}/payments/{paymentId}/reverse` | IMPLEMENTED | 填写原因，按原还款日冲正误录还款，恢复付款账户余额、信用负债与账单待付提醒。重复提交返回同一冲正分录；账单详情保留原还款记录及状态 |
| PATCH | `/credit-statements/{id}/archive` | IMPLEMENTED | `{"archived":true}` 将未还金额恰为零的账单移出普通列表，保留明细与还款。传 `false` 恢复。通过 `GET /credit-statements?archived=true` 查看归档；账单余额再次变化时自动恢复 |
| POST | `/credit-statements/{id}/reconcile` | IMPLEMENTED | 录入平台账单金额（只记核对值） |
| PATCH | `/credit-instruments/{id}` | IMPLEMENTED | 周期、额度和启停，记录变更审计 |
| DELETE | `/credit-instruments/{id}` | IMPLEMENTED | 将未使用工具移出列表与支付选项，重复请求返回成功。已有账单、交易或分录时返回 409 |
| GET | `/credit-instruments/{id}/limit` | IMPLEMENTED | 实时负债、已用额度、可用额度与使用率（基点）；未配置额度时后两项为 null |
| GET | `/credit-statements/{id}` | IMPLEMENTED | 账单明细与核对差额 |
| PATCH | `/credit-statements/{id}/cycle` | IMPLEMENTED | 人工修正单期账单日期，须填写原因 |

账单列表和详情根据上海业务日与应还、实还金额派生状态。信用转分期见下节。

### `/installments` · `/debts` · `/schedule`（S4）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/installments` | IMPLEMENTED | 合同；校验期次本金/费用合计 |
| POST | `/installments/{id}/payments` | IMPLEMENTED | 确认实付（`channel=cash\|credit`） |
| POST | `/installments/{id}/payments/{paymentId}/reverse` | IMPLEMENTED | 填写原因，按原付款日冲正并恢复期次应付及提醒。信用渠道回写原账单，原账单未还额不足时返回 409。重复提交返回同一冲正分录 |
| POST | `/installments/convert-from-credit` | IMPLEMENTED（V0.1.1） | 指定 `credit_instrument_id` 与 `statement_id`，同一事务重分类本金、冲减原账单及已有待办。仅当该工具恰有一张未还账单时可省略 `statement_id` |
| GET | `/schedule/due-items` | IMPLEMENTED | 待支付事项（只读，业务日时钟） |
| POST | `/schedule/generate` | IMPLEMENTED | 手动幂等生成未来 30 天待办 |
| GET | `/schedule/runs` | IMPLEMENTED | 当前家庭最近 50 次投影记录，含成功及可持久化的失败 |
| GET | `/installments/{id}` | IMPLEMENTED | 合同与期次计划 |
| POST | `/installments/{id}/reverse` | IMPLEMENTED | 填写原因，按原入账日冲正无有效付款和退款的分期，取消期次及待付提醒。付款可先逐笔冲正，保留历史。信用账单转分期将本金恢复到原账单。重复提交返回同一冲正分录 |
| POST | `/installments/{id}/refunds` · `/installments/{id}/adjust-schedule` · `/installments/{id}/settle-early` | IMPLEMENTED | 退款、显式调整未付款期次与提前结清 |
| POST | `/installments/{id}/refunds/{refundId}/reverse` | IMPLEMENTED | 按原退款日期撤销，恢复原账户或信用账单。撤销后剩余退款不足以覆盖已减免金额时，自动按最近减免优先（同期次按本金优先）恢复期次应付并补回待付提醒，后续实付保留；减免来自提前结清或记录与计划不一致时返回 409。重复提交返回同一更正记录 |
| POST | `/installments/{id}/confirm-schedule` | PLANNED | 创建合同时已写入校验后的计划。后续调整通过 `adjust-schedule` 显式提交 |
| GET/POST | `/debts` | IMPLEMENTED | 房贷、贷款、保险等合同列表和创建 |
| GET/PATCH | `/debts/{id}` | IMPLEMENTED | 合同与期次详情、合同资料编辑 |
| DELETE | `/debts/{id}` | IMPLEMENTED | 删除零本金且无付款历史的合同，取消期次并移除待支付提醒。重复删除成功，有入账本金或付款历史返回 409 |
| POST | `/debts/{id}/reverse` | IMPLEMENTED | 填写原因，按原日期撤销合同起始本金，保留合同与记账历史并取消待付款。所有付款已按原日撤销时也可操作。旧版跨日处理的付款须单独核对。重复提交返回同一纠正记录 |
| POST | `/debts/{id}/schedule` · `/debts/{id}/payments` | IMPLEMENTED | 录入计划和按期确认付款 |
| POST | `/debts/{id}/payments/{paymentId}/reverse` · `/debts/{id}/settle-early` | IMPLEMENTED | 按原付款日撤销误录付款，恢复每期金额、待付款和原信用账单，或提前结清 |

### `/investments`（S5）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/investments/assets` | IMPLEMENTED | 股票/ETF/理财/自定义资产 |
| GET/POST | `/investments/trades` | IMPLEMENTED | 买入/卖出（移动加权平均成本）。列表保留冲正状态，`?order=posted` 按入账顺序查看最近 200 笔 |
| POST | `/investments/trades/{id}/reverse` | IMPLEMENTED | 冲正该资产任一笔已入账成交（按原成交日期生成抵销分录）。撤销早期成交时按入账顺序重放其余成交，重算其后卖出的移动加权成本与已实现收益，并为每笔成本变化的卖出补记联动更正分录（`invest_cost_correction`，backdate 至被更正卖出日）；重放将超卖或入账顺序不明确时返回 409。重复提交返回同一冲正分录 |
| GET | `/investments/positions` | IMPLEMENTED | 数量/总成本/移动均价/已实现·未实现收益/净分红/参考收益率；口径第 9 项已确认为「总收益 ÷ 累计买入成本」，**不得标为年化** |
| POST | `/investments/dividends` | IMPLEMENTED | `tax_minor: null` = 未填写（不默认 0） |
| GET | `/investments/dividends` | IMPLEMENTED | 分红历史与年度汇总，支持 `?asset_id=` / `?from=` / `?to=`。历史含分录状态，汇总仅纳入有效分红，未知税额不按 0 处理 |
| POST | `/investments/dividends/{id}/reverse` | IMPLEMENTED | 填写冲正原因，按原到账日期生成抵销分录并更新持仓净分红。重复提交返回同一冲正分录 |
| POST | `/investments/valuations` | IMPLEMENTED | 手工录入估值，估值日期显式展示 |
| DELETE | `/investments/valuations/{asset_id}/{valued_on}` | IMPLEMENTED | 按资产和日期删除手工估值，更新最新估值日期并记录审计。对同家庭资产的同日重复删除返回成功 |
| POST | `/investments/fee-rules` | IMPLEMENTED | 按投资账户追加版本。交易按成交日选择已生效版本并保存快照 |
| GET | `/investments/fee-rules` | IMPLEMENTED（V0.1.2） | 本家庭费率规则列表，支持 `?account_id=`；返回版本、生效日、各项 bps 与最低佣金；SQL 层 `household_id` 隔离 |
| POST | `/investments/fee-rules/preview` | IMPLEMENTED（V0.1.2） | 纯计算、零写入的费用预览；与真实交易共用 `computeTradeFee`，返回佣金/印花税/过户费/合计及适用规则版本 |
| GET | `/investments/return` | PLANNED | 独立收益率端点（当前由 `/positions` 与 `/dashboard/summary` 给出） |

### `/dashboard`（S6）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET | `/dashboard/summary` | IMPLEMENTED | 收入/支出/结余/现金净流量/应还负债/投资总收益/净资产；**全部按 household_id 隔离** |
| GET | `/dashboard/categories` | IMPLEMENTED | 分类支出排行（一级汇总二级） |
| GET | `/dashboard/projects` | IMPLEMENTED | 项目累计收支 |
| GET | `/dashboard/credit` | IMPLEMENTED | 按到期日统计账单应还、已还与逾期，并返回实时信用额度 |
| GET | `/dashboard/net-worth` | IMPLEMENTED（V0.1.2） | 资产构成 / 负债结构 / 投资成本·市值·估值日期 / 净资产，逐项带 `ledger_code` 可下钻；与 `summary` 使用同一统计窗口，`net_worth_minor` 一致 |
| GET | `/dashboard/cashflow` · `/dashboard/liabilities` · `/dashboard/investment-return` | PLANNED | 独立报表。现有数据入口为 `/dashboard/summary`、`/dashboard/net-worth`、`/schedule/due-items`、`/investments/positions` 与 `/investments/dividends` |

**三套口径（收支 / 现金流 / 负债到期）不得合并成一个「本月支出」**；当前三套数值均由 `/dashboard/summary` 分别给出。

`/dashboard/credit` 的信用总已用包含所有启用工具。汇总可用额度和使用率只按已配置额度的启用工具计算。没有已配置额度时，两项均返回 null。

### `/admin`（S7）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET | `/admin/audit-logs` | IMPLEMENTED | 当前家庭最近审计事件，`limit` 默认 50，范围 1–100，响应排除事件原始载荷 |
| GET | `/admin/invitations` · `/admin/join-applications` · `/admin/users` | IMPLEMENTED | 本家庭的有效邀请、待审批申请、账号列表 |
| DELETE | `/admin/invitations/{id}` | IMPLEMENTED | 撤销邀请及关联的待审批申请 |
| POST | `/admin/join-applications/{id}` · `/admin/users/{id}/disable` | IMPLEMENTED | 审批加入申请、停用成员账号并撤销会话 |
| PATCH | `/admin/users/{id}/role` | IMPLEMENTED | 更改启用成员的记账权限 |
| PATCH | `/admin/users/{id}/profile` | IMPLEMENTED | 编辑本家庭成员或只读账号资料，返回 `status` 和 `updated` |
| POST | `/admin/users/{id}/password` | IMPLEMENTED | 返回 403 verification_required；成员须通过邮箱自行找回 |
| DELETE | `/admin/users/{id}` | IMPLEMENTED | 删除本家庭成员或只读账号及其会话，保留成员档案、历史账务和审计 |
| GET / POST | `/admin/migrations` · `/admin/backup` | PLANNED | 迁移就绪由 `/readyz` 验证，具体版本和备份从部署侧数据库及备份任务核对 |

## 3. 与 Phase 0 的兼容边界

- `healthz` 报告进程存活。`readyz` 检查数据库连通性或完整性以及迁移版本。
- `apps/web` 仍只请求相对路径 `/api/v1/*`，不得硬编码主机、证书或数据库地址。
- Service Worker 继续显式跳过 `/api/`，且对所有 `/api/v1/*` 响应强制 `no-store`。
- Phase 0 的「无登录、纯虚构 Demo」描述仅适用于 `previews/*` 静态存档，不适用于当前 `apps/web`。

> 未完成模块与正式发布门禁见 `IMPLEMENTATION_REPORT.md` 第 8/9 节、`docs/reviews/v0.1-release-gates.md`；10 项账务口径状态见 `docs/reviews/v0.1-accounting-decisions.md`（**2026-11-14 用户已确认 1–10 全部推荐值，十项 `CONFIRMED`**；旧报告中的「0/10 待确认」为历史记录）。
> V0.1.2 追加端点的实施与测试证据见 `docs/reviews/latest-feature-evidence-matrix.md`、`docs/reviews/next-version-test-report.md`。

### 家庭账号角色

管理员可通过 `PATCH /admin/users/{id}/role` 将本家庭的启用成员设为 `member` 或 `viewer`。角色在每次请求时读取，已有会话随下一次请求采用新权限。只读成员可查看账本、更新自己的资料和密码并退出登录。账本写入与邀请码写操作返回 403。账号管理接口仅接受本家庭的 `member` 或 `viewer` 账号 ID，其他目标返回 404。

资料更新仅处理请求中的字段。姓名 `name` 写入已绑定的成员档案，未绑定时创建档案并绑定。用户名固定。头像可传最多 8 个字符的文字，或 64×64 像素 JPEG data URL（Base64 部分最多 32 KiB）。生日为 `YYYY-MM-DD` 且介于 1900-01-01 与上海业务日之间，发薪日为 1 至 31，邮箱最多 254 字节。可选字段可提交 `null` 清除，清除后响应省略对应字段。管理员账号列表对本家庭返回同样的可选字段。密码长度为 8 至 256 字节。所有资料及密码写入均要求有效会话和 CSRF。

## 邮件与邮箱验证（2026-11-18 已部署）

参见 [飞书邮件接入与上线说明](feishu-mail.md)。以下路径均位于 `/api/v1`，写操作除注册与找回外要求会话及 CSRF。

| 方法 | 路径 | 行为 |
| --- | --- | --- |
| GET / POST | `/site/mail` | 仅站点管理员读取脱敏全站配置、测试候选并原子保存；POST 要求 `config`、`password`（空则保留）、`current_password`、`test_recipient`、当前 `version` |
| GET | `/auth/me/mail` | 验证邮箱、公告订阅、站点管理权限与邮件可用状态 |
| POST | `/auth/me/mail/preferences` | 更新 `announcements` 布尔偏好 |
| POST | `/auth/mail/challenge` | `email` 与 `purpose=register/reset`，返回 `challenge_id`；reset 对未知邮箱同样返回 accepted |
| POST | `/auth/me/mail/challenge` | `purpose=bind/old_email/password`、`current_password`，bind 另传目标 `email` |
| POST | `/auth/me/mail/confirm` | `email`、`current_password`、`proof`；更换已验证邮箱另需 `old_proof` |
| POST | `/auth/mail/reset` | `email`、`new_password`、`proof`，成功撤销会话 |
| GET / POST | `/household/mail/announcements` | 仅家庭管理员查询投递状态，或提交 `title/body/preview/expected_count` 预览及发送 |

`proof` 为 `{challenge_id, code}`。验证码 10 分钟有效，60 秒可重发，最多尝试 5 次。注册 `/auth/register` 额外要求 `email` 与 `proof`；密码修改 `/auth/me/password` 额外要求 `proof`，成功后所有会话失效。`/auth/me` 新增 `email_verified`。

资料接口不能直接改变邮箱；家庭管理员密码重置接口现返回 403 `verification_required`，须用户自行通过邮箱找回。SMTP 缺失或禁用返回 `mail_unavailable`，额度不足返回 `mail_quota`；队列的 `smtp_accepted` 仅表示 SMTP 接受。
