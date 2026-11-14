# TinyNest V0.1 · API 契约（含实现状态）

> 依据 V0.1 实施计划第 14 节与**当前实际注册路由**。前缀统一 `/api/v1`。
> **状态定义**：`IMPLEMENTED` = 已注册且有自动化回归；`PARTIAL` = 已注册但能力/口径未完整；`PLANNED` = 仅设计、未注册路由。
> 本文件不是「承诺清单」：凡标 `PLANNED` 的端点当前调用会返回 404/405，请勿据此开发前端。状态依据 `apps/api/internal/api/*.go` 的 `Register` 与 `IMPLEMENTATION_REPORT.md` 第 8 节。

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
| `GET /readyz` | IMPLEMENTED | 无 | SQLite `PRAGMA quick_check` + 迁移版本校验 |
| `POST /auth/session` | IMPLEMENTED | 无（口令校验 + 失败限流） | 成功后签发会话 Cookie |
| `POST /setup` | IMPLEMENTED | **一次性 `X-Setup-Token`** | 仅空库可调用一次，创建首个家庭与唯一 admin |

- Bootstrap Token 由部署者在后端主机通过 `TINYNEST_SETUP_TOKEN` 主动设置；**未配置时端点返回 403 `setup_disabled`（fail closed）**，绝不隐式开放。
- 令牌比较在固定长度 SHA-256 摘要上做常量时间比较；不写入日志、不进入仓库、不进入浏览器打包代码。
- 已初始化后（含并发请求）一律返回 409 `conflict`，令牌不可再用于新增 admin。
- **不能**用 `RemoteAddr == 127.0.0.1` 代替令牌：Nginx 反代会让公网请求在 Go 侧显示为本机。可在 Nginx 额外限制初始化入口，但不可替代令牌。
- 初始化流程：在 B 机生成随机令牌 → 注入服务环境 → 启动 API → 管理员在初始化页面填入令牌提交一次 → 确认 `201` → 令牌即失效。

### 1.2 生产 Cookie 与可信反代（FIX-04 后）

`r.TLS` 在 Go 侧恒为 nil（Nginx 终止 TLS 后明文转发），因此**不再用于判定安全属性**。`Secure` 由 `TINYNEST_COOKIE_SECURE=true|false` 显式决定；未设置或取值非法时服务拒绝启动。若采用 `X-Forwarded-Proto` 辅助判定，只有 `TINYNEST_TRUSTED_PROXIES` 中配置的来源才被采信，任意客户端伪造的请求头无效。

## 2. 接口分组（逐路由状态）

### `/auth` · `/setup`

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| POST | `/setup` | IMPLEMENTED | 需 `X-Setup-Token`，仅一次 |
| POST | `/auth/session` | IMPLEMENTED | 登录、建会话；含失败限流 |
| DELETE | `/auth/session` | IMPLEMENTED | 登出、撤销会话 |
| GET | `/auth/me` | IMPLEMENTED | 当前用户 + household_id + csrf_token |

### `/household` · `/categories` · `/projects`（S1）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/categories` | IMPLEMENTED | 分类列表 / 新增（两级，level<=2） |
| PATCH | `/categories/{id}` | PLANNED | 改名/排序/停用 |
| GET/POST | `/projects` | IMPLEMENTED | 项目列表 / 新建 |
| GET | `/projects/{id}/summary` | IMPLEMENTED | 累计收支、退款、净支出 |
| GET | `/projects/{id}` · PATCH `/projects/{id}` | PLANNED | 详情 / 编辑 |
| GET/POST | `/household/current` · `/household/members` | PLANNED | 家庭信息与成员档案（非登录账号） |

### `/accounts`（S1）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/accounts` | IMPLEMENTED | 现金/证券/理财账户；创建可带 `opening_balance_minor` |
| GET | `/accounts/{id}/balance` | IMPLEMENTED | 由分录重建余额 |
| GET/PATCH | `/accounts/{id}` | PLANNED | 详情 / 编辑 |
| GET | `/ledger/accounts` | PLANNED | 科目表（调试/对账） |

### `/transactions` · `/refunds`（S2 / S4）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/transactions` | IMPLEMENTED | 流水 / 支出·收入·转账（`splits[]` 合计=总额） |
| GET | `/transactions/{id}` | IMPLEMENTED | 详情（业务 + 分录影响） |
| POST | `/transactions/{id}/reverse` | IMPLEMENTED | 冲正（保留原分录，业务日按 Asia/Shanghai） |
| POST | `/refunds` | IMPLEMENTED | 全额/部分退款、超额拒绝 |
| GET | `/refunds/by-transaction/{id}` | IMPLEMENTED | 累计退款、净支出、剩余可退 |

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
| PATCH | `/credit-instruments/{id}` | PLANNED | 周期/额度/停用 |
| GET | `/credit-statements/{id}` | PLANNED | 账单明细与核对差额 |
| POST | `/credit-instruments/{id}/convert-to-installment` | PLANNED | 内核函数已实现并测试，**HTTP 端点未接线** |
| — | 账单状态自动流转 / 额度已用·可用·使用率 | PLANNED | S3 未完成项 |

### `/installments` · `/debts` · `/schedule`（S4）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/installments` | IMPLEMENTED | 合同；校验期次本金/费用合计 |
| POST | `/installments/{id}/payments` | IMPLEMENTED | 确认实付（`channel=cash\|credit`） |
| POST | `/installments/convert-from-credit` | IMPLEMENTED（V0.1.1） | 指定 `credit_instrument_id` 与 `statement_id`，同一事务重分类本金、冲减原账单及已有待办。仅当该工具恰有一张未还账单时可省略 `statement_id` |
| GET | `/schedule/due-items` | IMPLEMENTED | 待支付事项（只读，业务日时钟） |
| POST | `/schedule/generate` | IMPLEMENTED（PARTIAL 语义） | **手动**幂等投影未来 30 天期次；每日自动调度器仍为 PLANNED |
| GET | `/installments/{id}` | PLANNED | 合同 + 期次计划 |
| POST | `/installments/{id}/confirm-schedule` · `/settle-early` | PLANNED | 计划确认 / 提前结清 |
| GET/POST | `/debts` · `/debts/{id}/schedule` · `/debts/{id}/payments` | PLANNED | 房贷/贷款/保险 HTTP CRUD 与期次付款 |

### `/investments`（S5）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET/POST | `/investments/assets` | IMPLEMENTED | 股票/ETF/理财/自定义资产 |
| GET/POST | `/investments/trades` | IMPLEMENTED | 买入/卖出（移动加权平均成本） |
| GET | `/investments/positions` | IMPLEMENTED | 数量/总成本/移动均价/已实现·未实现收益/净分红/参考收益率；口径第 9 项已确认为「总收益 ÷ 累计买入成本」，**不得标为年化** |
| POST | `/investments/dividends` | IMPLEMENTED | `tax_minor: null` = 未填写（不默认 0） |
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
| GET | `/dashboard/net-worth` | IMPLEMENTED（V0.1.2） | 资产构成 / 负债结构 / 投资成本·市值·估值日期 / 净资产，逐项带 `ledger_code` 可下钻；与 `summary` 使用同一统计窗口，`net_worth_minor` 一致 |
| GET | `/dashboard/cashflow` · `/liabilities` · `/investment-dividends` | PLANNED | 独立口径端点 |

**三套口径（收支 / 现金流 / 负债到期）不得合并成一个「本月支出」**；当前三套数值均由 `/dashboard/summary` 分别给出。

### `/admin`（S7）

| 方法 | 路径 | 状态 | 说明 |
|---|---|---|---|
| GET | `/admin/audit-logs` | IMPLEMENTED | 当前家庭最近审计事件，`limit` 默认 50，范围 1–100，响应排除事件原始载荷 |
| GET | `/admin/migrations` · POST `/admin/backup` | PLANNED | 迁移状态与手动触发备份 |

## 3. 与 Phase 0 的兼容边界

- `healthz` 仍只报告进程存活；`readyz` 在 V0.1 已是真实 SQLite `quick_check` + 迁移版本校验。
- `apps/web` 仍只请求相对路径 `/api/v1/*`，不得硬编码主机、证书或数据库地址。
- Service Worker 继续显式跳过 `/api/`，且对所有 `/api/v1/*` 响应强制 `no-store`。
- Phase 0 的「无登录、纯虚构 Demo」描述仅适用于 `previews/*` 静态存档，不适用于当前 `apps/web`。

> 未完成模块与正式发布门禁见 `IMPLEMENTATION_REPORT.md` 第 8/9 节、`docs/reviews/v0.1-release-gates.md`；10 项账务口径状态见 `docs/reviews/v0.1-accounting-decisions.md`（**2026-11-14 用户已确认 1–10 全部推荐值，十项 `CONFIRMED`**；旧报告中的「0/10 待确认」为历史记录）。
> V0.1.2 追加端点的实施与测试证据见 `docs/reviews/latest-feature-evidence-matrix.md`、`docs/reviews/next-version-test-report.md`。
