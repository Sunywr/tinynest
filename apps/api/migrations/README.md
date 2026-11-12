# Phase 0 数据库边界

当前仅提供 SQLite 文件初始化脚本和 `schema_migrations` 元信息表，不建立业务数据表。

运行 `python3 scripts/init_sqlite.py --db /var/lib/tinynest/tinynest.sqlite3`。

Go 后端 Phase 0 不引入 SQLite driver，不查询业务库；`/readyz` 只检测 `TINYNEST_DATA_DIR` 存在，不代表数据库可读/已迁移。
后续阶段需由用户确认业务模型和数据库访问选型后，再加入迁移/连接池与真正数据库 readiness。
