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
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
	"tinynest/api/internal/money"
)

type statementMailPreview struct {
	ReceivedOn  string       `json:"received_on,omitempty"`
	StatementOn string       `json:"statement_on,omitempty"`
	DueOn       string       `json:"due_on,omitempty"`
	AmountDue   *money.Minor `json:"amount_due_minor,omitempty"`
	NeedsReview string       `json:"needs_review,omitempty"`
}

var (
	statementHTMLHidden   = regexp.MustCompile(`(?is)<(?:style|script)\b[^>]*>.*?</(?:style|script)\s*>`)
	statementHTMLBreak    = regexp.MustCompile(`(?i)</(?:td|th|tr|p|div)>|<br\s*/?>`)
	statementHTMLTag      = regexp.MustCompile(`(?s)<[^>]*>`)
	statementCardTail     = regexp.MustCompile(`(?:卡尾号|卡号(?:后|末)(?:四|4)位|卡号\s*[:：]\s*[0-9]{0,6}[*＊]{4,12})\s*[:：]?\s*([0-9]{4})(?:[^0-9]|$)`)
	statementDueLabel     = regexp.MustCompile(`(?:本期应还(?:款)?总(?:金)?额|本期应还款金额|本期账单应还金额)\s*[:：]?\s*(?:人民币\s*|CNY\s*|[¥￥]\s*)?([+-]?(?:[0-9]{1,3}(?:,[0-9]{3})+|[0-9]+)(?:\.[0-9]{1,2})?)(?:\s|$)`)
	statementDateLayout   = `([0-9]{4}(?:[-/][0-9]{1,2}[-/][0-9]{1,2}|年[0-9]{1,2}月[0-9]{1,2}日))(?:[^0-9]|$)`
	statementClosingLabel = regexp.MustCompile(`(?:账单日期|账单日)\s*[:：]?\s*` + statementDateLayout)
	statementDueDateLabel = regexp.MustCompile(`(?:到期还款日|最后还款日|还款到期日)\s*[:：]?\s*` + statementDateLayout)
	statementCITICRow     = regexp.MustCompile(`[0-9]{4}-[0-9]{2}[*＊]{2}-[*＊]{4}-([0-9]{4})\s+CNY\s+(?:[-+0-9,.]+\s+){4}([-+0-9,.]+)(?:\s|$)`)
)

func parseStatementMail(raw []byte, bank, tail string) (statementMailPreview, bool) {
	var out statementMailPreview
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil || !statementSenderMatches(msg.Header.Get("From"), bank) {
		return out, false
	}
	decoder := mime.WordDecoder{CharsetReader: statementCharsetReader}
	subject, err := decoder.DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || !strings.Contains(subject, "账单") || !statementBankMatches(subject, bank) {
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
	body, err := statementMIMEText(msg.Header.Get("Content-Type"), msg.Header.Get("Content-Disposition"),
		msg.Header.Get("Content-Transfer-Encoding"), msg.Body, 0, &parts)
	if err != nil || body == "" {
		out.NeedsReview = "邮件正文无法识别"
		return out, true
	}
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
		}
	} else if statementHasOnlyCard(body, tail) &&
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
	if out.AmountDue == nil || out.StatementOn == "" {
		out.NeedsReview = "请核对卡号、账单日期和人民币应还金额"
	}
	return out, true
}

func statementSenderMatches(from, bank string) bool {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return false
	}
	address := strings.ToLower(addr.Address)
	if bank == "spdb" {
		return address == "estmtservice@eb.spdbccc.com.cn" || address == "estmtservice@eb.spdb.com.cn"
	}
	_, domain, _ := strings.Cut(address, "@")
	switch domain {
	case "bill.citiccard.com", "citiccard.com", "citicibank.com", "ecitic.com", "citicbank.com":
		return bank == "citic"
	}
	return false
}

func statementBankMatches(text, bank string) bool {
	if bank == "spdb" {
		return strings.Contains(text, "浦发") || strings.Contains(text, "浦东发展")
	}
	return bank == "citic" && strings.Contains(text, "中信")
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

func statementMIMEText(contentType, disposition, transfer string, src io.Reader, depth int, parts *int) (string, error) {
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
				part.Header.Get("Content-Transfer-Encoding"), part, depth+1, parts)
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
		text = statementHTMLBreak.ReplaceAllString(text, "\n")
		text = html.UnescapeString(statementHTMLTag.ReplaceAllString(text, ""))
	}
	return text, nil
}
