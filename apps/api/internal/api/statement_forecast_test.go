package api

import (
	"testing"
	"time"
)

func TestStatementForecastUsesBankBalanceAndLaterPayments(t *testing.T) {
	day := func(s string) time.Time {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	closeOn := day("2026-11-01")
	due := day("2026-11-25")
	card := creditCardState{
		configured: true,
		charges: []creditCharge{
			{amount: 5000, occurredOn: closeOn, periodEnd: closeOn, dueOn: due},
			{amount: 12000, occurredOn: due, periodEnd: closeOn, dueOn: due, placeholder: true},
			{amount: 3000, occurredOn: day("2026-11-02"), periodEnd: day("2026-12-01"), dueOn: day("2026-12-25")},
		},
		reductions: 4000,
		payments: []creditCharge{
			{amount: 1500, occurredOn: day("2026-10-30")},
			{amount: 2500, occurredOn: day("2026-11-03")},
		},
	}
	projected := forecastCreditCard(&card, statementForecast{start: day("2026-10-02"), end: closeOn, due: due, amount: 12000})
	buckets, _, _, _, _ := allocateCreditDebt(&projected, day("2026-11-04"))
	if len(buckets) != 2 || buckets[0].amount != 9500 || buckets[1].amount != 3000 {
		t.Fatalf("forecast should replace the billed period and subtract only later payments: %+v", buckets)
	}
	if len(card.charges) != 3 || card.reductions != 4000 {
		t.Fatal("forecast changed ledger-derived card state")
	}
}
