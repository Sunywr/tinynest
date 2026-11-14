package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"tinynest/api/internal/db"
)

func (a *API) registerV011Credit(mux *http.ServeMux) {
	a.route(mux, "PATCH /api/v1/credit-instruments/{id}", a.handlePatchInstrument)
	a.route(mux, "GET /api/v1/credit-instruments/{id}/limit", a.handleInstrumentLimit)
	a.route(mux, "GET /api/v1/credit-statements/{id}", a.handleGetStatement)
	a.route(mux, "PATCH /api/v1/credit-statements/{id}/cycle", a.handlePatchStatementCycle)
}

// deriveStatementStatus is a pure function: status comes from the reporting
// date plus posted amounts, never from a stale cached column. A period only
// becomes "billed" after its cycle end; an unpaid/partly paid period past its
// due date is "overdue".
func deriveStatementStatus(cycleEnd, dueOn, today string, sysDue, paid int64) string {
	if sysDue <= 0 {
		if today > cycleEnd {
			return "billed"
		}
		return "unbilled"
	}
	if paid >= sysDue {
		return "paid"
	}
	if today > dueOn {
		return "overdue"
	}
	if paid > 0 {
		return "partial"
	}
	if today > cycleEnd {
		return "billed"
	}
	return "unbilled"
}

// creditLimitFields never invents a percentage when the limit is unknown.
// All arithmetic is integer minor units; usage is basis points (10000 = 100%).
func creditLimitFields(limit *int64, outstanding int64) map[string]any {
	used := outstanding
	if used < 0 {
		used = 0
	}
	out := map[string]any{"used_minor": used}
	if limit == nil || *limit <= 0 {
		out["credit_limit_configured"] = false
		out["limit_display"] = "未配置"
		out["credit_limit_minor"] = limit
		out["available_minor"] = nil
		out["usage_bps"] = nil
		return out
	}
	available := *limit - used
	usageBps := (used*10000 + *limit/2) / *limit // half-up, integer only
	if usageBps > 10000 {
		usageBps = 10000
	}
	out["credit_limit_configured"] = true
	out["limit_display"] = "已配置"
	out["credit_limit_minor"] = *limit
	out["available_minor"] = available
	out["usage_bps"] = usageBps
	return out
}

func (a *API) handleInstrumentLimit(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var id, code string
	var limit *int64
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT id,ledger_code,credit_limit_minor FROM credit_instruments WHERE id=? AND household_id=?`,
		r.PathValue("id"), p.HouseholdID).Scan(&id, &code, &limit); err != nil {
		writeErr(w, 404, "not_found", "信用工具不存在")
		return
	}
	bal, err := db.Balance(r.Context(), a.DB, p.HouseholdID, code)
	if err != nil {
		internalErr(w, err)
		return
	}
	out := creditLimitFields(limit, -int64(bal))
	out["instrument_id"] = id
	out["outstanding_minor"] = -int64(bal)
	writeJSON(w, 200, out)
}

// handlePatchInstrument edits configuration only. Issued statements keep their
// own stored cycle/due dates, so changing a rule never silently re-computes
// confirmed history. Every change is written to an append-only audit table.
func (a *API) handlePatchInstrument(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var in struct {
		Name             *string `json:"name"`
		Issuer           *string `json:"issuer"`
		CreditLimitMinor *int64  `json:"credit_limit_minor"`
		ClearCreditLimit bool    `json:"clear_credit_limit"`
		CycleStartDay    *int    `json:"cycle_start_day"`
		CycleEndDay      *int    `json:"cycle_end_day"`
		DueDay           *int    `json:"due_day"`
		DueMonthOffset   *int    `json:"due_month_offset"`
		Status           *string `json:"status"`
		Note             *string `json:"note"`
		Reason           *string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	for _, d := range []*int{in.CycleStartDay, in.CycleEndDay, in.DueDay} {
		if d != nil && (*d < 1 || *d > 31) {
			writeErr(w, 400, "invalid_request", "账单日须在 1..31")
			return
		}
	}
	if in.DueMonthOffset != nil && (*in.DueMonthOffset < 0 || *in.DueMonthOffset > 12) {
		writeErr(w, 400, "invalid_request", "还款月偏移须在 0..12")
		return
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "inactive" {
		writeErr(w, 400, "invalid_request", "status 仅支持 active/inactive")
		return
	}
	if in.CreditLimitMinor != nil && *in.CreditLimitMinor <= 0 {
		writeErr(w, 400, "invalid_amount", "信用额度必须为正整数分")
		return
	}
	if in.Name != nil && *in.Name == "" {
		writeErr(w, 400, "invalid_request", "名称不能为空")
		return
	}

	changed := map[string]any{}
	if in.Name != nil {
		changed["name"] = *in.Name
	}
	if in.Issuer != nil {
		changed["issuer"] = *in.Issuer
	}
	if in.CreditLimitMinor != nil {
		changed["credit_limit_minor"] = *in.CreditLimitMinor
	}
	if in.ClearCreditLimit {
		changed["credit_limit_minor"] = "cleared"
	}
	if in.CycleStartDay != nil {
		changed["cycle_start_day"] = *in.CycleStartDay
	}
	if in.CycleEndDay != nil {
		changed["cycle_end_day"] = *in.CycleEndDay
	}
	if in.DueDay != nil {
		changed["due_day"] = *in.DueDay
	}
	if in.DueMonthOffset != nil {
		changed["due_month_offset"] = *in.DueMonthOffset
	}
	if in.Status != nil {
		changed["status"] = *in.Status
	}
	if in.Note != nil {
		changed["note"] = *in.Note
	}
	if len(changed) == 0 {
		writeErr(w, 400, "invalid_request", "没有需要修改的字段")
		return
	}
	changedJSON, _ := json.Marshal(changed)
	clear := 0
	if in.ClearCreditLimit {
		clear = 1
	}
	reason := ""
	if in.Reason != nil {
		reason = *in.Reason
	}
	finalStatus := "active"

	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(r.Context(),
			`SELECT status FROM credit_instruments WHERE id=? AND household_id=?`, id, p.HouseholdID).
			Scan(&finalStatus); err != nil {
			return errors.New("信用工具不存在")
		}
		res, err := tx.ExecContext(r.Context(), `
			UPDATE credit_instruments SET
			  name=COALESCE(?,name), issuer=COALESCE(?,issuer),
			  credit_limit_minor=CASE WHEN ?=1 THEN NULL ELSE COALESCE(?,credit_limit_minor) END,
			  cycle_start_day=COALESCE(?,cycle_start_day), cycle_end_day=COALESCE(?,cycle_end_day),
			  due_day=COALESCE(?,due_day), due_month_offset=COALESCE(?,due_month_offset),
			  status=COALESCE(?,status), note=COALESCE(?,note)
			WHERE id=? AND household_id=?`,
			ptrArg(in.Name), ptrArg(in.Issuer), clear, ptrArg(in.CreditLimitMinor),
			ptrArg(in.CycleStartDay), ptrArg(in.CycleEndDay), ptrArg(in.DueDay), ptrArg(in.DueMonthOffset),
			ptrArg(in.Status), ptrArg(in.Note), id, p.HouseholdID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errors.New("信用工具不存在")
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO credit_instrument_changes(id,household_id,instrument_id,changed_fields,reason,actor)
			VALUES(?,?,?,?,?,?)`,
			newID("cichg"), p.HouseholdID, id, string(changedJSON), reason, p.Username); err != nil {
			return err
		}
		if in.Status != nil {
			finalStatus = *in.Status
		}
		return audit(r.Context(), tx, p, "patch_credit_instrument", "credit_instrument", id, string(changedJSON))
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "status": finalStatus, "changed_fields": changed,
		"note": "仅修改配置；已出账历史账单的周期、还款日与金额不会被重算",
	})
}

// statementDetail rebuilds one statement from business facts. It intentionally
// returns derived_status alongside stored_status so a stale column can never
// contradict the journal.
func (a *API) statementDetail(ctx context.Context, q db.Queryer, householdID, stmtID string) (map[string]any, error) {
	var instrumentID, name, start, end, due, stored string
	var sysDue, paid int64
	var platform *int64
	if err := q.QueryRowContext(ctx, `
		SELECT s.instrument_id,i.name,s.cycle_start_on,s.cycle_end_on,s.due_on,
		       s.system_due_minor,s.paid_minor,s.platform_amount_minor,s.status
		FROM credit_statements s JOIN credit_instruments i ON i.id=s.instrument_id
		WHERE s.id=? AND s.household_id=?`, stmtID, householdID).
		Scan(&instrumentID, &name, &start, &end, &due, &sysDue, &paid, &platform, &stored); err != nil {
		return nil, err
	}
	out := map[string]any{
		"id": stmtID, "instrument_id": instrumentID, "instrument_name": name,
		"cycle_start_on": start, "cycle_end_on": end, "due_on": due,
		"system_due_minor": sysDue, "paid_minor": paid, "remaining_minor": sysDue - paid,
		"platform_amount_minor": platform,
		"status":                deriveStatementStatus(end, due, a.businessDate(), sysDue, paid),
		"derived_status":        deriveStatementStatus(end, due, a.businessDate(), sysDue, paid),
		"stored_status":         stored,
	}
	if platform != nil {
		out["reconcile_diff_minor"] = sysDue - *platform
	}
	rows, err := q.QueryContext(ctx,
		`SELECT source_type,source_id,amount_minor,occurred_on FROM credit_statement_items
		 WHERE statement_id=? ORDER BY occurred_on,id`, stmtID)
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for rows.Next() {
		var typ, sid, on string
		var amount int64
		if err := rows.Scan(&typ, &sid, &amount, &on); err != nil {
			_ = rows.Close()
			return nil, err
		}
		items = append(items, map[string]any{
			"source_type": typ, "source_id": sid, "amount_minor": amount, "occurred_on": on,
		})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out["items"] = items
	var adjCount int
	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM credit_statement_adjustments WHERE statement_id=? AND household_id=?`,
		stmtID, householdID).Scan(&adjCount); err != nil {
		return nil, err
	}
	out["adjustment_count"] = adjCount
	return out, nil
}

func (a *API) handleGetStatement(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	out, err := a.statementDetail(r.Context(), a.DB, p.HouseholdID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "not_found", "账单不存在")
			return
		}
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, out)
}

// handlePatchStatementCycle performs an explicit, audited manual correction of
// ONE issued statement's dates. It never touches system_due_minor, paid_minor,
// statement items or any journal line.
func (a *API) handlePatchStatementCycle(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	stmtID := r.PathValue("id")
	var in struct {
		CycleStartOn *string `json:"cycle_start_on"`
		CycleEndOn   *string `json:"cycle_end_on"`
		DueOn        *string `json:"due_on"`
		Reason       string  `json:"reason"`
	}
	if err := decode(r, &in); err != nil || in.Reason == "" {
		writeErr(w, 400, "invalid_request", "人工修正必须填写 reason")
		return
	}
	fields := []struct {
		name string
		val  *string
	}{
		{"cycle_start_on", in.CycleStartOn},
		{"cycle_end_on", in.CycleEndOn},
		{"due_on", in.DueOn},
	}
	for _, f := range fields {
		if f.val != nil && !validDate(*f.val) {
			writeErr(w, 400, "invalid_request", "日期须为 YYYY-MM-DD")
			return
		}
	}
	var changed []string
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var start, end, due string
		if err := tx.QueryRowContext(r.Context(),
			`SELECT cycle_start_on,cycle_end_on,due_on FROM credit_statements WHERE id=? AND household_id=?`,
			stmtID, p.HouseholdID).Scan(&start, &end, &due); err != nil {
			return errors.New("账单不存在")
		}
		newStart, newEnd, newDue := start, end, due
		if in.CycleStartOn != nil {
			newStart = *in.CycleStartOn
		}
		if in.CycleEndOn != nil {
			newEnd = *in.CycleEndOn
		}
		if in.DueOn != nil {
			newDue = *in.DueOn
		}
		if newStart > newEnd || newEnd > newDue {
			return errors.New("周期开始、周期结束、还款日顺序不正确")
		}

		old := map[string]string{"cycle_start_on": start, "cycle_end_on": end, "due_on": due}
		for _, f := range fields {
			if f.val == nil || *f.val == old[f.name] {
				continue
			}
			if _, err := tx.ExecContext(r.Context(), fmt.Sprintf(
				`UPDATE credit_statements SET %s=? WHERE id=? AND household_id=?`, f.name),
				*f.val, stmtID, p.HouseholdID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), `
				INSERT INTO credit_statement_adjustments(id,household_id,statement_id,field,old_value,new_value,reason,actor)
				VALUES(?,?,?,?,?,?,?,?)`,
				newID("stadj"), p.HouseholdID, stmtID, f.name, old[f.name], *f.val, in.Reason, p.Username); err != nil {
				return err
			}
			changed = append(changed, f.name)
		}
		return audit(r.Context(), tx, p, "adjust_statement_cycle", "credit_statement", stmtID, in.Reason)
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": stmtID, "status": "adjusted", "changed_fields": changed,
		"note": "仅修正日期；系统应还、已还、账单明细与分录均未改动",
	})
}
