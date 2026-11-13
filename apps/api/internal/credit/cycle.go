// Package credit implements billing-cycle date rules.
// Month-end days 28/29/30/31 are clamped to the last valid day of the month.
package credit

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidDay     = errors.New("credit: day must be within 1..31")
	ErrInvalidOffset  = errors.New("credit: due month offset must be 0..12")
	ErrInvalidPeriods = errors.New("credit: periods must be 1..600")
)

type Cycle struct {
	Start time.Time `json:"cycle_start_on"`
	End   time.Time `json:"cycle_end_on"`
	Due   time.Time `json:"due_on"`
}

func (c Cycle) String() string {
	f := "2006-01-02"
	return c.Start.Format(f) + " ~ " + c.End.Format(f) + " (due " + c.Due.Format(f) + ")"
}

// ClampDay resolves a configured day-of-month against a real calendar month.
// Feb 31 -> Feb 28 (or Feb 29 in a leap year); Apr 31 -> Apr 30.
func ClampDay(year int, month time.Month, day int) (time.Time, error) {
	if day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("%w: %d", ErrInvalidDay, day)
	}
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

// AddMonths returns the FIRST day of the month n months away.
// Operating on day 1 avoids time.AddDate's day overflow (e.g. Jan 31 + 1 month -> Mar 3).
func AddMonths(t time.Time, n int) time.Time {
	return time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
}

// CycleFor computes the billing cycle containing anchor.
//   - startDay <= endDay: period stays inside one calendar month.
//   - startDay > endDay:  period spans the previous month (e.g. 26th -> 25th).
//   - dueMonthOffset:     0 = same month as cycle end, 1 = next month (common).
func CycleFor(anchor time.Time, startDay, endDay, dueDay, dueMonthOffset int) (Cycle, error) {
	for _, d := range []int{startDay, endDay, dueDay} {
		if d < 1 || d > 31 {
			return Cycle{}, fmt.Errorf("%w: %d", ErrInvalidDay, d)
		}
	}
	if dueMonthOffset < 0 || dueMonthOffset > 12 {
		return Cycle{}, ErrInvalidOffset
	}
	anchorDay := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)

	end, err := ClampDay(anchor.Year(), anchor.Month(), endDay)
	if err != nil {
		return Cycle{}, err
	}
	if anchorDay.After(end) {
		next := AddMonths(end, 1)
		end, err = ClampDay(next.Year(), next.Month(), endDay)
		if err != nil {
			return Cycle{}, err
		}
	}

	var start time.Time
	if startDay <= endDay {
		start, err = ClampDay(end.Year(), end.Month(), startDay)
	} else {
		prev := AddMonths(end, -1)
		start, err = ClampDay(prev.Year(), prev.Month(), startDay)
	}
	if err != nil {
		return Cycle{}, err
	}

	dueMonth := AddMonths(end, dueMonthOffset)
	due, err := ClampDay(dueMonth.Year(), dueMonth.Month(), dueDay)
	if err != nil {
		return Cycle{}, err
	}
	if !due.After(end) {
		next := AddMonths(dueMonth, 1)
		due, err = ClampDay(next.Year(), next.Month(), dueDay)
		if err != nil {
			return Cycle{}, err
		}
	}
	return Cycle{Start: start, End: end, Due: due}, nil
}

// InstallmentDueDates generates monthly due dates, clamping each month end.
func InstallmentDueDates(first time.Time, periods int) ([]time.Time, error) {
	if periods < 1 || periods > 600 {
		return nil, ErrInvalidPeriods
	}
	day := first.Day()
	base := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.UTC)
	out := make([]time.Time, periods)
	for i := 0; i < periods; i++ {
		m := AddMonths(base, i)
		d, err := ClampDay(m.Year(), m.Month(), day)
		if err != nil {
			return nil, err
		}
		out[i] = d
	}
	return out, nil
}
