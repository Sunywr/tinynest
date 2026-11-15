# CI 与备份复核

| 项目 | 证据 |
| --- | --- |
| 远端基线 | f3279d1 的 Go、Web、Chromium E2E、仓库检查均通过 [GitHub Actions](https://github.com/Sunywr/tinynest/actions/runs/43606578300) |
| 本地候选 | Go 全量与竞态检测、Web 构建及 57 条浏览器测试通过。新增竞态步骤的远端运行待验证 |
| SQLite 空库与升级 | db_drill.py 两种模式均通过。e569f32 写入的房贷、还款和投资记录升级后逐行一致，从备份副本启动 API 后余额与持仓成本一致 |
| MySQL 本地隔离恢复 | MySQL 8.0.46 上通过实际 API 账务测试。恢复至空库后有序数据导出的哈希一致，共 16 条分录，借贷各 3,309,500 分 |
| 生产恢复与异地副本 | UNVERIFIED。需使用授权的生产备份和隔离恢复目标核验 |

详细范围见[发布候选复核](rc-readiness.md)，SQLite 命令见[开发说明](../DEVELOPMENT.md)。

新增 Linux CI 步骤调用 `scripts/mysql-ci-drill.sh`，在禁用网络的临时 MySQL 8.0 容器上通过挂载的 Unix socket 执行真实 API 用例，再将一致性备份恢复到空库并比对数据和借贷平衡。本地仅做 Bash 语法检查，Docker 运行与远端 CI 均为 UNVERIFIED。

生产备份的操作与验收口径见[部署文档](../deployment.md#mysql-生产备份恢复与异地副本验收)。当前缺少生产备份、隔离目标和异地副本的实测记录，状态仍为 UNVERIFIED。
