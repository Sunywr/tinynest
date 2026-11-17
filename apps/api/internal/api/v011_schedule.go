package api

import (
	"context"
	"database/sql"
	"errors"
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
	runOn := a.businessDate()
	var founds []dueReminder
	add := func(kind, id, due, label string, remaining int64) {
		if remaining > 0 {
			founds = append(founds, dueReminder{kind: kind, id: id, due: due, label: label, remaining: remaining})
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
		if err := rows.Err(); err != nil {
			return err
		}
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
			if err := rows.Err(); err != nil {
				return err
			}
		}
		insertDue := "INSERT OR IGNORE INTO schedule_due_items(id,household_id,source_type,source_id,biz_key,due_on,amount_minor,status,source_label) VALUES(?,?,?,?,?,?,?,'pending',?)"
		if a.DB.IsMySQL() {
			insertDue = "INSERT INTO schedule_due_items(id,household_id,source_type,source_id,biz_key,due_on,amount_minor,status,source_label) VALUES(?,?,?,?,?,?,?,'pending',?) ON DUPLICATE KEY UPDATE id=id"
		}
		for _, f := range founds {
			if a.DB.IsMySQL() && f.kind == "credit_statement" {
				// A settlement or archive can race the earlier snapshot.
				err := tx.QueryRowContext(ctx, `
					SELECT due_on,system_due_minor-paid_minor FROM credit_statements
					WHERE household_id=? AND id=? AND system_due_minor>paid_minor AND due_on<=?
					FOR SHARE`, householdID, f.id, horizon).Scan(&f.due, &f.remaining)
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				if err != nil {
					return err
				}
			}
			if a.DB.IsMySQL() && (f.kind == "debt" || f.kind == "installment") {
				periodTable, contractTable := "debt_schedule_items", "debt_contracts"
				if f.kind == "installment" {
					periodTable, contractTable = "installment_schedule_items", "installment_contracts"
				}
				// Recheck the current period under a shared lock. A contract
				// deletion that raced the snapshot either removes the newly
				// projected item or cancels the period before this read.
				err := tx.QueryRowContext(ctx, `
					SELECT total_minor-paid_minor FROM `+periodTable+`
					WHERE id=? AND status IN ('pending','partial','overdue')
					AND contract_id IN (SELECT id FROM `+contractTable+` WHERE household_id=?)
					FOR SHARE`, f.id, householdID).Scan(&f.remaining)
				if errors.Is(err, sql.ErrNoRows) || (err == nil && f.remaining <= 0) {
					continue
				}
				if err != nil {
					return err
				}
			}
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
			newID("run"), householdID, runOn, trigger, horizonDays, generated); err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		return generated, nil
	}
	slog.Error("api: due-item projection failed", "household", householdID, "trigger", trigger, "error", err)
	if _, recordErr := a.DB.ExecContext(ctx, `
		INSERT INTO schedule_runs(id,household_id,run_on,`+"`trigger`"+`,horizon_days,`+"`generated`"+`,status,detail)
		VALUES(?,?,?,?,?,0,'failed',?)`,
		newID("run"), householdID, runOn, trigger, horizonDays, "待办生成失败，详情见服务日志"); recordErr != nil {
		slog.Error("api: could not record failed projection", "household", householdID, "trigger", trigger, "error", recordErr)
	}
	return 0, err
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
	if err := rows.Err(); err != nil {
		return 0, err
	}
	total := 0
	var failures []error
	for _, id := range ids {
		n, err := a.projectDueItems(ctx, id, trigger, 30)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		total += n
	}
	return total, errors.Join(failures...)
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
	if _, err := a.runDailyProjection(ctx, trigger); err == nil {
		return
	} else {
		slog.Error("api: scheduler run failed", "trigger", trigger, "error", err)
	}
	delay := time.Minute
	if a.opts.SchedulerInterval > 0 {
		delay = a.opts.SchedulerInterval
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	if ctx.Err() == nil {
		if _, err := a.runDailyProjection(ctx, "retry"); err != nil {
			slog.Error("api: scheduler retry failed", "trigger", trigger, "error", err)
		}
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

type dueReminder struct {
	kind, id, due, label string
	remaining            int64
}

func (a *API) restoreStatementReminder(ctx context.Context, tx *sql.Tx, householdID, statementID string) error {
	query := `SELECT s.due_on,s.cycle_end_on,s.system_due_minor,s.paid_minor,s.status,
		(SELECT name FROM credit_instruments WHERE household_id=s.household_id AND id=s.instrument_id)
		FROM credit_statements s WHERE s.household_id=? AND s.id=?`
	if a.DB.IsMySQL() {
		query += " FOR UPDATE"
	}
	var due, cycleEnd, stored, name string
	var systemDue, paid int64
	if err := tx.QueryRowContext(ctx, query, householdID, statementID).Scan(&due, &cycleEnd, &systemDue, &paid, &stored, &name); err != nil {
		return err
	}
	remaining := systemDue - paid
	if stored == "archived" && remaining != 0 {
		status := deriveStatementStatus(cycleEnd, due, a.businessDate(), systemDue, paid)
		if _, err := tx.ExecContext(ctx, `UPDATE credit_statements SET status=? WHERE household_id=? AND id=?`,
			status, householdID, statementID); err != nil {
			return err
		}
	}
	return a.restoreDueReminder(ctx, tx, householdID, dueReminder{
		kind: "credit_statement", id: statementID, due: due, label: name + " 账单", remaining: remaining,
	})
}

func (a *API) restoreDueReminder(ctx context.Context, tx *sql.Tx, householdID string, item dueReminder) error {
	if item.remaining <= 0 {
		_, err := tx.ExecContext(ctx, `DELETE FROM schedule_due_items
			WHERE household_id=? AND source_type=? AND source_id=? AND status='pending'`,
			householdID, item.kind, item.id)
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE schedule_due_items SET amount_minor=?,status='pending',due_on=?
		WHERE household_id=? AND source_type=? AND source_id=? AND status IN ('pending','confirmed')`,
		item.remaining, item.due, householdID, item.kind, item.id); err != nil {
		return err
	}
	if item.due > a.businessToday().AddDate(0, 0, 30).Format("2006-01-02") {
		return nil
	}
	insert := `INSERT INTO schedule_due_items(id,household_id,source_type,source_id,biz_key,due_on,amount_minor,status,source_label)
		VALUES(?,?,?,?,?,?,?,'pending',?)`
	if a.DB.IsMySQL() {
		insert += " ON DUPLICATE KEY UPDATE id=id"
	} else {
		insert += " ON CONFLICT(biz_key) DO NOTHING"
	}
	_, err := tx.ExecContext(ctx, insert, newID("due"), householdID, item.kind, item.id, householdID+":"+item.kind+":"+item.id,
		item.due, item.remaining, item.label)
	return err
}
