# 账务回归

2026-11-15，本地候选基于 f3279d1。Go 全量测试及竞态检测通过，使用隔离库。

| 路径 | 验证 |
| --- | --- |
| 复式账本、信用、退款、分期、投资 | API 端到端用例 TestV01EndToEnd 通过；在独立 MySQL 8.0.46 上也通过 |
| 跨年退款 | TestCrossYearCashRefundKeepsProjectAndCashflowSeparate 覆盖原项目净支出与现金退款日期分离 |
| 固定负债及保险 | Go 回归覆盖房贷信用支付和保险本金约束；SQLite 恢复演练核对两期期次和首期付款 |
| 账户停用 | TestDisabledAccountRetainsBalance 覆盖新支出被拒、历史余额与流水保留、重新启用 |
| 家庭及角色 | Go 回归覆盖双家庭隔离与成员会话角色更新；TestMemberAccountLifecycle 验证只读写入拒绝、恢复写入、停用会话 |

历史账务口径见[决策台账](v0.1-accounting-decisions.md)，恢复证据见[CI 与备份复核](ci-and-backup-verification.md)。

重复提交门槛已验证。原请求成功后，冲正和带同一幂等键的退款各重发 20 次，均被拒绝且只保留一笔记录，现金余额为 9,050 分。分期部分付款重发 20 次，均返回原付款 ID，期次剩余和合同守恒不变。Go 全量测试及上述用例的竞态检测通过。

TestAccountStatusAndMemberRoleStayInHousehold 使用同一个 SQLite 库内的两个家庭验证：跨家庭读取账户和流水、停用账户、修改成员角色均返回 404。账户状态和 1,200 分余额保持不变，越权角色更新未落库。同家庭管理员账号也无法被降级。Go 全量回归通过。
