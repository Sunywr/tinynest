package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"
)

// V0.1.1 M4: automatic daily due-item projection.
//
// The projection ONLY inserts pending obligations. It never marks anything
// paid, never posts journal entries and never changes a balance. Idempotency
// comes from the UNIQUE biz_key (household:source:source_id) combined with
// INSERT OR IGNORE, so repeating the task any number of times is a no-op.
// Every run (startup backfill, daily tick or manual POST) is logged in
// schedule_runs with per-household results and failures.

// confirmDueItem marks the matching pending due item as confirmed inside the
// SAME transaction as the business payment, so the to-do list can never
// disagree with the ledger. A due item may legitimately not exist (e.g. the
// bill was paid long before its due date), which is not an error.
func confirmDueItem(ctx context.Context, tx *sql.Tx, householdID, sourceType, sourceID string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE schedule_due_items SET status='confirmed'
		 WHERE household_id=? AND source_type=? AND source_id=? AND status='pending'`,
		householdID, sourceType, sourceID); err != nil {
		slog.Error("api: confirm due item failed", "error", err)
		return err
	}
	return nil
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

// projectDueItems builds the pending to-do projection for ONE household and
// records the run in schedule_runs. horizonDays of 0 means the built-in
// 30-day window. Only UNPAID remainders are projected, so a partial payment
// leaves the rest still due instead of hiding it.
func (a *API) projectDueItems(ctx context.Context, householdID, trigger string, horizonDays int) (int, error) {
	if horizonDays <= 0 {
		horizonDays = 30
	}
	horizon := a.businessToday().AddDate(0, 0, horizonDays).Format("2006-01-02")
	type found struct {
		kind, id, due, label string
		remaining            int64
	}
	var founds []found
	add := func(kind, id, due, label string, remaining int64) {
		if remaining > 0 {
			founds = append(founds, found{kind: kind, id: id, due: due, label: label, remaining: remaining})
		}
	}
	generated := 0
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		// Credit statements: the unpaid remainder is derived from facts
		// (paid_minor < system_due_minor), never from a cached status.
		rows, err := tx.QueryContext(ctx, `
			SELECT s.id,s.due_on,s.system_due_minor-s.paid_minor,i.name
			FROM credit_statements s JOIN credit_instruments i ON i.id=s.instrument_id
			WHERE s.household_id=? AND s.due_on<=? AND s.paid_minor<s.system_due_minor
			ORDER BY s.due_on`, householdID, horizon)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, due, name string
			var remaining int64
			if err := rows.Scan(&id, &due, &remaining, &name); err != nil {
				_ = rows.Close()
				return err
			}
			add("credit_statement", id, due, name+" 账单", remaining)
		}
		_ = rows.Close()
		// Installment and debt periods with unpaid remainders.
		for _, src := range []struct{ table, kind, contractTable string }{
			{"installment_schedule_items", "installment", "installment_contracts"},
			{"debt_schedule_items", "debt", "debt_contracts"},
		} {
			q := `
				SELECT s.id,s.due_on,s.total_minor-s.paid_minor,c.name,s.installment_no
				FROM ` + src.table + ` s JOIN ` + src.contractTable + ` c ON c.id=s.contract_id
				WHERE c.household_id=? AND s.status IN ('pending','partial','overdue') AND s.due_on<=?
				ORDER BY s.due_on`
			rows, err := tx.QueryContext(ctx, q, householdID, horizon)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id, due, name string
				var no int
				var remaining int64
				if err := rows.Scan(&id, &due, &remaining, &name, &no); err != nil {
					_ = rows.Close()
					return err
				}
				add(src.kind, id, due, name+" 第"+itoa(no)+"期", remaining)
			}
			_ = rows.Close()
		}
		insertDue := "INSERT OR IGNORE INTO schedule_due_items(id,household_id,source_type,source_id,biz_key,due_on,amount_minor,status,source_label) VALUES(?,?,?,?,?,?,?,'pending',?)"
		if a.DB.IsMySQL() {
			insertDue = "INSERT INTO schedule_due_items(id,household_id,source_type,source_id,biz_key,due_on,amount_minor,status,source_label) VALUES(?,?,?,?,?,?,?,'pending',?) ON DUPLICATE KEY UPDATE id=id"
		}
		for _, f := range founds {
			bizKey := householdID + ":" + f.kind + ":" + f.id
			res, err := tx.ExecContext(ctx, insertDue,
				newID("due"), householdID, f.kind, f.id, bizKey, f.due, f.remaining, f.label)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				generated++
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO schedule_runs(id,household_id,run_on,`+"`trigger`"+`,horizon_days,`+"`generated`"+`,status,detail)
			VALUES(?,?,?,?,?,?,'ok',NULL)`,
			newID("run"), householdID, a.businessDate(), trigger, horizonDays, generated); err != nil {
			return err
		}
		return nil
	})
	return generated, err
}

// runDailyProjection projects due items for EVERY household, isolating
// per-household failures so one bad row can never starve the others.
func (a *API) runDailyProjection(ctx context.Context, trigger string) (int, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT id FROM households ORDER BY id`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	total := 0
	for _, id := range ids {
		n, err := a.projectDueItems(ctx, id, trigger, 30)
		if err != nil {
			slog.Error("api: due-item projection failed", "household", id, "trigger", trigger, "error", err)
			continue
		}
		total += n
	}
	return total, nil
}

// nextDailyRun returns the next Asia/Shanghai 00:07 boundary after t.
func nextDailyRun(t time.Time) time.Time {
	cst := time.FixedZone("CST", 8*3600)
	d := t.In(cst)
	next := time.Date(d.Year(), d.Month(), d.Day(), 0, 7, 0, 0, cst)
	if !next.After(d) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// StartScheduler launches the lightweight single-instance daily scheduler:
// a startup backfill plus one run per Asia/Shanghai day at 00:07. The
// projection is idempotent, so a missed tick self-heals on the next run and
// a restarted process re-scans on boot. A positive SchedulerInterval switches
// to a fixed cadence (test hook only; production uses the daily boundary).
func (a *API) StartScheduler(parent context.Context) {
	if a.opts.SchedulerInterval > 0 {
		go a.schedulerLoop(parent, a.opts.SchedulerInterval)
		return
	}
	go func() {
		a.runSchedulerOnce(parent, "startup_backfill")
		for {
			delay := time.Until(nextDailyRun(a.now()))
			if delay <= 0 {
				delay = time.Minute
			}
			timer := time.NewTimer(delay)
			select {
			case <-parent.Done():
				timer.Stop()
				return
			case <-timer.C:
				a.runSchedulerOnce(parent, "daily")
			}
		}
	}()
}

func (a *API) schedulerLoop(parent context.Context, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	a.runSchedulerOnce(parent, "startup_backfill")
	for {
		select {
		case <-parent.Done():
			return
		case <-tick.C:
			a.runSchedulerOnce(parent, "tick")
		}
	}
}

// runSchedulerOnce serializes runs: the "single instance" guarantee is an
// in-process mutex; combined with the idempotent biz_key this makes repeated
// or overlapping executions harmless.
func (a *API) runSchedulerOnce(ctx context.Context, trigger string) {
	a.schedMu.Lock()
	defer a.schedMu.Unlock()
	if _, err := a.runDailyProjection(ctx, trigger); err != nil {
		slog.Error("api: scheduler run failed", "trigger", trigger, "error", err)
	}
}

// GET /api/v1/schedule/runs lists recent projection runs (traceability).
func (a *API) handleListScheduleRuns(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(),
		("SELECT run_on,`trigger`,horizon_days,`generated`,status,detail,created_at FROM schedule_runs " +
			"WHERE household_id=? ORDER BY created_at DESC LIMIT 50"), p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	runs := []map[string]any{}
	for rows.Next() {
		var runOn, trigger, status, created string
		var horizon, generated int
		var detail *string
		if err := rows.Scan(&runOn, &trigger, &horizon, &generated, &status, &detail, &created); err != nil {
			internalErr(w, err)
			return
		}
		runs = append(runs, map[string]any{
			"run_on": runOn, "trigger": trigger, "horizon_days": horizon,
			"generated": generated, "status": status, "detail": detail, "created_at": created,
		})
	}
	writeJSON(w, 200, map[string]any{"runs": runs, "total": len(runs)})
}
