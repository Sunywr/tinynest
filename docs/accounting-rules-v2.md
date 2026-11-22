# TinyNest V2 · 账务规则与统计口径

> 依据代码：`apps/api/internal/ledger/rules.go`、`internal/ledger/subjects.go`、`internal/ledger/entry.go`、
> `internal/api/transactions.go`、`internal/api/accounts.go`、`internal/api/investments.go`、`internal/api/stats.go`、`internal/db/journal.go`。
> 本文只描述**当前代码真实存在**的规则；退役模块的旧规则（信用账单、分期、投资交易、退款、拆分等）不在 V2 范围内。
> 2026-11-22 起另有 pfm 导入的只读证券/理财估值快照；它只用于资产页/图表的估值展示，不写分录，不改变下述复式账务规则。

## 1. 三条铁律

1. **单一事实来源**：`journal_entries` + `journal_lines` 是唯一账务事实；`transactions` / `accounts` 只表达业务意图与状态，余额与统计一律从分录聚合重建，页面不维护余额。
   **例外**：`investment_accounts` / `investment_holdings` 是 pfm 导入的只读估值快照；资产页“投资市值”和“含投资市值的净资产”是明确标注的估值视图，不是账本余额，不参与复式记账、转账或流水。
2. **单事务提交**：一条流水行、其全部分录行与审计日志必须在同一个数据库事务内写入或同时回滚；不允许只写流水不写分录。
3. **不可变 + 冲正**：已入账分录不 UPDATE、不 DELETE；纠错 = 生成借贷取反的冲正分录（`reversal_of`）+ 新建正确流水；删除流水同样只写冲正。

金额一律整数分（`money.Minor`，`internal/money`），禁止浮点参与计算；API 与数据库都不存「元」。

## 2. 科目编码

科目编码首数字代表类别：`1` 资产 / `2` 负债 / `3` 权益 / `4` 收入 / `5` 费用。具体账户用「编码:业务 ID」寻址，例如 `1010:acc_xxx`。

| 编码 | 常量 | 用途 | 方向 |
| --- | --- | --- | --- |
| `1010` | `SubjCash` | 每个 `cash` 账户一个子科目，`1010:<account_id>` | 借方增加余额 |
| `2010` | `SubjCreditDebt` | 每个 `credit` 账户一个子科目，`2010:<account_id>` | 贷方增加欠款（余额为负表示欠款） |
| `3010` | `SubjOpeningEquity` | 期初余额的唯一对方科目，`3010` 不细分 | 权益 |
| `4010` | `SubjIncomeBase` | 收入分类子科目，`4010:<category_id>` | 贷方增加收入 |
| `5010` | `SubjExpenseBase` | 支出分类子科目，`5010:<category_id>` | 借方增加费用 |

`accounts.ledger_code` 保存该账户的完整科目编码（`1010:<id>` 或 `2010:<id>`），并同步在 `ledger_accounts(code, category)` 建立对应科目；`category` 取值 1（现金资产）或 2（信用卡负债）。分类不生成 `ledger_accounts` 行，分类子科目直接由 `4010:` / `5010:` 前缀拼出。

## 3. 分录规则（全部借贷平衡）

| # | 业务事件 | 借方 | 贷方 | 代码入口 | `biz_type` |
| --- | --- | --- | --- | --- | --- |
| 1 | 现金支出（金额 `v`，分类 `C`） | `5010:C` `v` | `1010:<账户>` `v` | `ledger.CashExpense` | `expense` |
| 2 | 信用卡支出（金额 `v`，分类 `C`） | `5010:C` `v` | `2010:<账户>` `v` | `ledger.CreditExpense` | `expense` |
| 3 | 收入（金额 `v`，分类 `C`，现金账户） | `1010:<账户>` `v` | `4010:C` `v` | `ledger.Income` | `income` |
| 4 | 现金账户互转 | `1010:<转入>` `v` | `1010:<转出>` `v` | `ledger.Transfer` | `transfer` |
| 5 | 现金账户 → 信用卡（还款） | `2010:<信用卡>` `v` | `1010:<现金>` `v` | `ledger.RepayCredit` | `credit_repayment` |
| 6 | 现金账户期初余额 | `1010:<账户>` `v` | `3010` `v` | `ledger.OpeningCash` | `opening_cash` |
| 7 | 信用卡期初欠款 | `3010` `v` | `2010:<账户>` `v` | `ledger.OpeningBalance` | `opening_liability` |
| 8 | 冲正（任意已入账分录） | 原分录各行借贷互换 | 原分录各行借贷互换 | `ledger.Entry.Reverse` | `reversal` |

约束：

- **收入必须记入现金账户**：`POST /transactions` 对 `type=income` 且账户 `kind=credit` 返回 400「收入必须记入现金账户」。
- **支出**按所选账户类型自动走现金或信用卡分录；信用卡支出的现金余额不变、费用当期确认。
- **转账**：转出与转入账户不能相同；`credit` 账户不能作为转出账户（错误提示「信用卡账户不能作为转出账户；还款请选择现金账户转入信用卡」）；`cash → credit` 按还款处理，`credit → cash` 被拒绝。
- **还款不是支出**：现金 → 信用卡只做负债结算（借 `2010` / 贷 `1010`），不产生 `5010` 行，因此不计入支出统计。
- **期初余额不进收入/支出**：对方科目固定 `3010` 期初权益；信用卡期初欠款同样走权益，不确认费用。
- 每笔流水只有一个 `category_id`（单层分类）；V2 API 不再接受拆分行，内核的 `ledger.Split` 类型只作为单分类调用路径使用。

## 4. API 事件映射

| API | 请求要点 | 分录 | 备注 |
| --- | --- | --- | --- |
| `POST /transactions`（`type=expense`） | `category_id`、`account_id`、`amount_minor`、`occurred_on` | 规则 1 或 2 | 分类 `kind` 必须为 `expense` |
| `POST /transactions`（`type=income`） | 同上 | 规则 3 | 分类 `kind` 必须为 `income`；账户必须为 `cash` |
| `POST /transactions`（`type=transfer`） | `account_id`、`to_account_id` | 规则 4 或 5 | `category_id` 忽略 |
| `POST /accounts/transfers` | `from_account_id`、`to_account_id` | 规则 4 或 5 | 备注缺省「账户转账」 |
| `POST /accounts`（带 `opening_minor`） | `kind`、`opening_minor` | 规则 6 或 7 | 同时写 `account_opening_balances` |
| `PATCH /transactions/{id}` | 任意可改字段 | 冲正原分录 + 规则 1–5 重新入账 | 同一事务 |
| `DELETE /transactions/{id}` | 无 | 规则 8 | 只冲正，不物理删除 |

冲正分录的 `biz_id` 沿用原业务 ID、`biz_type=reversal`、行 memo 为「撤销原记录」；`occurred_on` 沿用原日期。

## 5. 更正、删除与冲正

- **更正**（`PATCH /transactions/{id}`）：同一事务内先构建替换分录（构建失败不动原流水），再冲正原分录、写替换分录与新流水行；`type` 不允许修改（返回 400「更正不支持修改流水类型，请冲正后重新记账」）。响应返回新流水 ID 与 `replaces` 原 ID。
- **删除**（`DELETE /transactions/{id}`）：只写冲正分录并把原流水/原分录状态置 `reversed`，不物理删除；重复删除返回 409。
- **已冲正不可再操作**：服务端在事务内以 `transactions.status='posted'` 作为条件更新，影响行数不为 1 即失败回滚。
- **冲正期间一致**：冲正沿用原 `occurred_on`，因此同月统计净额归零；不会把抵消额记到更正发生的月份。
- 账户与分类的「删除」是软删除（`accounts.status='deleted'`、`categories.active=-1`），历史分录与研究余额保留。

## 6. 统计口径

所有统计只从 `journal_lines` 派生，并在 SQL 内按 `journal_entries.household_id` 限定家庭；不读取 `transactions.amount_minor` 累加，也不先全库聚合再过滤。

### 6.1 收入与支出

| 口径 | 科目匹配 | 金额公式 | 代码 |
| --- | --- | --- | --- |
| 收入 | `account_code LIKE '4010:%'` | `SUM(credit_minor - debit_minor)` | `internal/api/stats.go` 的 `between()` |
| 支出 | `account_code LIKE '5010:%'` | `SUM(debit_minor - credit_minor)` | 同上 |
| 结余 | 不单独存 | `income - expense` | `handleStatsSummary` |

转账（`1010`/`2010`）、信用卡还款（`2010` 借方 / `1010` 贷方）、期初余额（`3010`）都不匹配上述前缀，因此**不计入收支**。

### 6.2 分类统计

- 分类 ID 从科目后缀解析：`SUBSTR(account_code, 6)`（`"5010:"` 共 5 个字符，从第 6 位起是 `category_id`），SQLite 与 MySQL 语法一致。
- `kind=expense` 按支出公式聚合，`kind=income` 按收入公式聚合；`LEFT JOIN categories` 取名称与 emoji（分类已删除时名称可能为空）。
- `HAVING SUM(...) <> 0`：被完整冲正的分类净额为零，不出现在排行里。
- `share_bp = amount_minor * 10000 / total`（整数向下取整；`total<=0` 时为 0）。

### 6.3 日汇总与月汇总

- 日汇总 `GET /stats/daily?month=YYYY-MM`：区间 `[当月 1 日, 次月 1 日)`，按 `occurred_on` 分组，只返回支出或收入净额非零的日期。
- 月汇总 `GET /stats/monthly`：`months` 默认 6（范围 1–120），以 `to` 月为终点向前回溯，按 `SUBSTR(occurred_on,1,7)` 分组；无数据的月份补 0 返回。

### 6.4 预算

- `PUT /budgets/{month}` 写入 `budgets.amount_minor`（按 `(household_id, month)` 覆盖）。
- 已用金额 = 该月支出科目净额（与 6.1 支出同公式、同家庭范围），由 `monthExpense()` 实时计算，不缓存。
- 未设置预算时 `GET /budgets/{month}` 返回 `amount_minor: null` 与实时 `spent_minor`；`null` 不等于 0。

### 6.5 信用卡还款现金计划（派生统计，不写分录）

`GET /accounts?include=credit_debt` 返回只读计划；**不新增账单表、不写 `journal_entries` / `journal_lines`、不改余额**。配置只存在于 `accounts.statement_day` / `accounts.due_day`（均可空，1–31）。

1. **账单周期**：账单截止日 = 事件日期当天或之后第一个 `statement_day`（按当月实际天数钳制）；周期开始日 = 上一月账单截止日的次日。例：账单日 5，2026-11-10 的消费归入 `2026-11-06 ~ 2026-12-05`。
2. **还款日**：严格晚于账单截止日的第一个 `due_day`。例：账单日 5/还款日 23 → 11-05 截止、11-23 到期；账单日 25/还款日 13 → 12-25 截止、次年 01-13 到期。28/29/30/31 与闰年全部钳制到月末。
3. **有效债务事件**：只取截至今天、`status='posted'` 且 `reversal_of` 为空的 `journal_entries`；消费须对应当前 `status='posted'` 的 `type=expense` 流水，还款须对应 `status='posted'` 的 `type=transfer` 且 `to_account_id` 为该卡；另加 `biz_type='opening_liability'` 的期初欠款。已冲正/已删除/已更正的旧记录不计入，其原分录与冲正分录仍完整保留在总账中。
4. **还款分配**：按到期日从早到晚 FIFO 冲抵；还款多于欠款时为溢缴款（`credit_balance_minor`），欠款显示 0。
5. **期初欠款**：视为在账户创建时已经出账，归入创建日之前最近的账单截止日，避免把已有欠款错误推到下一期。
6. **聚合**：`total_debt_minor` 为全部未还（含未出账、逾期）；`unbilled_minor` 为账单截止日 > 今天的未还；`overdue_minor` 为到期日 < 今天的未还；`next_due_on` 只取今天及以后最早未还到期日。
7. **发薪日**：取当前登录用户的 `users.payday`；下次发薪日 = 严格晚于今天的第一个发薪日（今天发薪则算下月），31 日按月末钳制。`cash_needed_minor` = 到期日 ≤ 下次发薪日的未还合计（含逾期）；未设置发薪日或没有任何已配置卡时为 `null`。
8. **可用现金**：启用的 `cash` 账户在 `occurred_on <= 今天` 的余额合计；不扣除发薪日前其他开销，信用卡溢缴款不计入。
9. **完成度**：`complete` 仅在发薪日已设置且所有未删除信用卡都配置时才是 true；否则界面显示「已知需准备 / 未配置卡未计入」，不得宣称现金足够。
10. **无快照**：不保存历史账单；修改账单日/还款日会按当前配置重算未还金额。未来日期的流水不计入「截至今天」的计划。

## 7. 幂等

- 仅 `POST /transactions` 与 `POST /accounts/transfers` 读取 `Idempotency-Key` 请求头。
- 键写入 `transactions.idempotency_key` 与 `journal_entries.idempotency_key`（两表均有唯一约束）。
- 同一家庭内重复提交同一键：返回 200 `{id,status:"posted",replayed:true}`，不产生重复分录；事务失败后会重新查询是否已被并发请求提交。
- 其它写操作（更正、删除、分类/账户/预算、成员与管理员操作）没有 API 级幂等键；重试前应先读取当前状态。
- 幂等只防重复提交，不替代「已冲正不可再操作」等业务状态校验。

## 8. 余额重建、并发与家庭隔离

- 账户余额：`SELECT SUM(l.debit_minor - l.credit_minor) FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id WHERE l.account_code=? AND e.household_id=?`（`internal/db/journal.go` 的 `Balance`）。
- `cash` 账户正常余额为正；`credit` 账户余额为负表示欠款。净资产 = 现金账户余额合计 + 信用卡账户余额合计（后者为负）。
- SQLite 单连接单写（WAL、`foreign_keys=ON`、`busy_timeout=5000`），MySQL 连接池上限 1；写操作全部走事务。
- `journal_lines` 无家庭列，任何查询都必须经 `journal_entries` 关联后限定 `household_id`；服务层不接受「先查全部再在 Go 里过滤」。

## 9. 内核中保留但未暴露的能力

`internal/ledger/rules.go` 中仍保留退款分录构造器（`RefundToCash`、`RefundToCredit`、`ValidateRefund`、`BizRefund`）与 `ledger.Split` 类型，`rules_test.go` 覆盖了它们；但 V2 **没有任何 HTTP 路由**调用退款构造器，数据库也没有 `refunds` / `transaction_splits` 表。它们不属于 V2 产品能力，不得在文档或界面中描述为「已支持退款」。

## 10. 与 V0.1 规则的差异

| 项 | V0.1（历史） | V2（当前） |
| --- | --- | --- |
| 账户类型 | 现金/证券/理财/信用卡/信用工具等 | 记账账户仅 `cash` / `credit`；另有只读证券/理财估值快照（非记账账户） |
| 流水拆分 | `transaction_splits`，多分类分摊 | 单分类，无拆分行 |
| 退款 | `/refunds` 路由与 `refunds` 表 | 退役；内核构造器未暴露 |
| 信用账单/分期/负债/投资交易 | 有独立表与规则 | 整套退役；仅保留 pfm 只读估值快照 |
| 信用卡账单日/还款日 | 独立 `credit_instruments` + 账单表 | 仅 `accounts` 两个可空配置列 + 只读派生计划，无账单表/分期/利息 |
| 分类 | 两级，可设 `parent_id` | 单层，无 `parent_id` |
| 统计科目 | 含利息、手续费、保险、分红、投资损益等 | 仅 `4010:%` 收入、`5010:%` 支出；还款计划与投资收益单独只读派生 |
| 迁移 | 多版本增量 | 单一迁移器 + `baseline=v2` 基线重置（当前 001–005） |

## 11. 相关代码与文档

- 分录构造与冲正：`apps/api/internal/ledger/rules.go`、`entry.go`、`subjects.go`。
- 流水写入/更正/删除：`apps/api/internal/api/transactions.go`；账户与期初：`internal/api/accounts.go`。
- 统计与预算：`apps/api/internal/api/stats.go`。
- 信用卡还款计划：`apps/api/internal/api/credit_debt.go`（只读派生）；账户配置列在 `internal/api/accounts.go`。
- 证券/理财估值快照：`apps/api/internal/api/investments.go`（只读投影）；表在 `005_investments.sql`。
- 余额重建：`apps/api/internal/db/journal.go`。
- 数据模型：[er-v2.md](er-v2.md)；接口契约：[api-v2.md](api-v2.md)；迁移与基线：[DEVELOPMENT.md](DEVELOPMENT.md) §5。
