package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

func (a *API) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT `+accountColumns+` FROM accounts WHERE household_id=? AND status<>'deleted' ORDER BY created_at,id`,
		p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	type accountRow struct {
		id, kind, name, code, status string
		subtype, owner, note         sql.NullString
	}
	var list []accountRow
	for rows.Next() {
		var v accountRow
		if err := rows.Scan(&v.id, &v.kind, &v.name, &v.code, &v.status, &v.subtype, &v.owner, &v.note); err != nil {
			_ = rows.Close()
			internalErr(w, err)
			return
		}
		list = append(list, v)
	}
	// Close the rows BEFORE issuing per-account balance queries: the SQLite pool
	// is intentionally single-connection, so a nested query here would deadlock.
	if err := rows.Close(); err != nil {
		internalErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, v := range list {
		balance, err := db.Balance(r.Context(), a.DB, p.HouseholdID, v.code)
		if err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"id": v.id, "kind": v.kind, "subtype": nullString(v.subtype), "name": v.name,
			"owner_member_id": nullString(v.owner), "status": v.status, "note": nullString(v.note),
			"balance_minor": balance,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func nullString(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

func validAccountName(name string) bool {
	length := utf8.RuneCountInString(name)
	return length >= 1 && length <= 100
}

func (a *API) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Kind         string  `json:"kind"`
		Name         string  `json:"name"`
		Subtype      *string `json:"subtype"`
		OwnerMember  *string `json:"owner_member_id"`
		Note         *string `json:"note"`
		OpeningMinor int64   `json:"opening_minor"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Kind != "cash" && in.Kind != "credit" {
		writeErr(w, 400, "invalid_request", "kind 必须为 cash/credit")
		return
	}
	if !validAccountName(in.Name) {
		writeErr(w, 400, "invalid_request", "账户名称须为 1 至 100 个字符")
		return
	}
	opening := money.Minor(in.OpeningMinor)
	if opening < 0 {
		writeErr(w, 400, "invalid_amount", "期初金额不能为负")
		return
	}
	id := newID("acc")
	code := accountLedgerCode(in.Kind, id)
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := a.validateAccountOwner(r, tx, p.HouseholdID, in.OwnerMember); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO accounts
			(id,household_id,kind,subtype,name,owner_member_id,ledger_code,note) VALUES(?,?,?,?,?,?,?,?)`,
			id, p.HouseholdID, in.Kind, in.Subtype, in.Name, in.OwnerMember, code, in.Note); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO ledger_accounts
			(id,household_id,code,name,category,ref_type,ref_id) VALUES(?,?,?,?,?,'account',?)`,
			newID("la"), p.HouseholdID, code, in.Name, accountLedgerCategory(in.Kind), id); err != nil {
			return err
		}
		if opening > 0 {
			entryID := newID("je")
			entryContext := ledger.Context{EntryID: entryID, BizID: id, OccurredOn: a.businessDate()}
			var entry *ledger.Entry
			var entryErr error
			if in.Kind == "credit" {
				// A credit account opens with a liability: 借 期初权益 / 贷 信用卡欠款.
				entry, entryErr = ledger.OpeningBalance(entryContext, code, opening)
			} else {
				entry, entryErr = ledger.OpeningCash(entryContext, id, opening)
			}
			if entryErr != nil {
				return entryErr
			}
			if err := db.SaveEntry(r.Context(), tx, entry, p.HouseholdID, "", p.Username); err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(),
				`INSERT INTO account_opening_balances(id,account_id,amount_minor,entry_id) VALUES(?,?,?,?)`,
				newID("ob"), id, int64(opening), entryID); err != nil {
				return err
			}
		}
		return audit(r.Context(), tx, p, "create_account", "account", id, "{}")
	})
	if errors.Is(err, errInvalidAccountOwner) {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	balance, err := db.Balance(r.Context(), a.DB, p.HouseholdID, code)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "name": in.Name, "kind": in.Kind, "ledger_code": code,
		"opening_minor": opening, "balance_minor": balance,
	})
}

func (a *API) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	info, err := loadAccount(r.Context(), a.DB, p.HouseholdID, r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "账户不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	balance, err := db.Balance(r.Context(), a.DB, p.HouseholdID, info.Code)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": info.ID, "kind": info.Kind, "subtype": nullString(info.Subtype), "name": info.Name,
		"owner_member_id": nullString(info.Owner), "status": info.Status, "note": nullString(info.Note),
		"ledger_code": info.Code, "balance_minor": balance,
	})
}

func (a *API) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Name        *string `json:"name"`
		Subtype     *string `json:"subtype"`
		OwnerMember *string `json:"owner_member_id"`
		Note        *string `json:"note"`
		Status      *string `json:"status"`
	}
	if err := decode(r, &in); err != nil || (in.Name == nil && in.Subtype == nil && in.OwnerMember == nil && in.Note == nil && in.Status == nil) {
		writeErr(w, 400, "invalid_request", "请提供要修改的账户资料")
		return
	}
	if in.Name != nil {
		*in.Name = strings.TrimSpace(*in.Name)
		if !validAccountName(*in.Name) {
			writeErr(w, 400, "invalid_request", "账户名称须为 1 至 100 个字符")
			return
		}
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "disabled" {
		writeErr(w, 400, "invalid_request", "账户状态无效")
		return
	}
	id := r.PathValue("id")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := a.validateAccountOwner(r, tx, p.HouseholdID, in.OwnerMember); err != nil {
			return err
		}
		var code string
		query := `SELECT ledger_code FROM accounts WHERE id=? AND household_id=? AND status<>'deleted'`
		if a.DB.IsMySQL() {
			query += " FOR UPDATE"
		}
		if err := tx.QueryRowContext(r.Context(), query, id, p.HouseholdID).Scan(&code); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE accounts SET
			name=COALESCE(?,name), status=COALESCE(?,status),
			subtype=CASE WHEN ? IS NULL THEN subtype ELSE NULLIF(?,'') END,
			owner_member_id=CASE WHEN ? IS NULL THEN owner_member_id ELSE NULLIF(?,'') END,
			note=CASE WHEN ? IS NULL THEN note ELSE NULLIF(?,'') END
			WHERE id=? AND household_id=? AND status<>'deleted'`,
			in.Name, in.Status, in.Subtype, in.Subtype, in.OwnerMember, in.OwnerMember, in.Note, in.Note,
			id, p.HouseholdID); err != nil {
			return err
		}
		if in.Name != nil {
			if _, err := tx.ExecContext(r.Context(), `UPDATE ledger_accounts SET name=? WHERE code=? AND household_id=?`,
				*in.Name, code, p.HouseholdID); err != nil {
				return err
			}
		}
		return audit(r.Context(), tx, p, "update_account", "account", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "账户不存在")
	case errors.Is(err, errInvalidAccountOwner):
		writeErr(w, 400, "invalid_request", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id})
	}
}

// handleDeleteAccount is a SOFT delete: status='deleted' keeps the balance and
// every historical journal line; the account can no longer be used for new
// transactions.
func (a *API) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		result, err := tx.ExecContext(r.Context(),
			`UPDATE accounts SET status='deleted' WHERE id=? AND household_id=? AND status<>'deleted'`,
			id, p.HouseholdID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			var existing string
			return tx.QueryRowContext(r.Context(),
				`SELECT id FROM accounts WHERE id=? AND household_id=? AND status='deleted'`, id, p.HouseholdID).Scan(&existing)
		}
		return audit(r.Context(), tx, p, "delete_account", "account", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "账户不存在")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]any{"id": id, "status": "deleted"})
	}
}

// handleAccountJournal returns the last 100 journal lines that touched this
// account's ledger code. change_minor is signed: positive = debit.
func (a *API) handleAccountJournal(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	info, err := loadAccount(r.Context(), a.DB, p.HouseholdID, r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "账户不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT e.id,e.occurred_on,e.biz_type,e.reversal_of,SUM(l.debit_minor-l.credit_minor)
		FROM journal_entries e JOIN journal_lines l ON l.entry_id=e.id
		WHERE e.household_id=? AND l.account_code=?
		GROUP BY e.id,e.occurred_on,e.biz_type,e.reversal_of,e.created_at
		ORDER BY e.occurred_on DESC,e.created_at DESC,e.id DESC LIMIT 100`, p.HouseholdID, info.Code)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID         string  `json:"id"`
		Date       string  `json:"occurred_on"`
		Kind       string  `json:"kind"`
		ReversalOf *string `json:"reversal_of"`
		Change     int64   `json:"change_minor"`
	}
	items := []item{}
	for rows.Next() {
		var line item
		var reversal sql.NullString
		if err := rows.Scan(&line.ID, &line.Date, &line.Kind, &reversal, &line.Change); err != nil {
			internalErr(w, err)
			return
		}
		if reversal.Valid {
			line.ReversalOf = &reversal.String
		}
		items = append(items, line)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "limit": 100})
}

// handleAccountTransfer moves money between accounts:
//   - cash -> cash: ledger.Transfer (net worth unchanged)
//   - cash -> credit: ledger.RepayCredit (a repayment, never a new expense)
//
// credit -> cash is rejected: money does not flow out of a liability account.
func (a *API) handleAccountTransfer(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		FromAccountID string  `json:"from_account_id"`
		ToAccountID   string  `json:"to_account_id"`
		AmountMinor   int64   `json:"amount_minor"`
		OccurredOn    string  `json:"occurred_on"`
		Note          *string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	amount := money.Minor(in.AmountMinor)
	if err := money.MustPositive(amount); err != nil || !validDate(in.OccurredOn) {
		writeErr(w, 400, "invalid_amount", "金额必须为正整数分，日期须为 YYYY-MM-DD")
		return
	}
	key := idempotencyKey(r)
	txID, entryID := newID("tx"), newID("je")
	var replayID string
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if key != "" {
			existing, err := findReplayedTransaction(r, tx, p.HouseholdID, key)
			if err != nil {
				return err
			}
			if existing != "" {
				replayID = existing
				return nil
			}
		}
		note := ""
		if in.Note != nil {
			note = strings.TrimSpace(*in.Note)
		}
		if note == "" {
			note = "账户转账"
		}
		return a.saveTransfer(r, tx, p, transferInput{
			FromAccountID: in.FromAccountID,
			ToAccountID:   in.ToAccountID,
			Amount:        amount,
			OccurredOn:    in.OccurredOn,
			Note:          &note,
			Key:           key,
		}, txID, entryID)
	})
	if err != nil && key != "" && replayID == "" {
		if committed, lookupErr := a.lookupReplay(r, p.HouseholdID, key); lookupErr == nil && committed != "" {
			replayID, err = committed, nil
		}
	}
	if replayID != "" {
		writeJSON(w, http.StatusOK, map[string]any{"id": replayID, "status": "posted", "replayed": true})
		return
	}
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": txID, "entry_id": entryID, "status": "posted"})
}
