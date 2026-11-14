package ledger

import (
	"errors"
	"fmt"

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

// --- S1/S2: openings, daily transactions -----------------------------------

// OpeningBalance: 借 资产科目 / 贷 期初权益. Opening never touches income.
func OpeningBalance(c Context, assetSubject string, amount money.Minor) (*Entry, error) {
	if err := money.MustPositive(amount); err != nil {
		return nil, err
	}
	return c.entry(BizOpeningCash,
		Line{Account: assetSubject, Debit: amount},
		Line{Account: SubjOpeningEquity, Credit: amount})
}

// OpeningCash is the cash-account specialisation of OpeningBalance.
func OpeningCash(c Context, accountID string, amount money.Minor) (*Entry, error) {
	return OpeningBalance(c, CashSubject(accountID), amount)
}

// OpeningLiability: 借 期初权益 / 贷 信用负债. Opening never touches expense.
func OpeningLiability(c Context, instrumentID string, amount money.Minor) (*Entry, error) {
	if err := money.MustPositive(amount); err != nil {
		return nil, err
	}
	return c.entry(BizOpeningLiability,
		Line{Account: SubjOpeningEquity, Debit: amount},
		Line{Account: CreditSubject(instrumentID), Credit: amount})
}

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
func CreditExpense(c Context, instrumentID string, splits []Split, total money.Minor) (*Entry, error) {
	if err := money.MustPositive(total); err != nil {
		return nil, err
	}
	if err := validateSplits(splits, total); err != nil {
		return nil, err
	}
	lines := append(expenseLines(splits), Line{Account: CreditSubject(instrumentID), Credit: total})
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

// --- S4: refunds ------------------------------------------------------------

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

// --- S5: investments --------------------------------------------------------

// InvestBuy: 借 持仓(成交+费用) / 贷 资金账户. Asset conversion, NOT consumption.
func InvestBuy(c Context, cashSubject, holdingSubject string, gross, fee money.Minor) (*Entry, error) {
	if err := money.MustPositive(gross); err != nil {
		return nil, err
	}
	if fee < 0 {
		return nil, fmt.Errorf("%w: negative buy fee", ErrInvalidComponents)
	}
	total, err := money.Add(gross, fee)
	if err != nil {
		return nil, err
	}
	return c.entry(BizInvestBuy,
		Line{Account: holdingSubject, Debit: total},
		Line{Account: cashSubject, Credit: total})
}

// InvestSell: 借 资金账户(净收入) [+ 借 已实现损失] / 贷 持仓(结转成本) [+ 贷 已实现收益].
func InvestSell(c Context, cashSubject, holdingSubject string, net, carryCost money.Minor) (*Entry, error) {
	if err := money.MustPositive(net); err != nil {
		return nil, err
	}
	if carryCost <= 0 {
		return nil, fmt.Errorf("%w: carry cost must be positive", ErrInvalidComponents)
	}
	lines := []Line{{Account: cashSubject, Debit: net}}
	switch {
	case net > carryCost:
		lines = append(lines,
			Line{Account: holdingSubject, Credit: carryCost},
			Line{Account: SubjRealizedGain, Credit: net - carryCost})
	case net < carryCost:
		lines = append(lines,
			Line{Account: SubjRealizedLoss, Debit: carryCost - net},
			Line{Account: holdingSubject, Credit: carryCost})
	default:
		lines = append(lines, Line{Account: holdingSubject, Credit: carryCost})
	}
	return c.entry(BizInvestSell, lines...)
}

// Dividend records a cash dividend. A nil tax means "unknown" and is never treated as zero:
// only the actually received amount is posted, and the business row keeps tax unknown.
func Dividend(c Context, toSubject string, gross money.Minor, tax *money.Minor, net money.Minor) (*Entry, error) {
	if err := money.MustPositive(net); err != nil {
		return nil, err
	}
	lines := []Line{{Account: toSubject, Debit: net}}
	if tax == nil {
		lines = append(lines, Line{Account: SubjIncomeDividend, Credit: net})
		return c.entry(BizDividend, lines...)
	}
	if *tax < 0 {
		return nil, fmt.Errorf("%w: negative dividend tax", ErrInvalidComponents)
	}
	if gross != net+*tax {
		return nil, fmt.Errorf("%w: gross(%d) != net(%d) + tax(%d)", ErrInvalidComponents, gross, net, *tax)
	}
	if *tax > 0 {
		lines = append(lines, Line{Account: SubjDividendTax, Debit: *tax})
	}
	lines = append(lines, Line{Account: SubjIncomeDividend, Credit: gross})
	return c.entry(BizDividend, lines...)
}

// --- S4: installments, loans, insurance ------------------------------------

// LoanPayment: 借 贷款本金 + 借 利息费用 / 贷 付款渠道(现金或信用工具).
func LoanPayment(c Context, contractID, paySubject string, principal, interest money.Minor) (*Entry, error) {
	if principal <= 0 || interest < 0 {
		return nil, fmt.Errorf("%w: principal>0 and interest>=0 required", ErrInvalidComponents)
	}
	total, err := money.Add(principal, interest)
	if err != nil {
		return nil, err
	}
	lines := []Line{{Account: LoanSubject(contractID), Debit: principal}}
	if interest > 0 {
		lines = append(lines, Line{Account: SubjExpenseInterest, Debit: interest})
	}
	lines = append(lines, Line{Account: paySubject, Credit: total})
	return c.entry(BizLoanPayment, lines...)
}

// InstallmentCreate recognises the full original consumption once and books the principal liability.
// Fees are NOT accrued upfront; they are recognised period by period.
func InstallmentCreate(c Context, contractID, categoryID string, principal money.Minor) (*Entry, error) {
	if err := money.MustPositive(principal); err != nil {
		return nil, err
	}
	return c.entry(BizInstallmentNew,
		Line{Account: ExpenseSubject(categoryID), Debit: principal},
		Line{Account: InstallmentSubject(contractID), Credit: principal})
}

// InstallmentPayment settles principal + recognises the period fee.
func InstallmentPayment(c Context, contractID, paySubject string, principal, fee money.Minor) (*Entry, error) {
	if principal <= 0 || fee < 0 {
		return nil, fmt.Errorf("%w: principal>0 and fee>=0 required", ErrInvalidComponents)
	}
	total, err := money.Add(principal, fee)
	if err != nil {
		return nil, err
	}
	lines := []Line{{Account: InstallmentSubject(contractID), Debit: principal}}
	if fee > 0 {
		lines = append(lines, Line{Account: SubjExpenseFee, Debit: fee})
	}
	lines = append(lines, Line{Account: paySubject, Credit: total})
	return c.entry(BizInstallmentPay, lines...)
}

// OpeningLoan books an existing loan principal against opening equity. It is
// never income and never an expense: the liability already existed before the
// ledger was opened (V0.1 plan 5.4).
func OpeningLoan(c Context, contractID string, amount money.Minor) (*Entry, error) {
	if err := money.MustPositive(amount); err != nil {
		return nil, err
	}
	return c.entry(BizOpeningLiability,
		Line{Account: SubjOpeningEquity, Debit: amount},
		Line{Account: LoanSubject(contractID), Credit: amount})
}

// DebtPaymentSpec is an EXPLICIT user-confirmed split. V0.1.1 deliberately does
// not infer any allocation order (10 accounting policies are still unconfirmed).
type DebtPaymentSpec struct {
	Principal money.Minor
	Interest  money.Minor
	Fee       money.Minor
}

// DebtPayment settles a fixed obligation. Principal reduces the loan liability,
// interest and fees are expenses, and an insurance premium never creates a
// fictitious loan principal. The payment channel may be cash or a credit tool.
func DebtPayment(c Context, contractID, paySubject, kind string, spec DebtPaymentSpec) (*Entry, error) {
	if spec.Principal < 0 || spec.Interest < 0 || spec.Fee < 0 {
		return nil, fmt.Errorf("%w: negative payment component", ErrInvalidComponents)
	}
	total, err := money.Add(spec.Principal, spec.Interest)
	if err != nil {
		return nil, err
	}
	total, err = money.Add(total, spec.Fee)
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, fmt.Errorf("%w: zero-value payment", ErrInvalidComponents)
	}
	if kind == "insurance" && spec.Principal != 0 {
		return nil, fmt.Errorf("%w: insurance must not create loan principal", ErrInvalidComponents)
	}
	lines := []Line{}
	if spec.Principal > 0 {
		lines = append(lines, Line{Account: LoanSubject(contractID), Debit: spec.Principal})
	}
	if spec.Interest > 0 {
		lines = append(lines, Line{Account: SubjExpenseInterest, Debit: spec.Interest})
	}
	if spec.Fee > 0 {
		feeSubject := SubjExpenseFee
		if kind == "insurance" {
			feeSubject = SubjExpenseInsurance
		}
		lines = append(lines, Line{Account: feeSubject, Debit: spec.Fee})
	}
	lines = append(lines, Line{Account: paySubject, Credit: total})
	return c.entry(BizLoanPayment, lines...)
}

// InstallmentPaymentWithSplit settles a period with an EXPLICIT split supplied
// by the caller. Unlike InstallmentPayment it accepts principal == 0 (a fee-only
// partial payment), which V0.1.1 must never infer automatically.
func InstallmentPaymentWithSplit(c Context, contractID, paySubject string, principal, fee money.Minor) (*Entry, error) {
	if principal < 0 || fee < 0 {
		return nil, fmt.Errorf("%w: negative split component", ErrInvalidComponents)
	}
	total, err := money.Add(principal, fee)
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, fmt.Errorf("%w: zero-value payment", ErrInvalidComponents)
	}
	lines := []Line{}
	if principal > 0 {
		lines = append(lines, Line{Account: InstallmentSubject(contractID), Debit: principal})
	}
	if fee > 0 {
		lines = append(lines, Line{Account: SubjExpenseFee, Debit: fee})
	}
	lines = append(lines, Line{Account: paySubject, Credit: total})
	return c.entry(BizInstallmentPay, lines...)
}

// ConvertToInstallment reclassifies an existing credit-card balance into installment debt.
// No expense is created and the principal can never exist in both liabilities.
func ConvertToInstallment(c Context, instrumentID, contractID string, principal money.Minor) (*Entry, error) {
	if err := money.MustPositive(principal); err != nil {
		return nil, err
	}
	return c.entry(BizConvertInstall,
		Line{Account: CreditSubject(instrumentID), Debit: principal},
		Line{Account: InstallmentSubject(contractID), Credit: principal})
}

// InsurancePayment: 借 保险费用 / 贷 付款渠道. No fictitious loan principal.
func InsurancePayment(c Context, paySubject string, amount money.Minor) (*Entry, error) {
	if err := money.MustPositive(amount); err != nil {
		return nil, err
	}
	return c.entry(BizInsurance,
		Line{Account: SubjExpenseInsurance, Debit: amount},
		Line{Account: paySubject, Credit: amount})
}
