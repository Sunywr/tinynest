package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

func (a *API) registerTransactions(mux *http.ServeMux) {
	a.route(mux, "GET /api/v1/transactions", a.handleListTransactions)
	a.route(mux, "POST /api/v1/transactions", a.handleCreateTransaction)
	a.route(mux, "GET /api/v1/transactions/{id}", a.handleGetTransaction)
	a.route(mux, "POST /api/v1/transactions/{id}/reverse", a.handleReverseTransaction)
	a.route(mux, "POST /api/v1/refunds", a.handleCreateRefund)
	a.route(mux, "GET /api/v1/refunds/by-transaction/{id}", a.handleListRefunds)
}

type splitInput struct {
	CategoryID  string  `json:"category_id"`
	ProjectID   *string `json:"project_id"`
	AmountMinor int64   `json:"amount_minor"`
	Note        *string `json:"note"`
}

type txInput struct {
	Type               string       `json:"type"`
	AmountMinor        int64        `json:"amount_minor"`
	Currency           string       `json:"currency"`
	OccurredOn         string       `json:"occurred_on"`
	PaymentKind        string       `json:"payment_kind"`
	AccountID          *string      `json:"account_id"`
	ToAccountID        *string      `json:"to_account_id"`
	CreditInstrumentID *string      `json:"credit_instrument_id"`
	MemberID           *string      `json:"member_id"`
	Merchant           *string      `json:"merchant"`
	Note               *string      `json:"note"`
	Splits             []splitInput `json:"splits"`
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func (a *API) validateCategory(ctx *sql.Tx, r *http.Request, householdID, catID, kind string) error {
	var gotKind string
	if err := ctx.QueryRowContext(r.Context(),
		`SELECT kind FROM categories WHERE id=? AND household_id=? AND active=1`, catID, householdID).Scan(&gotKind); err != nil {
		return errors.New("分类不存在或已停用")
	}
	if gotKind != kind {
		return errors.New("分类方向与交易类型不匹配")
	}
	return nil
}

func (a *API) validateAccount(ctx *sql.Tx, r *http.Request, householdID, accID string) (string, error) {
	var code string
	err := ctx.QueryRowContext(r.Context(),
		`SELECT ledger_code FROM accounts WHERE id=? AND household_id=? AND status='active'`, accID, householdID).Scan(&code)
	return code, err
}

func (a *API) handleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in txInput
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	total := money.Minor(in.AmountMinor)
	if err := money.MustPositive(total); err != nil || !validDate(in.OccurredOn) {
		writeErr(w, 400, "invalid_amount", "金额必须为正整数分，日期须为 YYYY-MM-DD")
		return
	}
	if in.Type != "expense" && in.Type != "income" && in.Type != "transfer" {
		writeErr(w, 400, "invalid_request", "type 必须为 expense/income/transfer")
		return
	}
	if in.Currency == "" {
		in.Currency = "CNY"
	}
	key := idempotencyKey(r)
	if key != "" {
		var existing string
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT id FROM transactions WHERE idempotency_key=? AND household_id=?`, key, p.HouseholdID).Scan(&existing); err == nil {
			writeJSON(w, 200, map[string]any{"id": existing, "status": "posted", "replayed": true})
			return
		}
	}

	txID, entryID := newID("tx"), newID("je")
	c := ledger.Context{EntryID: entryID, BizID: txID, OccurredOn: in.OccurredOn}

	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var entry *ledger.Entry
		var err error
		var splits []splitInput

		if in.MemberID != nil {
			var id string
			if err := tx.QueryRowContext(r.Context(),
				`SELECT id FROM household_members WHERE id=? AND household_id=? AND status='active'`,
				*in.MemberID, p.HouseholdID).Scan(&id); err != nil {
				return errors.New("家庭成员不存在或已停用")
			}
		}

		switch in.Type {
		case "transfer":
			if in.AccountID == nil || in.ToAccountID == nil {
				return errors.New("转账需要 account_id 与 to_account_id")
			}
			if _, err = a.validateAccount(tx, r, p.HouseholdID, *in.AccountID); err != nil {
				return errors.New("转出账户不存在")
			}
			if _, err = a.validateAccount(tx, r, p.HouseholdID, *in.ToAccountID); err != nil {
				return errors.New("转入账户不存在")
			}
			entry, err = ledger.Transfer(c, *in.AccountID, *in.ToAccountID, total)
			if err != nil {
				return err
			}

		case "expense", "income":
			if len(in.Splits) == 0 {
				return errors.New("至少需要一条分类拆分行")
			}
			ls := make([]ledger.Split, 0, len(in.Splits))
			kind := "expense"
			if in.Type == "income" {
				kind = "income"
			}
			for _, s := range in.Splits {
				if err := a.validateCategory(tx, r, p.HouseholdID, s.CategoryID, kind); err != nil {
					return err
				}
				if s.ProjectID != nil {
					var n int
					if err := tx.QueryRowContext(r.Context(),
						`SELECT COUNT(*) FROM projects WHERE id=? AND household_id=?`, *s.ProjectID, p.HouseholdID).Scan(&n); err != nil || n == 0 {
						return errors.New("项目不存在")
					}
				}
				ls = append(ls, ledger.Split{CategoryID: s.CategoryID, Amount: money.Minor(s.AmountMinor)})
			}
			splits = in.Splits

			if in.Type == "expense" && in.PaymentKind == "credit" {
				if in.CreditInstrumentID == nil {
					return errors.New("信用消费需要 credit_instrument_id")
				}
				var n int
				if err := tx.QueryRowContext(r.Context(),
					`SELECT COUNT(*) FROM credit_instruments WHERE id=? AND household_id=? AND status='active'`,
					*in.CreditInstrumentID, p.HouseholdID).Scan(&n); err != nil || n == 0 {
					return errors.New("信用工具不存在")
				}
				entry, err = ledger.CreditExpense(c, *in.CreditInstrumentID, ls, total)
			} else if in.Type == "expense" {
				if in.AccountID == nil {
					return errors.New("现金支出需要 account_id")
				}
				if _, err = a.validateAccount(tx, r, p.HouseholdID, *in.AccountID); err != nil {
					return errors.New("付款账户不存在")
				}
				entry, err = ledger.CashExpense(c, *in.AccountID, ls, total)
			} else {
				if in.AccountID == nil {
					return errors.New("收入需要 account_id")
				}
				if _, err = a.validateAccount(tx, r, p.HouseholdID, *in.AccountID); err != nil {
					return errors.New("收款账户不存在")
				}
				entry, err = ledger.Income(c, *in.AccountID, ls, total)
			}
		}
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, entry, p.HouseholdID, key, p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO transactions(id,household_id,type,amount_minor,currency,occurred_on,
			  account_id,to_account_id,credit_instrument_id,member_id,merchant,note,status,entry_id,idempotency_key,created_by)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,'posted',?,?,?)`,
			txID, p.HouseholdID, in.Type, int64(total), in.Currency, in.OccurredOn,
			in.AccountID, in.ToAccountID, in.CreditInstrumentID, in.MemberID, in.Merchant, in.Note,
			entryID, nullableKey(key), p.Username); err != nil {
			return err
		}
		for i, s := range splits {
			if _, err := tx.ExecContext(r.Context(), `
				INSERT INTO transaction_splits(transaction_id,line_no,category_id,project_id,amount_minor,note)
				VALUES(?,?,?,?,?,?)`, txID, i+1, s.CategoryID, s.ProjectID, s.AmountMinor, s.Note); err != nil {
				return err
			}
		}
		if in.Type == "expense" && in.PaymentKind == "credit" {
			if err := a.assignStatement(r, tx, p, *in.CreditInstrumentID, "transaction", txID, int64(total), in.OccurredOn); err != nil {
				return err
			}
		}
		return audit(r.Context(), tx, p, "create_transaction", "transaction", txID, "")
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": txID, "entry_id": entryID, "status": "posted"})
}

func nullableKey(k string) any {
	if k == "" {
		return nil
	}
	return k
}

func (a *API) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	txType := q.Get("type")
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id,type,amount_minor,occurred_on,account_id,to_account_id,credit_instrument_id,
		       merchant,note,status,entry_id
		FROM transactions WHERE household_id=?
		  AND (?='' OR occurred_on>=?) AND (?='' OR occurred_on<=?) AND (?='' OR type=?)
		ORDER BY occurred_on DESC, created_at DESC LIMIT 200`,
		p.HouseholdID, from, from, to, to, txType, txType)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, typ, occurred, status, entryID string
		var amount int64
		var acc, toAcc, ci, merchant, note *string
		if err := rows.Scan(&id, &typ, &amount, &occurred, &acc, &toAcc, &ci, &merchant, &note, &status, &entryID); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"id": id, "type": typ, "amount_minor": amount, "occurred_on": occurred,
			"account_id": acc, "to_account_id": toAcc, "credit_instrument_id": ci,
			"merchant": merchant, "note": note, "status": status, "entry_id": entryID,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

// loadEntry rebuilds a ledger.Entry from persisted lines (used for reversals).
func loadEntry(ctx *sql.Tx, r *http.Request, entryID string) (*ledger.Entry, error) {
	e := &ledger.Entry{ID: entryID}
	var err error
	err = ctx.QueryRowContext(r.Context(),
		`SELECT biz_type,biz_id,occurred_on,currency,status FROM journal_entries WHERE id=?`, entryID).
		Scan(&e.BizType, &e.BizID, &e.OccurredOn, &e.Currency, &e.Status)
	if err != nil {
		return nil, err
	}
	rows, err := ctx.QueryContext(r.Context(),
		`SELECT account_code,debit_minor,credit_minor,memo FROM journal_lines WHERE entry_id=? ORDER BY line_no`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l ledger.Line
		var dr, cr int64
		var memo *string
		if err := rows.Scan(&l.Account, &dr, &cr, &memo); err != nil {
			return nil, err
		}
		l.Debit, l.Credit = money.Minor(dr), money.Minor(cr)
		if memo != nil {
			l.Memo = *memo
		}
		e.Lines = append(e.Lines, l)
	}
	return e, rows.Err()
}

func (a *API) handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var typ, occurred, status, entryID string
	var amount int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT type,amount_minor,occurred_on,status,entry_id FROM transactions WHERE id=? AND household_id=?`,
		id, p.HouseholdID).Scan(&typ, &amount, &occurred, &status, &entryID)
	if err != nil {
		writeErr(w, 404, "not_found", "流水不存在")
		return
	}
	lines, _ := a.DB.QueryContext(r.Context(),
		`SELECT account_code,debit_minor,credit_minor FROM journal_lines WHERE entry_id=? ORDER BY line_no`, entryID)
	journal := []map[string]any{}
	if lines != nil {
		defer lines.Close()
		for lines.Next() {
			var code string
			var dr, cr int64
			if lines.Scan(&code, &dr, &cr) == nil {
				journal = append(journal, map[string]any{"account": code, "debit_minor": dr, "credit_minor": cr})
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "type": typ, "amount_minor": amount, "occurred_on": occurred,
		"status": status, "entry_id": entryID, "journal_lines": journal,
	})
}

func (a *API) handleReverseTransaction(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	revID := newID("je")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var entryID, status string
		if err := tx.QueryRowContext(r.Context(),
			`SELECT entry_id,status FROM transactions WHERE id=? AND household_id=?`, id, p.HouseholdID).
			Scan(&entryID, &status); err != nil {
			return errors.New("流水不存在")
		}
		if status != "posted" {
			return errors.New("只有已入账流水可以冲正")
		}
		orig, err := loadEntry(tx, r, entryID)
		if err != nil {
			return err
		}
		rev, err := orig.Reverse(revID, a.businessDate())
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, rev, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE transactions SET status='reversed' WHERE id=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE journal_entries SET status='reversed' WHERE id=?`, entryID); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "reverse_transaction", "transaction", id, revID)
	})
	if err != nil {
		writeErr(w, 400, "conflict", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"transaction_id": id, "reversal_entry_id": revID, "status": "reversed"})
}

func (a *API) handleCreateRefund(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		TransactionID   string  `json:"transaction_id"`
		SplitLineNo     int     `json:"split_line_no"`
		AmountMinor     int64   `json:"amount_minor"`
		RefundOn        string  `json:"refund_on"`
		DestinationType string  `json:"destination_type"`
		DestinationID   string  `json:"destination_id"`
		Note            *string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	amount := money.Minor(in.AmountMinor)
	if !validDate(in.RefundOn) || in.DestinationType != "cash" && in.DestinationType != "credit" {
		writeErr(w, 400, "invalid_request", "日期或退款去向不正确")
		return
	}
	refundID, entryID := newID("ref"), newID("je")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var original int64
		var txType, txStatus string
		if err := tx.QueryRowContext(r.Context(),
			`SELECT amount_minor,type,status FROM transactions WHERE id=? AND household_id=?`,
			in.TransactionID, p.HouseholdID).Scan(&original, &txType, &txStatus); err != nil {
			return errors.New("原交易不存在")
		}
		if txType != "expense" || txStatus == "reversed" {
			return errors.New("只有已入账支出可以退款")
		}
		var splitID int64
		var categoryID string
		var splitAmount int64
		if in.SplitLineNo > 0 {
			if err := tx.QueryRowContext(r.Context(),
				`SELECT id,category_id,amount_minor FROM transaction_splits WHERE transaction_id=? AND line_no=?`,
				in.TransactionID, in.SplitLineNo).Scan(&splitID, &categoryID, &splitAmount); err != nil {
				return errors.New("拆分行不存在")
			}
		} else {
			if err := tx.QueryRowContext(r.Context(),
				`SELECT id,category_id,amount_minor FROM transaction_splits WHERE transaction_id=? ORDER BY line_no LIMIT 1`,
				in.TransactionID).Scan(&splitID, &categoryID, &splitAmount); err != nil {
				return errors.New("原交易没有分类行")
			}
		}
		var already int64
		if err := tx.QueryRowContext(r.Context(),
			`SELECT COALESCE(SUM(amount_minor),0) FROM refunds WHERE split_id=?`, splitID).Scan(&already); err != nil {
			return err
		}
		if _, err := ledger.ValidateRefund(money.Minor(splitAmount), money.Minor(already), amount); err != nil {
			return fmt.Errorf("退款金额不合法: %w", err)
		}
		var entry *ledger.Entry
		var err error
		c := ledger.Context{EntryID: entryID, BizID: refundID, OccurredOn: in.RefundOn}
		if in.DestinationType == "cash" {
			if _, err = a.validateAccount(tx, r, p.HouseholdID, in.DestinationID); err != nil {
				return errors.New("退款到账账户不存在")
			}
			entry, err = ledger.RefundToCash(c, in.DestinationID, categoryID, amount)
		} else {
			var n int
			if err := tx.QueryRowContext(r.Context(),
				`SELECT COUNT(*) FROM credit_instruments WHERE id=? AND household_id=? AND status='active'`,
				in.DestinationID, p.HouseholdID).Scan(&n); err != nil || n == 0 {
				return errors.New("退款信用工具不存在")
			}
			entry, err = ledger.RefundToCredit(c, in.DestinationID, categoryID, amount)
		}
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, entry, p.HouseholdID, idempotencyKey(r), p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO refunds(id,household_id,transaction_id,split_id,category_id,amount_minor,refund_on,
			  destination_type,destination_id,note,entry_id,idempotency_key)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			refundID, p.HouseholdID, in.TransactionID, splitID, categoryID, int64(amount), in.RefundOn,
			in.DestinationType, in.DestinationID, in.Note, entryID, nullableKey(idempotencyKey(r))); err != nil {
			return err
		}
		if in.DestinationType == "credit" {
			if err := a.assignStatement(r, tx, p, in.DestinationID, "refund", refundID, -int64(amount), in.RefundOn); err != nil {
				return err
			}
		}
		return audit(r.Context(), tx, p, "create_refund", "refund", refundID, in.TransactionID)
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": refundID, "entry_id": entryID})
}

func (a *API) handleListRefunds(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var total int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT amount_minor FROM transactions WHERE id=? AND household_id=?`, id, p.HouseholdID).Scan(&total)
	if err != nil {
		writeErr(w, 404, "not_found", "原交易不存在")
		return
	}
	var refunded int64
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(SUM(amount_minor),0) FROM refunds WHERE transaction_id=? AND household_id=?`,
		id, p.HouseholdID).Scan(&refunded)
	writeJSON(w, 200, map[string]any{
		"transaction_id": id, "original_minor": total,
		"refunded_minor": refunded, "net_expense_minor": total - refunded,
		"remaining_refundable_minor": total - refunded,
	})
}
