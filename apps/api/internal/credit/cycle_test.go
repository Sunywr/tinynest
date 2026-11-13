package credit

import (
	"errors"
	"testing"
	"time"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// Case 9: month-end 28/29/30/31 and leap years must never yield an invalid date.
func TestCase09_MonthEndClamping(t *testing.T) {
	cases := []struct {
		year  int
		month time.Month
		day   int
		want  string
	}{
		{2026, time.February, 31, "2026-02-28"},
		{2028, time.February, 31, "2028-02-29"}, // leap year
		{2026, time.February, 30, "2026-02-28"},
		{2026, time.April, 31, "2026-04-30"},
		{2026, time.January, 31, "2026-01-31"},
		{2026, time.February, 28, "2026-02-28"},
		{2026, time.February, 29, "2026-02-28"},
	}
	for _, c := range cases {
		got, err := ClampDay(c.year, c.month, c.day)
		if err != nil {
			t.Fatalf("ClampDay(%d,%v,%d): %v", c.year, c.month, c.day, err)
		}
		if got.Format("2006-01-02") != c.want {
			t.Errorf("ClampDay(%d,%v,%d)=%s want %s", c.year, c.month, c.day, got.Format("2006-01-02"), c.want)
		}
	}
	for _, d := range []int{0, 32, -1} {
		if _, err := ClampDay(2026, time.March, d); !errors.Is(err, ErrInvalidDay) {
			t.Errorf("day %d must be rejected, got %v", d, err)
		}
	}
}

func TestCycleForSameMonthPeriod(t *testing.T) {
	// 1st -> month end, due on the 10th of the following month.
	got, err := CycleFor(date("2026-02-15"), 1, 31, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Start.Format("2006-01-02") != "2026-02-01" ||
		got.End.Format("2006-01-02") != "2026-02-28" ||
		got.Due.Format("2006-01-02") != "2026-03-10" {
		t.Fatalf("unexpected cycle: %s", got)
	}

	// A 31-day month keeps 3/31 and moves the due date to 4/10.
	got, err = CycleFor(date("2026-03-05"), 1, 31, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.End.Format("2006-01-02") != "2026-03-31" || got.Due.Format("2006-01-02") != "2026-04-10" {
		t.Fatalf("unexpected March cycle: %s", got)
	}
}

func TestCycleForCrossMonthPeriod(t *testing.T) {
	// 26th of previous month -> 25th of current month, due next month.
	got, err := CycleFor(date("2026-03-10"), 26, 25, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Start.Format("2006-01-02") != "2026-02-26" || got.End.Format("2006-01-02") != "2026-03-25" {
		t.Fatalf("unexpected cross-month cycle: %s", got)
	}
	if got.Due.Format("2006-01-02") != "2026-04-05" {
		t.Fatalf("unexpected due date: %s", got)
	}
}

func TestCycleRollingToNextMonth(t *testing.T) {
	// After the cycle end (2/28), the anchor belongs to the March cycle.
	got, err := CycleFor(date("2026-03-01"), 1, 31, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Start.Format("2006-01-02") != "2026-03-01" || got.End.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("unexpected rolled cycle: %s", got)
	}
	if _, err := CycleFor(date("2026-03-01"), 1, 31, 10, 13); !errors.Is(err, ErrInvalidOffset) {
		t.Errorf("invalid offset must be rejected, got %v", err)
	}
}

func TestInstallmentDueDatesClampEveryMonth(t *testing.T) {
	got, err := InstallmentDueDates(date("2026-01-31"), 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30"}
	for i, d := range got {
		if d.Format("2006-01-02") != want[i] {
			t.Errorf("period %d = %s want %s", i+1, d.Format("2006-01-02"), want[i])
		}
	}
	if _, err := InstallmentDueDates(date("2026-01-31"), 0); !errors.Is(err, ErrInvalidPeriods) {
		t.Errorf("zero periods must be rejected, got %v", err)
	}
}
