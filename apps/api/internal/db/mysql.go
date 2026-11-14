package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

//go:embed mysql/*.sql
var mysqlFS embed.FS

// OpenMySQL connects through a local Unix socket. The connection limit
// preserves the single-writer behavior relied upon by the ledger.
func OpenMySQL(socket, database, user, password string) (*DB, error) {
	if socket == "" || database == "" || user == "" || password == "" {
		return nil, errors.New("db: MySQL socket, database, user and password are required")
	}
	config := mysql.NewConfig()
	config.Net, config.Addr = "unix", socket
	config.DBName, config.User, config.Passwd = database, user, password
	config.Collation = "utf8mb4_0900_bin"
	config.Timeout = 5 * time.Second
	conn, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return &DB{DB: conn, mysql: true}, nil
}

func (d *DB) IsMySQL() bool { return d.mysql }

func (d *DB) ExpectedVersions() ([]int, error) {
	if d.mysql {
		return []int{1}, nil
	}
	return EmbeddedVersions()
}

// MySQL DDL commits implicitly. The baseline is repeatable after interruption.
func (d *DB) migrateMySQL(ctx context.Context) ([]int, error) {
	conn, err := d.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var locked int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(CONCAT('tinynest:', DATABASE()), 10)").Scan(&locked); err != nil {
		return nil, err
	}
	if locked != 1 {
		return nil, errors.New("db: MySQL schema is locked")
	}
	defer conn.ExecContext(context.Background(), "DO RELEASE_LOCK(CONCAT('tinynest:', DATABASE()))")
	if _, err := conn.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version INT PRIMARY KEY, description VARCHAR(191) NOT NULL, applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP) ENGINE=InnoDB"); err != nil {
		return nil, err
	}
	var version int
	if err := conn.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&version); err != nil {
		return nil, err
	}
	switch version {
	case 1:
		return nil, nil
	case 0:
	default:
		return nil, fmt.Errorf("db: unsupported MySQL schema version %d", version)
	}
	body, err := mysqlFS.ReadFile("mysql/001_initial.sql")
	if err != nil {
		return nil, err
	}
	for _, statement := range strings.Split(string(body), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return nil, fmt.Errorf("%w: MySQL baseline: %v", ErrMigration, err)
		}
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO schema_migrations(version, description) VALUES(1,'001_initial.sql')"); err != nil {
		return nil, err
	}
	return []int{1}, nil
}

func (d *DB) SeedForHousehold(ctx context.Context, q Queryer, householdID string) error {
	if !d.mysql {
		return SeedForHousehold(ctx, q, householdID)
	}
	body, err := mysqlFS.ReadFile("mysql/seed.sql")
	if err != nil {
		return err
	}
	for _, statement := range strings.Split(string(body), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := q.ExecContext(ctx, statement, householdID); err != nil {
			return err
		}
	}
	return nil
}
