package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"tinynest/api/internal/credit"
	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

// errSplitRequired marks the safe-mode refusal of an unallocated partial payment.
var errSplitRequired = errors.New("split_required")

func (a *API) registerV011Installment(mux *http.ServeMux) {
	a.route(mux, "GET /api/v1/installments/{id}", a.handleGetInstallment)
	a.route(mux, "POST /api/v1/installments/{id}/refunds", a.handleInstallmentRefund)
	a.route(mux, "POST /api/v1/installments/{id}/adjust-schedule", a.handleAdjustInstallmentSchedule)
	a.route(mux, "POST /api/v1/installments/{id}/settle-early", a.handleSettleEarlyInstallment)
	a.route(mux, "POST /api/v1/installments/convert-from-credit", a.handleConvertFromCredit)
}

type instalPeriod struct {
	id, due, status             string
	no                          int
	principal, fee, total, paid int64
}

func (a *API) loadInstalPeriods(r *http.Request, q db.Queryer, householdID, contractID string) ([]instalPeriod, error) {
	rows, err := q.QueryContext(r.Context(), `
		SELECT s.id,s.installment_no,s.due_on,s.principal_minor,s.fee_minor,s.total_minor,s.paid_minor,s.status
		FROM installment_schedule_items s JOIN installment_contracts c ON c.id=s.contract_id
		WHERE s.contract_id=? AND c.household_id=? ORDER BY s.installment_no`, contractID, householdID)
	if err != nil {
		return nil, err
	}
	var out []instalPeriod
	for rows.Next() {
		var v instalPeriod
		if err := rows.Scan(&v.id, &v.no, &v.due, &v.principal, &v.fee, &v.total, &v.paid, &v.status); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Close()
}

// GET /installments/{id} exposes the full plan, payments and an explicit
// conservation block. Nothing here is a cached balance: all totals are rebuilt
// from schedule rows and posted journal effects.
func (a *API) handleGetInstallment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var name, status, firstDue, code string
	var note, sourceTx *string
	var principal, fee int64
	var periods int
	if err := a.DB.QueryRowContext(r.Context(), `
		SELECT name,principal_minor,total_fee_minor,periods,first_due_on,ledger_code,status,note,source_transaction_id
		FROM installment_contracts WHERE id=? AND household_id=?`, id, p.HouseholdID).
		Scan(&name, &principal, &fee, &periods, &firstDue, &code, &status, &note, &sourceTx); err != nil {
		writeErr(w, 404, "not_found", "分期合同不存在")
		return
	}
	periodsList, err := a.loadInstalPeriods(r, a.DB, p.HouseholdID, id)
	if err != nil {
		internalErr(w, err)
		return
	}
	schedule := []map[string]any{}
	var scheduledPrincipal, scheduledFee, totalDue, paid int64
	for _, v := range periodsList {
		scheduledPrincipal += v.principal
		scheduledFee += v.fee
		if v.status != "cancelled" {
			totalDue += v.total
			paid += v.paid
		}
		schedule = append(schedule, map[string]any{
			"id": v.id, "installment_no": v.no, "due_on": v.due,
			"principal_minor": v.principal, "fee_minor": v.fee, "total_minor": v.total,
			"paid_minor": v.paid, "remaining_minor": v.total - v.paid,
			"status": v.status, "derived_status": derivePeriodStatus(v.due, a.businessDate(), v.total, v.paid),
		})
	}
	payRows, err := a.DB.QueryContext(r.Context(), `
		SELECT p.id,p.schedule_item_id,p.amount_minor,p.paid_on,p.channel,p.channel_id,
		       p.principal_minor,p.fee_minor,p.reversed,p.entry_id
		FROM installment_payments p JOIN installment_schedule_items s ON s.id=p.schedule_item_id
		JOIN installment_contracts c ON c.id=s.contract_id
		WHERE s.contract_id=? AND c.household_id=? ORDER BY p.paid_on,p.id`, id, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	payments := []map[string]any{}
	for payRows.Next() {
		var pid, sid, paidOn, ch, chID, entryID string
		var amount, pv, fv int64
		var reversed int
		if err := payRows.Scan(&pid, &sid, &amount, &paidOn, &ch, &chID, &pv, &fv, &reversed, &entryID); err != nil {
			_ = payRows.Close()
			internalErr(w, err)
			return
		}
		payments = append(payments, map[string]any{
			"id": pid, "schedule_item_id": sid, "amount_minor": amount, "paid_on": paidOn,
			"channel": ch, "channel_id": chID, "principal_minor": pv, "fee_minor": fv,
			"reversed": reversed == 1, "entry_id": entryID,
		})
	}
	_ = payRows.Close()

	var refunded, adjustmentCount int64
	if sourceTx != nil {
		_ = a.DB.QueryRowContext(r.Context(),
			`SELECT COALESCE(SUM(amount_minor),0) FROM refunds WHERE transaction_id=? AND household_id=?`,
			*sourceTx, p.HouseholdID).Scan(&refunded)
	}
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM installment_adjustments WHERE contract_id=? AND household_id=?`,
		id, p.HouseholdID).Scan(&adjustmentCount)

	writeJSON(w, 200, map[string]any{
		"id": id, "name": name, "status": status,
		"principal_minor": principal, "total_fee_minor": fee, "periods": periods,
		"first_due_on": firstDue, "ledger_code": code, "note": note,
		"source_transaction_id": sourceTx,
		"schedule":              schedule, "payments": payments,
		"conservation": map[string]any{
			"scheduled_principal_minor": scheduledPrincipal,
			"scheduled_fee_minor":       scheduledFee,
			"principal_balanced":        scheduledPrincipal == principal,
			"fee_balanced":              scheduledFee == fee,
			"total_due_minor":           totalDue,
			"paid_minor":                paid,
			"remaining_minor":           totalDue - paid,
			"refunded_minor":            refunded,
			"adjustment_count":          adjustmentCount,
		},
	})
}

// A refund is linked to the ORIGINAL consumption transaction. It never deletes
// paid periods, never rewrites posted entries, and only REPORTS which future
// periods would be affected - the user must confirm any schedule change.
func (a *API) handleInstallmentRefund(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	contractID := r.PathValue("id")
	var in struct {
		AmountMinor     int64   `json:"amount_minor"`
		RefundOn        string  `json:"refund_on"`
		DestinationType string  `json:"destination_type"`
		DestinationID   string  `json:"destination_id"`
		SplitLineNo     int     `json:"split_line_no"`
		Note            *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || !validDate(in.RefundOn) ||
		(in.DestinationType != "cash" && in.DestinationType != "credit") {
		writeErr(w, 400, "invalid_request", "日期或退款去向不正确")
		return
	}
	amount := money.Minor(in.AmountMinor)
	if err := money.MustPositive(amount); err != nil {
		writeErr(w, 400, "invalid_amount", "退款金额必须为正")
		return
	}
	key := scopedKey(p.HouseholdID, idempotencyKey(r))
	if key != "" {
		var existingID, existingEntry string
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT id,entry_id FROM refunds WHERE idempotency_key=? AND household_id=?`,
			key, p.HouseholdID).Scan(&existingID, &existingEntry); err == nil {
			writeJSON(w, 200, map[string]any{"id": existingID, "entry_id": existingEntry, "replayed": true})
			return
		}
	}
	refundID, entryID := newID("ref"), newID("je")
	var refunded int64
	affected := []map[string]any{}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var sourceTx sql.NullString
		if err := tx.QueryRowContext(r.Context(),
			`SELECT source_transaction_id FROM installment_contracts WHERE id=? AND household_id=?`,
			contractID, p.HouseholdID).Scan(&sourceTx); err != nil {
			return errors.New("分期合同不存在")
		}
		if !sourceTx.Valid {
			return errors.New("该合同没有关联原始消费，不能按消费退款")
		}
		var original int64
		var txType, txStatus string
		if err := tx.QueryRowContext(r.Context(),
			`SELECT amount_minor,type,status FROM transactions WHERE id=? AND household_id=?`,
			sourceTx.String, p.HouseholdID).Scan(&original, &txType, &txStatus); err != nil {
			return errors.New("原交易不存在")
		}
		if txType != "expense" || txStatus == "reversed" {
			return errors.New("只有已入账支出可以退款")
		}
		var splitID int64
		var categoryID string
		var splitAmount int64
		if in.SplitLineNo > 0 {
			if err := tx.QueryRowContext(r.Context(),
				`SELECT id,category_id,amount_minor FROM transaction_splits WHERE transaction_id=? AND line_no=?`,
				sourceTx.String, in.SplitLineNo).Scan(&splitID, &categoryID, &splitAmount); err != nil {
				return errors.New("拆分行不存在")
			}
		} else {
			if err := tx.QueryRowContext(r.Context(),
				`SELECT id,category_id,amount_minor FROM transaction_splits WHERE transaction_id=? ORDER BY line_no LIMIT 1`,
				sourceTx.String).Scan(&splitID, &categoryID, &splitAmount); err != nil {
				return errors.New("原交易没有分类行")
			}
		}
		var already int64
		if err := tx.QueryRowContext(r.Context(),
			`SELECT COALESCE(SUM(amount_minor),0) FROM refunds WHERE split_id=?`, splitID).Scan(&already); err != nil {
			return err
		}
		if _, err := ledger.ValidateRefund(money.Minor(splitAmount), money.Minor(already), amount); err != nil {
			return errors.New("退款金额不合法或超过原交易可退金额")
		}
		c := ledger.Context{EntryID: entryID, BizID: refundID, OccurredOn: in.RefundOn}
		var e *ledger.Entry
		var err error
		if in.DestinationType == "cash" {
			if _, err = a.validateAccount(tx, r, p.HouseholdID, in.DestinationID); err != nil {
				return errors.New("退款到账账户不存在")
			}
			e, err = ledger.RefundToCash(c, in.DestinationID, categoryID, amount)
		} else {
			var n int
			if err := tx.QueryRowContext(r.Context(),
				`SELECT COUNT(*) FROM credit_instruments WHERE id=? AND household_id=? AND status='active'`,
				in.DestinationID, p.HouseholdID).Scan(&n); err != nil || n == 0 {
				return errors.New("退款信用工具不存在")
			}
			e, err = ledger.RefundToCredit(c, in.DestinationID, categoryID, amount)
		}
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, key, p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO refunds(id,household_id,transaction_id,split_id,category_id,amount_minor,refund_on,
			  destination_type,destination_id,note,entry_id,idempotency_key)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			refundID, p.HouseholdID, sourceTx.String, splitID, categoryID, int64(amount), in.RefundOn,
			in.DestinationType, in.DestinationID, in.Note, entryID, nullableKey(key)); err != nil {
			return err
		}
		if in.DestinationType == "credit" {
			if err := a.assignStatement(r, tx, p, in.DestinationID, "refund", refundID,
				-int64(amount), in.RefundOn); err != nil {
				return err
			}
		}
		refunded = already + int64(amount)
		periods, err := a.loadInstalPeriods(r, tx, p.HouseholdID, contractID)
		if err != nil {
			return err
		}
		for _, v := range periods {
			if v.status != "paid" && v.status != "cancelled" {
				affected = append(affected, map[string]any{
					"installment_no": v.no, "due_on": v.due,
					"total_minor": v.total, "paid_minor": v.paid, "status": v.status,
				})
			}
		}
		return audit(r.Context(), tx, p, "installment_refund", "refund", refundID, sourceTx.String)
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{
		"id": refundID, "entry_id": entryID,
		"refunded_minor":               refunded,
		"future_periods_affected":      affected,
		"requires_explicit_adjustment": true,
		"note":                         "已付款期次与所有分录保持不变；如需调整未来期次，请显式调用 /adjust-schedule",
	})
}

// An adjustment may only RELIEVE obligations, never increase them, and the
// cumulative relief can never exceed cumulative refunds. Paid periods are
// immutable. Every adjustment is appended to installment_adjustments.
func (a *API) handleAdjustInstallmentSchedule(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	contractID := r.PathValue("id")
	var in struct {
		Reason string `json:"reason"`
		Items  []struct {
			InstallmentNo  int    `json:"installment_no"`
			Action         string `json:"action"`
			PrincipalMinor *int64 `json:"principal_minor"`
			FeeMinor       *int64 `json:"fee_minor"`
		} `json:"items"`
	}
	if err := decode(r, &in); err != nil || in.Reason == "" || len(in.Items) == 0 {
		writeErr(w, 400, "invalid_request", "调整必须填写 reason 与 items")
		return
	}
	var reduced, refunded int64
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var principal, fee int64
		var sourceTx sql.NullString
		if err := tx.QueryRowContext(r.Context(),
			`SELECT principal_minor,total_fee_minor,source_transaction_id FROM installment_contracts WHERE id=? AND household_id=?`,
			contractID, p.HouseholdID).Scan(&principal, &fee, &sourceTx); err != nil {
			return errors.New("分期合同不存在")
		}
		if sourceTx.Valid {
			if err := tx.QueryRowContext(r.Context(),
				`SELECT COALESCE(SUM(amount_minor),0) FROM refunds WHERE transaction_id=? AND household_id=?`,
				sourceTx.String, p.HouseholdID).Scan(&refunded); err != nil {
				return err
			}
		}
		var activeTotal int64
		if err := tx.QueryRowContext(r.Context(),
			`SELECT COALESCE(SUM(total_minor),0) FROM installment_schedule_items
			 WHERE contract_id=? AND status<>'cancelled'`, contractID).Scan(&activeTotal); err != nil {
			return err
		}
		alreadyRelieved := principal + fee - activeTotal
		type change struct {
			id     string
			action string
			p, f   int64
			delta  int64
		}
		var changes []change
		for _, it := range in.Items {
			if it.Action != "cancel" && it.Action != "reduce" {
				return errors.New("action 仅支持 cancel/reduce")
			}
			var sid, status string
			var oldP, oldF, oldTotal, paid int64
			if err := tx.QueryRowContext(r.Context(), `
				SELECT s.id,s.status,s.principal_minor,s.fee_minor,s.total_minor,s.paid_minor
				FROM installment_schedule_items s JOIN installment_contracts c ON c.id=s.contract_id
				WHERE s.contract_id=? AND s.installment_no=? AND c.household_id=?`,
				contractID, it.InstallmentNo, p.HouseholdID).
				Scan(&sid, &status, &oldP, &oldF, &oldTotal, &paid); err != nil {
				return errors.New("期次不存在")
			}
			if status == "paid" || status == "cancelled" || paid > 0 {
				return errors.New("已付款或已取消的期次不可调整")
			}
			ch := change{id: sid, action: it.Action}
			if it.Action == "reduce" {
				if it.PrincipalMinor == nil || it.FeeMinor == nil {
					return errors.New("reduce 必须显式提供 principal_minor 与 fee_minor")
				}
				ch.p, ch.f = *it.PrincipalMinor, *it.FeeMinor
				if ch.p < 0 || ch.f < 0 || ch.p > oldP || ch.f > oldF || ch.p+ch.f > oldTotal {
					return errors.New("调整金额不合法或超过原期次金额")
				}
			}
			ch.delta = oldTotal - (ch.p + ch.f)
			reduced += ch.delta
			changes = append(changes, ch)
		}
		if alreadyRelieved+reduced > refunded {
			return errors.New("累计减免额不得超过累计退款额")
		}
		for _, ch := range changes {
			if ch.action == "cancel" {
				if _, err := tx.ExecContext(r.Context(),
					`UPDATE installment_schedule_items SET status='cancelled' WHERE id=?`, ch.id); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(r.Context(),
					`UPDATE installment_schedule_items SET principal_minor=?,fee_minor=?,total_minor=? WHERE id=?`,
					ch.p, ch.f, ch.p+ch.f, ch.id); err != nil {
					return err
				}
			}
		}
		payload, _ := json.Marshal(map[string]any{
			"items": in.Items, "reduced_minor": reduced, "refunded_minor": refunded,
		})
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO installment_adjustments(id,household_id,contract_id,action,payload_json,reason,actor)
			VALUES(?,?,?,?,?,?,?)`,
			newID("iadj"), p.HouseholdID, contractID, "adjust_schedule", string(payload),
			in.Reason, p.Username); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "adjust_installment_schedule", "installment_contract", contractID, in.Reason)
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"contract_id": contractID, "status": "adjusted",
		"adjusted_periods": len(in.Items), "reduced_minor": reduced,
		"refunded_minor": refunded,
		"note":           "仅调整未付款期次；已付款历史与分录保持不变",
	})
}

func (a *API) handleSettleEarlyInstallment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil || in.Reason == "" {
		writeErr(w, 400, "invalid_request", "提前结清必须填写 reason")
		return
	}
	cancelled := 0
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM installment_contracts WHERE id=? AND household_id=?`,
			id, p.HouseholdID).Scan(&n); err != nil || n == 0 {
			return errors.New("分期合同不存在")
		}
		res, err := tx.ExecContext(r.Context(), `
			UPDATE installment_schedule_items SET status='cancelled'
			WHERE contract_id=? AND paid_minor=0 AND status IN ('pending','partial','overdue')`, id)
		if err != nil {
			return err
		}
		n64, _ := res.RowsAffected()
		cancelled = int(n64)
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO installment_adjustments(id,household_id,contract_id,action,payload_json,reason,actor)
			VALUES(?,?,?,'settle_early',?,?,?)`,
			newID("iadj"), p.HouseholdID, id,
			itoa(cancelled), in.Reason, p.Username); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "settle_early_installment", "installment_contract", id, in.Reason)
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"contract_id": id, "cancelled_periods": cancelled, "status": "settled",
		"note": "仅取消未付款期次；已付款历史与分录全部保留",
	})
}

// Converting an existing credit-card balance into an instalment contract MUST
// reclassify the principal, never create a second copy of it. No consumption is
// recognised again and no cash moves.
func (a *API) handleConvertFromCredit(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		CreditInstrumentID string  `json:"credit_instrument_id"`
		StatementID        string  `json:"statement_id"`
		Name               string  `json:"name"`
		PrincipalMinor     int64   `json:"principal_minor"`
		TotalFeeMinor      int64   `json:"total_fee_minor"`
		Periods            int     `json:"periods"`
		FirstDueOn         string  `json:"first_due_on"`
		Note               *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || in.Name == "" || in.CreditInstrumentID == "" ||
		!validDate(in.FirstDueOn) || in.Periods < 1 || in.Periods > 600 {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	if in.PrincipalMinor <= 0 || in.TotalFeeMinor < 0 {
		writeErr(w, 400, "invalid_amount", "本金须为正，手续费不能为负")
		return
	}
	first, _ := time.Parse("2006-01-02", in.FirstDueOn)
	dues, err := credit.InstallmentDueDates(first, in.Periods)
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	key := scopedKey(p.HouseholdID, idempotencyKey(r))
	if key != "" {
		var bizID, existingEntry string
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT biz_id,id FROM journal_entries WHERE idempotency_key=? AND household_id=? AND biz_type=?`,
			key, p.HouseholdID, ledger.BizConvertInstall).Scan(&bizID, &existingEntry); err == nil {
			writeJSON(w, 200, map[string]any{"id": bizID, "entry_id": existingEntry, "replayed": true})
			return
		}
	}
	contractID, entryID := newID("inst"), newID("je")
	code := ledger.InstallmentSubject(contractID)
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var instrumentCode string
		if err := tx.QueryRowContext(r.Context(),
			`SELECT ledger_code FROM credit_instruments WHERE id=? AND household_id=? AND status='active'`,
			in.CreditInstrumentID, p.HouseholdID).Scan(&instrumentCode); err != nil {
			return errors.New("信用工具不存在或已停用")
		}
		bal, err := db.Balance(r.Context(), tx, p.HouseholdID, instrumentCode)
		if err != nil {
			return err
		}
		outstanding := -int64(bal)
		if in.PrincipalMinor > outstanding {
			return errors.New("转分期金额不得超过信用工具当前未偿余额")
		}
		var stmtID string
		var remaining int64
		if err := tx.QueryRowContext(r.Context(), `
			SELECT MIN(id), COALESCE(MAX(system_due_minor-paid_minor),0)
			FROM credit_statements
			WHERE household_id=? AND instrument_id=? AND system_due_minor>paid_minor
			  AND (?='' OR id=?)
			HAVING COUNT(*)=1`, p.HouseholdID, in.CreditInstrumentID, in.StatementID, in.StatementID).
			Scan(&stmtID, &remaining); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("请选择一张有未还余额的原账单")
			}
			return err
		}
		if in.PrincipalMinor > remaining {
			return errors.New("转分期金额不得超过原账单未还余额")
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO credit_statement_items(statement_id,source_type,source_id,amount_minor,occurred_on)
			VALUES(?,'installment_conversion',?,?,?)`, stmtID, contractID, -in.PrincipalMinor, a.businessDate()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			UPDATE credit_statements
			SET system_due_minor=(SELECT SUM(amount_minor) FROM credit_statement_items WHERE statement_id=?)
			WHERE id=? AND household_id=?`, stmtID, stmtID, p.HouseholdID); err != nil {
			return err
		}
		remaining -= in.PrincipalMinor
		if remaining == 0 {
			if _, err := tx.ExecContext(r.Context(), `
				DELETE FROM schedule_due_items WHERE household_id=? AND source_type='credit_statement'
				  AND source_id=? AND status='pending'`, p.HouseholdID, stmtID); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(r.Context(), `
			UPDATE schedule_due_items SET amount_minor=?
			WHERE household_id=? AND source_type='credit_statement' AND source_id=? AND status='pending'`,
			remaining, p.HouseholdID, stmtID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO installment_contracts(id,household_id,name,principal_minor,total_fee_minor,periods,
			  first_due_on,ledger_code,status,note)
			VALUES(?,?,?,?,?,?,?,?,'active',?)`,
			contractID, p.HouseholdID, in.Name, in.PrincipalMinor, in.TotalFeeMinor, in.Periods,
			in.FirstDueOn, code, in.Note); err != nil {
			return err
		}
		baseP := in.PrincipalMinor / int64(in.Periods)
		baseF := in.TotalFeeMinor / int64(in.Periods)
		var sumP, sumF int64
		for i, due := range dues {
			pv, fv := baseP, baseF
			if i == in.Periods-1 {
				pv, fv = in.PrincipalMinor-sumP, in.TotalFeeMinor-sumF
			}
			sumP, sumF = sumP+pv, sumF+fv
			if _, err := tx.ExecContext(r.Context(), `
				INSERT INTO installment_schedule_items(id,contract_id,installment_no,due_on,
				  principal_minor,fee_minor,total_minor,status)
				VALUES(?,?,?,?,?,?,?,'pending')`,
				newID("isi"), contractID, i+1, due.Format("2006-01-02"), pv, fv, pv+fv); err != nil {
				return err
			}
		}
		if sumP != in.PrincipalMinor || sumF != in.TotalFeeMinor {
			return errors.New("期次本金/手续费合计不等于合同金额")
		}
		e, err := ledger.ConvertToInstallment(
			ledger.Context{EntryID: entryID, BizID: contractID, OccurredOn: in.FirstDueOn},
			in.CreditInstrumentID, contractID, money.Minor(in.PrincipalMinor))
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, key, p.Username); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "convert_to_installment", "installment_contract", contractID, in.CreditInstrumentID)
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{
		"id": contractID, "entry_id": entryID, "status": "active",
		"note": "仅重分类负债：信用负债减少、分期负债增加，消费只确认一次",
	})
}
