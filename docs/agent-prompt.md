# 直接复制给任何 AI Coding Agent 的指令（V0.1 当前版本）

> **历史提示**：本文件早期版本是「只做 Phase 0、严禁记账业务与登录」的启动提示词。该阶段已完成并归档，下面这段是当前 V0.1 的有效指令。`plan.md` 与 `.plan/tinynest-phase0-cream-plan.md` 仅作历史存档，不再约束当前代码。

```text
你是一名资深全栈工程师。请在现有 TinyNest 仓库（中文「小家」/ 英文 TinyNest）上做增量维护，不要另建目录，不要重建项目。

先按 AGENTS.md 核对仓库根目录及 Git 状态，同步 main 到 origin/main。从同步后的 main 开始任务，并阅读 docs/project-status.md。

首先完整阅读：
- AGENTS.md（当前 V0.1 守则，最高优先级）
- docs/plan-v0.1.md（V0.1 权威实施计划）
- docs/accounting-rules-v0.1.md（账务设计基线，注意第 8 节 10 项待确认口径）
- docs/api-v0.1.md（API 契约及逐路由 IMPLEMENTED / PARTIAL / PLANNED 状态）
- docs/architecture.md、docs/DEVELOPMENT.md、docs/security.md、docs/deployment.md
- IMPLEMENTATION_REPORT.md 与 docs/reviews/ 下的审查 / 修复报告

当前状态：V0.1 已实现复式记账内核、家庭隔离、会话认证与 CSRF、登录失败限流、一次性初始化令牌、7 个版本化迁移、Vue3+TS+Vite 前端与 PWA 静态壳。品牌「A · 奶油团子」与 assets/brand/original-mochi-1024.png 不可改绘，用 scripts/verify-brand.py 验证。SQLite 在后端 B 机，浏览器同源调用 /api/v1/*。

必须遵守：
1. 账务唯一事实来源是 journal_entries + journal_lines；不得直接改余额，不得 UPDATE/DELETE 已入账分录，纠错走冲正分录。
2. 所有财务查询必须在 SQL 层带 household_id，不得只在 handler 层过滤。
3. 业务日期固定 Asia/Shanghai（UTC+8），统一走 API.businessToday() / businessDate()；审计与会话时间戳仍按 UTC 存储。
4. 安全配置必须显式：TINYNEST_SETUP_TOKEN、TINYNEST_COOKIE_SECURE、TINYNEST_TRUSTED_PROXIES。不得依赖 r.TLS != nil、RemoteAddr == 127.0.0.1 或客户端可伪造的 X-Forwarded-*。
5. 修缺陷先补失败测试（记录 RED），再改实现（GREEN），最后跑完整回归；不虚报测试结果，未跑的写 UNVERIFIED。
6. 第 8 节 10 项账务口径状态均为 PENDING_USER_CONFIRMATION；「按默认值实现」不等于「用户已确认」，不得改写状态，不得据此录入真实财务数据。
7. 不把未实现模块（S3 账单自动状态/额度、S4 房贷/保险 HTTP API、S5 费率规则写入 API、S7 定时任务/审计覆盖、前端 E2E / iPhone 实机 / 双服务器 HTTPS）写成已完成。

验证命令：
- cd apps/api && go build ./... && go test ./... -count=1 && go vet ./... && go mod tidy -diff
- cd apps/web && npm ci && npm run typecheck && npm run build
- python scripts/verify-brand.py、git diff --check
- 迁移与备份恢复只在隔离临时库演练，禁止触碰生产 SQLite。

边界：不 push、不创建或合并 PR、不改 main、不部署生产、不访问或修改真实财务数据。完成后给出真实命令、退出码与剩余缺口，停止等待用户复核。
```
