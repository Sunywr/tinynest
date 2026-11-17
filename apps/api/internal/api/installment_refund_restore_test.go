package api

import (
	"testing"
)

// Reversing a refund whose money already relieved schedule obligations must
// restore the relieved periods (newest relief first, principal before fee)
// while leaving later real payments and the posted ledger untouched.

func restoreContract(t *testing.T, c *testClient, principal, fee int64, periods int, firstDue string) (string, string) {
	t.Helper()
	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "退款恢复账户", "opening_balance_minor": 50_000})["id"].(string)
	id := makeInstallment(t, c, principal, fee, periods, firstDue)
	return id, cash
}

func periodOf(t *testing.T, detail map[string]any, no int) map[string]any {
	t.Helper()
	sched, _ := detail["schedule"].([]any)
	if no < 1 || no > len(sched) {
		t.Fatalf("period %d missing in %v", no, sched)
	}
	return sched[no-1].(map[string]any)
}

func TestReverseInstallmentRefundRestoresReliefLifoPartial(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	// 3 periods of (10000 principal + 1000 fee). Refunds 12000 + 8000.
	// Relief: period 2 reduced to 9000 (-2000), period 3 cancelled (-11000).
	inst, cash := restoreContract(t, c, 30_000, 3_000, 3, "2026-12-01")
	path := "/api/v1/installments/" + inst
	c.mustOK("POST", path+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 11_000, "paid_on": "2026-12-01", "channel": "cash", "channel_id": cash})
	c.mustOK("POST", path+"/refunds", map[string]any{
		"amount_minor": 12_000, "refund_on": "2026-12-05", "destination_type": "cash", "destination_id": cash})
	refundB := c.mustOK("POST", path+"/refunds", map[string]any{
		"amount_minor": 8_000, "refund_on": "2026-12-06", "destination_type": "cash", "destination_id": cash})["id"].(string)
	c.mustOK("POST", path+"/adjust-schedule", map[string]any{"reason": "按退款减免", "items": []map[string]any{
		{"installment_no": 2, "action": "reduce", "principal_minor": 8_000, "fee_minor": 1_000}}})
	c.mustOK("POST", path+"/adjust-schedule", map[string]any{"reason": "取消第三期", "items": []map[string]any{
		{"installment_no": 3, "action": "cancel"}}})
	// A real payment lands on the relieved period 2 afterwards.
	c.mustOK("POST", path+"/payments", map[string]any{
		"installment_no": 2, "amount_minor": 4_000, "paid_on": "2026-12-02", "channel": "cash", "channel_id": cash})
	// The cancelled period left a lingering projected reminder behind.
	var item3 string
	if err := database.QueryRow(
		`SELECT id FROM installment_schedule_items WHERE contract_id=? AND installment_no=3`, inst).Scan(&item3); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO schedule_due_items(id,household_id,source_type,source_id,biz_key,due_on,amount_minor,status,source_label)
		 VALUES('due_restore_3',(SELECT household_id FROM installment_contracts WHERE id=?),'installment',?,?,'2026-12-01',11000,'pending','T3 分期 第3期')`,
		inst, item3, inst+":installment:"+item3); err != nil {
		t.Fatal(err)
	}

	// Reversing refund B (8000) leaves 12000 refunds < 13000 relieved: the
	// newest relief (cancel of period 3) is partially restored, principal first.
	reason := map[string]string{"reason": "误录退款"}
	first := c.mustOK("POST", path+"/refunds/"+refundB+"/reverse", reason)
	if retry := c.mustOK("POST", path+"/refunds/"+refundB+"/reverse", reason); retry["reversal_entry_id"] != first["reversal_entry_id"] {
		t.Fatalf("restoring reversal not idempotent: %v / %v", first, retry)
	}

	detail := c.mustOK("GET", path, nil)
	if got := number(conservation(t, detail)["refunded_minor"]); got != 12_000 {
		t.Fatalf("refunded after restore = %d, want 12000", got)
	}
	// Period 3: cancelled -> pending with 1000 principal restored, no fee.
	p3 := periodOf(t, detail, 3)
	if p3["status"] != "pending" || number(p3["principal_minor"]) != 1_000 ||
		number(p3["fee_minor"]) != 0 || number(p3["total_minor"]) != 1_000 || number(p3["paid_minor"]) != 0 {
		t.Fatalf("period 3 partial restore = %v", p3)
	}
	// Period 2 keeps its relief AND the later real payment.
	p2 := periodOf(t, detail, 2)
	if number(p2["total_minor"]) != 9_000 || number(p2["paid_minor"]) != 4_000 || number(p2["remaining_minor"]) != 5_000 {
		t.Fatalf("period 2 must keep relief and later payment: %v", p2)
	}
	// Period 1 paid history untouched.
	if p1 := periodOf(t, detail, 1); p1["status"] != "paid" || number(p1["paid_minor"]) != 11_000 {
		t.Fatalf("period 1 changed: %v", p1)
	}
	// The restore is plan-only: the liability subject only ever carries the
	// principal (creation -30000, payments -10000/-3556) and neither the
	// refunds nor the restore touch it; cash moves only by the refund flow.
	if got := subjectMinor(t, database, "2020:"+inst); got != -16_444 {
		t.Fatalf("liability changed by plan restore: %d", got)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 47_000 {
		t.Fatalf("cash after restored reversal = %d, want 47000", got)
	}
	// The lingering reminder was reactivated with the restored remainder.
	var reminder int64
	var reminderStatus string
	if err := database.QueryRow(
		`SELECT amount_minor,status FROM schedule_due_items WHERE source_type='installment' AND source_id=?`, item3).
		Scan(&reminder, &reminderStatus); err != nil || reminder != 1_000 || reminderStatus != "pending" {
		t.Fatalf("restored reminder = %d/%s, err=%v", reminder, reminderStatus, err)
	}
	// The restoration is auditable as an adjustment of its own.
	if got := number(conservation(t, detail)["adjustment_count"]); got != 3 {
		t.Fatalf("restoration was not audited: %v", conservation(t, detail))
	}
}

func TestReverseInstallmentRefundRestoresNewestReliefFirst(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	// 3 periods of 10000. Refunds 10000 + 5000; relief 5000 (period 2) then
	// 3000 (period 3). Reversing refund A leaves 5000 < 8000: the newest
	// relief (period 3) is fully restored, the older one stays.
	inst, cash := restoreContract(t, c, 30_000, 0, 3, "2026-12-01")
	path := "/api/v1/installments/" + inst
	refundA := c.mustOK("POST", path+"/refunds", map[string]any{
		"amount_minor": 10_000, "refund_on": "2026-12-05", "destination_type": "cash", "destination_id": cash})["id"].(string)
	c.mustOK("POST", path+"/refunds", map[string]any{
		"amount_minor": 5_000, "refund_on": "2026-12-06", "destination_type": "cash", "destination_id": cash})
	c.mustOK("POST", path+"/adjust-schedule", map[string]any{"reason": "减免第二期", "items": []map[string]any{
		{"installment_no": 2, "action": "reduce", "principal_minor": 5_000, "fee_minor": 0}}})
	c.mustOK("POST", path+"/adjust-schedule", map[string]any{"reason": "减免第三期", "items": []map[string]any{
		{"installment_no": 3, "action": "reduce", "principal_minor": 7_000, "fee_minor": 0}}})

	reason := map[string]string{"reason": "误录退款"}
	c.mustOK("POST", path+"/refunds/"+refundA+"/reverse", reason)

	detail := c.mustOK("GET", path, nil)
	if got := number(conservation(t, detail)["refunded_minor"]); got != 5_000 {
		t.Fatalf("refunded after restore = %d, want 5000", got)
	}
	p3 := periodOf(t, detail, 3)
	if p3["status"] != "pending" || number(p3["principal_minor"]) != 10_000 || number(p3["total_minor"]) != 10_000 {
		t.Fatalf("newest relief must be restored first: %v", p3)
	}
	if p2 := periodOf(t, detail, 2); number(p2["total_minor"]) != 5_000 {
		t.Fatalf("older relief must stay: %v", p2)
	}
	if got := subjectMinor(t, database, "2020:"+inst); got != -30_000 {
		t.Fatalf("liability changed by plan restore: %d", got)
	}
}

func TestReverseInstallmentRefundKeepsRefusingUnrestorableRelief(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	// Early settlement cancels period 2 without an explicit adjust-schedule
	// relief, so reversing the refund has nothing safe to restore.
	inst, cash := restoreContract(t, c, 20_000, 0, 2, "2026-12-01")
	path := "/api/v1/installments/" + inst
	refund := c.mustOK("POST", path+"/refunds", map[string]any{
		"amount_minor": 12_000, "refund_on": "2026-12-05", "destination_type": "cash", "destination_id": cash})["id"].(string)
	c.mustOK("POST", path+"/settle-early", map[string]any{"reason": "提前结清"})
	if status, body := c.do("POST", path+"/refunds/"+refund+"/reverse", map[string]string{"reason": "误录退款"}); status != 409 {
		t.Fatalf("settle-early relief reversal = %d %v, want 409", status, body)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 62_000 {
		t.Fatalf("refused reversal moved cash: %d", got)
	}
	detail := c.mustOK("GET", path, nil)
	if p2 := periodOf(t, detail, 2); p2["status"] != "cancelled" {
		t.Fatalf("settle-early cancellation must stay: %v", p2)
	}
}

func TestReverseInstallmentRefundDetectsTornAdjustmentHistory(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	inst, cash := restoreContract(t, c, 20_000, 0, 2, "2026-12-01")
	path := "/api/v1/installments/" + inst
	refund := c.mustOK("POST", path+"/refunds", map[string]any{
		"amount_minor": 12_000, "refund_on": "2026-12-05", "destination_type": "cash", "destination_id": cash})["id"].(string)
	c.mustOK("POST", path+"/adjust-schedule", map[string]any{"reason": "取消第二期", "items": []map[string]any{
		{"installment_no": 2, "action": "cancel"}}})
	// Corrupt the history: an adjustment event that never applied to the plan.
	var household string
	if err := database.QueryRow(`SELECT id FROM households LIMIT 1`).Scan(&household); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO installment_adjustments(id,household_id,contract_id,action,payload_json,reason,actor,created_at)
		VALUES(?,?,?,'adjust_schedule',?,?,?,?)`,
		"iadj_bogus", household, inst, `{"items":[{"installment_no":1,"action":"reduce","principal_minor":5000,"fee_minor":0}]}`,
		"伪造", "tester", "2999-01-01T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	if status, body := c.do("POST", path+"/refunds/"+refund+"/reverse", map[string]string{"reason": "误录退款"}); status != 409 {
		t.Fatalf("torn history reversal = %d %v, want 409", status, body)
	}
	detail := c.mustOK("GET", path, nil)
	if got := number(conservation(t, detail)["refunded_minor"]); got != 12_000 {
		t.Fatalf("refused reversal changed refunds: %d", got)
	}
	if p1 := periodOf(t, detail, 1); p1["status"] != "pending" || number(p1["principal_minor"]) != 10_000 {
		t.Fatalf("refused reversal changed schedule: %v", p1)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 62_000 {
		t.Fatalf("refused reversal moved cash: %d", got)
	}
}
