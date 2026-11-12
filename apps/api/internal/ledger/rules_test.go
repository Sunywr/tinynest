package ledger

import (
	"errors"
	"testing"

	"tinynest/api/internal/money"
)

func c(id string) Context {
	return Context{EntryID: "e_" + id, BizID: id, OccurredOn: "2026-11-12"}
}

// Curried so a two-value rule call can be passed directly: must(t)(CashExpense(...)).
func must(t *testing.T) func(*Entry, error) *Entry {
	t.Helper()
	return func(e *Entry, err error) *Entry {
		t.Helper()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		dr, cr := e.Totals()
		if dr != cr || dr == 0 {
			t.Fatalf("entry not balanced: debit=%d credit=%d", dr, cr)
		}
		return e
	}
}

// delta aggregates debit-minus-credit across entries for one subject.
func delta(es []*Entry, subject string) money.Minor {
	var d money.Minor
	for _, e := range es {
		d += e.Delta(subject)
	}
	return d
}

const (
	cashA   = "1010:cash_a"
	cashB   = "1010:cash_b"
	card    = "2010:card_01"
	food    = "5010:cat_food"
	daily   = "5010:cat_daily"
	salary  = "4010:cat_salary"
	holder  = "1040:asset_01"
	secCash = "1020:securities_01"
	loan    = "2030:mortgage_01"
	inst    = "2020:installment_01"
)

// Case 1: opening 10,000.00 then a 100.00 cash expense -> 9,900.00 balance.
func TestCase01_CashExpense(t *testing.T) {
	es := []*Entry{
		must(t)(OpeningCash(c("open"), "cash_a", 1_000_000)),
		must(t)(CashExpense(c("tx1"), "cash_a", []Split{{"cat_food", 10_000}}, 10_000)),
	}
	if got := delta(es, cashA); got != 990_000 {
		t.Errorf("cash balance = %d, want 990000", got)
	}
	if got := delta(es, food); got != 10_000 {
		t.Errorf("expense = %d, want 10000", got)
	}
	if got := delta(es, SubjOpeningEquity); got != -1_000_000 {
		t.Errorf("opening equity = %d, want -1000000 (never income)", got)
	}
}

// Case 2: 8,000.00 salary increases income by exactly 8,000.00.
func TestCase02_Income(t *testing.T) {
	es := []*Entry{must(t)(Income(c("tx2"), "cash_a", []Split{{"cat_salary", 800_000}}, 800_000))}
	if got := -delta(es, salary); got != 800_000 {
		t.Errorf("income = %d, want 800000", got)
	}
	if got := delta(es, cashA); got != 800_000 {
		t.Errorf("cash = %d, want 800000", got)
	}
}

// Case 3: an internal transfer changes neither net worth nor income/expense totals.
func TestCase03_Transfer(t *testing.T) {
	es := []*Entry{
		must(t)(OpeningCash(c("open"), "cash_a", 1_000_000)),
		must(t)(Transfer(c("tx3"), "cash_a", "cash_b", 100_000)),
	}
	if got := delta(es, cashA) + delta(es, cashB); got != 1_000_000 {
		t.Errorf("total assets = %d, want 1000000", got)
	}
	if got := delta(es, food) + delta(es, daily) + delta(es, salary); got != 0 {
		t.Errorf("income/expense must be untouched, got %d", got)
	}
}

// Case 4 & 29: split amounts must sum exactly to the transaction total.
func TestCase04_29_Splits(t *testing.T) {
	es := []*Entry{must(t)(CashExpense(c("tx4"), "cash_a",
		[]Split{{"cat_food", 30_000}, {"cat_daily", 20_000}}, 50_000))}
	if got := delta(es, cashA); got != -50_000 {
		t.Errorf("cash = %d, want -50000", got)
	}
	if _, err := CashExpense(c("bad"), "cash_a", []Split{{"cat_food", 30_000}}, 50_000); !errors.Is(err, ErrSplitMismatch) {
		t.Errorf("split mismatch must be rejected, got %v", err)
	}
}

// Cases 5-8: credit expense, repayment, refund, and over-refund protection.
func TestCase05to08_CreditAndRefund(t *testing.T) {
	es := []*Entry{
		must(t)(CreditExpense(c("tx5"), "card_01", []Split{{"cat_food", 100_000}}, 100_000)),
	}
	if got := delta(es, cashA); got != 0 {
		t.Errorf("credit expense must not touch cash, got %d", got)
	}
	if got := -delta(es, card); got != 100_000 {
		t.Errorf("credit liability = %d, want 100000", got)
	}

	es = append(es, must(t)(RepayCredit(c("tx6"), "cash_a", "card_01", 60_000)))
	if got := delta(es, cashA); got != -60_000 {
		t.Errorf("cash after repayment = %d, want -60000", got)
	}
	if got := delta(es, food); got != 100_000 {
		t.Errorf("repayment must NOT add expense, got %d", got)
	}
	if got := -delta(es, card); got != 40_000 {
		t.Errorf("remaining credit liability = %d, want 40000", got)
	}

	es = append(es, must(t)(RefundToCredit(c("tx7"), "card_01", "cat_food", 30_000)))
	if got := delta(es, food); got != 70_000 {
		t.Errorf("net expense after refund = %d, want 70000", got)
	}
	if got := -delta(es, card); got != 10_000 {
		t.Errorf("credit liability after refund = %d, want 10000", got)
	}

	remaining, err := ValidateRefund(100_000, 30_000, 30_000)
	if err != nil || remaining != 40_000 {
		t.Fatalf("second partial refund: remaining=%d err=%v, want 40000/nil", remaining, err)
	}
	if _, err := ValidateRefund(100_000, 70_000, 40_000); !errors.Is(err, ErrRefundExceeds) {
		t.Errorf("over-refund must be rejected, got %v", err)
	}
}

// Case 9 is covered by the credit cycle package (month-end 28/29/30/31 rules).

// Case 10: paying 2,000.00 mortgage with a credit card (1,500 principal + 500 interest).
func TestCase10_CreditPaysMortgage(t *testing.T) {
	es := []*Entry{must(t)(LoanPayment(c("tx10"), "mortgage_01", CreditSubject("card_01"), 150_000, 50_000))}
	// A liability decreases on the DEBIT side, so the net debit-credit delta is +150000.
	if got := delta(es, loan); got != 150_000 {
		t.Errorf("loan principal reduction = %d, want 150000 (debit)", got)
	}
	if got := delta(es, SubjExpenseInterest); got != 50_000 {
		t.Errorf("interest expense = %d, want 50000", got)
	}
	if got := -delta(es, card); got != 200_000 {
		t.Errorf("credit liability increase = %d, want 200000", got)
	}
	if got := delta(es, cashA); got != 0 {
		t.Errorf("cash must be unchanged, got %d", got)
	}
}

// Cases 11, 13, 14: installment creation, cash payment, credit payment.
func TestCase11to14_Installment(t *testing.T) {
	const principal, totalFee, periods = 300_000, 9_000, 3
	var sumPrincipal, sumFee money.Minor
	for i := 0; i < periods; i++ {
		sumPrincipal += principal / periods
		sumFee += totalFee / periods
	}
	if sumPrincipal != principal || sumFee != totalFee {
		t.Fatalf("schedule sums mismatch: principal=%d fee=%d", sumPrincipal, sumFee)
	}

	es := []*Entry{must(t)(InstallmentCreate(c("inst"), "installment_01", "cat_daily", principal))}
	if got := delta(es, daily); got != principal {
		t.Errorf("consumption recognised once = %d, want %d", got, principal)
	}
	if got := -delta(es, inst); got != principal {
		t.Errorf("installment liability = %d, want %d", got, principal)
	}
	// No fee is accrued at creation time.
	if got := delta(es, SubjExpenseFee); got != 0 {
		t.Errorf("fee must not be accrued upfront, got %d", got)
	}

	es = append(es, must(t)(InstallmentPayment(c("p1"), "installment_01", CashSubject("cash_a"), 100_000, 3_000)))
	if got := delta(es, cashA); got != -103_000 {
		t.Errorf("cash after one period = %d, want -103000", got)
	}
	if got := delta(es, SubjExpenseFee); got != 3_000 {
		t.Errorf("period fee = %d, want 3000", got)
	}
	if got := -delta(es, inst); got != 200_000 {
		t.Errorf("remaining installment principal = %d, want 200000", got)
	}

	es = append(es, must(t)(InstallmentPayment(c("p2"), "installment_01", CreditSubject("card_01"), 100_000, 3_000)))
	if got := delta(es, cashA); got != -103_000 {
		t.Errorf("credit-funded period must not change cash, got %d", got)
	}
	if got := -delta(es, card); got != 103_000 {
		t.Errorf("credit liability = %d, want 103000", got)
	}
	if got := -delta(es, inst); got != 100_000 {
		t.Errorf("remaining installment principal = %d, want 100000", got)
	}
}

// Case 18: converting existing credit consumption into an installment keeps a single principal.
func TestCase18_ConvertToInstallment(t *testing.T) {
	es := []*Entry{
		must(t)(CreditExpense(c("buy"), "card_01", []Split{{"cat_daily", 300_000}}, 300_000)),
		must(t)(ConvertToInstallment(c("conv"), "card_01", "installment_01", 300_000)),
	}
	if got := -delta(es, card); got != 0 {
		t.Errorf("credit liability must be reclassified to zero, got %d", got)
	}
	if got := -delta(es, inst); got != 300_000 {
		t.Errorf("installment liability = %d, want 300000", got)
	}
	if got := delta(es, daily); got != 300_000 {
		t.Errorf("expense must not be duplicated, got %d", got)
	}
}

// Cases 18-21 (investment postings) and 22 (dividend tax unknown).
func TestInvestmentPostings(t *testing.T) {
	buy := must(t)(InvestBuy(c("buy1"), secCash, holder, 100_000, 500))
	if got := delta([]*Entry{buy}, holder); got != 100_500 {
		t.Errorf("holding cost incl. buy fee = %d, want 100500", got)
	}
	if got := delta([]*Entry{buy}, food); got != 0 {
		t.Errorf("buying an asset is never consumption, got %d", got)
	}

	// Sell 50 shares: net proceeds 645.00, carried cost 552.50 -> realised gain 92.50
	sell := must(t)(InvestSell(c("sell1"), secCash, holder, 64_500, 55_250))
	if got := -delta([]*Entry{sell}, SubjRealizedGain); got != 9_250 {
		t.Errorf("realised gain = %d, want 9250", got)
	}
	if got := delta([]*Entry{sell}, holder); got != -55_250 {
		t.Errorf("carried cost = %d, want -55250", got)
	}
	if got := delta([]*Entry{sell}, salary); got != 0 {
		t.Error("sale proceeds must never be posted as ordinary income")
	}

	loss := must(t)(InvestSell(c("sell2"), secCash, holder, 40_000, 50_000))
	if got := delta([]*Entry{loss}, SubjRealizedLoss); got != 10_000 {
		t.Errorf("realised loss = %d, want 10000", got)
	}

	// tax unknown: only the received amount is posted, no zero-tax assumption
	unknown := must(t)(Dividend(c("div1"), secCash, 10_000, nil, 9_000))
	if got := delta([]*Entry{unknown}, SubjDividendTax); got != 0 {
		t.Errorf("unknown tax must not be booked as zero, got %d", got)
	}
	if got := -delta([]*Entry{unknown}, SubjIncomeDividend); got != 9_000 {
		t.Errorf("dividend income = %d, want 9000", got)
	}

	tax := money.Minor(1_000)
	known := must(t)(Dividend(c("div2"), secCash, 10_000, &tax, 9_000))
	if got := delta([]*Entry{known}, SubjDividendTax); got != 1_000 {
		t.Errorf("dividend tax = %d, want 1000", got)
	}
	if got := -delta([]*Entry{known}, SubjIncomeDividend); got != 10_000 {
		t.Errorf("gross dividend income = %d, want 10000", got)
	}
}

// Case 19 & 26: reversing keeps the original and restores every balance.
func TestCase26_Reversal(t *testing.T) {
	orig := must(t)(CashExpense(c("tx26"), "cash_a", []Split{{"cat_food", 10_000}}, 10_000))
	rev := must(t)(orig.Reverse("e_rev26", "2026-11-13"))
	if rev.ReversalOf != orig.ID {
		t.Errorf("reversal_of not linked: %q", rev.ReversalOf)
	}
	if orig.Status != StatusPosted {
		t.Error("original entry must be preserved, not mutated")
	}
	es := []*Entry{orig, rev}
	for _, s := range []string{cashA, food} {
		if got := delta(es, s); got != 0 {
			t.Errorf("subject %s net effect after reversal = %d, want 0", s, got)
		}
	}
	// The pure kernel never mutates the original. Marking it StatusReversed and
	// refusing a second reversal is the persistence layer's responsibility.
}

func TestKernelRejectsInvalidEntries(t *testing.T) {
	if _, err := New("x", BizExpense, "x", "2026-11-12",
		Line{Account: food, Debit: 100}, Line{Account: cashA, Credit: 99}); !errors.Is(err, ErrUnbalanced) {
		t.Errorf("unbalanced entry must be rejected, got %v", err)
	}
	if _, err := New("y", BizExpense, "y", "2026-11-12",
		Line{Account: food, Debit: 100, Credit: 100}, Line{Account: cashA, Credit: 100}); !errors.Is(err, ErrBadLine) {
		t.Errorf("two-sided line must be rejected, got %v", err)
	}
	if _, err := New("z", BizExpense, "z", "2026-11-12", Line{Account: food, Debit: 100}); !errors.Is(err, ErrTooFewLines) {
		t.Errorf("single-line entry must be rejected, got %v", err)
	}
	if _, err := Transfer(c("same"), "cash_a", "cash_a", 100); !errors.Is(err, ErrInvalidComponents) {
		t.Errorf("self transfer must be rejected, got %v", err)
	}
}
