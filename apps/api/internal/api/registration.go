package api

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"tinynest/api/internal/auth"
)

var registrationName = regexp.MustCompile("^[a-z0-9][a-z0-9_.-]{2,39}$")
var registrationWork = make(chan struct{}, 2)
var registrationAttempts = newLoginLimiter(LoginLimiterConfig{MaxFailures: 8}, time.Now)

func limitRegistration(w http.ResponseWriter, ip string) bool {
	if blocked, retry := registrationAttempts.failure("registration", ip); blocked {
		w.Header().Set("Retry-After", retryAfterSeconds(retry))
		writeErr(w, 429, "rate_limited", "操作太频繁，请稍后重试")
		return false
	}
	return true
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !limitRegistration(w, a.clientIP(r)) {
		return
	}
	var in struct {
		Username string
		Password string
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if decode(r, &in) != nil || !registrationName.MatchString(in.Username) || len(in.Password) < 12 || len(in.Password) > 256 {
		writeErr(w, 400, "invalid_request", "登录名须为 3–40 位小写字母、数字或 ._-，密码至少 12 位")
		return
	}
	select {
	case registrationWork <- struct{}{}:
		defer func() { <-registrationWork }()
	default:
		writeErr(w, 429, "rate_limited", "请稍后重试")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		internalErr(w, err)
		return
	}
	// Existing names produce the same response and never overwrite credentials.
	query := `INSERT INTO pending_registrations(username,password_hash,created_at)
 SELECT ?,?,? WHERE NOT EXISTS(SELECT 1 FROM users WHERE username=?)
 ON CONFLICT(username) DO NOTHING`
	if a.DB.IsMySQL() {
		query = `INSERT INTO pending_registrations(username,password_hash,created_at)
 SELECT ?,?,? WHERE NOT EXISTS(SELECT 1 FROM users WHERE username=?)
 ON DUPLICATE KEY UPDATE created_at=pending_registrations.created_at`
	}
	if _, err = a.DB.ExecContext(r.Context(), query, in.Username, hash, a.nowUTC(), in.Username); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "accepted"})
}

func (a *API) handleIssueInvite(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		MemberID   string `json:"member_id"`
		MemberName string `json:"member_name"`
		Username   string
	}
	if decode(r, &in) != nil {
		writeErr(w, 400, "invalid_request", "邀请码资料无效")
		return
	}
	in.MemberName = strings.TrimSpace(in.MemberName)
	if (in.MemberID == "") == (in.MemberName == "") || utf8.RuneCountInString(in.MemberName) > 100 {
		writeErr(w, 400, "invalid_request", "请选择现有档案，或填写新成员姓名（最多 100 字）")
		return
	}
	if in.Username != "" && !registrationName.MatchString(in.Username) {
		writeErr(w, 400, "invalid_request", "指定登录名须为 3–40 位小写字母、数字或 ._-")
		return
	}
	raw, hash, err := auth.NewToken()
	if err != nil {
		internalErr(w, err)
		return
	}
	unavailable := errors.New("成员档案已绑定或不可用，请选择其他档案")
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		ctx := r.Context()
		if in.MemberName != "" {
			in.MemberID = newID("mem")
			if _, err := tx.ExecContext(ctx, `INSERT INTO household_members(id,household_id,name) VALUES(?,?,?)`,
				in.MemberID, p.HouseholdID, in.MemberName); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO household_invites(token_hash,household_id,member_id,target_username,expires_at)
			SELECT ?,m.household_id,m.id,NULLIF(?,''),? FROM household_members m
			WHERE m.id=? AND m.household_id=? AND m.status='active'
			AND NOT EXISTS(SELECT 1 FROM household_user_members b WHERE b.member_id=m.id)`,
			hash, in.Username, a.now().UTC().Add(48*time.Hour).Format(time.RFC3339), in.MemberID, p.HouseholdID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return unavailable
		}
		return audit(ctx, tx, p, "issue_invitation", "household_member", in.MemberID, "{}")
	})
	if errors.Is(err, unavailable) {
		writeErr(w, 404, "not_found", err.Error())
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"code": raw})
}

func (a *API) handleApplyInvite(w http.ResponseWriter, r *http.Request) {
	if !limitRegistration(w, a.clientIP(r)) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var in struct{ Username, Password, Code string }
	if decode(r, &in) != nil || !registrationName.MatchString(in.Username) || len(in.Password) > 256 || len(in.Code) > 160 {
		writeErr(w, 400, "invalid_request", "申请信息无效")
		return
	}
	select {
	case registrationWork <- struct{}{}:
		defer func() { <-registrationWork }()
	default:
		writeErr(w, 429, "rate_limited", "请稍后重试")
		return
	}
	var hash string
	err := a.DB.QueryRowContext(r.Context(), `SELECT password_hash FROM pending_registrations WHERE username=?`, in.Username).Scan(&hash)
	if err != nil || !auth.VerifyPassword(hash, in.Password) {
		writeErr(w, 401, "unauthorized", "注册账号或密码无效")
		return
	}
	result, err := a.DB.ExecContext(r.Context(), `UPDATE household_invites SET applicant=?
 WHERE token_hash=? AND expires_at>? AND decided_at IS NULL
 AND (applicant IS NULL OR applicant=?) AND (target_username IS NULL OR target_username=?)`,
		in.Username, auth.HashToken(strings.TrimSpace(in.Code)), a.nowUTC(), in.Username, in.Username)
	if err != nil {
		internalErr(w, err)
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		writeErr(w, 400, "invalid_request", "邀请码无效或已使用")
		return
	}
	writeJSON(w, 202, map[string]string{"status": "pending"})
}

func (a *API) handleListApplications(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `SELECT i.token_hash,i.applicant,m.name FROM household_invites i
 JOIN household_members m ON m.id=i.member_id AND m.household_id=i.household_id
 WHERE i.household_id=? AND i.applicant IS NOT NULL AND i.decided_at IS NULL AND i.expires_at>? ORDER BY i.expires_at`, p.HouseholdID, a.nowUTC())
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, username, name string
		if err := rows.Scan(&id, &username, &name); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]string{"id": id, "username": username, "member_name": name})
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *API) handleDecideApplication(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct{ Approve *bool }
	if decode(r, &in) != nil || in.Approve == nil {
		writeErr(w, 400, "invalid_request", "请选择同意或拒绝")
		return
	}
	id := r.PathValue("id")
	unavailable := errors.New("申请已处理、已过期或成员已绑定")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		ctx := r.Context()
		result, err := tx.ExecContext(ctx, `UPDATE household_invites SET decided_at=?
  WHERE token_hash=? AND household_id=? AND applicant IS NOT NULL AND decided_at IS NULL AND expires_at>?`, a.nowUTC(), id, p.HouseholdID, a.nowUTC())
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return unavailable
		}
		if !*in.Approve {
			return audit(ctx, tx, p, "reject_join", "household_member", "", "{}")
		}
		var username, member, hash string
		err = tx.QueryRowContext(ctx, `SELECT i.applicant,i.member_id,r.password_hash FROM household_invites i
  JOIN pending_registrations r ON r.username=i.applicant
  JOIN household_members m ON m.id=i.member_id AND m.household_id=i.household_id AND m.status='active'
  WHERE i.token_hash=? AND i.household_id=?
  AND NOT EXISTS(SELECT 1 FROM household_user_members b WHERE b.member_id=i.member_id)
  AND NOT EXISTS(SELECT 1 FROM users u WHERE u.username=i.applicant)`, id, p.HouseholdID).Scan(&username, &member, &hash)
		if errors.Is(err, sql.ErrNoRows) {
			return unavailable
		}
		if err != nil {
			return err
		}
		user := newID("usr")
		if _, err = tx.ExecContext(ctx, `INSERT INTO users(id,household_id,username,password_hash,role) VALUES(?,?,?,?,'member')`, user, p.HouseholdID, username, hash); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO household_user_members(user_id,member_id) VALUES(?,?)`, user, member); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM pending_registrations WHERE username=?`, username); err != nil {
			return err
		}
		return audit(ctx, tx, p, "approve_join", "household_member", member, "{}")
	})
	if errors.Is(err, unavailable) {
		writeErr(w, 409, "conflict", unavailable.Error())
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "decided"})
}

func (a *API) handleListInvitations(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT i.token_hash,m.name,COALESCE(i.target_username,''),i.expires_at
		FROM household_invites i JOIN household_members m
		  ON m.id=i.member_id AND m.household_id=i.household_id
		WHERE i.household_id=? AND i.decided_at IS NULL AND i.expires_at>?
		ORDER BY i.expires_at DESC,i.token_hash`, p.HouseholdID, a.nowUTC())
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, name, target, expires string
		if err := rows.Scan(&id, &name, &target, &expires); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]string{"id": id, "member_name": name, "target_username": target, "expires_at": expires})
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *API) handleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	unavailable := errors.New("邀请码已失效或不存在")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		now := a.nowUTC()
		result, err := tx.ExecContext(r.Context(), `
			UPDATE household_invites SET decided_at=?
			WHERE token_hash=? AND household_id=? AND decided_at IS NULL AND expires_at>?`,
			now, id, p.HouseholdID, now)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return unavailable
		}
		return audit(r.Context(), tx, p, "revoke_invitation", "household_invite", id, "{}")
	})
	if errors.Is(err, unavailable) {
		writeErr(w, 404, "not_found", unavailable.Error())
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
