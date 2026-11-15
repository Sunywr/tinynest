# 发布候选复核

2026-11-15，复核基线为远端 main 的 f3279d1。190269e 实现列表优先、动作打开表单、首页快速记账和本机偏好。f3279d1 补入计划和历史验收资料。

| 计划 | 证据与状态 |
| --- | --- |
| M1 源码、范围与 CI | [源码审计](latest-source-audit.md)及[功能矩阵](feature-acceptance-matrix.md)记录历史基线。[当前基线 CI](https://github.com/Sunywr/tinynest/actions/runs/43606578300)的 Go、Web、E2E、仓库检查四项通过 |
| M2 公开入口、认证、PWA | 公开页、受控初始化、会话与退出、焦点陷阱、减少动态效果、安装图标与旧缓存清理均有自动化覆盖。iPhone 实机未验证 |
| M3 账务回归 | Go 全量通过，涵盖信用、保险本金约束、分期、跨年退款、投资估值与家庭隔离。验证使用测试库 |
| M4 浏览器和 SQLite | 三视口及主要业务流程由浏览器回归覆盖。SQLite 演练八步通过，迁移 1–10、重启、重复迁移、备份还原、借贷各 50,000 分、完整性与外键检查通过 |
| M5 发布准备度 | 开发基线 CI 已通过。预发布与生产验收仍有未验证条目 |

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

独立交付记录：[当前提交与覆盖](current-sha-and-coverage.md)、[账务回归](ledger-regression.md)、[浏览器回归](web-e2e-results.md)、[CI 与备份](ci-and-backup-verification.md)、[下一轮待办](next-iteration-backlog.md)。
