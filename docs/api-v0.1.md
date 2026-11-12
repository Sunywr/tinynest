# TinyNest V0.1 · S0 API 契约草案

> 依据 V0.1 实施计划第 14 节，按现有 Go 工程风格收敛。前缀统一 `/api/v1`。
> **Phase 0 的 `/api/v1/healthz`、`/api/v1/readyz` 保持不变**，新增业务路由独立注册，不改动原有 handler。

## 1. 通用约定

| 项 | 约定 |
|---|---|
| 传输 | JSON；`Content-Type: application/json; charset=utf-8` |
| 金额 | 一律 **整数分**（JSON number，整数）或十进制字符串；**禁止浮点小数**。响应字段名 `*_minor` |
| 数量/价格 | `quantity_scaled` + `quantity_scale`；`price_scaled` + `price_scale` |
| 日期 | `YYYY-MM-DD`（Asia/Shanghai 业务日）；时间戳 RFC3339 UTC |
| 分页 | `?page=1&page_size=20&sort=-occurred_on`；响应 `{items,total,page,page_size}` |
| 幂等 | 所有写操作可带 `Idempotency-Key`；重复返回首次结果（含原 `Location`） |
| 错误 | `{"error":{"code":"...","message":"...","details":{...}}}`；**不泄露 SQL/路径/堆栈** |
| 缓存 | 所有 `/api/v1/*` 响应强制 `Cache-Control: no-store` |
| 认证 | Cookie Session（HttpOnly/Secure/SameSite=Lax）+ 写操作 `X-CSRF-Token`；除 `/auth/session`、`/healthz`、`/readyz` 外均需认证 |

标准错误码：`unauthorized` / `forbidden` / `invalid_amount` / `unbalanced_entry` / `idempotency_conflict` / `refund_exceeds_original` / `split_sum_mismatch` / `schedule_sum_mismatch` / `duplicate_biz_key` / `not_found` / `conflict` / `internal_error`。

## 2. 接口分组

### `/auth`
| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/auth/session` | 登录，建会话；失败限流 |
| DELETE | `/auth/session` | 登出 |
| GET | `/auth/me` | 当前用户 + household_id + csrf_token |

### `/household` · `/categories` · `/projects`（S1）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/household/current` | 家庭信息 |
| GET/POST | `/household/members` | 成员档案（非登录账号） |
| GET | `/categories?kind=expense&tree=true` | 两级分类树 |
| POST | `/categories` | 新增（`parent_id` 可空，`level<=2`） |
| PATCH | `/categories/{id}` | 改名/排序/停用（不删历史） |
| GET/POST | `/projects` | 项目列表 / 新建（记账表单内快速创建复用此接口） |
| GET | `/projects/{id}` | 累计收入/支出/净额/分类构成/明细 |
| PATCH | `/projects/{id}` | 编辑/停用 |

### `/accounts`（S1）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/accounts` | 现金/证券/理财账户；创建可带 `opening_balance_minor` |
| GET/PATCH | `/accounts/{id}` | 详情 / 编辑 |
| GET | `/accounts/{id}/balance` | 由分录重建的余额 + 最近流水 |
| GET | `/ledger/accounts` | 科目表（调试/对账） |

### `/transactions` · `/refunds`（S2 / S4）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/transactions?from&to&type&category_id&project_id&account_id&status` | 分页流水 |
| POST | `/transactions` | 支出/收入/转账；`splits[]` 至少 1 行且合计=总额 |
| GET | `/transactions/{id}` | 详情：业务 + 分录影响 + 退款 + 关联分期/负债/项目 |
| POST | `/transactions/{id}/reverse` | 冲正（保留原分录，生成取反分录） |
| POST | `/refunds` | 全额/部分退款；`transaction_id` + 可选 `split_id` |
| GET | `/refunds/by-transaction/{id}` | 累计退款、净支出、剩余可退 |

`POST /transactions` 请求示例（信用消费 1000.00，拆两行）：
```json
{
  "type": "expense", "amount_minor": 100000, "currency": "CNY",
  "occurred_on": "2026-11-12", "payment_kind": "credit",
  "credit_instrument_id": "ci_01", "member_id": "hm_01", "merchant": "超市",
  "splits": [
    {"category_id":"cat_food","amount_minor":60000,"project_id":null},
    {"category_id":"cat_daily","amount_minor":40000,"project_id":"prj_01"}
  ]
}
```
响应 `201`：`{"id":"...","status":"posted","entry_id":"...","balance_minor":...}`。
分录：借 `X_food` 60000 + `X_daily` 40000 / 贷 `L_ci_01` 100000。

### `/credit-instruments` · `/credit-statements`（S3）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/credit-instruments` | 信用卡/花呗/美团月付/抖音月付/自定义 |
| PATCH | `/credit-instruments/{id}` | 周期起止日、还款日、偏移、额度、停用 |
| GET | `/credit-statements?instrument_id&status` | 账单列表 |
| GET | `/credit-statements/{id}` | 明细/系统应还/已还/剩余/核对差额 |
| POST | `/credit-statements/{id}/reconcile` | 录入平台账单金额（只记核对值，不覆盖系统流水） |
| POST | `/credit-statements/{id}/payments` | 部分/提前/全额还款 |
| POST | `/credit-instruments/{id}/convert-to-installment` | 信用卡消费转分期（负债重分类） |

### `/installments` · `/debts`（S4）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/installments` | 合同；创建时校验期次本金合计=本金、费用合计=总费用 |
| GET | `/installments/{id}` | 合同 + 期次计划 |
| POST | `/installments/{id}/confirm-schedule` | 用户确认计划（最后一期承担舍入差） |
| POST | `/installments/{id}/payments` | 确认实付；`channel=cash\|credit` |
| POST | `/installments/{id}/settle-early` | 提前结清，未来期次取消/调整，保留历史付款 |
| GET/POST | `/debts` | 房贷/贷款/保险/其他合同 |
| POST | `/debts/{id}/schedule` | 录入/生成每期本金/利息/费用 |
| POST | `/debts/{id}/payments` | 确认付款 |
| GET | `/schedule/due-items?from&to&status` | 待支付事项（**只读，不自动入账**） |

### `/investments`（S5）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/investments/assets` | 股票/ETF/理财/自定义 |
| GET/POST | `/investments/fee-rules` | 账户费率模板（版本化） |
| POST | `/investments/fee-preview` | 费用预览（后端计算，前端不重算） |
| POST | `/investments/trades` | 买入/卖出；底层按交易前后持仓识别建仓/加仓/减仓/清仓 |
| GET | `/investments/trades` | 交易流水 |
| GET | `/investments/positions` | 数量/总成本/移动均价/估值/未实现收益 |
| POST | `/investments/dividends` | 分红；`tax_minor: null` = 未填写（**不默认 0**） |
| POST | `/investments/valuations` | 手工录入价格/净值与估值日期 |
| GET | `/investments/return` | 已实现 + 未实现 + 净分红 + 参考收益率 |

### `/dashboard`（S6）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/dashboard/summary?from=&to=` | 收入/支出/结余/现金净流量/应还负债/投资总收益/净资产 |
| GET | `/dashboard/cashflow?from=&to=` | 现金流口径明细 |
| GET | `/dashboard/liabilities?from=&to=` | 负债到期口径；未来 7/30 天待还、逾期 |
| GET | `/dashboard/categories?from=&to=&top_n=` | 分类支出排行（一级汇总二级） |
| GET | `/dashboard/projects?from=&to=` | 项目累计收支 |
| GET | `/dashboard/investment-dividends?year=` | 分红与税额统计 |

**三套口径分别由三个接口提供，禁止合并成一个「本月支出」。**

### `/admin`（S7）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/admin/audit-logs` | 审计日志查询 |
| POST | `/admin/backup` | 触发一致性备份（仅后端服务器） |
| GET | `/admin/migrations` | 迁移版本状态 |

## 3. 与 Phase 0 的兼容边界

- `healthz` 继续只报告进程存活；`readyz` 在 V0.1 升级为**真实** SQLite `PRAGMA quick_check` + 迁移版本校验（会同步更新文档，避免继续声称"仅目录检查"）。
- `apps/web` 仍只请求相对路径 `/api/v1/*`，不得硬编码任何主机、证书或数据库地址。
- Service Worker 继续显式跳过 `/api/`，且对所有 `/api/v1/*` 响应强制 `no-store`。
