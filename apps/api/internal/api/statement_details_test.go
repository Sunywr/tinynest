package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
)

// Uploading the bank's XLS detail report is the path that turns a credit-card bill
// into real ledger lines. The server classifies every row from the bank's own
// description and posts ONLY charges as 5010 expenses: refunds, repayments/transfers,
// fees and anything it cannot recognise are recorded as held rows and never become a
// transaction, because a negative amount in a bank report is not proof of a refund.
// Booking dates use the reported 交易日期, and idempotency is per line so re-uploading
// a statement cannot post an expense twice.

// spdbRow is one 浦发 data line in the measured column order:
// 交易日期 / 记账日期 / 交易摘要 / 卡号末四位 / 卡片类型 / 交易币种 / 交易金额 / 原始交易金额&币种
func spdbRow(txnOn, postOn, description, tail, currency, amount string) []string {
	return []string{txnOn, postOn, description, tail, "主卡", currency, amount, amount + "(" + currency + ")"}
}

func spdbMatrix(rows ...[]string) [][]string {
	header := []string{"交易日期", "记账日期", "交易摘要", "卡号末四位", "卡片类型", "交易币种", "交易金额", "原始交易金额&币种"}
	out := append([][]string{header}, rows...)
	return out
}

// sampleMatrix mixes every classification the parser must separate: two charges, a
// refund, a self-repayment, a fee, an unrecognised negative, a foreign-currency row
// and a line belonging to another card.
func sampleMatrix() [][]string {
	return spdbMatrix(
		spdbRow("20261020", "20261022", "支付宝－某商户", "4321", "人民币", "123.45"),
		spdbRow("20261025", "20261026", "某超市", "4321", "人民币", "300.00"),
		spdbRow("20261026", "20261027", "某商户退款", "4321", "人民币", "-60.00"),
		spdbRow("20261028", "20261028", "自助还款", "4321", "人民币", "-500.00"),
		spdbRow("20261029", "20261029", "年费", "4321", "人民币", "100.00"),
		spdbRow("20261030", "20261030", "神秘交易", "4321", "人民币", "-9.99"),
		spdbRow("20261031", "20261102", "境外消费", "4321", "USD", "10.00"),
		spdbRow("20261020", "20261021", "别人卡的消费", "8888", "人民币", "99.99"),
	)
}

func (f *statementSyncFixture) detailInput(rows [][]string) map[string]any {
	return map[string]any{"rows": rows, "card_tail": "4321"}
}

func (f *statementSyncFixture) preview(body map[string]any) (int, map[string]any) {
	return f.client.do("POST", "/api/v1/accounts/"+f.cardID+"/statement-detail-preview", body)
}

func (f *statementSyncFixture) importDetails(body map[string]any) (int, map[string]any) {
	return f.client.do("POST", "/api/v1/accounts/"+f.cardID+"/statement-detail-import", body)
}

func (f *statementSyncFixture) detail(path string, body map[string]any) (int, map[string]any) {
	return f.client.do("POST", path, body)
}

func detailCounts(body map[string]any) map[string]int64 {
	counts, _ := body["counts"].(map[string]any)
	out := map[string]int64{}
	for key, value := range counts {
		out[key] = number(value)
	}
	return out
}

func detailItems(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, _ := body["items"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, value := range raw {
		row, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("detail item is not an object: %v", value)
		}
		out = append(out, row)
	}
	return out
}

func findDetailItem(t *testing.T, body map[string]any, description string) map[string]any {
	t.Helper()
	for _, item := range detailItems(t, body) {
		if item["description"] == description {
			return item
		}
	}
	t.Fatalf("no detail item for %q in %v", description, body["items"])
	return nil
}

// detailRows reads the stored statement_details rows straight from the database so a
// test can tell "classified as held" apart from "never seen at all".
func (f *statementSyncFixture) detailRows() []map[string]any {
	f.t.Helper()
	rows, err := f.database.Query(`SELECT kind,state,txn_on,post_on,description,amount_minor,review_reason,transaction_id
		FROM statement_details ORDER BY row_no,seq`)
	if err != nil {
		f.t.Fatalf("statement_details: %v", err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var kind, state, txnOn, description, reviewReason string
		var postOn, transactionID sql.NullString
		var amount int64
		if err := rows.Scan(&kind, &state, &txnOn, &postOn, &description, &amount, &reviewReason, &transactionID); err != nil {
			f.t.Fatalf("scan statement_details: %v", err)
		}
		item := map[string]any{"kind": kind, "state": state, "txn_on": txnOn, "post_on": postOn.String,
			"description": description, "amount_minor": amount, "review_reason": reviewReason}
		if transactionID.Valid {
			item["transaction_id"] = transactionID.String
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		f.t.Fatalf("statement_details rows: %v", err)
	}
	return out
}

func (f *statementSyncFixture) sourceCount() int {
	f.t.Helper()
	var rows int
	if err := f.database.QueryRow(`SELECT COUNT(*) FROM card_statement_sources WHERE kind='xls'`).Scan(&rows); err != nil {
		f.t.Fatalf("count xls sources: %v", err)
	}
	return rows
}

func (f *statementSyncFixture) sourceKind() string {
	f.t.Helper()
	var kind string
	if err := f.database.QueryRow(`SELECT kind FROM card_statement_sources WHERE kind='xls' LIMIT 1`).Scan(&kind); err != nil {
		return ""
	}
	return kind
}

func (f *statementSyncFixture) householdID() string {
	f.t.Helper()
	var id string
	if err := f.database.QueryRow(`SELECT id FROM households LIMIT 1`).Scan(&id); err != nil {
		f.t.Fatalf("household id: %v", err)
	}
	return id
}

func (f *statementSyncFixture) detailCount() int {
	f.t.Helper()
	var rows int
	if err := f.database.QueryRow(`SELECT COUNT(*) FROM statement_details`).Scan(&rows); err != nil {
		f.t.Fatalf("count statement_details: %v", err)
	}
	return rows
}

func (f *statementSyncFixture) transactionsFor(amount int64) int {
	f.t.Helper()
	var rows int
	if err := f.database.QueryRow(`SELECT COUNT(*) FROM transactions WHERE amount_minor=?`, amount).Scan(&rows); err != nil {
		f.t.Fatalf("count transactions: %v", err)
	}
	return rows
}

func TestStatementDetailPreviewClassifiesWithoutWrites(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	before := fixture.counts()

	status, body := fixture.preview(map[string]any{"rows": sampleMatrix(), "card_tail": "4321"})
	if status != http.StatusOK {
		t.Fatalf("preview = %d %v, want 200", status, body)
	}
	counts := detailCounts(body)
	for key, want := range map[string]int64{
		"rows": 8, "charges": 2, "refunds": 1, "payments": 1, "fees": 1, "review": 3,
		"matched": 0, "missing": 2, "held": 6,
	} {
		if counts[key] != want {
			t.Fatalf("counts[%s] = %d, want %d (all: %v)", key, counts[key], want, counts)
		}
	}
	if number(body["charge_minor"]) != 42345 || number(body["missing_minor"]) != 42345 {
		t.Fatalf("preview totals = %v / %v, want 42345", body["charge_minor"], body["missing_minor"])
	}
	for description, want := range map[string][2]string{
		"支付宝－某商户": {"charge", "missing"},
		"某商户退款":   {"refund", "held"},
		"自助还款":    {"payment", "held"},
		"年费":      {"fee", "held"},
		"神秘交易":    {"review", "held"},
		"境外消费":    {"review", "held"},
		"别人卡的消费":  {"review", "held"},
	} {
		item := findDetailItem(t, body, description)
		if item["kind"] != want[0] || item["state"] != want[1] {
			t.Fatalf("%s = %v/%v, want %v", description, item["kind"], item["state"], want)
		}
	}
	// A preview must be free of side effects: nothing is booked until the user
	// confirms, and the household must be able to preview repeatedly.
	after := fixture.counts()
	if after != before || fixture.detailCount() != 0 {
		t.Fatalf("preview wrote rows: %+v -> %+v, details=%d", before, after, fixture.detailCount())
	}
}

func TestStatementDetailImportPostsOnlyCharges(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.expense(30000, "2026-10-25") // already booked, same day and amount
	before := fixture.counts()

	status, body := fixture.importDetails(map[string]any{"rows": sampleMatrix(), "card_tail": "4321"})
	if status != http.StatusOK {
		t.Fatalf("import = %d %v, want 200", status, body)
	}
	if number(body["created"]) != 1 || number(body["matched"]) != 1 || number(body["held"]) != 6 {
		t.Fatalf("import counts = %v, want created 1, matched 1, held 6", body)
	}
	if number(body["amount_minor"]) != 12345 {
		t.Fatalf("imported amount = %v, want 12345", body["amount_minor"])
	}
	created, _ := body["transactions"].([]any)
	if len(created) != 1 {
		t.Fatalf("imported transactions = %v, want exactly one", body["transactions"])
	}
	transactionID, _ := created[0].(string)
	row := fixture.client.mustOK("GET", "/api/v1/transactions/"+transactionID, nil)
	// 交易日期 is the business date, not 记账日期: the charge belongs to the day the money
	// was spent, which is what the monthly reports and the statement window key on.
	if row["type"] != "expense" || number(row["amount_minor"]) != 12345 ||
		row["occurred_on"] != "2026-10-20" || row["account_id"] != fixture.cardID ||
		row["status"] != "posted" {
		t.Fatalf("imported transaction: %v", row)
	}
	if name := fmt.Sprint(row["category_name"]); name != "账单补录" {
		t.Fatalf("imported category = %q, want 账单补录", name)
	}

	after := fixture.counts()
	if after.transactions != before.transactions+1 || after.entries != before.entries+1 ||
		after.lines != before.lines+2 {
		t.Fatalf("import wrote tx %d->%d, entries %d->%d, lines %d->%d, want +1/+1/+2",
			before.transactions, after.transactions, before.entries, after.entries, before.lines, after.lines)
	}
	// Every other row is stored but NOT booked. Repayments and transfers must never
	// reach a 5010 expense category, so their amounts appear in no transaction at all.
	for amount, label := range map[int64]string{
		6000: "退款", 50000: "自助还款", 10000: "年费", 999: "未识别负项", 1000: "外币", 9999: "他卡",
	} {
		if got := fixture.transactionsFor(amount); got != 0 {
			t.Fatalf("%s (%d 分) produced %d transactions, want none", label, amount, got)
		}
	}
	stored := fixture.detailRows()
	if len(stored) != 8 {
		t.Fatalf("stored details = %d, want all 8 rows", len(stored))
	}
	for _, item := range stored {
		switch item["description"] {
		case "某超市":
			if item["state"] != "matched" || item["transaction_id"] == nil {
				t.Fatalf("already-booked row = %v, want matched with its transaction", item)
			}
		case "支付宝－某商户":
			if item["state"] != "imported" || item["post_on"] != "2026-10-22" {
				t.Fatalf("imported row = %v, want state=imported and post_on kept for reference", item)
			}
		case "自助还款", "某商户退款", "年费", "神秘交易":
			if item["state"] != "held" || item["transaction_id"] != nil || item["review_reason"] == "" {
				t.Fatalf("held row %v must carry a reason and no transaction", item)
			}
		}
	}
}

func TestStatementDetailImportIsIdempotentPerLine(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	input := map[string]any{"rows": sampleMatrix(), "card_tail": "4321"}
	if status, body := fixture.importDetails(input); status != http.StatusOK || number(body["created"]) != 2 {
		t.Fatalf("first import = %d %v, want both charges booked", status, body)
	}
	firstCounts, firstDetails := fixture.counts(), fixture.detailCount()

	// Re-uploading the identical file must change nothing.
	status, body := fixture.importDetails(input)
	if status != http.StatusOK || number(body["created"]) != 0 || body["replayed"] != true {
		t.Fatalf("replay = %d %v, want 200 with created 0 and replayed", status, body)
	}
	if fixture.counts() != firstCounts || fixture.detailCount() != firstDetails {
		t.Fatalf("replay changed the ledger or details: %+v vs %+v", firstCounts, fixture.counts())
	}

	// A re-downloaded report that gained one extra charge must post only that line,
	// which is why the idempotency key is the row itself and not the file digest.
	extended := append(sampleMatrix(), spdbRow("20261027", "20261029", "多出来的一笔", "4321", "人民币", "77.70"))
	status, body = fixture.importDetails(map[string]any{"rows": extended, "card_tail": "4321"})
	if status != http.StatusOK || number(body["created"]) != 1 {
		t.Fatalf("extended import = %d %v, want one new charge only", status, body)
	}
	// 报告金额只算这一次真正入账的行：把此前已补录的行也计进来就是虚报。
	if number(body["amount_minor"]) != 7770 {
		t.Fatalf("extended import amount_minor = %v, want 7770 (only the new line)", body["amount_minor"])
	}
	if got := fixture.transactionsFor(7770); got != 1 {
		t.Fatalf("new line posted %d times, want 1", got)
	}
	if got := fixture.transactionsFor(12345); got != 1 {
		t.Fatalf("re-sent line posted %d times, want 1", got)
	}
	if got := fixture.detailCount(); got != 9 {
		t.Fatalf("stored details = %d, want 9", got)
	}
}

func TestStatementDetailImportRefusesAmbiguousMatch(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.expense(12345, "2026-10-20")
	fixture.expense(12345, "2026-10-20") // two candidates, indistinguishable
	before := fixture.counts()

	status, body := fixture.importDetails(map[string]any{"rows": sampleMatrix(), "card_tail": "4321"})
	if status != http.StatusOK {
		t.Fatalf("import = %d %v", status, body)
	}
	if number(body["created"]) != 1 {
		t.Fatalf("created = %v, want only the unambiguous 300.00... got %v", body["created"], body)
	}
	item := findDetailItem(t, body, "支付宝－某商户")
	if item["state"] != "held" || item["reason"] == "" {
		t.Fatalf("ambiguous row = %v, want held with a reason", item)
	}
	if got := fixture.transactionsFor(12345); got != 2 {
		t.Fatalf("ambiguous match posted %d transactions, want the 2 pre-existing ones only", got)
	}
	after := fixture.counts()
	if after.transactions != before.transactions+1 {
		t.Fatalf("transactions %d -> %d, want only the unambiguous charge booked", before.transactions, after.transactions)
	}
}

func TestStatementDetailImportHonoursStatementWindow(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	statementID := statement["id"].(string)
	// The stored summary covers 2026-10-02..2026-11-01, so a later charge belongs to
	// the next bill and must not be swept into this one.
	rows := spdbMatrix(
		spdbRow("20261020", "20261021", "窗内消费", "4321", "人民币", "50.00"),
		spdbRow("20261120", "20261121", "窗外消费", "4321", "人民币", "60.00"),
	)

	status, body := fixture.importDetails(map[string]any{"rows": rows, "card_tail": "4321", "statement_id": statementID})
	if status != http.StatusOK {
		t.Fatalf("import = %d %v", status, body)
	}
	if number(body["created"]) != 1 || number(body["held"]) != 1 {
		t.Fatalf("windowed import = %v, want the in-window line only", body)
	}
	if got := fixture.transactionsFor(5000); got != 1 || fixture.transactionsFor(6000) != 0 {
		t.Fatalf("cross-cycle line posted: 5000=%d 6000=%d, want 1 and 0", got, fixture.transactionsFor(6000))
	}
	// Without a statement there is no window to check, so the same line is bookable:
	// the guard is the bill, not an invented date range.
	status, body = fixture.importDetails(map[string]any{"rows": spdbMatrix(
		spdbRow("20261121", "20261122", "另一笔窗外", "4321", "人民币", "66.00")), "card_tail": "4321"})
	if status != http.StatusOK || number(body["created"]) != 1 {
		t.Fatalf("unlinked import = %d %v, want the line booked", status, body)
	}
}

func TestStatementDetailImportLinksUploadedFileToStatement(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	rows := map[string]any{"rows": spdbMatrix(spdbRow("20261020", "20261021", "某商户", "4321", "人民币", "50.00")),
		"card_tail": "4321", "statement_id": statement["id"].(string)}

	if status, body := fixture.importDetails(rows); status != http.StatusOK {
		t.Fatalf("import = %d %v", status, body)
	}
	if got := fixture.sourceCount(); got != 1 {
		t.Fatalf("xls sources = %d, want one traceability row", got)
	}
	if kind := fixture.sourceKind(); kind != "xls" {
		t.Fatalf("source kind = %q, want xls", kind)
	}
	fixture.importDetails(rows)
	if got := fixture.sourceCount(); got != 1 {
		t.Fatalf("re-upload produced %d source rows, want the file counted once", got)
	}
}

func TestStatementDetailImportRefusesLivePlaceholderCycle(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	status, body := fixture.placeholder(statement["id"])
	if status != http.StatusCreated {
		t.Fatalf("placeholder = %d %v", status, body)
	}
	placeholderID, _ := body["transaction_id"].(string)
	before := fixture.counts()

	// Booking real details inside a cycle the placeholder already reserves would
	// count the same bill twice, and silently reversing the placeholder is the
	// user's call. So the import refuses, names the placeholder, and writes nothing:
	// the UI offers the existing 冲正 and the household re-uploads afterwards.
	status, body = fixture.importDetails(map[string]any{
		"rows":      spdbMatrix(spdbRow("20261020", "20261021", "某商户", "4321", "人民币", "50.00")),
		"card_tail": "4321", "statement_id": statement["id"]})
	if status != http.StatusConflict || placeholderError(body) != "placeholder_not_reversed" {
		t.Fatalf("import over a live placeholder = %d %v, want 409 placeholder_not_reversed", status, body)
	}
	placeholder, _ := body["placeholder"].(map[string]any)
	if placeholder == nil {
		t.Fatalf("import hid the placeholder: %v", body)
	}
	if placeholder["transaction_id"] != placeholderID || number(placeholder["amount_minor"]) != 120000 {
		t.Fatalf("placeholder echo = %v, want %s / 120000", placeholder, placeholderID)
	}
	if after := fixture.counts(); after != before || fixture.detailCount() != 0 {
		t.Fatalf("refused import wrote rows: %+v -> %+v, details=%d", before, after, fixture.detailCount())
	}
	if row := fixture.client.mustOK("GET", "/api/v1/transactions/"+placeholderID, nil); row["status"] != "posted" {
		t.Fatalf("placeholder was auto-reversed by the import: %v", row)
	}

	// Once the household reverses the placeholder themselves, the same upload books.
	fixture.client.mustOK("DELETE", "/api/v1/transactions/"+placeholderID, map[string]any{})
	if status, body = fixture.importDetails(map[string]any{
		"rows":      spdbMatrix(spdbRow("20261020", "20261021", "某商户", "4321", "人民币", "50.00")),
		"card_tail": "4321", "statement_id": statement["id"]}); status != http.StatusOK || number(body["created"]) != 1 {
		t.Fatalf("import after reversal = %d %v, want the line booked", status, body)
	}
	if got := fixture.transactionsFor(5000); got != 1 {
		t.Fatalf("line posted %d times after the placeholder was reversed, want 1", got)
	}
}

func TestStatementDetailPermissionsAndScoping(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	for _, statement := range []string{
		`INSERT INTO households(id,name) VALUES('hh_other_sd','其他家庭')`,
		`INSERT INTO accounts(id,household_id,kind,name,ledger_code)
			VALUES('acc_other_sd','hh_other_sd','credit','他卡','2010:acc_other_sd')`,
		`INSERT INTO card_statements(id,household_id,account_id,period_end_on,due_on,amount_due_minor,created_at,updated_at)
			VALUES('stmt_other_sd','hh_other_sd','acc_other_sd','2026-11-01','2026-11-25',500000,
			      '2026-11-25T00:00:00Z','2026-11-25T00:00:00Z')`,
	} {
		if _, err := fixture.database.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	rows := map[string]any{"rows": sampleMatrix(), "card_tail": "4321"}

	// Another household's bill id must read as "not found", never as a foreign row.
	if status, _ := fixture.importDetails(map[string]any{"rows": sampleMatrix(), "card_tail": "4321",
		"statement_id": "stmt_other_sd"}); status != http.StatusNotFound {
		t.Fatalf("foreign statement = %d, want 404", status)
	}
	if status, _ := fixture.importDetails(map[string]any{"rows": sampleMatrix(), "card_tail": "4321",
		"statement_id": "stmt_missing_sd"}); status != http.StatusNotFound {
		t.Fatalf("unknown statement = %d, want 404", status)
	}
	cash := fixture.client.mustOK("POST", "/api/v1/accounts", map[string]any{"kind": "cash", "name": "现金"})
	if status, _ := fixture.detail("/api/v1/accounts/"+cash["id"].(string)+"/statement-detail-preview", rows); status != http.StatusNotFound {
		t.Fatalf("cash account = %d, want 404", status)
	}
	for name, body := range map[string]map[string]any{
		"empty":   {"rows": [][]string{}, "card_tail": "4321"},
		"missing": {"card_tail": "4321"},
		"tail":    {"rows": sampleMatrix()},
	} {
		if status, _ := fixture.detail("/api/v1/accounts/"+fixture.cardID+"/statement-detail-preview", body); status != http.StatusBadRequest {
			t.Fatalf("%s rows = %d, want 400", name, status)
		}
	}
	// The card is disabled last: an inactive account must not accept an upload at all.
	fixture.client.mustOK("PATCH", "/api/v1/accounts/"+fixture.cardID, map[string]string{"status": "disabled"})
	if status, _ := fixture.preview(rows); status != http.StatusNotFound {
		t.Fatalf("disabled card = %d, want 404", status)
	}
}

func TestStatementDetailImportWritesAllOrNothing(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	// A bill from another card of the SAME household is the case the guard exists
	// for: the household check alone would let these lines attach to the wrong card.
	other := fixture.client.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "credit", "name": "第二张卡(1111)", "statement_day": 5, "due_day": 25,
	})
	if _, err := fixture.database.Exec(`INSERT INTO card_statements
		(id,household_id,account_id,period_start_on,period_end_on,due_on,amount_due_minor,created_at,updated_at)
		VALUES('stmt_second',?,?,?,'2026-11-05','2026-11-25',1000,'2026-11-05T00:00:00Z','2026-11-05T00:00:00Z')`,
		fixture.householdID(), other["id"], "2026-10-06"); err != nil {
		t.Fatalf("second card statement: %v", err)
	}
	before := fixture.counts()
	beforeDetails := fixture.detailCount()

	// A refused batch must leave nothing behind: no detail row, no transaction, no
	// half-imported bill.
	status, body := fixture.importDetails(map[string]any{"rows": sampleMatrix(), "card_tail": "4321",
		"statement_id": "stmt_second"})
	if status == http.StatusOK {
		t.Fatal("import accepted a statement belonging to another card")
	}
	if status != http.StatusNotFound {
		t.Fatalf("foreign bill = %d %v, want 404", status, body)
	}
	if after := fixture.counts(); after != before || fixture.detailCount() != beforeDetails {
		t.Fatalf("refused import wrote rows: %+v -> %+v, details %d -> %d",
			before, after, beforeDetails, fixture.detailCount())
	}

	// The accepted batch then proves the all-or-nothing shape: every stored line is
	// accounted for, and every booked line has its full ledger trail.
	if status, body = fixture.importDetails(map[string]any{"rows": sampleMatrix(), "card_tail": "4321",
		"statement_id": statement["id"]}); status != http.StatusOK {
		t.Fatalf("import = %d %v", status, body)
	}
	var booked, orphan int
	if err := fixture.database.QueryRow(`SELECT
		SUM(CASE WHEN transaction_id IS NOT NULL THEN 1 ELSE 0 END),
		SUM(CASE WHEN transaction_id IS NOT NULL
		          AND NOT EXISTS (SELECT 1 FROM transactions WHERE transactions.id=statement_details.transaction_id)
		     THEN 1 ELSE 0 END)
		FROM statement_details`).Scan(&booked, &orphan); err != nil {
		t.Fatal(err)
	}
	if orphan != 0 {
		t.Fatalf("%d detail rows point at missing transactions", orphan)
	}
	if booked == 0 || booked > 2 {
		t.Fatalf("booked detail rows = %d, want at most the two charges", booked)
	}
}

func TestStatementDetailRowsAreHouseholdConsistent(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	if _, err := fixture.database.Exec(`INSERT INTO households(id,name) VALUES('hh_fk_sd','另一家庭')`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.database.Exec(`INSERT INTO statement_details
		(id,household_id,account_id,row_key,row_no,seq,card_tail,kind,state,txn_on,description,
		 amount_minor,currency,review_reason,source_digest,created_at)
		VALUES('sd_fk','hh_fk_sd','acc_fk','rowkey_fk',1,0,'4321','charge','held','2026-10-20','x',1,'CNY','','','2026-11-26T00:00:00Z')`); err == nil {
		t.Fatal("a detail row for a household that does not own the card was accepted")
	}
}

// The uploaded file never decides where a charge lands: 账单补录 is chosen by the
// server, so a client-supplied category_id — an income category, a deleted one, or
// a made-up id — must not steer the booking or make the import fail. Realignment
// happens afterwards through the normal transaction PATCH.
func TestStatementDetailImportIgnoresClientCategory(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	income := fixture.client.mustOK("POST", "/api/v1/categories",
		map[string]any{"kind": "income", "name": "补录不该用的收入"})
	deleted := fixture.client.mustOK("POST", "/api/v1/categories",
		map[string]any{"kind": "expense", "name": "补录不该用的支出"})
	fixture.client.mustOK("DELETE", "/api/v1/categories/"+deleted["id"].(string), map[string]any{})

	for index, category := range []string{income["id"].(string), deleted["id"].(string), "cat_absent_sd"} {
		merchant := fmt.Sprintf("某书店%d", index) // a distinct row_key each round
		status, body := fixture.importDetails(map[string]any{
			"rows":      spdbMatrix(spdbRow("20261110", "20261111", merchant, "4321", "人民币", "39.90")),
			"card_tail": "4321", "category_id": category})
		if status != http.StatusOK || number(body["created"]) != 1 {
			t.Fatalf("import with category_id=%s = %d %v, want the line booked", category, status, body)
		}
		created, _ := body["transactions"].([]any)
		transactionID, _ := created[0].(string)
		row := fixture.client.mustOK("GET", "/api/v1/transactions/"+transactionID, nil)
		if name := fmt.Sprint(row["category_name"]); name != "账单补录" {
			t.Fatalf("category_id=%s booked under %q, want 账单补录", category, name)
		}
	}
}

func TestStatementSummaryReplacesCoveredCardExpense(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	fixture.expense(7000, "2026-10-21")
	fixture.expense(3000, "2026-10-01")
	input := map[string]any{"rows": spdbMatrix(spdbRow("20261020", "20261021", "某商户", "4321", "人民币", "50.00")),
		"card_tail": "4321", "statement_id": statement["id"].(string)}
	status, preview := fixture.preview(input)
	unmatched, _ := preview["unmatched_ledger"].([]any)
	if status != http.StatusOK || len(unmatched) != 1 {
		t.Fatalf("preview should flag the unmatched in-period ledger charge: %d %v", status, preview)
	}
	if status, body := fixture.importDetails(input); status != http.StatusOK {
		t.Fatalf("import: %d %v", status, body)
	}
	status, summary := fixture.client.do("GET", "/api/v1/stats/summary?from=2026-10-01&to=2026-10-31", nil)
	if status != http.StatusOK || number(summary["expense_minor"]) != 8000 || number(summary["booked_expense_minor"]) != 15000 {
		t.Fatalf("want 5000 bank expense + 3000 uncovered expense; booked total 15000: %d %v", status, summary)
	}
}
