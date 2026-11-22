package ledger

import (
	"errors"
	"fmt"
	"strings"

	"tinynest/api/internal/money"
)

// ErrRefundExceeds protects the original transaction from being over-refunded.
var ErrRefundExceeds = errors.New("ledger: cumulative refund exceeds original amount")

// Context identifies the business event that produces an entry.
type Context struct {
	EntryID    string
	BizID      string
	OccurredOn string
}

func (c Context) entry(bizType string, lines ...Line) (*Entry, error) {
	return New(c.EntryID, bizType, c.BizID, c.OccurredOn, lines...)
}

// Split is one category line of a transaction. Sum of all splits MUST equal the total.
type Split struct {
	CategoryID string
	Amount     money.Minor
}

func validateSplits(splits []Split, total money.Minor) error {
	if len(splits) == 0 {
		return ErrSplitMissing
	}
	var sum money.Minor
	for i, s := range splits {
		if err := money.MustPositive(s.Amount); err != nil {
			return fmt.Errorf("split %d: %w", i, err)
		}
		if s.CategoryID == "" {
			return fmt.Errorf("split %d: %w", i, ErrCategoryMissing)
		}
		var err error
		if sum, err = money.Add(sum, s.Amount); err != nil {
			return err
		}
	}
	if sum != total {
		return fmt.Errorf("%w: splits=%d total=%d", ErrSplitMismatch, sum, total)
	}
	return nil
}

func expenseLines(splits []Split) []Line {
	lines := make([]Line, 0, len(splits)+1)
	for _, s := range splits {
		lines = append(lines, Line{Account: ExpenseSubject(s.CategoryID), Debit: s.Amount})
	}
	return lines
}

func incomeLines(splits []Split) []Line {
	lines := make([]Line, 0, len(splits)+1)
	for _, s := range splits {
		lines = append(lines, Line{Account: IncomeSubject(s.CategoryID), Credit: s.Amount})
	}
	return lines
}

// --- opening balances -------------------------------------------------------

// OpeningBalance books a balance that already existed before the ledger was
// opened. The direction is derived from the account class:
//
//	asset (1xxx):   借 科目 / 贷 期初权益
//	liability (2xxx): 借 期初权益 / 贷 科目
//
// Opening balances never touch income or expense; they are capital, not cash
// flow. Credit-card accounts are created with the liability direction so the
// outstanding amount is visible from day one.
func OpeningBalance(c Context, subject string, amount money.Minor) (*Entry, error) {
	if err := money.MustPositive(amount); err != nil {
		return nil, err
	}
	switch {
	case strings.HasPrefix(subject, "2"), strings.HasPrefix(subject, "3"), strings.HasPrefix(subject, "4"):
		return c.entry(BizOpeningLiability,
			Line{Account: SubjOpeningEquity, Debit: amount},
			Line{Account: subject, Credit: amount})
	default:
		return c.entry(BizOpeningCash,
			Line{Account: subject, Debit: amount},
			Line{Account: SubjOpeningEquity, Credit: amount})
	}
}

// OpeningCash is the cash-account specialisation of OpeningBalance.
func OpeningCash(c Context, accountID string, amount money.Minor) (*Entry, error) {
	return OpeningBalance(c, CashSubject(accountID), amount)
}

// --- daily bookkeeping ------------------------------------------------------

// CashExpense: 借 费用分类 / 贷 现金账户.
func CashExpense(c Context, accountID string, splits []Split, total money.Minor) (*Entry, error) {
	if err := money.MustPositive(total); err != nil {
		return nil, err
	}
	if err := validateSplits(splits, total); err != nil {
		return nil, err
	}
	lines := append(expenseLines(splits), Line{Account: CashSubject(accountID), Credit: total})
	return c.entry(BizExpense, lines...)
}

// CreditExpense: 借 费用分类 / 贷 信用负债. Cash is untouched; expense is recognised now.
func CreditExpense(c Context, accountID string, splits []Split, total money.Minor) (*Entry, error) {
	if err := money.MustPositive(total); err != nil {
		return nil, err
	}
	if err := validateSplits(splits, total); err != nil {
		return nil, err
	}
	lines := append(expenseLines(splits), Line{Account: CreditSubject(accountID), Credit: total})
	return c.entry(BizExpense, lines...)
}

// Income: 借 现金账户 / 贷 收入分类.
func Income(c Context, accountID string, splits []Split, total money.Minor) (*Entry, error) {
	if err := money.MustPositive(total); err != nil {
		return nil, err
	}
	if err := validateSplits(splits, total); err != nil {
		return nil, err
	}
	lines := append([]Line{{Account: CashSubject(accountID), Debit: total}}, incomeLines(splits)...)
	return c.entry(BizIncome, lines...)
}

// Transfer: 借 转入账户 / 贷 转出账户. Net worth, income and expense are unchanged.
func Transfer(c Context, fromAccountID, toAccountID string, amount money.Minor) (*Entry, error) {
	if err := money.MustPositive(amount); err != nil {
		return nil, err
	}
	if fromAccountID == toAccountID {
		return nil, fmt.Errorf("%w: transfer to the same account", ErrInvalidComponents)
	}
	return c.entry(BizTransfer,
		Line{Account: CashSubject(toAccountID), Debit: amount},
		Line{Account: CashSubject(fromAccountID), Credit: amount})
}

// RepayCredit: 借 信用负债 / 贷 现金. Settlement only, NEVER a second expense.
func RepayCredit(c Context, accountID, instrumentID string, amount money.Minor) (*Entry, error) {
	if err := money.MustPositive(amount); err != nil {
		return nil, err
	}
	return c.entry(BizCreditRepayment,
		Line{Account: CreditSubject(instrumentID), Debit: amount},
		Line{Account: CashSubject(accountID), Credit: amount})
}

// --- refunds ----------------------------------------------------------------

// RefundToCash: 借 现金 / 贷 原费用分类 (contra-expense, never ordinary income).
func RefundToCash(c Context, accountID, categoryID string, amount money.Minor) (*Entry, error) {
	if err := money.MustPositive(amount); err != nil {
		return nil, err
	}
	return c.entry(BizRefund,
		Line{Account: CashSubject(accountID), Debit: amount},
		Line{Account: ExpenseSubject(categoryID), Credit: amount})
}

// RefundToCredit: 借 信用负债 / 贷 原费用分类.
func RefundToCredit(c Context, instrumentID, categoryID string, amount money.Minor) (*Entry, error) {
	if err := money.MustPositive(amount); err != nil {
		return nil, err
	}
	return c.entry(BizRefund,
		Line{Account: CreditSubject(instrumentID), Debit: amount},
		Line{Account: ExpenseSubject(categoryID), Credit: amount})
}

// ValidateRefund returns the remaining refundable amount for a split line and
// rejects any refund that would exceed the original. Multiple partial refunds are allowed.
func ValidateRefund(originalTotal, alreadyRefunded, amount money.Minor) (money.Minor, error) {
	if err := money.MustPositive(amount); err != nil {
		return 0, err
	}
	if originalTotal <= 0 || alreadyRefunded < 0 {
		return 0, fmt.Errorf("%w: invalid refund state", ErrInvalidComponents)
	}
	if alreadyRefunded >= originalTotal {
		return 0, ErrRefundExceeds
	}
	remaining, err := money.Sub(originalTotal, alreadyRefunded)
	if err != nil {
		return 0, err
	}
	if amount > remaining {
		return remaining, ErrRefundExceeds
	}
	after, err := money.Sub(remaining, amount)
	if err != nil {
		return 0, err
	}
	return after, nil
}
