package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

func (a *API) registerV011Debt(mux *http.ServeMux) {
	a.route(mux, "GET /api/v1/debts", a.handleListDebts)
	a.route(mux, "POST /api/v1/debts", a.handleCreateDebt)
	a.route(mux, "GET /api/v1/debts/{id}", a.handleGetDebt)
	a.route(mux, "PATCH /api/v1/debts/{id}", a.handlePatchDebt)
	a.route(mux, "POST /api/v1/debts/{id}/schedule", a.handleDebtSchedule)
	a.route(mux, "POST /api/v1/debts/{id}/payments", a.handleDebtPayment)
	a.route(mux, "POST /api/v1/debts/{id}/payments/{paymentId}/reverse", a.handleReverseDebtPayment)
	a.route(mux, "POST /api/v1/debts/{id}/settle-early", a.handleSettleEarlyDebt)
}

// derivePeriodStatus is computed from posted amounts and the business date.
func derivePeriodStatus(dueOn, today string, total, paid int64) string {
	if total > 0 && paid >= total {
		return "paid"
	}
	if paid > 0 {
		return "partial"
	}
	if today > dueOn {
		return "overdue"
	}
	return "pending"
}

func (a *API) handleCreateDebt(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Kind             string  `json:"kind"`
		Name             string  `json:"name"`
		PrincipalMinor   int64   `json:"principal_minor"`
		StartOn          string  `json:"start_on"`
		EndOn            *string `json:"end_on"`
		DueDay           *int    `json:"due_day"`
		Cycle            *string `json:"cycle"`
		DefaultChannel   *string `json:"default_channel"`
		DefaultChannelID *string `json:"default_channel_id"`
		MemberID         *string `json:"member_id"`
		Note             *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || in.Name == "" || !validDate(in.StartOn) {
		writeErr(w, 400, "invalid_request", "名称、类型与开始日期必填")
		return
	}
	switch in.Kind {
	case "mortgage", "loan", "insurance", "other":
	default:
		writeErr(w, 400, "invalid_request", "不支持的负债类型")
		return
	}
	if in.PrincipalMinor < 0 {
		writeErr(w, 400, "invalid_amount", "初始本金不能为负")
		return
	}
	// An insurance premium is an expense, never a long-term loan principal.
	if in.Kind == "insurance" && in.PrincipalMinor != 0 {
		writeErr(w, 400, "invalid_request", "保险缴费不得设置贷款本金")
		return
	}
	if in.EndOn != nil && !validDate(*in.EndOn) {
		writeErr(w, 400, "invalid_request", "结束日期格式不正确")
		return
	}
	dueDay := 1
	if in.DueDay != nil {
		dueDay = *in.DueDay
		if dueDay < 1 || dueDay > 31 {
			writeErr(w, 400, "invalid_request", "还款日须在 1..31")
			return
		}
	}
	cycle := "monthly"
	if in.Cycle != nil {
		cycle = *in.Cycle
		if cycle != "monthly" && cycle != "custom" {
			writeErr(w, 400, "invalid_request", "周期仅支持 monthly/custom")
			return
		}
	}
	channel := "cash"
	if in.DefaultChannel != nil {
		channel = *in.DefaultChannel
		if channel != "cash" && channel != "credit" {
			writeErr(w, 400, "invalid_request", "默认渠道仅支持 cash/credit")
			return
		}
	}

	id, entryID := newID("debt"), newID("je")
	code := ""
	if in.PrincipalMinor > 0 {
		code = ledger.LoanSubject(id)
	}
	var ledgerCodeArg any
	if code != "" {
		ledgerCodeArg = code
	}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if in.DefaultChannelID != nil {
			if channel == "cash" {
				if _, err := a.validateAccount(tx, r, p.HouseholdID, *in.DefaultChannelID); err != nil {
					return errors.New("默认付款账户不存在")
				}
			} else {
				var n int
				if err := tx.QueryRowContext(r.Context(),
					`SELECT COUNT(*) FROM credit_instruments WHERE id=? AND household_id=? AND status='active'`,
					*in.DefaultChannelID, p.HouseholdID).Scan(&n); err != nil || n == 0 {
					return errors.New("默认信用工具不存在")
				}
			}
		}
		if code != "" {
			if _, err := tx.ExecContext(r.Context(),
				`INSERT INTO ledger_accounts(id,household_id,code,name,category,ref_type,ref_id) VALUES(?,?,?,?,2,'debt_contract',?)`,
				newID("la"), p.HouseholdID, code, in.Name, id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO debt_contracts(id,household_id,kind,name,principal_minor,remaining_principal_minor,
			  start_on,end_on,due_day,cycle,default_channel,default_channel_id,member_id,ledger_code,status,note)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'active',?)`,
			id, p.HouseholdID, in.Kind, in.Name, in.PrincipalMinor, in.PrincipalMinor,
			in.StartOn, ptrArg(in.EndOn), dueDay, cycle, channel, ptrArg(in.DefaultChannelID),
			ptrArg(in.MemberID), ledgerCodeArg, ptrArg(in.Note)); err != nil {
			return err
		}
		if in.PrincipalMinor > 0 {
			e, err := ledger.OpeningLoan(
				ledger.Context{EntryID: entryID, BizID: id, OccurredOn: in.StartOn},
				id, money.Minor(in.PrincipalMinor))
			if err != nil {
				return err
			}
			if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, "", p.Username); err != nil {
				return err
			}
		}
		return audit(r.Context(), tx, p, "create_debt", "debt_contract", id, "")
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{
		"id": id, "kind": in.Kind, "ledger_code": ledgerCodeArg,
		"opening_entry_id": entryID, "status": "active",
	})
}

func (a *API) handleListDebts(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT c.id,c.kind,c.name,c.principal_minor,c.remaining_principal_minor,
		       c.start_on,c.end_on,c.due_day,c.cycle,c.default_channel,c.status,c.ledger_code,c.note,
		       COUNT(s.id),COALESCE(SUM(s.paid_minor),0),COALESCE(SUM(s.total_minor),0)
		FROM debt_contracts c LEFT JOIN debt_schedule_items s ON s.contract_id=c.id
		WHERE c.household_id=? GROUP BY c.id ORDER BY c.created_at DESC`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	type row struct {
		id, kind, name, start, cycle, channel, status string
		end, code, note                               *string
		principal, remaining, paid, total             int64
		dueDay, periods                               int
	}
	var list []row
	for rows.Next() {
		var v row
		if err := rows.Scan(&v.id, &v.kind, &v.name, &v.principal, &v.remaining,
			&v.start, &v.end, &v.dueDay, &v.cycle, &v.channel, &v.status, &v.code, &v.note,
			&v.periods, &v.paid, &v.total); err != nil {
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
		m := map[string]any{
			"id": v.id, "kind": v.kind, "name": v.name,
			"principal_minor": v.principal, "remaining_principal_minor": v.remaining,
			"start_on": v.start, "end_on": v.end, "due_day": v.dueDay,
			"cycle": v.cycle, "default_channel": v.channel, "status": v.status,
			"ledger_code": v.code, "note": v.note,
			"periods": v.periods, "paid_minor": v.paid, "total_minor": v.total,
			"remaining_minor": v.total - v.paid,
		}
		if v.code != nil {
			bal, err := db.Balance(r.Context(), a.DB, p.HouseholdID, *v.code)
			if err == nil {
				m["outstanding_principal_minor"] = -int64(bal)
			}
		} else {
			m["outstanding_principal_minor"] = 0
		}
		items = append(items, m)
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) handleGetDebt(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var kind, name, start, cycle, channel, status string
	var end, code, note *string
	var principal, remaining int64
	var dueDay int
	if err := a.DB.QueryRowContext(r.Context(), `
		SELECT kind,name,principal_minor,remaining_principal_minor,start_on,end_on,due_day,
		       cycle,default_channel,status,ledger_code,note
		FROM debt_contracts WHERE id=? AND household_id=?`, id, p.HouseholdID).
		Scan(&kind, &name, &principal, &remaining, &start, &end, &dueDay,
			&cycle, &channel, &status, &code, &note); err != nil {
		writeErr(w, 404, "not_found", "负债合同不存在")
		return
	}
	out := map[string]any{
		"id": id, "kind": kind, "name": name,
		"principal_minor": principal, "remaining_principal_minor": remaining,
		"start_on": start, "end_on": end, "due_day": dueDay, "cycle": cycle,
		"default_channel": channel, "status": status, "ledger_code": code, "note": note,
	}
	if code != nil {
		bal, err := db.Balance(r.Context(), a.DB, p.HouseholdID, *code)
		if err == nil {
			out["outstanding_principal_minor"] = -int64(bal)
		}
	} else {
		out["outstanding_principal_minor"] = 0
	}

	schedRows, err := a.DB.QueryContext(r.Context(), `
		SELECT s.id,s.installment_no,s.due_on,s.principal_minor,s.interest_minor,s.fee_minor,
		       s.total_minor,s.paid_minor,s.status
		FROM debt_schedule_items s JOIN debt_contracts c ON c.id=s.contract_id
		WHERE s.contract_id=? AND c.household_id=? ORDER BY s.installment_no`, id, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	schedule := []map[string]any{}
	for schedRows.Next() {
		var sid, due, st string
		var no int
		var pv, iv, fv, total, paid int64
		if err := schedRows.Scan(&sid, &no, &due, &pv, &iv, &fv, &total, &paid, &st); err != nil {
			_ = schedRows.Close()
			internalErr(w, err)
			return
		}
		schedule = append(schedule, map[string]any{
			"id": sid, "installment_no": no, "due_on": due,
			"principal_minor": pv, "interest_minor": iv, "fee_minor": fv,
			"total_minor": total, "paid_minor": paid,
			"remaining_minor": total - paid, "status": st,
			"derived_status": derivePeriodStatus(due, a.businessDate(), total, paid),
		})
	}
	if err := schedRows.Close(); err != nil {
		internalErr(w, err)
		return
	}
	out["schedule"] = schedule

	payRows, err := a.DB.QueryContext(r.Context(), `
		SELECT p.id,p.schedule_item_id,p.amount_minor,p.paid_on,p.channel,p.channel_id,
		       p.principal_minor,p.interest_minor,p.fee_minor,p.reversed,p.entry_id
		FROM debt_payments p
		JOIN debt_schedule_items s ON s.id=p.schedule_item_id
		JOIN debt_contracts c ON c.id=s.contract_id
		WHERE s.contract_id=? AND c.household_id=? ORDER BY p.paid_on,p.id`, id, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	payments := []map[string]any{}
	for payRows.Next() {
		var pid, sid, paidOn, ch, chID, entryID string
		var amount, pv, iv, fv int64
		var reversed int
		if err := payRows.Scan(&pid, &sid, &amount, &paidOn, &ch, &chID, &pv, &iv, &fv, &reversed, &entryID); err != nil {
			_ = payRows.Close()
			internalErr(w, err)
			return
		}
		payments = append(payments, map[string]any{
			"id": pid, "schedule_item_id": sid, "amount_minor": amount, "paid_on": paidOn,
			"channel": ch, "channel_id": chID,
			"principal_minor": pv, "interest_minor": iv, "fee_minor": fv,
			"reversed": reversed == 1, "entry_id": entryID,
		})
	}
	_ = payRows.Close()
	out["payments"] = payments
	writeJSON(w, 200, out)
}

func (a *API) handlePatchDebt(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var in struct {
		Name             *string `json:"name"`
		Note             *string `json:"note"`
		EndOn            *string `json:"end_on"`
		DueDay           *int    `json:"due_day"`
		Cycle            *string `json:"cycle"`
		DefaultChannel   *string `json:"default_channel"`
		DefaultChannelID *string `json:"default_channel_id"`
		Status           *string `json:"status"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	if in.DueDay != nil && (*in.DueDay < 1 || *in.DueDay > 31) {
		writeErr(w, 400, "invalid_request", "还款日须在 1..31")
		return
	}
	if in.Cycle != nil && *in.Cycle != "monthly" && *in.Cycle != "custom" {
		writeErr(w, 400, "invalid_request", "周期仅支持 monthly/custom")
		return
	}
	if in.DefaultChannel != nil && *in.DefaultChannel != "cash" && *in.DefaultChannel != "credit" {
		writeErr(w, 400, "invalid_request", "默认渠道仅支持 cash/credit")
		return
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "inactive" {
		writeErr(w, 400, "invalid_request", "status 仅支持 active/inactive")
		return
	}
	if in.EndOn != nil && !validDate(*in.EndOn) {
		writeErr(w, 400, "invalid_request", "结束日期格式不正确")
		return
	}
	finalStatus := "active"
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(r.Context(),
			`SELECT status FROM debt_contracts WHERE id=? AND household_id=?`, id, p.HouseholdID).
			Scan(&finalStatus); err != nil {
			return errors.New("负债合同不存在")
		}
		res, err := tx.ExecContext(r.Context(), `
			UPDATE debt_contracts SET
			  name=COALESCE(?,name), note=COALESCE(?,note), end_on=COALESCE(?,end_on),
			  due_day=COALESCE(?,due_day), cycle=COALESCE(?,cycle),
			  default_channel=COALESCE(?,default_channel),
			  default_channel_id=COALESCE(?,default_channel_id),
			  status=COALESCE(?,status)
			WHERE id=? AND household_id=?`,
			ptrArg(in.Name), ptrArg(in.Note), ptrArg(in.EndOn), ptrArg(in.DueDay),
			ptrArg(in.Cycle), ptrArg(in.DefaultChannel), ptrArg(in.DefaultChannelID),
			ptrArg(in.Status), id, p.HouseholdID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errors.New("负债合同不存在")
		}
		if in.Status != nil {
			finalStatus = *in.Status
		}
		return audit(r.Context(), tx, p, "patch_debt", "debt_contract", id, "")
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "status": finalStatus,
		"note": "本金、期次计划与已入账分录均不可编辑；纠错请走冲正",
	})
}

type debtScheduleInput struct {
	InstallmentNo  int    `json:"installment_no"`
	DueOn          string `json:"due_on"`
	PrincipalMinor int64  `json:"principal_minor"`
	InterestMinor  int64  `json:"interest_minor"`
	FeeMinor       int64  `json:"fee_minor"`
}

func (a *API) handleDebtSchedule(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var in struct {
		Items []debtScheduleInput `json:"items"`
	}
	if err := decode(r, &in); err != nil || len(in.Items) == 0 {
		writeErr(w, 400, "invalid_request", "至少需要一个期次")
		return
	}
	seen := map[int]bool{}
	var plannedPrincipal int64
	for _, it := range in.Items {
		if it.InstallmentNo < 1 || seen[it.InstallmentNo] {
			writeErr(w, 400, "invalid_request", "期次编号必须为正且唯一")
			return
		}
		seen[it.InstallmentNo] = true
		if !validDate(it.DueOn) {
			writeErr(w, 400, "invalid_request", "期次日期须为 YYYY-MM-DD")
			return
		}
		if it.PrincipalMinor < 0 || it.InterestMinor < 0 || it.FeeMinor < 0 {
			writeErr(w, 400, "invalid_amount", "本金/利息/费用不能为负")
			return
		}
		if it.PrincipalMinor+it.InterestMinor+it.FeeMinor <= 0 {
			writeErr(w, 400, "invalid_amount", "每期应付必须大于零")
			return
		}
		plannedPrincipal += it.PrincipalMinor
	}
	created := 0
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var kind, status string
		var principal int64
		if err := tx.QueryRowContext(r.Context(),
			`SELECT kind,status,principal_minor FROM debt_contracts WHERE id=? AND household_id=?`,
			id, p.HouseholdID).Scan(&kind, &status, &principal); err != nil {
			return errors.New("负债合同不存在")
		}
		if status != "active" {
			return errors.New("已停用合同不能新增期次")
		}
		var existing int
		if err := tx.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM debt_schedule_items WHERE contract_id=?`, id).Scan(&existing); err != nil {
			return err
		}
		if existing > 0 {
			return errors.New("期次计划已存在；请使用期次调整接口，不得静默覆盖")
		}
		if kind == "insurance" && plannedPrincipal != 0 {
			return errors.New("保险期次不得包含贷款本金")
		}
		if plannedPrincipal > principal {
			return fmt.Errorf("期次本金合计 %d 超过合同本金 %d", plannedPrincipal, principal)
		}
		for _, it := range in.Items {
			if kind == "insurance" && it.PrincipalMinor != 0 {
				return errors.New("保险期次不得包含贷款本金")
			}
			total := it.PrincipalMinor + it.InterestMinor + it.FeeMinor
			if _, err := tx.ExecContext(r.Context(), `
				INSERT INTO debt_schedule_items(id,contract_id,installment_no,due_on,
				  principal_minor,interest_minor,fee_minor,total_minor,paid_minor,status)
				VALUES(?,?,?,?,?,?,?,?,0,'pending')`,
				newID("dsi"), id, it.InstallmentNo, it.DueOn,
				it.PrincipalMinor, it.InterestMinor, it.FeeMinor, total); err != nil {
				return err
			}
			created++
		}
		return audit(r.Context(), tx, p, "create_debt_schedule", "debt_contract", id, "")
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"contract_id": id, "created": created, "status": "pending"})
}

func (a *API) handleDebtPayment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	contractID := r.PathValue("id")
	var in struct {
		InstallmentNo  int     `json:"installment_no"`
		AmountMinor    int64   `json:"amount_minor"`
		PaidOn         string  `json:"paid_on"`
		Channel        string  `json:"channel"`
		ChannelID      string  `json:"channel_id"`
		PrincipalMinor *int64  `json:"principal_minor"`
		InterestMinor  *int64  `json:"interest_minor"`
		FeeMinor       *int64  `json:"fee_minor"`
		Note           *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || !validDate(in.PaidOn) || in.AmountMinor <= 0 ||
		(in.Channel != "cash" && in.Channel != "credit") || in.ChannelID == "" {
		writeErr(w, 400, "invalid_request", "日期、金额与付款渠道不正确")
		return
	}
	// The 10 accounting policies are still unconfirmed: never infer an
	// allocation order. A payment MUST carry an explicit principal/interest/fee split.
	if in.PrincipalMinor == nil || in.InterestMinor == nil || in.FeeMinor == nil {
		writeErr(w, 400, "split_required", "部分付款口径未确认，请显式填写本金/利息/手续费拆分")
		return
	}
	pv, iv, fv := *in.PrincipalMinor, *in.InterestMinor, *in.FeeMinor
	if pv < 0 || iv < 0 || fv < 0 || pv+iv+fv != in.AmountMinor {
		writeErr(w, 400, "invalid_amount", "拆分金额之和必须等于本次实付金额")
		return
	}
	key := scopedKey(p.HouseholdID, idempotencyKey(r))
	if key != "" {
		var existingID, existingEntry string
		if err := a.DB.QueryRowContext(r.Context(), `
			SELECT p.id,p.entry_id FROM debt_payments p
			JOIN debt_schedule_items s ON s.id=p.schedule_item_id
			JOIN debt_contracts c ON c.id=s.contract_id
			WHERE p.idempotency_key=? AND c.household_id=? AND c.id=?`,
			key, p.HouseholdID, contractID).Scan(&existingID, &existingEntry); err == nil {
			writeJSON(w, 200, map[string]any{
				"id": existingID, "entry_id": existingEntry, "contract_id": contractID,
				"status": "confirmed", "replayed": true,
			})
			return
		}
	}
	payID, entryID := newID("dpay"), newID("je")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var itemID, dueOn, status, kind string
		var plP, plI, plF, total, paid int64
		if err := tx.QueryRowContext(r.Context(), `
			SELECT s.id,s.due_on,s.status,s.principal_minor,s.interest_minor,s.fee_minor,
			       s.total_minor,s.paid_minor,c.kind
			FROM debt_schedule_items s JOIN debt_contracts c ON c.id=s.contract_id
			WHERE s.contract_id=? AND s.installment_no=? AND c.household_id=?`,
			contractID, in.InstallmentNo, p.HouseholdID).
			Scan(&itemID, &dueOn, &status, &plP, &plI, &plF, &total, &paid, &kind); err != nil {
			return errors.New("期次不存在")
		}
		if status == "paid" || status == "cancelled" {
			return errors.New("该期次已结清或已取消")
		}
		if in.AmountMinor > total-paid {
			return errors.New("付款金额超过该期剩余应付")
		}
		var usedP, usedI, usedF int64
		if err := tx.QueryRowContext(r.Context(),
			`SELECT COALESCE(SUM(principal_minor),0),COALESCE(SUM(interest_minor),0),COALESCE(SUM(fee_minor),0)
			 FROM debt_payments WHERE schedule_item_id=? AND reversed=0`, itemID).
			Scan(&usedP, &usedI, &usedF); err != nil {
			return err
		}
		if pv > plP-usedP || iv > plI-usedI || fv > plF-usedF {
			return errors.New("拆分金额超过该期对应科目的剩余金额")
		}
		if kind == "insurance" && pv != 0 {
			return errors.New("保险期次不得冲减贷款本金")
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
		e, err := ledger.DebtPayment(
			ledger.Context{EntryID: entryID, BizID: payID, OccurredOn: in.PaidOn},
			contractID, paySubject, kind,
			ledger.DebtPaymentSpec{Principal: money.Minor(pv), Interest: money.Minor(iv), Fee: money.Minor(fv)})
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, key, p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO debt_payments(id,schedule_item_id,amount_minor,paid_on,channel,channel_id,entry_id,
			  idempotency_key,principal_minor,interest_minor,fee_minor,reversed)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,0)`,
			payID, itemID, in.AmountMinor, in.PaidOn, in.Channel, in.ChannelID, entryID,
			nullableKey(key), pv, iv, fv); err != nil {
			return err
		}
		// A credit-channel payment creates a NEW credit liability. Fold the
		// amount into that card's billing cycle (no journal entry is created here).
		if in.Channel == "credit" {
			if err := a.assignStatement(r, tx, p, in.ChannelID, "debt_payment", payID,
				in.AmountMinor, in.PaidOn); err != nil {
				return err
			}
		}
		newPaid := paid + in.AmountMinor
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE debt_schedule_items SET paid_minor=?, status=? WHERE id=?`,
			newPaid, derivePeriodStatus(dueOn, a.businessDate(), total, newPaid), itemID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE debt_contracts SET remaining_principal_minor=remaining_principal_minor-? WHERE id=?`,
			pv, contractID); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "debt_payment", "debt_payment", payID, in.PaidOn)
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{
		"id": payID, "entry_id": entryID, "status": "confirmed",
		"split": map[string]any{"principal_minor": pv, "interest_minor": iv, "fee_minor": fv},
	})
}

func (a *API) handleReverseDebtPayment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	contractID, paymentID := r.PathValue("id"), r.PathValue("paymentId")
	var in struct {
		Reason string `json:"reason"`
	}
	_ = decode(r, &in)
	revID := newID("je")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var entryID, paidOn, channel, channelID, itemID, dueOn, kind string
		var amount, pv, iv, fv, total, paid int64
		var reversed int
		if err := tx.QueryRowContext(r.Context(), `
			SELECT p.entry_id,p.paid_on,p.channel,p.channel_id,p.amount_minor,
			       p.principal_minor,p.interest_minor,p.fee_minor,p.reversed,
			       p.schedule_item_id,s.due_on,s.total_minor,s.paid_minor,c.kind
			FROM debt_payments p
			JOIN debt_schedule_items s ON s.id=p.schedule_item_id
			JOIN debt_contracts c ON c.id=s.contract_id
			WHERE p.id=? AND c.id=? AND c.household_id=?`,
			paymentID, contractID, p.HouseholdID).
			Scan(&entryID, &paidOn, &channel, &channelID, &amount,
				&pv, &iv, &fv, &reversed, &itemID, &dueOn, &total, &paid, &kind); err != nil {
			return errors.New("付款记录不存在")
		}
		if reversed == 1 {
			return errors.New("该付款已冲正")
		}
		orig, err := loadEntry(tx, r, entryID)
		if err != nil {
			return err
		}
		rev, err := orig.Reverse(revID, a.businessDate())
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, rev, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE journal_entries SET status='reversed' WHERE id=?`, entryID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE debt_payments SET reversed=1 WHERE id=?`, paymentID); err != nil {
			return err
		}
		newPaid := paid - amount
		if newPaid < 0 {
			newPaid = 0
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE debt_schedule_items SET paid_minor=?, status=? WHERE id=?`,
			newPaid, derivePeriodStatus(dueOn, a.businessDate(), total, newPaid), itemID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE debt_contracts SET remaining_principal_minor=remaining_principal_minor+? WHERE id=?`,
			pv, contractID); err != nil {
			return err
		}
		// Reverse the credit-statement effect too, in the SAME cycle as the
		// original payment so the bill stays reconcilable with the journal.
		if channel == "credit" {
			if err := a.assignStatement(r, tx, p, channelID, "debt_payment_reversal", revID,
				-amount, paidOn); err != nil {
				return err
			}
		}
		return audit(r.Context(), tx, p, "reverse_debt_payment", "debt_payment", paymentID, in.Reason)
	})
	if err != nil {
		writeErr(w, 400, "conflict", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"payment_id": paymentID, "status": "reversed", "reversal_entry_id": revID,
		"note": "原分录与冲正分录均保留；期次与剩余本金已按冲正回滚",
	})
}

func (a *API) handleSettleEarlyDebt(w http.ResponseWriter, r *http.Request) {
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
			`SELECT COUNT(*) FROM debt_contracts WHERE id=? AND household_id=?`, id, p.HouseholdID).Scan(&n); err != nil || n == 0 {
			return errors.New("负债合同不存在")
		}
		// Paid / partially-paid periods are NEVER silently deleted.
		res, err := tx.ExecContext(r.Context(), `
			UPDATE debt_schedule_items SET status='cancelled'
			WHERE contract_id=? AND paid_minor=0 AND status IN ('pending','partial','overdue')`, id)
		if err != nil {
			return err
		}
		n64, _ := res.RowsAffected()
		cancelled = int(n64)
		return audit(r.Context(), tx, p, "settle_early_debt", "debt_contract", id, in.Reason)
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
