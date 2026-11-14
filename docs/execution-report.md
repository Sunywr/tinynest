# TinyNest 执行报告（本机真实执行，2026-11-12）

> **历史存档**：本文记录 2026-11-12 当时的真实命令与结果，其中的 `.plan/tinynest-v0.1-implementation-plan.md` 路径已废弃（权威计划为 `docs/plan-v0.1.md`），Phase 0 相关结论不再代表当前代码状态。当前状态见 `IMPLEMENTATION_REPORT.md` 与 `docs/reviews/`。
> 本文件由 2026-11-12 本次实施**实际执行的命令与结果**重写，替换原先「打包方核验」的二手声明。
> 完整交付结论见仓库根目录 `IMPLEMENTATION_REPORT.md`。

## 1. 环境

| 项 | 实际值 |
|---|---|
| OS / Shell | Windows / Git Bash |
| Node / npm | v22.22.2 / 10.9.7 |
| Python | 3.13.14 |
| Go | **本机原本缺失**；从阿里云镜像下载 Go 1.23.6 windows-amd64 便携版，解压到 `%TEMP%\tinynest-go`（仓库外，未改系统 PATH） |
| Go 依赖源 | `https://goproxy.cn`（`proxy.golang.org` 在本机不可达），`GOSUMDB=off` |
| Git | 分支 `feature/tinynest-phase0-v0.1`，基于 `main`；**未 push、未改 main** |

## 2. 实际执行的命令与结果

| # | 命令 | 结果 |
|---|---|---|
| 1 | `git status` / `git log --oneline` / `git branch -a` | 干净工作区，仅 `.plan/tinynest-v0.1-implementation-plan.md` 未跟踪；2 个历史提交 |
| 2 | `git checkout -b feature/tinynest-phase0-v0.1` | ✅ 已创建独立分支 |
| 3 | `python scripts/verify-brand.py` | ✅ 原始图与 Web 副本 SHA-256 一致 |
| 4 | `python -m json.tool apps/web/public/manifest.webmanifest` | ✅ JSON 合法 |
| 5 | `python scripts/init_sqlite.py --db <临时库>` + `PRAGMA quick_check` | ✅ 仅 `schema_migrations`，`quick_check=ok`，`journal_mode=wal` |
| 6 | `python scripts/backup-sqlite.py --help` | ✅ 参数正常（仅冒烟，未做恢复演练） |
| 7 | `cd apps/web && npm install` | ✅ 50 包（耗时约 7 分钟，registry 可达但慢） |
| 8 | `npm run build` | ❌ **首次失败**：`vite.config.ts(5,29) TS2591: Cannot find name 'process'`；修为 `loadEnv(mode,'.','VITE_')` 后 ✅ 通过 |
| 9 | `go get modernc.org/sqlite@v1.34.5`、`golang.org/x/crypto@v0.31.0` | ✅ 16 秒完成（纯 Go 驱动，无 CGO） |
| 10 | `go test ./...` | ✅ 全部通过（api / credit / invest / ledger / money / server） |
| 11 | `go vet ./...` | ✅ 无告警 |
| 12 | `go build ./...` + `go build -o tinynest-api.exe ./cmd/server` | ✅ 15 MB 二进制 |
| 13 | `bash scripts/check-phase0.sh` | ✅ 全通过（品牌 + Go test/vet + Vue build） |
| 14 | 启动真实 `tinynest-api.exe` + curl 全链路 | ✅ 见第 3 节 |
| 15 | 端到端集成测试 `go test ./internal/api -v` | ✅ 通过 |

## 3. 真实进程联调记录

```
healthz 200 {"app":"tinynest-api","status":"ok","phase":"v0.1"}
readyz  200 {"status":"ready","check":"sqlite_quick_check_and_migrations"}
setup   201 {"status":"initialized"}
login   200 (HttpOnly Cookie + csrf_token)
me      200
categories   21 个默认分类（16 支出 + 4 收入 + 投资分红）
POST /accounts 201 → 期初 1,000,000 分
POST /transactions 201 → 支出 10,000 分
GET /accounts/{id}/balance 200 → 990000 分（由分录重建）
GET /dashboard/summary → income=0 expense=10000 cashflow=-10000 net_worth=990000
未认证 GET /accounts → 401
```

## 4. 修复的真实缺陷（均已回归验证）

1. `vite.config.ts` 使用 `process.cwd()` 缺 `@types/node` → Vue 构建失败。
2. 单连接 SQLite 下 `rows` 未关闭即嵌套查询余额 → 3 处 handler 死锁。
3. `investment_trades.price_scaled` 未写入 → CHECK 约束失败、交易 400。
4. `last_valued_on` 日期字符串 Scan 成 int64 → 持仓接口 500。
5. 首页按 `4%`/`5%` 汇总把投资分红/已实现损益/红利税混入本期收支 → 口径错误。
6. 期初余额计入「本期实际现金净流量」→ 口径错误。

## 5. 未验证 / 阻碍（不得视为通过）

| 项 | 状态 |
|---|---|
| iPhone Safari 添加到主屏幕实机、启动画面、安全区 | **未验证**（无实机 / 无 HTTPS 域名） |
| 生产 HTTPS、A→B 私网 TLS、双服务器真实上线 | **未验证**（仅模板，未连接任何服务器） |
| SQLite 备份后**恢复演练**与账务一致性比对 | **未验证**（脚本冒烟通过，未做恢复） |
| 真实浏览器 390 / 768 / 1440px 视觉实测 | **未验证**（仅有 Phase 0 存档预览图） |
| 前端 E2E / 视觉自动化测试 | **未验证**（仅类型检查 + 生产构建） |
| V0.1 全部发布门禁 | **未通过**（未完成项见 `IMPLEMENTATION_REPORT.md` 第 8 节） |

## 6. 变更文件

- 修改：`apps/web/vite.config.ts`、`apps/web/index.html`、`apps/web/public/manifest.webmanifest`、`apps/web/public/sw.js`、`apps/web/src/App.vue`、`apps/web/src/services/api.ts`、`apps/api/cmd/server/main.go`、`apps/api/internal/server/server.go`、`apps/api/go.mod`、本文件、README / 架构 / 开发文档。
- 新增：`apps/api/internal/{money,ledger,invest,credit,db,auth,api}/`、`apps/api/internal/db/migrations/001..007.sql`、`apps/api/go.sum`、`apps/api/internal/api/api_test.go`、`docs/{plan-v0.1,accounting-rules-v0.1,er-v0.1,api-v0.1}.md`、`IMPLEMENTATION_REPORT.md`、`apps/web/package-lock.json`。
- 保留未动：`assets/brand/*`（原始图标字节未改）、`previews/*`、`deploy/*` 模板、`scripts/*`、`plan.md`、`.plan/*`。

## Phase 0 计划归档

Phase 0 交付静态奶油团子原型、Vue 与 Go 工程骨架、PWA 安装资源、健康检查和双机部署模板。原始计划保留在 Git 历史中。后续账本已经采用真实数据库和 API，当前进度及仍需实机验证的事项见 [项目进度](project-status.md)。

## 7. 原始报告的下一步建议

1. 在具备标准 Go 环境的机器复现 `go test ./... && npm run build`。
2. 用户提供域名、A/B 服务器私网信息与证书后，再落地 HTTPS 与 iPhone 实机验证。
3. 执行 V0.1 业务库的备份→恢复→账务一致性演练。
4. 确认 `docs/accounting-rules-v0.1.md` 第 8 节 10 项口径后，再用于真实家庭账务。
