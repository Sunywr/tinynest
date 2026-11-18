package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

func (a *API) handleAnnouncements(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	if r.Method == "GET" {
		rows, e := a.DB.QueryContext(r.Context(), `SELECT status,COUNT(*) FROM mail_queue WHERE household_id=? AND kind='announcement' GROUP BY status`, p.HouseholdID)
		if e != nil {
			internalErr(w, e)
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var s string
			var n int
			if e = rows.Scan(&s, &n); e != nil {
				internalErr(w, e)
				return
			}
			items = append(items, map[string]any{"status": s, "count": n})
		}
		writeJSON(w, 200, map[string]any{"items": items})
		return
	}
	var in struct {
		Title         string `json:"title"`
		Body          string `json:"body"`
		Preview       bool   `json:"preview"`
		ExpectedCount int    `json:"expected_count"`
	}
	if decode(r, &in) != nil || len(in.Title) < 1 || len(in.Title) > 200 || len(in.Body) < 1 || len(in.Body) > 20000 {
		writeErr(w, 400, "invalid_request", "请填写标题和正文")
		return
	}
	a.mailMu.Lock()
	defer a.mailMu.Unlock()
	c, _, _, e := a.mailConfig(r.Context())
	if e != nil || !c.Enabled || len(a.opts.MailKey) != 32 {
		writeErr(w, 503, "mail_unavailable", "邮件服务未启用")
		return
	}
	count := 0
	e = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(r.Context(), `SELECT u.id,i.email FROM users u JOIN email_identities i ON i.owner=u.id LEFT JOIN mail_preferences p ON p.user_id=u.id WHERE u.household_id=? AND u.status='active' AND COALESCE(p.announcements,1)=1`, p.HouseholdID)
		if e != nil {
			return e
		}
		type recipient struct{ id, email string }
		list := []recipient{}
		for rows.Next() {
			var v recipient
			if e = rows.Scan(&v.id, &v.email); e != nil {
				rows.Close()
				return e
			}
			list = append(list, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		count = len(list)
		var used, ann int
		_ = tx.QueryRowContext(r.Context(), `SELECT total,announcements FROM mail_daily_usage WHERE day=?`, a.businessDate()).Scan(&used, &ann)
		var queued int
		if e = tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM mail_queue WHERE kind='announcement' AND status IN ('queued','sending') AND expires_at>?`, a.nowUTC()).Scan(&queued); e != nil {
			return e
		}
		if count+queued+used > c.DailyLimit || count+queued+ann > c.AnnouncementLimit {
			return errors.New("quota")
		}
		if in.Preview {
			return nil
		}
		if count != in.ExpectedCount {
			return errors.New("recipients changed")
		}
		for _, v := range list {
			if e = a.enqueueMail(r.Context(), tx, p.HouseholdID, v.id, v.email, "announcement", in.Title, in.Body+"\n\n可在小家设置中关闭公告邮件。", a.now().UTC().Add(24*time.Hour).Format(time.RFC3339)); e != nil {
				return e
			}
		}
		return audit(r.Context(), tx, p, "send_announcement", "mail_queue", "", `{}`)
	})
	if e != nil {
		writeErr(w, 409, "announcement_rejected", "额度不足或收件人已变化，请重新预览")
		return
	}
	writeJSON(w, 200, map[string]any{"recipient_count": count, "preview": in.Preview, "status": "queued"})
}
func (a *API) StartMailWorker(ctx context.Context) {
	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				a.processMail(ctx)
			}
		}
	}()
}
func (a *API) processMail(ctx context.Context) {
	a.mailMu.Lock()
	defer a.mailMu.Unlock()
	_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='queued' WHERE status='sending' AND due_at<?`, a.now().UTC().Add(-time.Minute).Format(time.RFC3339))
	_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='expired',payload='' WHERE status IN ('queued','sending') AND expires_at<=?`, a.nowUTC())
	c, password, _, e := a.mailConfig(ctx)
	if e != nil || !c.Enabled {
		return
	}
	password, e = a.mailCrypt(password, true)
	if e != nil {
		return
	}
	var id, hh, user, to, kind, payload string
	var attempts int
	e = a.DB.QueryRowContext(ctx, `SELECT id,household_id,user_id,recipient,kind,payload,attempts FROM mail_queue WHERE status='queued' AND due_at<=? AND expires_at>? ORDER BY CASE kind WHEN 'verification' THEN 0 WHEN 'security' THEN 1 ELSE 2 END,created_at LIMIT 1`, a.nowUTC(), a.nowUTC()).Scan(&id, &hh, &user, &to, &kind, &payload, &attempts)
	if e != nil {
		return
	}
	if kind == "announcement" {
		var n int
		e = a.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users u JOIN email_identities i ON i.owner=u.id LEFT JOIN mail_preferences p ON p.user_id=u.id WHERE u.id=? AND u.household_id=? AND u.status='active' AND i.email=? AND COALESCE(p.announcements,1)=1`, user, hh, to).Scan(&n)
		if e != nil {
			return
		}
		if n != 1 {
			_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='cancelled',payload='' WHERE id=?`, id)
			return
		}
	}
	res, e := a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='sending',due_at=? WHERE id=? AND status='queued'`, a.nowUTC(), id)
	if e != nil {
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return
	}
	if e = a.reserveMail(ctx, kind, c); e != nil {
		_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='queued',due_at=? WHERE id=?`, a.now().UTC().Add(time.Minute).Format(time.RFC3339), id)
		return
	}
	plain, e := a.mailCrypt(payload, true)
	var msg []string
	if e == nil {
		e = json.Unmarshal([]byte(plain), &msg)
	}
	if e == nil && len(msg) >= 2 && kind == "verification" {
		var active int
		if len(msg) != 3 {
			e = errors.New("missing challenge reference")
		} else {
			e = a.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_challenges WHERE id=? AND owner=? AND email=? AND consumed=0 AND expires_at>?`, msg[2], user, to, a.nowUTC()).Scan(&active)
			if e == nil && active != 1 {
				_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='expired',payload='' WHERE id=?`, id)
				return
			}
		}
	}
	if e == nil && len(msg) >= 2 {
		e = a.sendMail(ctx, c, password, to, msg[0], msg[1])
	} else {
		e = errors.New("mail payload invalid")
	}
	if e == nil {
		_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='smtp_accepted',payload='',attempts=attempts+1 WHERE id=?`, id)
		return
	}
	attempts++
	var delivery *mailDeliveryError
	if attempts >= 4 || (errors.As(e, &delivery) && delivery.permanent) {
		_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='failed',payload='',attempts=? WHERE id=?`, attempts, id)
	} else {
		_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='queued',attempts=?,due_at=? WHERE id=?`, attempts, a.now().UTC().Add(time.Duration(1<<attempts)*time.Minute).Format(time.RFC3339), id)
	}
}
