# TinyNest 2026-11-14 WorkBuddy 任务包

本包供 TinyNest 项目下一轮 WorkBuddy Hy4 使用，**不包含源码，也不声称已验收最新 GitHub 提交**。

| 文件 | 用途 |
|---|---|
| `TINYNEST-ACCEPTANCE-AND-NEXT-PLAN.md` | 最新分支代码验收门禁、V0.1.1 收尾或 V0.1.2 开发选择树 |
| `WORKBUDDY-HY4-LATEST-AUDIT-AND-IMPLEMENT.md` | 直接供 WorkBuddy 执行的完整提示词 |
| `MANUAL-ACCEPTANCE-CHECKLIST.md` | PC/iPhone、信用/负债/投资、工程安全清单 |

用法：将三个文件放到项目 `docs/plans/2026-11-14/`，在 WorkBuddy 中输入：

> 使用 Hy4 模型及「高级开发工程师」吴八哥专家技能，完整读取 `docs/plans/2026-11-14/WORKBUDDY-HY4-LATEST-AUDIT-AND-IMPLEMENT.md` 和同目录计划。先从最新 `main`/feature 分支真实代码、最新 CI、API/DB 和浏览器 E2E 完成验收，再按计划增量实施尚未完成的 V0.1.1 或 V0.1.2 功能。只在独立本地分支和隔离 SQLite 测试库工作，不推送/合并/部署生产。提交实际测试证据与最终报告。

用户可在确认验收后，另行授权 Agent 创建 PR、推送或部署隔离预发布环境。
