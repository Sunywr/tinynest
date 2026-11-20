package credit

import "testing"

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
