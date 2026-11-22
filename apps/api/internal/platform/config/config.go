// Package config owns the operator-supplied settings shared by the HTTP server
// and the destructive operator CLI, so both always address the same database.
//
// Every value here is non-secret except the MySQL password, which is read from
// a protected file (systemd LoadCredential or an operator-chosen path). A
// password is never accepted as a command-line argument: arguments leak through
// shell history and `ps`.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Driver names accepted by TINYNEST_DB_DRIVER.
const (
	DriverSQLite = "sqlite"
	DriverMySQL  = "mysql"
)

// Database is the resolved connection target.
type Database struct {
	Driver     string
	DataDir    string
	SQLitePath string

	MySQLSocket   string
	MySQLDatabase string
	MySQLUser     string
	MySQLPassword string
}

// Name reports the database name used for confirmation prompts. For SQLite the
// file path is the only stable identity, so it is reported as-is.
func (d Database) Name() string {
	if d.Driver == DriverMySQL {
		return d.MySQLDatabase
	}
	return d.SQLitePath
}

// LoadDatabase resolves the connection target from the environment. It fails
// closed: an unknown driver or an unreadable password file is an error, never a
// silent fallback to a different database.
func LoadDatabase() (Database, error) {
	d := Database{
		DataDir:    env("TINYNEST_DATA_DIR", "./data"),
		SQLitePath: env("TINYNEST_DB_PATH", ""),
	}
	if d.SQLitePath == "" {
		d.SQLitePath = filepath.Join(d.DataDir, "tinynest.sqlite3")
	}

	switch driver := strings.TrimSpace(os.Getenv("TINYNEST_DB_DRIVER")); driver {
	case "", DriverSQLite:
		d.Driver = DriverSQLite
		return d, nil
	case DriverMySQL:
		d.Driver = DriverMySQL
	default:
		return Database{}, errors.New("config: unsupported TINYNEST_DB_DRIVER (want sqlite or mysql)")
	}

	d.MySQLSocket = strings.TrimSpace(os.Getenv("TINYNEST_MYSQL_SOCKET"))
	d.MySQLDatabase = strings.TrimSpace(os.Getenv("TINYNEST_MYSQL_DATABASE"))
	d.MySQLUser = strings.TrimSpace(os.Getenv("TINYNEST_MYSQL_USER"))
	if d.MySQLSocket == "" || d.MySQLDatabase == "" || d.MySQLUser == "" {
		return Database{}, errors.New("config: TINYNEST_MYSQL_SOCKET, TINYNEST_MYSQL_DATABASE and TINYNEST_MYSQL_USER are required for the mysql driver")
	}
	password, err := readMySQLPassword()
	if err != nil {
		return Database{}, err
	}
	d.MySQLPassword = password
	return d, nil
}

// readMySQLPassword accepts only a file. The explicit variable wins; otherwise
// systemd's CREDENTIALS_DIRECTORY/mysql-password is used, matching
// docs/deployment.md.
func readMySQLPassword() (string, error) {
	path := strings.TrimSpace(os.Getenv("TINYNEST_MYSQL_PASSWORD_FILE"))
	if path == "" {
		if dir := strings.TrimSpace(os.Getenv("CREDENTIALS_DIRECTORY")); dir != "" {
			path = filepath.Join(dir, "mysql-password")
		}
	}
	if path == "" {
		return "", errors.New("config: TINYNEST_MYSQL_PASSWORD_FILE (or CREDENTIALS_DIRECTORY/mysql-password) is required for the mysql driver")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("config: cannot read the MySQL password file: " + err.Error())
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if password == "" {
		return "", errors.New("config: the MySQL password file is empty")
	}
	return password, nil
}

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
