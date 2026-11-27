package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"tinynest/api/internal/db"
)

// card_statements holds what the BANK reported for one closing period. The
// ledger stays the only accounting fact: nothing here writes transactions or
// journal entries, and the credit repayment plan never reads these rows.
// ledger_charge_minor is derived from the journal for the same period so the
// two views can be compared without either one rewriting the other.
const cardStatementColumns = `
	SELECT s.id, s.account_id, a.name, s.period_start_on, s.period_end_on, s.due_on,
	       s.currency, s.amount_due_minor, s.status, s.review_reason,
	       (SELECT COUNT(*) FROM card_statement_sources src
		  WHERE src.household_id=s.household_id AND src.statement_id=s.id) AS source_count,
	       CASE WHEN s.period_start_on IS NULL THEN NULL ELSE (
			SELECT COALESCE(SUM(l.credit_minor-l.debit_minor),0)
			FROM journal_entries e
			JOIN journal_lines l ON l.entry_id=e.id
			JOIN transactions t ON t.id=e.biz_id AND t.household_id=e.household_id
			WHERE e.household_id=s.household_id AND l.account_code=a.ledger_code
			  AND e.occurred_on>=s.period_start_on AND e.occurred_on<=s.period_end_on
			  AND e.status='posted' AND e.reversal_of IS NULL AND e.biz_type='expense'
			  AND t.status='posted' AND t.type='expense' AND t.account_id=s.account_id
	       ) END AS ledger_charge_minor,
	       pt.id AS placeholder_transaction_id,
	       pt.status AS placeholder_status
	FROM card_statements s
	JOIN accounts a ON a.household_id=s.household_id AND a.id=s.account_id
	-- At most one placeholder row per statement is enforced by the primary key, so
	-- this join cannot multiply the list.
	LEFT JOIN statement_placeholders ph ON ph.household_id=s.household_id AND ph.statement_id=s.id
	LEFT JOIN transactions pt ON pt.household_id=ph.household_id AND pt.id=ph.transaction_id
	WHERE s.household_id=?
	ORDER BY s.period_end_on DESC, a.name ASC, s.id ASC`

func (a *API) handleListCardStatements(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), cardStatementColumns, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, accountID, accountName, periodEnd, currency, status, reviewReason string
		var amount, sourceCount int64
		var periodStart, dueOn, ledgerCharge sql.NullString
		var placeholderID, placeholderStatus sql.NullString
		if err := rows.Scan(&id, &accountID, &accountName, &periodStart, &periodEnd, &dueOn,
			&currency, &amount, &status, &reviewReason, &sourceCount, &ledgerCharge,
			&placeholderID, &placeholderStatus); err != nil {
			internalErr(w, err)
			return
		}
		row := map[string]any{
			"id": id, "account_id": accountID, "account_name": accountName,
			"period_start_on": nullable(periodStart), "period_end_on": periodEnd,
			"due_on": nullable(dueOn), "currency": currency, "amount_due_minor": amount,
			"status": status, "review_reason": reviewReason, "source_count": sourceCount,
			"ledger_charge_minor": nil, "difference_minor": nil,
			"placeholder_transaction_id": nullable(placeholderID),
			"placeholder_state":          placeholderState(placeholderStatus),
		}
		if ledgerCharge.Valid {
			charge, err := strconv.ParseInt(ledgerCharge.String, 10, 64)
			if err != nil {
				internalErr(w, err)
				return
			}
			row["ledger_charge_minor"] = charge
			row["difference_minor"] = amount - charge
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func nullable(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}

// placeholderState tells the bill view whether a 待补明细 transaction still stands
// in for the missing details, so the operator can see a reversed placeholder that
// was never replaced instead of an unexplained gap.
func placeholderState(status sql.NullString) string {
	if !status.Valid {
		return ""
	}
	if status.String == "posted" {
		return "awaiting_detail"
	}
	return "reversed"
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// statementSummaryReady is the only shape allowed into card_statements: a
// recognised closing date, a non-negative CNY amount and no review flag.
func statementSummaryReady(item statementMailPreview) bool {
	return item.NeedsReview == "" && item.StatementOn != "" &&
		item.AmountDue != nil && *item.AmountDue >= 0
}

// The digest pins one mailbox message to the card and period it describes, so a
// replayed mail is recognised without trusting the volatile IMAP UID.
func statementSourceDigest(target statementReadTarget, item statementMailPreview) string {
	sum := sha256.New()
	fmt.Fprintf(sum, "email\n%s\n%s\n%d\n%s\n%s\n%s\n%s\n%d\nCNY",
		target.provider, target.userID, item.UID, target.accountID,
		item.StatementOn, item.DueOn, target.tail, int64(*item.AmountDue))
	return hex.EncodeToString(sum.Sum(nil))
}

func (a *API) handleStatementSync(w http.ResponseWriter, r *http.Request) {
	provider := statementMailboxProvider(w, r)
	if provider == "" {
		return
	}
	version, ok := statementRequestVersion(w, r)
	if !ok {
		return
	}
	target, ok := a.prepareStatementRead(w, r, provider, version)
	if !ok {
		return
	}
	if !reserveStatementSlot(w) {
		return
	}
	defer func() { <-mailboxProbeSlots }()
	since := a.businessToday().AddDate(0, 0, -45)
	items, ok := a.readStatementMails(w, r, target,
		"statement_sync_failed", "无法读取账单邮件，请稍后重试", since)
	if !ok {
		return
	}

	created, linked, conflicts, reported := 0, 0, 0, []map[string]any{}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		// Re-verify before writing: disabling the mailbox releases the binding
		// and rotating the authorisation code raises the config version.
		config, err := loadStatementMailbox(r.Context(), tx, principalOf(target), target.provider)
		if err != nil {
			return err
		}
		if config.version != target.version || config.cipher == "" {
			return errConflict
		}
		if _, err := a.statementCardName(r.Context(), tx, target); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errConflict
			}
			return err
		}
		statementDay, err := a.statementCardDay(r.Context(), tx, target)
		if err != nil {
			return err
		}
		for _, item := range items {
			result, err := a.storeStatementSummary(r.Context(), tx, target, item, statementDay)
			if err != nil {
				return err
			}
			reported = append(reported, map[string]any{
				"statement_on": item.StatementOn, "due_on": item.DueOn, "result": result,
			})
			switch result {
			case "created":
				created++
			case "linked", "unchanged":
				linked++
			case "conflict":
				conflicts++
			}
		}
		if created+conflicts > 0 {
			return audit(r.Context(), tx, principalOf(target), "sync_card_statement",
				"card_statement", target.accountID, fmt.Sprintf(
					`{"created":%d,"linked":%d,"conflicts":%d}`, created, linked, conflicts))
		}
		return nil
	})
	if err != nil {
		writeStatementMailboxError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"since": since.Format(time.DateOnly), "created": created, "linked": linked,
		"conflicts": conflicts, "needs_review": len(items) - created - linked - conflicts,
		"items": reported,
	})
}

func principalOf(target statementReadTarget) Principal {
	return Principal{HouseholdID: target.householdID, UserID: target.userID}
}

// storeStatementSummary records one recognised bank summary. An existing period
// keeps the amount first reported; a later disagreement only fills review_reason.
func (a *API) storeStatementSummary(ctx context.Context, tx *sql.Tx, target statementReadTarget,
	item statementMailPreview, statementDay int) (string, error) {
	if !statementSummaryReady(item) {
		return "needs_review", nil
	}
	amount := int64(*item.AmountDue)
	periodStart := item.PeriodStart
	if periodStart == "" {
		periodStart = statementPeriodStart(item.StatementOn, statementDay)
	}
	var statementID string
	var recordedAmount int64
	created := false
	err := tx.QueryRowContext(ctx, `SELECT id,amount_due_minor FROM card_statements
		WHERE household_id=? AND account_id=? AND period_end_on=?`,
		target.householdID, target.accountID, item.StatementOn).Scan(&statementID, &recordedAmount)
	if errors.Is(err, sql.ErrNoRows) {
		statementID = newID("stmt")
		if _, err := tx.ExecContext(ctx, `INSERT INTO card_statements
			(id,household_id,account_id,period_start_on,period_end_on,due_on,currency,
			 amount_due_minor,status,review_reason,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			statementID, target.householdID, target.accountID,
			nullableText(periodStart),
			item.StatementOn, nullableText(item.DueOn), "CNY", amount,
			"pending_review", "", a.nowUTC(), a.nowUTC()); err != nil {
			return "", err
		}
		created = true
		recordedAmount = amount
	} else if err != nil {
		return "", err
	}
	if err := a.recordStatementSource(ctx, tx, target, item, statementID); err != nil {
		return "", err
	}
	if created {
		return "created", nil
	}
	if recordedAmount != amount {
		_, err := tx.ExecContext(ctx, `UPDATE card_statements SET review_reason=?,updated_at=?
			WHERE household_id=? AND id=? AND review_reason=''`,
			"银行账单金额与已记录金额不一致，请核对", a.nowUTC(), target.householdID, statementID)
		if err != nil {
			return "", err
		}
		return "conflict", nil
	}
	return "linked", nil
}

// recordStatementSource stores the mailbox evidence once per digest, so replaying
// the same message changes nothing and adds no audit row.
func (a *API) recordStatementSource(ctx context.Context, tx *sql.Tx, target statementReadTarget,
	item statementMailPreview, statementID string) error {
	digest := statementSourceDigest(target, item)
	var seen int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM card_statement_sources
		WHERE household_id=? AND digest=?`, target.householdID, digest).Scan(&seen); err != nil {
		return err
	}
	if seen > 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO card_statement_sources
		(id,household_id,statement_id,kind,provider,user_id,uid,received_on,digest,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		newID("srcs"), target.householdID, statementID, "email", target.provider, target.userID,
		item.UID, nullableText(item.ReceivedOn), digest, a.nowUTC())
	return err
}

func statementPeriodStart(periodEnd string, statementDay int) string {
	end, err := time.Parse(time.DateOnly, periodEnd)
	if err != nil || statementDay < 1 {
		return ""
	}
	return previousStatementClose(end, statementDay).AddDate(0, 0, 1).Format(time.DateOnly)
}

func (a *API) statementCardName(ctx context.Context, q db.Queryer, target statementReadTarget) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `
		SELECT a.name FROM accounts a
		JOIN statement_mailbox_card_links b ON b.household_id=a.household_id AND b.account_id=a.id
		JOIN users u ON u.household_id=b.household_id AND u.id=b.user_id
		WHERE a.household_id=? AND a.id=? AND a.kind='credit' AND a.status='active'
		  AND b.user_id=? AND b.provider=? AND u.status='active' AND u.role IN ('admin','member')`,
		target.householdID, target.accountID, target.userID, target.provider).Scan(&name)
	return name, err
}

func (a *API) statementCardDay(ctx context.Context, tx *sql.Tx, target statementReadTarget) (int, error) {
	var statementDay sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT statement_day FROM accounts
		WHERE household_id=? AND id=?`, target.householdID, target.accountID).Scan(&statementDay)
	if err != nil {
		return 0, err
	}
	return int(statementDay.Int64), nil
}
