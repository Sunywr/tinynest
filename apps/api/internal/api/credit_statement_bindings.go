package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type statementBindingCard struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Status  string                 `json:"status"`
	Bound   bool                   `json:"bound"`
	Taken   bool                   `json:"taken"`
	Periods []mailboxBillingPeriod `json:"billing_periods,omitempty"`
}

type mailboxBillingPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

func mailboxBillingPeriods(today time.Time, day int64) []mailboxBillingPeriod {
	if day < 1 || day > 31 {
		return nil
	}
	periods := make([]mailboxBillingPeriod, 0, 12)
	end := previousStatementCloseOnOrBefore(today, int(day))
	for range 12 {
		previous := previousStatementClose(end, int(day))
		periods = append(periods, mailboxBillingPeriod{
			Start: previous.AddDate(0, 0, 1).Format(time.DateOnly),
			End:   end.Format(time.DateOnly),
		})
		end = previous
	}
	return periods
}

func (a *API) handleStatementBindingCards(w http.ResponseWriter, r *http.Request) {
	provider := statementMailboxProvider(w, r)
	if provider == "" {
		return
	}
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT a.id,a.name,a.status,b.user_id,b.provider,a.statement_day
		FROM accounts a LEFT JOIN statement_mailbox_card_links b
		  ON b.household_id=a.household_id AND b.account_id=a.id
		WHERE a.household_id=? AND a.kind='credit'
		  AND (a.status='active' OR (a.status='disabled' AND b.user_id=? AND b.provider=?))
		ORDER BY a.name,a.id`, p.HouseholdID, p.UserID, provider)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	cards := []statementBindingCard{}
	today := a.businessToday()
	for rows.Next() {
		var card statementBindingCard
		var owner, boundProvider sql.NullString
		var statementDay sql.NullInt64
		if err := rows.Scan(&card.ID, &card.Name, &card.Status, &owner, &boundProvider, &statementDay); err != nil {
			internalErr(w, err)
			return
		}
		card.Bound = owner.Valid && owner.String == p.UserID && boundProvider.String == provider
		card.Taken = owner.Valid && !card.Bound
		if card.Bound && card.Status == "active" {
			card.Periods = mailboxBillingPeriods(today, statementDay.Int64)
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": cards})
}

var (
	errStatementCardTaken    = errors.New("credit card already bound")
	errStatementCardInactive = errors.New("credit card inactive")
)

func (a *API) handleStatementBinding(w http.ResponseWriter, r *http.Request) {
	provider := statementMailboxProvider(w, r)
	if provider == "" {
		return
	}
	version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || version < 1 {
		writeErr(w, 400, "invalid_request", "请提供邮箱配置版本")
		return
	}
	p, _ := principal(r)
	ctx, accountID := r.Context(), r.PathValue("id")
	bind := r.Method == http.MethodPut
	err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.lockStatementMailboxUser(ctx, tx, p); err != nil {
			return err
		}
		config, err := loadStatementMailbox(ctx, tx, p, provider)
		if err != nil {
			return err
		}
		if config.cipher == "" || config.version != version {
			return errConflict
		}
		lock := ""
		if a.DB.IsMySQL() {
			lock = " FOR UPDATE"
		}
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM accounts
			WHERE household_id=? AND id=? AND kind='credit' AND status<>'deleted'`+lock,
			p.HouseholdID, accountID).Scan(&status); err != nil {
			return err
		}
		if bind && status != "active" {
			return errStatementCardInactive
		}
		var owner, boundProvider string
		err = tx.QueryRowContext(ctx, `SELECT user_id,provider FROM statement_mailbox_card_links
			WHERE household_id=? AND account_id=?`+lock, p.HouseholdID, accountID).Scan(&owner, &boundProvider)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if owner != "" && (owner != p.UserID || boundProvider != provider) {
			if bind {
				return errStatementCardTaken
			}
			return sql.ErrNoRows
		}
		if bind == (owner != "") {
			return nil
		}
		action := "unbind_statement_card"
		if bind {
			action = "bind_statement_card"
			_, err = tx.ExecContext(ctx, `INSERT INTO statement_mailbox_card_links(household_id,account_id,user_id,provider) VALUES(?,?,?,?)`,
				p.HouseholdID, accountID, p.UserID, provider)
		} else {
			_, err = tx.ExecContext(ctx, `DELETE FROM statement_mailbox_card_links WHERE household_id=? AND account_id=? AND user_id=? AND provider=?`,
				p.HouseholdID, accountID, p.UserID, provider)
		}
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, action, "account", accountID, "")
	})
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "信用卡或绑定不存在")
	case errors.Is(err, errStatementCardTaken):
		writeErr(w, 409, "card_already_bound", "这张信用卡已绑定其他账单邮箱")
	case errors.Is(err, errStatementCardInactive):
		writeErr(w, 400, "invalid_request", "仅可绑定启用中的信用卡")
	default:
		writeStatementMailboxError(w, err)
	}
}
