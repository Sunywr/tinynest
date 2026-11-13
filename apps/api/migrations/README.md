# 数据库迁移（V0.1）

> 本目录的 README 只作说明；**实际被 Go 编译进二进制的迁移文件在 `apps/api/internal/db/migrations/`**，由 `//go:embed migrations/*.sql` 嵌入。修改 schema 必须改那一目录下的 `.sql`，改本目录不会生效。

## 当前实现

- 共 7 个版本化迁移：`001_base.sql` ~ `007_seed.sql`，按文件名前 3 位数字解析版本号并排序。
- 迁移账本为 `schema_migrations(version, description, applied_at)`。Phase 0 的 `scripts/init_sqlite.py` 曾创建该表，V0.1 沿用 `CREATE TABLE IF NOT EXISTS`，不重建、不删表。
- `db.Migrate()` 逐版本检查 `schema_migrations`，每个未应用版本在独立事务内执行，成功后再写版本记录；重复运行是 no-op。
- `SeedForHousehold()` 只对新建家庭重跑幂等的 `007_seed.sql` 分类种子数据。
- `/readyz` 在 V0.1 由组合根注入真实 `quick_check` + 迁移版本探针；未注入时**只**检查 `TINYNEST_DATA_DIR` 目录存在，响应里以 `check=data_dir_exists_only` 明示，不得当作数据库已就绪。

## 约束

- SQLite 开启 `foreign_keys=ON`、`journal_mode=WAL`。
- 迁移只允许追加新版本文件，**不得修改已发布的迁移内容**；已有库不会重跑旧版本。
- 任何迁移变更都必须在临时隔离库验证：空库全量迁移、已有库重复迁移、备份恢复、`PRAGMA quick_check` 与 `PRAGMA foreign_key_check`。
- `python3 scripts/init_sqlite.py --db <临时库>` 仅用于 Phase 0 遗留的元数据初始化冒烟；它**不创建业务表**，业务表由 Go 侧迁移建立。
