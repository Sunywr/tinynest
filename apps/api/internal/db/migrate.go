package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Both dialects are embedded so ExpectedVersions() is always derived from the
// files this binary was built with — never from a hardcoded list.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

//go:embed mysql/*.sql
var mysqlFS embed.FS

// schemaMetaBaselineKey / schemaMetaBaselineValue identify the one schema
// generation this binary is allowed to operate on.
const (
	schemaMetaBaselineKey   = "baseline"
	schemaMetaBaselineValue = "v2"
)

// ErrBaseline is returned when an existing database was not created by the V2
// binary. V2 is a baseline reset, so the only safe action is to refuse to start.
var ErrBaseline = errors.New("db: database is not a V2 baseline")

type migrationFile struct {
	version  int
	name     string
	body     []byte
	checksum string
}

func checksumOf(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// migrationFiles returns the embedded migrations for the active dialect, sorted
// by their 3-digit version prefix. Both Migrate and ExpectedVersions read this
// single source of truth.
func (d *DB) migrationFiles() ([]migrationFile, error) {
	dir, fsys := "migrations", embed.FS(migrationFS)
	if d.mysql {
		dir, fsys = "mysql", mysqlFS
	}
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %v", ErrMigration, dir, err)
	}
	files := make([]migrationFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") || len(entry.Name()) < 4 {
			continue
		}
		version, err := strconv.Atoi(entry.Name()[:3])
		if err != nil {
			return nil, fmt.Errorf("%w: bad migration filename %s", ErrMigration, entry.Name())
		}
		body, err := fsys.ReadFile(dir + "/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("%w: read %s: %v", ErrMigration, entry.Name(), err)
		}
		files = append(files, migrationFile{version: version, name: entry.Name(), body: body, checksum: checksumOf(body)})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: no embedded migrations for dialect %s", ErrMigration, dir)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })
	return files, nil
}

// ExpectedVersions returns every migration version this binary expects to see
// applied. It is derived from the embedded files for BOTH dialects.
func (d *DB) ExpectedVersions() ([]int, error) {
	files, err := d.migrationFiles()
	if err != nil {
		return nil, err
	}
	versions := make([]int, 0, len(files))
	for _, f := range files {
		versions = append(versions, f.version)
	}
	return versions, nil
}

// Migrate applies every missing version in ascending order and then records the
// V2 baseline in schema_meta. Re-running is a no-op.
func (d *DB) Migrate(ctx context.Context) ([]int, error) {
	files, err := d.migrationFiles()
	if err != nil {
		return nil, err
	}
	if err := d.ensureMigrationLedger(ctx); err != nil {
		return nil, err
	}
	versions, err := d.appliedVersionList(ctx)
	if err != nil {
		return nil, err
	}

	applied := make(map[int]string, len(versions))
	if len(versions) > 0 {
		// A V1 ledger has no checksum column: refuse before reading or changing
		// anything. V2 is a baseline reset and never upgrades an old database.
		if !d.ledgerHasChecksumColumn(ctx) {
			return nil, v2BaselineError("schema_migrations 是 V1 结构（缺少 checksum 列）")
		}
		// BASELINE GUARD: an existing ledger means the database already has a
		// schema. Unless schema_meta says it is the V2 baseline, refuse to touch
		// it. The operator follows docs/plans/v2-cutover-runbook.md instead.
		if err := d.assertV2Baseline(ctx); err != nil {
			return nil, err
		}
		if applied, err = d.appliedMigrations(ctx); err != nil {
			return nil, err
		}
		expected := make(map[int]migrationFile, len(files))
		for _, f := range files {
			expected[f.version] = f
		}
		for version, stored := range applied {
			f, ok := expected[version]
			if !ok {
				return nil, fmt.Errorf("%w: database reports migration version %d which this binary does not embed; refusing to start",
					ErrMigration, version)
			}
			if stored != f.checksum {
				return nil, fmt.Errorf("%w: checksum mismatch for %s: embedded %s, recorded %s; the migration file was modified after it was applied",
					ErrMigration, f.name, f.checksum, stored)
			}
		}
	} else if !d.ledgerHasChecksumColumn(ctx) {
		// An empty pre-V2 ledger holds no rows and therefore no schema history:
		// replacing it is safe and required before the first V2 row can be written.
		if err := d.recreateMigrationLedger(ctx); err != nil {
			return nil, err
		}
	}

	var newlyApplied []int
	for _, f := range files {
		if _, ok := applied[f.version]; ok {
			continue
		}
		if err := d.applyMigration(ctx, f); err != nil {
			return newlyApplied, err
		}
		newlyApplied = append(newlyApplied, f.version)
	}
	if err := d.recordBaseline(ctx); err != nil {
		return newlyApplied, err
	}
	return newlyApplied, nil
}

// appliedVersionList reads only the version numbers. It must work on both the
// V2 ledger and a V1 ledger (which has no checksum column), so the baseline
// guard can produce its explicit Chinese refusal instead of an SQL error.
func (d *DB) appliedVersionList(ctx context.Context) ([]int, error) {
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("%w: read schema_migrations: %v", ErrMigration, err)
	}
	defer rows.Close()
	versions := []int{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

// ledgerHasChecksumColumn distinguishes the V2 ledger from the V1 one.
func (d *DB) ledgerHasChecksumColumn(ctx context.Context) bool {
	var query string
	if d.mysql {
		query = `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema=DATABASE() AND table_name='schema_migrations' AND column_name='checksum'`
	} else {
		query = `SELECT COUNT(*) FROM pragma_table_info('schema_migrations') WHERE name='checksum'`
	}
	var count int
	return d.QueryRowContext(ctx, query).Scan(&count) == nil && count > 0
}

// recreateMigrationLedger replaces an EMPTY legacy ledger. It is only called
// after proving the ledger contains no rows, so no schema history is lost.
func (d *DB) recreateMigrationLedger(ctx context.Context) error {
	if _, err := d.ExecContext(ctx, `DROP TABLE schema_migrations`); err != nil {
		return fmt.Errorf("%w: drop empty legacy schema_migrations: %v", ErrMigration, err)
	}
	return d.ensureMigrationLedger(ctx)
}

// ensureMigrationLedger creates the ledger table the migrator itself needs. It
// is deliberately not part of the embedded DDL: the very first run needs it
// before it can read which files are already applied.
func (d *DB) ensureMigrationLedger(ctx context.Context) error {
	var ddl string
	if d.mysql {
		ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			description VARCHAR(191) NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			checksum VARCHAR(64) NOT NULL
		) ENGINE=InnoDB`
	} else {
		ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			checksum TEXT NOT NULL
		)`
	}
	if _, err := d.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("%w: create schema_migrations: %v", ErrMigration, err)
	}
	return nil
}

// appliedMigrations maps an applied version to its recorded checksum.
func (d *DB) appliedMigrations(ctx context.Context) (map[int]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("%w: read schema_migrations: %v", ErrMigration, err)
	}
	defer rows.Close()
	applied := make(map[int]string)
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, err
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}

// assertV2Baseline refuses anything that is not the V2 baseline. A missing
// schema_meta table (a V1 database) and a missing/other baseline value are both
// fatal: guessing would risk mixing two incompatible schemas.
func (d *DB) assertV2Baseline(ctx context.Context) error {
	var baseline string
	err := d.QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE meta_key=?`, schemaMetaBaselineKey).Scan(&baseline)
	if err == nil && baseline == schemaMetaBaselineValue {
		return nil
	}
	if err != nil && !isMissingTableError(err) && !errors.Is(err, sql.ErrNoRows) {
		// A genuine I/O error must not be disguised as a baseline mismatch.
		return fmt.Errorf("%w: read schema_meta: %v", ErrMigration, err)
	}
	detail := "schema_meta.baseline 缺失"
	if err == nil {
		detail = fmt.Sprintf("schema_meta.baseline=%q", baseline)
	}
	return v2BaselineError(detail)
}

// v2BaselineError is the single, explicit Chinese refusal for non-V2 databases.
func v2BaselineError(detail string) error {
	return fmt.Errorf("%w: 检测到现有数据库不是 V2 基线（%s）。V2 是一次基线重置，不支持旧库原地升级；"+
		"请按 docs/plans/v2-cutover-runbook.md 新建数据库并导入身份数据，不要用本二进制打开旧库",
		ErrBaseline, detail)
}

// isMissingTableError reports SQLite/MySQL "no such table" style errors without
// importing either driver into this file.
func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist")
}

// applyMigration executes one whole file and records the ledger row.
//
// SQLite: the file runs inside a transaction, together with its ledger row, so
// an interrupted migration leaves no half-applied version.
//
// MySQL: DDL statements commit implicitly, so there is no usable transaction.
// Statements are executed one by one and the ledger row is written afterwards.
// Every MySQL migration file must therefore be re-runnable (CREATE TABLE IF NOT
// EXISTS / guarded ALTER): a crash between the last DDL and the ledger write is
// repaired by running the same file again on the next start.
func (d *DB) applyMigration(ctx context.Context, f migrationFile) error {
	if d.mysql {
		for _, statement := range splitSQLStatements(string(f.body)) {
			if _, err := d.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("%w: MySQL %s: %v", ErrMigration, f.name, err)
			}
		}
		if _, err := d.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, description, checksum) VALUES(?,?,?)`,
			f.version, f.name, f.checksum); err != nil {
			return fmt.Errorf("%w: record MySQL %s: %v", ErrMigration, f.name, err)
		}
		return nil
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(f.body)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("%w: %s: %v", ErrMigration, f.name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, description, checksum) VALUES(?,?,?)`,
		f.version, f.name, f.checksum); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("%w: record %s: %v", ErrMigration, f.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: commit %s: %v", ErrMigration, f.name, err)
	}
	return nil
}

// recordBaseline marks the schema as V2 for the baseline guard and for operators.
func (d *DB) recordBaseline(ctx context.Context) error {
	var statement string
	if d.mysql {
		statement = `INSERT INTO schema_meta(meta_key, value) VALUES(?, ?)
			ON DUPLICATE KEY UPDATE value=VALUES(value)`
	} else {
		statement = `INSERT INTO schema_meta(meta_key, value) VALUES(?, ?)
			ON CONFLICT(meta_key) DO UPDATE SET value=excluded.value`
	}
	if _, err := d.ExecContext(ctx, statement, schemaMetaBaselineKey, schemaMetaBaselineValue); err != nil {
		return fmt.Errorf("%w: record schema baseline: %v", ErrMigration, err)
	}
	return nil
}

// Versions returns applied migration versions for readiness checks.
func (d *DB) Versions(ctx context.Context) ([]int, error) {
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := []int{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

// splitSQLStatements splits a MySQL script on semicolons that are not inside a
// quoted string, drops `--` / `#` line comments and `/* ... */` block comments,
// and ignores blank fragments. Migration files must not contain procedures or
// trigger bodies (no DELIMITER support).
func splitSQLStatements(script string) []string {
	var statements []string
	var current strings.Builder
	var quote rune
	runes := []rune(script)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if quote != 0 {
			current.WriteRune(ch)
			if ch == quote {
				// Doubled quote escapes ("''" or "\"\"") are consumed as two chars.
				if i+1 < len(runes) && runes[i+1] == quote {
					current.WriteRune(runes[i+1])
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			current.WriteRune(ch)
			continue
		}
		// Line comments: -- or # up to (but not including) the newline.
		if (ch == '-' && i+1 < len(runes) && runes[i+1] == '-') || ch == '#' {
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			current.WriteRune('\n')
			continue
		}
		// Block comments: /* ... */
		if ch == '/' && i+1 < len(runes) && runes[i+1] == '*' {
			i += 2
			for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			if i+1 < len(runes) {
				i++ // consume the closing '/'
			}
			current.WriteRune(' ')
			continue
		}
		if ch == ';' {
			if statement := strings.TrimSpace(current.String()); statement != "" {
				statements = append(statements, statement)
			}
			current.Reset()
			continue
		}
		current.WriteRune(ch)
	}
	if statement := strings.TrimSpace(current.String()); statement != "" {
		statements = append(statements, statement)
	}
	return statements
}
