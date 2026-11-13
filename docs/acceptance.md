# TinyNest V0.1 验收清单

> 本文件是**可执行清单**，不是「已通过」声明。Phase 0 时期的快速验收清单（虚构演示数据、无提交按钮、仅 `schema_migrations`）已随 V0.1 实现作废。逐项执行后据实打勾，未执行的一律保留 `- [ ]` 并标 `UNVERIFIED`。
> 权威执行记录见 `docs/reviews/v0.1-release-gates.md` 与 `docs/reviews/v0.1-pr-remediation-report.md`。

## 1. 构建与静态检查

- [ ] `cd apps/api && go build ./...`
- [ ] `cd apps/api && go test ./... -count=1`（含 `remediation_test.go` 新增用例）
- [ ] `cd apps/api && go vet ./...`
- [ ] `cd apps/api && go mod tidy -diff`（退出码必须为 0）
- [ ] `cd apps/web && npm ci`
- [ ] `cd apps/web && npm run typecheck`
- [ ] `cd apps/web && npm run build`
- [ ] `git diff --check`（退出码必须为 0）
- [ ] `python3 scripts/verify-brand.py`（原始图与 Web 副本 SHA-256 一致）
- [ ] `python3 -m json.tool apps/web/public/manifest.webmanifest`

## 2. 账务与安全回归（本轮 PR 阻断修复）

- [ ] `TestSetupRequiresBootstrapToken` / `TestSetupDisabledWithoutConfiguredToken` / `TestSetupConcurrentBootstrapOnlyOneSucceeds`
- [ ] `TestHouseholdIsolation` / `TestHouseholdIsolationOnDetailEndpoints`：两家庭收入、支出、现金流、净资产、到期事项互不串账
- [ ] `TestBusinessDateOpeningBalanceBoundaries` / `TestBusinessDateReversalUsesShanghaiDay` / `TestBusinessDateDueItemsAndDefaultPeriod`：UTC+8 边界（00:01 / 16:00 / 23:59 / 月末 / 闰年 / 跨年）
- [ ] `TestSessionCookieSecureInProductionProxy` / `...CanBeDisabledForLocalDev` / `...HonorsTrustedProxyForwardedProto` / `...IgnoresUntrustedForwardedProto`
- [ ] `TestLoginFailureRateLimit` / `TestLoginRateLimitScopeIsolation` / `TestLoginRateLimitConcurrentAndAuditSafe`（429 + `Retry-After`，审计不含口令/token）
- [ ] 复式记账内核既有业务核心用例 1–29
- [ ] `go test -race ./...`：本机缺 `gcc`/cgo，标记 `UNVERIFIED`，不得伪造通过

## 3. 数据库（只在隔离临时库执行）

- [ ] 空库全量应用 7 个迁移，`schema_migrations` 版本为 1–7
- [ ] 已有库重复执行 `Migrate()` 为 no-op，无数据变化
- [ ] `scripts/backup-sqlite.py` 备份 → 恢复到**隔离库** → 数据一致
- [ ] `PRAGMA quick_check` 返回 `ok`
- [ ] `PRAGMA foreign_key_check` 返回空
- [ ] 确认未触碰任何生产 / 真实 SQLite 文件

## 4. 部署与运行配置

- [ ] 显式设置 `TINYNEST_SETUP_TOKEN`（强随机、只存后端主机，不写仓库/日志/前端）
- [ ] 显式设置 `TINYNEST_COOKIE_SECURE=true`（缺失或非法时服务应拒绝启动）
- [ ] 生产反代来源加入 `TINYNEST_TRUSTED_PROXIES`，确认伪造 `X-Forwarded-Proto` 不被采信
- [ ] 首次初始化仅执行一次，之后 `/api/v1/setup` 返回 `409 conflict`
- [ ] `/readyz` 的 `check=sqlite_quick_check_and_migrations`
- [ ] systemd / Nginx 模板与实际环境变量一致，A/B 分机私网与防火墙约束明确
- [ ] `UNVERIFIED`：真实 HTTPS 双服务器联调、iPhone Safari 主屏 PWA 实机安装、前端 E2E

## 5. 上线门禁（未通过前不得录真实数据）

- [ ] `docs/accounting-rules-v0.1.md` 第 8 节 10 项口径全部经用户确认（`PENDING_USER_CONFIRMATION` → 已确认）
- [ ] S3 账单自动状态/额度、S4 房贷/保险 HTTP API、S5 费率规则写入 API、S7 定时任务/审计覆盖补齐
- [ ] 生产 HTTPS / 可信反代配置、iPhone 实机、端到端测试、备份恢复与安全回归全部通过
- [ ] GitHub Actions CI 有真实绿灯（未跑过时标 `CI_UNVERIFIED`）
