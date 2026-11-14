# TinyNest · 下一步实施计划与 PR 合并建议（2026-11-14）

> 本轮分支：`feature/tinynest-v0.1.2-fee-and-networth`（基于 `main` `f6fe9b8`）
> 本轮路线：**B — 补齐 V0.1.2 真实缺口**。V0.1.1 已 DEV_READY，V0.1.2 投资内核已完成，故未重开路线 A，也未进入纯验收的路线 C。
> 状态：**DEV_READY（有条件）**；本分支 CI 尚未触发（`CI_UNVERIFIED`）。

## 1. 本轮完成清单（不再重复开发）

| 模块 | 交付 | 证据 |
|---|---|---|
| 后端 | `apps/api/internal/api/v012.go`（新）：`GET /investments/fee-rules`、`POST /investments/fee-rules/preview`、`GET /dashboard/net-worth`；`computeTradeFee` 作为预览与入账的唯一费用计算源 | `TestV012_*` 6 个测试 |
| 后端接线 | `setup.go` 增加一行 `registerV012`；`investments.go` 费用计算改为调用共享函数（等价替换，未改分录规则） | 全量 74 个 Go 测试 PASS |
| 前端 | `api.ts` 4 个新方法；`App.vue` 费率面板 + 费用预览 + 首页「家庭资产构成」卡片；修复交易表单默认 `fee_minor: 0` 覆盖费率规则的缺陷 | typecheck / build PASS，E2E #23–#25 PASS |
| 文档 | `docs/api-v0.1.md` 更新新增路由与口径注记；7 份审计/测试/门禁报告 | 本文件及 `docs/reviews/next-version-*.md` |
| 测试 | 6 个 Go 新测试 + 3 条浏览器 E2E；全套 25 条 E2E、74 个 Go 测试、数据库演练全绿 | `next-version-test-report.md` |

**刻意跳过（已完成，不重构）**：复式记账内核、信用账单生命周期、固定负债/分期、每日调度器、首页三口径、移动加权成本与收益算法、分红税未知语义、手工估值、PWA/奶油团子主题、原始图标、MySQL 兼容层。

## 2. PR 合并建议（按依赖顺序，需用户授权）

1. **用户审阅**：`docs/reviews/latest-progress-2026-11-14.md`、`latest-feature-evidence-matrix.md`、`next-version-gap-analysis.md`、`next-version-test-report.md`、`next-version-e2e-report.md`、`next-version-release-gates.md`。
2. **确认本轮未改账务规则**：10 项口径维持 `CONFIRMED` 原值；本轮只新增只读/配置端点与前端展示，未动 `ledger/rules.go`、未加迁移、未加列。
3. **授权 push**（本轮未推送）→ 由 GitHub Actions 产出本分支真实绿灯（当前 `CI_UNVERIFIED`）。注意 CI Linux runner 默认 `proxy.golang.org` 可用；本机需 `GOPROXY` 换源只是本地网络限制。
4. **确认 CI E2E job 的 Linux Chromium 运行**：本机是 Windows Edge；需远端 `npx playwright install --with-deps chromium` 的 job 真实通过。
5. **PR 评审 → 合并 main**：合并前复跑 `git diff --check`；若团队要求 CI 先绿，则本分支保持 `DRAFT_PR_ONLY`。
6. 合并后**仍不得**宣布生产可用或录入真实家庭财务数据（门禁 B 仍为 NO）。

## 3. 待用户决策（不改代码，等确认）

| # | 事项 | 现状 | 需要决策 |
|---|---|---|---|
| D1 | 分期合同创建分录 `occurred_on` 取 `first_due_on` | `credit.go:421` 既有行为；跨月时「截至某日」资产负债快照与「消费发生日」口径不同 | 是否统一改为消费发生日？涉及口径第 4 项，**未获确认前不擅自修改** |
| D2 | 费率规则「删」 | 有意不提供硬删除（历史交易存 `fee_rule_id` + 快照） | 是否需要「停用」语义？若需要，须同时补 SQLite 与 MySQL 迁移，另开任务 |
| D3 | 费用预览是否要在输入变动时自动刷新 | 当前点击「预览费用」按钮触发，结果带成交金额与规则版本 | 是否改为实时联动（会增加请求量） |
| D4 | 历史审查文档的「0/10 待确认」表述 | `v0.1.1-accounting-decision-impact.md`、`v0.1.1-baseline.md` 第 7 节 | 是否允许改写历史记录，还是保留原文 + 加注 |
| D5 | `README.md` / `DEVELOPMENT.md` / `project-status.md` 过时描述 | 仍写「7 版迁移 / E2E 未验证 / 口径待确认」 | 是否在本 PR 一并刷新（建议下一个文档专项 PR） |

## 4. 未完成缺陷台账（P0=0 / P1=0）

| 级别 | 项 |
|---|---|
| P2-1 | `investments.go:310` 估值子查询未显式带 `household_id`（父查询已隔离，实际不串账，形式不符守则） |
| P2-2 | 11-13 历史审查文档的「0/10」与最新台账「10/10」冲突 |
| P2-3 | README / DEVELOPMENT / project-status 与代码事实不符 |
| P2-4 | 本机环境限制：默认 Go 代理不可达、Go 不在 PATH、npm ci 沙箱批量删除保护、`HTTP_PROXY` 拦截本地 E2E 探测（均有可复现绕过方式，非代码缺陷） |
| P2-5 | 分期创建分录日期口径（见 D1） |
| P2-6 | `apps/web/test-results/.last-run.json` 被 git 跟踪，建议 `git rm --cached` |

## 5. 后续路线（本轮不实施）

**路线 C · V0.1.3 预发布可靠性**（在 V0.1.2 合并且 CI 绿灯后）：

1. 本分支远端 CI 与 Linux Chromium E2E 真实绿灯；`go test -race` 在具备 gcc 的 runner 通过。
2. 双服务器 HTTPS 正式链路、可信反代网段收敛、伪造 `X-Forwarded-*` 拒绝验证。
3. 一次性初始化令牌的启用/轮换/日志不落令牌闭环；生产备份定时任务与异地副本、季度恢复演练。
4. iPhone Safari 主屏 PWA 实机清单（Add to Home Screen、离线壳、390px 实操、恢复联网）。
5. 性能基准与并发压力；审计查询生产形态抽查。
6. 文档专项：README / DEVELOPMENT / project-status 与当前代码对齐。

> 任何阶段都不得把 `DEV_READY` 自动升级为 `PRODUCTION_READY`；正式放行需用户逐项确认门禁 B。
