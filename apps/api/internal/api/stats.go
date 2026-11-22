package api

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tinynest/api/internal/db"
	"tinynest/api/internal/money"
)

// Stats are ALWAYS derived from journal_lines. The household scope is part of
// every SQL WHERE clause: balances are never aggregated globally and filtered in
// Go.
const (
	incomeAccountPattern  = "4010:%"
	expenseAccountPattern = "5010:%"
)

// statsPeriod returns (from, to) as inclusive YYYY-MM-DD bounds. Missing bounds
// default to the current business month. Both bounds are validated.
func (a *API) statsPeriod(r *http.Request) (from, to string, ok bool) {
	query := r.URL.Query()
	from, to = query.Get("from"), query.Get("to")
	if from == "" {
		from = a.businessToday().Format("2006-01") + "-01"
	}
	if to == "" {
		to = a.businessDate()
	}
	if !validDate(from) || !validDate(to) || from > to {
		return "", "", false
	}
	return from, to, true
}

// between sums income and expense for an inclusive date range, household-scoped
// in SQL and derived only from the journal.
func (a *API) between(ctx context.Context, q db.Queryer, householdID, from, to string) (income, expense money.Minor, err error) {
	var inc, exp int64
	err = q.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE WHEN l.account_code LIKE ? THEN l.credit_minor-l.debit_minor ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN l.account_code LIKE ? THEN l.debit_minor-l.credit_minor ELSE 0 END),0)
		FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id
		WHERE e.household_id=? AND e.occurred_on>=? AND e.occurred_on<=?`,
		incomeAccountPattern, expenseAccountPattern, householdID, from, to).Scan(&inc, &exp)
	if err != nil {
		return 0, 0, err
	}
	return money.Minor(inc), money.Minor(exp), nil
}

func (a *API) handleStatsSummary(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	from, to, ok := a.statsPeriod(r)
	if !ok {
		writeErr(w, 400, "invalid_request", "from/to 须为 YYYY-MM-DD 且 from<=to")
		return
	}
	income, expense, err := a.between(r.Context(), a.DB, p.HouseholdID, from, to)
	if err != nil {
		internalErr(w, err)
		return
	}
	balance, err := money.Sub(income, expense)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"from": from, "to": to,
		"income_minor": income, "expense_minor": expense, "balance_minor": balance,
	})
}

func (a *API) handleStatsCategories(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	from, to, ok := a.statsPeriod(r)
	if !ok {
		writeErr(w, 400, "invalid_request", "from/to 须为 YYYY-MM-DD 且 from<=to")
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "expense"
	}
	var pattern string
	var sign string
	switch kind {
	case "expense":
		pattern, sign = expenseAccountPattern, "l.debit_minor-l.credit_minor"
	case "income":
		pattern, sign = incomeAccountPattern, "l.credit_minor-l.debit_minor"
	default:
		writeErr(w, 400, "invalid_request", "kind 必须为 expense/income")
		return
	}
	// SUBSTR(account_code, 6) extracts "<category_id>" from "5010:<category_id>";
	// it works identically in SQLite and MySQL. HAVING SUM(...) <> 0 drops
	// categories whose flows were fully reversed (they net to exactly zero).
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT SUBSTR(l.account_code, 6) AS category_id, COALESCE(c.name,''), COALESCE(c.emoji,''), COALESCE(c.color,''),
		       COALESCE(SUM(`+sign+`),0) AS amount_minor
		FROM journal_lines l
		JOIN journal_entries e ON e.id=l.entry_id
		LEFT JOIN categories c ON c.id=SUBSTR(l.account_code, 6) AND c.household_id=e.household_id
		WHERE e.household_id=? AND e.occurred_on>=? AND e.occurred_on<=? AND l.account_code LIKE ?
		GROUP BY SUBSTR(l.account_code, 6), c.name, c.emoji, c.color
		HAVING SUM(`+sign+`) <> 0
		ORDER BY amount_minor DESC, category_id`, p.HouseholdID, from, to, pattern)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		CategoryID  string `json:"category_id"`
		Name        string `json:"name"`
		Emoji       string `json:"emoji"`
		Color       string `json:"color"`
		AmountMinor int64  `json:"amount_minor"`
		ShareBP     int64  `json:"share_bp"`
	}
	amounts := []item{}
	var total int64
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.CategoryID, &it.Name, &it.Emoji, &it.Color, &it.AmountMinor); err != nil {
			internalErr(w, err)
			return
		}
		total += it.AmountMinor
		amounts = append(amounts, it)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	for i := range amounts {
		if total > 0 {
			amounts[i].ShareBP = amounts[i].AmountMinor * 10000 / total
		}
	}
	writeJSON(w, 200, map[string]any{"from": from, "to": to, "kind": kind, "items": amounts})
}

func parseMonth(value string) (time.Time, bool) {
	month, err := time.Parse("2006-01", value)
	if err != nil {
		return time.Time{}, false
	}
	return month, true
}

func monthBounds(month time.Time) (from, toExclusive string) {
	return month.Format("2006-01-02"), month.AddDate(0, 1, 0).Format("2006-01-02")
}

func (a *API) handleStatsDaily(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	monthValue := r.URL.Query().Get("month")
	if monthValue == "" {
		monthValue = a.businessToday().Format("2006-01")
	}
	month, ok := parseMonth(monthValue)
	if !ok {
		writeErr(w, 400, "invalid_request", "month 须为 YYYY-MM")
		return
	}
	from, toExclusive := monthBounds(month)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT e.occurred_on,
		       COALESCE(SUM(CASE WHEN l.account_code LIKE ? THEN l.debit_minor-l.credit_minor ELSE 0 END),0) AS expense_minor,
		       COALESCE(SUM(CASE WHEN l.account_code LIKE ? THEN l.credit_minor-l.debit_minor ELSE 0 END),0) AS income_minor
		FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id
		WHERE e.household_id=? AND e.occurred_on>=? AND e.occurred_on<?
		GROUP BY e.occurred_on
		HAVING expense_minor<>0 OR income_minor<>0
		ORDER BY e.occurred_on`,
		expenseAccountPattern, incomeAccountPattern, p.HouseholdID, from, toExclusive)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	type day struct {
		Day          string `json:"day"`
		ExpenseMinor int64  `json:"expense_minor"`
		IncomeMinor  int64  `json:"income_minor"`
	}
	days := []day{}
	for rows.Next() {
		var d day
		if err := rows.Scan(&d.Day, &d.ExpenseMinor, &d.IncomeMinor); err != nil {
			internalErr(w, err)
			return
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"month": monthValue, "days": days})
}

func (a *API) handleStatsMonthly(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	toValue := r.URL.Query().Get("to")
	if toValue == "" {
		toValue = a.businessToday().Format("2006-01")
	}
	toMonth, ok := parseMonth(toValue)
	if !ok {
		writeErr(w, 400, "invalid_request", "to 须为 YYYY-MM")
		return
	}
	count := 6
	if raw := r.URL.Query().Get("months"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 120 {
			writeErr(w, 400, "invalid_request", "months 须为 1 至 120")
			return
		}
		count = n
	}
	fromMonth := toMonth.AddDate(0, -(count - 1), 0)
	from, _ := monthBounds(fromMonth)
	_, toExclusive := monthBounds(toMonth)

	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT SUBSTR(e.occurred_on,1,7) AS month,
		       COALESCE(SUM(CASE WHEN l.account_code LIKE ? THEN l.credit_minor-l.debit_minor ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN l.account_code LIKE ? THEN l.debit_minor-l.credit_minor ELSE 0 END),0)
		FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id
		WHERE e.household_id=? AND e.occurred_on>=? AND e.occurred_on<?
		GROUP BY SUBSTR(e.occurred_on,1,7) ORDER BY month`,
		incomeAccountPattern, expenseAccountPattern, p.HouseholdID, from, toExclusive)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	type summary struct{ income, expense int64 }
	byMonth := map[string]summary{}
	for rows.Next() {
		var month string
		var entry summary
		if err := rows.Scan(&month, &entry.income, &entry.expense); err != nil {
			internalErr(w, err)
			return
		}
		byMonth[month] = entry
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	type monthRow struct {
		Month        string `json:"month"`
		IncomeMinor  int64  `json:"income_minor"`
		ExpenseMinor int64  `json:"expense_minor"`
	}
	months := make([]monthRow, 0, count)
	for i := 0; i < count; i++ {
		value := fromMonth.AddDate(0, i, 0).Format("2006-01")
		entry := byMonth[value]
		months = append(months, monthRow{Month: value, IncomeMinor: entry.income, ExpenseMinor: entry.expense})
	}
	writeJSON(w, 200, map[string]any{"months": months})
}

// --- budgets ----------------------------------------------------------------

func normalizeMonth(value string) (string, bool) {
	if _, ok := parseMonth(value); !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

// monthExpense sums the month's expense from journal lines (household-scoped in SQL).
func (a *API) monthExpense(ctx context.Context, householdID, monthValue string) (money.Minor, error) {
	month, _ := parseMonth(monthValue)
	from, toExclusive := monthBounds(month)
	var value int64
	err := a.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(l.debit_minor-l.credit_minor),0)
		FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id
		WHERE e.household_id=? AND e.occurred_on>=? AND e.occurred_on<? AND l.account_code LIKE ?`,
		householdID, from, toExclusive, expenseAccountPattern).Scan(&value)
	return money.Minor(value), err
}

func (a *API) handleGetBudget(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	month, ok := normalizeMonth(r.PathValue("month"))
	if !ok {
		writeErr(w, 400, "invalid_request", "month 须为 YYYY-MM")
		return
	}
	var amount int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT amount_minor FROM budgets WHERE household_id=? AND month=?`, p.HouseholdID, month).Scan(&amount)
	var amountValue any
	switch {
	case err == nil:
		amountValue = amount
	case err != nil && !isNoRows(err):
		internalErr(w, err)
		return
	}
	spent, err := a.monthExpense(r.Context(), p.HouseholdID, month)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"month": month, "amount_minor": amountValue, "spent_minor": spent})
}

func (a *API) handlePutBudget(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	month, ok := normalizeMonth(r.PathValue("month"))
	if !ok {
		writeErr(w, 400, "invalid_request", "month 须为 YYYY-MM")
		return
	}
	var in struct {
		AmountMinor int64 `json:"amount_minor"`
	}
	if err := decode(r, &in); err != nil || in.AmountMinor < 0 {
		writeErr(w, 400, "invalid_request", "amount_minor 须为非负整数分")
		return
	}
	statement := `INSERT INTO budgets(household_id,month,amount_minor) VALUES(?,?,?)
		ON CONFLICT(household_id,month) DO UPDATE SET amount_minor=excluded.amount_minor`
	if a.DB.IsMySQL() {
		statement = `INSERT INTO budgets(household_id,month,amount_minor) VALUES(?,?,?)
			ON DUPLICATE KEY UPDATE amount_minor=VALUES(amount_minor)`
	}
	if err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(), statement, p.HouseholdID, month, in.AmountMinor); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "set_budget", "budget", month, "{}")
	}); err != nil {
		internalErr(w, err)
		return
	}
	spent, err := a.monthExpense(r.Context(), p.HouseholdID, month)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"month": month, "amount_minor": in.AmountMinor, "spent_minor": spent})
}
