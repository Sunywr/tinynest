// Package ledger implements the double-entry kernel.
// Every posted business event is expressed as a balanced, immutable journal entry.
package ledger

import (
	"errors"
	"fmt"

	"tinynest/api/internal/money"
)

var (
	ErrUnbalanced        = errors.New("ledger: debit and credit totals differ")
	ErrTooFewLines       = errors.New("ledger: an entry needs at least two lines")
	ErrBadLine           = errors.New("ledger: exactly one positive side per line is required")
	ErrEmptyAccount      = errors.New("ledger: line account is empty")
	ErrAlreadyReversed   = errors.New("ledger: entry is already reversed")
	ErrSplitMissing      = errors.New("ledger: at least one split is required")
	ErrSplitMismatch     = errors.New("ledger: split amounts do not sum to transaction total")
	ErrCategoryMissing   = errors.New("ledger: split category is empty")
	ErrInvalidComponents = errors.New("ledger: invalid amount components")
)

const (
	StatusPosted   = "posted"
	StatusReversed = "reversed"
)

// Line is one side of a journal entry. Exactly one of Debit/Credit must be > 0.
type Line struct {
	Account string      `json:"account"`
	Debit   money.Minor `json:"debit_minor"`
	Credit  money.Minor `json:"credit_minor"`
	Memo    string      `json:"memo,omitempty"`
}

// Entry is immutable once posted; corrections are separate reversing entries.
type Entry struct {
	ID         string `json:"id"`
	BizType    string `json:"biz_type"`
	BizID      string `json:"biz_id"`
	OccurredOn string `json:"occurred_on"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
	ReversalOf string `json:"reversal_of,omitempty"`
	Lines      []Line `json:"lines"`
}

// New validates balance and returns a posted entry, or an error (nothing is persisted).
func New(id, bizType, bizID, occurredOn string, lines ...Line) (*Entry, error) {
	e := &Entry{
		ID: id, BizType: bizType, BizID: bizID, OccurredOn: occurredOn,
		Currency: "CNY", Status: StatusPosted, Lines: lines,
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return e, nil
}

// Validate enforces the two invariants that protect all balances.
func (e *Entry) Validate() error {
	if len(e.Lines) < 2 {
		return ErrTooFewLines
	}
	var dr, cr money.Minor
	for i, l := range e.Lines {
		if l.Account == "" {
			return fmt.Errorf("%w at line %d", ErrEmptyAccount, i)
		}
		if l.Debit < 0 || l.Credit < 0 {
			return fmt.Errorf("%w at line %d: negative amount", ErrBadLine, i)
		}
		if (l.Debit > 0) == (l.Credit > 0) {
			return fmt.Errorf("%w at line %d: debit=%d credit=%d", ErrBadLine, i, l.Debit, l.Credit)
		}
		var err error
		if dr, err = money.Add(dr, l.Debit); err != nil {
			return err
		}
		if cr, err = money.Add(cr, l.Credit); err != nil {
			return err
		}
	}
	if dr == 0 {
		return fmt.Errorf("%w: zero-value entry", ErrUnbalanced)
	}
	if dr != cr {
		return fmt.Errorf("%w: debit=%d credit=%d", ErrUnbalanced, dr, cr)
	}
	return nil
}

// Reverse builds the compensating entry (debit/credit swapped).
// The original entry is NEVER mutated or deleted.
func (e *Entry) Reverse(newID, occurredOn string) (*Entry, error) {
	if e.Status == StatusReversed {
		return nil, ErrAlreadyReversed
	}
	lines := make([]Line, len(e.Lines))
	for i, l := range e.Lines {
		lines[i] = Line{Account: l.Account, Debit: l.Credit, Credit: l.Debit, Memo: "冲正"}
	}
	r, err := New(newID, "reversal", e.BizID, occurredOn, lines...)
	if err != nil {
		return nil, err
	}
	r.ReversalOf = e.ID
	return r, nil
}

// Deltas returns net (debit - credit) per subject.
// For asset/expense subjects a positive delta is an increase; for liability/equity/income it is a decrease.
func (e *Entry) Deltas() map[string]money.Minor {
	out := make(map[string]money.Minor, len(e.Lines))
	for _, l := range e.Lines {
		out[l.Account] += l.Debit - l.Credit
	}
	return out
}

// Delta is the net effect on one subject.
func (e *Entry) Delta(account string) money.Minor {
	var d money.Minor
	for _, l := range e.Lines {
		if l.Account == account {
			d += l.Debit - l.Credit
		}
	}
	return d
}

// Totals returns the balanced debit/credit sums.
func (e *Entry) Totals() (money.Minor, money.Minor) {
	var dr, cr money.Minor
	for _, l := range e.Lines {
		dr += l.Debit
		cr += l.Credit
	}
	return dr, cr
}
