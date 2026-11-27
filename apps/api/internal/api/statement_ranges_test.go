package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestedStatementRanges(t *testing.T) {
	req := httptest.NewRequest("POST", "/?range=2026-08-01,2026-08-31&range=2026-10-01,2026-10-31", nil)
	ranges, ok := requestedStatementRanges(httptest.NewRecorder(), req)
	if !ok || len(ranges) != 2 || ranges[0].start.Format(time.DateOnly) != "2026-08-01" {
		t.Fatalf("unexpected ranges: %v", ranges)
	}
	if !ranges[1].includes("2026-10-31") || ranges[1].includes("2026-11-01") {
		t.Fatal("statement date selection must include the end date")
	}
	invalid := httptest.NewRequest("POST", "/?range=2026-10-31,2026-10-01", nil)
	response := httptest.NewRecorder()
	if _, ok := requestedStatementRanges(response, invalid); ok || response.Code != 400 {
		t.Fatal("reversed range accepted")
	}
}
