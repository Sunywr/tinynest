package ledger

// Chart of accounts (first digit: 1 asset, 2 liability, 3 equity, 4 income, 5 expense).
// A concrete account is addressed as "<code>:<business id>", e.g. "1010:cash_01".
const (
	SubjCash             = "1010"
	SubjSecuritiesCash   = "1020"
	SubjWealthCash       = "1030"
	SubjHoldingStock     = "1040"
	SubjHoldingETF       = "1050"
	SubjHoldingWealth    = "1060"
	SubjCreditDebt       = "2010"
	SubjInstallmentDebt  = "2020"
	SubjLoanDebt         = "2030"
	SubjOpeningEquity    = "3010"
	SubjIncomeBase       = "4010"
	SubjIncomeDividend   = "4050"
	SubjRealizedGain     = "4060"
	SubjExpenseBase      = "5010"
	SubjExpenseInterest  = "5100"
	SubjExpenseFee       = "5110"
	SubjExpenseInsurance = "5120"
	SubjDividendTax      = "5130"
	SubjRealizedLoss     = "5190"
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
	BizInvestBuy        = "invest_buy"
	BizInvestSell       = "invest_sell"
	BizDividend         = "dividend"
	BizLoanPayment      = "loan_payment"
	BizInstallmentNew   = "installment_create"
	BizInstallmentPay   = "installment_payment"
	BizConvertInstall   = "convert_to_installment"
	BizInsurance        = "insurance_payment"
)

// Subj composes a concrete ledger account code.
func Subj(code, ref string) string {
	if ref == "" {
		return code
	}
	return code + ":" + ref
}

// ExpenseSubject maps a (possibly second-level) category to an expense sub-account.
func ExpenseSubject(categoryID string) string { return Subj(SubjExpenseBase, categoryID) }

// IncomeSubject maps a category to an income sub-account.
func IncomeSubject(categoryID string) string { return Subj(SubjIncomeBase, categoryID) }

// CashSubject / CreditSubject describe payment channels used by debt & installment rules.
func CashSubject(accountID string) string         { return Subj(SubjCash, accountID) }
func CreditSubject(instrumentID string) string    { return Subj(SubjCreditDebt, instrumentID) }
func LoanSubject(contractID string) string        { return Subj(SubjLoanDebt, contractID) }
func InstallmentSubject(contractID string) string { return Subj(SubjInstallmentDebt, contractID) }
