# 发布候选复核

## 2026-11-16 本地候选

代码基线为 `5f395da`（包含 `b538a3f` 的信用额度修复）。Windows 本地使用 Go 1.27.4 和隔离 SQLite，通过 `go test ./... -count=1`、`go vet ./...`、`go mod tidy -diff`、`go build ./...`、`npm run build`、全部 62 条 Chromium E2E 及品牌哈希检查。两项缺陷均在修复前复现，修复后通过回归。

- Go race 已通过。在 Windows 当前进程中将 MinGW UCRT 和 Git for Windows 的 `mingw64/bin` 加入 PATH，设置 `CGO_ENABLED=1`，用 Go 1.27.4 运行 `go test -race ./... -count=1`。候选远端 CI 仍为 `UNVERIFIED`。
- SQLite 执行 `python scripts/db_drill.py --api-dir apps/api`，启动迁移、重复启动、备份、恢复和前后快照比较通过。恢复库含迁移 1–10，借贷各 105,600 分，API 返回现金 44,500 分、持仓成本 10,100 分。
- Windows 隔离 MySQL 8.0.46 以合成数据通过 `TestV01EndToEnd`、`TestV011_T1_CreditStatementLifecycle` 与 `TestConcurrentTransactionReplays`。16 个并发同键提交只入账一次。测试进程以临时拨号适配器将连接转到回环 TCP，临时实例与适配器均已清理。
- MySQL 合成账本一致性导出后恢复到同一隔离实例的空库，耗时 2.85 秒。源库与恢复库的按主键导出 SHA-256 均为 `787a03516c1f73486cf27a45f7c3af7d981438962c9fa20ad62e5a911519e229`。恢复库含迁移 1、2 和 16 条分录，借贷各 3,309,500 分。
- Linux CI 的 MySQL 演练为上述三项 Go 测试分配独立的临时数据库，以 Unix socket 执行，随后恢复并核对端到端测试库。Go 全量测试与脚本 `bash -n` 已通过。当前 Windows 工作区无 Docker 或 Linux 运行环境，更新后的脚本端到端执行仍待候选 CI。
- `STAGING_READY` 和 `PRODUCTION_READY` 仍为 `UNVERIFIED`。候选缺少隔离 Linux MySQL 预发布及并发测量、A/B 私网 TLS 现场验收、生产备份隔离恢复与异地副本校验、iPhone Safari 实机验收。生产未部署。

## 历史候选

2026-11-15 的历史候选为远端 main 的 4f24e34。Go 普通测试及竞态检测、Web 构建、Chromium E2E 和仓库检查通过，Linux MySQL Unix socket 与备份恢复步骤失败。公开记录仅提供退出码。见[候选 CI 与备份复核](ci-and-backup-verification.md)。

| 计划 | 证据与状态 |
| --- | --- |
| M1 源码、范围与 CI | [源码审计](latest-source-audit.md)及[功能矩阵](feature-acceptance-matrix.md)记录历史基线。f3279d1 的四项 CI 通过，4f24e34 的 MySQL 步骤失败 |
| M2 公开入口、认证、PWA | 公开页、受控初始化、会话与退出、焦点陷阱、减少动态效果、安装图标与旧缓存清理均有自动化覆盖。iPhone 实机未验证 |
| M3 账务回归 | Go 全量通过，涵盖信用、保险本金约束、分期、跨年退款、投资估值与家庭隔离。验证使用测试库 |
| M4 浏览器和 SQLite | 三视口及主要业务流程由浏览器回归覆盖。SQLite 演练八步通过，迁移 1–10、重启、重复迁移、备份还原、借贷各 50,000 分、完整性与外键检查通过 |
| M5 发布准备度 | 当前候选 CI 未通过。预发布与生产验收仍有未验证条目 |

## 待验收

- STAGING_READY：UNVERIFIED。需核验隔离 MySQL 部署及恢复、代理信任和私网 TLS。
- PRODUCTION_READY：UNVERIFIED。需 iPhone Safari 实机安装、更新、离线与登出证据，以及生产备份隔离恢复和异地副本验证。
- SQLite 带数据升级与恢复已验证：e569f32 写入两账户、两期房贷及首期还款、证券买入后，由当前 API 将迁移 1–9 升至 1–10。合同、期次、付款、成交及持仓逐行一致。借贷合计各 105,600 分，恢复副本启动后 API 返回现金 44,500 分、持仓成本 10,100 分。
- 家庭权限已补齐管理员、可记账成员和只读成员三级。已有会话即时采用新角色，细化到单账户的授权规则尚未定义。

## 交付索引

- 当前 SHA 与覆盖范围见本页 M1；历史差异和原始矩阵保留在对应审查文档。
- 账务回归证据见本页 M3 和 API 测试。浏览器、CI 与备份证据见本页 M1、M2、M4。
- 下一轮待办见上方待验收清单及[项目进度](../project-status.md)。
- 十项账务口径的确认记录见[决策台账](v0.1-accounting-decisions.md)。

本地补齐快速记账金额栏自动聚焦。新增断言先在基线复现失败，修复后 57 条浏览器测试全通过。Go build/test/vet/tidy、前端构建和品牌哈希通过。远端 CI 记录对应基线 f3279d1。

## MySQL 隔离恢复复核

Windows 本机独立 MySQL 8.0.46 实例运行 TestV01EndToEnd，通过测试进程内的传输适配连接回环 TCP。测试覆盖实际 Go MySQL 迁移与 HTTP 账务流程。使用 mysqldump 一致性导出到临时文件，再恢复至同一隔离实例的空库。两库按主键导出的数据内容哈希一致，经恢复的迁移版本为 1、2，共 16 条分录，借贷各 3,309,500 分。实例已关闭。

此证据覆盖本机 MySQL 业务与逻辑恢复。Linux Unix socket、生产备份恢复、异地副本与 iPhone 实机仍为 UNVERIFIED。本机 Windows 补齐 MinGW 运行库后，Go 1.23.6 的 go test -race ./... -count=1 全部通过。CI 已加入同一命令，修改后的远端工作流仍待运行。

交付记录见[当前提交与覆盖](current-sha-and-coverage.md)、[账务回归](ledger-regression.md)、[浏览器回归](web-e2e-results.md)、[CI 与备份](ci-and-backup-verification.md)和[待实施计划](../plans/next-implementation-plan.md)。

## 本地账本读性能基线

Windows amd64、Go 1.23.6、SQLite，以 API 写入一个家庭的 2,500 条合成支出后，对认证后的 HTTP 读路径执行 Go 并发基准。每次请求校验返回状态，以及月度支出总额或最近流水条数。

| 读路径 | 1 个并行工作协程 | 8 个并行工作协程 |
| --- | ---: | ---: |
| 月度首页汇总 | 55.89 ms/op | 56.61 ms/op |
| 流水首页（200 条） | 4.65 ms/op | 2.44 ms/op |
| 流水第二页（200 条） | 6.26 ms/op | 2.53 ms/op |

从 `apps/api` 运行 `go test ./internal/api -run '^$' -bench '^BenchmarkLedgerReads$' -benchtime=2s '-cpu=1,8'`。并发列为按吞吐量折算的平均每操作时间。延迟分位数、写入吞吐及生产 MySQL 性能仍为 UNVERIFIED。

SQLite 并发写入回归以 16 个并行 HTTP 请求提交同一笔支出。连续 25 轮均验证一次创建、15 次返回同一流水 ID、账户只扣款一次。复跑命令为 `go test ./internal/api -run '^TestConcurrentTransactionReplays$' -count=25 -parallel=16`。
