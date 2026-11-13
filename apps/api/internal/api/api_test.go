package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tinynest/api/internal/db"
	"tinynest/api/internal/server"
)

type testClient struct {
	t       *testing.T
	base    string
	hc      *http.Client
	csrf    string
	headers map[string]string
}

func newTestServer(t *testing.T) (*httptest.Server, *testClient, *db.DB) {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	applied, err := database.Migrate(context.Background())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	expected, err := db.EmbeddedVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(expected) {
		t.Fatalf("expected %d migrations, got %v", len(expected), applied)
	}
	a := New(database, Options{SetupToken: testSetupToken})
	mux := server.NewMux(dir, server.Options{Register: a.Register})
	ts := httptest.NewServer(mux)
	jar, _ := cookiejar.New(nil)
	// The one-time bootstrap token is used ONLY for the initial setup call.
	tc := &testClient{t: t, base: ts.URL, hc: &http.Client{Jar: jar},
		headers: map[string]string{setupTokenHeader: testSetupToken}}

	code, body := tc.do("POST", "/api/v1/setup", map[string]any{
		"household_name": "测试小家", "username": "admin", "password": "correct-horse-battery",
	})
	if code != 201 {
		t.Fatalf("setup: %d %v", code, body)
	}
	tc.headers = nil
	code, body = tc.do("POST", "/api/v1/auth/session", map[string]any{
		"username": "admin", "password": "correct-horse-battery",
	})
	if code != 200 {
		t.Fatalf("login: %d %v", code, body)
	}
	tc.csrf, _ = body["csrf_token"].(string)
	if tc.csrf == "" {
		t.Fatal("missing csrf token")
	}
	return ts, tc, database
}

func (c *testClient) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func number(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func (c *testClient) mustOK(method, path string, body any) map[string]any {
	c.t.Helper()
	code, out := c.do(method, path, body)
	if code < 200 || code >= 300 {
		c.t.Fatalf("%s %s => %d %v", method, path, code, out)
	}
	return out
}

func (c *testClient) firstCategory(kind string) string {
	c.t.Helper()
	_, out := c.do("GET", "/api/v1/categories", nil)
	items, _ := out["items"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m["kind"] == kind {
			return m["id"].(string)
		}
	}
	c.t.Fatalf("no %s category found", kind)
	return ""
}

// Full S1->S2->S3->S4->S5->S6 smoke over real HTTP + real SQLite.
func TestV01EndToEnd(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	// --- S1: accounts with opening balance ---
	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "工资卡", "opening_balance_minor": 1_000_000,
	})["id"].(string)
	cash2 := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "家庭共同账户", "opening_balance_minor": 0,
	})["id"].(string)
	bal := c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)
	if number(bal["balance_minor"]) != 1_000_000 {
		t.Fatalf("opening balance = %v, want 1000000", bal["balance_minor"])
	}

	expCat, incCat := c.firstCategory("expense"), c.firstCategory("income")

	// --- S2: expense / income / transfer ---
	tx1 := c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 10_000, "occurred_on": "2026-11-12",
		"payment_kind": "cash", "account_id": cash,
		"splits": []any{map[string]any{"category_id": expCat, "amount_minor": 10_000}},
	})["id"].(string)
	bal = c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)
	if number(bal["balance_minor"]) != 990_000 {
		t.Errorf("balance after expense = %v, want 990000", bal["balance_minor"])
	}

	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "income", "amount_minor": 800_000, "occurred_on": "2026-11-12",
		"account_id": cash,
		"splits":     []any{map[string]any{"category_id": incCat, "amount_minor": 800_000}},
	})
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "transfer", "amount_minor": 100_000, "occurred_on": "2026-11-12",
		"account_id": cash, "to_account_id": cash2,
	})
	bal = c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)
	if number(bal["balance_minor"]) != 1_690_000 {
		t.Errorf("balance after income+transfer = %v, want 1690000", bal["balance_minor"])
	}

	// Split sum mismatch must roll back BOTH the transaction and its journal lines.
	code, _ := c.do("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 50_000, "occurred_on": "2026-11-12",
		"payment_kind": "cash", "account_id": cash,
		"splits": []any{map[string]any{"category_id": expCat, "amount_minor": 30_000}},
	})
	if code == 201 || code == 200 {
		t.Fatal("split mismatch must be rejected")
	}
	bal = c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)
	if number(bal["balance_minor"]) != 1_690_000 {
		t.Error("failed transaction must not change any balance (transaction rollback)")
	}
	var entries, lines int
	_ = database.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&entries)
	_ = database.QueryRow(`SELECT COUNT(*) FROM journal_lines`).Scan(&lines)
	if entries != 4 || lines < 8 {
		t.Errorf("rollback leaked rows: entries=%d lines=%d", entries, lines)
	}

	// --- S3: credit instrument, credit expense, statement, repayment ---
	ci := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"kind": "credit_card", "name": "测试信用卡",
		"cycle_start_day": 1, "cycle_end_day": 31, "due_day": 10, "due_month_offset": 1,
	})["id"].(string)
	creditTx := c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100_000, "occurred_on": "2026-11-12",
		"payment_kind": "credit", "credit_instrument_id": ci,
		"splits": []any{map[string]any{"category_id": expCat, "amount_minor": 100_000}},
	})["id"].(string)
	bal = c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)
	if number(bal["balance_minor"]) != 1_690_000 {
		t.Error("credit expense must NOT reduce cash")
	}
	_, stmts := c.do("GET", "/api/v1/credit-statements", nil)
	items, _ := stmts["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 auto-collected statement, got %d", len(items))
	}
	stmt := items[0].(map[string]any)
	if number(stmt["system_due_minor"]) != 100_000 {
		t.Errorf("statement due = %v, want 100000", stmt["system_due_minor"])
	}
	stmtID := stmt["id"].(string)
	c.mustOK("POST", "/api/v1/credit-statements/"+stmtID+"/payments", map[string]any{
		"amount_minor": 60_000, "paid_on": "2026-11-12", "from_account_id": cash,
	})
	bal = c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)
	if number(bal["balance_minor"]) != 1_630_000 {
		t.Errorf("cash after repayment = %v, want 1630000", bal["balance_minor"])
	}

	// --- S4: partial refund back to the credit instrument ---
	c.mustOK("POST", "/api/v1/refunds", map[string]any{
		"transaction_id": creditTx, "amount_minor": 30_000, "refund_on": "2026-11-13",
		"destination_type": "credit", "destination_id": ci,
	})
	ref := c.mustOK("GET", "/api/v1/refunds/by-transaction/"+creditTx, nil)
	if number(ref["net_expense_minor"]) != 70_000 {
		t.Errorf("net expense after refund = %v, want 70000", ref["net_expense_minor"])
	}
	code, _ = c.do("POST", "/api/v1/refunds", map[string]any{
		"transaction_id": creditTx, "amount_minor": 80_000, "refund_on": "2026-11-14",
		"destination_type": "credit", "destination_id": ci,
	})
	if code == 201 {
		t.Error("over-refund must be rejected")
	}

	// --- S4: installment + idempotent due-item generation ---
	inst := c.mustOK("POST", "/api/v1/installments", map[string]any{
		"name": "装修分期", "category_id": expCat,
		"principal_minor": 300_000, "total_fee_minor": 9_000,
		"periods": 3, "first_due_on": "2026-12-01",
	})["id"].(string)
	gen1 := c.mustOK("POST", "/api/v1/schedule/generate", map[string]any{})
	gen2 := c.mustOK("POST", "/api/v1/schedule/generate", map[string]any{})
	// The daily task only projects obligations due within 30 days, so with monthly
	// periods just the first one is created; a second run must create nothing at all.
	if number(gen1["generated"]) != 1 || number(gen2["generated"]) != 0 {
		t.Errorf("due-item generation not idempotent: first=%v second=%v", gen1["generated"], gen2["generated"])
	}
	c.mustOK("POST", "/api/v1/installments/"+inst+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 103_000, "paid_on": "2026-12-01",
		"channel": "cash", "channel_id": cash,
	})

	// --- S5: investment buy / sell / dividend / valuation ---
	sec := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "securities", "name": "证券账户", "opening_balance_minor": 500_000,
	})["id"].(string)
	asset := c.mustOK("POST", "/api/v1/investments/assets", map[string]any{
		"code": "TEST001", "name": "测试股票", "kind": "stock", "account_id": sec,
	})["id"].(string)
	c.mustOK("POST", "/api/v1/investments/trades", map[string]any{
		"asset_id": asset, "side": "buy", "trade_on": "2026-11-12",
		"quantity_scaled": 100_000_000, "gross_minor": 100_000, "fee_minor": 500,
		"from_account_id": sec,
	})
	c.mustOK("POST", "/api/v1/investments/trades", map[string]any{
		"asset_id": asset, "side": "buy", "trade_on": "2026-11-13",
		"quantity_scaled": 100_000_000, "gross_minor": 120_000, "fee_minor": 500,
		"from_account_id": sec,
	})
	_, posOut := c.do("GET", "/api/v1/investments/positions", nil)
	positions, _ := posOut["items"].([]any)
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	pos := positions[0].(map[string]any)
	if number(pos["total_cost_minor"]) != 221_000 {
		t.Errorf("moving-average cost = %v, want 221000", pos["total_cost_minor"])
	}
	if number(pos["avg_cost_minor"]) != 1_105 {
		t.Errorf("avg cost = %v, want 1105 (11.05 CNY)", pos["avg_cost_minor"])
	}
	c.mustOK("POST", "/api/v1/investments/trades", map[string]any{
		"asset_id": asset, "side": "sell", "trade_on": "2026-11-14",
		"quantity_scaled": 50_000_000, "gross_minor": 65_000, "fee_minor": 500,
		"from_account_id": sec,
	})
	c.mustOK("POST", "/api/v1/investments/dividends", map[string]any{
		"asset_id": asset, "received_on": "2026-11-15",
		"gross_minor": 10_000, "tax_minor": 1_000, "to_account_id": sec,
	})
	c.mustOK("POST", "/api/v1/investments/valuations", map[string]any{
		"asset_id": asset, "valued_on": "2026-11-15", "market_value_minor": 180_000,
	})
	code, posOut = c.do("GET", "/api/v1/investments/positions", nil)
	positions, _ = posOut["items"].([]any)
	if code != 200 || len(positions) != 1 {
		t.Fatalf("positions: %d %v", code, posOut)
	}
	pos = positions[0].(map[string]any)
	if number(pos["realized_pnl_minor"]) != 9_250 {
		t.Errorf("realised P&L = %v, want 9250", pos["realized_pnl_minor"])
	}
	if number(pos["net_dividend_minor"]) != 9_000 {
		t.Errorf("net dividend = %v, want 9000", pos["net_dividend_minor"])
	}

	// --- S6: dashboard exposes three separate accounting views ---
	_, summary := c.do("GET", "/api/v1/dashboard/summary?from=2026-11-01&to=2026-11-30", nil)
	for _, field := range []string{"income_minor", "expense_minor", "cashflow_minor", "liabilities_due_minor", "net_worth_minor", "investment_total_return_minor"} {
		if _, ok := summary[field]; !ok {
			t.Errorf("dashboard missing %s", field)
		}
	}
	if number(summary["income_minor"]) != 800_000 {
		t.Errorf("period income = %v, want 800000", summary["income_minor"])
	}
	// cashflow differs from expense: credit consumption does not move cash.
	if number(summary["cashflow_minor"]) == number(summary["expense_minor"])*-1 {
		t.Error("cashflow must be a distinct view from expense")
	}

	// --- S2: reversal restores the balance and keeps both entries ---
	before := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"])
	c.mustOK("POST", "/api/v1/transactions/"+tx1+"/reverse", map[string]any{})
	after := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"])
	if after != before+10_000 {
		t.Errorf("reversal did not restore balance: before=%d after=%d", before, after)
	}
	var reversedCount int
	_ = database.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE status='reversed'`).Scan(&reversedCount)
	if reversedCount != 1 {
		t.Errorf("original entry should be preserved as reversed, got %d", reversedCount)
	}
}

// TestSetupIsSingleUseAndSessionAuthEnforced covers the post-FIX-01 contract:
// the endpoint is single-use AND bootstrap-protected; ordinary reads still need
// a session and writes still need CSRF. (The old name claimed an auth assertion
// it never made.)
func TestSetupIsSingleUseAndSessionAuthEnforced(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	// A second, correctly tokened bootstrap must be rejected (no second admin).
	c.headers = map[string]string{setupTokenHeader: testSetupToken}
	code, _ := c.do("POST", "/api/v1/setup", map[string]any{
		"household_name": "另一个家", "username": "admin2", "password": "another-password",
	})
	if code != 409 {
		t.Errorf("duplicate setup should be 409, got %d", code)
	}
	c.headers = nil
	var admins int
	_ = database.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&admins)
	if admins != 1 {
		t.Errorf("duplicate setup must not create a second admin, got %d", admins)
	}

	// P1-01 regression: a tokenless bootstrap attempt is refused outright.
	noToken := &testClient{t: t, base: c.base, hc: &http.Client{}}
	code, _ = noToken.do("POST", "/api/v1/setup", map[string]any{
		"household_name": "抢注家庭", "username": "attacker", "password": "another-password",
	})
	if code != 403 && code != 401 {
		t.Errorf("tokenless setup should be rejected, got %d", code)
	}

	// A fresh client without a session cookie cannot read financial data.
	fresh := &testClient{t: t, base: c.base, hc: &http.Client{}}
	code, _ = fresh.do("GET", "/api/v1/accounts", nil)
	if code != 401 {
		t.Errorf("unauthenticated read should be 401, got %d", code)
	}
	// CSRF is enforced on writes even with a valid session.
	noCSRF := &testClient{t: t, base: c.base, hc: c.hc, csrf: "wrong-token"}
	code, _ = noCSRF.do("POST", "/api/v1/projects", map[string]any{"name": "x"})
	if code != 403 {
		t.Errorf("bad CSRF should be 403, got %d", code)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	ts, _, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	applied, err := database.Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Errorf("re-running migrations should be a no-op, got %v", applied)
	}
	var tables int
	if err := database.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables < 25 {
		t.Errorf("expected full V0.1 schema, got %d tables", tables)
	}
}
