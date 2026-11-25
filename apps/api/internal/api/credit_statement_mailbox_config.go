package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"tinynest/api/internal/db"
)

type statementMailbox struct {
	email   string
	cipher  string
	version int64
}

type statementMailboxResponse struct {
	Provider    string `json:"provider"`
	Enabled     bool   `json:"enabled"`
	MaskedEmail string `json:"masked_email,omitempty"`
	Version     int64  `json:"version"`
	KeyReady    bool   `json:"key_ready"`
}

func loadStatementMailbox(ctx context.Context, q db.Queryer, p Principal, provider string) (statementMailbox, error) {
	var config statementMailbox
	err := q.QueryRowContext(ctx, `
		SELECT email, auth_code_cipher, version FROM statement_mailbox_configs
		WHERE household_id=? AND user_id=? AND provider=?`, p.HouseholdID, p.UserID, provider).
		Scan(&config.email, &config.cipher, &config.version)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return config, err
}

func (a *API) statementMailboxResponse(provider string, config statementMailbox) statementMailboxResponse {
	out := statementMailboxResponse{
		Provider: provider, Enabled: config.cipher != "", Version: config.version,
		KeyReady: len(a.opts.MailKey) == 32,
	}
	if out.Enabled {
		local, _, _ := strings.Cut(config.email, "@")
		out.MaskedEmail = "***@" + provider + ".com"
		if len(local) > 3 && local[0] >= '0' && local[0] <= '9' {
			out.MaskedEmail = local[:1] + out.MaskedEmail
		}
	}
	return out
}

func (a *API) handleGetStatementMailbox(w http.ResponseWriter, r *http.Request) {
	provider := statementMailboxProvider(w, r)
	if provider == "" {
		return
	}
	p, _ := principal(r)
	config, err := loadStatementMailbox(r.Context(), a.DB, p, provider)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, a.statementMailboxResponse(provider, config))
}

func (a *API) handlePutStatementMailbox(w http.ResponseWriter, r *http.Request) {
	provider := statementMailboxProvider(w, r)
	if provider == "" {
		return
	}
	var input struct {
		Email    string `json:"email"`
		AuthCode string `json:"auth_code"`
		Version  *int64 `json:"version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if decode(r, &input) != nil || input.Version == nil || *input.Version < 0 {
		writeErr(w, 400, "invalid_request", "请填写邮箱地址、授权码与配置版本")
		return
	}
	email := strings.TrimSpace(input.Email)
	if !validStatementMailboxCredentials(provider, email, input.AuthCode) {
		writeErr(w, 400, "invalid_request", "邮箱地址须与所选服务商一致，请检查地址和授权码")
		return
	}
	if len(a.opts.MailKey) != 32 {
		writeErr(w, 503, "mail_unavailable", "账单邮箱加密密钥尚未配置")
		return
	}

	p, _ := principal(r)
	current, err := loadStatementMailbox(r.Context(), a.DB, p, provider)
	if err != nil || current.version != *input.Version {
		if err == nil {
			err = errConflict
		}
		writeStatementMailboxError(w, err)
		return
	}
	next := statementMailbox{email: email, version: current.version + 1}
	next.cipher, err = a.mailCryptWithPurpose(input.AuthCode, false, statementMailboxPurpose(p, next))
	if err != nil {
		internalErr(w, err)
		return
	}
	if !a.probeStatementMailbox(w, r, email, input.AuthCode) {
		return
	}
	result, err := a.saveStatementMailbox(r.Context(), p, provider, current.version, next, "save_statement_mailbox")
	if err != nil {
		writeStatementMailboxError(w, err)
		return
	}
	writeJSON(w, 200, a.statementMailboxResponse(provider, result))
}

func statementMailboxPurpose(p Principal, config statementMailbox) string {
	return fmt.Sprintf("tinynest-statement-mailbox-v1\n%s\n%s\n%s\n%d",
		p.HouseholdID, p.UserID, config.email, config.version)
}

func (a *API) handleDeleteStatementMailbox(w http.ResponseWriter, r *http.Request) {
	provider := statementMailboxProvider(w, r)
	if provider == "" {
		return
	}
	var input struct {
		Version *int64 `json:"version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256)
	if decode(r, &input) != nil || input.Version == nil || *input.Version < 0 {
		writeErr(w, 400, "invalid_request", "请提供配置版本")
		return
	}
	p, _ := principal(r)
	next := statementMailbox{version: *input.Version + 1}
	result, err := a.saveStatementMailbox(r.Context(), p, provider, *input.Version, next, "disable_statement_mailbox")
	if err != nil {
		writeStatementMailboxError(w, err)
		return
	}
	writeJSON(w, 200, a.statementMailboxResponse(provider, result))
}

var errStatementMailboxForbidden = errors.New("statement mailbox permission changed")

// Serialize this user's mailbox changes and recheck the current role.
func (a *API) lockStatementMailboxUser(ctx context.Context, tx *sql.Tx, p Principal) error {
	query := `SELECT id FROM users WHERE household_id=? AND id=? AND status='active' AND role IN ('admin','member')`
	if a.DB.IsMySQL() {
		query += " FOR UPDATE"
	}
	var userID string
	err := tx.QueryRowContext(ctx, query, p.HouseholdID, p.UserID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return errStatementMailboxForbidden
	}
	return err
}

func (a *API) saveStatementMailbox(ctx context.Context, p Principal, provider string, expected int64, next statementMailbox, action string) (statementMailbox, error) {
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.lockStatementMailboxUser(ctx, tx, p); err != nil {
			return err
		}
		current, err := loadStatementMailbox(ctx, tx, p, provider)
		if err != nil {
			return err
		}
		if current.version != expected {
			return errConflict
		}
		if next.cipher == "" && current.cipher == "" {
			next = current
			return nil
		}
		if expected == 0 {
			_, err = tx.ExecContext(ctx, `INSERT INTO statement_mailbox_configs
				(household_id,user_id,provider,email,auth_code_cipher,version,updated_at) VALUES(?,?,?,?,?,?,?)`,
				p.HouseholdID, p.UserID, provider, next.email, next.cipher, next.version, a.nowUTC())
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE statement_mailbox_configs
				SET email=?,auth_code_cipher=?,version=?,updated_at=? WHERE household_id=? AND user_id=? AND provider=?`,
				next.email, next.cipher, next.version, a.nowUTC(), p.HouseholdID, p.UserID, provider)
		}
		if err != nil {
			return err
		}
		if current.email != "" && !strings.EqualFold(current.email, next.email) {
			if _, err := tx.ExecContext(ctx, `DELETE FROM statement_mailbox_card_links WHERE household_id=? AND user_id=? AND provider=?`,
				p.HouseholdID, p.UserID, provider); err != nil {
				return err
			}
		}
		return audit(ctx, tx, p, action, "credit_statement_mailbox", p.UserID, "")
	})
	return next, err
}

func writeStatementMailboxError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errConflict):
		writeErr(w, 409, "conflict", "邮箱配置已变化，请刷新后重试")
	case errors.Is(err, errStatementMailboxForbidden):
		writeErr(w, 403, "forbidden", "当前账号不能修改此邮箱配置")
	default:
		internalErr(w, err)
	}
}
