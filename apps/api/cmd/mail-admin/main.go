// mail-admin is an operator-only local command. No HTTP elevation endpoint exists.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"tinynest/api/internal/auth"
	"tinynest/api/internal/db"
)

func run() error {
	action := flag.String("action", "", "grant-site-admin, revoke-site-admin, generate-key, recover-password")
	id := flag.String("user-id", "", "explicit user ID")
	keyPath := flag.String("key-file", "", "new encryption key path")
	passwordFile := flag.String("password-file", "", "protected file with replacement password")
	flag.Parse()
	if *action == "generate-key" {
		if *keyPath == "" {
			return errors.New("key-file required")
		}
		b := make([]byte, 32)
		if _, e := rand.Read(b); e != nil {
			return e
		}
		f, e := os.OpenFile(*keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = f.Write(b)
		return e
	}
	if *id == "" {
		return errors.New("explicit user-id required")
	}
	var d *db.DB
	var e error
	if os.Getenv("TINYNEST_DB_DRIVER") == "mysql" {
		p := os.Getenv("TINYNEST_MYSQL_PASSWORD_FILE")
		if p == "" {
			p = filepath.Join(os.Getenv("CREDENTIALS_DIRECTORY"), "mysql-password")
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		d, e = db.OpenMySQL(os.Getenv("TINYNEST_MYSQL_SOCKET"), os.Getenv("TINYNEST_MYSQL_DATABASE"), os.Getenv("TINYNEST_MYSQL_USER"), strings.TrimRight(string(b), "\r\n"))
	} else {
		d, e = db.Open(os.Getenv("TINYNEST_DB_PATH"))
	}
	if e != nil {
		return e
	}
	defer d.Close()
	var replacement string
	if *action == "recover-password" {
		b, e := os.ReadFile(*passwordFile)
		if e != nil {
			return e
		}
		p := strings.TrimRight(string(b), "\r\n")
		if len(p) < 12 || len(p) > 256 {
			return errors.New("password length must be 12..256")
		}
		replacement, e = auth.HashPassword(p)
		if e != nil {
			return e
		}
	}
	return d.WithTx(context.Background(), func(tx *sql.Tx) error {
		var hh string
		if e := tx.QueryRow(`SELECT household_id FROM users WHERE id=? AND status='active'`, *id).Scan(&hh); e != nil {
			return e
		}
		switch *action {
		case "grant-site-admin":
			q := `INSERT INTO site_administrators(user_id) VALUES(?) ON CONFLICT(user_id) DO NOTHING`
			if d.IsMySQL() {
				q = `INSERT IGNORE INTO site_administrators(user_id) VALUES(?)`
			}
			_, e = tx.Exec(q, *id)
		case "revoke-site-admin":
			_, e = tx.Exec(`DELETE FROM site_administrators WHERE user_id=?`, *id)
		case "recover-password":
			_, e = tx.Exec(`UPDATE users SET password_hash=? WHERE id=?`, replacement, *id)
			if e == nil {
				_, e = tx.Exec(`UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339), *id)
			}
			if e == nil {
				_, e = tx.Exec(`UPDATE email_challenges SET consumed=1 WHERE owner=?`, *id)
			}
		default:
			return errors.New("unknown action")
		}
		if e != nil {
			return e
		}
		payload, _ := json.Marshal(map[string]string{"operator": "local-command"})
		_, e = tx.Exec(`INSERT INTO audit_logs(household_id,actor,action,entity,entity_id,payload_json) VALUES(?,?,?,?,?,?)`, hh, "operator", *action, "user", *id, string(payload))
		return e
	})
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "mail-admin failed:", e)
		os.Exit(1)
	}
	fmt.Println("mail-admin completed")
}
