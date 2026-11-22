package api

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"tinynest/api/internal/money"
)

// creditDebtPlanResponse is the read-only projection returned by
// GET /accounts?include=credit_debt. It is derived from journal lines plus the
// currently active business transactions; no statement rows are stored and no
// ledger entry is written.
type creditDebtPlanResponse struct {
	Today              string           `json:"today"`
	Payday             *int64           `json:"payday"`
	NextPayday         *string          `json:"next_payday"`
	DaysUntilPayday    *int64           `json:"days_until_payday"`
	Complete           bool             `json:"complete"`
	UnconfiguredCards  int              `json:"unconfigured_cards"`
	CashAvailableMinor int64            `json:"cash_available_minor"`
	CashNeededMinor    *int64           `json:"cash_needed_minor"`
	CashGapMinor       *int64           `json:"cash_gap_minor"`
	TotalDebtMinor     int64            `json:"total_debt_minor"`
	OverdueMinor       int64            `json:"overdue_minor"`
	UnbilledMinor      int64            `json:"unbilled_minor"`
	Cards              []creditDebtCard `json:"cards"`
}

type creditDebtCard struct {
	AccountID          string       `json:"account_id"`
	Name               string       `json:"name"`
	Status             string       `json:"status"`
	Configured         bool         `json:"configured"`
	StatementDay       *int64       `json:"statement_day"`
	DueDay             *int64       `json:"due_day"`
	TotalDebtMinor     int64        `json:"total_debt_minor"`
	UnbilledMinor      int64        `json:"unbilled_minor"`
	OverdueMinor       int64        `json:"overdue_minor"`
	CreditBalanceMinor int64        `json:"credit_balance_minor"`
	CashNeededMinor    *int64       `json:"cash_needed_minor"`
	NextDueOn          *string      `json:"next_due_on"`
	NextDueMinor       int64        `json:"next_due_minor"`
	NextDuePeriodStart *string      `json:"next_due_period_start_on"`
	NextDuePeriodEnd   *string      `json:"next_due_period_end_on"`
	CurrentCycle       *creditCycle `json:"current_cycle"`
}

type creditCycle struct {
	StartOn string `json:"start_on"`
	EndOn   string `json:"end_on"`
	DueOn   string `json:"due_on"`
}

type creditCardState struct {
	id           string
	name         string
	status       string
	statementDay int
	dueDay       int
	configured   bool
	charges      []creditCharge
	reductions   money.Minor
}

type creditCharge struct {
	amount      money.Minor
	occurredOn  time.Time
	periodStart time.Time
	periodEnd   time.Time
	dueOn       time.Time
}

type creditDueBucket struct {
	dueOn       time.Time
	periodStart time.Time
	periodEnd   time.Time
	amount      money.Minor
}

// creditDebtPlan builds the household credit-card repayment plan for the
// authenticated user's next payday. All queries are household-scoped in SQL.
func (a *API) creditDebtPlan(ctx context.Context, householdID, userID string, now time.Time) (creditDebtPlanResponse, error) {
	today := dateOnly(now)
	todayText := today.Format("2006-01-02")

	var payday sql.NullInt64
	if err := a.DB.QueryRowContext(ctx,
		`SELECT payday FROM users WHERE id=? AND household_id=?`, userID, householdID).Scan(&payday); err != nil {
		return creditDebtPlanResponse{}, err
	}
	validPayday := payday.Valid && payday.Int64 >= 1 && payday.Int64 <= 31

	cards, order, err := a.loadCreditCards(ctx, householdID)
	if err != nil {
		return creditDebtPlanResponse{}, err
	}
	if err := a.loadCreditEvents(ctx, householdID, todayText, cards); err != nil {
		return creditDebtPlanResponse{}, err
	}

	var cashAvailable int64
	if err := a.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(l.debit_minor-l.credit_minor),0)
		FROM accounts a
		JOIN journal_lines l ON l.account_code=a.ledger_code
		JOIN journal_entries e ON e.id=l.entry_id AND e.household_id=a.household_id
		WHERE a.household_id=? AND a.kind='cash' AND a.status='active' AND e.occurred_on<=?`,
		householdID, todayText).Scan(&cashAvailable); err != nil {
		return creditDebtPlanResponse{}, err
	}

	out := creditDebtPlanResponse{
		Today:              todayText,
		CashAvailableMinor: cashAvailable,
		Cards:              []creditDebtCard{},
	}
	var nextPayday time.Time
	if validPayday {
		p := payday.Int64
		out.Payday = &p
		nextPayday = nextPaydayAfter(today, int(p))
		nextText := nextPayday.Format("2006-01-02")
		days := int64(nextPayday.Sub(today).Hours() / 24)
		out.NextPayday = &nextText
		out.DaysUntilPayday = &days
	}

	var totalNeeded int64
	var configuredCount int
	for _, id := range order {
		card := cards[id]
		buckets, unbilled, overdue, total, creditBalance := allocateCreditDebt(card, today)

		row := creditDebtCard{
			AccountID:          card.id,
			Name:               card.name,
			Status:             card.status,
			Configured:         card.configured,
			TotalDebtMinor:     int64(total),
			UnbilledMinor:      int64(unbilled),
			OverdueMinor:       int64(overdue),
			CreditBalanceMinor: int64(creditBalance),
		}
		if card.configured {
			s, d := int64(card.statementDay), int64(card.dueDay)
			row.StatementDay, row.DueDay = &s, &d
			cycle := cycleFor(today, card.statementDay, card.dueDay)
			row.CurrentCycle = &creditCycle{
				StartOn: cycle.start.Format("2006-01-02"),
				EndOn:   cycle.end.Format("2006-01-02"),
				DueOn:   cycle.due.Format("2006-01-02"),
			}
			if next, ok := firstFutureBucket(buckets, today); ok {
				dueText := next.dueOn.Format("2006-01-02")
				startText := next.periodStart.Format("2006-01-02")
				endText := next.periodEnd.Format("2006-01-02")
				row.NextDueOn = &dueText
				row.NextDueMinor = int64(next.amount)
				row.NextDuePeriodStart = &startText
				row.NextDuePeriodEnd = &endText
			}
			configuredCount++
			if validPayday {
				needed := neededBefore(buckets, nextPayday)
				row.CashNeededMinor = &needed
				totalNeeded += needed
			}
		}
		out.TotalDebtMinor += row.TotalDebtMinor
		out.OverdueMinor += row.OverdueMinor
		out.UnbilledMinor += row.UnbilledMinor
		out.Cards = append(out.Cards, row)
	}

	out.UnconfiguredCards = len(order) - configuredCount
	out.Complete = validPayday && out.UnconfiguredCards == 0
	if validPayday && configuredCount > 0 {
		needed := totalNeeded
		out.CashNeededMinor = &needed
		gap := needed - cashAvailable
		if gap < 0 {
			gap = 0
		}
		out.CashGapMinor = &gap
	}
	return out, nil
}

// loadCreditCards returns every non-deleted credit account, preserving creation
// order for a stable UI. Cards without events still appear so the user can see
// the current cycle or discover missing configuration.
func (a *API) loadCreditCards(ctx context.Context, householdID string) (map[string]*creditCardState, []string, error) {
	rows, err := a.DB.QueryContext(ctx, `
		SELECT id,name,status,statement_day,due_day
		FROM accounts
		WHERE household_id=? AND kind='credit' AND status<>'deleted'
		ORDER BY created_at,id`, householdID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cards := map[string]*creditCardState{}
	order := []string{}
	for rows.Next() {
		var id, name, status string
		var statementDay, dueDay sql.NullInt64
		if err := rows.Scan(&id, &name, &status, &statementDay, &dueDay); err != nil {
			return nil, nil, err
		}
		card := &creditCardState{id: id, name: name, status: status}
		if statementDay.Valid && dueDay.Valid &&
			statementDay.Int64 >= 1 && statementDay.Int64 <= 31 &&
			dueDay.Int64 >= 1 && dueDay.Int64 <= 31 {
			card.configured = true
			card.statementDay = int(statementDay.Int64)
			card.dueDay = int(dueDay.Int64)
		}
		cards[id] = card
		order = append(order, id)
	}
	return cards, order, rows.Err()
}

// loadCreditEvents reads only the events that still represent current business
// intent:
//   - a posted credit-card expense (transaction type=expense, posted),
//   - a posted cash -> credit repayment (transaction type=transfer, posted),
//   - an opening liability entry.
//
// Reversed/deleted/corrected originals and reversal entries are excluded; their
// net effect is already zero on the ledger and they must not be scheduled twice.
func (a *API) loadCreditEvents(ctx context.Context, householdID, today string, cards map[string]*creditCardState) error {
	rows, err := a.DB.QueryContext(ctx, `
		SELECT a.id,e.occurred_on,e.biz_type,
		       COALESCE(SUM(l.credit_minor-l.debit_minor),0) AS delta_minor
		FROM accounts a
		JOIN journal_lines l ON l.account_code=a.ledger_code
		JOIN journal_entries e ON e.id=l.entry_id AND e.household_id=a.household_id
		LEFT JOIN transactions t ON t.id=e.biz_id AND t.household_id=e.household_id
		WHERE a.household_id=? AND a.kind='credit' AND a.status<>'deleted'
		  AND e.occurred_on<=? AND e.status='posted' AND e.reversal_of IS NULL
		  AND (
		    e.biz_type='opening_liability'
		    OR (t.status='posted' AND (
		      (e.biz_type='expense' AND t.type='expense' AND t.account_id=a.id)
		      OR (e.biz_type='credit_repayment' AND t.type='transfer' AND t.to_account_id=a.id)
		    ))
		  )
		GROUP BY a.id,e.id,e.occurred_on,e.biz_type
		ORDER BY a.id,e.occurred_on,e.id`, householdID, today)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID, occurredOn, bizType string
		var delta int64
		if err := rows.Scan(&accountID, &occurredOn, &bizType, &delta); err != nil {
			return err
		}
		card := cards[accountID]
		if card == nil || delta == 0 {
			continue
		}
		on, err := time.Parse("2006-01-02", occurredOn)
		if err != nil {
			return err
		}
		if delta > 0 {
			charge := creditCharge{amount: money.Minor(delta), occurredOn: on}
			if card.configured {
				if bizType == "opening_liability" {
					// Opening debt is already billed as of the account's opening date:
					// assign it to the most recent statement close on/before that date.
					end := previousStatementCloseOnOrBefore(on, card.statementDay)
					charge.periodEnd = end
					charge.periodStart = previousStatementClose(end, card.statementDay).AddDate(0, 0, 1)
					charge.dueOn = dueAfter(end, card.dueDay)
				} else {
					cycle := cycleFor(on, card.statementDay, card.dueDay)
					charge.periodStart, charge.periodEnd, charge.dueOn = cycle.start, cycle.end, cycle.due
				}
			}
			card.charges = append(card.charges, charge)
		} else {
			card.reductions += money.Minor(-delta)
		}
	}
	return rows.Err()
}

// allocateCreditDebt applies repayments FIFO to charges ordered by due date and
// returns the outstanding buckets plus the aggregate views used by the API.
// When a card is not configured there are no dates to schedule; only the debt
// total and any credit balance are still meaningful.
func allocateCreditDebt(card *creditCardState, today time.Time) (buckets []creditDueBucket, unbilled, overdue, total, creditBalance money.Minor) {
	if !card.configured {
		total = totalCharges(card) - card.reductions
		if total < 0 {
			creditBalance = -total
			total = 0
		}
		return nil, 0, 0, total, creditBalance
	}
	sort.SliceStable(card.charges, func(i, j int) bool {
		if !card.charges[i].dueOn.Equal(card.charges[j].dueOn) {
			return card.charges[i].dueOn.Before(card.charges[j].dueOn)
		}
		return card.charges[i].occurredOn.Before(card.charges[j].occurredOn)
	})
	remainingReduction := card.reductions
	for _, charge := range card.charges {
		outstanding := charge.amount
		if remainingReduction > 0 {
			offset := outstanding
			if remainingReduction < offset {
				offset = remainingReduction
			}
			outstanding -= offset
			remainingReduction -= offset
		}
		if outstanding <= 0 {
			continue
		}
		total += outstanding
		if charge.periodEnd.After(today) {
			unbilled += outstanding
		}
		found := false
		for i := range buckets {
			if buckets[i].dueOn.Equal(charge.dueOn) {
				buckets[i].amount += outstanding
				found = true
				break
			}
		}
		if !found {
			buckets = append(buckets, creditDueBucket{
				dueOn:       charge.dueOn,
				periodStart: charge.periodStart,
				periodEnd:   charge.periodEnd,
				amount:      outstanding,
			})
		}
	}
	if remainingReduction > 0 {
		creditBalance = remainingReduction
	}
	for _, bucket := range buckets {
		if bucket.dueOn.Before(today) {
			overdue += bucket.amount
		}
	}
	return buckets, unbilled, overdue, total, creditBalance
}

func totalCharges(card *creditCardState) money.Minor {
	var total money.Minor
	for _, charge := range card.charges {
		total += charge.amount
	}
	return total
}

// neededBefore sums every outstanding bucket whose due date is on or before the
// next payday. Overdue buckets are included because the cash is still needed.
func neededBefore(buckets []creditDueBucket, nextPayday time.Time) int64 {
	var total int64
	for _, bucket := range buckets {
		if !bucket.dueOn.After(nextPayday) {
			total += int64(bucket.amount)
		}
	}
	return total
}

func firstFutureBucket(buckets []creditDueBucket, today time.Time) (creditDueBucket, bool) {
	for _, bucket := range buckets {
		if !bucket.dueOn.Before(today) {
			return bucket, true
		}
	}
	return creditDueBucket{}, false
}

// --- date helpers -----------------------------------------------------------

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func clampDay(year int, month time.Month, day int) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// statementCloseOnOrAfter is the first configured statement day on/after on.
// A charge made on the statement day belongs to that day's close.
func statementCloseOnOrAfter(on time.Time, day int) time.Time {
	close := clampDay(on.Year(), on.Month(), day)
	if close.Before(on) {
		close = clampDay(on.Year(), on.Month()+1, day)
	}
	return close
}

func previousStatementClose(end time.Time, day int) time.Time {
	return clampDay(end.Year(), end.Month()-1, day)
}

func previousStatementCloseOnOrBefore(on time.Time, day int) time.Time {
	close := clampDay(on.Year(), on.Month(), day)
	if close.After(on) {
		close = clampDay(on.Year(), on.Month()-1, day)
	}
	return close
}

// dueAfter is the first configured due day strictly after the statement close.
// If the configured due day is the same as (or earlier than) the close day, the
// repayment falls into the following month.
func dueAfter(close time.Time, day int) time.Time {
	due := clampDay(close.Year(), close.Month(), day)
	if !due.After(close) {
		due = clampDay(close.Year(), close.Month()+1, day)
	}
	return due
}

type creditCycleDates struct {
	start time.Time
	end   time.Time
	due   time.Time
}

func cycleFor(on time.Time, statementDay, dueDay int) creditCycleDates {
	end := statementCloseOnOrAfter(on, statementDay)
	start := previousStatementClose(end, statementDay).AddDate(0, 0, 1)
	return creditCycleDates{start: start, end: end, due: dueAfter(end, dueDay)}
}

// nextPaydayAfter returns the first configured payday strictly after today.
// Month-end days (28/29/30/31) are clamped to the real length of each month.
func nextPaydayAfter(today time.Time, payday int) time.Time {
	for offset := 0; ; offset++ {
		month := time.Date(today.Year(), today.Month()+time.Month(offset), 1, 0, 0, 0, 0, time.UTC)
		candidate := clampDay(month.Year(), month.Month(), payday)
		if candidate.After(today) {
			return candidate
		}
	}
}
