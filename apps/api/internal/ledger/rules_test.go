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
	cashA  = "1010:cash_a"
	cashB  = "1010:cash_b"
	card   = "2010:card_01"
	food   = "5010:cat_food"
	daily  = "5010:cat_daily"
	salary = "4010:cat_salary"
)

// Opening 10,000.00 then a 100.00 cash expense -> 9,900.00 balance.
func TestCashExpense(t *testing.T) {
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

// A credit account can be opened with an existing liability: the opening equity
// is debited and the liability credited, so the balance is negative (= owed).
func TestOpeningLiability(t *testing.T) {
	e := must(t)(OpeningBalance(c("open_card"), CreditSubject("card_01"), 500_000))
	if got := delta([]*Entry{e}, card); got != -500_000 {
		t.Errorf("credit opening delta = %d, want -500000", got)
	}
	if got := delta([]*Entry{e}, SubjOpeningEquity); got != 500_000 {
		t.Errorf("opening equity delta = %d, want 500000", got)
	}
	if e.BizType != BizOpeningLiability {
		t.Errorf("biz type = %q, want %q", e.BizType, BizOpeningLiability)
	}
}

// 8,000.00 salary increases income by exactly 8,000.00.
func TestIncome(t *testing.T) {
	es := []*Entry{must(t)(Income(c("tx2"), "cash_a", []Split{{"cat_salary", 800_000}}, 800_000))}
	if got := -delta(es, salary); got != 800_000 {
		t.Errorf("income = %d, want 800000", got)
	}
	if got := delta(es, cashA); got != 800_000 {
		t.Errorf("cash = %d, want 800000", got)
	}
}

// An internal transfer changes neither net worth nor income/expense totals.
func TestTransfer(t *testing.T) {
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
	if _, err := Transfer(c("same"), "cash_a", "cash_a", 100); !errors.Is(err, ErrInvalidComponents) {
		t.Errorf("self transfer must be rejected, got %v", err)
	}
}

// Split amounts must sum exactly to the transaction total.
func TestSplits(t *testing.T) {
	es := []*Entry{must(t)(CashExpense(c("tx4"), "cash_a",
		[]Split{{"cat_food", 30_000}, {"cat_daily", 20_000}}, 50_000))}
	if got := delta(es, cashA); got != -50_000 {
		t.Errorf("cash = %d, want -50000", got)
	}
	if _, err := CashExpense(c("bad"), "cash_a", []Split{{"cat_food", 30_000}}, 50_000); !errors.Is(err, ErrSplitMismatch) {
		t.Errorf("split mismatch must be rejected, got %v", err)
	}
	if _, err := CashExpense(c("bad2"), "cash_a", nil, 10_000); !errors.Is(err, ErrSplitMissing) {
		t.Errorf("missing split must be rejected, got %v", err)
	}
}

// Credit expense, repayment, refund, and over-refund protection.
func TestCreditAndRefund(t *testing.T) {
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

// Reversing keeps the original and restores every balance.
func TestReversal(t *testing.T) {
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
	if _, err := OpeningCash(c("neg"), "cash_a", 0); err == nil {
		t.Error("zero opening balance must be rejected")
	}
}
