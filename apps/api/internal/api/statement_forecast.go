package api

import (
	"context"
	"time"

	"tinynest/api/internal/money"
)

type statementForecast struct {
	start, end, due time.Time
	amount          money.Minor
}

func (a *API) loadStatementForecasts(ctx context.Context, householdID, today string) (map[string]statementForecast, error) {
	rows, err := a.DB.QueryContext(ctx, `
 SELECT s.account_id, COALESCE(s.period_start_on,s.period_end_on), s.period_end_on, s.due_on, s.amount_due_minor
 FROM card_statements s
 WHERE s.household_id=? AND s.currency='CNY' AND s.period_end_on<=? AND s.due_on IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM card_statements newer
     WHERE newer.household_id=s.household_id AND newer.account_id=s.account_id AND newer.currency='CNY'
       AND newer.period_end_on>s.period_end_on AND newer.period_end_on<=? AND newer.due_on IS NOT NULL)
 `, householdID, today, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	forecasts := map[string]statementForecast{}
	for rows.Next() {
		var id, start, end, due string
		var bill statementForecast
		if err := rows.Scan(&id, &start, &end, &due, &bill.amount); err != nil {
			return nil, err
		}
		for _, date := range []struct {
			text string
			out  *time.Time
		}{
			{start, &bill.start}, {end, &bill.end}, {due, &bill.due},
		} {
			*date.out, err = time.Parse(time.DateOnly, date.text)
			if err != nil {
				return nil, err
			}
		}
		forecasts[id] = bill
	}
	return forecasts, rows.Err()
}

func forecastCreditCard(card *creditCardState, bill statementForecast) creditCardState {
	projected := *card
	projected.charges = []creditCharge{{
		amount: bill.amount, occurredOn: bill.end, periodStart: bill.start, periodEnd: bill.end, dueOn: bill.due,
	}}
	for _, charge := range card.charges {
		if charge.occurredOn.After(bill.end) && (!charge.placeholder || charge.periodEnd.After(bill.end)) {
			projected.charges = append(projected.charges, charge)
		}
	}
	projected.reductions = 0
	for _, payment := range card.payments {
		if payment.occurredOn.After(bill.end) {
			projected.reductions += payment.amount
		}
	}
	projected.configured = true
	return projected
}
