package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
	"tinynest/api/internal/auth"
	"tinynest/api/internal/db"
)

var errMailValidation = errors.New("验证失败、已过期或已使用")

type mailChallengeInput struct {
	Email           string `json:"email"`
	Purpose         string `json:"purpose"`
	CurrentPassword string `json:"current_password"`
}
type mailProof struct {
	ChallengeID string `json:"challenge_id"`
	Code        string `json:"code"`
}

func (a *API) digestCode(id, code string) string {
	m := hmac.New(sha256.New, a.opts.MailKey)
	m.Write([]byte(id + ":" + code))
	return hex.EncodeToString(m.Sum(nil))
}
func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func (a *API) handleMailIdentity(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var email string
	var subscribed = 1
	e := a.DB.QueryRowContext(r.Context(), `SELECT email FROM email_identities WHERE owner=?`, p.UserID).Scan(&email)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		internalErr(w, e)
		return
	}
	_ = a.DB.QueryRowContext(r.Context(), `SELECT announcements FROM mail_preferences WHERE user_id=?`, p.UserID).Scan(&subscribed)
	c, _, _, e := a.mailConfig(r.Context())
	if e != nil {
		internalErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"email": email, "verified": email != "", "announcements": subscribed == 1, "site_admin": a.siteAdmin(r), "mail_available": c.Enabled && len(a.opts.MailKey) == 32})
}
func (a *API) handleMailPreferences(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Announcements bool `json:"announcements"`
	}
	if decode(r, &in) != nil {
		writeErr(w, 400, "invalid_request", "请求无效")
		return
	}
	q := `INSERT INTO mail_preferences(user_id,announcements) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET announcements=excluded.announcements`
	if a.DB.IsMySQL() {
		q = `INSERT INTO mail_preferences(user_id,announcements) VALUES(?,?) ON DUPLICATE KEY UPDATE announcements=VALUES(announcements)`
	}
	if _, e := a.DB.ExecContext(r.Context(), q, p.UserID, in.Announcements); e != nil {
		internalErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (a *API) handlePrivateChallenge(w http.ResponseWriter, r *http.Request) {
	var in mailChallengeInput
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if decode(r, &in) != nil {
		writeErr(w, 400, "invalid_request", "请求无效")
		return
	}
	p, _ := principal(r)
	if !a.verifyCurrent(r, p.UserID, in.CurrentPassword) {
		writeErr(w, 400, "invalid_request", "当前密码无效或尝试过多")
		return
	}
	in.Email = normalizeEmail(in.Email)
	switch in.Purpose {
	case "bind":
		if !validEmail(in.Email) {
			writeErr(w, 400, "invalid_request", "邮箱格式无效")
			return
		}
	case "old_email", "password":
		if a.DB.QueryRowContext(r.Context(), `SELECT email FROM email_identities WHERE owner=?`, p.UserID).Scan(&in.Email) != nil {
			writeErr(w, 400, "email_unverified", "请先验证邮箱")
			return
		}
	default:
		writeErr(w, 400, "invalid_request", "验证用途无效")
		return
	}
	a.issueChallenge(w, r, p.UserID, p.HouseholdID, in.Email, in.Purpose)
}
func (a *API) handlePublicChallenge(w http.ResponseWriter, r *http.Request) {
	var in mailChallengeInput
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if decode(r, &in) != nil {
		writeErr(w, 400, "invalid_request", "请求无效")
		return
	}
	in.Email = normalizeEmail(in.Email)
	if !validEmail(in.Email) || (in.Purpose != "register" && in.Purpose != "reset") {
		writeErr(w, 400, "invalid_request", "邮箱或用途无效")
		return
	}
	owner := "registration:" + in.Email
	hh := ""
	if in.Purpose == "reset" {
		owner = ""
		_ = a.DB.QueryRowContext(r.Context(), `SELECT u.id,u.household_id FROM email_identities e JOIN users u ON u.id=e.owner WHERE e.email=? AND u.status='active'`, in.Email).Scan(&owner, &hh)
	}
	a.issueChallenge(w, r, owner, hh, in.Email, in.Purpose)
}
func (a *API) issueChallenge(w http.ResponseWriter, r *http.Request, owner, hh, email, purpose string) {
	// Independent buckets prevent rotating email addresses or IPs from bypassing limits.
	for _, bucket := range []string{"mail-ip:" + a.clientIP(r), "mail-email:" + email, "mail-owner:" + email} {
		if blocked, _ := a.limiter.failure(bucket, "mail"); blocked {
			writeErr(w, 429, "rate_limited", "操作过于频繁，请稍后重试")
			return
		}
	}
	a.mailMu.Lock()
	defer a.mailMu.Unlock()
	c, _, _, e := a.mailConfig(r.Context())
	if e != nil || !c.Enabled || len(a.opts.MailKey) != 32 {
		writeErr(w, 503, "mail_unavailable", "邮件服务尚未启用")
		return
	}
	var used, queued int
	e = a.DB.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(total),0) FROM mail_daily_usage WHERE day=?`, a.businessDate()).Scan(&used)
	if e != nil {
		internalErr(w, e)
		return
	}
	e = a.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM mail_queue WHERE status IN ('queued','sending') AND expires_at>?`, a.nowUTC()).Scan(&queued)
	if e != nil {
		internalErr(w, e)
		return
	}
	if used+queued >= c.DailyLimit {
		writeErr(w, 429, "mail_quota", "当日邮件额度不足，请稍后重试")
		return
	}

	if err := a.checkRecipientBudget(r.Context(), a.DB, email); err != nil {
		if errors.Is(err, errRecipientDailyLimit) {
			writeErr(w, 429, "email_daily_limit", err.Error())
		} else {
			internalErr(w, err)
		}
		return
	}
	id := newID("challenge")
	if owner == "" {
		writeJSON(w, 202, map[string]string{"status": "accepted", "challenge_id": id})
		return
	}
	var recent int
	if e = a.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM email_challenges WHERE email=? AND purpose=? AND created_at>?`, email, purpose, a.now().UTC().Add(-time.Minute).Format(time.RFC3339)).Scan(&recent); e != nil {
		internalErr(w, e)
		return
	}
	if recent > 0 {
		if purpose == "reset" {
			writeJSON(w, 202, map[string]string{"status": "accepted", "challenge_id": id})
			return
		}
		writeErr(w, 429, "rate_limited", "请在60秒后重发")
		return
	}
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		internalErr(w, e)
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())
	expires := a.now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	e = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `UPDATE email_challenges SET consumed=1 WHERE owner=? AND purpose=?`, owner, purpose); e != nil {
			return e
		}
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO email_challenges(id,owner,email,purpose,digest,created_at,expires_at) VALUES(?,?,?,?,?,?,?)`, id, owner, email, purpose, a.digestCode(id, code), a.nowUTC(), expires); e != nil {
			return e
		}
		return a.enqueueMail(r.Context(), tx, hh, owner, email, "verification", "TinyNest 邮箱验证码", "验证码："+code+"，10分钟内有效。请勿向他人透露。", expires, id)
	})
	if e != nil {
		internalErr(w, e)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "accepted", "challenge_id": id})
}

// Verify attempts are committed independently, including rejected guesses. Successful
// consumption occurs in the business transaction, so concurrent replay cannot win.
func (a *API) checkProof(ctx context.Context, proof mailProof, owner, purpose, email string) error {
	if len(proof.Code) != 6 {
		return errMailValidation
	}
	res, e := a.DB.ExecContext(ctx, `UPDATE email_challenges SET attempts=attempts+1 WHERE id=? AND owner=? AND purpose=? AND email=? AND consumed=0 AND attempts<5 AND expires_at>?`, proof.ChallengeID, owner, purpose, email, a.nowUTC())
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errMailValidation
	}
	var digest string
	e = a.DB.QueryRowContext(ctx, `SELECT digest FROM email_challenges WHERE id=?`, proof.ChallengeID).Scan(&digest)
	if e != nil || !hmac.Equal([]byte(digest), []byte(a.digestCode(proof.ChallengeID, proof.Code))) {
		return errMailValidation
	}
	return nil
}
func (a *API) consumeProof(ctx context.Context, q db.Queryer, proof mailProof, owner, purpose, email string) error {
	res, e := q.ExecContext(ctx, `UPDATE email_challenges SET consumed=1 WHERE id=? AND owner=? AND purpose=? AND email=? AND digest=? AND consumed=0 AND attempts<=5 AND expires_at>?`, proof.ChallengeID, owner, purpose, email, a.digestCode(proof.ChallengeID, proof.Code), a.nowUTC())
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errMailValidation
	}
	return nil
}
func (a *API) handleBindEmail(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Email           string    `json:"email"`
		CurrentPassword string    `json:"current_password"`
		Proof           mailProof `json:"proof"`
		OldProof        mailProof `json:"old_proof"`
	}
	if decode(r, &in) != nil || !validEmail(normalizeEmail(in.Email)) {
		writeErr(w, 400, "invalid_request", "邮箱无效")
		return
	}
	in.Email = normalizeEmail(in.Email)
	if !a.verifyCurrent(r, p.UserID, in.CurrentPassword) {
		writeErr(w, 400, "invalid_request", "当前密码无效")
		return
	}
	var old string
	_ = a.DB.QueryRowContext(r.Context(), `SELECT email FROM email_identities WHERE owner=?`, p.UserID).Scan(&old)
	if a.checkProof(r.Context(), in.Proof, p.UserID, "bind", in.Email) != nil || (old != "" && a.checkProof(r.Context(), in.OldProof, p.UserID, "old_email", old) != nil) {
		writeErr(w, 400, "verification_failed", errMailValidation.Error())
		return
	}
	e := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if e := a.consumeProof(r.Context(), tx, in.Proof, p.UserID, "bind", in.Email); e != nil {
			return e
		}
		if old != "" {
			if e := a.consumeProof(r.Context(), tx, in.OldProof, p.UserID, "old_email", old); e != nil {
				return e
			}
		}
		// Delete is conditional on the old identity to reject concurrent address changes.
		if old != "" {
			res, e := tx.ExecContext(r.Context(), `DELETE FROM email_identities WHERE owner=? AND email=?`, p.UserID, old)
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			if n != 1 {
				return errMailValidation
			}
		}
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO email_identities(email,owner,verified_at) VALUES(?,?,?)`, in.Email, p.UserID, a.nowUTC()); e != nil {
			return errMailValidation
		}
		if _, e := tx.ExecContext(r.Context(), `UPDATE users SET email=? WHERE id=? AND household_id=?`, in.Email, p.UserID, p.HouseholdID); e != nil {
			return e
		}
		if old != "" {
			if e := a.enqueueMail(r.Context(), tx, p.HouseholdID, p.UserID, old, "security", "TinyNest 邮箱已变更", "你的账号邮箱已变更。如非本人操作，请联系站点管理员。", a.now().UTC().Add(24*time.Hour).Format(time.RFC3339)); e != nil {
				return e
			}
		}
		return audit(r.Context(), tx, p, "verify_email", "user", p.UserID, "{}")
	})
	if e != nil {
		writeErr(w, 409, "verification_failed", "验证失败或邮箱已被绑定，请重新验证")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "verified"})
}
func (a *API) handleEmailReset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email       string    `json:"email"`
		NewPassword string    `json:"new_password"`
		Proof       mailProof `json:"proof"`
	}
	if decode(r, &in) != nil || len(in.NewPassword) < 12 || len(in.NewPassword) > 256 {
		writeErr(w, 400, "invalid_request", "新密码须为12至256位")
		return
	}
	email := normalizeEmail(in.Email)
	var owner string
	_ = a.DB.QueryRowContext(r.Context(), `SELECT owner FROM email_identities WHERE email=?`, email).Scan(&owner)
	if a.checkProof(r.Context(), in.Proof, owner, "reset", email) != nil {
		writeErr(w, 400, "verification_failed", errMailValidation.Error())
		return
	}
	hash, e := auth.HashPassword(in.NewPassword)
	if e != nil {
		internalErr(w, e)
		return
	}
	e = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if e := a.consumeProof(r.Context(), tx, in.Proof, owner, "reset", email); e != nil {
			return e
		}
		var hh, username string
		if e := tx.QueryRowContext(r.Context(), `SELECT household_id,username FROM users WHERE id=? AND status='active'`, owner).Scan(&hh, &username); e != nil {
			return e
		}
		var current string
		if e := tx.QueryRowContext(r.Context(), `SELECT email FROM email_identities WHERE owner=?`, owner).Scan(&current); e != nil || current != email {
			return errMailValidation
		}
		if _, e := tx.ExecContext(r.Context(), `UPDATE users SET password_hash=? WHERE id=? AND status='active'`, hash, owner); e != nil {
			return e
		}
		return a.passwordSecurity(r.Context(), tx, Principal{UserID: owner, HouseholdID: hh, Username: username}, email)
	})
	if e != nil {
		writeErr(w, 400, "verification_failed", errMailValidation.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (a *API) passwordSecurity(ctx context.Context, q db.Queryer, p Principal, email string) error {
	if _, e := q.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, a.nowUTC(), p.UserID); e != nil {
		return e
	}
	if _, e := q.ExecContext(ctx, `UPDATE email_challenges SET consumed=1 WHERE owner=?`, p.UserID); e != nil {
		return e
	}
	if e := a.enqueueMail(ctx, q, p.HouseholdID, p.UserID, email, "security", "TinyNest 密码已修改", "你的账号密码已修改，所有登录会话已退出。如非本人操作，请联系站点管理员。", a.now().UTC().Add(24*time.Hour).Format(time.RFC3339)); e != nil {
		return e
	}
	return audit(ctx, q, p, "password_security_reset", "user", p.UserID, "{}")
}
func (a *API) enqueueMail(ctx context.Context, q db.Queryer, hh, user, to, kind, subject, body, expires string, refs ...string) error {
	parts := []string{subject, body}
	parts = append(parts, refs...)
	raw, _ := json.Marshal(parts)
	encrypted, e := a.mailCrypt(string(raw), false)
	if e != nil {
		return e
	}
	_, e = q.ExecContext(ctx, `INSERT INTO mail_queue(id,household_id,user_id,recipient,kind,payload,status,due_at,expires_at,created_at) VALUES(?,?,?,?,?,?,'queued',?,?,?)`, newID("mail"), hh, user, to, kind, encrypted, a.nowUTC(), expires, a.nowUTC())
	return e
}
