package api

import (
	"testing"
	"time"
)

func fixedBusinessClock() func() time.Time {
	fixed := time.Date(2026, 11, 22, 4, 0, 0, 0, time.UTC)
	return func() time.Time { return fixed }
}

func getCreditDebtPlan(t *testing.T, c *testClient) map[string]any {
	t.Helper()
	body := c.mustOK("GET", "/api/v1/accounts?include=credit_debt", nil)
	plan, ok := body["credit_debt"].(map[string]any)
	if !ok {
		t.Fatalf("missing credit_debt projection: %v", body)
	}
	return plan
}

func findCreditCard(t *testing.T, plan map[string]any, accountID string) map[string]any {
	t.Helper()
	raw, _ := plan["cards"].([]any)
	for _, item := range raw {
		row, _ := item.(map[string]any)
		if row["account_id"] == accountID {
			return row
		}
	}
	t.Fatalf("card %s not found in %v", accountID, plan["cards"])
	return nil
}

func mustSetPayday(t *testing.T, c *testClient, payday int) {
	t.Helper()
	c.mustOK("PATCH", "/api/v1/auth/me/profile", map[string]any{"payday": payday})
}

func TestCreditCycleForBoundaries(t *testing.T) {
	type want struct {
		start, end, due string
	}
	cases := []struct {
		name string
		on   string
		s, d int
		want want
	}{
		{"close day belongs to current statement", "2026-11-05", 5, 23, want{"2026-10-06", "2026-11-05", "2026-11-23"}},
		{"day after close opens next statement", "2026-11-06", 5, 23, want{"2026-11-06", "2026-12-05", "2026-12-23"}},
		{"february clamps statement day 31", "2026-02-28", 31, 20, want{"2026-02-01", "2026-02-28", "2026-03-20"}},
		{"march follows february close", "2026-03-01", 31, 20, want{"2026-03-01", "2026-03-31", "2026-04-20"}},
		{"statement day 30 clamps in february", "2026-02-28", 30, 30, want{"2026-01-31", "2026-02-28", "2026-03-30"}},
		{"leap day statement close", "2028-02-29", 31, 15, want{"2028-02-01", "2028-02-29", "2028-03-15"}},
		{"same due day rolls to next month", "2026-11-05", 5, 5, want{"2026-10-06", "2026-11-05", "2026-12-05"}},
		{"year boundary", "2026-12-20", 25, 13, want{"2026-11-26", "2026-12-25", "2027-01-13"}},
		{"statement day one spans year", "2027-01-01", 1, 20, want{"2026-12-02", "2027-01-01", "2027-01-20"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			on, err := time.Parse("2006-01-02", tc.on)
			if err != nil {
				t.Fatal(err)
			}
			got := cycleFor(on, tc.s, tc.d)
			if got.start.Format("2006-01-02") != tc.want.start ||
				got.end.Format("2006-01-02") != tc.want.end ||
				got.due.Format("2006-01-02") != tc.want.due {
				t.Fatalf("cycleFor(%s, %d, %d) = %s..%s due %s, want %v", tc.on, tc.s, tc.d,
					got.start.Format("2006-01-02"), got.end.Format("2006-01-02"), got.due.Format("2006-01-02"), tc.want)
			}
		})
	}
}

func TestNextPaydayAfterBoundaries(t *testing.T) {
	cases := []struct {
		today  string
		payday int
		want   string
	}{
		{"2026-11-22", 25, "2026-11-25"},
		{"2026-11-25", 25, "2026-12-25"},
		{"2026-02-27", 31, "2026-02-28"},
		{"2026-02-28", 31, "2026-03-31"},
		{"2026-04-29", 31, "2026-04-30"},
		{"2026-04-30", 31, "2026-05-31"},
		{"2026-12-31", 1, "2027-01-01"},
		{"2026-12-31", 31, "2027-01-31"},
	}
	for _, tc := range cases {
		today, err := time.Parse("2006-01-02", tc.today)
		if err != nil {
			t.Fatal(err)
		}
		if got := nextPaydayAfter(today, tc.payday).Format("2006-01-02"); got != tc.want {
			t.Errorf("nextPaydayAfter(%s, %d) = %s, want %s", tc.today, tc.payday, got, tc.want)
		}
	}
}

func TestCreditDebtPlanBeforePayday(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{Clock: fixedBusinessClock()})
	defer database.Close()
	defer ts.Close()

	mustSetPayday(t, c, 25)
	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "现金", "opening_minor": 50000,
	})
	card := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "credit", "name": "招行卡", "statement_day": 5, "due_day": 23,
	})
	cardID := card["id"].(string)
	category := c.firstCategory("expense")

	for _, tx := range []map[string]any{
		{"amount_minor": 30000, "occurred_on": "2026-09-20"},
		{"amount_minor": 100000, "occurred_on": "2026-10-20"},
		{"amount_minor": 50000, "occurred_on": "2026-11-10"},
	} {
		tx["type"] = "expense"
		tx["category_id"] = category
		tx["account_id"] = cardID
		c.mustOK("POST", "/api/v1/transactions", tx)
	}
	c.mustOK("POST", "/api/v1/accounts/transfers", map[string]any{
		"from_account_id": cash["id"], "to_account_id": cardID,
		"amount_minor": 40000, "occurred_on": "2026-11-01",
	})

	plan := getCreditDebtPlan(t, c)
	if number(plan["payday"]) != 25 || plan["next_payday"] != "2026-11-25" || number(plan["days_until_payday"]) != 3 {
		t.Fatalf("unexpected payday fields: %v", plan)
	}
	if number(plan["total_debt_minor"]) != 140000 || number(plan["unbilled_minor"]) != 50000 ||
		number(plan["overdue_minor"]) != 0 || number(plan["cash_needed_minor"]) != 90000 ||
		number(plan["cash_available_minor"]) != 10000 || number(plan["cash_gap_minor"]) != 80000 || plan["complete"] != true {
		t.Fatalf("unexpected aggregates: %v", plan)
	}
	row := findCreditCard(t, plan, cardID)
	if row["configured"] != true || number(row["total_debt_minor"]) != 140000 ||
		number(row["unbilled_minor"]) != 50000 || number(row["overdue_minor"]) != 0 ||
		number(row["cash_needed_minor"]) != 90000 || row["next_due_on"] != "2026-11-23" ||
		number(row["next_due_minor"]) != 90000 ||
		row["next_due_period_start_on"] != "2026-10-06" || row["next_due_period_end_on"] != "2026-11-05" {
		t.Fatalf("unexpected card row: %v", row)
	}
	cycle, _ := row["current_cycle"].(map[string]any)
	if cycle["start_on"] != "2026-11-06" || cycle["end_on"] != "2026-12-05" || cycle["due_on"] != "2026-12-23" {
		t.Fatalf("unexpected current cycle: %v", cycle)
	}

	items, _ := c.mustOK("GET", "/api/v1/accounts", nil)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("default accounts response changed: %v", items)
	}
	body := c.mustOK("GET", "/api/v1/accounts", nil)
	if _, exists := body["credit_debt"]; exists {
		t.Fatalf("credit_debt must only appear when requested: %v", body)
	}
}

func TestCreditDebtPlanMissingPaydayAndUnconfigured(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{Clock: fixedBusinessClock()})
	defer database.Close()
	defer ts.Close()

	card := c.mustOK("POST", "/api/v1/accounts", map[string]any{"kind": "credit", "name": "待配置卡"})
	cardID := card["id"].(string)
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 10000, "occurred_on": "2026-10-20",
		"category_id": c.firstCategory("expense"), "account_id": cardID,
	})

	plan := getCreditDebtPlan(t, c)
	if plan["payday"] != nil || plan["next_payday"] != nil || plan["cash_needed_minor"] != nil ||
		plan["cash_gap_minor"] != nil || plan["complete"] != false ||
		number(plan["total_debt_minor"]) != 10000 || number(plan["unconfigured_cards"]) != 1 {
		t.Fatalf("unexpected missing-payday plan: %v", plan)
	}
	row := findCreditCard(t, plan, cardID)
	if row["configured"] != false || row["current_cycle"] != nil || row["cash_needed_minor"] != nil ||
		number(row["total_debt_minor"]) != 10000 {
		t.Fatalf("unexpected unconfigured card: %v", row)
	}

	mustSetPayday(t, c, 25)
	plan = getCreditDebtPlan(t, c)
	if plan["complete"] != false || number(plan["unconfigured_cards"]) != 1 ||
		plan["cash_needed_minor"] != nil {
		t.Fatalf("unconfigured cards must block completion: %v", plan)
	}

	c.mustOK("PATCH", "/api/v1/accounts/"+cardID, map[string]any{"statement_day": 5, "due_day": 23})
	plan = getCreditDebtPlan(t, c)
	if plan["complete"] != true || number(plan["unconfigured_cards"]) != 0 ||
		number(plan["cash_needed_minor"]) != 10000 {
		t.Fatalf("configured card was not counted: %v", plan)
	}
	row = findCreditCard(t, plan, cardID)
	if row["next_due_on"] != "2026-11-23" || number(row["next_due_minor"]) != 10000 {
		t.Fatalf("unexpected configured card: %v", row)
	}

	if code, _ := c.do("GET", "/api/v1/accounts?include=bogus", nil); code != 400 {
		t.Fatalf("invalid include returned %d, want 400", code)
	}
}

func TestCreditDebtPlanOpeningDebtIsAlreadyBilled(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{Clock: fixedBusinessClock()})
	defer database.Close()
	defer ts.Close()

	if code, _ := c.do("PATCH", "/api/v1/auth/me/profile", map[string]any{"payday": 25}); code != 200 {
		t.Fatalf("profile update returned %d", code)
	}
	card := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "credit", "name": "期初卡", "opening_minor": 5000,
		"statement_day": 5, "due_day": 23,
	})
	plan := getCreditDebtPlan(t, c)
	row := findCreditCard(t, plan, card["id"].(string))
	if number(plan["cash_needed_minor"]) != 5000 || row["next_due_on"] != "2026-11-23" {
		t.Fatalf("opening debt must be treated as already billed: plan=%v card=%v", plan, row)
	}
}

func TestCreditDebtPlanReflectsReversalAndCorrection(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{Clock: fixedBusinessClock()})
	defer database.Close()
	defer ts.Close()

	mustSetPayday(t, c, 25)
	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{"kind": "cash", "name": "现金"})
	card := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "credit", "name": "冲正卡", "statement_day": 5, "due_day": 23,
	})
	cardID := card["id"].(string)
	category := c.firstCategory("expense")
	createCharge := func(amount int64) map[string]any {
		return c.mustOK("POST", "/api/v1/transactions", map[string]any{
			"type": "expense", "amount_minor": amount, "occurred_on": "2026-10-20",
			"category_id": category, "account_id": cardID,
		})
	}

	deleted := createCharge(10000)
	if got := number(getCreditDebtPlan(t, c)["cash_needed_minor"]); got != 10000 {
		t.Fatalf("initial charge = %d, want 10000", got)
	}
	c.mustOK("DELETE", "/api/v1/transactions/"+deleted["id"].(string), nil)
	if got := number(getCreditDebtPlan(t, c)["cash_needed_minor"]); got != 0 {
		t.Fatalf("deleted charge still counted: %d", got)
	}

	corrected := createCharge(20000)
	if got := number(getCreditDebtPlan(t, c)["cash_needed_minor"]); got != 20000 {
		t.Fatalf("replacement charge = %d, want 20000", got)
	}
	c.mustOK("PATCH", "/api/v1/transactions/"+corrected["id"].(string), map[string]any{"amount_minor": 25000})
	if got := number(getCreditDebtPlan(t, c)["cash_needed_minor"]); got != 25000 {
		t.Fatalf("corrected charge = %d, want 25000", got)
	}

	repayment := c.mustOK("POST", "/api/v1/accounts/transfers", map[string]any{
		"from_account_id": cash["id"], "to_account_id": cardID,
		"amount_minor": 5000, "occurred_on": "2026-11-01",
	})
	if got := number(getCreditDebtPlan(t, c)["cash_needed_minor"]); got != 20000 {
		t.Fatalf("repayment not applied: %d", got)
	}
	c.mustOK("DELETE", "/api/v1/transactions/"+repayment["id"].(string), nil)
	if got := number(getCreditDebtPlan(t, c)["cash_needed_minor"]); got != 25000 {
		t.Fatalf("deleted repayment still reduced debt: %d", got)
	}
}

func TestCreditDebtPlanFutureAndOverpayment(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{Clock: fixedBusinessClock()})
	defer database.Close()
	defer ts.Close()

	mustSetPayday(t, c, 25)
	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{"kind": "cash", "name": "现金"})
	card := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "credit", "name": "未来卡", "statement_day": 5, "due_day": 23,
	})
	cardID := card["id"].(string)
	category := c.firstCategory("expense")

	// A future-dated charge is not debt as of today.
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 10000, "occurred_on": "2026-12-10",
		"category_id": category, "account_id": cardID,
	})
	if got := number(getCreditDebtPlan(t, c)["total_debt_minor"]); got != 0 {
		t.Fatalf("future charge counted as today's debt: %d", got)
	}

	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 10000, "occurred_on": "2026-10-20",
		"category_id": category, "account_id": cardID,
	})
	c.mustOK("POST", "/api/v1/accounts/transfers", map[string]any{
		"from_account_id": cash["id"], "to_account_id": cardID,
		"amount_minor": 15000, "occurred_on": "2026-11-01",
	})
	plan := getCreditDebtPlan(t, c)
	row := findCreditCard(t, plan, cardID)
	if number(plan["total_debt_minor"]) != 0 || number(plan["cash_needed_minor"]) != 0 ||
		number(row["credit_balance_minor"]) != 5000 || row["next_due_on"] != nil {
		t.Fatalf("overpayment not reflected: plan=%v card=%v", plan, row)
	}
}

func TestCreditDebtPlanHouseholdIsolation(t *testing.T) {
	e := newRemEnv(t, Options{Clock: fixedBusinessClock(), SetupToken: testSetupToken})
	a := bootstrapHousehold(t, e, "credit_a")
	b := bootstrapHousehold(t, e, "credit_b")
	ca, cb := a.client(t, e.ts.URL), b.client(t, e.ts.URL)

	setup := func(c *remClient, cardName string, amount int64, statementDay, dueDay int) (string, map[string]any) {
		c.mustCall("PATCH", "/api/v1/auth/me/profile", map[string]any{"payday": 25})
		c.createAccount("cash", cardName+"现金", 50000)
		card := c.mustCall("POST", "/api/v1/accounts", map[string]any{
			"kind": "credit", "name": cardName, "statement_day": statementDay, "due_day": dueDay,
		}).Body["id"].(string)
		c.createExpense(amount, "2026-10-20", c.firstCategory("expense"), card)
		plan, _ := c.mustCall("GET", "/api/v1/accounts?include=credit_debt", nil).Body["credit_debt"].(map[string]any)
		return card, plan
	}

	cardA, planA := setup(ca, "A 卡", 10000, 5, 23)
	cardB, planB := setup(cb, "B 卡", 20000, 10, 20)
	if number(planA["total_debt_minor"]) != 10000 || number(planB["total_debt_minor"]) != 20000 {
		t.Fatalf("household debt leaked: A=%v B=%v", planA, planB)
	}
	rowA := findCreditCard(t, planA, cardA)
	rowB := findCreditCard(t, planB, cardB)
	if rowA["name"] != "A 卡" || rowB["name"] != "B 卡" {
		t.Fatalf("household cards leaked: A=%v B=%v", rowA, rowB)
	}
	if len(planA["cards"].([]any)) != 1 || len(planB["cards"].([]any)) != 1 {
		t.Fatalf("household card lists leaked: A=%v B=%v", planA["cards"], planB["cards"])
	}
}
