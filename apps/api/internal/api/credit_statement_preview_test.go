package api

import (
	"bufio"
	"encoding/base64"
	"mime"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestStatementMailPreviewParsesBankSummaries(t *testing.T) {
	for _, tc := range []struct {
		bank, sender, subject, body string
	}{
		{"spdb", "estmtservice@eb.spdbccc.com.cn", "浦发信用卡电子账单", `<table><tr><td>账单日期</td><td>2026年11月1日</td></tr>
		<tr><td>到期还款日</td><td>2026/11/25</td></tr><tr><td>卡号末四位</td><td>4321</td></tr>
		<tr><td>本期应还总额</td><td>人民币 1,234.56</td></tr></table>`},
		{"comm", "statement@bocomcc.com", "交通银行信用卡电子账单", `账单日: 2026-11-01<br>最后还款日: 2026-11-25<br>卡号末四位: 4321<br>本期应还金额: 人民币 1,234.56`},
		{"gdb", "creditcardbill@cgbchina.com.cn", "广发银行信用卡电子账单", `账单日: 2026-11-01<br>最后还款日: 2026-11-25<br>卡号末四位: 4321<br>本期应还金额: 人民币 1,234.56`},
		{"ceb", "statement@cardcenter.cebbank.com", "光大银行信用卡电子账单", `账单日: 2026-11-01<br>最后还款日: 2026-11-25<br>卡号末四位: 4321<br>本期应还金额: 人民币 1,234.56`},
		{"cmb", "statement@message.cmbchina.com", "招商银行信用卡电子账单", `账单日: 2026-11-01<br>最后还款日: 2026-11-25<br>卡号末四位: 4321<br>本期应还金额: 人民币 1,234.56`},
		{"citic", "statement@bill.citiccard.com", "中信信用卡电子账单", `账单日: 2026-11-01<br>最后还款日: 2026-11-25<br>
		本期新增金额 <table><tr><td>1234-56**-****-4321</td><td>CNY</td><td>50</td><td>20</td><td>10</td><td>1234.56</td><td>61.73</td></tr></table>`},
	} {
		t.Run(tc.bank, func(t *testing.T) {
			body, err := simplifiedchinese.GBK.NewEncoder().String(tc.body)
			if err != nil {
				t.Fatal(err)
			}
			raw := "From: " + tc.sender + "\r\nSubject: " + mime.BEncoding.Encode("UTF-8", tc.subject) +
				"\r\nContent-Type: text/html; charset=gbk\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
				base64.StdEncoding.EncodeToString([]byte(body))
			item, ok := parseStatementMail([]byte(raw), tc.bank, "4321")
			if !ok || item.StatementOn != "2026-11-01" || item.DueOn != "2026-11-25" ||
				item.AmountDue == nil || *item.AmountDue != 123456 || item.NeedsReview != "" {
				t.Fatalf("unexpected preview: %+v (%v)", item, ok)
			}
			wrong, ok := parseStatementMail([]byte(raw), tc.bank, "9999")
			if !ok || wrong.AmountDue != nil || wrong.NeedsReview == "" {
				t.Fatalf("unmatched card should remain pending: %+v (%v)", wrong, ok)
			}
			spoofed := strings.Replace(raw, tc.sender, tc.sender+".example.net", 1)
			if _, ok := parseStatementMail([]byte(spoofed), tc.bank, "4321"); ok {
				t.Fatal("unrecognized sender accepted")
			}
		})
	}
}

func TestStatementPreviewIMAPRequestsBoundedReadOnlyFetch(t *testing.T) {
	wire := strings.NewReader("* SEARCH 11 12\r\nS1 OK search complete\r\n* 1 FETCH (BODY[]<0> {5}\r\nhello UID 12)\r\nF1 OK fetch complete\r\n")
	var commands strings.Builder
	reader := bufio.NewReader(wire)
	uids, err := searchStatementMail(&commands, reader, "citic", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(uids) != 2 || uids[0] != 12 {
		t.Fatalf("search: %v %v", uids, err)
	}
	raw, err := fetchStatementMail(&commands, reader, uids[0])
	if err != nil || string(raw) != "hello" {
		t.Fatalf("fetch: %q %v", raw, err)
	}
	want := "S1 UID SEARCH SINCE 01-Nov-2026 FROM \"citic\"\r\nF1 UID FETCH 12 (UID BODY.PEEK[]<0.524289>)\r\n"
	if commands.String() != want {
		t.Fatalf("unexpected commands: %q", commands.String())
	}
}

func TestStatementPreviewRequiresOwnedBinding(t *testing.T) {
	server, client, database := newTestServer(t)
	defer database.Close()
	defer server.Close()
	card := client.mustOK("POST", "/api/v1/accounts", map[string]string{"kind": "credit", "name": "浦发信用卡(4321)"})
	path := "/api/v1/accounts/" + card["id"].(string) + "/credit-statement-preview?version=1&provider=163"
	if status, _ := client.do("POST", path, nil); status != http.StatusNotFound {
		t.Fatalf("unbound preview got HTTP %d, want 404", status)
	}
}

func TestSPDBGenericStatementNoticeRemainsVisibleAfterClosingDate(t *testing.T) {
	raw := []byte("From: estmtservice@eb.spdb.com.cn\r\nSubject: " +
		mime.BEncoding.Encode("UTF-8", "您的电子账单已出") +
		"\r\nDate: Sun, 08 Nov 2026 09:00:00 +0800\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n请验证后查看账单详情")
	item, ok := parseStatementMail(raw, "spdb", "5296")
	if !ok || item.NeedsReview == "" || item.AmountDue != nil {
		t.Fatalf("expected review-only official notice: %+v, %v", item, ok)
	}
	period := statementDateRange{
		start: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
		end:   time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC),
	}
	if !period.matchesMail(item) {
		t.Fatal("an unparsed notice delivered just after closing should remain visible")
	}
	item.StatementOn = "2026-11-08"
	if period.matchesMail(item) {
		t.Fatal("known statement date cannot be assigned to an adjacent period")
	}
}

func TestStatementSearchCanOmitSenderForDateFallback(t *testing.T) {
	var command strings.Builder
	reader := bufio.NewReader(strings.NewReader("* SEARCH 12\r\nS1 OK search complete\r\n"))
	day := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	ids, err := searchStatementMail(&command, reader, "", day, day)
	if err != nil || len(ids) != 1 || strings.Contains(command.String(), "FROM") {
		t.Fatalf("expected date-only lookup, got %q, %v, %v", command.String(), ids, err)
	}
}

func TestSPDBSummaryWithoutCardIdentityShowsAmountForReview(t *testing.T) {
	raw := []byte("From: estmtservice@eb.spdb.com.cn\r\nSubject: " +
		mime.BEncoding.Encode("UTF-8", "浦发银行-信用卡电子账单") +
		"\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		"<div>请点击查看电子对账单。<br>本期应还款总额：&nbsp;<strong>￥123.45</strong>" +
		"<br>本期最低还款额：￥10.00<br>到期还款日：&nbsp;2026/11/27</div>")
	item, ok := parseStatementMail(raw, "spdb", "4321")
	if !ok || item.AmountDue == nil || *item.AmountDue != 12345 || item.DueOn != "2026-11-27" {
		t.Fatalf("missing preview summary: %+v (%v)", item, ok)
	}
	if item.StatementOn != "" || item.NeedsReview == "" || statementSummaryReady(item) {
		t.Fatalf("unattributed summary must remain review-only: %+v", item)
	}
}

func TestBankSummaryWithoutCardIdentityIsPreviewOnly(t *testing.T) {
	for _, fixture := range []struct{ bank, sender, subject string }{
		{"citic", "statement@bill.citiccard.com", "中信信用卡电子账单"},
		{"comm", "statement@bocomcc.com", "交通银行信用卡电子账单"},
		{"gdb", "creditcardbill@cgbchina.com.cn", "广发银行信用卡电子账单"},
		{"ceb", "statement@cardcenter.cebbank.com", "光大银行信用卡电子账单"},
		{"cmb", "statement@message.cmbchina.com", "招商银行信用卡电子账单"},
	} {
		t.Run(fixture.bank, func(t *testing.T) {
			headers := "From: " + fixture.sender + "\r\nSubject: " +
				mime.BEncoding.Encode("UTF-8", fixture.subject) + "\r\nContent-Type: text/html; charset=utf-8\r\n\r\n"
			summary := "<div>账单日期：2026/11/07<br>本期应还款总额：&nbsp;￥123.45<br>本期最低还款额：￥10.00<br>到期还款日：&nbsp;2026/11/27</div>"
			item, ok := parseStatementMail([]byte(headers+summary), fixture.bank, "4321")
			if !ok || item.AmountDue == nil || *item.AmountDue != 12345 || item.DueOn != "2026-11-27" ||
				item.NeedsReview == "" || statementSummaryReady(item) {
				t.Fatalf("expected a review-only summary: %+v (%t)", item, ok)
			}
			wrong, ok := parseStatementMail([]byte(headers+summary+"<div>卡号末四位：9876</div>"), fixture.bank, "4321")
			if !ok || wrong.AmountDue != nil || statementSummaryReady(wrong) {
				t.Fatalf("different card must not supply the preview amount: %+v (%t)", wrong, ok)
			}
		})
	}
}
