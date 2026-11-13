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
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT s.id,s.instrument_id,i.name,s.cycle_start_on,s.cycle_end_on,s.due_on,
		       s.system_due_minor,s.paid_minor,s.platform_amount_minor,s.status
		FROM credit_statements s JOIN credit_instruments i ON i.id=s.instrument_id
		WHERE s.household_id=? ORDER BY s.cycle_start_on DESC`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, instrID, name, start, end, due, status string
		var sysDue, paid int64
		var platform *int64
		if err := rows.Scan(&id, &instrID, &name, &start, &end, &due, &sysDue, &paid, &platform, &status); err != nil {
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
			"status": derived, "derived_status": derived, "stored_status": status,
			"platform_amount_minor": platform,
		}
		if platform != nil {
			m["reconcile_diff_minor"] = sysDue - *platform
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
	replayKey := idempotencyKey(r)
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
		if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, idempotencyKey(r), p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO credit_payments(id,household_id,statement_id,amount_minor,paid_on,from_account_id,entry_id,idempotency_key)
			VALUES(?,?,?,?,?,?,?,?)`,
			payID, p.HouseholdID, stmtID, int64(amount), in.PaidOn, in.FromAccountID, entryID,
			nullableKey(idempotencyKey(r))); err != nil {
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
		InstallmentNo int    `json:"installment_no"`
		AmountMinor   int64  `json:"amount_minor"`
		PaidOn        string `json:"paid_on"`
		Channel       string `json:"channel"`
		ChannelID     string `json:"channel_id"`
	}
	if err := decode(r, &in); err != nil || !validDate(in.PaidOn) {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	amount := money.Minor(in.AmountMinor)
	if err := money.MustPositive(amount); err != nil || (in.Channel != "cash" && in.Channel != "credit") {
		writeErr(w, 400, "invalid_request", "金额或付款渠道不正确")
		return
	}
	payID, entryID := newID("ipay"), newID("je")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var itemID string
		var pv, fv, total, paid int64
		var status string
		if err := tx.QueryRowContext(r.Context(), `
			SELECT i.id,i.principal_minor,i.fee_minor,i.total_minor,i.paid_minor,i.status
			FROM installment_schedule_items i JOIN installment_contracts c ON c.id=i.contract_id
			WHERE i.contract_id=? AND i.installment_no=? AND c.household_id=?`,
			contractID, in.InstallmentNo, p.HouseholdID).Scan(&itemID, &pv, &fv, &total, &paid, &status); err != nil {
			return errors.New("期次不存在")
		}
		if status == "paid" || status == "cancelled" {
			return errors.New("该期次已结清或已取消")
		}
		if int64(amount) > total-paid {
			return errors.New("付款金额超过该期剩余应付")
		}
		if in.Channel == "cash" {
			if _, err := a.validateAccount(tx, r, p.HouseholdID, in.ChannelID); err != nil {
				return errors.New("付款账户不存在")
			}
		}
		paySubject := ledger.CreditSubject(in.ChannelID)
		if in.Channel == "cash" {
			paySubject = ledger.CashSubject(in.ChannelID)
		}
		// Partial payments reduce principal first; fee is recognised on the period's final settlement.
		principalPart := int64(amount)
		feePart := int64(0)
		if int64(amount) >= total-paid {
			principalPart, feePart = pv, fv
		} else if principalPart > pv {
			principalPart = pv
		}
		if principalPart <= 0 {
			return errors.New("付款金额无效")
		}
		e, err := ledger.InstallmentPayment(
			ledger.Context{EntryID: entryID, BizID: payID, OccurredOn: in.PaidOn},
			contractID, paySubject, money.Minor(principalPart), money.Minor(feePart))
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, idempotencyKey(r), p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO installment_payments(id,schedule_item_id,amount_minor,paid_on,channel,channel_id,entry_id,idempotency_key)
			VALUES(?,?,?,?,?,?,?,?)`,
			payID, itemID, int64(amount), in.PaidOn, in.Channel, in.ChannelID, entryID,
			nullableKey(idempotencyKey(r))); err != nil {
			return err
		}
		newPaid := paid + int64(amount)
		newStatus := "partial"
		if newPaid >= total {
			newStatus = "paid"
		}
		_, err = tx.ExecContext(r.Context(),
			`UPDATE installment_schedule_items SET paid_minor=?, status=? WHERE id=?`, newPaid, newStatus, itemID)
		return err
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": payID, "entry_id": entryID, "status": "confirmed"})
}

// handleGenerateDueItems is the daily task. It ONLY inserts pending obligations;
// it NEVER marks anything paid and never changes any balance. biz_key makes it idempotent.
func (a *API) handleGenerateDueItems(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	horizon := a.businessToday().AddDate(0, 0, 30).Format("2006-01-02")
	generated := 0
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		// The schedule tables have no household_id column of their own: ownership is
		// resolved through their parent contract. Filtering here (not after the fact)
		// is what prevents household A from projecting household B's obligations.
		for _, src := range []struct{ table, kind, contractTable string }{
			{"installment_schedule_items", "installment", "installment_contracts"},
			{"debt_schedule_items", "debt", "debt_contracts"},
		} {
			rows, err := tx.QueryContext(r.Context(), fmt.Sprintf(`
			SELECT s.id,s.due_on,s.total_minor,s.paid_minor
			FROM %s s JOIN %s c ON c.id = s.contract_id
			WHERE c.household_id = ? AND s.status='pending' AND s.due_on<=?`,
				src.table, src.contractTable), p.HouseholdID, horizon)
			if err != nil {
				return err
			}
			type row struct {
				id, due     string
				total, paid int64
			}
			var found []row
			for rows.Next() {
				var v row
				if err := rows.Scan(&v.id, &v.due, &v.total, &v.paid); err != nil {
					_ = rows.Close()
					return err
				}
				found = append(found, v)
			}
			_ = rows.Close()
			for _, v := range found {
				// Scope the idempotency key to the household so two households can
				// never contend for the same globally-unique biz_key.
				bizKey := p.HouseholdID + ":" + src.kind + ":" + v.id
				res, err := tx.ExecContext(r.Context(), `
					INSERT OR IGNORE INTO schedule_due_items(id,household_id,source_type,source_id,biz_key,due_on,amount_minor,status)
					VALUES(?,?,?,?,?,?,?,'pending')`,
					newID("due"), p.HouseholdID, src.kind, v.id, bizKey, v.due, v.total-v.paid)
				if err != nil {
					return err
				}
				if n, _ := res.RowsAffected(); n > 0 {
					generated++
				}
			}
		}
		return nil
	})
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
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,source_type,source_id,biz_key,due_on,amount_minor,status FROM schedule_due_items
		 WHERE household_id=? AND due_on<=? ORDER BY due_on`, p.HouseholdID, to)
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
		if err := rows.Scan(&id, &typ, &sid, &key, &due, &amount, &status); err != nil {
			internalErr(w, err)
			return
		}
		total += amount
		items = append(items, map[string]any{
			"id": id, "source_type": typ, "source_id": sid, "biz_key": key,
			"due_on": due, "amount_minor": amount, "status": status,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items), "total_minor": total, "days": days})
}
