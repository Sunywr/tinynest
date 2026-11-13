package api

import (
	"net/http"

	"tinynest/api/internal/db"
)

// M5 credit dashboard: everything is rebuilt from journal facts + statement
// rows for the authenticated household; nothing is a cached balance.
//
//   - 应还/已还 are measured per STATEMENT due window (到期口径);
//   - 逾期 is derived (facts + business date), never read from a cache;
//   - limits come from the instrument row + the live liability balance.
func (a *API) handleDashboardCredit(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	from, to := a.period(r)
	ctx := r.Context()
	today := a.businessDate()

	// 到期口径：本期到期的账单，应还=系统应还，已还=本期实还，未还=应还-已还，
	// 逾期=已到期仍未还清的账单未还额。本金只确认一次（消费当月），还款与退款
	// 永远不再计入支出。
	var due, paid, overdue int64
	overdueCount := 0
	rows, err := a.DB.QueryContext(ctx, `
		SELECT s.due_on,s.system_due_minor,s.paid_minor,s.cycle_end_on
		FROM credit_statements s JOIN credit_instruments i ON i.id=s.instrument_id
		WHERE s.household_id=? AND s.due_on>=? AND s.due_on<=?`, p.HouseholdID, from, to)
	if err != nil {
		internalErr(w, err)
		return
	}
	for rows.Next() {
		var dueOn, cycleEnd string
		var sysDue, paidMinor int64
		if err := rows.Scan(&dueOn, &sysDue, &paidMinor, &cycleEnd); err != nil {
			_ = rows.Close()
			internalErr(w, err)
			return
		}
		due += sysDue
		paid += paidMinor
		if deriveStatementStatus(cycleEnd, dueOn, today, sysDue, paidMinor) == "overdue" {
			overdue += sysDue - paidMinor
			overdueCount++
		}
	}
	_ = rows.Close()

	// 信用额度：已用=信用负债科目实时余额；未知额度不得算出假百分比。
	instruments := []map[string]any{}
	irows, err := a.DB.QueryContext(ctx,
		`SELECT id,name,kind,credit_limit_minor FROM credit_instruments
		 WHERE household_id=? AND status='active' ORDER BY created_at`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	type inst struct {
		id, name, kind string
		limit          *int64
	}
	var list []inst
	for irows.Next() {
		var v inst
		var lim *int64
		if err := irows.Scan(&v.id, &v.name, &v.kind, &lim); err != nil {
			_ = irows.Close()
			internalErr(w, err)
			return
		}
		v.limit = lim
		list = append(list, v)
	}
	_ = irows.Close()
	var totalLimit, totalUsed int64
	limitKnown := false
	for _, v := range list {
		bal, err := db.Balance(ctx, a.DB, p.HouseholdID, "2010:"+v.id)
		if err != nil {
			internalErr(w, err)
			return
		}
		used := -int64(bal)
		if used < 0 {
			used = 0 // a credit balance (prepaid) is not "used limit"
		}
		entry := map[string]any{
			"id": v.id, "name": v.name, "kind": v.kind, "used_minor": used,
		}
		totalUsed += used
		if v.limit != nil && *v.limit > 0 {
			available := *v.limit - used
			if available < 0 {
				available = 0
			}
			entry["limit_minor"] = *v.limit
			entry["available_minor"] = available
			entry["usage_bps"] = int64(10000) * used / *v.limit
			totalLimit += *v.limit
			limitKnown = true
		} else {
			entry["limit_minor"] = nil
			entry["available_minor"] = nil
			entry["usage_bps"] = nil
		}
		instruments = append(instruments, entry)
	}
	creditSummary := map[string]any{"used_minor": totalUsed, "instruments": instruments}
	if limitKnown {
		available := totalLimit - totalUsed
		if available < 0 {
			available = 0
		}
		creditSummary["limit_minor"] = totalLimit
		creditSummary["available_minor"] = available
		creditSummary["usage_bps"] = int64(10000) * totalUsed / totalLimit
	} else {
		creditSummary["limit_minor"] = nil
		creditSummary["available_minor"] = nil
		creditSummary["usage_bps"] = nil
	}

	writeJSON(w, 200, map[string]any{
		"period": map[string]string{"from": from, "to": to, "basis": "Asia/Shanghai"},
		// 到期口径（按账单 due_on 归属）
		"due_minor": due, "paid_minor": paid, "unpaid_minor": due - paid,
		"overdue_minor": overdue, "overdue_count": overdueCount,
		// 额度口径（实时负债余额）
		"credit": creditSummary,
	})
}
