# TinyNest 实施报告（指针）

> **本文件不再维护 V1 内容。** 完整 V0.1 实施报告已归档到
> [docs/reviews/v1-implementation-report.md](docs/reviews/v1-implementation-report.md)（历史记录，不代表当前代码状态）。
>
> 当前实现状态与 V2 交付请以以下文档为准：
>
> - [当前进度](docs/project-status.md)：V2 已交付范围、实测数字、已退役模块与 `UNVERIFIED` 清单。
> - [V2 API 契约](docs/api-v2.md)、[V2 ER](docs/er-v2.md)、[V2 账务规则](docs/accounting-rules-v2.md)。
> - [V2 基线证据](docs/reviews/v2-baseline.md)：V1 基线、迁移/路由/表数清点与 P5 结果。
> - [开发与本地验证](docs/DEVELOPMENT.md)、[架构](docs/architecture.md)、[安全](docs/security.md)、[部署](docs/deployment.md)、[验收](docs/acceptance.md)。
> - V2 计划与生产手册：[计划](docs/plans/v2-bookkeeping-reset.md)、[模块规格](docs/plans/v2-bookkeeping-module-spec.md)、[三端优化](docs/plans/v2-platform-optimization.md)、[生产清理](docs/plans/v2-prod-purge-runbook.md)、[新库切换](docs/plans/v2-cutover-runbook.md)。
>
> V1 的信用账单、分期、负债、到期调度、投资、股票、账单导入、信用账单邮件、项目、仪表盘与退款模块已删除；
> V2 不支持 V1 旧库原地升级，生产清理与切换由用户按手册执行。
