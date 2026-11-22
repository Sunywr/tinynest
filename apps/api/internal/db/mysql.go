package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// OpenMySQL connects through a local Unix socket. The connection limit
// preserves the single-writer behavior relied upon by the ledger. Schema
// migration is shared with SQLite through migrate.go — there is no separate
// per-version MySQL path anymore.
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
		_ = conn.Close()
		return nil, err
	}
	return &DB{DB: conn, mysql: true}, nil
}

// IsMySQL reports the active dialect so SQL-only differences (upserts, FOR
// UPDATE, DDL) can be branched explicitly.
func (d *DB) IsMySQL() bool { return d.mysql }
