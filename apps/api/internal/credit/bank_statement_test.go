package credit

import (
	"testing"
	"time"
)

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
