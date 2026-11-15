package api

import (
	"context"
	"testing"
	"time"
)

func TestSchedulerRecordsFailureAndRetries(t *testing.T) {
	ts, client, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	// Fail the first projection after its work has started. The failed
	// attempt must roll back before an independent failure row can be saved.
	_, err := database.Exec(`CREATE TRIGGER fail_first_projection BEFORE INSERT ON schedule_runs
		WHEN NEW.status='ok' AND NOT EXISTS (SELECT 1 FROM schedule_runs WHERE status='failed')
		BEGIN SELECT RAISE(ABORT, 'fixture projection failure'); END`)
	if err != nil {
		t.Fatal(err)
	}

	api := New(database, Options{SchedulerInterval: time.Millisecond})
	api.runSchedulerOnce(context.Background(), "daily")
	out := client.mustOK("GET", "/api/v1/schedule/runs", nil)
	runs := out["runs"].([]any)
	statuses := map[string]bool{}
	for _, item := range runs {
		run := item.(map[string]any)
		statuses[run["trigger"].(string)+":"+run["status"].(string)] = true
	}
	if len(runs) != 2 || !statuses["daily:failed"] || !statuses["retry:ok"] {
		t.Fatalf("expected failed scheduled run followed by successful retry, got %v", runs)
	}
}
