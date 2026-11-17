package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
)

const refundJournalJoin = `FROM refunds r JOIN journal_entries e ON e.id=r.entry_id AND e.household_id=r.household_id`

type installmentRefundHistoryRow struct {
	ID          string `json:"id"`
	RefundOn    string `json:"refund_on"`
	Destination string `json:"destination_type"`
	AmountMinor int64  `json:"amount_minor"`
	Status      string `json:"status"`
}

func listInstallmentRefunds(ctx context.Context, q db.Queryer, householdID, transactionID string) ([]installmentRefundHistoryRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT r.id,r.refund_on,r.destination_type,r.amount_minor,e.status `+refundJournalJoin+`
		WHERE r.household_id=? AND r.transaction_id=? ORDER BY r.refund_on DESC,r.id DESC`, householdID, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []installmentRefundHistoryRow{}
	for rows.Next() {
		var item installmentRefundHistoryRow
		if err := rows.Scan(&item.ID, &item.RefundOn, &item.Destination, &item.AmountMinor, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func refundTotalForTransaction(ctx context.Context, q db.Queryer, householdID, transactionID string) (int64, error) {
	var total int64
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(r.amount_minor),0) `+refundJournalJoin+`
		WHERE r.household_id=? AND r.transaction_id=? AND e.status='posted'`, householdID, transactionID).Scan(&total)
	return total, err
}

var errReliefUnrestorable = errors.New("减免金额无法自动恢复（可能来自提前结清），请先人工核对")
var errReliefHistoryTorn = errors.New("减免记录与付款计划不一致，请人工核对")
var errRefundRecordUnavailable = errors.New("找不到可撤销的退款记账记录")

func (a *API) handleReverseInstallmentRefund(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	contractID, refundID := r.PathValue("id"), r.PathValue("refundId")
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || utf8.RuneCountInString(in.Reason) > 200 {
		writeErr(w, 400, "invalid_request", "撤销原因须为 1 至 200 个字符")
		return
	}
	var reversalID string
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		status, source, err := a.lockInstallment(r, tx, p.HouseholdID, contractID)
		if err != nil {
			return err
		}
		if !source.Valid {
			return sql.ErrNoRows
		}
		// The original purchase also guards refunds from the regular ledger.
		if _, err := a.installmentOpeningEntry(r, tx, p.HouseholdID, contractID, source); err != nil {
			return err
		}
		var entryID, destination, destinationID, refundStatus string
		var amount int64
		if err := tx.QueryRowContext(ctx, `
			SELECT r.entry_id,r.destination_type,r.destination_id,r.amount_minor,e.status `+refundJournalJoin+`
			WHERE r.household_id=? AND r.transaction_id=? AND r.id=?`,
			p.HouseholdID, source.String, refundID).Scan(&entryID, &destination, &destinationID, &amount, &refundStatus); err != nil {
			return err
		}
		if refundStatus == ledger.StatusReversed {
			return tx.QueryRowContext(ctx, `SELECT id FROM journal_entries WHERE household_id=? AND reversal_of=?`,
				p.HouseholdID, entryID).Scan(&reversalID)
		}
		if status == "cancelled" || amount <= 0 || refundStatus != ledger.StatusPosted {
			return errRefundRecordUnavailable
		}
		refunded, err := refundTotalForTransaction(ctx, tx, p.HouseholdID, source.String)
		if err != nil {
			return err
		}
		var relieved int64
		if err := tx.QueryRowContext(ctx, `
			SELECT c.principal_minor+c.total_fee_minor-COALESCE(SUM(s.total_minor),0)
			FROM installment_contracts c
			LEFT JOIN installment_schedule_items s ON s.contract_id=c.id AND s.status<>'cancelled'
			WHERE c.household_id=? AND c.id=? GROUP BY c.id`, p.HouseholdID, contractID).Scan(&relieved); err != nil {
			return err
		}
		if shortfall := max(0, relieved) - (refunded - amount); shortfall > 0 {
			if err := a.restoreReliefsForRefundReversal(r, tx, p.HouseholdID, contractID, refundID, shortfall, in.Reason, p.Username); err != nil {
				return err
			}
		}
		original, err := loadEntry(tx, r, entryID)
		if err != nil {
			return err
		}
		if original.BizType != ledger.BizRefund || original.BizID != refundID {
			return errRefundRecordUnavailable
		}
		reversalID = newID("je")
		reversal, err := original.Reverse(reversalID, original.OccurredOn)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(ctx, tx, reversal, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if destination == "credit" {
			if err := a.restoreCreditRefund(r, tx, p.HouseholdID, destinationID, refundID, reversalID, amount); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE journal_entries SET status='reversed' WHERE household_id=? AND id=?`,
			p.HouseholdID, entryID); err != nil {
			return err
		}
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "reverse_installment_refund", "refund", refundID, string(payload))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "分期或退款记录不存在")
	case errors.Is(err, errReliefUnrestorable), errors.Is(err, errReliefHistoryTorn), errors.Is(err, errRefundRecordUnavailable):
		writeErr(w, 409, "refund_not_reversible", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": refundID, "status": "reversed", "reversal_entry_id": reversalID})
	}
}

func (a *API) restoreCreditRefund(r *http.Request, tx *sql.Tx, householdID, instrumentID, refundID, reversalID string, amount int64) error {
	ctx := r.Context()
	query := `SELECT id FROM credit_instruments WHERE household_id=? AND id=?`
	if a.DB.IsMySQL() {
		query += " FOR UPDATE"
	}
	var lockedID string
	if err := tx.QueryRowContext(ctx, query, householdID, instrumentID).Scan(&lockedID); err != nil {
		return err
	}
	var statementID, originalDate string
	err := tx.QueryRowContext(ctx, `
		SELECT MIN(s.id),MIN(i.occurred_on)
		FROM credit_statement_items i JOIN credit_statements s ON s.id=i.statement_id
		WHERE s.household_id=? AND s.instrument_id=? AND i.source_type='refund'
		  AND i.source_id=? AND i.amount_minor=? HAVING COUNT(*)=1`,
		householdID, instrumentID, refundID, -amount).Scan(&statementID, &originalDate)
	if errors.Is(err, sql.ErrNoRows) {
		return errRefundRecordUnavailable
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE credit_statements SET system_due_minor=system_due_minor+?
		WHERE household_id=? AND id=?`, amount, householdID, statementID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_statement_items(statement_id,source_type,source_id,amount_minor,occurred_on)
		VALUES(?,'installment_refund_reversal',?,?,?)`, statementID, reversalID, amount, originalDate); err != nil {
		return err
	}
	return a.restoreStatementReminder(ctx, tx, householdID, statementID)
}

// reliefEvent is one adjust-schedule action replayed against the plan.
type reliefEvent struct {
	no                   int
	action               string
	p, f                 int64  // reduce payload targets (zero for cancel)
	createdAt            string // adjustment row timestamp (millisecond precision)
	seq                  int    // position in the deterministic base scan
	preP, preF           int64  // plan amounts right before this event
	postP, postF         int64  // plan amounts right after this event
	restoredP, restoredF int64  // plan amounts after the refund-reversal restore
}

// restoreReliefsForRefundReversal reinstates obligations that consumed the
// refund being reversed. Within each period the relief chain is rebuilt from
// amount semantics (a relief can only shrink obligations, so the chain order
// is the unique valid chronology) and replayed from the creation-time plan —
// the per-period split is fully determined by the contract — to recover every
// event's pre-state. The replayed final state must match the stored rows,
// otherwise the history is torn and the reversal refuses. Chains are then
// undone newest-first (later period breaks same-moment ties), principal
// before fee, until the shortfall is covered; later real payments stay
// untouched because payments only ever touch paid_minor and never rewrite
// relieved amounts.
func (a *API) restoreReliefsForRefundReversal(r *http.Request, tx *sql.Tx, householdID, contractID, refundID string, shortfall int64, reason, actor string) error {
	ctx := r.Context()
	var principal, fee int64
	var periods int
	var name string
	if err := tx.QueryRowContext(ctx, `
		SELECT principal_minor,total_fee_minor,periods,name FROM installment_contracts
		WHERE household_id=? AND id=?`, householdID, contractID).Scan(&principal, &fee, &periods, &name); err != nil {
		return err
	}
	original := func(no int) (int64, int64) {
		baseP, baseF := principal/int64(periods), fee/int64(periods)
		if no == periods {
			return principal - baseP*int64(periods-1), fee - baseF*int64(periods-1)
		}
		return baseP, baseF
	}
	// Load the adjustment history in a deterministic base order. created_at
	// only has millisecond precision and ids are random, so timestamps alone
	// cannot order events; the per-period chains below are rebuilt from amount
	// semantics (a relief can only shrink obligations), which yields the true
	// chronology within each period. Cross-period ties fall back to the higher
	// period number, documented in the plan.
	rows, err := tx.QueryContext(ctx, `
		SELECT payload_json,created_at FROM installment_adjustments
		WHERE household_id=? AND contract_id=? AND action='adjust_schedule'
		ORDER BY created_at ASC, id ASC`, householdID, contractID)
	if err != nil {
		return err
	}
	type payload struct {
		Items []struct {
			InstallmentNo  int    `json:"installment_no"`
			Action         string `json:"action"`
			PrincipalMinor *int64 `json:"principal_minor"`
			FeeMinor       *int64 `json:"fee_minor"`
		} `json:"items"`
	}
	var scanned []*reliefEvent
	seq := 0
	for rows.Next() {
		var raw, createdAt string
		if err := rows.Scan(&raw, &createdAt); err != nil {
			_ = rows.Close()
			return err
		}
		var pl payload
		if err := json.Unmarshal([]byte(raw), &pl); err != nil {
			_ = rows.Close()
			return errReliefHistoryTorn
		}
		for _, it := range pl.Items {
			ev := &reliefEvent{no: it.InstallmentNo, action: it.Action, createdAt: createdAt, seq: seq}
			seq++
			if it.Action == "reduce" {
				if it.PrincipalMinor == nil || it.FeeMinor == nil {
					_ = rows.Close()
					return errReliefHistoryTorn
				}
				ev.p, ev.f = *it.PrincipalMinor, *it.FeeMinor
			}
			scanned = append(scanned, ev)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if len(scanned) == 0 {
		return errReliefUnrestorable
	}
	// Group by period. A period is relieved by at most one cancel, and that
	// cancel is terminal; reduces chain in strictly shrinking post-state
	// order (largest post-total first), which is the only valid chronology.
	byPeriod := map[int][]*reliefEvent{}
	for _, ev := range scanned {
		byPeriod[ev.no] = append(byPeriod[ev.no], ev)
	}
	type periodChain struct {
		no      int
		events  []*reliefEvent // chronological
		lastAt  string
		lastSeq int
	}
	chains := []periodChain{}
	for no, evs := range byPeriod {
		var cancels, reduces []*reliefEvent
		for _, ev := range evs {
			switch ev.action {
			case "cancel":
				cancels = append(cancels, ev)
			case "reduce":
				reduces = append(reduces, ev)
			default:
				return errReliefHistoryTorn
			}
		}
		if len(cancels) > 1 {
			return errReliefHistoryTorn
		}
		// Shrinking post-total first; equal posts keep the base scan order.
		for i := 1; i < len(reduces); i++ {
			for j := i; j > 0 && (reduces[j].p+reduces[j].f) > (reduces[j-1].p+reduces[j-1].f); j-- {
				reduces[j], reduces[j-1] = reduces[j-1], reduces[j]
			}
		}
		chain := periodChain{no: no, events: reduces}
		if len(reduces) > 0 {
			last := reduces[len(reduces)-1]
			chain.lastAt, chain.lastSeq = last.createdAt, last.seq
		}
		if len(cancels) == 1 {
			if len(reduces) > 0 {
				last := reduces[len(reduces)-1]
				if cancels[0].createdAt < last.createdAt ||
					(cancels[0].createdAt == last.createdAt && cancels[0].seq < last.seq) {
					return errReliefHistoryTorn
				}
			}
			chain.events = append(chain.events, cancels[0])
			chain.lastAt, chain.lastSeq = cancels[0].createdAt, cancels[0].seq
		}
		chains = append(chains, chain)
	}
	// Replay each chain from the creation-time plan to recover pre-states.
	byNo := map[int]instalPeriod{}
	stored, err := a.loadInstalPeriods(r, tx, householdID, contractID)
	if err != nil {
		return err
	}
	for _, sp := range stored {
		byNo[sp.no] = sp
	}
	for i := range chains {
		ch := &chains[i]
		row, ok := byNo[ch.no]
		if !ok {
			return errReliefHistoryTorn
		}
		p0, f0 := original(ch.no)
		curP, curF := p0, f0
		for _, ev := range ch.events {
			ev.preP, ev.preF = curP, curF
			if ev.action == "cancel" {
				ev.postP, ev.postF = 0, 0
			} else {
				if ev.p < 0 || ev.f < 0 || ev.p > curP || ev.f > curF {
					return errReliefHistoryTorn
				}
				ev.postP, ev.postF = ev.p, ev.f
			}
			curP, curF = ev.postP, ev.postF
		}
		// The replayed chain must reproduce the stored plan exactly.
		lastEv := ch.events[len(ch.events)-1]
		if lastEv.action == "cancel" {
			if row.status != "cancelled" || row.principal != lastEv.preP || row.fee != lastEv.preF {
				return errReliefHistoryTorn
			}
		} else {
			if row.status == "cancelled" || row.principal != lastEv.postP || row.fee != lastEv.postF {
				return errReliefHistoryTorn
			}
		}
	}
	// Undo the newest relief first (later period breaks same-moment ties),
	// principal before fee, until the shortfall is covered.
	for i := 1; i < len(chains); i++ {
		for j := i; j > 0 && (chains[j].lastAt > chains[j-1].lastAt ||
			(chains[j].lastAt == chains[j-1].lastAt && chains[j].no > chains[j-1].no)); j-- {
			chains[j], chains[j-1] = chains[j-1], chains[j]
		}
	}
	remaining := shortfall
	var undone []*reliefEvent
	for ci := range chains {
		evs := chains[ci].events
		for i := len(evs) - 1; i >= 0 && remaining > 0; i-- {
			ev := evs[i]
			capacity := (ev.preP - ev.postP) + (ev.preF - ev.postF)
			if capacity <= 0 {
				continue
			}
			take := min(remaining, capacity)
			rp := min(take, ev.preP-ev.postP)
			ev.restoredP, ev.restoredF = ev.postP+rp, ev.postF+(take-rp)
			remaining -= take
			undone = append(undone, ev)
		}
	}
	if remaining > 0 {
		return errReliefUnrestorable
	}
	for _, ev := range undone {
		row := byNo[ev.no]
		status := row.status
		if ev.action == "cancel" {
			// A cancelled period is reactivated even on a partial restore.
			status = "pending"
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE installment_schedule_items SET principal_minor=?,fee_minor=?,total_minor=?,status=?
			WHERE id=?`, ev.restoredP, ev.restoredF, ev.restoredP+ev.restoredF, status, row.id); err != nil {
			return err
		}
		if rem := ev.restoredP + ev.restoredF - row.paid; rem > 0 {
			if err := a.restoreDueReminder(ctx, tx, householdID, dueReminder{
				kind: "installment", id: row.id, due: row.due, label: name + " 第" + itoa(ev.no) + "期", remaining: rem,
			}); err != nil {
				return err
			}
		}
	}
	type restoredItem struct {
		InstallmentNo  int    `json:"installment_no"`
		Action         string `json:"action"`
		PrincipalMinor int64  `json:"principal_minor"`
		FeeMinor       int64  `json:"fee_minor"`
	}
	items := []restoredItem{}
	var restoredTotal int64
	for _, ev := range undone {
		items = append(items, restoredItem{
			InstallmentNo: ev.no, Action: ev.action,
			PrincipalMinor: ev.restoredP, FeeMinor: ev.restoredF,
		})
		restoredTotal += (ev.restoredP - ev.postP) + (ev.restoredF - ev.postF)
	}
	auditPayload, err := json.Marshal(map[string]any{
		"refund_id": refundID, "restored_minor": restoredTotal, "items": items,
	})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO installment_adjustments(id,household_id,contract_id,action,payload_json,reason,actor)
		VALUES(?,?,?,'restore_for_refund_reversal',?,?,?)`,
		newID("iadj"), householdID, contractID, string(auditPayload), reason, actor)
	return err
}
