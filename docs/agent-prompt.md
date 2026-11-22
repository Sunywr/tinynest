# 直接复制给任何 AI Coding Agent 的指令（V2 当前版本）

> **历史提示**：本文件早期版本是「只做 Phase 0、严禁记账业务与登录」的启动提示词，该阶段已完成并归档；V0.1 的完整提示词已被 V2 取代。
> Phase 0 交付见 [执行报告](execution-report.md)，V0.1 实施见 [V0.1 历史报告](reviews/v1-implementation-report.md)。

```text
你是一名资深全栈工程师。请在现有 TinyNest 仓库（中文「小家」/ 英文 TinyNest）上做增量维护，不要另建目录，不要重建项目。

先按 AGENTS.md 核对仓库根目录与 Git 状态，并运行 git status --short；从同步后的 main 或已获授权的任务分支开始，不要覆盖已有改动。阅读 docs/project-status.md 确认范围。

首先完整阅读：
- AGENTS.md（V2 守则，最高优先级）
- docs/plans/v2-bookkeeping-reset.md（P0–P5 总计划）、docs/plans/v2-bookkeeping-module-spec.md（模块与 API 契约冻结稿）、docs/plans/v2-platform-optimization.md
- docs/accounting-rules-v2.md（分录规则与统计口径）、docs/api-v2.md（55 条业务路由契约）、docs/er-v2.md（27 张业务表与冲正不变量）
- docs/architecture.md、docs/DEVELOPMENT.md、docs/security.md、docs/deployment.md、docs/acceptance.md
- docs/plans/v2-prod-purge-runbook.md 与 docs/plans/v2-cutover-runbook.md（生产清理与切换，用户执行）
- docs/reviews/v2-baseline.md（V2 基线与 P5 结果）

当前状态：V2 已完成代码与文档，保留身份/权限/家庭/邮件、复式记账内核（分类、账户、流水、统计、预算、前端五界面）、信用卡还款计划与 pfm 导入的只读证券/理财估值快照；信用账单、分期、负债、V1 投资交易、导入、项目、仪表盘、退款、家庭公告已退役。生产库清理与新库切换尚未执行，依赖生产、真机与外部网络的事项一律标 UNVERIFIED。

必须遵守：
1. 账务唯一事实来源是 journal_entries + journal_lines；不得直接改余额；已入账分录不 UPDATE/DELETE，纠错走冲正分录；删除流水也是冲正。
2. 所有财务查询必须在 SQL 层带 household_id（journal_lines 经 journal_entries 限定），不得只在 Go 层过滤。
3. 业务日期固定 Asia/Shanghai，统一走可注入的 API.businessToday() / businessDate()；审计与会话时间戳仍按 UTC 存储。
4. 安全配置必须显式：TINYNEST_SETUP_TOKEN、TINYNEST_SETUP_DISABLED、TINYNEST_COOKIE_SECURE、TINYNEST_TRUSTED_PROXIES。不得依赖 r.TLS != nil、RemoteAddr == 127.0.0.1 或客户端可伪造的 X-Forwarded-*。
5. V2 是基线重置：不得让 V2 二进制打开 V1 旧库，不得绕过 schema_meta.baseline=v2 守卫，不得把旧库原地升级写进代码。MySQL 迁移文件必须可重跑。
6. 路由预算冻结为 55 条（TestRouteBudget）；新增路由必须先删除或合并同类路由，并同步 docs/api-v2.md 与实际状态码。
7. tinynest-admin 只能本地执行、不得暴露 HTTP；purge 需 -execute -confirm=<DB名> -yes-service-stopped，凭据只从文件读取。
8. 修缺陷先补失败测试（记录 RED），再改实现（GREEN），最后跑完整回归；不虚报测试结果，未跑的写 UNVERIFIED。
9. 不恢复已退役模块；不新增框架/中间件；前端唯一新增依赖是 vue-router 4；唯一主题是 A · 奶油团子，品牌原图字节不改。
10. 不把未实现或未执行的事项写成已完成：生产清理/切库、iPhone 实机、A↔B 私网 TLS、生产 MySQL 链路与生产性能，默认 UNVERIFIED。

验证命令：
- cd apps/api && go build ./... && go test ./... -count=1 && go vet ./... && go mod tidy -diff
- cd apps/web && npm.cmd ci && npm.cmd run typecheck && npm.cmd run build && npm.cmd run test:e2e（Windows 被执行策略拦截时用 npm.cmd）
- python3 scripts/verify-brand.py、git diff --check、全树行尾空白检查（CI 同款）
- 迁移与备份恢复只在隔离临时库演练，禁止触碰生产数据库或真实账本。

边界：不 push、不创建或合并 PR、不改 main、不部署生产、不访问或修改真实账务数据。完成后给出真实命令、退出码与剩余缺口，停止等待用户复核。
```
