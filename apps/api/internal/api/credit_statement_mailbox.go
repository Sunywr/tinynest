package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const qqIMAPHost = "imap.qq.com"

var mailboxProbeSlots = make(chan struct{}, 4)

func (a *API) handleTestStatementMailbox(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		AuthCode string `json:"auth_code"`
		Version  *int64 `json:"version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if decode(r, &input) != nil {
		writeErr(w, 400, "invalid_request", "请填写 QQ 邮箱和授权码")
		return
	}
	if input.Version != nil {
		if input.Email != "" || input.AuthCode != "" || *input.Version < 1 {
			writeErr(w, 400, "invalid_request", "请提供有效的配置版本")
			return
		}
		a.testSavedStatementMailbox(w, r, *input.Version)
		return
	}
	email := strings.TrimSpace(input.Email)
	if !validQQMailboxCredentials(email, input.AuthCode) {
		writeErr(w, 400, "invalid_request", "请填写有效的 QQ 邮箱和授权码")
		return
	}

	if a.probeStatementMailbox(w, r, email, input.AuthCode) {
		writeJSON(w, 200, map[string]string{"status": "connected"})
	}
}

func (a *API) testSavedStatementMailbox(w http.ResponseWriter, r *http.Request, version int64) {
	p, _ := principal(r)
	config, err := loadStatementMailbox(r.Context(), a.DB, p)
	if err != nil {
		internalErr(w, err)
		return
	}
	if config.version != version || config.cipher == "" {
		writeStatementMailboxError(w, errConflict)
		return
	}
	code, err := a.mailCryptWithPurpose(config.cipher, true, statementMailboxPurpose(p, config))
	if err != nil || !validQQMailboxCredentials(config.email, code) {
		writeErr(w, 503, "mail_unavailable", "账单邮箱授权码不可用，请重新配置")
		return
	}
	if !a.probeStatementMailbox(w, r, config.email, code) {
		return
	}
	current, err := loadStatementMailbox(r.Context(), a.DB, p)
	if err != nil {
		internalErr(w, err)
		return
	}
	if current.version != version {
		writeStatementMailboxError(w, errConflict)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "connected"})
}

func validQQMailboxCredentials(email, authCode string) bool {
	return validIMAPCredential(email, 254) && validEmail(email) &&
		strings.HasSuffix(strings.ToLower(email), "@qq.com") && validIMAPCredential(authCode, 128)
}

func (a *API) probeStatementMailbox(w http.ResponseWriter, r *http.Request, email, authCode string) bool {
	p, _ := principal(r)
	limitKey := "statement-mailbox-test:" + p.UserID
	ip := a.clientIP(r)
	if allowed, _ := a.limiter.allow(limitKey, ip); !allowed {
		writeErr(w, 429, "rate_limited", "连接测试次数过多，请稍后重试")
		return false
	}
	select {
	case mailboxProbeSlots <- struct{}{}:
		defer func() { <-mailboxProbeSlots }()
	default:
		writeErr(w, 429, "rate_limited", "连接测试繁忙，请稍后重试")
		return false
	}
	probe := a.opts.StatementMailboxProbe
	if probe == nil {
		probe = testQQStatementMailbox
	}
	if err := probe(r.Context(), email, authCode); err != nil {
		a.limiter.failure(limitKey, ip)
		writeErr(w, 502, "mailbox_test_failed", "QQ 邮箱连接失败，请检查 IMAP 服务和授权码后重试")
		return false
	}
	return true
}

func validIMAPCredential(value string, maxBytes int) bool {
	if len(value) == 0 || len(value) > maxBytes {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '!' || value[i] > '~' {
			return false
		}
	}
	return true
}

func testQQStatementMailbox(ctx context.Context, email, authCode string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	dialer := tls.Dialer{Config: &tls.Config{
		ServerName: qqIMAPHost,
		MinVersion: tls.VersionTLS12,
	}}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(qqIMAPHost, "993"))
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	return examineStatementInbox(conn, email, authCode)
}

// The connection test only authenticates and opens INBOX read-only. It does
// not use the mailbox search or message-fetch protocol.
func examineStatementInbox(conn io.ReadWriter, email, authCode string) error {
	if !validIMAPCredential(email, 254) || !validIMAPCredential(authCode, 128) {
		return errors.New("invalid mailbox credentials")
	}
	reader := bufio.NewReaderSize(conn, 8192)
	line, err := reader.ReadSlice('\n')
	if err != nil || !imapStatus(string(line), "*", "OK") {
		return errors.New("IMAP greeting rejected")
	}

	quote := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	login := `LOGIN "` + quote.Replace(email) + `" "` + quote.Replace(authCode) + `"`
	if err := mailboxProbeCommand(conn, reader, "A1", login); err != nil {
		return err
	}
	return mailboxProbeCommand(conn, reader, "A2", "EXAMINE INBOX")
}

func mailboxProbeCommand(conn io.Writer, reader *bufio.Reader, tag, command string) error {
	if _, err := fmt.Fprintf(conn, "%s %s\r\n", tag, command); err != nil {
		return errors.New("IMAP command could not be sent")
	}
	for i := 0; i < 64; i++ {
		raw, err := reader.ReadSlice('\n')
		if err != nil {
			break
		}
		line := string(raw)
		if imapStatus(line, tag, "OK") {
			return nil
		}
		if !strings.HasPrefix(line, "* ") || imapStatus(line, "*", "BYE") {
			break
		}
	}
	return errors.New("IMAP command rejected")
}

func imapStatus(line, tag, status string) bool {
	first, rest, ok := strings.Cut(line, " ")
	if !ok || first != tag || !strings.HasSuffix(rest, "\r\n") {
		return false
	}
	word, _, _ := strings.Cut(strings.TrimSuffix(rest, "\r\n"), " ")
	return strings.EqualFold(word, status)
}
