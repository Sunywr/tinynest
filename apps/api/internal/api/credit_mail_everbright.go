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
	Review       []everbrightRowReview          `json:"review,omitempty"`
	Error        string                         `json:"error,omitempty"`
	Amounts      *credit.StatementAmounts       `json:"amounts,omitempty"`
	Provision    string                         `json:"provision,omitempty"`
	UseAmountDue bool                           `json:"use_amount_due,omitempty"`
}

func (a *API) previewEverbrightMail(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from != "" || to != "" {
		start, startErr := time.Parse(time.DateOnly, from)
		end, endErr := time.Parse(time.DateOnly, to)
		if startErr != nil || endErr != nil || end.Before(start) {
			writeErr(w, 400, "invalid_statement_range", "请选择有效的账单日期范围")
			return
		}
	}
	p, _ := principal(r)
	ctx, cancel := context.WithTimeout(r.Context(), 24*time.Second)
	defer cancel()
	var host, user, folder, cipher, mailboxID, keyword, tail, name, issuer string
	err := a.DB.QueryRowContext(ctx, `SELECT m.host,m.username,m.folder,m.password_cipher,m.id,b.subject_keyword,b.card_tail,i.name,COALESCE(i.issuer,'')
  FROM credit_mail_bindings b
  JOIN credit_mailboxes m ON m.id=b.mailbox_id AND m.household_id=b.household_id
  JOIN credit_instruments i ON i.id=b.instrument_id AND i.household_id=b.household_id
  WHERE b.household_id=? AND b.instrument_id=? AND m.enabled=1 AND i.status='active' AND i.kind='credit_card'`, p.HouseholdID, r.PathValue("id")).
		Scan(&host, &user, &folder, &cipher, &mailboxID, &keyword, &tail, &name, &issuer)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "missing_binding", "请选择已绑定邮箱的信用卡")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	card := r.PathValue("id")
	bank := statementBank(issuer, name)
	if bank == "" {
		writeErr(w, 400, "unsupported_statement_bank", "该银行尚未支持邮件明细解析")
		return
	}
	saved, err := readSavedCreditPreview(ctx, a.DB, p.HouseholdID, card)
	if err != nil {
		internalErr(w, err)
		return
	}
	password, err := creditMailboxCrypt(a.opts.CreditMailKey, p.HouseholdID, mailboxID, cipher, true)
	if err != nil {
		writeErr(w, 503, "mail_key_unavailable", "无法读取授权码，请重新测试并保存邮箱")
		return
	}
	statements, err := readBankPreview(ctx, bank, host, user, password, folder, keyword, tail)
	if err != nil {
		writeErr(w, 502, "credit_mail_preview_failed", "读取账单失败，请检查邮箱连接后重试")
		return
	}
	if bank == "spdb" {
		var start, end, due, offset int
		var bindings int
		err = a.DB.QueryRowContext(ctx, `SELECT i.cycle_start_day,i.cycle_end_day,i.due_day,i.due_month_offset,
			(SELECT COUNT(*) FROM credit_mail_bindings b JOIN credit_instruments other
			 ON other.id=b.instrument_id AND other.household_id=b.household_id
			 WHERE b.household_id=i.household_id AND b.mailbox_id=? AND other.status='active'
			 AND (other.name LIKE '%浦发%' OR other.issuer LIKE '%浦发%' OR other.name LIKE '%浦东发展%' OR other.issuer LIKE '%浦东发展%'))
			 FROM credit_instruments i WHERE i.id=? AND i.household_id=?`, mailboxID, card, p.HouseholdID).
			Scan(&start, &end, &due, &offset, &bindings)
		if err != nil {
			internalErr(w, err)
			return
		}
		if bindings == 1 {
			for i := range statements {
				enableSPDBAmountDue(&statements[i], start, end, due, offset)
			}
		}
	}
	// Missing or unreadable mail should not discard an existing usable preview.
	present := make(map[string]bool)
	for _, statement := range statements {
		if statement.Error == "" {
			present[statement.ReceivedOn] = true
		}
	}
	for _, statement := range saved.Statements {
		workbook := (bank == "spdb" || bank == "citic") && len(statement.Transactions) > 0
		if statement.Error == "" && (workbook || !present[statement.ReceivedOn]) {
			statement.Review = nil
			if statement.Provision == "eligible" {
				statement.Provision = "review"
			}
			statements = append(statements, statement)
		}
	}
	if err := a.reviewEverbrightStatements(ctx, p.HouseholdID, card, statements); err != nil {
		internalErr(w, err)
		return
	}
	saved.Statements = statements
	saved.SynchronizedAt = a.nowUTC()
	err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := readSavedCreditPreview(ctx, tx, p.HouseholdID, card)
		if err != nil {
			return err
		}
		if current.Generation != saved.Generation {
			return errConflict
		}
		return a.writeSavedCreditPreview(ctx, tx, p.HouseholdID, card, saved)
	})
	if errors.Is(err, errConflict) {
		writeErr(w, 409, "preview_changed", "预览已变更，请重新打开后再同步")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, saved)
}

func enableSPDBAmountDue(item *everbrightPreviewStatement, start, end, due, offset int) {
	if item.Amounts == nil || item.Amounts.NewCharges != nil || item.Amounts.AmountDue == nil || *item.Amounts.AmountDue <= 0 {
		return
	}
	if item.Amounts.StatementDate == "" {
		received, err := time.Parse(time.DateOnly, item.ReceivedOn)
		if err != nil {
			return
		}
		cycle, err := credit.CycleFor(received.AddDate(0, 0, -3), start, end, due, offset)
		if err != nil || cycle.End.Before(received.AddDate(0, 0, -3)) || cycle.End.After(received) {
			return
		}
		item.Amounts.StatementDate = cycle.End.Format(time.DateOnly)
	}
	item.UseAmountDue = true
	item.Provision = "eligible"
}

var imapFetchUID = regexp.MustCompile(`(?i)\bUID ([0-9]+)\b`)
var imapLiteralLength = regexp.MustCompile(`\{([0-9]+)\}\r?\n$`)

func statementBank(issuer, name string) string {
	for _, text := range []string{issuer, name} {
		for _, item := range []struct{ name, key string }{{"光大", "ceb"}, {"广发", "gdb"}, {"交通", "comm"}, {"招商", "cmb"}, {"浦发", "spdb"}, {"浦东发展", "spdb"}, {"中信", "citic"}} {
			if strings.Contains(text, item.name) {
				return item.key
			}
		}
	}
	return ""
}

func bankStatementIMAPSearch(bank string) string {
	switch bank {
	case "citic":
		// All accepted CITIC sender domains contain this ASCII substring.
		// Fetched headers still undergo exact domain and subject validation.
		return `UID SEARCH FROM "citic"`
	case "spdb":
		// The old and new statement addresses share this local part.
		return `UID SEARCH FROM "estmtservice"`
	default:
		return "UID SEARCH ALL"
	}
}

func readBankPreview(ctx context.Context, bank, host, username, password, folder, keyword, tail string) ([]everbrightPreviewStatement, error) {
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
	collectUIDs := func(line string, _ []byte) {
		if strings.HasPrefix(strings.ToUpper(line), "* SEARCH ") {
			uids = append(uids, strings.Fields(line)[2:]...)
		}
	}
	if err := command(bankStatementIMAPSearch(bank), collectUIDs); err != nil {
		return nil, err
	}
	// Some providers index sender searches differently from received headers.
	if bank == "spdb" && len(uids) == 0 {
		if err := command("UID SEARCH ALL", collectUIDs); err != nil {
			return nil, err
		}
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
			if uid != nil && err == nil && credit.MatchBankStatementHeader(bank, msg.Header.Get("From"), msg.Header.Get("Subject"), keyword) {
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
	progress, _ := ctx.Value(creditMailProgressKey{}).(func(int, int))
	if progress != nil {
		progress(0, len(matches))
	}
	statements := make([]everbrightPreviewStatement, 0, len(matches))
	seen := make(map[[32]byte]bool)
	for start := 0; start < len(matches); start += 12 {
		batch := strings.Join(matches[start:min(start+12, len(matches))], ",")
		err := command("UID FETCH "+batch+" (BODY.PEEK[])", func(_ string, body []byte) {
			if len(body) == 0 {
				return
			}
			item := everbrightPreviewStatement{Error: "邮件没有可解析的账单正文"}
			duplicate := false
			defer func() {
				if !duplicate {
					statements = append(statements, item)
				}
			}()
			msg, err := mail.ReadMessage(bytes.NewReader(body))
			if err != nil || !credit.MatchBankStatementHeader(bank, msg.Header.Get("From"), msg.Header.Get("Subject"), keyword) {
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
			amounts := credit.ParseStatementAmounts(html)
			matchesCard := bank != "gdb" && credit.StatementSummaryMatchesCard(html, tail)
			if bank == "citic" {
				amounts, matchesCard = credit.ParseCITICStatementAmounts(html, tail)
			}
			if amounts.NewCharges != nil || amounts.AmountDue != nil {
				item.Amounts = &amounts
				item.Provision = "review"
				if matchesCard {
					item.Provision = "eligible"
				}
			}
			if bank == "spdb" || bank == "citic" {
				item.Error = "邮件中未识别到账单金额"
				if item.Amounts != nil {
					item.Error = ""
				}
				return
			}
			rows, err := credit.ParseBankStatement(bank, html, received, tail)
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
		if progress != nil {
			progress(min(start+12, len(matches)), len(matches))
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
