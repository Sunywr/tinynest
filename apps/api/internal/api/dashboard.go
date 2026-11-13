package api

import (
	"net/http"
	"time"

	"tinynest/api/internal/db"
)

func (a *API) registerDashboard(mux *http.ServeMux) {
	a.route(mux, "GET /api/v1/dashboard/summary", a.handleDashboardSummary)
	a.route(mux, "GET /api/v1/dashboard/categories", a.handleDashboardCategories)
	a.route(mux, "GET /api/v1/dashboard/projects", a.handleDashboardProjects)
}

// period resolves the reporting window. Business dates come from the injectable
// Asia/Shanghai clock, never from a direct wall-clock read (FIX-03).
func (a *API) period(r *http.Request) (string, string) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" || to == "" {
		now := a.businessToday()
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		to = time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	return from, to
}

// The three accounting views are deliberately computed from different sources:
// income/expense from P&L subjects, cashflow from cash asset subjects, due from schedule items.
func (a *API) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	from, to := a.period(r)
	ctx := r.Context()

	// 收支口径只统计普通收入/费用分类和真实费用科目。
	// 4050 分红、4060 已实现收益、5130 红利税、5190 已实现损失属于
	// 「投资总收益」口径，绝不能混入本期收入或本期支出。
	income, err := db.Aggregate(ctx, a.DB, p.HouseholdID, "4010:%", from, to)
	if err != nil {
		internalErr(w, err)
		return
	}
	expense := int64(0)
	for _, pattern := range []string{"5010:%", "5100", "5110", "5120"} {
		v, err := db.Aggregate(ctx, a.DB, p.HouseholdID, pattern, from, to)
		if err != nil {
			internalErr(w, err)
			return
		}
		expense += int64(v)
	}
	cash, err := db.Cashflow(ctx, a.DB, p.HouseholdID, from, to)
	if err != nil {
		internalErr(w, err)
		return
	}
	assets, err := db.Aggregate(ctx, a.DB, p.HouseholdID, "1%", "0000-01-01", to)
	if err != nil {
		internalErr(w, err)
		return
	}
	liabilities, err := db.Aggregate(ctx, a.DB, p.HouseholdID, "2%", "0000-01-01", to)
	if err != nil {
		internalErr(w, err)
		return
	}

	var dueInPeriod, due7, due30 int64
	_ = a.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_minor),0) FROM schedule_due_items
		WHERE household_id=? AND status='pending' AND due_on>=? AND due_on<=?`, p.HouseholdID, from, to).Scan(&dueInPeriod)
	today := a.businessToday().Format("2006-01-02")
	_ = a.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_minor),0) FROM schedule_due_items
		WHERE household_id=? AND status='pending' AND due_on>=? AND due_on<=?`,
		p.HouseholdID, today, a.businessToday().AddDate(0, 0, 7).Format("2006-01-02")).Scan(&due7)
	_ = a.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_minor),0) FROM schedule_due_items
		WHERE household_id=? AND status='pending' AND due_on>=? AND due_on<=?`,
		p.HouseholdID, today, a.businessToday().AddDate(0, 0, 30).Format("2006-01-02")).Scan(&due30)

	var realized, netDiv, cost, market int64
	_ = a.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(realized_pnl_minor),0), COALESCE(SUM(net_dividend_minor),0), COALESCE(SUM(total_cost_minor),0)
		FROM investment_positions p JOIN investment_assets a ON a.id=p.asset_id WHERE a.household_id=?`,
		p.HouseholdID).Scan(&realized, &netDiv, &cost)
	_ = a.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(v.market_value_minor),0) FROM investment_valuations v
		JOIN investment_assets a ON a.id=v.asset_id
		WHERE a.household_id=? AND v.valued_on = (
		  SELECT MAX(v2.valued_on) FROM investment_valuations v2 WHERE v2.asset_id=v.asset_id)`,
		p.HouseholdID).Scan(&market)
	unrealized := market - cost

	accounts := []map[string]any{}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id,name,ledger_code FROM accounts WHERE household_id=? AND status='active' ORDER BY created_at`, p.HouseholdID)
	if err == nil {
		type acc struct{ id, name, code string }
		var list []acc
		for rows.Next() {
			var v acc
			if rows.Scan(&v.id, &v.name, &v.code) == nil {
				list = append(list, v)
			}
		}
		_ = rows.Close() // release the single connection before balance queries
		for _, v := range list {
			bal, _ := db.Balance(ctx, a.DB, p.HouseholdID, v.code)
			accounts = append(accounts, map[string]any{"id": v.id, "name": v.name, "balance_minor": bal})
		}
	}

	writeJSON(w, 200, map[string]any{
		"period": map[string]string{"from": from, "to": to, "basis": "Asia/Shanghai"},
		// 口径一：收支（业务发生日）
		"income_minor": -int64(income), "expense_minor": expense,
		"balance_minor": -int64(income) - int64(expense),
		// 口径二：现金流（现金类资产实际收付日）
		"cashflow_minor": int64(cash),
		// 口径三：负债到期（合同约定应付日）
		"liabilities_due_minor": dueInPeriod,
		"due_7d_minor":          due7, "due_30d_minor": due30,
		// 投资与净资产
		"investment_realized_minor":     realized,
		"investment_unrealized_minor":   unrealized,
		"investment_dividend_minor":     netDiv,
		"investment_total_return_minor": realized + unrealized + netDiv,
		"investment_valuation_note":     "手工估值，非实时行情；该收益率不是年化收益率",
		"net_worth_minor":               int64(assets) + int64(liabilities),
		"accounts":                      accounts,
	})
}

func (a *API) handleDashboardCategories(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	from, to := a.period(r)
	// Roll every second-level split up to its top-level category; without this join
	// child categories would be silently dropped from the ranking.
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT COALESCE(c.parent_id, c.id) AS root_id,
		       COALESCE(parent.name, c.name) AS root_name,
		       SUM(s.amount_minor) AS total
		FROM transaction_splits s
		JOIN transactions t ON t.id=s.transaction_id AND t.status='posted' AND t.type='expense'
		  AND t.occurred_on>=? AND t.occurred_on<=?
		JOIN categories c ON c.id=s.category_id
		LEFT JOIN categories parent ON parent.id=c.parent_id
		WHERE t.household_id=? AND c.kind='expense'
		GROUP BY root_id, root_name ORDER BY total DESC`, from, to, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name string
		var total int64
		if err := rows.Scan(&id, &name, &total); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{"category_id": id, "name": name, "expense_minor": total})
	}
	writeJSON(w, 200, map[string]any{"period": map[string]string{"from": from, "to": to}, "items": items, "total": len(items)})
}

func (a *API) handleDashboardProjects(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	from, to := a.period(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT pr.id,pr.name,
		  COALESCE(SUM(CASE WHEN t.type='expense' THEN s.amount_minor ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN t.type='income'  THEN s.amount_minor ELSE 0 END),0)
		FROM projects pr
		LEFT JOIN transaction_splits s ON s.project_id=pr.id
		LEFT JOIN transactions t ON t.id=s.transaction_id AND t.status='posted'
		  AND t.occurred_on>=? AND t.occurred_on<=?
		WHERE pr.household_id=? GROUP BY pr.id ORDER BY pr.name`, from, to, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name string
		var expense, income int64
		if err := rows.Scan(&id, &name, &expense, &income); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"project_id": id, "name": name,
			"expense_minor": expense, "income_minor": income, "net_minor": income - expense,
		})
	}
	writeJSON(w, 200, map[string]any{"period": map[string]string{"from": from, "to": to}, "items": items, "total": len(items)})
}
