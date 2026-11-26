package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tinynest/api/internal/db"
	"tinynest/api/internal/money"
)

// 待补明细 is the placeholder for a household that lets the bank statement arrive
// but never uploads the card details. One posted expense carries the amount the
// BANK reported as due for a closed cycle and is dated at the reported repayment
// deadline, so the read-only repayment plan reserves that cash before the money
// leaves instead of discovering the bill afterwards.
//
// It is an ordinary transaction on purpose: balances, statistics and the
// reversal machinery keep working unchanged. What it does not claim is to know
// what was bought, so it lives in its own 待补明细 category and is replaced by the
// real details (or by a reversal) once they arrive. Amounts and dates are always
// read back from the stored bank summary, never taken from the request body.

const (
	placeholderCategoryName  = "待补明细"
	placeholderCategoryCode  = "pending"
	placeholderCategoryEmoji = "receipt"
	placeholderCategoryColor = "6"
	placeholderKeyPrefix     = "statement-placeholder:"
	placeholderAuditAction   = "create_statement_placeholder"
)

type statementPlaceholderRequest struct {
	StatementID string `json:"statement_id"`
}

// statementPlaceholderSource is the bank summary as stored: the reported period,
// the reported deadline (possibly absent) and the reported amount due.
type statementPlaceholderSource struct {
	statementID string
	periodStart string
	periodEnd   string
	reportedDue string
	amountMinor int64
}

type placeholderLink struct {
	transactionID string
	status        string
	found         bool
}

type placeholderRefusal struct {
	code    string
	message string
}

// Unresolvable refusals are reported inside the write transaction so a
// concurrent detail import cannot slip between the check and the insert.
var (
	errPlaceholderCycleRecorded = errors.New("api: statement cycle already recorded")
	errPlaceholderExists        = errors.New("api: statement placeholder already exists")
)

func (a *API) handleStatementPlaceholder(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	accountID := r.PathValue("id")
	var in statementPlaceholderRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	statementID := strings.TrimSpace(in.StatementID)
	if statementID == "" {
		writeErr(w, 400, "invalid_request", "缺少 statement_id")
		return
	}
	account, err := loadActiveAccount(r.Context(), a.DB, p.HouseholdID, accountID)
	if err != nil || account.Kind != "credit" {
		writeErr(w, 404, "not_found", "账户不存在或已停用")
		return
	}
	source, err := a.loadStatementForPlaceholder(r.Context(), p.HouseholdID, accountID, statementID)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "账单记录不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	if link, err := a.loadPlaceholderLink(r.Context(), a.DB, p.HouseholdID, statementID); err != nil {
		internalErr(w, err)
		return
	} else if link.found {
		writePlaceholderLink(w, link)
		return
	}
	dueOn, refusal := placeholderPlan(source, account, a.businessToday())
	if refusal.code != "" {
		writeErr(w, http.StatusConflict, refusal.code, refusal.message)
		return
	}
	recorded, err := a.statementCycleCharge(r.Context(), a.DB, p.HouseholdID, account, source)
	if err != nil {
		internalErr(w, err)
		return
	}
	if recorded != 0 {
		writeErr(w, http.StatusConflict, "cycle_already_recorded", placeholderRecordedMessage(recorded))
		return
	}

	transactionID, entryID, key := newID("tx"), newID("je"), placeholderKey(statementID)
	note := placeholderNote(source, dueOn)
	values := transactionValues{
		Type:       "expense",
		Amount:     money.Minor(source.amountMinor),
		OccurredOn: dueOn,
		AccountID:  &accountID,
		Note:       &note,
	}
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		// Re-check the two facts that make this write safe after the tx lock is
		// held: the statement may have been synchronised again, and the same
		// request may have been replayed while the checks above were running.
		link, err := a.loadPlaceholderLink(r.Context(), tx, p.HouseholdID, statementID)
		if err != nil {
			return err
		}
		if link.found {
			return errPlaceholderExists
		}
		charge, err := a.statementCycleCharge(r.Context(), tx, p.HouseholdID, account, source)
		if err != nil {
			return err
		}
		if charge != 0 {
			recorded = charge
			return errPlaceholderCycleRecorded
		}
		categoryID, err := a.placeholderCategory(r.Context(), tx, p.HouseholdID)
		if err != nil {
			return err
		}
		values.CategoryID = &categoryID
		entry, err := a.buildTransactionEntry(r.Context(), tx, p.HouseholdID, values, transactionID, entryID)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, entry, p.HouseholdID, key, p.Username); err != nil {
			return err
		}
		if err := insertTransactionRow(r.Context(), tx, p.HouseholdID, values, transactionID, entryID, key, p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO statement_placeholders
			(household_id,statement_id,transaction_id,period_start_on,period_end_on,due_on,created_at)
			VALUES(?,?,?,?,?,?,?)`,
			p.HouseholdID, statementID, transactionID, nullableText(source.periodStart),
			source.periodEnd, dueOn, a.nowUTC()); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, placeholderAuditAction, "transaction", transactionID,
			fmt.Sprintf(`{"statement_id":%q,"amount_minor":%d,"due_on":%q}`, statementID, source.amountMinor, dueOn))
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, map[string]any{
			"transaction_id": transactionID, "statement_id": statementID,
			"account_id": accountID, "account_name": account.Name,
			"amount_minor": source.amountMinor, "currency": "CNY",
			"occurred_on": dueOn, "due_on": dueOn,
			"period_start_on": nullableText(source.periodStart), "period_end_on": source.periodEnd,
			"category_id": *values.CategoryID, "category_name": placeholderCategoryName,
		})
	case errors.Is(err, errPlaceholderCycleRecorded):
		writeErr(w, http.StatusConflict, "cycle_already_recorded", placeholderRecordedMessage(recorded))
	case errors.Is(err, errPlaceholderExists):
		a.writePlaceholderConflict(w, r, p.HouseholdID, statementID)
	default:
		// A unique-constraint loss against a concurrent duplicate still means
		// "already placed", not "broken"; re-read after the rollback to be sure.
		if link, lookupErr := a.loadPlaceholderLink(r.Context(), a.DB, p.HouseholdID, statementID); lookupErr == nil && link.found {
			a.writePlaceholderConflict(w, r, p.HouseholdID, statementID)
			return
		}
		internalErr(w, err)
	}
}

// loadStatementForPlaceholder reads one stored bank summary, scoped to the
// household AND the card in the path, so another household's statement id and
// another card of the same household both answer 404.
func (a *API) loadStatementForPlaceholder(ctx context.Context, householdID, accountID, statementID string) (statementPlaceholderSource, error) {
	var out statementPlaceholderSource
	var periodStart, dueOn sql.NullString
	err := a.DB.QueryRowContext(ctx, `
		SELECT id, period_start_on, period_end_on, due_on, amount_due_minor
		FROM card_statements WHERE household_id=? AND account_id=? AND id=?`,
		householdID, accountID, statementID).Scan(
		&out.statementID, &periodStart, &out.periodEnd, &dueOn, &out.amountMinor)
	out.periodStart, out.reportedDue = periodStart.String, dueOn.String
	return out, err
}

func (a *API) loadPlaceholderLink(ctx context.Context, q db.Queryer, householdID, statementID string) (placeholderLink, error) {
	var out placeholderLink
	var status string
	err := q.QueryRowContext(ctx, `
		SELECT p.transaction_id, t.status FROM statement_placeholders p
		JOIN transactions t ON t.household_id=p.household_id AND t.id=p.transaction_id
		WHERE p.household_id=? AND p.statement_id=?`, householdID, statementID).Scan(&out.transactionID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.status, out.found = status, true
	return out, nil
}

// placeholderPlan decides the booking date and refuses what must not be booked
// yet. A statement whose cycle has not closed has no bill to reserve, and a
// placeholder without a deadline would be an amount with no schedule.
func placeholderPlan(source statementPlaceholderSource, account accountInfo, today time.Time) (string, placeholderRefusal) {
	if source.periodEnd > today.Format(time.DateOnly) {
		return "", placeholderRefusal{"cycle_not_closed",
			"账期尚未结束（账单日 " + source.periodEnd + "），还没有应还账单可以占位"}
	}
	if source.amountMinor <= 0 {
		return "", placeholderRefusal{"amount_unknown", "银行账单没有报出应还金额，无法生成待补流水"}
	}
	if source.reportedDue != "" {
		if _, err := time.Parse(time.DateOnly, source.reportedDue); err != nil {
			return "", placeholderRefusal{"due_date_unknown", "银行报出的还款截止日无法解析，未生成待补流水"}
		}
		return source.reportedDue, placeholderRefusal{}
	}
	if !account.DueDay.Valid || account.DueDay.Int64 < 1 || account.DueDay.Int64 > 31 {
		return "", placeholderRefusal{"due_date_unknown",
			"银行未报还款截止日，账户也没有配置还款日，无法确定待补流水的日期"}
	}
	close, err := time.Parse(time.DateOnly, source.periodEnd)
	if err != nil {
		return "", placeholderRefusal{"due_date_unknown", "账单日期无法解析，未生成待补流水"}
	}
	return dueAfter(close, int(account.DueDay.Int64)).Format(time.DateOnly), placeholderRefusal{}
}

func placeholderKey(statementID string) string { return placeholderKeyPrefix + statementID }

func placeholderNote(source statementPlaceholderSource, dueOn string) string {
	period := source.periodEnd
	if source.periodStart != "" {
		period = source.periodStart + " 至 " + source.periodEnd
	}
	return fmt.Sprintf("待补明细：%s 银行应还 %s，还款截止 %s；明细补录后请冲正本笔",
		period, money.Minor(source.amountMinor).String(), dueOn)
}

func placeholderRecordedMessage(charge int64) string {
	return fmt.Sprintf("该账期已入账 %s，重复占位会双计；请补录明细或只核对差额",
		money.Minor(charge).String())
}

func writePlaceholderLink(w http.ResponseWriter, link placeholderLink) {
	code, message := "placeholder_exists", "这笔账单已经生成过待补明细流水"
	if link.status != "posted" {
		code = "placeholder_reversed"
		message = "这笔账单的待补明细流水已被冲正，请补录明细而不是重复占位"
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":          map[string]string{"code": code, "message": message},
		"transaction_id": link.transactionID,
	})
}

func (a *API) writePlaceholderConflict(w http.ResponseWriter, r *http.Request, householdID, statementID string) {
	link, err := a.loadPlaceholderLink(r.Context(), a.DB, householdID, statementID)
	if err != nil {
		internalErr(w, err)
		return
	}
	if !link.found {
		internalErr(w, errors.New("api: placeholder vanished after write: "+statementID))
		return
	}
	writePlaceholderLink(w, link)
}

// statementCycleCharge totals what the ledger already holds for the billed period
// on this card, so a placeholder never doubles a cycle the details were uploaded
// for. An unknown cycle start is read as "everything up to the closing date",
// which refuses rather than risks double counting.
func (a *API) statementCycleCharge(ctx context.Context, q db.Queryer, householdID string,
	account accountInfo, source statementPlaceholderSource) (int64, error) {
	start := source.periodStart
	if start == "" {
		start = "0001-01-01"
	}
	var charge int64
	err := q.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(l.credit_minor-l.debit_minor),0)
		FROM journal_entries e
		JOIN journal_lines l ON l.entry_id=e.id
		JOIN transactions t ON t.id=e.biz_id AND t.household_id=e.household_id
		WHERE e.household_id=? AND l.account_code=? AND e.occurred_on>=? AND e.occurred_on<=?
		  AND e.status='posted' AND e.reversal_of IS NULL AND e.biz_type='expense'
		  AND t.status='posted' AND t.type='expense' AND t.account_id=?`,
		householdID, account.Code, start, source.periodEnd, account.ID).Scan(&charge)
	return charge, err
}

// placeholderCategory returns the household's 待补明细 expense category, creating
// it on first use so no household gets a silent default category and the ledger
// never files the placeholder under a category that claims to know what was
// bought. The id is deterministic, so racing requests cannot create two; a
// category the user deleted earlier is revived because the placeholder cannot be
// booked without it.
func (a *API) placeholderCategory(ctx context.Context, tx *sql.Tx, householdID string) (string, error) {
	id := "cat_" + householdID + "_" + placeholderCategoryCode
	var active int
	err := tx.QueryRowContext(ctx, `SELECT active FROM categories WHERE household_id=? AND id=?`,
		householdID, id).Scan(&active)
	if err == nil {
		if active != 1 {
			if _, err := tx.ExecContext(ctx, `UPDATE categories SET active=1 WHERE household_id=? AND id=?`,
				householdID, id); err != nil {
				return "", err
			}
		}
		return id, nil
	}
	if !isNoRows(err) {
		return "", err
	}
	var sort int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort),0)+1 FROM categories
		WHERE household_id=? AND kind='expense'`, householdID).Scan(&sort); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO categories(id,household_id,kind,name,sort,emoji,color)
		VALUES(?,?,?,?,?,?,?)`, id, householdID, "expense", placeholderCategoryName,
		sort, placeholderCategoryEmoji, placeholderCategoryColor)
	return id, err
}
