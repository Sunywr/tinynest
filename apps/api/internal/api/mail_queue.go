package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// StartMailWorker drains the encrypted mail queue on a short tick. The queue is
// the only path that talks to SMTP, so request handlers never block on a remote
// mail server.
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
		// An announcement queued before a preference change must not be delivered
		// after the recipient opted out.
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
		if e = a.reserveMail(ctx, kind, c, to); e != nil {
			if errors.Is(e, errRecipientDailyLimit) {
				_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='daily_limit',payload='' WHERE id=?`, id)
				return
			}
			_, _ = a.DB.ExecContext(ctx, `UPDATE mail_queue SET status='queued',due_at=? WHERE id=?`, a.now().UTC().Add(time.Minute).Format(time.RFC3339), id)
			return
		}

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
