package api

import (
	"testing"
	"time"
)

// Household isolation regressions (originally v011_isolation_test.go).
// Every financial query must scope household_id in SQL, never post-filter in Go.

// insertStatsJournal writes a balanced income + expense entry for one household
// using the same category/account subject codes as every other household, so a
// missing household_id predicate immediately leaks the other household's numbers.
func insertStatsJournal(t *testing.T, e *remEnv, h isoHousehold, scale int64) {
	t.Helper()
	entryID := "je_iso_" + h.id
	if _, err := e.database.Exec(`
		INSERT INTO journal_entries(id,household_id,biz_type,biz_id,occurred_on,currency,status,created_by)
		VALUES(?,?,?,?,?,'CNY','posted','isolation-test')`,
		entryID, h.id, "manual", "biz_iso_"+h.id, "2026-11-12"); err != nil {
		t.Fatal(err)
	}
	type line struct {
		code   string
		dr, cr int64
	}
	lines := []line{
		{"1010:cash", 500000 * scale, 0},
		{"4010:cat_salary", 0, 500000 * scale},
		{"5010:cat_food", 30000 * scale, 0},
		{"1010:cash", 0, 30000 * scale},
	}
	for i, l := range lines {
		if _, err := e.database.Exec(
			`INSERT INTO journal_lines(entry_id,line_no,account_code,debit_minor,credit_minor) VALUES(?,?,?,?,?)`,
			entryID, i+1, l.code, l.dr, l.cr); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHouseholdIsolation(t *testing.T) {
	fixed := time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)
	e := newRemEnv(t, Options{
		Clock:      func() time.Time { return fixed },
		SetupToken: testSetupToken,
	})
	a := bootstrapHousehold(t, e, "a")
	b := bootstrapHousehold(t, e, "b")
	insertStatsJournal(t, e, a, 1)
	insertStatsJournal(t, e, b, 2)
	ca, cb := a.client(t, e.ts.URL), b.client(t, e.ts.URL)

	path := "/api/v1/stats/summary?from=2026-01-01&to=2026-12-31"
	ra := ca.call("GET", path, nil, nil)
	rb := cb.call("GET", path, nil, nil)
	if ra.Status != 200 || rb.Status != 200 {
		t.Fatalf("[FIX-02] stats summary failed: A=%d %v B=%d %v", ra.Status, ra.Body, rb.Status, rb.Body)
	}
	type pair struct{ a, b int64 }
	want := map[string]pair{
		"income_minor":  {500000, 1000000},
		"expense_minor": {30000, 60000},
		"balance_minor": {470000, 940000},
	}
	for field, w := range want {
		if got := number(ra.Body[field]); got != w.a {
			t.Errorf("[FIX-02] household A %s = %d, want %d (cross-household leak?)", field, got, w.a)
		}
		if got := number(rb.Body[field]); got != w.b {
			t.Errorf("[FIX-02] household B %s = %d, want %d (cross-household leak?)", field, got, w.b)
		}
	}

	// Category ranking must also stay inside the household.
	catA := c0(ca.call("GET", "/api/v1/stats/categories?from=2026-01-01&to=2026-12-31", nil, nil).Body)
	catB := c0(cb.call("GET", "/api/v1/stats/categories?from=2026-01-01&to=2026-12-31", nil, nil).Body)
	if len(catA) != 1 || len(catB) != 1 {
		t.Fatalf("[FIX-02] each household must rank exactly its own category, got A=%d B=%d", len(catA), len(catB))
	}
	if number(catA[0]) == 0 || number(catB[0]) == 0 || number(catA[0]) == number(catB[0]) {
		t.Fatalf("[FIX-02] category amounts look mixed up: A=%v B=%v", catA, catB)
	}
}

// c0 flattens items and keeps the amount_minor numbers for quick comparisons.
func c0(body map[string]any) []any {
	items, _ := body["items"].([]any)
	out := make([]any, 0, len(items))
	for _, item := range items {
		row, _ := item.(map[string]any)
		out = append(out, row["amount_minor"])
	}
	return out
}

func TestHouseholdIsolationOnDetailEndpoints(t *testing.T) {
	fixed := time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)
	e := newRemEnv(t, Options{Clock: func() time.Time { return fixed }, SetupToken: testSetupToken})
	a := bootstrapHousehold(t, e, "a")
	b := bootstrapHousehold(t, e, "b")
	accID := "acc_iso_a"
	if _, err := e.database.Exec(
		`INSERT INTO accounts(id,household_id,kind,name,ledger_code) VALUES(?,?,?,?,?)`,
		accID, a.id, "cash", "隔离账户", "1010:"+accID); err != nil {
		t.Fatal(err)
	}
	cb := b.client(t, e.ts.URL)
	list := cb.call("GET", "/api/v1/accounts", nil, nil)
	items, _ := list.Body["items"].([]any)
	if len(items) != 0 {
		t.Errorf("[FIX-02] household B must not see household A's accounts, got %d", len(items))
	}
	detail := cb.call("GET", "/api/v1/accounts/"+accID, nil, nil)
	if detail.Status != 404 {
		t.Errorf("[FIX-02] household B must not read A's account, got %d", detail.Status)
	}
	txList := cb.call("GET", "/api/v1/transactions?limit=10", nil, nil)
	txItems, _ := txList.Body["items"].([]any)
	if len(txItems) != 0 {
		t.Errorf("[FIX-02] household B must not see household A's flows, got %d", len(txItems))
	}
}
