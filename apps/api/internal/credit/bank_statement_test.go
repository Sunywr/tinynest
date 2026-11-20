package credit

import (
	"errors"
	"testing"
	"time"
)

func TestCMBSevenColumnTransactionRows(t *testing.T) {
	const body = `<table>
	<tr><td>交易日</td><td>记账日</td><td>交易摘要</td><td>人民币金额</td><td>卡号末四位</td><td>交易地</td><td>交易地金额</td></tr>
	<tr><td>0416</td><td>0417</td><td>测试消费</td><td>￥28.80</td><td>0123</td><td>CN</td><td>28.80</td></tr>
	<tr><td>0417</td><td>0418</td><td>其他卡消费</td><td>￥15.00</td><td>4567</td><td>CN</td><td>15.00</td></tr>
	</table>`
	rows, err := ParseBankStatement("cmb", body, time.Date(2023, 5, 17, 0, 0, 0, 0, time.UTC), "0123")
	if err != nil || len(rows) != 1 || rows[0].Amount != 2880 || rows[0].TransactionDate != "2023-04-16" {
		t.Fatalf("unexpected CMB detail: %v (%v)", rows, err)
	}
	if _, err := ParseBankStatement("cmb", body, time.Date(2023, 5, 17, 0, 0, 0, 0, time.UTC), "9999"); !errors.Is(err, ErrCMBNoMatchingCard) {
		t.Fatalf("unexpected unmatched card error: %v", err)
	}
}

func TestGuangfaSharedEmailSeparatesCards(t *testing.T) {
	const header = "<tr><td>交易日期</td><td>入账日期</td><td>交易摘要</td><td>交易金额</td><td>交易货币</td><td>入账金额</td><td>入账货币</td></tr>"
	body := "<table><tr><td>卡号：1234********0123</td></tr></table><table>" + header +
		"<tr><td>2026/11/01</td><td>2026/11/02</td><td>第一张卡</td><td>3.00</td><td>美元</td><td>20.00</td><td>人民币</td></tr></table>" +
		"<table><tr><td>卡号：1234********4567</td></tr></table><table>" + header +
		"<tr><td>2026/11/01</td><td>2026/11/02</td><td>第二张卡</td><td>-9.00</td><td>人民币</td><td>-9.00</td><td>人民币</td></tr></table>"
	date := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	for tail, want := range map[string]int64{"0123": 2000, "4567": -900} {
		rows, err := ParseBankStatement("gdb", body, date, tail)
		if err != nil || len(rows) != 1 || int64(rows[0].Amount) != want || rows[0].CardTail != tail {
			t.Fatalf("unexpected card parse: %v %v", rows, err)
		}
	}
	if _, err := ParseBankStatement("gdb", body, date, "9999"); err == nil {
		t.Fatal("unmatched card accepted")
	}
}
func TestCMBHeaderMappedHistory(t *testing.T) {
	const body = `<table>
	<tr><th>卡号末四位</th><th>交易日期</th><th>入账日期</th><th>交易说明</th><th>人民币金额</th></tr>
	<tr><td>0123</td><td>04/16</td><td>04/17</td><td>测试退款</td><td>-28.80</td></tr>
	</table>`
	rows, err := ParseBankStatement("cmb", body, time.Date(2015, 5, 17, 0, 0, 0, 0, time.UTC), "0123")
	if err != nil || len(rows) != 1 || rows[0].Amount != -2880 || rows[0].PostingDate != "2015-04-17" {
		t.Fatalf("unexpected CMB detail: %v (%v)", rows, err)
	}
}
