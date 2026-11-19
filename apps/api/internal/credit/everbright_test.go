package credit

import (
	"mime"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestMatchEverbrightStatementHeader(t *testing.T) {
	const keyword = "光大信用卡电子账单"
	const sender = "光大银行 <statement@cardcenter.cebbank.com>"
	for _, subject := range []string{keyword, "光大信用卡电子账单（2023年9月13日）", "光大信用卡（2023年9月13日）电子账单", "2023-09-13 " + keyword} {
		if !MatchEverbrightStatementHeader(sender, mime.BEncoding.Encode("UTF-8", subject), keyword) {
			t.Errorf("did not match %q", subject)
		}
	}
	gb, err := simplifiedchinese.GB18030.NewEncoder().String("光大信用卡电子账单（2023年9月13日）")
	gbSender, senderErr := simplifiedchinese.GB18030.NewEncoder().String("光大银行")
	legacyFrom := mime.BEncoding.Encode("gb18030", gbSender) + " <statement@cardcenter.cebbank.com>"
	if err != nil || senderErr != nil || !MatchEverbrightStatementHeader(legacyFrom, mime.BEncoding.Encode("gb18030", gb), keyword) {
		t.Fatal("GB18030 statement title did not match")
	}
	if MatchEverbrightStatementHeader("statement@cardcenter.cebbank.com.example.org", keyword, keyword) {
		t.Fatal("accepted a lookalike sender domain")
	}
	if MatchEverbrightStatementHeader(sender, "中国光大银行信用卡消费提醒", keyword) {
		t.Fatal("accepted an unrelated notice")
	}
}

func TestParseEverbrightHTML(t *testing.T) {
	const detail = `<html><head><meta style="font style="font-size: 12px;"></head><body><table><tr><td><table>
 <tr><td><p>交易日<br>Trans. Date</p></td><td>记账日 Post Date</td><td>卡号末四位</td><td>交易说明 Description of Transaction</td><td>金额 Amount(RMB)</td></tr>
 <tr><td>12/31</td><td>01/01</td><td>0123</td><td>测试商户</td><td>1,234.56</td></tr>
 <tr><td>2026/01/02</td><td>2026/01/03</td><td>0123</td><td>还款 跨行实时还款</td><td>(存入)1,234.56</td></tr>
 <tr><td>01/02</td><td>01/03</td><td>9999</td><td>其他卡</td><td>10.00</td></tr>
 </table></td></tr></table></body></html>`
	closing, _ := time.Parse(time.DateOnly, "2026-01-05")
	rows, err := ParseEverbrightHTML(detail, closing, "0123")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].TransactionDate != "2025-12-31" || rows[0].PostingDate != "2026-01-01" ||
		rows[0].Amount != 123456 || rows[1].Amount != -123456 || rows[1].Description != "还款 跨行实时还款" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if _, err := ParseEverbrightHTML(strings.Replace(detail, "1,234.56", "1.2,3", 1), closing, "0123"); err == nil {
		t.Fatal("invalid amount accepted")
	}
	if _, err := ParseEverbrightHTML(strings.Replace(detail, "Amount(RMB)", "Amount(USD)", 1), closing, "0123"); err == nil {
		t.Fatal("foreign currency table accepted")
	}
}
