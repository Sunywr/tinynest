package api

import (
	"testing"
	"time"
)

// V0.1.3 preflight: the handoff plan (M2) requires a dividend history with an
// annual summary. Written BEFORE the endpoint exists, so a 404/405 is a real
// failing test rather than an assumption in a report.

func TestV013_DividendHistoryAndAnnualSummary(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := v012BrokerAndAsset(t, c)

	post := func(on string, gross int64, tax *int64) map[string]any {
		body := map[string]any{
			"asset_id": asset, "received_on": on, "gross_minor": gross,
			"to_account_id": broker,
		}
		if tax != nil {
			body["tax_minor"] = *tax
		}
		return c.mustOK("POST", "/api/v1/investments/dividends", body)
	}
	tax1, tax3 := int64(1_000), int64(2_000)
	known := post("2026-11-11", 10_000, &tax1)
	if known["tax_unknown"] != false {
		t.Fatalf("known tax must not be flagged unknown: %v", known)
	}
	unknown := post("2026-12-05", 5_000, nil)
	if unknown["tax_unknown"] != true {
		t.Fatalf("missing tax must stay unknown, never zero: %v", unknown)
	}
	post("2027-01-10", 20_000, &tax3)

	list := c.mustOK("GET", "/api/v1/investments/dividends", nil)
	rows := v012Rows(t, list)
	if len(rows) != 3 {
		t.Fatalf("dividend history = %d rows, want 3: %v", len(rows), list)
	}
	// Newest first, and every row must be drill-downable to its journal entry.
	if rows[0]["received_on"] != "2027-01-10" || rows[2]["received_on"] != "2026-11-11" {
		t.Errorf("dividend history not ordered by received_on DESC: %v", rows)
	}
	for _, row := range rows {
		for _, key := range []string{
			"id", "asset_id", "code", "name", "received_on", "gross_minor",
			"net_minor", "tax_unknown", "to_account_id", "entry_id",
		} {
			if _, ok := row[key]; !ok {
				t.Errorf("dividend row missing %q: %v", key, row)
			}
		}
		if row["asset_id"] != asset {
			t.Errorf("dividend row leaks another asset: %v", row)
		}
	}
	unknownRow := rows[1]
	if unknownRow["tax_unknown"] != true || unknownRow["tax_minor"] != nil {
		t.Errorf("unknown tax must be nullable and flagged: %v", unknownRow)
	}
	if got := number(unknownRow["net_minor"]); got != 5_000 {
		t.Errorf("unknown-tax dividend net = %d, want 5000", got)
	}

	summary, ok := list["summary"].(map[string]any)
	if !ok {
		t.Fatalf("annual summary missing: %v", list)
	}
	if got := number(summary["total_gross_minor"]); got != 35_000 {
		t.Errorf("total gross = %d, want 35000", got)
	}
	if got := number(summary["total_tax_minor"]); got != 3_000 {
		t.Errorf("known tax total = %d, want 3000", got)
	}
	if got := number(summary["total_net_minor"]); got != 32_000 {
		t.Errorf("total net = %d, want 32000", got)
	}
	if got := number(summary["unknown_tax_count"]); got != 1 {
		t.Errorf("unknown tax count = %d, want 1", got)
	}
	if got := number(summary["unknown_gross_minor"]); got != 5_000 {
		t.Errorf("unknown-tax gross = %d, want 5000", got)
	}
	years, ok := summary["by_year"].([]any)
	if !ok || len(years) != 2 {
		t.Fatalf("by_year missing or wrong length: %v", summary["by_year"])
	}
	found := false
	for _, y := range years {
		m, _ := y.(map[string]any)
		if m["year"] == "2026" {
			found = true
			if number(m["gross_minor"]) != 15_000 || number(m["tax_minor"]) != 1_000 ||
				number(m["net_minor"]) != 14_000 || number(m["count"]) != 2 ||
				number(m["unknown_tax_count"]) != 1 {
				t.Errorf("2026 annual summary wrong: %v", m)
			}
		}
	}
	if !found {
		t.Errorf("2026 bucket missing: %v", years)
	}

	// Filters must be honoured and must not break the summary.
	filtered := c.mustOK("GET", "/api/v1/investments/dividends?from=2026-12-01&to=2026-12-31", nil)
	if got := len(v012Rows(t, filtered)); got != 1 {
		t.Errorf("date filter returned %d rows, want 1", got)
	}
	if got := number(filtered["summary"].(map[string]any)["total_net_minor"]); got != 5_000 {
		t.Errorf("filtered summary total net = %d, want 5000", got)
	}
	byAsset := c.mustOK("GET", "/api/v1/investments/dividends?asset_id="+asset, nil)
	if got := len(v012Rows(t, byAsset)); got != 3 {
		t.Errorf("asset filter returned %d rows, want 3", got)
	}
	foreign := c.mustOK("GET", "/api/v1/investments/dividends?asset_id=ast_foreign", nil)
	if got := len(v012Rows(t, foreign)); got != 0 {
		t.Errorf("foreign asset id returned %d rows, want 0", got)
	}
}

func TestV013_DividendHistoryIsHouseholdScoped(t *testing.T) {
	now := time.Date(2026, 11, 14, 4, 0, 0, 0, time.UTC)
	e := newRemEnv(t, Options{Clock: func() time.Time { return now }, SetupToken: testSetupToken})
	defer e.ts.Close()
	defer e.database.Close()
	ha := bootstrapHousehold(t, e, "v13a")
	hb := bootstrapHousehold(t, e, "v13b")
	ca := ha.client(t, e.ts.URL)
	cb := hb.client(t, e.ts.URL)
	call := func(c *remClient, method, path string, body any) remResult {
		return c.call(method, "/api/v1"+path, body, nil)
	}

	broker := call(ca, "POST", "/accounts", map[string]any{
		"kind": "securities", "name": "A 券商", "opening_balance_minor": 1_000_000,
	}).Body["id"].(string)
	asset := call(ca, "POST", "/investments/assets", map[string]any{
		"code": "A1", "name": "A 股票", "kind": "stock", "account_id": broker,
	}).Body["id"].(string)
	created := call(ca, "POST", "/investments/dividends", map[string]any{
		"asset_id": asset, "received_on": "2026-11-14", "gross_minor": 10_000,
		"tax_minor": 1_000, "to_account_id": broker,
	})
	if created.Status != 201 {
		t.Fatalf("A create dividend: %d %v", created.Status, created.Body)
	}

	own := call(ca, "GET", "/investments/dividends", nil)
	if own.Status != 200 || len(v012Rows(t, own.Body)) != 1 {
		t.Fatalf("A must see its own dividend: %d %v", own.Status, own.Body)
	}
	for _, q := range []string{
		"/investments/dividends",
		"/investments/dividends?asset_id=" + asset,
	} {
		other := call(cb, "GET", q, nil)
		if other.Status != 200 {
			t.Fatalf("B list %s: %d %v", q, other.Status, other.Body)
		}
		if got := len(v012Rows(t, other.Body)); got != 0 {
			t.Errorf("B saw %d foreign dividends via %s", got, q)
		}
		if got := number(other.Body["summary"].(map[string]any)["total_net_minor"]); got != 0 {
			t.Errorf("B summary includes A's dividend: %d", got)
		}
	}
}
