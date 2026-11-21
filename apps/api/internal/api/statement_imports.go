package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

type statementImportRow struct {
	ID          string `json:"id"`
	Reference   string `json:"reference"`
	Day         string `json:"day"`
	Merchant    string `json:"merchant"`
	Method      string `json:"method"`
	Kind        string `json:"kind"`
	Currency    string `json:"currency"`
	Amount      int64  `json:"amount_minor"`
	Member      string `json:"member_id"`
	Account     string `json:"account_id"`
	Card        string `json:"card_id"`
	Source      string `json:"source"`
	Profile     string `json:"profile"`
	UploadedBy  string `json:"uploaded_by"`
	Transaction string `json:"transaction_id"`
	Resolution  string `json:"resolution"`
	Created     bool   `json:"created,omitempty"`
}

type statementCandidate struct {
	ID       string `json:"id"`
	Day      string `json:"day"`
	Merchant string `json:"merchant"`
	Note     string `json:"note"`
	Exact    bool   `json:"exact"`
	Linked   bool   `json:"linked"`
}

func (a *API) registerStatementImports(mux *http.ServeMux) {
	a.route(mux, "GET /api/v1/statement-imports", a.listStatementImports)
	a.route(mux, "GET /api/v1/statement-imports/pending-count", a.countStatementImports)
	a.route(mux, "POST /api/v1/statement-imports", a.stageStatementImports)
	a.route(mux, "GET /api/v1/statement-imports/{id}/matches", a.statementImportMatches)
	a.route(mux, "PUT /api/v1/statement-imports/{id}", a.resolveStatementImport)
}

const selectStatementImport = `SELECT i.id,i.source,i.profile,i.uploaded_by,i.payload,
	COALESCE(i.transaction_id,''),i.resolution,
	COALESCE(t.status,re.status,pe.status,''),COALESCE(b.account_id,''),COALESCE(b.card_id,'')` + statementImportFrom

const statementImportFrom = `
	FROM statement_import_rows i LEFT JOIN transactions t
	ON t.household_id=i.household_id AND t.id=i.transaction_id
	LEFT JOIN refunds rf ON rf.household_id=i.household_id AND rf.id=i.transaction_id AND i.resolution='linked_refund'
	LEFT JOIN journal_entries re ON re.household_id=i.household_id AND re.id=rf.entry_id
	LEFT JOIN credit_payments cp ON cp.household_id=i.household_id AND cp.id=i.transaction_id AND i.resolution='linked_repayment'
	LEFT JOIN journal_entries pe ON pe.household_id=i.household_id AND pe.id=cp.entry_id
	LEFT JOIN statement_import_bindings b ON b.household_id=i.household_id AND b.binding_key=i.binding_key
	WHERE i.household_id=?`

func (a *API) countStatementImports(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var count int
	err := a.DB.QueryRowContext(r.Context(), `SELECT COUNT(*)`+statementImportFrom+
		` AND (i.resolution='' OR (i.transaction_id IS NOT NULL AND COALESCE(t.status,re.status,pe.status,'')<>'posted'))`, p.HouseholdID).Scan(&count)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]int{"count": count})
}

func scanStatementImport(s interface{ Scan(...any) error }) (statementImportRow, error) {
	var row statementImportRow
	var payload, status, account, card string
	if err := s.Scan(&row.ID, &row.Source, &row.Profile, &row.UploadedBy, &payload, &row.Transaction, &row.Resolution, &status, &account, &card); err != nil {
		return row, err
	}
	err := json.Unmarshal([]byte(payload), &row)
	if row.Account == "" && row.Card == "" {
		row.Account, row.Card = account, card
	}
	if row.Transaction != "" && status != "posted" {
		row.Transaction, row.Resolution = "", ""
	}
	return row, err
}

func importBindingKey(source, profile, member, method string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(source+"\x00"+profile+"\x00"+member+"\x00"+strings.TrimSpace(method))))
}

func importReferenceHash(reference string) string {
	if len(reference) < 16 {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(reference)))
}

type statementImportData struct {
	Reference string `json:"reference,omitempty"`
	Day       string `json:"day"`
	Merchant  string `json:"merchant"`
	Method    string `json:"method"`
	Kind      string `json:"kind"`
	Currency  string `json:"currency"`
	Amount    int64  `json:"amount_minor"`
	Member    string `json:"member_id,omitempty"`
	Account   string `json:"account_id,omitempty"`
	Card      string `json:"card_id,omitempty"`
}

func (a *API) listStatementImports(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), selectStatementImport+` AND i.id>? ORDER BY i.id LIMIT 201`, p.HouseholdID, r.URL.Query().Get("after"))
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []statementImportRow{}
	for rows.Next() {
		row, err := scanStatementImport(rows)
		if err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	next := ""
	if len(items) > 200 {
		items = items[:200]
		next = items[199].ID
	}
	writeJSON(w, 200, map[string]any{"items": items, "next": next})
}

func (a *API) stageStatementImports(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Source  string                `json:"source"`
		Profile string                `json:"profile"`
		File    string                `json:"file"`
		Offset  int                   `json:"offset"`
		Rows    []statementImportData `json:"rows"`
	}
	if decode(r, &in) != nil || len(in.Rows) == 0 || len(in.Rows) > 200 || in.Offset < 0 || in.Offset > 1000000 ||
		strings.TrimSpace(in.Profile) == "" || len(in.Profile) > 120 || len(in.File) != 64 ||
		!slices.Contains([]string{"alipay", "wechat", "credit", "debit"}, in.Source) {
		writeErr(w, 400, "invalid_statement_import", "账单来源、账户标识或明细无效")
		return
	}
	for i := range in.Rows {
		row := &in.Rows[i]
		row.Account, row.Card = "", ""
		if row.Currency == "" {
			row.Currency = "CNY"
		}
		if !validDate(row.Day) || money.MustPositive(money.Minor(row.Amount)) != nil || len(row.Reference) > 200 ||
			len(row.Merchant) > 180 || len(row.Method) > 180 || len(row.Currency) != 3 ||
			!validImportKind(row.Kind) {
			writeErr(w, 400, "invalid_statement_row", "账单日期、金额或明细格式无效")
			return
		}
	}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		query := `INSERT INTO statement_import_rows(household_id,id,source,profile,uploaded_by,binding_key,reference_hash,payload) VALUES(?,?,?,?,?,?,?,?)`
		if a.DB.IsMySQL() {
			query += ` ON DUPLICATE KEY UPDATE id=statement_import_rows.id`
		} else {
			query += ` ON CONFLICT(household_id,id) DO NOTHING`
		}
		for i, row := range in.Rows {
			if row.Member != "" {
				status, err := a.lockMember(r, tx, p.HouseholdID, row.Member)
				if err != nil || status != "active" {
					return errors.New("请选择本家庭的有效账单成员")
				}
			}
			// Row position is meaningful only within the same file when the provider has no ID.
			key := "ref:" + row.Reference
			if row.Reference == "" {
				key = "file:" + in.File + ":" + strconv.Itoa(in.Offset+i)
			}
			id := fmt.Sprintf("%x", sha256.Sum256([]byte(in.Source+"\x00"+strings.TrimSpace(in.Profile)+"\x00"+row.Member+"\x00"+key)))
			payload, err := json.Marshal(row)
			if err != nil {
				return err
			}
			binding := importBindingKey(in.Source, strings.TrimSpace(in.Profile), row.Member, row.Method)
			if _, err := tx.ExecContext(r.Context(), query, p.HouseholdID, id, in.Source, strings.TrimSpace(in.Profile), p.UserID, binding, importReferenceHash(row.Reference), string(payload)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, 400, "import_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]int{"processed": len(in.Rows)})
}

func validImportKind(kind string) bool {
	return slices.Contains([]string{"expense", "income", "refund", "repayment", "transfer", "review"}, kind)
}

func importMerchant(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

func statementCandidates(q db.Queryer, r *http.Request, household string, row statementImportRow) ([]statementCandidate, error) {
	items := []statementCandidate{}
	if row.Kind == "refund" || row.Kind == "repayment" || row.Kind == "transfer" {
		return statementMovementCandidates(q, r, household, row)
	}
	if row.Kind != "expense" && row.Kind != "income" || row.Account == "" && row.Card == "" {
		return items, nil
	}
	day, _ := time.Parse(time.DateOnly, row.Day)
	rows, err := q.QueryContext(r.Context(), `SELECT t.id,t.occurred_on,COALESCE(t.merchant,''),COALESCE(t.note,''),
		EXISTS(SELECT 1 FROM statement_import_rows i WHERE i.household_id=t.household_id AND i.transaction_id=t.id
		AND i.source=? AND i.id<>?),
		EXISTS(SELECT 1 FROM statement_import_rows i WHERE i.household_id=t.household_id AND i.transaction_id=t.id
		AND i.source<>? AND i.reference_hash<>'' AND i.reference_hash=?)
		FROM transactions t WHERE t.household_id=? AND t.status='posted' AND t.type=? AND t.currency=? AND t.amount_minor=?
		AND t.occurred_on BETWEEN ? AND ? AND COALESCE(t.account_id,'')=? AND COALESCE(t.credit_instrument_id,'')=?
		AND (?='' OR COALESCE(t.member_id,'')='' OR t.member_id=?)
		ORDER BY t.occurred_on,t.id LIMIT 30`, row.Source, row.ID, row.Source, importReferenceHash(row.Reference), household, row.Kind, row.Currency, row.Amount,
		day.AddDate(0, 0, -3).Format(time.DateOnly), day.AddDate(0, 0, 3).Format(time.DateOnly), row.Account, row.Card, row.Member, row.Member)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	merchant := importMerchant(row.Merchant)
	for rows.Next() {
		var item statementCandidate
		var sharedReference bool
		if err := rows.Scan(&item.ID, &item.Day, &item.Merchant, &item.Note, &item.Linked, &sharedReference); err != nil {
			return nil, err
		}
		item.Exact = !item.Linked && (sharedReference ||
			item.Day == row.Day && len([]rune(merchant)) >= 3 && merchant != "支付宝" && merchant != "微信支付" &&
				merchant == importMerchant(item.Merchant))
		items = append(items, item)
	}
	return items, rows.Err()
}

func statementMovementCandidates(q db.Queryer, r *http.Request, household string, row statementImportRow) ([]statementCandidate, error) {
	items := []statementCandidate{}
	if row.Account == "" && row.Card == "" || row.Currency != "CNY" {
		return items, nil
	}
	var query string
	var funding []any
	switch row.Kind {
	case "refund":
		query = `SELECT m.id,m.refund_on,COALESCE(t.merchant,''),COALESCE(t.note,'')
			FROM refunds m JOIN journal_entries e ON e.household_id=m.household_id AND e.id=m.entry_id AND e.status='posted'
			JOIN transactions t ON t.household_id=m.household_id AND t.id=m.transaction_id
			WHERE m.household_id=? AND m.amount_minor=? AND m.refund_on BETWEEN ? AND ?
			AND ((m.destination_type='cash' AND m.destination_id=?) OR (m.destination_type='credit' AND m.destination_id=?))`
		funding = []any{row.Account, row.Card}
	case "repayment":
		query = `SELECT m.id,m.paid_on,ci.name,s.cycle_end_on
			FROM credit_payments m JOIN journal_entries e ON e.household_id=m.household_id AND e.id=m.entry_id AND e.status='posted'
			JOIN credit_statements s ON s.household_id=m.household_id AND s.id=m.statement_id
			JOIN credit_instruments ci ON ci.household_id=s.household_id AND ci.id=s.instrument_id
			WHERE m.household_id=? AND m.amount_minor=? AND m.paid_on BETWEEN ? AND ?
			AND (m.from_account_id=? OR s.instrument_id=?)`
		funding = []any{row.Account, row.Card}
	case "transfer":
		if row.Account == "" {
			return items, nil
		}
		query = `SELECT m.id,m.occurred_on,COALESCE(src.name,''),COALESCE(dst.name,'')
			FROM transactions m JOIN accounts src ON src.household_id=m.household_id AND src.id=m.account_id
			JOIN accounts dst ON dst.household_id=m.household_id AND dst.id=m.to_account_id
			WHERE m.household_id=? AND m.amount_minor=? AND m.occurred_on BETWEEN ? AND ?
			AND m.status='posted' AND m.type='transfer' AND m.currency='CNY' AND (m.account_id=? OR m.to_account_id=?)`
		funding = []any{row.Account, row.Account}
	}
	day, _ := time.Parse(time.DateOnly, row.Day)
	args := []any{household, row.Amount, day.AddDate(0, 0, -3).Format(time.DateOnly), day.AddDate(0, 0, 3).Format(time.DateOnly)}
	rows, err := q.QueryContext(r.Context(), query+" LIMIT 30", append(args, funding...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item statementCandidate
		if err := rows.Scan(&item.ID, &item.Day, &item.Merchant, &item.Note); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *API) statementImportMatches(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	row, err := scanStatementImport(a.DB.QueryRowContext(r.Context(), selectStatementImport+` AND i.id=?`, p.HouseholdID, r.PathValue("id")))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "missing_statement_row", "账单明细不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	if r.URL.Query().Has("account_id") || r.URL.Query().Has("card_id") {
		row.Account, row.Card = r.URL.Query().Get("account_id"), r.URL.Query().Get("card_id")
	}
	matches, err := statementCandidates(a.DB, r, p.HouseholdID, row)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": matches})
}

func (a *API) resolveStatementImport(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var in struct {
		Action      string `json:"action"`
		Category    string `json:"category_id"`
		Account     string `json:"account_id"`
		Card        string `json:"card_id"`
		Transaction string `json:"transaction_id"`
		Remember    bool   `json:"remember_funding"`
		Kind        string `json:"kind"`
	}
	if decode(r, &in) != nil || in.Account != "" && in.Card != "" {
		writeErr(w, 400, "invalid_statement_resolution", "请选择一个实际资金账户")
		return
	}
	ctx := r.Context()
	var result statementImportRow
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if a.DB.IsMySQL() {
			var locked string
			if err := tx.QueryRowContext(ctx, `SELECT id FROM households WHERE id=? FOR UPDATE`, p.HouseholdID).Scan(&locked); err != nil {
				return err
			}
		}
		row, err := scanStatementImport(tx.QueryRowContext(ctx, selectStatementImport+` AND i.id=?`, p.HouseholdID, id))
		if err != nil {
			return err
		}
		result = row
		if in.Action == "unlink" {
			if !strings.HasPrefix(row.Resolution, "linked") && row.Resolution != "skipped" {
				return errConflict
			}
			row.Transaction, row.Resolution = "", ""
		} else {
			if row.Resolution != "" {
				return nil
			}
			if in.Account != "" || in.Card != "" {
				row.Account, row.Card = in.Account, in.Card
			}
			if row.Kind == "expense" || row.Kind == "income" {
				if row.Source == "credit" && row.Account != "" || row.Source == "debit" && row.Card != "" {
					return errors.New("资金账户类型与银行卡账单来源不符")
				}
			}
			if row.Account != "" {
				if _, err := a.validateAccount(tx, r, p.HouseholdID, row.Account); err != nil {
					return errors.New("实际资金账户不可用")
				}
			}
			if row.Card != "" {
				var card string
				if err := tx.QueryRowContext(ctx, `SELECT id FROM credit_instruments WHERE household_id=? AND id=? AND status='active'`, p.HouseholdID, row.Card).Scan(&card); err != nil {
					return errors.New("信用工具不可用")
				}
			}
			switch in.Action {
			case "classify":
				if !validImportKind(in.Kind) || row.Kind != "review" || row.Currency != "CNY" {
					return errConflict
				}
				row.Kind = in.Kind
			case "bind", "skip":
				if in.Action == "skip" {
					row.Resolution = "skipped"
				}
				if in.Action == "bind" && in.Remember {
					if row.Account == "" && row.Card == "" || (row.Source == "alipay" || row.Source == "wechat") && row.Method == "" {
						return errors.New("请选择实际资金账户，平台账单还需要明确付款方式")
					}
					query := `INSERT INTO statement_import_bindings(household_id,binding_key,account_id,card_id) VALUES(?,?,?,?)`
					if a.DB.IsMySQL() {
						query += ` ON DUPLICATE KEY UPDATE account_id=VALUES(account_id),card_id=VALUES(card_id)`
					} else {
						query += ` ON CONFLICT(household_id,binding_key) DO UPDATE SET account_id=excluded.account_id,card_id=excluded.card_id`
					}
					binding := importBindingKey(row.Source, row.Profile, row.Member, row.Method)
					if _, err := tx.ExecContext(ctx, query, p.HouseholdID, binding, row.Account, row.Card); err != nil {
						return err
					}
				}
			case "link":
				matches, err := statementCandidates(tx, r, p.HouseholdID, row)
				if err != nil {
					return err
				}
				for _, match := range matches {
					if match.ID == in.Transaction {
						row.Transaction, row.Resolution = match.ID, "linked"
						if row.Kind == "refund" || row.Kind == "repayment" {
							row.Resolution += "_" + row.Kind
						}
						break
					}
				}
				if row.Transaction == "" {
					return errConflict
				}
			case "auto", "create":
				if row.Account == "" && row.Card == "" || row.Currency != "CNY" || row.Kind != "expense" && row.Kind != "income" || row.Kind == "income" && row.Card != "" {
					return errors.New("此明细需要核对资金账户或交易性质")
				}
				matches, err := statementCandidates(tx, r, p.HouseholdID, row)
				if err != nil {
					return err
				}
				if in.Action == "auto" && len(matches) > 0 {
					if len(matches) != 1 || !matches[0].Exact {
						return errConflict
					}
					row.Transaction, row.Resolution = matches[0].ID, "linked"
				} else {
					input := txInput{Type: row.Kind, AmountMinor: row.Amount, Currency: row.Currency, OccurredOn: row.Day,
						Merchant: &row.Merchant, Splits: []splitInput{{CategoryID: in.Category, AmountMinor: row.Amount}}}
					if row.Member != "" {
						input.MemberID = &row.Member
					}
					if row.Card != "" {
						input.PaymentKind, input.CreditInstrumentID = "credit", &row.Card
					} else {
						input.PaymentKind, input.AccountID = "cash", &row.Account
					}
					c := ledger.Context{EntryID: newID("je"), BizID: newID("tx"), OccurredOn: row.Day}
					if err := a.saveTransaction(tx, r, p, input, "", "", c); err != nil {
						return err
					}
					row.Transaction, row.Resolution, row.Created = c.BizID, "posted", true
				}
			default:
				return errors.New("请选择有效的核对操作")
			}
		}
		data := statementImportData{Reference: row.Reference, Day: row.Day, Merchant: row.Merchant, Method: row.Method,
			Kind: row.Kind, Currency: row.Currency, Amount: row.Amount, Member: row.Member, Account: row.Account, Card: row.Card}
		payload, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE statement_import_rows SET payload=?,transaction_id=?,resolution=? WHERE household_id=? AND id=?`,
			string(payload), nullableKey(row.Transaction), row.Resolution, p.HouseholdID, id); err != nil {
			return err
		}
		result = row
		return audit(ctx, tx, p, "statement_import_"+in.Action, "statement_import_row", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "missing_statement_row", "账单明细不存在")
	case errors.Is(err, errConflict):
		writeErr(w, 409, "statement_needs_review", "请查看候选流水并确认")
	case err != nil:
		writeErr(w, 400, "statement_resolution_failed", err.Error())
	default:
		writeJSON(w, 200, result)
	}
}
