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
| GET | `/auth/me` | IMPLEMENTED | 当前用户、角色、household_id 与 csrf_token；已绑定且启用的成员档案返回 member_id 和 member_name，否则为空 |
| POST | `/auth/register` · `/auth/join` | IMPLEMENTED | 注册登录名、凭邀请码申请加入家庭。申请由家庭管理员审批 |

### `/household` · `/categories` · `/projects`（S1）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/categories` | IMPLEMENTED | 分类列表 / 新增（两级，level<=2） |
| PATCH | `/categories/{id}` | IMPLEMENTED | 传入任一字段：`name`（非空）、`sort`（整数）、`active`（布尔值）；仅更新本家庭分类。停用后不可用于新流水，历史关联保留 |
| GET/POST | `/projects` | IMPLEMENTED | 项目列表 / 新建 |
| GET | `/projects/{id}/summary` | IMPLEMENTED | 累计收支、退款、净支出、分类构成与最近 100 条拆分流水 |
| GET | `/projects/{id}` · PATCH `/projects/{id}` | IMPLEMENTED | 项目详情与编辑。支持名称、起止日期、说明和启用或归档状态；日期或说明传空字符串可清除 |
| GET/POST | `/household/members` | IMPLEMENTED | 本家庭成员列表与新增；`name` 为 1 至 100 个字符，成员档案不创建登录账号 |
| POST | `/household/members/current` | IMPLEMENTED | 返回当前可记账账号绑定的成员档案。尚未绑定的旧账号按用户名创建并绑定自己的档案。重复调用返回同一档案 |
| GET | `/household/current` | IMPLEMENTED | 当前会话所属家庭的名称、币种和业务时区 |
| POST | `/household/invitations` | IMPLEMENTED | 为现有或新建成员档案签发邀请 |

### `/accounts`（S1）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/accounts` | IMPLEMENTED | 现金/证券/理财账户；创建可带 `opening_balance_minor` |
| GET | `/accounts/{id}/balance` | IMPLEMENTED | 由分录重建余额 |
| GET/PATCH | `/accounts/{id}` | IMPLEMENTED | 账户详情与资料编辑（名称、子类型、归属成员、备注、启用状态），余额由分录计算 |
| GET | `/ledger/accounts` | PLANNED | 完整科目表。现有账户、账户分录和净资产接口支持已公开的账户与科目下钻 |
| GET | `/accounts/{id}/journal` | IMPLEMENTED | 本家庭账户最近 100 笔分录净变动，含期初余额和冲正 |

### `/transactions` · `/refunds`（S2 / S4）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/transactions` | IMPLEMENTED | 流水筛选与游标分页 / 支出·收入·转账（`splits[]` 合计=总额） |
| GET | `/transactions/{id}` | IMPLEMENTED | 详情（业务 + 分录影响） |
| POST | `/transactions/{id}/reverse` | IMPLEMENTED | 冲正（保留原分录，业务日按 Asia/Shanghai） |
| POST | `/refunds` | IMPLEMENTED | 全额/部分退款、超额拒绝 |
| GET | `/refunds/by-transaction/{id}` | IMPLEMENTED | 累计退款、净支出、剩余可退 |

`GET /transactions` 接受 `from`、`to`（含边界，YYYY-MM-DD）、`type`（expense、income、transfer）及 `limit`（1–200，默认 200）。响应含 `items`、本页条数 `total` 和 `next_cursor`。下一页保持原筛选条件，将 `next_cursor` 传为 `cursor`；值为 `null` 时已到末页。流水按业务日期、创建时间与 ID 降序排列。

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
| GET/POST | `/credit-instruments` | IMPLEMENTED | 信用卡/花呗/自定义工具 |
| GET | `/credit-statements` | IMPLEMENTED | 账单列表（自动归集） |
| POST | `/credit-statements/{id}/payments` | IMPLEMENTED | 部分/全额还款 |
| POST | `/credit-statements/{id}/reconcile` | IMPLEMENTED | 录入平台账单金额（只记核对值） |
| PATCH | `/credit-instruments/{id}` | IMPLEMENTED | 周期、额度和启停，记录变更审计 |
| GET | `/credit-instruments/{id}/limit` | IMPLEMENTED | 实时负债、已用额度、可用额度与使用率（基点）；未配置额度时后两项为 null |
| GET | `/credit-statements/{id}` | IMPLEMENTED | 账单明细与核对差额 |
| PATCH | `/credit-statements/{id}/cycle` | IMPLEMENTED | 人工修正单期账单日期，须填写原因 |

账单列表和详情根据上海业务日与应还、实还金额派生状态。信用转分期见下节。

### `/installments` · `/debts` · `/schedule`（S4）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/installments` | IMPLEMENTED | 合同；校验期次本金/费用合计 |
| POST | `/installments/{id}/payments` | IMPLEMENTED | 确认实付（`channel=cash\|credit`） |
| POST | `/installments/convert-from-credit` | IMPLEMENTED（V0.1.1） | 指定 `credit_instrument_id` 与 `statement_id`，同一事务重分类本金、冲减原账单及已有待办。仅当该工具恰有一张未还账单时可省略 `statement_id` |
| GET | `/schedule/due-items` | IMPLEMENTED | 待支付事项（只读，业务日时钟） |
| POST | `/schedule/generate` | IMPLEMENTED | 手动幂等生成未来 30 天待办 |
| GET | `/schedule/runs` | IMPLEMENTED | 当前家庭最近 50 次投影记录，含成功及可持久化的失败 |
| GET | `/installments/{id}` | IMPLEMENTED | 合同与期次计划 |
| POST | `/installments/{id}/refunds` · `/installments/{id}/adjust-schedule` · `/installments/{id}/settle-early` | IMPLEMENTED | 退款、显式调整未付款期次与提前结清 |
| POST | `/installments/{id}/confirm-schedule` | PLANNED | 创建合同时已写入校验后的计划。后续调整通过 `adjust-schedule` 显式提交 |
| GET/POST | `/debts` | IMPLEMENTED | 房贷、贷款、保险等合同列表和创建 |
| GET/PATCH | `/debts/{id}` | IMPLEMENTED | 合同与期次详情、合同资料编辑 |
| POST | `/debts/{id}/schedule` · `/debts/{id}/payments` | IMPLEMENTED | 录入计划和按期确认付款 |
| POST | `/debts/{id}/payments/{paymentId}/reverse` · `/debts/{id}/settle-early` | IMPLEMENTED | 付款冲正、提前结清 |

### `/investments`（S5）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/investments/assets` | IMPLEMENTED | 股票/ETF/理财/自定义资产 |
| GET/POST | `/investments/trades` | IMPLEMENTED | 买入/卖出（移动加权平均成本） |
| GET | `/investments/positions` | IMPLEMENTED | 数量/总成本/移动均价/已实现·未实现收益/净分红/参考收益率；口径第 9 项已确认为「总收益 ÷ 累计买入成本」，**不得标为年化** |
| POST | `/investments/dividends` | IMPLEMENTED | `tax_minor: null` = 未填写（不默认 0） |
| GET | `/investments/dividends` | IMPLEMENTED（V0.1.3 预检） | 分红历史与年度汇总；支持 `?asset_id=` / `?from=` / `?to=`；返回逐笔 `tax_unknown`、`entry_id` 与 `by_year`；SQL 层 `household_id` 隔离，未知税额不按 0 处理 |
| POST | `/investments/valuations` | IMPLEMENTED | 手工录入估值，估值日期显式展示 |
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
| GET / POST | `/admin/migrations` · `/admin/backup` | PLANNED | 迁移就绪由 `/readyz` 验证，具体版本和备份从部署侧数据库及备份任务核对 |

## 3. 与 Phase 0 的兼容边界

- `healthz` 报告进程存活。`readyz` 检查数据库连通性或完整性以及迁移版本。
- `apps/web` 仍只请求相对路径 `/api/v1/*`，不得硬编码主机、证书或数据库地址。
- Service Worker 继续显式跳过 `/api/`，且对所有 `/api/v1/*` 响应强制 `no-store`。
- Phase 0 的「无登录、纯虚构 Demo」描述仅适用于 `previews/*` 静态存档，不适用于当前 `apps/web`。

> 未完成模块与正式发布门禁见 `IMPLEMENTATION_REPORT.md` 第 8/9 节、`docs/reviews/v0.1-release-gates.md`；10 项账务口径状态见 `docs/reviews/v0.1-accounting-decisions.md`（**2026-11-14 用户已确认 1–10 全部推荐值，十项 `CONFIRMED`**；旧报告中的「0/10 待确认」为历史记录）。
> V0.1.2 追加端点的实施与测试证据见 `docs/reviews/latest-feature-evidence-matrix.md`、`docs/reviews/next-version-test-report.md`。

### 家庭账号角色

管理员可通过 `PATCH /admin/users/{id}/role` 将本家庭的启用成员设为 `member` 或 `viewer`。角色在每次请求时读取，已有会话随下一次请求采用新权限。只读成员可查看账本并退出登录，账本写入与邀请码写操作返回 403。管理员角色不通过此接口修改。
