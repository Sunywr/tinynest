package ledger

// Chart of accounts for the V2 bookkeeping module (first digit: 1 asset,
// 2 liability, 3 equity, 4 income, 5 expense). A concrete account is addressed
// as "<code>:<business id>", e.g. "1010:<account_id>".
const (
	SubjCash          = "1010"
	SubjCreditDebt    = "2010"
	SubjOpeningEquity = "3010"
	SubjIncomeBase    = "4010"
	SubjExpenseBase   = "5010"
)

// Business types recorded on journal entries.
const (
	BizOpeningCash      = "opening_cash"
	BizOpeningLiability = "opening_liability"
	BizExpense          = "expense"
	BizIncome           = "income"
	BizTransfer         = "transfer"
	BizCreditRepayment  = "credit_repayment"
	BizRefund           = "refund"
)

// Subj composes a concrete ledger account code.
func Subj(code, ref string) string {
	if ref == "" {
		return code
	}
	return code + ":" + ref
}

// ExpenseSubject maps a category to an expense sub-account.
func ExpenseSubject(categoryID string) string { return Subj(SubjExpenseBase, categoryID) }

// IncomeSubject maps a category to an income sub-account.
func IncomeSubject(categoryID string) string { return Subj(SubjIncomeBase, categoryID) }

// CashSubject / CreditSubject describe the two V2 account classes.
func CashSubject(accountID string) string   { return Subj(SubjCash, accountID) }
func CreditSubject(accountID string) string { return Subj(SubjCreditDebt, accountID) }
