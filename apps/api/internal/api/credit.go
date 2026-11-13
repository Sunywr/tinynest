package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"tinynest/api/internal/credit"
	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

func (a *API) registerCredit(mux *http.ServeMux) {
	a.route(mux, "GET /api/v1/credit-instruments", a.handleListInstruments)
	a.route(mux, "POST /api/v1/credit-instruments", a.handleCreateInstrument)
	a.route(mux, "GET /api/v1/credit-statements", a.handleListStatements)
	a.route(mux, "POST /api/v1/credit-statements/{id}/payments", a.handleStatementPayment)
	a.route(mux, "POST /api/v1/credit-statements/{id}/reconcile", a.handleReconcile)
	a.route(mux, "GET /api/v1/installments", a.handleListInstallments)
	a.route(mux, "POST /api/v1/installments", a.handleCreateInstallment)
	a.route(mux, "POST /api/v1/installments/{id}/payments", a.handleInstallmentPayment)
	a.route(mux, "GET /api/v1/schedule/due-items", a.handleListDueItems)
	a.route(mux, "POST /api/v1/schedule/generate", a.handleGenerateDueItems)
}

// assignStatement folds a credit-side business event into its billing cycle.
// Items are keyed by (statement, source_type, source_id), so retries never double count.
func (a *API) assignStatement(r *http.Request, tx *sql.Tx, p Principal, instrumentID, sourceType, sourceID string, amount int64, occurredOn string) error {
	var startDay, endDay, dueDay, offset int
	if err := tx.QueryRowContext(r.Context(),
		`SELECT cycle_start_day,cycle_end_day,due_day,due_month_offset FROM credit_instruments WHERE id=? AND household_id=?`,
		instrumentID, p.HouseholdID).Scan(&startDay, &endDay, &dueDay, &offset); err != nil {
		return fmt.Errorf("信用工具不存在: %w", err)
	}
	anchor, err := time.Parse("2006-01-02", occurredOn)
	if err != nil {
		return err
	}
	cycle, err := credit.CycleFor(anchor, startDay, endDay, dueDay, offset)
	if err != nil {
		return err
	}
	start, end, due := cycle.Start.Format("2006-01-02"), cycle.End.Format("2006-01-02"), cycle.Due.Format("2006-01-02")
	if _, err := tx.ExecContext(r.Context(), `
		INSERT OR IGNORE INTO credit_statements
		  (id,household_id,instrument_id,cycle_start_on,cycle_end_on,due_on,system_due_minor,paid_minor,status)
		VALUES(?,?,?,?,?,?,0,0,'unbilled')`,
		newID("stmt"), p.HouseholdID, instrumentID, start, end, due); err != nil {
		return err
	}
	var stmtID string
	if err := tx.QueryRowContext(r.Context(),
		`SELECT id FROM credit_statements WHERE instrument_id=? AND cycle_start_on=?`, instrumentID, start).
		Scan(&stmtID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(r.Context(),
		`INSERT OR IGNORE INTO credit_statement_items(statement_id,source_type,source_id,amount_minor,occurred_on) VALUES(?,?,?,?,?)`,
		stmtID, sourceType, sourceID, amount, occurredOn); err != nil {
		return err
	}
	// Recompute from items instead of trusting a mutable running total.
	_, err = tx.ExecContext(r.Context(), `
		UPDATE credit_statements SET system_due_minor =
		  (SELECT COALESCE(SUM(amount_minor),0) FROM credit_statement_items WHERE statement_id=?)
		WHERE id=?`, stmtID, stmtID)
	return err
}

func (a *API) handleCreateInstrument(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Kind             string  `json:"kind"`
		Name             string  `json:"name"`
		Issuer           *string `json:"issuer"`
		CreditLimitMinor *int64  `json:"credit_limit_minor"`
		CycleStartDay    int     `json:"cycle_start_day"`
		CycleEndDay      int     `json:"cycle_end_day"`
		DueDay           int     `json:"due_day"`
		DueMonthOffset   int     `json:"due_month_offset"`
		Note             *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || in.Name == "" {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	if _, err := credit.ClampDay(2026, time.February, in.CycleStartDay); err != nil {
		writeErr(w, 400, "invalid_request", "账单开始日须在 1..31")
		return
	}
	if _, err := credit.ClampDay(2026, time.February, in.CycleEndDay); err != nil {
		writeErr(w, 400, "invalid_request", "账单结束日须在 1..31")
		return
	}
	if _, err := credit.ClampDay(2026, time.February, in.DueDay); err != nil {
		writeErr(w, 400, "invalid_request", "还款日须在 1..31")
		return
	}
	if in.DueMonthOffset < 0 || in.DueMonthOffset > 12 {
		writeErr(w, 400, "invalid_request", "还款月偏移须在 0..12")
		return
	}
	switch in.Kind {
	case "credit_card", "huabei", "meituan", "douyin", "custom":
	default:
		writeErr(w, 400, "invalid_request", "不支持的信用工具类型")
		return
	}
	id := newID("ci")
	code := ledger.CreditSubject(id)
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO credit_instruments(id,household_id,kind,name,issuer,credit_limit_minor,
			  cycle_start_day,cycle_end_day,due_day,due_month_offset,ledger_code,note)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, p.HouseholdID, in.Kind, in.Name, in.Issuer, in.CreditLimitMinor,
			in.CycleStartDay, in.CycleEndDay, in.DueDay, in.DueMonthOffset, code, in.Note); err != nil {
			return err
		}
		_, err := tx.ExecContext(r.Context(),
			`INSERT INTO ledger_accounts(id,household_id,code,name,category,ref_type,ref_id) VALUES(?,?,?,?,2,'credit_instrument',?)`,
			newID("la"), p.HouseholdID, code, in.Name, id)
		return err
	})
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "name": in.Name, "ledger_code": code})
}

func (a *API) handleListInstruments(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,kind,name,credit_limit_minor,cycle_start_day,cycle_end_day,due_day,due_month_offset,ledger_code,status
		 FROM credit_instruments WHERE household_id=? ORDER BY created_at`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	type instrumentRow struct {
		id, kind, name, code, status string
		limit                        *int64
		sd, ed, dd, off              int
	}
	var list []instrumentRow
	for rows.Next() {
		var v instrumentRow
		if err := rows.Scan(&v.id, &v.kind, &v.name, &v.limit, &v.sd, &v.ed, &v.dd, &v.off, &v.code, &v.status); err != nil {
			_ = rows.Close()
			internalErr(w, err)
			return
		}
		list = append(list, v)
	}
	if err := rows.Close(); err != nil {
		internalErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, v := range list {
		bal, err := db.Balance(r.Context(), a.DB, p.HouseholdID, v.code)
		if err != nil {
			internalErr(w, err)
			return
		}
		row := map[string]any{
			"id": v.id, "kind": v.kind, "name": v.name, "credit_limit_minor": v.limit,
			"cycle_start_day": v.sd, "cycle_end_day": v.ed, "due_day": v.dd, "due_month_offset": v.off,
			"status": v.status, "outstanding_minor": -bal,
		}
		// V0.1.1: used/available/usage are derived from the posted journal, not a
		// cached balance. An unconfigured limit never produces a fake percentage.
		for k, val := range creditLimitFields(v.limit, -int64(bal)) {
			row[k] = val
		}
		items = append(items, row)
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) handleListStatements(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	// Drill-down filters (M5): window on due_on, plus derived status filter.
	// Status filtering happens on the DERIVED value computed from facts, so a
	// stale cached status can never leak into the response.
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	status := q.Get("status")
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT s.id,s.instrument_id,i.name,s.cycle_start_on,s.cycle_end_on,s.due_on,
		       s.system_due_minor,s.paid_minor,s.platform_amount_minor,s.status
		FROM credit_statements s JOIN credit_instruments i ON i.id=s.instrument_id
		WHERE s.household_id=? AND (?='' OR s.due_on>=?) AND (?='' OR s.due_on<=?)
		ORDER BY s.cycle_start_on DESC`, p.HouseholdID, from, from, to, to)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, instrID, name, start, end, due, stored string
		var sysDue, paid int64
		var platform *int64
		if err := rows.Scan(&id, &instrID, &name, &start, &end, &due, &sysDue, &paid, &platform, &stored); err != nil {
			internalErr(w, err)
			return
		}
		// V0.1.1: status is derived from the business date + posted amounts.
		// stored_status is kept visible so a stale cached value can be diagnosed.
		derived := deriveStatementStatus(end, due, a.businessDate(), sysDue, paid)
		m := map[string]any{
			"id": id, "instrument_id": instrID, "instrument_name": name,
			"cycle_start_on": start, "cycle_end_on": end, "due_on": due,
			"system_due_minor": sysDue, "paid_minor": paid,
			"remaining_minor": sysDue - paid,
			"status":          derived, "derived_status": derived, "stored_status": stored,
			"platform_amount_minor": platform,
		}
		if platform != nil {
			m["reconcile_diff_minor"] = sysDue - *platform
		}
		if status != "" && derived != status {
			continue
		}
		items = append(items, m)
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) handleStatementPayment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	stmtID := r.PathValue("id")
	var in struct {
		AmountMinor   int64  `json:"amount_minor"`
		PaidOn        string `json:"paid_on"`
		FromAccountID string `json:"from_account_id"`
	}
	if err := decode(r, &in); err != nil || !validDate(in.PaidOn) {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	amount := money.Minor(in.AmountMinor)
	if err := money.MustPositive(amount); err != nil {
		writeErr(w, 400, "invalid_amount", "还款金额必须为正")
		return
	}
	// V0.1.1: replaying the same Idempotency-Key returns the FIRST result,
	// never a UNIQUE error and never a second journal entry.
	replayKey := scopedKey(p.HouseholdID, idempotencyKey(r))
	if replayKey != "" {
		var existingID, existingEntry string
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT id,entry_id FROM credit_payments WHERE idempotency_key=? AND household_id=?`,
			replayKey, p.HouseholdID).Scan(&existingID, &existingEntry); err == nil {
			writeJSON(w, 200, map[string]any{
				"id": existingID, "entry_id": existingEntry, "statement_id": stmtID,
				"status": "confirmed", "replayed": true,
			})
			return
		}
	}
	payID, entryID := newID("cpay"), newID("je")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var instrID, cycleEnd, dueOn string
		var sysDue, paid int64
		if err := tx.QueryRowContext(r.Context(),
			`SELECT instrument_id,cycle_end_on,due_on,system_due_minor,paid_minor FROM credit_statements WHERE id=? AND household_id=?`,
			stmtID, p.HouseholdID).Scan(&instrID, &cycleEnd, &dueOn, &sysDue, &paid); err != nil {
			return errors.New("账单不存在")
		}
		if int64(amount) > sysDue-paid {
			return errors.New("还款金额超过剩余应还")
		}
		if _, err := a.validateAccount(tx, r, p.HouseholdID, in.FromAccountID); err != nil {
			return errors.New("付款账户不存在")
		}
		e, err := ledger.RepayCredit(ledger.Context{EntryID: entryID, BizID: payID, OccurredOn: in.PaidOn},
			in.FromAccountID, instrID, amount)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, replayKey, p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO credit_payments(id,household_id,statement_id,amount_minor,paid_on,from_account_id,entry_id,idempotency_key)
			VALUES(?,?,?,?,?,?,?,?)`,
			payID, p.HouseholdID, stmtID, int64(amount), in.PaidOn, in.FromAccountID, entryID,
			nullableKey(replayKey)); err != nil {
			return err
		}
		// Status is derived from facts + business date, not a mutable running flag.
		newPaid := paid + int64(amount)
		newStatus := deriveStatementStatus(cycleEnd, dueOn, a.businessDate(), sysDue, newPaid)
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE credit_statements SET paid_minor=?, status=? WHERE id=?`,
			newPaid, newStatus, stmtID); err != nil {
			return err
		}
		if err := confirmDueItem(r.Context(), tx, p.HouseholdID, "credit_statement", stmtID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": payID, "entry_id": entryID, "statement_id": stmtID})
}

func (a *API) handleReconcile(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	stmtID := r.PathValue("id")
	var in struct {
		PlatformAmountMinor int64 `json:"platform_amount_minor"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE credit_statements SET platform_amount_minor=? WHERE id=? AND household_id=?`,
		in.PlatformAmountMinor, stmtID, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "not_found", "账单不存在")
		return
	}
	var sysDue int64
	_ = a.DB.QueryRowContext(r.Context(), `SELECT system_due_minor FROM credit_statements WHERE id=?`, stmtID).Scan(&sysDue)
	writeJSON(w, 200, map[string]any{
		"statement_id": stmtID, "system_due_minor": sysDue,
		"platform_amount_minor": in.PlatformAmountMinor,
		"diff_minor":            sysDue - in.PlatformAmountMinor,
		"note":                  "仅记录平台核对值，不覆盖系统流水",
	})
}

// --- S4: installments -------------------------------------------------------

func (a *API) handleCreateInstallment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Name           string  `json:"name"`
		CategoryID     string  `json:"category_id"`
		PrincipalMinor int64   `json:"principal_minor"`
		TotalFeeMinor  int64   `json:"total_fee_minor"`
		Periods        int     `json:"periods"`
		FirstDueOn     string  `json:"first_due_on"`
		Note           *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || !validDate(in.FirstDueOn) || in.Periods < 1 || in.Periods > 600 {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	principalMin, feeMin := money.Minor(in.PrincipalMinor), money.Minor(in.TotalFeeMinor)
	if err := money.MustPositive(principalMin); err != nil || feeMin < 0 {
		writeErr(w, 400, "invalid_amount", "本金须为正，手续费不能为负")
		return
	}
	first, _ := time.Parse("2006-01-02", in.FirstDueOn)
	dues, err := credit.InstallmentDueDates(first, in.Periods)
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	// Integer split; the final period absorbs any rounding remainder.
	baseP, baseF := int64(principalMin)/int64(in.Periods), int64(feeMin)/int64(in.Periods)
	contractID, entryID, txID := newID("inst"), newID("je"), newID("tx")
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := a.validateCategory(tx, r, p.HouseholdID, in.CategoryID, "expense"); err != nil {
			return err
		}
		code := ledger.InstallmentSubject(contractID)
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO installment_contracts(id,household_id,name,principal_minor,total_fee_minor,periods,first_due_on,ledger_code,note)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			contractID, p.HouseholdID, in.Name, int64(principalMin), int64(feeMin), in.Periods, in.FirstDueOn, code, in.Note); err != nil {
			return err
		}
		var sumP, sumF int64
		for i, due := range dues {
			pv, fv := baseP, int64(baseF)
			if i == in.Periods-1 {
				pv, fv = int64(principalMin)-sumP, int64(feeMin)-sumF
			}
			sumP, sumF = sumP+pv, sumF+fv
			if _, err := tx.ExecContext(r.Context(), `
				INSERT INTO installment_schedule_items(id,contract_id,installment_no,due_on,principal_minor,fee_minor,total_minor,status)
				VALUES(?,?,?,?,?,?,?,'pending')`,
				newID("isi"), contractID, i+1, due.Format("2006-01-02"), pv, fv, pv+fv); err != nil {
				return err
			}
		}
		if sumP != int64(principalMin) || sumF != int64(feeMin) {
			return errors.New("期次本金/手续费合计不等于合同金额")
		}
		c := ledger.Context{EntryID: entryID, BizID: txID, OccurredOn: in.FirstDueOn}
		e, err := ledger.InstallmentCreate(c, contractID, in.CategoryID, principalMin)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		// Mirror the original consumption into the transaction list so the ledger stays auditable.
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO transactions(id,household_id,type,amount_minor,occurred_on,note,status,entry_id,created_by)
			VALUES(?,?,'expense',?,?,?,'posted',?,?)`,
			txID, p.HouseholdID, int64(principalMin), in.FirstDueOn, in.Name, entryID, p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO transaction_splits(transaction_id,line_no,category_id,amount_minor) VALUES(?,1,?,?)`,
			txID, in.CategoryID, int64(principalMin)); err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(),
			`UPDATE installment_contracts SET source_transaction_id=? WHERE id=?`, txID, contractID)
		return err
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": contractID, "source_transaction_id": txID, "periods": in.Periods})
}

func (a *API) handleListInstallments(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT c.id,c.name,c.principal_minor,c.total_fee_minor,c.periods,c.status,
		       COUNT(i.id),COALESCE(SUM(i.paid_minor),0)
		FROM installment_contracts c LEFT JOIN installment_schedule_items i ON i.contract_id=c.id
		WHERE c.household_id=? GROUP BY c.id ORDER BY c.created_at DESC`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, status string
		var principal, fee, paid int64
		var periods, count int
		if err := rows.Scan(&id, &name, &principal, &fee, &periods, &status, &count, &paid); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"id": id, "name": name, "principal_minor": principal, "total_fee_minor": fee,
			"periods": periods, "status": status, "schedule_items": count, "paid_minor": paid,
			"remaining_minor": principal + fee - paid,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) handleInstallmentPayment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	contractID := r.PathValue("id")
	var in struct {
		InstallmentNo  int    `json:"installment_no"`
		AmountMinor    int64  `json:"amount_minor"`
		PaidOn         string `json:"paid_on"`
		Channel        string `json:"channel"`
		ChannelID      string `json:"channel_id"`
		PrincipalMinor *int64 `json:"principal_minor"`
		FeeMinor       *int64 `json:"fee_minor"`
	}
	if err := decode(r, &in); err != nil || !validDate(in.PaidOn) {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	amount := money.Minor(in.AmountMinor)
	if err := money.MustPositive(amount); err != nil ||
		(in.Channel != "cash" && in.Channel != "credit") || in.ChannelID == "" {
		writeErr(w, 400, "invalid_request", "金额或付款渠道不正确")
		return
	}
	key := scopedKey(p.HouseholdID, idempotencyKey(r))
	if key != "" {
		var existingID, existingEntry string
		if err := a.DB.QueryRowContext(r.Context(), `
			SELECT p.id,p.entry_id FROM installment_payments p
			JOIN installment_schedule_items s ON s.id=p.schedule_item_id
			JOIN installment_contracts c ON c.id=s.contract_id
			WHERE p.idempotency_key=? AND c.household_id=? AND c.id=?`,
			key, p.HouseholdID, contractID).Scan(&existingID, &existingEntry); err == nil {
			writeJSON(w, 200, map[string]any{
				"id": existingID, "entry_id": existingEntry, "contract_id": contractID,
				"status": "confirmed", "replayed": true,
			})
			return
		}
	}
	payID, entryID := newID("ipay"), newID("je")
	var splitPrincipal, splitFee int64
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var itemID, dueOn, status string
		var pv, fv, total, paid, usedP, usedF int64
		if err := tx.QueryRowContext(r.Context(), `
			SELECT i.id,i.due_on,i.status,i.principal_minor,i.fee_minor,i.total_minor,i.paid_minor
			FROM installment_schedule_items i JOIN installment_contracts c ON c.id=i.contract_id
			WHERE i.contract_id=? AND i.installment_no=? AND c.household_id=?`,
			contractID, in.InstallmentNo, p.HouseholdID).
			Scan(&itemID, &dueOn, &status, &pv, &fv, &total, &paid); err != nil {
			return errors.New("期次不存在")
		}
		if status == "paid" || status == "cancelled" {
			return errors.New("该期次已结清或已取消")
		}
		if int64(amount) > total-paid {
			return errors.New("付款金额超过该期剩余应付")
		}
		if err := tx.QueryRowContext(r.Context(),
			`SELECT COALESCE(SUM(principal_minor),0),COALESCE(SUM(fee_minor),0)
			 FROM installment_payments WHERE schedule_item_id=? AND reversed=0`, itemID).
			Scan(&usedP, &usedF); err != nil {
			return err
		}
		remainP, remainF := pv-usedP, fv-usedF
		settling := int64(amount) == total-paid
		var payP, payF int64
		if in.PrincipalMinor == nil || in.FeeMinor == nil {
			// A final settlement consumes the remaining PLAN amounts (explicit).
			// A partial payment must NEVER be auto-allocated: all 10 accounting
			// policies are still PENDING_USER_CONFIRMATION.
			if !settling {
				return errSplitRequired
			}
			payP, payF = remainP, remainF
		} else {
			payP, payF = *in.PrincipalMinor, *in.FeeMinor
		}
		if payP < 0 || payF < 0 || payP+payF != int64(amount) {
			return errors.New("拆分金额之和必须等于本次实付金额")
		}
		if payP > remainP || payF > remainF {
			return errors.New("拆分金额超过该期本金/手续费剩余金额")
		}
		paySubject := ledger.CreditSubject(in.ChannelID)
		if in.Channel == "cash" {
			if _, err := a.validateAccount(tx, r, p.HouseholdID, in.ChannelID); err != nil {
				return errors.New("付款账户不存在")
			}
			paySubject = ledger.CashSubject(in.ChannelID)
		} else {
			var n int
			if err := tx.QueryRowContext(r.Context(),
				`SELECT COUNT(*) FROM credit_instruments WHERE id=? AND household_id=? AND status='active'`,
				in.ChannelID, p.HouseholdID).Scan(&n); err != nil || n == 0 {
				return errors.New("付款信用工具不存在")
			}
		}
		e, err := ledger.InstallmentPaymentWithSplit(
			ledger.Context{EntryID: entryID, BizID: payID, OccurredOn: in.PaidOn},
			contractID, paySubject, money.Minor(payP), money.Minor(payF))
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, key, p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO installment_payments(id,schedule_item_id,amount_minor,paid_on,channel,channel_id,
			  entry_id,idempotency_key,principal_minor,fee_minor,reversed)
			VALUES(?,?,?,?,?,?,?,?,?,?,0)`,
			payID, itemID, int64(amount), in.PaidOn, in.Channel, in.ChannelID, entryID,
			nullableKey(key), payP, payF); err != nil {
			return err
		}
		// A credit-channel instalment payment creates a NEW credit liability;
		// fold it into that card's billing cycle (no new journal entry here).
		if in.Channel == "credit" {
			if err := a.assignStatement(r, tx, p, in.ChannelID, "installment_payment", payID,
				int64(amount), in.PaidOn); err != nil {
				return err
			}
		}
		newPaid := paid + int64(amount)
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE installment_schedule_items SET paid_minor=?, status=? WHERE id=?`,
			newPaid, derivePeriodStatus(dueOn, a.businessDate(), total, newPaid), itemID); err != nil {
			return err
		}
		splitPrincipal, splitFee = payP, payF
		if err := confirmDueItem(r.Context(), tx, p.HouseholdID, "installment", itemID); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "installment_payment", "installment_payment", payID, in.PaidOn)
	})
	if err != nil {
		if errors.Is(err, errSplitRequired) {
			writeErr(w, 400, "split_required", "部分付款口径未确认，请显式填写本金/手续费拆分")
			return
		}
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{
		"id": payID, "entry_id": entryID, "status": "confirmed",
		"split": map[string]any{"principal_minor": splitPrincipal, "fee_minor": splitFee},
	})
}

// handleGenerateDueItems is the manual trigger of the daily projection. It
// ONLY inserts pending obligations; it NEVER marks anything paid and never
// changes any balance. The biz_key makes it idempotent.
func (a *API) handleGenerateDueItems(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	generated, err := a.projectDueItems(r.Context(), p.HouseholdID, "manual", 30)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"generated": generated, "status": "pending_only", "idempotent": true})
}

func (a *API) handleListDueItems(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	days := 30
	if r.URL.Query().Get("days") == "7" {
		days = 7
	}
	to := a.businessToday().AddDate(0, 0, days).Format("2006-01-02")
	status := r.URL.Query().Get("status")
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,source_type,source_id,biz_key,due_on,amount_minor,status,source_label FROM schedule_due_items
		 WHERE household_id=? AND due_on<=? AND (?='' OR status=?) ORDER BY due_on`,
		p.HouseholdID, to, status, status)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	var total int64
	for rows.Next() {
		var id, typ, sid, key, due, status string
		var amount int64
		var label *string
		if err := rows.Scan(&id, &typ, &sid, &key, &due, &amount, &status, &label); err != nil {
			internalErr(w, err)
			return
		}
		total += amount
		items = append(items, map[string]any{
			"id": id, "source_type": typ, "source_id": sid, "biz_key": key,
			"due_on": due, "amount_minor": amount, "status": status, "source_label": label,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items), "total_minor": total, "days": days})
}
