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

func TestCommBilingualStatementSummary(t *testing.T) {
	body := `<h3><span>交通银行个人信用卡</span><span>123456******4321</span></h3>
 <table><tr><td>账单周期<br><span>Statement Cycle</span></td><td>2026/10/23-2026/11/22</td></tr>
 <tr><td>到期还款日<br><span>Payment Due Date</span></td><td>2026-12-16</td></tr>
 <tr><td>本期应还款</td><td>￥123.45</td><td>＄---</td></tr>
 <tr><td>最低应还款</td><td>￥10.00</td></tr></table><p>美元账户：$0.00</p>`
	raw := []byte("From: statement@bocomcc.com\r\nSubject: " +
		mime.BEncoding.Encode("UTF-8", "交通银行信用卡电子账单") +
		"\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" + body)
	item, ok := parseStatementMail(raw, "comm", "4321")
	if !ok || !statementSummaryReady(item) || item.StatementOn != "2026-11-22" ||
		item.PeriodStart != "2026-10-23" || item.DueOn != "2026-12-16" || *item.AmountDue != 12345 {
		t.Fatalf("unexpected summary: %+v", item)
	}
	wrong, _ := parseStatementMail(raw, "comm", "9876")
	if statementSummaryReady(wrong) || wrong.AmountDue != nil {
		t.Fatal("must not use another card's balance")
	}
}

func TestStatementSubjectKeywordStillRequiresBankSender(t *testing.T) {
	raw := []byte("From: statement@bocomcc.com\r\nSubject: " +
		mime.BEncoding.Encode("UTF-8", "您的每月对账通知") + "\r\n\r\n账单日：2026-11-07")
	if _, ok := parseStatementMail(raw, "comm", "1234", "每月对账通知"); !ok {
		t.Fatal("custom decoded title did not match")
	}
	if _, ok := parseStatementMail(raw, "comm", "1234", "其他标题"); ok {
		t.Fatal("nonmatching title accepted")
	}
	spoofed := strings.Replace(string(raw), "statement@bocomcc.com", "statement@example.net", 1)
	if _, ok := parseStatementMail([]byte(spoofed), "comm", "1234", "每月对账通知"); ok {
		t.Fatal("unknown sender passed a custom title rule")
	}
}

func TestStatementSubjectKeywordSavedAndCleared(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	for _, change := range []struct{ input, want string }{
		{"  电子对账通知  ", "电子对账通知"},
		{"", ""},
	} {
		status, body := fixture.client.do("PUT", fixture.bindingPath()+"?version=1", map[string]string{"subject_keyword": change.input})
		if status != http.StatusNoContent {
			t.Fatalf("save keyword: %d %v", status, body)
		}
		result := fixture.client.mustOK("GET", "/api/v1/credit-statement-mailbox/cards", nil)
		cards := v012Rows(t, result)
		actual, _ := cards[0]["subject_keyword"].(string)
		if len(cards) != 1 || actual != change.want {
			t.Fatalf("expected persisted keyword %q in %v", change.want, cards)
		}
	}
}

func TestCMBHTMLSummaryUsesNamedFields(t *testing.T) {
	raw := []byte("From: statement@message.cmbchina.com\r\nSubject: " +
		mime.BEncoding.Encode("UTF-8", "招商银行信用卡电子账单") +
		"\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		`<div id="statementCycle"><font>2026/10/17-2026/11/16</font></div>
  <span id="L1rmbLcurrBal">¥&nbsp;123.45</span><span id="L1rmbLdueAmt">¥ 6.17</span>
  <div id='paymentDueDate'><font>2026/12/04</font></div><div id="D1rmbLcurrBal"><font>¥ 123.45</font></div>`)
	item, ok := parseStatementMail(raw, "cmb", "4321")
	if !ok || item.AmountDue == nil || *item.AmountDue != 12345 ||
		item.PeriodStart != "2026-10-17" || item.StatementOn != "2026-11-16" || item.DueOn != "2026-12-04" {
		t.Fatalf("unexpected named summary: %+v", item)
	}
	if item.NeedsReview == "" || statementSummaryReady(item) {
		t.Fatal("account-wide amount without explicit card identification must remain review-only")
	}
}

func TestStatementSenderDecodesGBKDisplayName(t *testing.T) {
	name, err := simplifiedchinese.GBK.NewEncoder().String("交通银行信用卡")
	if err != nil {
		t.Fatal(err)
	}
	from := mime.BEncoding.Encode("GBK", name) + " <statement@bocomcc.com>"
	if !statementSenderMatches(from, "comm") {
		t.Fatal("valid sender rejected with GBK display name")
	}
	if statementSenderMatches(strings.Replace(from, "bocomcc.com", "bocomcc.com.example", 1), "comm") {
		t.Fatal("unrecognized sender accepted")
	}
}

func TestGDBCardSummarySeparatesCardsAndNoDebt(t *testing.T) {
	raw := bankSummaryHTML("creditcardbill@cgbchina.com.cn", "广发银行信用卡账单", `
 <p>账单周期:2026/09/29-2026/10/28</p><p>账单日:2026/10/28</p>
 <table><tr><td>卡号末四位</td><td>本期账单金额</td><td>最低还款额</td><td>最后还款日</td><td>入账货币</td><td>存款</td><td>卡片消费额度</td></tr></table>
 <table><tr><td>1234<br></td><td>350.77</td><td>100.00</td><td>2026/11/17</td><td>人民币</td><td>0.00</td><td>47,000.00</td></tr></table>
 <table><tr><td>5678<br></td><td>无欠款</td><td>无欠款</td><td>2026/11/17</td><td>人民币</td><td>0.00</td><td>47,000.00</td></tr></table>`)
	for _, tc := range []struct {
		tail  string
		minor int64
	}{{"1234", 35077}, {"5678", 0}} {
		item, _ := parseStatementMail(raw, "gdb", tc.tail)
		if !statementSummaryReady(item) || int64(*item.AmountDue) != tc.minor || item.PeriodStart != "2026-09-29" || item.DueOn != "2026-11-17" {
			t.Fatalf("unexpected card summary: %+v", item)
		}
	}
	wrong, _ := parseStatementMail(raw, "gdb", "9999")
	if statementSummaryReady(wrong) {
		t.Fatal("wrong card accepted")
	}
}

func TestCEBBilingualSummaryVerifiesCardAndRMB(t *testing.T) {
	body := `<table><tr><td>账单周期<p>Statement Cycle</p></td><td>2026/10/11-2026/11/10</td><td>人民币本期应还款额<p>RMB Statement Balance</p></td><td>￥123.45</td></tr>
 <tr><td>账单日<p>Statement Date</p></td><td>2026/11/10</td><td>人民币本期最低还款额<p>RMB Minimum Payment Due</p></td><td>￥6.17</td></tr>
 <tr><td>到期还款日<p>Payment Due Date</p></td><td>2026/11/29</td></tr></table>
 <table><tr><td>账号<p>Account Number</p></td><td>账户名称<p>Card Name</p></td><td>本期余额<p>Total Balance</p></td><td>本期应还款额<p>Statement Balance</p></td><td>本期最低还款额<p>Minimum Payment Due</p></td></tr>
 <tr><td>12345678****4321</td><td>联名信用卡</td><td>123.45</td><td>123.45</td><td>6.17</td></tr></table>`
	raw := bankSummaryHTML("statement@cardcenter.cebbank.com", "光大信用卡账单", body)
	item, _ := parseStatementMail(raw, "ceb", "4321")
	if !statementSummaryReady(item) || *item.AmountDue != 12345 || item.PeriodStart != "2026-10-11" || item.DueOn != "2026-11-29" {
		t.Fatalf("unexpected CEB summary: %+v", item)
	}
	wrong, _ := parseStatementMail(raw, "ceb", "9999")
	if statementSummaryReady(wrong) {
		t.Fatal("wrong card accepted")
	}
}

func bankSummaryHTML(sender, subject, body string) []byte {
	return []byte("From: " + sender + "\r\nSubject: " + mime.BEncoding.Encode("UTF-8", subject) + "\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" + body)
}
