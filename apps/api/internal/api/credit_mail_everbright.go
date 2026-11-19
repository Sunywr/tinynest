package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tinynest/api/internal/credit"
)

type everbrightPreviewStatement struct {
	ReceivedOn   string                         `json:"received_on"`
	Transactions []credit.EverbrightTransaction `json:"transactions"`
	Error        string                         `json:"error,omitempty"`
}

func (a *API) previewEverbrightMail(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx, cancel := context.WithTimeout(r.Context(), 24*time.Second)
	defer cancel()
	var host, user, folder, cipher, mailboxID, keyword, tail string
	err := a.DB.QueryRowContext(ctx, `SELECT m.host,m.username,m.folder,m.password_cipher,m.id,b.subject_keyword,b.card_tail
  FROM credit_mail_bindings b
  JOIN credit_mailboxes m ON m.id=b.mailbox_id AND m.household_id=b.household_id
  JOIN credit_instruments i ON i.id=b.instrument_id AND i.household_id=b.household_id
  WHERE b.household_id=? AND b.instrument_id=? AND m.enabled=1 AND i.status='active'
  AND (i.issuer LIKE '%光大%' OR i.name LIKE '%光大%')`, p.HouseholdID, r.PathValue("id")).
		Scan(&host, &user, &folder, &cipher, &mailboxID, &keyword, &tail)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "missing_binding", "请选择已绑定邮箱的光大信用卡")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	password, err := creditMailboxCrypt(a.opts.CreditMailKey, p.HouseholdID, mailboxID, cipher, true)
	if err != nil {
		writeErr(w, 503, "mail_key_unavailable", "无法读取授权码，请重新测试并保存邮箱")
		return
	}
	statements, err := readEverbrightPreview(ctx, host, user, password, folder, keyword, tail)
	if err != nil {
		writeErr(w, 502, "credit_mail_preview_failed", "读取账单失败，请检查邮箱连接后重试")
		return
	}
	writeJSON(w, 200, map[string]any{"statements": statements})
}

var imapFetchUID = regexp.MustCompile(`(?i)\bUID ([0-9]+)\b`)
var imapLiteralLength = regexp.MustCompile(`\{([0-9]+)\}\r?\n$`)

func readEverbrightPreview(ctx context.Context, host, username, password, folder, keyword, tail string) ([]everbrightPreviewStatement, error) {
	user, err := quoteIMAPProbeValue(username)
	if err != nil {
		return nil, err
	}
	secret, err := quoteIMAPProbeValue(password)
	if err != nil {
		return nil, err
	}
	mailbox, err := quoteIMAPProbeValue(encodeIMAPFolder(folder))
	if err != nil {
		return nil, err
	}
	conn, err := dialCreditIMAP(ctx, host)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	reader := bufio.NewReaderSize(conn, 128<<10)
	if err := openCreditIMAPFolder(conn, reader, user, secret, mailbox); err != nil {
		return nil, err
	}
	var seq int
	command := func(value string, observe func(string, []byte)) error {
		seq++
		tag := fmt.Sprintf("B%04d", seq)
		if _, err := fmt.Fprintf(conn, "%s %s\r\n", tag, value); err != nil {
			return err
		}
		for range 512 {
			buf, err := reader.ReadSlice('\n')
			if err != nil {
				return err
			}
			line := string(buf)
			if strings.HasPrefix(line, tag+" ") {
				if strings.HasPrefix(strings.ToUpper(line), tag+" OK ") {
					return nil
				}
				return errors.New("IMAP command was rejected")
			}
			if !strings.HasPrefix(line, "* ") {
				return errors.New("invalid IMAP response")
			}
			var literal []byte
			if match := imapLiteralLength.FindStringSubmatch(line); match != nil {
				n, err := strconv.Atoi(match[1])
				if err != nil || n > 3<<20 {
					return errors.New("IMAP message exceeds preview limit")
				}
				literal = make([]byte, n)
				if _, err = io.ReadFull(reader, literal); err != nil {
					return err
				}
				suffix, err := reader.ReadSlice('\n')
				if err != nil {
					return err
				}
				line += string(suffix)
			}
			observe(line, literal)
		}
		return errors.New("too many IMAP response lines")
	}
	var uids []string
	if err := command("UID SEARCH ALL", func(line string, _ []byte) {
		if strings.HasPrefix(strings.ToUpper(line), "* SEARCH ") {
			uids = append(uids, strings.Fields(line)[2:]...)
		}
	}); err != nil {
		return nil, err
	}
	if len(uids) > 10000 {
		return nil, errors.New("mail folder exceeds preview limit")
	}
	for _, uid := range uids {
		if uid == "" || strings.Trim(uid, "0123456789") != "" {
			return nil, errors.New("invalid IMAP UID")
		}
	}
	var matches []string
	for start := 0; start < len(uids); start += 100 {
		batch := uids[start:min(start+100, len(uids))]
		err := command("UID FETCH "+strings.Join(batch, ",")+" (UID BODY.PEEK[HEADER.FIELDS (FROM SUBJECT)])", func(line string, body []byte) {
			if len(body) == 0 {
				return
			}
			uid := imapFetchUID.FindStringSubmatch(line)
			msg, err := mail.ReadMessage(bytes.NewReader(body))
			if uid != nil && err == nil && credit.MatchEverbrightStatementHeader(msg.Header.Get("From"), msg.Header.Get("Subject"), keyword) {
				matches = append(matches, uid[1])
			}
		})
		if err != nil {
			return nil, err
		}
	}
	if len(matches) > 120 {
		return nil, errors.New("too many matched statement messages")
	}
	statements := make([]everbrightPreviewStatement, 0, len(matches))
	seen := make(map[[32]byte]bool)
	for _, uid := range matches {
		item := everbrightPreviewStatement{Error: "邮件没有可解析的光大账单正文"}
		var duplicate bool
		err := command("UID FETCH "+uid+" (BODY.PEEK[])", func(_ string, body []byte) {
			if len(body) == 0 {
				return
			}
			msg, err := mail.ReadMessage(bytes.NewReader(body))
			if err != nil || !credit.MatchEverbrightStatementHeader(msg.Header.Get("From"), msg.Header.Get("Subject"), keyword) {
				return
			}
			received, err := msg.Header.Date()
			if err != nil {
				return
			}
			received = received.In(time.FixedZone("Asia/Shanghai", 8*3600))
			item.ReceivedOn = received.Format(time.DateOnly)
			html, err := everbrightMIMEHTML(mimeHeader(msg.Header), msg.Body, 0)
			if err != nil {
				return
			}
			sum := sha256.Sum256([]byte(html))
			if seen[sum] {
				duplicate = true
				return
			}
			seen[sum] = true
			rows, err := credit.ParseEverbrightHTML(html, received, tail)
			if err != nil {
				item.Error = "账单明细格式或日期未通过校验"
				return
			}
			if len(rows) == 0 {
				item.Error = "账单中没有匹配卡尾号的明细"
				return
			}
			item.Transactions = rows
			item.Error = ""
		})
		if err != nil {
			return nil, err
		}
		if !duplicate {
			statements = append(statements, item)
		}
	}
	return statements, nil
}

type mimeHeader map[string][]string

func (h mimeHeader) get(key string) string {
	values := h[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func everbrightMIMEHTML(header mimeHeader, body io.Reader, depth int) (string, error) {
	if depth > 6 {
		return "", errors.New("MIME nesting limit")
	}
	mediaType, params, err := mime.ParseMediaType(header.get("Content-Type"))
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		parts := multipart.NewReader(body, params["boundary"])
		for range 32 {
			part, err := parts.NextRawPart()
			if err != nil {
				return "", err
			}
			html, err := everbrightMIMEHTML(mimeHeader(part.Header), part, depth+1)
			part.Close()
			if err == nil {
				return html, nil
			}
		}
		return "", errors.New("MIME part limit")
	}
	if mediaType != "text/html" || strings.Contains(strings.ToLower(header.get("Content-Disposition")), "attachment") {
		return "", errors.New("not an HTML body")
	}
	switch strings.ToLower(header.get("Content-Transfer-Encoding")) {
	case "base64":
		body = base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		body = quotedprintable.NewReader(body)
	}
	body, err = credit.EverbrightCharsetReader(params["charset"], body)
	if err != nil {
		return "", err
	}
	raw, err := io.ReadAll(io.LimitReader(body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return "", errors.New("HTML body read failed")
	}
	return string(raw), nil
}
