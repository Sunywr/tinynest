package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
	"tinynest/api/internal/auth"
	"tinynest/api/internal/db"
)

type MailConfig struct {
	Enabled           bool   `json:"enabled"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Username          string `json:"username"`
	From              string `json:"from"`
	Name              string `json:"name"`
	DailyLimit        int    `json:"daily_limit"`
	AnnouncementLimit int    `json:"announcement_limit"`
}

func defaultMailConfig() MailConfig {
	return MailConfig{Host: "", Port: 465, Username: "", From: "", Name: "TinyNest 小家", DailyLimit: 100, AnnouncementLimit: 20}
}
func validEmail(s string) bool {
	a, e := mail.ParseAddress(s)
	return e == nil && a.Address == s && len(s) <= 254 && !strings.ContainsAny(s, "\r\n")
}
func (a *API) mailCrypt(value string, decrypt bool) (string, error) {
	return a.mailCryptWithPurpose(value, decrypt, "tinynest-mail-v1")
}

func (a *API) mailCryptWithPurpose(value string, decrypt bool, purpose string) (string, error) {
	if len(a.opts.MailKey) != 32 {
		return "", errors.New("mail encryption key unavailable")
	}
	b, e := aes.NewCipher(a.opts.MailKey)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return "", e
	}
	if decrypt {
		data, e := base64.StdEncoding.DecodeString(value)
		if e != nil || len(data) < g.NonceSize() {
			return "", errors.New("invalid mail ciphertext")
		}
		p, e := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], []byte(purpose))
		return string(p), e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return "", e
	}
	return base64.StdEncoding.EncodeToString(g.Seal(n, n, []byte(value), []byte(purpose))), nil
}
func (a *API) mailConfig(ctx context.Context) (MailConfig, string, int, error) {
	c := defaultMailConfig()
	var raw, p string
	var v int
	e := a.DB.QueryRowContext(ctx, `SELECT config_json,password_cipher,version FROM mail_configuration WHERE id=1`).Scan(&raw, &p, &v)
	if errors.Is(e, sql.ErrNoRows) {
		return c, "", 0, nil
	}
	if e != nil {
		return c, "", v, e
	}
	e = json.Unmarshal([]byte(raw), &c)
	return c, p, v, e
}
func (a *API) siteAdmin(r *http.Request) bool {
	p, _ := principal(r)
	var n int
	return a.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM site_administrators WHERE user_id=?`, p.UserID).Scan(&n) == nil && n == 1
}
func (a *API) registerMail(m *http.ServeMux) {
	// Site SMTP configuration: one handler serves both GET (read) and POST (test
	// + save), exactly as before, but registered as two explicit routes.
	siteAdminOnly := func(next http.HandlerFunc) http.HandlerFunc {
		return a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
			if !a.siteAdmin(r) {
				writeErr(w, 403, "forbidden", "仅站点管理员可配置系统邮件")
				return
			}
			next(w, r)
		})
	}
	a.register(m, "GET /api/v1/site/mail", siteAdminOnly(a.handleMailConfig))
	a.register(m, "POST /api/v1/site/mail", siteAdminOnly(a.handleMailConfig))
	a.register(m, "GET /api/v1/auth/me/mail", a.requireAuth(a.handleMailIdentity))
	a.register(m, "POST /api/v1/auth/me/mail/preferences", a.requireAuth(a.handleMailPreferences))
	a.register(m, "POST /api/v1/auth/me/mail/challenge", a.requireAuth(a.handlePrivateChallenge))
	a.register(m, "POST /api/v1/auth/me/mail/confirm", a.requireAuth(a.handleBindEmail))
	a.register(m, "POST /api/v1/auth/mail/challenge", a.handlePublicChallenge)
	a.register(m, "POST /api/v1/auth/mail/reset", a.handleEmailReset)
}
func (a *API) handleMailConfig(w http.ResponseWriter, r *http.Request) {
	c, stored, version, e := a.mailConfig(r.Context())
	if e != nil {
		internalErr(w, e)
		return
	}
	if r.Method == "GET" {
		var total, ann int
		_ = a.DB.QueryRowContext(r.Context(), `SELECT total,announcements FROM mail_daily_usage WHERE day=?`, a.businessDate()).Scan(&total, &ann)
		var lastAction, lastAt string
		_ = a.DB.QueryRowContext(r.Context(), `SELECT action,created_at FROM audit_logs WHERE action IN ('mail_test_failed','mail_config_updated_test_accepted') ORDER BY id DESC LIMIT 1`).Scan(&lastAction, &lastAt)
		writeJSON(w, 200, map[string]any{"last_test": map[string]string{"result": lastAction, "at": lastAt}, "config": c, "password_configured": stored != "", "version": version, "used_today": total, "announcements_today": ann, "key_ready": len(a.opts.MailKey) == 32})
		return
	}
	var in struct {
		Config          MailConfig `json:"config"`
		Password        string     `json:"password"`
		CurrentPassword string     `json:"current_password"`
		TestRecipient   string     `json:"test_recipient"`
		Version         int        `json:"version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if decode(r, &in) != nil || !validSMTPHost(in.Config.Host) || (in.Config.Port != 465 && in.Config.Port != 587) || !validEmail(in.Config.From) || strings.TrimSpace(in.Config.Username) == "" || len(in.Config.Username) > 320 || strings.ContainsAny(in.Config.Username, "\r\n") || len(in.Config.Name) > 100 || strings.ContainsAny(in.Config.Name, "\r\n") || in.Config.DailyLimit < 1 || in.Config.AnnouncementLimit < 0 || in.Config.AnnouncementLimit > in.Config.DailyLimit || len(in.Password) > 1024 || !validEmail(in.TestRecipient) {
		writeErr(w, 400, "invalid_request", "请检查SMTP服务器、登录账号、发件地址、额度和测试收件地址")
		return
	}
	p, _ := principal(r)
	if !a.verifyCurrent(r, p.UserID, in.CurrentPassword) {
		writeErr(w, 400, "invalid_request", "当前密码无效或尝试过多")
		return
	}
	a.mailMu.Lock()
	defer a.mailMu.Unlock()
	_, stored, version, e = a.mailConfig(r.Context())
	if e != nil {
		internalErr(w, e)
		return
	}
	if version != in.Version {
		writeErr(w, 409, "conflict", "配置已更新，请刷新后重试")
		return
	}
	password := in.Password
	if password == "" {
		password, e = a.mailCrypt(stored, true)
	}
	if e != nil || password == "" {
		writeErr(w, 503, "mail_unavailable", "请配置加密密钥和SMTP密码")
		return
	}
	encrypted, e := a.mailCrypt(password, false)
	if e != nil {
		writeErr(w, 503, "mail_unavailable", "邮件加密密钥未配置")
		return
	}
	if e = a.reserveMail(r.Context(), "test", in.Config, in.TestRecipient); e != nil {
		if errors.Is(e, errRecipientDailyLimit) {
			writeErr(w, 429, "email_daily_limit", errRecipientDailyLimit.Error())
		} else {
			writeErr(w, 429, "mail_quota", "当日发信额度不足")
		}
		return
	}
	if e = a.sendMail(r.Context(), in.Config, password, in.TestRecipient, "TinyNest 邮件配置测试", "这是一封系统邮件配置测试邮件。"); e != nil {
		_ = audit(r.Context(), a.DB, p, "mail_test_failed", "mail_configuration", "1", "{}")
		writeErr(w, 502, "mail_test_failed", "测试失败，请检查SMTP凭据、服务状态和网络；旧配置保持生效")
		return
	}
	raw, _ := json.Marshal(in.Config)
	e = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if version == 0 {
			_, e = tx.ExecContext(r.Context(), `INSERT INTO mail_configuration(id,version,config_json,password_cipher,updated_at) VALUES(1,1,?,?,?)`, string(raw), encrypted, a.nowUTC())
		} else {
			_, e = tx.ExecContext(r.Context(), `UPDATE mail_configuration SET version=version+1,config_json=?,password_cipher=?,updated_at=? WHERE id=1`, string(raw), encrypted, a.nowUTC())
		}
		if e != nil {
			return e
		}
		return audit(r.Context(), tx, p, "mail_config_updated_test_accepted", "mail_configuration", "1", `{"fields":["config","credential"]}`)
	})
	if e != nil {
		internalErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"status": "smtp_accepted", "version": version + 1})
}
func (a *API) verifyCurrent(r *http.Request, id, password string) bool {
	if len(password) > 256 {
		return false
	}
	if ok, _ := a.limiter.allow("mail-password:"+id, a.clientIP(r)); !ok {
		return false
	}
	var hash string
	e := a.DB.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id=? AND status='active'`, id).Scan(&hash)
	if e != nil || !auth.VerifyPassword(hash, password) {
		a.limiter.failure("mail-password:"+id, a.clientIP(r))
		return false
	}
	return true
}
func (a *API) sendMail(ctx context.Context, c MailConfig, password, to, subject, body string) error {
	if a.opts.MailSender != nil {
		return a.opts.MailSender(ctx, c, password, to, subject, body)
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, e := d.DialContext(ctx, "tcp", net.JoinHostPort(c.Host, fmt.Sprint(c.Port)))
	if e != nil {
		return errors.New("smtp connection failed")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	t := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if c.Port == 465 {
		tc := tls.Client(conn, t)
		if e = tc.HandshakeContext(ctx); e != nil {
			return errors.New("smtp TLS failed")
		}
		conn = tc
	}
	client, e := smtp.NewClient(conn, c.Host)
	if e != nil {
		return safeSMTPError(e)
	}
	defer client.Close()
	if c.Port == 587 {
		if e = client.StartTLS(t); e != nil {
			return errors.New("smtp STARTTLS failed")
		}
	}
	if e = client.Auth(smtp.PlainAuth("", c.Username, password, c.Host)); e != nil {
		return safeSMTPError(e)
	}
	if e = client.Mail(c.From); e != nil {
		return safeSMTPError(e)
	}
	if e = client.Rcpt(to); e != nil {
		return safeSMTPError(e)
	}
	w, e := client.Data()
	if e != nil {
		return safeSMTPError(e)
	}
	msg := "From: " + (&mail.Address{Name: c.Name, Address: c.From}).String() + "\r\nTo: " + to + "\r\nSubject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n" + smtpBody(body) + "\r\n"
	if _, e = io.WriteString(w, msg); e != nil {
		return errors.New("smtp write failed")
	}
	if e = w.Close(); e != nil {
		return safeSMTPError(e)
	}
	return nil
}

var errRecipientDailyLimit = errors.New("每个邮箱每天最多接收3封邮件，请明天再试")

const recipientDailyLimit = 3

func (a *API) reserveMail(ctx context.Context, kind string, c MailConfig, recipients ...string) error {
	return a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		day := a.businessDate()
		q := `INSERT INTO mail_daily_usage(day) VALUES(?) ON CONFLICT(day) DO NOTHING`
		if a.DB.IsMySQL() {
			q = `INSERT IGNORE INTO mail_daily_usage(day) VALUES(?)`
		}
		if _, e := tx.ExecContext(ctx, q, day); e != nil {
			return e
		}

		if len(recipients) > 0 {
			email := normalizeEmail(recipients[0])
			q := `INSERT INTO mail_recipient_daily_usage(day,email) VALUES(?,?) ON CONFLICT(day,email) DO NOTHING`
			if a.DB.IsMySQL() {
				q = `INSERT IGNORE INTO mail_recipient_daily_usage(day,email) VALUES(?,?)`
			}
			if _, err := tx.ExecContext(ctx, q, day, email); err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, `UPDATE mail_recipient_daily_usage SET total=total+1 WHERE day=? AND email=? AND total<?`, day, email, recipientDailyLimit)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return errRecipientDailyLimit
			}
		}
		ann := 0
		if kind == "announcement" {
			ann = 1
		}
		result, e := tx.ExecContext(ctx, `UPDATE mail_daily_usage SET total=total+1,announcements=announcements+? WHERE day=? AND total<? AND (?=0 OR announcements<?)`, ann, day, c.DailyLimit, ann, c.AnnouncementLimit)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e == nil && n != 1 {
			return errors.New("quota exhausted")
		}
		return e
	})
}

func smtpBody(body string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	var b strings.Builder
	for len(encoded) > 76 {
		b.WriteString(encoded[:76])
		b.WriteString("\r\n")
		encoded = encoded[76:]
	}
	b.WriteString(encoded)
	return b.String()
}

type mailDeliveryError struct{ permanent bool }

func (e *mailDeliveryError) Error() string { return "SMTP投递失败" }
func safeSMTPError(err error) error {
	var protocol *textproto.Error
	return &mailDeliveryError{permanent: errors.As(err, &protocol) && protocol.Code >= 500 && protocol.Code <= 599}
}

// Includes outstanding messages so successful challenge responses do not promise
// mail that has no recipient budget. The sending transaction remains authoritative.
func (a *API) checkRecipientBudget(ctx context.Context, q db.Queryer, email string) error {
	var used, queued int
	if err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(total),0) FROM mail_recipient_daily_usage WHERE day=? AND email=?`, a.businessDate(), normalizeEmail(email)).Scan(&used); err != nil {
		return err
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM mail_queue WHERE recipient=? AND status IN ('queued','sending') AND expires_at>?`, normalizeEmail(email), a.nowUTC()).Scan(&queued); err != nil {
		return err
	}
	if used+queued >= recipientDailyLimit {
		return errRecipientDailyLimit
	}
	return nil
}

// Only host names or IP literals are accepted; URLs and host:port input are not.
// Ports remain explicitly bound to authenticated TLS submission (465/587).
func validSMTPHost(host string) bool {
	if host == "" || host != strings.TrimSpace(host) || len(host) > 253 {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
	}
	return true
}
