# TinyNest · 合并门禁与生产发布准备度（2026-11-14 第三轮）

> 分支 `feature/tinynest-public-landing-motion`（本地，基线 `main`=`91ef72e`）。
> 两套结论严格分离：**DEV_READY（可提交 PR 的开发候选）** 与 **PRODUCTION_READY（可正式上线并录入真实数据）**。
> 本轮未 push、未合并 main、未创建 tag、未部署、未修改生产数据库或真实财务数据。

## 1. 结论

| 门禁 | 结论 | 允许的操作 |
|---|---|---|
| `AUDIT_COMPLETE` | **YES** | 最新 SHA、差异、模块矩阵、CI 与测试证据齐备 |
| `DEV_READY` | **YES（有条件）** | 可提交 PR 评审；条件是授权 push 后本分支 CI 真实绿灯 |
| `STAGING_READY` | **NO** | 不可申请隔离预发布 |
| `PRODUCTION_READY` | **NO** | 禁止部署生产、禁止录入真实家庭财务数据 |

## 2. DEV_READY 门禁（本轮实测）

| # | 要求 | 状态 | 证据 |
|---|---|---|---|
| D1 | 最新 SHA / 分支 / 差异核实 | PASS | `latest-code-audit.md`：`origin/main`=`91ef72e`，本分支从其派生，工作树仅保留用户未跟踪文档 |
| D2 | P0 / P1 缺陷为 0 | PASS | CI hygiene 红灯（P1）已修复；初始化公开入口（P0 面）已收口；账务无 P0 |
| D3 | Go build / test / vet / tidy -diff | PASS | 全 exit 0；**82 个 `Test*`**，含新增 `TestSetupDisabledFlag*`（20 次并发拒绝、受控首次部署仍可用） |
| D4 | Web npm ci / typecheck / build | PASS | 53 包；typecheck / build exit 0；CSS 26.27 kB、JS 159.37 kB |
| D5 | 真实浏览器 E2E ≥16 条新增且全通过 | PASS | **49/49**（既有 31 + 新增 18），`public-landing-e2e.md` |
| D6 | 390 / 768 / 1440 无溢出、键盘可操作 | PASS | L14 三视口 + L05 焦点陷阱 + L04 Escape；截图 `landing-*.png` |
| D7 | 账务回归不被削弱 | PASS | 既有 31 条（信用/房贷/分期/投资/统计/隔离/幂等/退款）全部保留并通过 |
| D8 | SQLite 迁移 / 重复迁移 / 备份恢复 / 借贷平衡 | PASS | `db_drill.py` 八步 exit 0（本轮未改任何迁移或账务表） |
| D9 | 安全回归（token / Cookie / 代理 / CSRF / 限流 / household） | PASS | `public-landing-security.md`；L01/L02/L06/L10/L11/L12/L15 |
| D10 | 品牌 A 奶油团子 + 原始图标字节不变 | PASS | `verify-brand.py` exit 0；动画仅作用于 `.mochi-float` wrapper |
| D11 | `git diff --check` / 敏感文件扫描 / `.workbuddy` 未入库 | PASS | 工作树与 PR 等价 `git diff --check origin/main...HEAD` = 0 |
| D12 | 本分支 GitHub Actions 真实绿灯 | **`CI_UNVERIFIED`** | 未推送；基线 #12/#13 Failure 根因已修，需授权 push 复验 |

> 若团队要求「CI 绿灯先于合并」，本分支在远端绿灯前保持 `DRAFT_PR_ONLY`。

## 3. CI 真实结果（只读网页取证）

| run | 提交 | 结论 | 失败点 |
|---|---|---|---|
| #9 `43569504212` | `f6fe9b8` | Success | — |
| #12 `43573205012` | `6fbe857`（PR #2 合并） | **Failure** | `Repository hygiene` exit 2 |
| #13 `43577502534` | **`91ef72e`（最新 main）** | **Failure** | `Repository hygiene` exit 2 |

根因：`91ef72e` 提交的 `docs/plans/2026-11-14/` 两份文档共 6 行 Markdown 尾随空格，`git diff --check` 返回 2。本轮已清理（工作树 + 暂存区 diff --check 均为 0）。**Go/Web/E2E 三个 job 在 #13 均未失败**。本轮新输入 `docs/plans/landing-20261114/01-NEXT-STEPS-PLAN.md` 也有 3 行同类尾随空格，当前未跟踪、不纳入提交；将来纳入版本控制前必须清理。

## 4. PRODUCTION_READY 门禁（NO）

| # | 要求 | 状态 |
|---|---|---|
| B1 | 生产已初始化的**现场验证**（读取实际配置/API） | **UNVERIFIED** —— 用户告知属环境事实，本轮未访问生产 |
| B2 | 生产已设置 `TINYNEST_SETUP_DISABLED=true` 并验证 setup 全量拒绝 | **UNVERIFIED** —— 本轮仅在隔离库与本机验证 |
| B3 | 双服务器 HTTPS / 可信反代网段收敛 / 伪造 `X-Forwarded-*` 拒绝实测 | **UNVERIFIED** |
| B4 | 生产令牌移除/轮换、日志不落令牌、systemd 权限闭环 | **UNVERIFIED** |
| B5 | iPhone Safari 主屏 PWA 实机（安装、离线壳、Safe Area、旧 SW 更新、恢复联网） | **UNVERIFIED** —— 无设备，需人工清单 |
| B6 | A→B 私网 TLS / 防火墙 / 备份定时任务与异地副本 / 季度恢复演练 | **UNVERIFIED** |
| B7 | Linux Chromium CI、`go test -race`、生产 LCP/CLS 与慢网样本 | **UNVERIFIED** —— 仅有本机 dev 样本（LCP 204–288ms / CLS 0.0375，明确不冒充生产） |
| B8 | 用户确认全部影响真实账务的规则与发布门禁 | 未完成（AGENTS.md 第 34 行仍为开发基线约束） |

**结论：PRODUCTION_READY = NO。** 任何「已上线 / 可录入真实账目」的表述在当前状态下都是虚假声明。

## 5. 本轮实际修改文件

| 类别 | 文件 |
|---|---|
| Go 后端 | `apps/api/internal/api/helpers.go`（Options 新字段）、`setup.go`（开关检查）、`cmd/server/main.go`（严格解析 + 注入）、`apps/api/internal/api/setup_disabled_test.go`（新增 2 测试）、`apps/api/.env.example` |
| 前端 | `apps/web/src/services/api.ts`（删 `setup()`、401 事件）、`apps/web/src/App.vue`（Landing/登录模态框/状态机/动效样式）、`apps/web/src/styles.css`（动效 tokens + reduced-motion） |
| PWA | `apps/web/public/sw.js`（CACHE 升级）、`apps/web/public/offline.html`（中性提示） |
| E2E | `apps/web/e2e/financial-closure.spec.ts`（01/02/27/ensureLogin 改造）、`apps/web/e2e/public-landing.spec.ts`（新增 18 条） |
| 文档 | `docs/api-v0.1.md`、`docs/deployment.md`、`docs/plans/2026-11-14/*.md`（清理尾随空格）、本轮 5 份交付文档 |

**未改动**：任何账务规则、口径常量、迁移、`ledger/rules.go`、原始团子图标字节、既有 31 条账务 E2E 的断言值。

## 6. P0 / P1 / P2 清单

| 级别 | 项 | 状态 |
|---|---|---|
| P0 | 公开页初始化入口（安全面） | **已修复**：UI/API 客户端/离线页全清；Go 三层保护 + 受控开关 |
| P1 | CI #12/#13 hygiene 红灯 | **已修复**（待远端复验） |
| P2 | 生产配置现场验证、iPhone 实机、双机 HTTPS、生产性能样本 | UNVERIFIED（门禁 B） |
| P2 | `go test -race`（缺 gcc）、Linux Chromium E2E | UNVERIFIED |
| P2 | 用户未跟踪计划文档含 3 行尾随空格 | 未改用户文件；纳入版本控制前需清理 |
| P2 | 文档口径冲突（AGENTS/账务设计文档仍写「10 项待确认」，权威台账为 10 `CONFIRMED`） | 沿用上一轮记录，本轮不擅自改写规范原文 |

## 7. PR 建议与生产部署条件

1. 审阅 5 份交付文档（`latest-code-audit.md`、[公开首页说明](../public-landing.md)、`public-landing-security.md`、`public-landing-e2e.md`、本文件）。
2. 确认本轮**无账务口径/迁移改动**，且新增 `TINYNEST_SETUP_DISABLED` 默认 `false` 不破坏首次部署。
3. 授权 push → 触发本分支 4 个 CI job，确认 `Repository hygiene` 归零、Linux Chromium E2E 绿 → PR 评审 → 合并开发基线。
4. 生产部署前单独批准：设置并实测 `TINYNEST_SETUP_DISABLED=true`、移除/轮换令牌、HTTPS 与可信反代、iPhone 实机 PWA、备份恢复运维、`go test -race` 与生产性能样本。
5. 合并后仍禁止自动上线；`PRODUCTION_READY` 需用户逐项确认门禁 B 后另行授权。
