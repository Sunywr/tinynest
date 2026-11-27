package api

import (
	"testing"
	"time"
)

func TestMailboxBillingPeriodsFollowCardClosingDay(t *testing.T) {
	today := time.Date(2026, time.March, 5, 0, 0, 0, 0, time.UTC)
	fourth := mailboxBillingPeriods(today, 4)
	if len(fourth) != 12 || fourth[0] != (mailboxBillingPeriod{Start: "2026-02-05", End: "2026-03-04"}) {
		t.Fatalf("4th-day cycle: %v", fourth)
	}
	monthEnd := mailboxBillingPeriods(today, 31)
	if len(monthEnd) != 12 || monthEnd[0] != (mailboxBillingPeriod{Start: "2026-02-01", End: "2026-02-28"}) {
		t.Fatalf("month-end cycle: %v", monthEnd)
	}
}
