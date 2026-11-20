package credit

import "testing"

func TestCMBStatementSummaryWithoutValidDetails(t *testing.T) {
	body := `<table><tr><td>账单周期<br>Statement Cycle</td><td>2023/04/17－2023/05/16</td></tr></table>
	<table><tr><td>本期应还金额<br>New Balance</td><td>上期账单金额</td><td>上期还款金额</td>
	<td>本期账单金额<br>New Charges</td><td>本期调整金额</td><td>循环利息</td></tr>
	<tr><td>¥ 161.75</td><td>¥ 22.02</td><td>¥ 22.02</td><td>¥ 160.70</td><td>¥ 0.00</td><td>¥ 1.05</td></tr></table>`
	got := ParseCMBStatementAmounts(body)
	if got.StatementDate != "2023-05-16" || got.NewCharges == nil || *got.NewCharges != 16070 ||
		got.AmountDue == nil || *got.AmountDue != 16175 {
		t.Fatalf("unexpected CMB summary: %+v", got)
	}
}

func TestCITICStatementSummaryUsesMatchingRMBAccount(t *testing.T) {
	body := `<tr><td>本期应还款总额</td><td></td><td>CNY 400.00</td></tr>
	<tr><td>账单日</td><td>2026年11月07日</td></tr>
	<tr><td>Card Number</td><td>New Charge</td><td>Acct. Statement Balance</td></tr>
	<tr><td><td>1234-56**-****-9876</td><td>CNY</td><td>10.00</td><td><td>2.00</td><td><td>30.00</td><td><td>38.00</td><td><td>4.00</td><td></tr>
	<tr><td><td>1234-56**-****-0123</td><td>CNY</td><td>20.00</td><td><td>5.00</td><td><td>40.00</td><td><td>55.00</td><td><td>5.00</td><td></tr>`
	got, matched := ParseCITICStatementAmounts(body, "0123")
	if !matched || got.NewCharges == nil || *got.NewCharges != 4000 || got.AmountDue == nil || *got.AmountDue != 5500 || got.StatementDate != "2026-11-07" {
		t.Fatalf("unexpected account summary: %+v, matched=%v", got, matched)
	}
	got, matched = ParseCITICStatementAmounts(body, "0000")
	if matched || got.NewCharges != nil || got.AmountDue == nil || *got.AmountDue != 40000 {
		t.Fatalf("unexpected unmatched summary: %+v, matched=%v", got, matched)
	}
}

func TestCITICStatementSender(t *testing.T) {
	const subject = "中信银行信用卡电子账单"
	if !MatchBankStatementHeader("citic", "statement@bill.citiccard.com", subject, "信用卡电子账单") {
		t.Fatal("CITIC statement subdomain rejected")
	}
	if MatchBankStatementHeader("citic", "statement@bill.citiccard.com.example.org", subject, "信用卡电子账单") {
		t.Fatal("unrelated sender domain accepted")
	}
}

func TestStatementAmountsRecognizesCreditBalance(t *testing.T) {
	got := ParseStatementAmounts("<table><tr><td>本期应还款总额：</td><td>￥-25.00</td></tr>" +
		"<tr><td>到期还款日：您无需还款</td></tr></table>")
	if got.AmountDue == nil || *got.AmountDue != -2500 || got.NewCharges != nil {
		t.Fatalf("unexpected credit balance summary: %+v", got)
	}
}

// The historical and current official SPDB mailboxes must both sync.
func TestSPDBSummarySender(t *testing.T) {
	for _, sender := range []string{"estmtservice@eb.spdbccc.com.cn", "estmtservice@eb.spdb.com.cn"} {
		if !MatchBankStatementHeader("spdb", sender, "浦发银行信用卡电子账单", "信用卡电子账单") {
			t.Fatalf("official address rejected: %s", sender)
		}
	}
	if MatchBankStatementHeader("spdb", "other@eb.spdb.com.cn", "浦发银行信用卡电子账单", "信用卡电子账单") {
		t.Fatal("unexpected sender accepted")
	}
}

func TestStatementAmountsSeparateChargesFromAmountDue(t *testing.T) {
	const due = "<tr><td>本期应还款总额：</td><td>￥1,234.56</td></tr>"
	const charges = "<tr><td><b>新签金额 / 其他费用</b></td><td>29.12</td></tr>"
	got := ParseStatementAmounts("<table><tr><td>账单日</td><td>2026/11/07</td></tr>" + due + charges + "</table>")
	if got.AmountDue == nil || *got.AmountDue != 123456 || got.NewCharges == nil || *got.NewCharges != 2912 {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if got.StatementDate != "2026-11-07" {
		t.Fatal("explicit closing date did not match")
	}
	dates := ParseStatementAmounts("<table><tr><td>账单日</td><td>2026/11/07</td></tr><tr><td>账单日期</td><td>2026-11-08</td></tr></table>")
	if dates.StatementDate != "" {
		t.Fatal("conflicting closing dates were accepted")
	}
	missing := ParseStatementAmounts("<table>" + due + "</table>")
	if missing.NewCharges != nil {
		t.Fatal("amount due was treated as new spending")
	}
	conflict := ParseStatementAmounts("<table>" + charges + "<tr><td>本期新增消费及费用</td><td>30.00</td></tr></table>")
	if conflict.NewCharges != nil {
		t.Fatal("conflicting amounts were accepted")
	}
}

func TestCMBLegacyDueSummary(t *testing.T) {
	const body = `<table><tr><td>&nbsp;</td><td>人民币交易(RMB)</td><td>美金交易(USD)</td></tr>
	<tr><td>本期应还金额&nbsp;New&nbsp;Balance</td><td>￥123.45</td><td>＄1.00</td></tr>
	<tr><td>最低还款额 Mini Payment</td><td>￥12.34</td><td>＄0.00</td></tr></table>`
	got := ParseCMBStatementAmounts(body)
	if got.AmountDue == nil || *got.AmountDue != 12345 || got.NewCharges != nil {
		t.Fatalf("unexpected legacy summary: %+v", got)
	}
}
