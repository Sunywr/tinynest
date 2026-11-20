package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

const provisionItem = "statement_provision"

func mailSummaryTarget(item everbrightPreviewStatement) *money.Minor {
	if item.Amounts == nil {
		return nil
	}
	if item.UseAmountDue {
		return item.Amounts.AmountDue
	}
	return item.Amounts.NewCharges
}

func (a *API) postCreditMailSummaries(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	card := r.PathValue("id")
	ctx := r.Context()
	var input struct {
		SynchronizedAt string `json:"synchronized_at"`
		From           string `json:"from"`
		To             string `json:"to"`
	}
	if decode(r, &input) != nil || input.SynchronizedAt == "" {
		writeErr(w, 400, "invalid_preview", "请先同步账单")
		return
	}
	if input.From != "" || input.To != "" {
		start, startErr := time.Parse(time.DateOnly, input.From)
		end, endErr := time.Parse(time.DateOnly, input.To)
		if startErr != nil || endErr != nil || end.Before(start) {
			writeErr(w, 400, "invalid_statement_range", "请选择有效的账单日期范围")
			return
		}
	}
	var state savedCreditPreview
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var id string
		query := `SELECT id FROM credit_instruments WHERE id=? AND household_id=? AND kind='credit_card' AND status='active'`
		if a.DB.IsMySQL() {
			query += " FOR UPDATE"
		}
		if err := tx.QueryRowContext(ctx, query, card, p.HouseholdID).Scan(&id); err != nil {
			return err
		}
		var err error
		state, err = readSavedCreditPreview(ctx, tx, p.HouseholdID, card)
		if err != nil {
			return err
		}
		if state.SynchronizedAt != input.SynchronizedAt {
			return errConflict
		}
		if err := a.applyMailProvisions(tx, r, p, card, state.Statements, input.From, input.To); err != nil {
			return err
		}
		return a.writeSavedCreditPreview(ctx, tx, p.HouseholdID, card, state)
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "missing_card", "信用卡不存在")
	case errors.Is(err, errConflict):
		writeErr(w, 409, "preview_changed", "预览已更新，请重新打开后入账")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]bool{"saved": true})
	}
}

func (a *API) applyMailProvisions(tx *sql.Tx, r *http.Request, p Principal, card string, statements []everbrightPreviewStatement, from, to string) error {
	// A conflicting message for the same period requires review.
	seen := make(map[string]int64)
	conflict := make(map[string]bool)
	for _, item := range statements {
		target := mailSummaryTarget(item)
		if target == nil {
			continue
		}
		day, amount := item.Amounts.StatementDate, int64(*target)
		if previous, exists := seen[day]; exists && previous != amount {
			conflict[day] = true
		}
		seen[day] = amount
	}
	ctx := r.Context()
	for i := range statements {
		item := &statements[i]
		if item.Provision != "eligible" {
			continue
		}
		amounts := item.Amounts
		requested := mailSummaryTarget(*item)
		if amounts != nil && amounts.StatementDate != "" && from != "" &&
			(amounts.StatementDate < from || amounts.StatementDate > to) {
			continue
		}
		item.Provision = "review"
		if requested == nil || *requested < 0 || amounts.StatementDate == "" ||
			amounts.StatementDate > a.businessDate() || conflict[amounts.StatementDate] {
			continue
		}
		id, err := a.ensureStatement(r, tx, p, card, amounts.StatementDate, true)
		if errors.Is(err, errConflict) {
			continue
		}
		if err != nil {
			return err
		}
		query := `SELECT target_minor FROM credit_statement_provisions WHERE statement_id=? AND household_id=?`
		if a.DB.IsMySQL() {
			query += " FOR UPDATE"
		}
		var target int64
		err = tx.QueryRowContext(ctx, query, id, p.HouseholdID).Scan(&target)
		if err == nil {
			if target == int64(*requested) {
				item.Provision = "recorded"
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO credit_statement_provisions(statement_id,household_id,target_minor)
			VALUES(?,?,?)`, id, p.HouseholdID, *requested); err != nil {
			return err
		}
		if amounts.AmountDue != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE credit_statements SET platform_amount_minor=COALESCE(platform_amount_minor,?)
				WHERE id=? AND household_id=?`, *amounts.AmountDue, id, p.HouseholdID); err != nil {
				return err
			}
		}
		if err := a.rebalanceStatementProvision(tx, r, p, id); err != nil {
			return err
		}
		if err := audit(ctx, tx, p, "auto_statement_provision", "credit_statement", id, "{}"); err != nil {
			return err
		}
		item.Provision = "recorded"
	}
	return nil
}

func (a *API) handleStatementProvision(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var input struct {
		Day    string `json:"statement_date"`
		Target *int64 `json:"new_charges_minor"`
		Due    *int64 `json:"amount_due_minor"`
	}
	if decode(r, &input) != nil || input.Target == nil || *input.Target < 0 || (input.Due != nil && *input.Due < 0) {
		writeErr(w, 400, "invalid_amount", "请填写非负的本期新增及应还金额")
		return
	}
	day, err := time.Parse(time.DateOnly, input.Day)
	if err != nil || day.Format(time.DateOnly) != input.Day || input.Day > a.businessToday().Format(time.DateOnly) {
		writeErr(w, 400, "invalid_statement_day", "请填写有效的账单日")
		return
	}
	var statement string
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		card := r.PathValue("id")
		statement, err = a.ensureStatement(r, tx, p, card, input.Day, true)
		if err != nil {
			return err
		}
		insert := `INSERT INTO credit_statement_provisions(statement_id,household_id,target_minor)
   VALUES(?,?,?) ON CONFLICT(statement_id) DO UPDATE SET target_minor=excluded.target_minor`
		if a.DB.IsMySQL() {
			insert = `INSERT INTO credit_statement_provisions(statement_id,household_id,target_minor)
   VALUES(?,?,?) ON DUPLICATE KEY UPDATE target_minor=VALUES(target_minor)`
		}
		if _, err := tx.ExecContext(r.Context(), insert, statement, p.HouseholdID, *input.Target); err != nil {
			return err
		}
		if input.Due != nil {
			if _, err := tx.ExecContext(r.Context(), `UPDATE credit_statements SET platform_amount_minor=? WHERE id=? AND household_id=?`, *input.Due, statement, p.HouseholdID); err != nil {
				return err
			}
		}
		if err := a.rebalanceStatementProvision(tx, r, p, statement); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "set_statement_provision", "credit_statement", statement, "{}")
	})
	if errors.Is(err, errConflict) {
		writeErr(w, 409, "statement_day_mismatch", "账单日与信用卡设置的账期结束日不一致")
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "missing_statement", "账单不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"statement_id": statement})
}

// rebalanceStatementProvision posts the uncovered portion of the confirmed
// new charges. It must run in the same transaction as each detail change.
func (a *API) rebalanceStatementProvision(tx *sql.Tx, r *http.Request, p Principal, statement string) error {
	ctx := r.Context()
	var target, current, covered int64
	var source, instrument, day string
	q := `SELECT x.target_minor,x.provision_minor,x.entry_id,s.instrument_id,s.cycle_end_on
 FROM credit_statement_provisions x JOIN credit_statements s ON s.id=x.statement_id AND s.household_id=x.household_id
 WHERE x.statement_id=? AND x.household_id=?`
	if a.DB.IsMySQL() {
		q += " FOR UPDATE"
	}
	err := tx.QueryRowContext(ctx, q, statement, p.HouseholdID).Scan(&target, &current, &source, &instrument, &day)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(i.amount_minor),0) FROM credit_statement_items i
 JOIN credit_statements s ON s.id=i.statement_id AND s.household_id=?
 JOIN transactions t ON t.id=i.source_id AND t.household_id=s.household_id AND t.status='posted'
 WHERE i.statement_id=? AND i.source_type='transaction' AND t.type='expense' AND t.credit_instrument_id=s.instrument_id`, p.HouseholdID, statement).Scan(&covered)
	if err != nil {
		return err
	}
	remaining := max(0, target-covered)
	if current == remaining {
		return nil
	}
	category := "cat_" + p.HouseholdID + "_statement_provision"
	account := ledger.ExpenseSubject(category)
	if source != "" {
		var originalDay string
		if err := tx.QueryRowContext(ctx, `SELECT occurred_on FROM journal_entries WHERE id=? AND household_id=?`, source, p.HouseholdID).Scan(&originalDay); err != nil {
			return err
		}
		entry, err := ledger.New(newID("je"), "reversal", statement, originalDay,
			ledger.Line{Account: account, Credit: money.Minor(current)},
			ledger.Line{Account: ledger.CreditSubject(instrument), Debit: money.Minor(current)})
		if err != nil {
			return err
		}
		entry.ReversalOf = source
		if err := db.SaveEntry(ctx, tx, entry, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE journal_entries SET status='reversed' WHERE id=? AND household_id=?`, source, p.HouseholdID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO credit_statement_items(statement_id,source_type,source_id,amount_minor,occurred_on)
   VALUES(?,?,?,?,?)`, statement, provisionItem, entry.ID, -current, originalDay); err != nil {
			return err
		}
	}
	entryID := ""
	if remaining > 0 {
		categoryInsert := `INSERT INTO categories(id,household_id,kind,name,level,active) VALUES(?,?,'expense','账单暂记',1,1) ON CONFLICT(id) DO NOTHING`
		if a.DB.IsMySQL() {
			categoryInsert = `INSERT INTO categories(id,household_id,kind,name,level,active) VALUES(?,?,'expense','账单暂记',1,1) ON DUPLICATE KEY UPDATE id=id`
		}
		if _, err := tx.ExecContext(ctx, categoryInsert, category, p.HouseholdID); err != nil {
			return err
		}
		entry, err := ledger.New(newID("je"), provisionItem, statement, day,
			ledger.Line{Account: account, Debit: money.Minor(remaining)},
			ledger.Line{Account: ledger.CreditSubject(instrument), Credit: money.Minor(remaining)})
		if err != nil {
			return err
		}
		if err := db.SaveEntry(ctx, tx, entry, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		entryID = entry.ID
		if _, err := tx.ExecContext(ctx, `INSERT INTO credit_statement_items(statement_id,source_type,source_id,amount_minor,occurred_on)
   VALUES(?,?,?,?,?)`, statement, provisionItem, entryID, remaining, day); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE credit_statement_provisions SET provision_minor=?,entry_id=? WHERE statement_id=? AND household_id=?`, remaining, entryID, statement, p.HouseholdID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE credit_statements SET system_due_minor=(SELECT COALESCE(SUM(amount_minor),0)
  FROM credit_statement_items WHERE statement_id=?) WHERE id=? AND household_id=?`, statement, statement, p.HouseholdID); err != nil {
		return err
	}
	return a.refreshPendingStatementAmount(ctx, tx, p.HouseholdID, statement)
}
