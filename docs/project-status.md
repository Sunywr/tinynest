# TinyNest 当前进度

2026-11-13 · V0.1 家庭账本开发基线。后续任务从同步后的 main 开始。

| 范围 | 进度 |
| --- | --- |
| Phase 0 | 工程骨架和品牌原型已归档，见 [执行报告](execution-report.md) |
| V0.1 | 复式记账、SQLite 迁移、登录会话、主要账务 API 与 Vue 界面已落地。逐路由状态见 [API 契约](api-v0.1.md) |
| PR #1 | 合入 V0.1 主体及初始化授权、家庭隔离、业务日期、Cookie 配置和限流修复，见 [修复报告](reviews/v0.1-pr-remediation-report.md) |
| 本机复验 | Go 测试、vet、build，npm ci 与前端生产构建（含类型检查），品牌哈希均通过 |
| 发布 | `d1e0a32` 已部署生产，双机 HTTPS 同源访问、MySQL 就绪、初始化关闭和公开首页实测通过。上线前 MySQL 备份任务执行成功 |
| 公开首页 | [登录、动效与 PWA 说明](public-landing.md)。本地 49 条浏览器测试通过，发布分支 [CI 四项通过](https://github.com/Sunywr/tinynest/actions/runs/43583991039) |
| 账务口径 | [十项已确认](reviews/v0.1-accounting-decisions.md)。实施缺口按当前代码逐项核对 |

在「记一笔 → 分类管理」可新增两级收支分类、改名、排序和停用。停用的分类仍可查看、重新启用，并保留历史流水关联。

## 待实施与待验收

- [核心账本计划](plan-v0.1.md)保留，家庭成员档案、项目和账户资料管理仍需补齐。
- [短期发布缺口](plans/next-implementation-plan.md)保留。iPhone Safari 实机 PWA、生产 MySQL 隔离恢复、异地备份与并发性能仍待验收。
- 生产浏览器抽查覆盖访客路径。账务回归使用隔离测试库。

开发入口见 [开发说明](DEVELOPMENT.md)。
