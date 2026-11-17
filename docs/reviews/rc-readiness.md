# 发布候选复核

## 2026-11-17 个人资料浮窗与头像图片发布

- A/B 均从 `f48523c` 切换到 `5ee466f`，旧版产物保留为回滚点。上线前 MySQL 备份作业成功，备份 `tinynest-20261117T142946Z.sql` 的 SHA-256 为 `62322524fabcb45e147f8891a4b0682347a91867eb6080ae078bc2d1f4868707`。
- MySQL 已应用 `005_avatar_image.sql`，生产 `users.avatar` 已为 `TEXT`。若回滚后端，停服务后仅撤回迁移记录 005，核对迁移序列为 001–004，再切回旧二进制。保留 `TEXT` 列及头像数据，避免将已保存的图片截断。
- [发布提交 CI](https://github.com/Sunywr/tinynest/actions/runs/44103245696) 四项通过，覆盖 Go race、MySQL Unix socket 与备份恢复、Web 构建及 80 项浏览器端到端回归。线上首页、JS 和 CSS 的 SHA-256 与发布产物一致。生产 MySQL 就绪，未登录的资料和密码写入接口均返回 401，www 返回 301。
- 生产账号头像上传与账务操作、本次备份的异地副本和独立恢复、iPhone Safari 实机验收为 `UNVERIFIED`。

## 2026-11-17 个人资料与账号管理发布

- A/B 均从 `cba9494` 切换到 `f48523c`。上线前 MySQL 备份作业成功，备份 `tinynest-20261117T132803Z.sql` 的 SHA-256 为 `7e5172bd5adae21680387f20dbe2b3e3deaac6f8a3021f729a73f65b9ac674da`。旧版产物保留为回滚点。旧版要求 MySQL 版本 003，回滚后端时需先停服务、撤回 004 的迁移记录并核对版本序列，保留新增列与资料数据，再切回旧二进制。
- [候选 CI](https://github.com/Sunywr/tinynest/actions/runs/44092126630) 四项通过，覆盖 Go race、MySQL Unix socket 备份恢复演练和 80 项 Chromium E2E。
- 生产 MySQL 已应用 004。HTTPS 首页、JavaScript 和 CSS 与发布产物摘要一致，MySQL 就绪检查通过，五个新增路由未登录均返回 401，www 返回 301。授权账号的生产操作、该次备份的异地副本与独立恢复、iPhone Safari 实机验收均为 `UNVERIFIED`。

## 2026-11-17 早期投资成交更正发布

- A/B 均从 `f09f0c9` 切换至 `cba9494`（合并 `d61988e` 早期投资成交更正），旧版本保留为回滚点。上线前 MySQL 备份服务执行成功（`tinynest-20261117T123946Z.sql`），后端二进制与前端产物的 SHA-256 均与构建机一致（后端 `451f4a0c…`，前端 JS `b0b04874…`）。
- [候选 CI](https://github.com/Sunywr/tinynest/actions/runs/44084445626) 四项通过，含 Go race、MySQL Unix socket 与备份恢复演练、80 项浏览器端到端回归（新增早期成交撤销重算场景）。
- 生产冒烟：HTTPS 首页 200 且引用新构建资源，线上 JS 哈希与本地一致；经 A→B 私网 TLS 的健康检查返回 ok，未登录 `/api/v1/accounts` 为 401，www 301 跳转正常。
- 授权账号的生产账务操作（一笔早期买入撤销及其更正链核对）、异地副本与 iPhone Safari 实机仍为 `UNVERIFIED`。

## 2026-11-17 退款减免恢复发布

- A/B 均从 `6f42b1e` 切换至 `f09f0c9`（含 `36cb771` 退款减免恢复），旧版本保留为回滚点。上线前 MySQL 备份服务成功，后端二进制与前端产物的 SHA-256 均与构建机一致。
- [候选 CI](https://github.com/Sunywr/tinynest/actions/runs?q=branch%3Afeature%2Frefund-reversal-restore) 四项通过，含 Go race、MySQL Unix socket、账务与备份恢复演练及 79 项浏览器端到端回归。
- 生产冒烟：HTTPS 落地页 200（www 跳转到主域），经 A→B 私网 TLS 的健康检查返回 ok，新前端资源可取，未登录 `/api/v1/accounts` 为 401。
- 授权账号的生产账务操作（含一笔带减免的退款撤销）、异地副本与 iPhone Safari 实机仍为 `UNVERIFIED`。

## 2026-11-17 删除与撤销功能发布

- A/B 均从 `3578ae7` 切换至 `6f42b1e`，保留旧版本作为回滚点。上线前 MySQL 备份服务成功，前后端产物通过 SHA-256 校验。
- [候选 CI](https://github.com/Sunywr/tinynest/actions/runs/44046088382) 四项通过，含 Go race、79 项浏览器端到端回归及 MySQL Unix socket、账务和备份恢复演练。
- 生产 HTTPS、前端 JavaScript 哈希和 MySQL 就绪检查通过。抽查成员删除、账单归档、分期退款撤销和投资成交撤销的未登录响应均为 401。授权账号的生产账务操作、异地副本与 iPhone Safari 实机仍为 `UNVERIFIED`。


## 2026-11-17 生产发布

- 按用户确认的 V1.0 前放宽规则，A/B 两机均由 `fd43a19` 切换到 `36c84f2`，旧版本继续作为回滚点。上线前生产 MySQL 备份服务执行成功，前后端产物通过 SHA-256 校验。候选 [CI 四项通过](https://github.com/Sunywr/tinynest/actions/runs/44010598288)，含隔离 MySQL 删除与备份恢复回归。
- 生产 HTTPS 首页返回新前端，线上 JavaScript 与本地构建哈希一致。MySQL 就绪检查通过，账户、项目、分类删除入口未登录时返回 401，初始化入口返回 403。
- 授权账号的生产账务操作、异地备份副本校验和 iPhone Safari 实机验收仍为 `UNVERIFIED`。

## 2026-11-16 生产验收

- 依用户确认，V1.0 之前直接部署到生产并现场验收。本次前后端均从 `190269e` 切换到 `b03f825`，旧版本保留为回滚点。候选四项 [CI](https://github.com/Sunywr/tinynest/actions/runs/43872134745) 通过。
- 生产 HTTPS、前后端产物校验、MySQL 就绪与受保护接口通过。未登录账本请求返回 401，初始化返回 403。确认代理上游启用 TLS 证书校验，服务配置启用 Secure Cookie。390px Chromium 通过首页、登录弹窗、PWA 离线壳和重连检查，API 响应未进入 Cache Storage。
- 上线前备份已导入生产 B 机的临时独立 MySQL 8.0.46 进程。实例使用独立数据目录、Unix socket 和网络隔离。导入耗时 2.55 秒，全程 13.55 秒。源与恢复后的数据行摘要一致，迁移为 1、2，分录借贷平衡。临时实例和目录已清理，生产 API 复查就绪。
- 异地副本校验、授权测试账号的账务操作、iPhone Safari 实机验收仍为 `UNVERIFIED`。

## 2026-11-16 本地候选

代码基线为 `5f395da`（包含 `b538a3f` 的信用额度修复）。Windows 本地使用 Go 1.27.4 和隔离 SQLite，通过 `go test ./... -count=1`、`go vet ./...`、`go mod tidy -diff`、`go build ./...`、`npm run build`、全部 62 条 Chromium E2E 及品牌哈希检查。两项缺陷均在修复前复现，修复后通过回归。

- Go race 已通过。在 Windows 当前进程中将 MinGW UCRT 和 Git for Windows 的 `mingw64/bin` 加入 PATH，设置 `CGO_ENABLED=1`，用 Go 1.27.4 运行 `go test -race ./... -count=1`。候选远端 CI 仍为 `UNVERIFIED`。
- SQLite 执行 `python scripts/db_drill.py --api-dir apps/api`，启动迁移、重复启动、备份、恢复和前后快照比较通过。恢复库含迁移 1–10，借贷各 105,600 分，API 返回现金 44,500 分、持仓成本 10,100 分。
- Windows 隔离 MySQL 8.0.46 以合成数据通过 `TestV01EndToEnd`、`TestV011_T1_CreditStatementLifecycle` 与 `TestConcurrentTransactionReplays`。16 个并发同键提交只入账一次。测试进程以临时拨号适配器将连接转到回环 TCP，临时实例与适配器均已清理。
- MySQL 合成账本一致性导出后恢复到同一隔离实例的空库，耗时 2.85 秒。源库与恢复库的按主键导出 SHA-256 均为 `787a03516c1f73486cf27a45f7c3af7d981438962c9fa20ad62e5a911519e229`。恢复库含迁移 1、2 和 16 条分录，借贷各 3,309,500 分。
- [PR #4 的 CI](https://github.com/Sunywr/tinynest/actions/runs/43871344082) 对 `4df8ed6` 的四项检查通过，含 Linux MySQL Unix socket 下的信用账务、同键并发、恢复和分录核对。
- MySQL CI 增加独立混合负载库。`TestMixedLedgerLoad` 使用 8 个客户端，各执行 32 次现金支出及余额读取，核对最终余额并输出总吞吐和读写各自的 p50、p95、p99。测量环境是 CI 的合成账本，生产容量验收仍需现场负载数据。
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
