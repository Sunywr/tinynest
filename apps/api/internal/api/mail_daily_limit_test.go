package api

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecipientDailyLimitSharedAndReset(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	now := time.Date(2026, 11, 18, 15, 59, 0, 0, time.UTC)
	a.opts.Clock = func() time.Time { return now }
	cfg := defaultMailConfig()
	cfg.DailyLimit = 100
	for i := 0; i < 3; i++ {
		if err := a.reserveMail(context.Background(), "verification", cfg, " Person@Example.com "); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.reserveMail(context.Background(), "security", cfg, "person@example.com"); err == nil {
		t.Fatal("fourth email accepted across purposes")
	}
	restarted := New(d, a.opts)
	if err := restarted.reserveMail(context.Background(), "announcement", cfg, "person@example.com"); err == nil {
		t.Fatal("restart bypassed recipient limit")
	}
	if err := a.reserveMail(context.Background(), "verification", cfg, "other@example.com"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err := a.reserveMail(context.Background(), "verification", cfg, "person@example.com"); err != nil {
		t.Fatal("Shanghai midnight did not reset", err)
	}
}
func TestRecipientDailyLimitConcurrentAndRollback(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	cfg := defaultMailConfig()
	var wg sync.WaitGroup
	var count atomic.Int32
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := New(d, a.opts)
			if other.reserveMail(context.Background(), "verification", cfg, "same@example.com") == nil {
				count.Add(1)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 3 {
		t.Fatalf("concurrent sends accepted=%d", count.Load())
	}
	cfg.DailyLimit = 3
	if a.reserveMail(context.Background(), "verification", cfg, "fresh@example.com") == nil {
		t.Fatal("global quota bypass")
	}
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM mail_recipient_daily_usage WHERE email='fresh@example.com'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("recipient reservation not rolled back", err)
	}
}
func TestRecipientLimitRejectsChallengeBeforeQueue(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	cfg := defaultMailConfig()
	cfg.Enabled = true
	raw, _ := json.Marshal(cfg)
	encrypted, _ := a.mailCrypt("fixture", false)
	if _, err := d.Exec(`INSERT INTO mail_configuration(id,version,config_json,password_cipher,updated_at) VALUES(1,1,?,?,?)`, string(raw), encrypted, a.nowUTC()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := a.reserveMail(context.Background(), "verification", cfg, "same@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	for _, purpose := range []string{"register", "reset"} {
		status, body := c.do("POST", "/api/v1/auth/mail/challenge", map[string]string{"email": "Same@Example.com", "purpose": purpose})
		if status != 429 || body["error"].(map[string]any)["code"] != "email_daily_limit" {
			t.Fatalf("%s status %d %v", purpose, status, body)
		}
	}
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM mail_queue`).Scan(&n)
	if n != 0 {
		t.Fatal("quota-exhausted challenge was queued")
	}
}

func TestRecipientLimitStopsQueuedDelivery(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	cfg := defaultMailConfig()
	cfg.Enabled = true
	raw, _ := json.Marshal(cfg)
	secret, _ := a.mailCrypt("fixture", false)
	if _, err := d.Exec(`INSERT INTO mail_configuration(id,version,config_json,password_cipher,updated_at) VALUES(1,1,?,?,?)`, string(raw), secret, a.nowUTC()); err != nil {
		t.Fatal(err)
	}
	sent := 0
	a.opts.MailSender = func(context.Context, MailConfig, string, string, string, string) error { sent++; return nil }
	for i := 0; i < 4; i++ {
		if err := a.enqueueMail(context.Background(), d, "hh", "user", "same@example.com", "security", "Notice", "Body", a.now().UTC().Add(time.Hour).Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		a.processMail(context.Background())
	}
	if sent != 3 {
		t.Fatalf("SMTP attempts=%d", sent)
	}
	var limited int
	if err := d.QueryRow(`SELECT COUNT(*) FROM mail_queue WHERE status='daily_limit' AND payload=''`).Scan(&limited); err != nil || limited != 1 {
		t.Fatal("exhausted queued email not stopped", err)
	}
}
