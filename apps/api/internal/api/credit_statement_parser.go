package api

import (
	"bytes"
	"encoding/base64"
	"errors"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
	"tinynest/api/internal/money"
)

type statementMailPreview struct {
	// UID is the IMAP UID of the message the fields were parsed from. It is
	// carried to the sync path for source traceability and never serialised.
	UID         uint32       `json:"-"`
	ReceivedOn  string       `json:"received_on,omitempty"`
	PeriodStart string       `json:"period_start_on,omitempty"`
	StatementOn string       `json:"statement_on,omitempty"`
	DueOn       string       `json:"due_on,omitempty"`
	AmountDue   *money.Minor `json:"amount_due_minor,omitempty"`
	NeedsReview string       `json:"needs_review,omitempty"`
}

var (
	statementHTMLHidden   = regexp.MustCompile(`(?is)<(?:style|script)\b[^>]*>.*?</(?:style|script)\s*>`)
	statementHTMLBreak    = regexp.MustCompile(`(?i)</(?:td|th|tr|p|div)>|<p(?:\s[^>]*)?>|<br\s*/?>`)
	statementHTMLTag      = regexp.MustCompile(`(?s)<[^>]*>`)
	statementCardTail     = regexp.MustCompile(`(?:卡尾号|卡号(?:后|末)(?:四|4)位|卡号\s*[:：]\s*[0-9]{0,6}[*＊]{4,12})\s*[:：]?\s*([0-9]{4})(?:[^0-9]|$)`)
	statementDueLabel     = regexp.MustCompile(`(?:本期应还(?:款)?总(?:金)?额|本期应还款?金额|本期账单应还金额)\s*[:：]?\s*(?:人民币\s*|CNY\s*|[¥￥]\s*)?([+-]?(?:[0-9]{1,3}(?:,[0-9]{3})+|[0-9]+)(?:\.[0-9]{1,2})?)(?:\s|$)`)
	statementDateLayout   = `([0-9]{4}(?:[-/][0-9]{1,2}[-/][0-9]{1,2}|年[0-9]{1,2}月[0-9]{1,2}日))(?:[^0-9]|$)`
	statementClosingLabel = regexp.MustCompile(`(?:账单日期|账单日)\s*[:：]?\s*` + statementDateLayout)
	statementDueDateLabel = regexp.MustCompile(`(?:到期还款日|最后还款日|还款到期日)\s*[:：]?\s*` + statementDateLayout)
	statementCITICRow     = regexp.MustCompile(`[0-9]{4}-[0-9]{2}[*＊]{2}-[*＊]{4}-([0-9]{4})\s+CNY\s+(?:[-+0-9,.]+\s+){3}([-+0-9,.]+)\s+[-+0-9,.]+(?:\s|$)`)
	statementCommCard     = regexp.MustCompile(`交通银行个人信用卡\s*([0-9]{6}[*＊]{6}([0-9]{4}))(?:[^0-9]|$)`)
	statementCommCycle    = regexp.MustCompile(`账单周期\s*(?:Statement Cycle\s*)?([0-9]{4}/[0-9]{1,2}/[0-9]{1,2})\s*[-–至]\s*` + statementDateLayout)
	statementCommDue      = regexp.MustCompile(`到期还款日\s*[:：]?\s*(?:Payment Due Date\s*)?` + statementDateLayout)
	statementCommAmount   = regexp.MustCompile(`本期应还款\s*[:：]?\s*[￥¥]\s*([+-]?(?:[0-9]{1,3}(?:,[0-9]{3})+|[0-9]+)(?:\.[0-9]{1,2})?)(?:\s|$)`)
)

func parseStatementMail(raw []byte, bank, tail string, subjectKeywords ...string) (statementMailPreview, bool) {
	var out statementMailPreview
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil || !statementSenderMatches(msg.Header.Get("From"), bank) {
		return out, false
	}
	decoder := mime.WordDecoder{CharsetReader: statementCharsetReader}
	subject, err := decoder.DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		return out, false
	}
	// The verified sender identifies SPDB even when a generic credit-card
	// statement notice omits the bank name from its subject.
	keyword := ""
	if len(subjectKeywords) > 0 {
		keyword = subjectKeywords[0]
	}
	if keyword != "" {
		if !strings.Contains(strings.ToLower(subject), strings.ToLower(keyword)) {
			return out, false
		}
	} else if !strings.Contains(subject, "账单") || (bank != "spdb" && !statementBankMatches(subject, bank)) {
		return out, false
	}
	if received, err := msg.Header.Date(); err == nil {
		out.ReceivedOn = received.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format(time.DateOnly)
	}
	if len(raw) > maxStatementMailBytes {
		out.NeedsReview = "邮件超出预览大小"
		return out, true
	}
	parts := 32
	fields := map[string]string{}
	var inspectHTML func(string)
	if bank == "cmb" {
		inspectHTML = func(source string) { collectCMBSummaryFields(fields, source) }
	}
	body, err := statementMIMEText(msg.Header.Get("Content-Type"), msg.Header.Get("Content-Disposition"),
		msg.Header.Get("Content-Transfer-Encoding"), msg.Body, 0, &parts, inspectHTML)
	if err != nil || body == "" {
		out.NeedsReview = "邮件正文无法识别"
		return out, true
	}
	if len(fields) > 0 {
		return parseCMBHTMLSummary(fields, out), true
	}
	body = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, body)
	if bank == "comm" && strings.Contains(body, "账单周期") && statementCommCard.MatchString(body) {
		return parseCommSummary(out, body, tail), true
	}
	if bank == "gdb" || bank == "ceb" {
		compact := strings.Join(strings.Fields(body), " ")
		if bank == "gdb" {
			if _, rows, ok := strings.Cut(compact, gdbSummaryHeader); ok {
				return parseGDBSummary(out, compact, rows, tail), true
			}
		} else if _, rows, ok := strings.Cut(compact, cebSummaryHeader); ok {
			return parseCEBSummary(out, compact, rows, tail), true
		}
	}
	unattributed := !statementCardTail.MatchString(body)
	out.StatementOn = statementFindDate(statementClosingLabel, body)
	out.DueOn = statementFindDate(statementDueDateLabel, body)
	if bank == "citic" && (strings.Contains(body, "本期新增金额") || strings.Contains(body, "New Charge")) {
		// Recognize only a single CNY account row for the bound card.
		var matches [][]string
		for _, row := range statementCITICRow.FindAllStringSubmatch(body, -1) {
			if row[1] == tail {
				matches = append(matches, row)
			}
		}
		if len(matches) == 1 {
			out.AmountDue = statementMinor(matches[0][2])
			unattributed = false
		}
	} else if (statementHasOnlyCard(body, tail) || unattributed) &&
		!strings.ContainsAny(body, "$€") &&
		!strings.Contains(body, "美元") && !strings.Contains(strings.ToUpper(body), "USD") &&
		!strings.Contains(body, "港币") && !strings.Contains(strings.ToUpper(body), "HKD") && !strings.Contains(strings.ToUpper(body), "EUR") {
		for _, match := range statementDueLabel.FindAllStringSubmatch(body, -1) {
			value := statementMinor(match[1])
			if value == nil || (out.AmountDue != nil && *value != *out.AmountDue) {
				out.AmountDue = nil
				break
			}
			out.AmountDue = value
		}
	}
	if unattributed && out.AmountDue != nil {
		out.NeedsReview = "邮件已读出应还金额，但正文未列出卡号末四位，无法确认归属账单"
	} else if out.AmountDue == nil || out.StatementOn == "" {
		out.NeedsReview = "请核对卡号、账单日期和人民币应还金额"
	}
	return out, true
}

func parseCommSummary(out statementMailPreview, body, tail string) statementMailPreview {
	out.NeedsReview = "请核对交行卡尾、账单周期、还款日和人民币应还金额"
	cards := statementCommCard.FindAllStringSubmatch(body, -1)
	cycles := statementCommCycle.FindAllStringSubmatch(body, -1)
	amounts := statementCommAmount.FindAllStringSubmatch(body, -1)
	if len(cards) != 1 || cards[0][2] != tail || len(cycles) != 1 || len(amounts) != 1 {
		return out
	}
	start, startErr := time.Parse("2006/1/2", cycles[0][1])
	endText := strings.NewReplacer("/", "-", "年", "-", "月", "-", "日", "").Replace(cycles[0][2])
	end, endErr := time.Parse("2006-1-2", endText)
	if startErr != nil || endErr != nil || end.Before(start) {
		return out
	}
	out.PeriodStart = start.Format(time.DateOnly)
	out.StatementOn = end.Format(time.DateOnly)
	out.DueOn = statementFindDate(statementCommDue, body)
	out.AmountDue = statementMinor(amounts[0][1])
	if out.DueOn != "" && out.DueOn >= out.StatementOn && out.AmountDue != nil {
		out.NeedsReview = ""
	}
	return out
}

func statementSenderMatches(from, bank string) bool {
	parser := mail.AddressParser{WordDecoder: &mime.WordDecoder{CharsetReader: statementCharsetReader}}
	addr, err := parser.Parse(from)
	if err != nil {
		return false
	}
	address := strings.ToLower(addr.Address)
	switch bank {
	case "spdb":
		return address == "estmtservice@eb.spdbccc.com.cn" || address == "estmtservice@eb.spdb.com.cn"
	case "gdb":
		return address == "creditcardbill@cgbchina.com.cn" || address == "creditcard@cgbchina.com.cn"
	}
	_, domain, _ := strings.Cut(address, "@")
	switch bank {
	case "citic":
		switch domain {
		case "bill.citiccard.com", "citiccard.com", "citicibank.com", "ecitic.com", "citicbank.com":
			return true
		}
	case "comm":
		return domain == "bocomcc.com"
	case "ceb":
		return domain == "cardcenter.cebbank.com"
	case "cmb":
		return domain == "message.cmbchina.com"
	}
	return false
}

var statementBanks = []struct {
	bank, sender string
	names        []string
}{
	{"spdb", "estmtservice", []string{"浦发", "浦东发展"}},
	{"citic", "citic", []string{"中信"}},
	{"comm", "bocomcc.com", []string{"交通银行", "交通信用卡", "交行"}},
	{"gdb", "cgbchina.com.cn", []string{"广发"}},
	{"ceb", "cardcenter.cebbank.com", []string{"光大"}},
	{"cmb", "message.cmbchina.com", []string{"招商", "招行"}},
}

func statementBankMatches(text, bank string) bool {
	for _, issuer := range statementBanks {
		if issuer.bank == bank {
			for _, name := range issuer.names {
				if strings.Contains(text, name) {
					return true
				}
			}
			return false
		}
	}
	return false
}

func statementHasOnlyCard(text, tail string) bool {
	matches := statementCardTail.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if match[1] != tail {
			return false
		}
	}
	return len(matches) > 0
}

func statementMinor(value string) *money.Minor {
	n, err := money.ParseYuan(strings.ReplaceAll(value, ",", ""))
	if err != nil {
		return nil
	}
	return &n
}

func statementFindDate(pattern *regexp.Regexp, text string) string {
	found := ""
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		value := strings.NewReplacer("/", "-", "年", "-", "月", "-", "日", "").Replace(match[1])
		date, err := time.Parse("2006-1-2", value)
		if err != nil || (found != "" && found != date.Format(time.DateOnly)) {
			return ""
		}
		found = date.Format(time.DateOnly)
	}
	return found
}

func statementCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	encoding, err := htmlindex.Get(charset)
	if err != nil {
		return nil, err
	}
	return encoding.NewDecoder().Reader(input), nil
}

func statementMIMEText(contentType, disposition, transfer string, src io.Reader, depth int, parts *int, inspectHTML ...func(string)) (string, error) {
	*parts--
	if depth > 6 || *parts < 0 {
		return "", errors.New("statement MIME limit exceeded")
	}
	kind, params, err := mime.ParseMediaType(contentType)
	if contentType == "" {
		kind = "text/plain"
	} else if err != nil {
		return "", err
	}
	if d, p, _ := mime.ParseMediaType(disposition); d == "attachment" || p["filename"] != "" {
		return "", nil
	}
	if strings.HasPrefix(kind, "multipart/") {
		reader := multipart.NewReader(src, params["boundary"])
		var result []string
		for {
			part, err := reader.NextRawPart()
			if err == io.EOF {
				return strings.Join(result, "\n"), nil
			}
			if err != nil {
				return "", err
			}
			text, err := statementMIMEText(part.Header.Get("Content-Type"), part.Header.Get("Content-Disposition"),
				part.Header.Get("Content-Transfer-Encoding"), part, depth+1, parts, inspectHTML...)
			if err != nil {
				return "", err
			}
			if text != "" {
				result = append(result, text)
			}
		}
	}
	if kind != "text/plain" && kind != "text/html" {
		return "", nil
	}
	switch strings.ToLower(strings.TrimSpace(transfer)) {
	case "base64":
		src = base64.NewDecoder(base64.StdEncoding, src)
	case "quoted-printable":
		src = quotedprintable.NewReader(src)
	}
	if charset := params["charset"]; charset != "" && !strings.EqualFold(charset, "utf-8") {
		src, err = statementCharsetReader(charset, src)
		if err != nil {
			return "", err
		}
	}
	body, err := io.ReadAll(io.LimitReader(src, maxStatementMailBytes+1))
	if err != nil || len(body) > maxStatementMailBytes || !utf8.Valid(body) {
		return "", errors.New("invalid statement MIME text")
	}
	text := string(body)
	if kind == "text/html" {
		text = statementHTMLHidden.ReplaceAllString(text, "")
		if len(inspectHTML) > 0 && inspectHTML[0] != nil {
			inspectHTML[0](text)
		}
		text = statementHTMLBreak.ReplaceAllString(text, "\n")
		text = html.UnescapeString(statementHTMLTag.ReplaceAllString(text, ""))
	}
	return text, nil
}
