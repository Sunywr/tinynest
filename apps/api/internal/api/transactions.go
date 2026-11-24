package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

// --- shared helpers ---------------------------------------------------------

const findReplayQuery = `SELECT id FROM transactions WHERE idempotency_key=? AND household_id=?`

// findReplayedTransaction returns the already-committed transaction for an
// idempotency key, or "" when the key is unused.
func findReplayedTransaction(r *http.Request, q db.Queryer, householdID, key string) (string, error) {
	var id string
	err := q.QueryRowContext(r.Context(), findReplayQuery, key, householdID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// lookupReplay re-reads the key AFTER a failed transaction rolled back: a
// concurrent request may have committed the same key while this one ran.
func (a *API) lookupReplay(r *http.Request, householdID, key string) (string, error) {
	var id string
	err := a.DB.QueryRowContext(r.Context(), findReplayQuery, key, householdID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// transactionValues is one fully-resolved "流水" row (used by POST and PATCH).
type transactionValues struct {
	BudgetID    *string
	Type        string
	Amount      money.Minor
	OccurredOn  string
	CategoryID  *string
	AccountID   *string
	ToAccountID *string
	MemberID    *string
	Note        *string
}

type transferInput struct {
	FromAccountID string
	ToAccountID   string
	Amount        money.Minor
	OccurredOn    string
	Note          *string
	MemberID      *string
	Key           string
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// buildTransactionEntry validates the business references and returns the
// balanced journal entry. Nothing is persisted here; the caller commits it with
// the transaction row in one database transaction.
func (a *API) buildTransactionEntry(ctx context.Context, q db.Queryer, householdID string, v transactionValues, txID, entryID string) (*ledger.Entry, error) {
	if v.BudgetID != nil {
		var id string
		if v.Type != "expense" {
			return nil, errors.New("预算仅适用于支出")
		}
		if err := q.QueryRowContext(ctx, `SELECT id FROM project_budgets WHERE household_id=? AND id=?`, householdID, *v.BudgetID).Scan(&id); err != nil {
			return nil, errors.New("项目预算不存在")
		}
	}
	c := ledger.Context{EntryID: entryID, BizID: txID, OccurredOn: v.OccurredOn}
	switch v.Type {
	case "transfer":
		if v.AccountID == nil || v.ToAccountID == nil {
			return nil, errors.New("转账需要 account_id 与 to_account_id")
		}
		from, err := loadActiveAccount(ctx, q, householdID, *v.AccountID)
		if err != nil {
			return nil, errors.New("转出账户不存在或已停用")
		}
		to, err := loadActiveAccount(ctx, q, householdID, *v.ToAccountID)
		if err != nil {
			return nil, errors.New("转入账户不存在或已停用")
		}
		if from.ID == to.ID {
			return nil, errors.New("转出与转入账户不能相同")
		}
		if from.Kind == "credit" {
			return nil, errors.New("信用卡账户不能作为转出账户；还款请选择现金账户转入信用卡")
		}
		if to.Kind == "credit" {
			// Repayment of a credit account: 借 信用卡欠款 / 贷 现金.
			return ledger.RepayCredit(c, from.ID, to.ID, v.Amount)
		}
		return ledger.Transfer(c, from.ID, to.ID, v.Amount)

	case "expense", "income":
		if v.CategoryID == nil {
			return nil, errors.New("支出和收入必须选择分类")
		}
		kind, err := loadCategoryKind(ctx, q, householdID, *v.CategoryID)
		if err != nil {
			return nil, errors.New("分类不存在或已停用")
		}
		if kind != v.Type {
			return nil, errors.New("分类方向与流水类型不匹配")
		}
		if v.AccountID == nil {
			return nil, errors.New("请选择账户")
		}
		account, err := loadActiveAccount(ctx, q, householdID, *v.AccountID)
		if err != nil {
			return nil, errors.New("账户不存在或已停用")
		}
		splits := []ledger.Split{{CategoryID: *v.CategoryID, Amount: v.Amount}}
		if v.Type == "income" {
			if account.Kind != "cash" {
				return nil, errors.New("收入必须记入现金账户")
			}
			return ledger.Income(c, account.ID, splits, v.Amount)
		}
		if account.Kind == "credit" {
			return ledger.CreditExpense(c, account.ID, splits, v.Amount)
		}
		return ledger.CashExpense(c, account.ID, splits, v.Amount)
	}
	return nil, errors.New("type 必须为 expense/income/transfer")
}

// insertTransactionRow writes the public transaction row. The journal entry
// (entryID) must already be persisted in the same database transaction.
func insertTransactionRow(ctx context.Context, tx *sql.Tx, householdID string, v transactionValues, txID, entryID, key, actor string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO transactions
		(id,household_id,type,amount_minor,currency,occurred_on,category_id,account_id,to_account_id,member_id,note,status,entry_id,idempotency_key,created_by)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,'posted',?,?,?)`,
		txID, householdID, v.Type, int64(v.Amount), "CNY", v.OccurredOn,
		v.CategoryID, v.AccountID, v.ToAccountID, v.MemberID, v.Note, entryID, nullableKey(key), actor)
	if err != nil || v.BudgetID == nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO transaction_budget_links(transaction_id,budget_id) VALUES(?,?)`, txID, *v.BudgetID)
	return err
}

// saveTransfer books cash -> cash or cash -> credit, records the transfer flow
// and audits it. Used by POST /transactions (type=transfer) and
// POST /accounts/transfers.
func (a *API) saveTransfer(r *http.Request, tx *sql.Tx, p Principal, in transferInput, txID, entryID string) error {
	if err := a.validateMemberInput(r, tx, p.HouseholdID, in.MemberID); err != nil {
		return err
	}
	values := transactionValues{
		Type:        "transfer",
		Amount:      in.Amount,
		OccurredOn:  in.OccurredOn,
		AccountID:   &in.FromAccountID,
		ToAccountID: &in.ToAccountID,
		MemberID:    in.MemberID,
		Note:        in.Note,
	}
	entry, err := a.buildTransactionEntry(r.Context(), tx, p.HouseholdID, values, txID, entryID)
	if err != nil {
		return err
	}
	if err := db.SaveEntry(r.Context(), tx, entry, p.HouseholdID, in.Key, p.Username); err != nil {
		return err
	}
	if err := insertTransactionRow(r.Context(), tx, p.HouseholdID, values, txID, entryID, in.Key, p.Username); err != nil {
		return err
	}
	return audit(r.Context(), tx, p, "create_transfer", "transaction", txID, "")
}

// --- POST /transactions -----------------------------------------------------

type transactionRequest struct {
	Type        string  `json:"type"`
	AmountMinor int64   `json:"amount_minor"`
	OccurredOn  string  `json:"occurred_on"`
	CategoryID  *string `json:"category_id"`
	AccountID   *string `json:"account_id"`
	ToAccountID *string `json:"to_account_id"`
	MemberID    *string `json:"member_id"`
	BudgetID    *string `json:"budget_id"`
	Note        *string `json:"note"`
}

func (a *API) handleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in transactionRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	amount := money.Minor(in.AmountMinor)
	if err := money.MustPositive(amount); err != nil || !validDate(in.OccurredOn) {
		writeErr(w, 400, "invalid_amount", "金额必须为正整数分，日期须为 YYYY-MM-DD")
		return
	}
	if in.Type != "expense" && in.Type != "income" && in.Type != "transfer" {
		writeErr(w, 400, "invalid_request", "type 必须为 expense/income/transfer")
		return
	}
	values := transactionValues{
		Type:        in.Type,
		Amount:      amount,
		OccurredOn:  in.OccurredOn,
		CategoryID:  trimPtr(in.CategoryID),
		AccountID:   trimPtr(in.AccountID),
		ToAccountID: trimPtr(in.ToAccountID),
		MemberID:    trimPtr(in.MemberID),
		BudgetID:    trimPtr(in.BudgetID),
		Note:        in.Note,
	}
	if values.BudgetID != nil && values.Type != "expense" {
		writeErr(w, 400, "invalid_request", "预算仅适用于支出")
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
		if values.Type == "transfer" {
			return a.saveTransfer(r, tx, p, transferInput{
				FromAccountID: derefString(values.AccountID),
				ToAccountID:   derefString(values.ToAccountID),
				Amount:        values.Amount,
				OccurredOn:    values.OccurredOn,
				Note:          values.Note,
				MemberID:      values.MemberID,
				Key:           key,
			}, txID, entryID)
		}
		if err := a.validateMemberInput(r, tx, p.HouseholdID, values.MemberID); err != nil {
			return err
		}
		entry, err := a.buildTransactionEntry(r.Context(), tx, p.HouseholdID, values, txID, entryID)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, entry, p.HouseholdID, key, p.Username); err != nil {
			return err
		}
		if err := insertTransactionRow(r.Context(), tx, p.HouseholdID, values, txID, entryID, key, p.Username); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "create_transaction", "transaction", txID, "")
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

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// --- GET /transactions ------------------------------------------------------

const transactionSelect = `
	SELECT t.id,t.type,t.amount_minor,t.currency,t.occurred_on,
	       t.category_id,c.name,c.emoji,c.color,
	       t.account_id,a.name,t.to_account_id,b.name,
	       t.member_id,t.note,t.status,t.entry_id,t.created_at,pb.id,pb.name
	FROM transactions t
	LEFT JOIN categories c ON c.id=t.category_id AND c.household_id=t.household_id
	LEFT JOIN accounts a ON a.id=t.account_id AND a.household_id=t.household_id
	LEFT JOIN accounts b ON b.id=t.to_account_id AND b.household_id=t.household_id
	LEFT JOIN transaction_budget_links bl ON bl.transaction_id=t.id
 LEFT JOIN project_budgets pb ON pb.id=bl.budget_id AND pb.household_id=t.household_id
 WHERE t.household_id=?`

type transactionRow struct {
	ID            string
	Type          string
	AmountMinor   int64
	Currency      string
	OccurredOn    string
	CategoryID    sql.NullString
	CategoryName  sql.NullString
	CategoryEmoji sql.NullString
	CategoryColor sql.NullString
	AccountID     sql.NullString
	AccountName   sql.NullString
	ToAccountID   sql.NullString
	ToAccountName sql.NullString
	MemberID      sql.NullString
	Note          sql.NullString
	Status        string
	EntryID       string
	CreatedAt     string
	BudgetID      sql.NullString
	BudgetName    sql.NullString
}

func (row transactionRow) public() map[string]any {
	return map[string]any{
		"id": row.ID, "type": row.Type, "amount_minor": row.AmountMinor, "currency": row.Currency,
		"occurred_on": row.OccurredOn,
		"category_id": nullString(row.CategoryID), "category_name": nullString(row.CategoryName),
		"category_emoji": nullString(row.CategoryEmoji), "category_color": nullString(row.CategoryColor),
		"account_id": nullString(row.AccountID), "account_name": nullString(row.AccountName),
		"to_account_id": nullString(row.ToAccountID), "to_account_name": nullString(row.ToAccountName),
		"member_id": nullString(row.MemberID), "note": nullString(row.Note),
		"budget_id": nullString(row.BudgetID), "budget_name": nullString(row.BudgetName),
		"status": row.Status, "entry_id": row.EntryID, "created_at": row.CreatedAt,
	}
}

func (a *API) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	query := r.URL.Query()
	from, to := query.Get("from"), query.Get("to")
	if from != "" && !validDate(from) || to != "" && !validDate(to) {
		writeErr(w, 400, "invalid_request", "from/to 须为 YYYY-MM-DD")
		return
	}
	txType := query.Get("type")
	if txType != "" && txType != "expense" && txType != "income" && txType != "transfer" {
		writeErr(w, 400, "invalid_request", "type 必须为 expense/income/transfer")
		return
	}
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			writeErr(w, 400, "invalid_request", "每页条数须为 1 至 200")
			return
		}
		limit = n
	}

	sqlQuery := transactionSelect
	args := []any{p.HouseholdID}
	if from != "" {
		sqlQuery += ` AND t.occurred_on>=?`
		args = append(args, from)
	}
	if to != "" {
		sqlQuery += ` AND t.occurred_on<=?`
		args = append(args, to)
	}
	if txType != "" {
		sqlQuery += ` AND t.type=?`
		args = append(args, txType)
	}
	if categoryID := strings.TrimSpace(query.Get("category_id")); categoryID != "" {
		sqlQuery += ` AND t.category_id=?`
		args = append(args, categoryID)
	}
	if accountID := strings.TrimSpace(query.Get("account_id")); accountID != "" {
		sqlQuery += ` AND (t.account_id=? OR t.to_account_id=?)`
		args = append(args, accountID, accountID)
	}
	if keyword := strings.TrimSpace(query.Get("q")); keyword != "" {
		terms := strings.Fields(keyword)
		if len(terms) > 16 || len(keyword) > 1024 {
			writeErr(w, 400, "invalid_request", "搜索内容过长")
			return
		}
		escape := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
		for _, term := range terms {
			pattern := "%" + escape.Replace(strings.ToLower(term)) + "%"
			sqlQuery += ` AND (LOWER(COALESCE(t.note,'')) LIKE ? ESCAPE '!'
				OR LOWER(COALESCE(c.name,'')) LIKE ? ESCAPE '!'
				OR EXISTS (SELECT 1 FROM transaction_source_metadata m
					WHERE m.transaction_id=t.id AND LOWER(m.value) LIKE ? ESCAPE '!'))`
			args = append(args, pattern, pattern, pattern)
		}
	}
	if cursor := strings.TrimSpace(query.Get("cursor")); cursor != "" {
		var occurredOn, createdAt string
		err := a.DB.QueryRowContext(r.Context(),
			`SELECT occurred_on,created_at FROM transactions WHERE id=? AND household_id=?`,
			cursor, p.HouseholdID).Scan(&occurredOn, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 400, "invalid_request", "流水分页游标无效")
			return
		}
		if err != nil {
			internalErr(w, err)
			return
		}
		sqlQuery += ` AND (t.occurred_on, t.created_at, t.id) < (?, ?, ?)`
		args = append(args, occurredOn, createdAt, cursor)
	}
	sqlQuery += ` ORDER BY t.occurred_on DESC, t.created_at DESC, t.id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := a.DB.QueryContext(r.Context(), sqlQuery, args...)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var row transactionRow
		if err := rows.Scan(&row.ID, &row.Type, &row.AmountMinor, &row.Currency, &row.OccurredOn,
			&row.CategoryID, &row.CategoryName, &row.CategoryEmoji, &row.CategoryColor,
			&row.AccountID, &row.AccountName, &row.ToAccountID, &row.ToAccountName,
			&row.MemberID, &row.Note, &row.Status, &row.EntryID, &row.CreatedAt, &row.BudgetID, &row.BudgetName); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, row.public())
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	var nextCursor any
	if len(items) > limit {
		items = items[:limit]
		nextCursor = items[limit-1]["id"]
	}
	if err := rows.Close(); err != nil {
		internalErr(w, err)
		return
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item["id"].(string))
	}
	metadata, err := loadTransactionMetadata(r.Context(), a.DB, p.HouseholdID, ids)
	if err != nil {
		internalErr(w, err)
		return
	}
	for i, id := range ids {
		if details := metadata[id]; details != nil {
			items[i]["source_metadata"] = details
		}
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_cursor": nextCursor})
}

// --- GET /transactions/{id} -------------------------------------------------

func (a *API) loadTransactionRow(ctx context.Context, q db.Queryer, householdID, id string, forUpdate bool) (transactionRow, error) {
	query := transactionSelect + ` AND t.id=?`
	if forUpdate && a.DB.IsMySQL() {
		query += ` FOR UPDATE`
	}
	var row transactionRow
	err := q.QueryRowContext(ctx, query, householdID, id).Scan(
		&row.ID, &row.Type, &row.AmountMinor, &row.Currency, &row.OccurredOn,
		&row.CategoryID, &row.CategoryName, &row.CategoryEmoji, &row.CategoryColor,
		&row.AccountID, &row.AccountName, &row.ToAccountID, &row.ToAccountName,
		&row.MemberID, &row.Note, &row.Status, &row.EntryID, &row.CreatedAt, &row.BudgetID, &row.BudgetName)
	return row, err
}

func (a *API) handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	row, err := a.loadTransactionRow(r.Context(), a.DB, p.HouseholdID, r.PathValue("id"), false)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "流水不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	lines, err := a.journalLines(r, p.HouseholdID, row.EntryID)
	if err != nil {
		internalErr(w, err)
		return
	}
	out := row.public()
	metadata, err := loadTransactionMetadata(r.Context(), a.DB, p.HouseholdID, []string{row.ID})
	if err != nil {
		internalErr(w, err)
		return
	}
	if details := metadata[row.ID]; details != nil {
		out["source_metadata"] = details
	}
	out["journal_lines"] = lines
	writeJSON(w, 200, out)
}

func (a *API) journalLines(r *http.Request, householdID, entryID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT l.account_code,l.debit_minor,l.credit_minor FROM journal_lines l
		JOIN journal_entries e ON e.id=l.entry_id
		WHERE e.id=? AND e.household_id=? ORDER BY l.line_no`, entryID, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []map[string]any{}
	for rows.Next() {
		var code string
		var dr, cr int64
		if err := rows.Scan(&code, &dr, &cr); err != nil {
			return nil, err
		}
		lines = append(lines, map[string]any{"account": code, "debit_minor": dr, "credit_minor": cr})
	}
	return lines, rows.Err()
}

// --- reversal engine --------------------------------------------------------

// loadEntry rebuilds a ledger.Entry from persisted lines (used for reversals).
func loadEntry(ctx context.Context, q db.Queryer, householdID, entryID string) (*ledger.Entry, error) {
	entry := &ledger.Entry{ID: entryID}
	err := q.QueryRowContext(ctx,
		`SELECT biz_type,biz_id,occurred_on,currency,status FROM journal_entries WHERE id=? AND household_id=?`,
		entryID, householdID).
		Scan(&entry.BizType, &entry.BizID, &entry.OccurredOn, &entry.Currency, &entry.Status)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx,
		`SELECT account_code,debit_minor,credit_minor,memo FROM journal_lines WHERE entry_id=? ORDER BY line_no`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var line ledger.Line
		var dr, cr int64
		var memo sql.NullString
		if err := rows.Scan(&line.Account, &dr, &cr, &memo); err != nil {
			return nil, err
		}
		line.Debit, line.Credit = money.Minor(dr), money.Minor(cr)
		if memo.Valid {
			line.Memo = memo.String
		}
		entry.Lines = append(entry.Lines, line)
	}
	return entry, rows.Err()
}

// reversePosted books the compensating entry, marks the original journal entry
// reversed and the public transaction reversed. The reversal keeps the original
// occurred_on so per-period statistics net to zero instead of leaking the
// correction into another month. Callers must hold the transaction row lock.
func (a *API) reversePosted(ctx context.Context, tx *sql.Tx, p Principal, row transactionRow, reversalEntryID string) error {
	original, err := loadEntry(ctx, tx, p.HouseholdID, row.EntryID)
	if err != nil {
		return errors.New("原分录不存在，无法冲正")
	}
	reversal, err := original.Reverse(reversalEntryID, row.OccurredOn)
	if err != nil {
		return err
	}
	if err := db.SaveEntry(ctx, tx, reversal, p.HouseholdID, "", p.Username); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE transactions SET status='reversed' WHERE id=? AND household_id=? AND status='posted'`,
		row.ID, p.HouseholdID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return errors.New("该流水已冲正，不能重复操作")
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE journal_entries SET status='reversed' WHERE id=? AND household_id=?`,
		row.EntryID, p.HouseholdID); err != nil {
		return err
	}
	return nil
}

// --- PATCH /transactions/{id} (correction) ----------------------------------

func (a *API) handleCorrectTransaction(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Type        *string `json:"type"`
		AmountMinor *int64  `json:"amount_minor"`
		OccurredOn  *string `json:"occurred_on"`
		CategoryID  *string `json:"category_id"`
		AccountID   *string `json:"account_id"`
		ToAccountID *string `json:"to_account_id"`
		MemberID    *string `json:"member_id"`
		BudgetID    *string `json:"budget_id"`
		Note        *string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	oldID := r.PathValue("id")
	newIDValue, newEntryID := newID("tx"), newID("je")
	var response map[string]any
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		row, err := a.loadTransactionRow(r.Context(), tx, p.HouseholdID, oldID, true)
		if err != nil {
			return err
		}
		if row.Status != "posted" {
			return errTransactionNotPosted
		}
		if in.BudgetID != nil && in.Type == nil && in.AmountMinor == nil && in.OccurredOn == nil &&
			in.CategoryID == nil && in.AccountID == nil && in.ToAccountID == nil && in.MemberID == nil && in.Note == nil {
			if row.Type != "expense" {
				return errors.New("预算仅适用于支出")
			}
			budgetID := trimPtr(in.BudgetID)
			if budgetID != nil {
				var id string
				if err := tx.QueryRowContext(r.Context(), `SELECT id FROM project_budgets WHERE household_id=? AND id=?`, p.HouseholdID, *budgetID).Scan(&id); err != nil {
					return errors.New("项目预算不存在")
				}
			}
			if _, err := tx.ExecContext(r.Context(), `DELETE FROM transaction_budget_links WHERE transaction_id=?`, row.ID); err != nil {
				return err
			}
			if budgetID != nil {
				if _, err := tx.ExecContext(r.Context(), `INSERT INTO transaction_budget_links(transaction_id,budget_id) VALUES(?,?)`, row.ID, *budgetID); err != nil {
					return err
				}
			}
			if err := audit(r.Context(), tx, p, "set_transaction_budget", "transaction", row.ID, "{}"); err != nil {
				return err
			}
			response = map[string]any{"id": row.ID, "entry_id": row.EntryID, "status": "posted"}
			return nil
		}
		if in.Type != nil && *in.Type != row.Type {
			return errors.New("更正不支持修改流水类型，请冲正后重新记账")
		}
		values := transactionValues{
			Type:       row.Type,
			Amount:     money.Minor(row.AmountMinor),
			OccurredOn: row.OccurredOn,
			CategoryID: nullPtr(row.CategoryID),
			AccountID:  nullPtr(row.AccountID),
			MemberID:   nullPtr(row.MemberID),
			BudgetID:   nullPtr(row.BudgetID),
			Note:       nullPtr(row.Note),
		}
		if row.ToAccountID.Valid {
			to := row.ToAccountID.String
			values.ToAccountID = &to
		}
		if in.AmountMinor != nil {
			values.Amount = money.Minor(*in.AmountMinor)
		}
		if in.OccurredOn != nil {
			values.OccurredOn = *in.OccurredOn
		}
		if in.CategoryID != nil {
			values.CategoryID = trimPtr(in.CategoryID)
		}
		if in.AccountID != nil {
			values.AccountID = trimPtr(in.AccountID)
		}
		if in.ToAccountID != nil {
			values.ToAccountID = trimPtr(in.ToAccountID)
		}
		if in.MemberID != nil {
			values.MemberID = trimPtr(in.MemberID)
		}
		if in.BudgetID != nil {
			values.BudgetID = trimPtr(in.BudgetID)
		}
		if in.Note != nil {
			values.Note = in.Note
		}
		if err := money.MustPositive(values.Amount); err != nil || !validDate(values.OccurredOn) {
			return errors.New("金额必须为正整数分，日期须为 YYYY-MM-DD")
		}
		if row.Type == "transfer" {
			values.CategoryID = nil
		}
		if err := a.validateMemberInput(r, tx, p.HouseholdID, values.MemberID); err != nil {
			return err
		}
		// Build the replacement entry BEFORE touching the original: an invalid
		// correction must leave the posted flow untouched.
		replacement, err := a.buildTransactionEntry(r.Context(), tx, p.HouseholdID, values, newIDValue, newEntryID)
		if err != nil {
			return err
		}
		reversalID := newID("je")
		if err := a.reversePosted(r.Context(), tx, p, row, reversalID); err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, replacement, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if err := insertTransactionRow(r.Context(), tx, p.HouseholdID, values, newIDValue, newEntryID, "", p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO transaction_source_metadata(transaction_id,source_id,item_key,kind,value)
			SELECT ?,m.source_id,m.item_key,m.kind,m.value
			FROM transaction_source_metadata m JOIN transactions t ON t.id=m.transaction_id
			WHERE t.household_id=? AND m.transaction_id=?`, newIDValue, p.HouseholdID, oldID); err != nil {
			return err
		}
		if err := audit(r.Context(), tx, p, "reverse_transaction", "transaction", oldID, reversalID); err != nil {
			return err
		}
		if err := audit(r.Context(), tx, p, "correct_transaction", "transaction", newIDValue, oldID); err != nil {
			return err
		}
		response = map[string]any{"id": newIDValue, "replaces": oldID, "entry_id": newEntryID, "status": "posted"}
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "流水不存在")
		return
	}
	if errors.Is(err, errTransactionNotPosted) {
		writeErr(w, 409, "conflict", "该流水已冲正，不能修改")
		return
	}
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 200, response)
}

func nullPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

// --- DELETE /transactions/{id} (reversal only) ------------------------------

func (a *API) handleDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var response map[string]any
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		row, err := a.loadTransactionRow(r.Context(), tx, p.HouseholdID, id, true)
		if err != nil {
			return err
		}
		if row.Status != "posted" {
			return errTransactionNotPosted
		}
		if err := a.reversePosted(r.Context(), tx, p, row, newID("je")); err != nil {
			return err
		}
		if err := audit(r.Context(), tx, p, "delete_transaction", "transaction", id, ""); err != nil {
			return err
		}
		response = map[string]any{"id": id, "status": "reversed"}
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "流水不存在")
		return
	}
	if errors.Is(err, errTransactionNotPosted) {
		writeErr(w, 409, "conflict", "该流水已冲正，不能重复删除")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, response)
}

var errTransactionNotPosted = errors.New("只有未冲正的流水可以修改或删除")
