package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"tinynest/api/internal/auth"
	"tinynest/api/internal/db"
)

// The reserved row satisfies the users foreign key in both database dialects.
// Household routes reject this ID, and no ledger or members are seeded for it.
const onboardingHousehold = "onboarding"

func (a *API) registerStandalone(w http.ResponseWriter, r *http.Request, username, password string) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		internalErr(w, err)
		return
	}
	ctx := r.Context()
	err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		query := `INSERT INTO households(id,name) VALUES(?,?) ON CONFLICT(id) DO NOTHING`
		if a.DB.IsMySQL() {
			query = `INSERT INTO households(id,name) VALUES(?,?) ON DUPLICATE KEY UPDATE id=id`
		}
		if _, err := tx.ExecContext(ctx, query, onboardingHousehold, "账号开通"); err != nil {
			return err
		}
		var taken bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username=?)
			OR EXISTS(SELECT 1 FROM pending_registrations WHERE username=?)`, username, username).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return errConflict
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO users(id,household_id,username,password_hash,role)
			VALUES(?,?,?,?,'member')`, newID("usr"), onboardingHousehold, username, hash)
		return err
	})
	switch {
	case errors.Is(err, errConflict):
		writeErr(w, 409, "conflict", "用户名已使用")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 201, map[string]string{"status": "registered"})
	}
}

func (a *API) canOnboard(w http.ResponseWriter, r *http.Request, p Principal) bool {
	if p.HouseholdID != onboardingHousehold {
		writeErr(w, 409, "conflict", "账号已属于一个家庭")
		return false
	}
	var verified bool
	err := a.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM email_identities WHERE owner=?)`, p.UserID).Scan(&verified)
	if err != nil {
		internalErr(w, err)
		return false
	}
	if !verified {
		writeErr(w, 403, "email_unverified", "请先验证邮箱")
	}
	return verified
}

func (a *API) handleCreateHousehold(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	if !a.canOnboard(w, r, p) {
		return
	}
	var in struct{ Name string }
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	err := decode(r, &in)
	name := strings.TrimSpace(in.Name)
	if err != nil || name == "" || utf8.RuneCountInString(name) > 100 {
		writeErr(w, 400, "invalid_request", "请输入家庭名称（最多 100 字）")
		return
	}
	ctx := r.Context()
	id := newID("hh")
	err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO households(id,name) VALUES(?,?)`, id, name); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE users SET household_id=?,role='admin'
			WHERE id=? AND household_id=? AND status='active'`, id, p.UserID, onboardingHousehold)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return errConflict
		}
		if err := db.SeedForHousehold(ctx, tx, id); err != nil {
			return err
		}
		return bindOrUpdateMemberName(ctx, tx, id, p.UserID, p.Username)
	})
	if errors.Is(err, errConflict) {
		writeErr(w, 409, "conflict", "账号已属于一个家庭")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"household_id": id})
}

func (a *API) handleSignedInJoin(w http.ResponseWriter, r *http.Request, code string) {
	p, _ := principal(r)
	if !a.canOnboard(w, r, p) || !a.limitRegistration(w, a.clientIP(r)) {
		return
	}
	if strings.TrimSpace(code) == "" {
		writeErr(w, 400, "invalid_request", "请输入有效邀请码")
		return
	}
	token := auth.HashToken(strings.TrimSpace(code))
	now := a.nowUTC()
	result, err := a.DB.ExecContext(r.Context(), `UPDATE household_invites SET applicant=?
		WHERE token_hash=? AND expires_at>? AND decided_at IS NULL
		AND (applicant IS NULL OR applicant=?) AND (target_username IS NULL OR target_username=?)
		AND EXISTS(SELECT 1 FROM users WHERE id=? AND household_id=? AND status='active')`,
		p.Username, token, now, p.Username, p.Username, p.UserID, onboardingHousehold)
	if err != nil {
		internalErr(w, err)
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		// MySQL reports zero changed rows when the same applicant retries.
		var pending bool
		err := a.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM household_invites
			WHERE token_hash=? AND applicant=? AND expires_at>? AND decided_at IS NULL
			AND EXISTS(SELECT 1 FROM users WHERE id=? AND household_id=? AND status='active'))`,
			token, p.Username, now, p.UserID, onboardingHousehold).Scan(&pending)
		if err != nil {
			internalErr(w, err)
			return
		}
		if !pending {
			writeErr(w, 400, "invalid_request", "邀请码无效或已使用")
			return
		}
	}
	writeJSON(w, 202, map[string]string{"status": "pending"})
}

func (a *API) approveStandalone(r *http.Request, tx *sql.Tx, p Principal, invite string) (bool, error) {
	ctx := r.Context()
	var user, member string
	err := tx.QueryRowContext(ctx, `SELECT u.id,m.id FROM household_invites i
		JOIN users u ON u.username=i.applicant AND u.household_id=? AND u.status='active'
		JOIN email_identities e ON e.owner=u.id
		JOIN household_members m ON m.id=i.member_id AND m.household_id=i.household_id AND m.status='active'
		WHERE i.token_hash=? AND i.household_id=?
		AND NOT EXISTS(SELECT 1 FROM household_user_members b WHERE b.member_id=m.id)`,
		onboardingHousehold, invite, p.HouseholdID).Scan(&user, &member)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE users SET household_id=? WHERE id=? AND household_id=? AND status='active'`,
		p.HouseholdID, user, onboardingHousehold)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return false, errConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO household_user_members(user_id,member_id) VALUES(?,?)`, user, member); err != nil {
		return false, err
	}
	return true, audit(ctx, tx, p, "approve_join", "household_member", member, "{}")
}
