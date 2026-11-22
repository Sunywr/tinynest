# 数据库迁移（V2）

> 本目录的 README 只作说明。**实际被 Go 编译进二进制的迁移文件在 `apps/api/internal/db/migrations/`（SQLite）与 `apps/api/internal/db/mysql/`（MySQL）**，由 `//go:embed migrations/*.sql` / `//go:embed mysql/*.sql` 嵌入。修改 schema 必须改那两个目录，改本目录不会生效。

## 当前实现（V2 基线）

- V2 是一次**基线重置**，不支持 V1 旧库原地升级。两个方言各只有两个版本文件，同名同表集：
  - `001_core.sql`：身份、家庭、邮件、审计与复式记账内核（21 张表）。
  - `002_bookkeeping.sql`：`categories` / `accounts` / `transactions` / `budgets`（4 张表）。
- 迁移账本为 `schema_migrations(version, description, applied_at, checksum)`；`schema_meta(meta_key, value)` 记录 `baseline=v2`。
- 单一通用迁移器 `db.Migrate()`（`internal/db/migrate.go`）：
  - 读取嵌入文件、解析文件名前 3 位版本号、升序执行未应用版本，然后写入账本行（含 SHA-256 校验和）。
  - SQLite：整个文件与账本行在同一事务内执行。
  - MySQL：DDL 隐式提交，逐条执行后写账本行；因此 MySQL 文件必须可重复执行（只用 `CREATE TABLE IF NOT EXISTS` / 内联索引）。
  - 已应用版本的校验和不一致、出现二进制不认识的版本号、或 `schema_meta.baseline` 不是 `v2` 时，启动直接失败（中文错误并指向 `docs/plans/v2-cutover-runbook.md`），绝不改动旧库。
- 默认分类不再来自 SQL 种子文件，而是 `db.DefaultCategories`（Go 常量）+ `db.SeedForHousehold()`；新建家庭、审批加入或运维清理后按 `cat_<household>_<code>` 稳定 id 幂等写入。
- `/readyz` 由组合根注入真实 `quick_check` + 迁移版本探测；`db.ExpectedVersions()` 完全由嵌入文件推导（两个方言一致），不再存在硬编码版本表。

## 约束

- SQLite 打开 `foreign_keys=ON`、`journal_mode=WAL`。
- V2 只支持空库建基线；已有 V1 数据必须先按 `docs/plans/v2-cutover-runbook.md` 新建库并导入身份表。
- 任何迁移变更都必须验证：空库全量迁移、重复迁移 no-op、校验和篡改拒绝、`baseline` 不符拒绝、`PRAGMA quick_check` 与 `PRAGMA foreign_key_check`。
