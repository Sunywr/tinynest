package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"tinynest/api/internal/db"
	"tinynest/api/internal/money"
)

// 账单明细补录 is the path that turns the bank's own detail report into ledger
// lines. The browser reads the XLS cells and posts them verbatim; this file
// decides what each line MEANS, because only the server may write the ledger.
//
// The classification vocabulary is frozen (2026-11-26, user-confirmed):
// 消费 / 退款 / 还款或转入 / 费用利息 / 未识别. Only 消费 becomes a 5010 expense.
// A negative amount in a bank report is not proof of a refund, so refunds,
// repayments, fees and anything unrecognised are stored as held rows carrying a
// reason, and are never booked: the household decides those separately.
//
// Idempotency is per LINE, keyed by the reported date, amount, card tail,
// currency and description, so re-downloading the same report, or uploading it
// again after one line changed, cannot post an expense twice.

const (
	detailCategoryName  = "账单补录"
	detailCategoryCode  = "statement_detail"
	detailCategoryEmoji = "receipt"
	detailCategoryColor = "7"
	detailAuditAction   = "import_statement_details"
	detailKeyPrefix     = "statement-detail:"
	// maxDetailRows bounds one upload: a card bill is not thousands of lines, and
	// the request body is already capped by decode().
	maxDetailRows = 3000
)

// 浦发账单明细 column order, measured from the bank's own XLS layout.
const (
	detailColTxnOn = iota
	detailColPostOn
	detailColDesc
	detailColTail
	detailColCardKind
	detailColCurrency
	detailColAmount
)

// Kinds and states mirror the CHECK constraints of migration 014.
const (
	detailKindCharge  = "charge"
	detailKindRefund  = "refund"
	detailKindPayment = "payment"
	detailKindFee     = "fee"
	detailKindReview  = "review"
)

const (
	detailStateImport  = "imported"
	detailStateMatched = "matched"
	detailStateHeld    = "held"
	// detailStateMissing is analysis-only: a charge the ledger does not hold yet.
	// It is never persisted — either it becomes imported or the batch is refused.
	detailStateMissing = "missing"
)

// errDetailPlaceholderLive aborts the write transaction because a 待补明细
// transaction still reserves the cycle these details would book.
var errDetailPlaceholderLive = errors.New("api: cycle still has a live 待补明细 placeholder")

// Description vocabulary, matched against the wording the bank prints in 交易摘要.
var (
	detailRefundWords  = []string{"退款", "退货", "冲退"}
	detailPaymentWords = []string{"还款", "转入", "转账", "汇入", "充值", "存入", "赎回", "溢缴款"}
	detailFeeWords     = []string{"年费", "手续费", "利息", "违约金", "滞纳金", "超限费", "取现费", "分期费", "工本费", "服务费", "罚息"}
)

type statementDetailRequest struct {
	Rows        []statementDetailCells `json:"rows"`
	CardTail    string                 `json:"card_tail"`
	StatementID string                 `json:"statement_id"`
}

// statementDetailCells is one spreadsheet row. Cells arrive as whatever the
// browser's XLSX reader produced, so numbers, booleans and blanks all collapse
// to the text the bank printed.
type statementDetailCells []string

func (c *statementDetailCells) UnmarshalJSON(data []byte) error {
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	cells := make([]string, 0, len(raw))
	for _, value := range raw {
		cells = append(cells, detailCellText(value))
	}
	*c = cells
	return nil
}

func detailCellText(value any) string {
	switch cell := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(cell)
	case bool:
		if cell {
			return "1"
		}
		return ""
	case float64:
		return strconv.FormatFloat(cell, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprint(cell))
	}
}

type statementDetailLine struct {
	RowNo       int
	Seq         int
	RowKey      string
	TxnOn       string
	PostOn      string
	Desc        string
	Tail        string
	Currency    string
	AmountMinor int64
	Kind        string
	State       string
	Reason      string
	// LedgerTransaction names the transaction this line describes: either the one
	// the ledger already held, or the one this request booked.
	LedgerTransaction string
	Stored            bool
}

type statementDetailWindow struct {
	statementID string
	startOn     string
	endOn       string
}

type detailPlaceholder struct {
	transactionID string
	statementID   string
	amountMinor   int64
	startOn       string
	endOn         string
	dueOn         string
}

type statementDetailBatch struct {
	accountID      string
	lines          []statementDetailLine
	digest         string
	window         statementDetailWindow
	chargeMinor    int64
	missingMinor   int64
	bookedMinor    int64
	placeholder    detailPlaceholder
	hasPlaceholder bool
	stored         map[string]statementDetailLine
}

// rowDisposition tells the caller whether a spreadsheet row is data at all.
const (
	detailRowSkipped = iota
	detailRowData
)

func (a *API) handleStatementDetailPreview(w http.ResponseWriter, r *http.Request) {
	p, account, in, ok := a.readStatementDetails(w, r)
	if !ok {
		return
	}
	batch, err := a.analyseRequestDetails(r, p.HouseholdID, account, in)
	if err != nil {
		writeDetailLookupErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch.response(false, nil))
}

func (a *API) handleStatementDetailImport(w http.ResponseWriter, r *http.Request) {
	p, account, in, ok := a.readStatementDetails(w, r)
	if !ok {
		return
	}
	batch, err := a.analyseRequestDetails(r, p.HouseholdID, account, in)
	if err != nil {
		writeDetailLookupErr(w, err)
		return
	}
	// Booking real details in a cycle the placeholder still reserves would count
	// the same bill twice, and reversing the placeholder is the household's call.
	// So the import refuses, names the placeholder and writes nothing.
	if batch.hasPlaceholder && batch.bookedMinor > 0 {
		writeDetailPlaceholderConflict(w, batch.placeholder)
		return
	}
	summary, err := a.writeStatementDetails(r, p, account, in)
	if err != nil {
		if errors.Is(err, errDetailPlaceholderLive) {
			// Re-read after the rollback: a placeholder that appeared while this
			// batch ran is the reason nothing was written.
			live, lookupErr := a.loadLivePlaceholders(r.Context(), a.DB, p.HouseholdID, account.ID)
			if lookupErr != nil || len(live) == 0 {
				internalErr(w, err)
				return
			}
			writeDetailPlaceholderConflict(w, live[0])
			return
		}
		internalErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// readStatementDetails validates the request envelope and the card. Anything that
// is not this household's active credit card answers 404, so a guessed account id
// reveals nothing about the rest of the site.
func (a *API) readStatementDetails(w http.ResponseWriter, r *http.Request) (Principal, accountInfo, statementDetailRequest, bool) {
	p, _ := principal(r)
	var in statementDetailRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return p, accountInfo{}, in, false
	}
	if len(in.Rows) == 0 {
		writeErr(w, 400, "invalid_request", "没有读到明细行")
		return p, accountInfo{}, in, false
	}
	if len(in.Rows) > maxDetailRows {
		writeErr(w, 400, "too_many_rows", fmt.Sprintf("一次最多处理 %d 行明细", maxDetailRows))
		return p, accountInfo{}, in, false
	}
	if normalizeCardTail(in.CardTail) == "" {
		writeErr(w, 400, "invalid_request", "缺少卡号末四位，无法确认明细属于本卡")
		return p, accountInfo{}, in, false
	}
	account, err := loadActiveAccount(r.Context(), a.DB, p.HouseholdID, r.PathValue("id"))
	if err != nil || account.Kind != "credit" {
		writeErr(w, 404, "not_found", "账户不存在、已停用或不是信用卡")
		return p, accountInfo{}, in, false
	}
	return p, account, in, true
}

func writeDetailLookupErr(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "账单记录不存在")
		return
	}
	internalErr(w, err)
}

func writeDetailPlaceholderConflict(w http.ResponseWriter, live detailPlaceholder) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": map[string]string{
			"code":    "placeholder_not_reversed",
			"message": "该账期还有一笔未冲正的待补明细流水，补录会双计；请先冲正待补流水，再上传明细",
		},
		"placeholder": map[string]any{
			"transaction_id": live.transactionID, "statement_id": live.statementID,
			"amount_minor": live.amountMinor, "period_start_on": nullableText(live.startOn),
			"period_end_on": live.endOn, "due_on": nullableText(live.dueOn),
		},
	})
}

// analyseRequestDetails resolves the bill window and classifies the rows outside
// any transaction, which is exactly what the preview answers with.
func (a *API) analyseRequestDetails(r *http.Request, householdID string,
	account accountInfo, in statementDetailRequest) (*statementDetailBatch, error) {
	window, err := a.loadDetailWindow(r.Context(), a.DB, householdID, account.ID, strings.TrimSpace(in.StatementID))
	if err != nil {
		return nil, err
	}
	return a.analyseStatementDetails(r.Context(), a.DB, householdID, account, in, window)
}

func (a *API) loadDetailWindow(ctx context.Context, q db.Queryer, householdID, accountID, statementID string) (statementDetailWindow, error) {
	if statementID == "" {
		return statementDetailWindow{}, nil
	}
	var out statementDetailWindow
	var start sql.NullString
	err := q.QueryRowContext(ctx, `SELECT id, period_start_on, period_end_on FROM card_statements
		WHERE household_id=? AND account_id=? AND id=?`, householdID, accountID, statementID).
		Scan(&out.statementID, &start, &out.endOn)
	out.startOn = start.String
	return out, err
}

// analyseStatementDetails parses, classifies, de-duplicates and ledger-matches
// every row. Preview and import share it, so what the household confirmed is the
// batch that gets written.
func (a *API) analyseStatementDetails(ctx context.Context, q db.Queryer, householdID string,
	account accountInfo, in statementDetailRequest, window statementDetailWindow) (*statementDetailBatch, error) {
	wantTail := normalizeCardTail(in.CardTail)
	batch := &statementDetailBatch{accountID: account.ID, window: window, stored: map[string]statementDetailLine{}}
	seen := map[string]int{}
	for index, cells := range in.Rows {
		line, disposition := parseStatementDetailRow(cells, wantTail)
		if disposition != detailRowData {
			continue
		}
		line.RowNo = index + 1
		line.Seq = seen[line.RowKey]
		seen[line.RowKey]++
		batch.lines = append(batch.lines, line)
	}
	batch.digest = statementDetailDigest(account.ID, in, wantTail)
	if err := a.loadStoredDetails(ctx, q, batch, householdID, account.ID); err != nil {
		return nil, err
	}
	live, err := a.loadLivePlaceholders(ctx, q, householdID, account.ID)
	if err != nil {
		return nil, err
	}
	for i := range batch.lines {
		line := &batch.lines[i]
		if line.Kind == detailKindCharge {
			batch.chargeMinor += line.AmountMinor
		}
		if stored, ok := batch.stored[storedKey(line.RowKey, line.Seq)]; ok {
			line.Stored = true
			line.State = stored.State
			line.LedgerTransaction = stored.LedgerTransaction
			continue
		}
		if line.Kind != detailKindCharge {
			line.State = detailStateHeld
			continue
		}
		if line.AmountMinor <= 0 {
			line.State = detailStateHeld
			line.Reason = "消费金额为 0 或负数，未入账，请人工核对"
			continue
		}
		if window.statementID != "" && !withinWindow(line.TxnOn, window) {
			line.State = detailStateHeld
			line.Reason = "交易日期不在该账单账期（" + windowLabel(window) + "）内"
			continue
		}
		if placeholder, ok := coveringPlaceholder(live, line.TxnOn); ok {
			batch.placeholder, batch.hasPlaceholder = placeholder, true
		}
		match, err := a.findDetailMatch(ctx, q, householdID, account.ID, line)
		if err != nil {
			return nil, err
		}
		switch match.count {
		case 0:
			line.State = detailStateMissing
			batch.missingMinor += line.AmountMinor
			batch.bookedMinor += line.AmountMinor
		case 1:
			line.State = detailStateMatched
			line.LedgerTransaction = match.transactionID
		default:
			line.State = detailStateHeld
			line.Reason = "同一天同金额已有多笔流水，无法唯一对应，请人工核对"
		}
	}
	return batch, nil
}

type detailMatch struct {
	count         int
	transactionID string
}

// findDetailMatch locates the ledger row one bank line already describes. A
// 待补明细 placeholder is excluded because it stands in for the whole bill rather
// than for one merchant charge, and a transaction another detail row claims is
// excluded so two lines can never point at one booking.
func (a *API) findDetailMatch(ctx context.Context, q db.Queryer, householdID, accountID string,
	line *statementDetailLine) (detailMatch, error) {
	var out detailMatch
	rows, err := q.QueryContext(ctx, `SELECT t.id FROM transactions t
		WHERE t.household_id=? AND t.account_id=? AND t.type='expense' AND t.status='posted'
		  AND t.amount_minor=? AND t.occurred_on=?
		  AND NOT EXISTS (SELECT 1 FROM statement_placeholders p
		                    WHERE p.household_id=t.household_id AND p.transaction_id=t.id)
		  AND NOT EXISTS (SELECT 1 FROM statement_details d
		                    WHERE d.household_id=t.household_id AND d.transaction_id=t.id)
		LIMIT 3`, householdID, accountID, line.AmountMinor, line.TxnOn)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return out, err
		}
		out.count++
		out.transactionID = id
	}
	return out, rows.Err()
}

// loadStoredDetails remembers the lines this card already imported. The key is
// row_key+seq, so a re-download only re-posts the lines that are genuinely new.
func (a *API) loadStoredDetails(ctx context.Context, q db.Queryer, batch *statementDetailBatch,
	householdID, accountID string) error {
	rows, err := q.QueryContext(ctx, `SELECT row_key,seq,state,transaction_id FROM statement_details
		WHERE household_id=? AND account_id=?`, householdID, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, state string
		var seq int
		var transactionID sql.NullString
		if err := rows.Scan(&key, &seq, &state, &transactionID); err != nil {
			return err
		}
		batch.stored[storedKey(key, seq)] = statementDetailLine{
			RowKey: key, Seq: seq, State: state, LedgerTransaction: transactionID.String,
		}
	}
	return rows.Err()
}

func storedKey(rowKey string, seq int) string { return rowKey + ":" + strconv.Itoa(seq) }

// loadLivePlaceholders returns the 待补明细 transactions that still stand for this
// card together with the bank period each one reserves. Only posted rows count: a
// reversed placeholder no longer holds any cash back.
func (a *API) loadLivePlaceholders(ctx context.Context, q db.Queryer, householdID, accountID string) ([]detailPlaceholder, error) {
	rows, err := q.QueryContext(ctx, `SELECT p.transaction_id, p.statement_id, tx.amount_minor,
			p.period_start_on, p.period_end_on, p.due_on
		FROM statement_placeholders p
		JOIN transactions tx ON tx.household_id=p.household_id AND tx.id=p.transaction_id
		JOIN card_statements s ON s.household_id=p.household_id AND s.id=p.statement_id
		WHERE p.household_id=? AND s.account_id=? AND tx.status='posted'
		ORDER BY p.period_end_on DESC`, householdID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []detailPlaceholder{}
	for rows.Next() {
		var item detailPlaceholder
		var start, due sql.NullString
		if err := rows.Scan(&item.transactionID, &item.statementID, &item.amountMinor,
			&start, &item.endOn, &due); err != nil {
			return nil, err
		}
		item.startOn, item.dueOn = start.String, due.String
		out = append(out, item)
	}
	return out, rows.Err()
}

func coveringPlaceholder(live []detailPlaceholder, txnOn string) (detailPlaceholder, bool) {
	for _, item := range live {
		start := item.startOn
		if start == "" {
			start = "0001-01-01"
		}
		if txnOn >= start && txnOn <= item.endOn {
			return item, true
		}
	}
	return detailPlaceholder{}, false
}

func withinWindow(txnOn string, window statementDetailWindow) bool {
	start := window.startOn
	if start == "" {
		start = "0001-01-01"
	}
	return txnOn >= start && txnOn <= window.endOn
}

func windowLabel(window statementDetailWindow) string {
	if window.startOn == "" {
		return "至 " + window.endOn
	}
	return window.startOn + " 至 " + window.endOn
}

// parseStatementDetailRow converts one spreadsheet row into a classified line.
// The report's own header and any fully blank row are reported as skipped so they
// are neither counted nor stored as a transaction.
func parseStatementDetailRow(cells statementDetailCells, wantTail string) (statementDetailLine, int) {
	cell := func(index int) string {
		if index < len(cells) {
			return cells[index]
		}
		return ""
	}
	rawDate, desc := cell(detailColTxnOn), cell(detailColDesc)
	if isDetailHeader(rawDate, desc) {
		return statementDetailLine{}, detailRowSkipped
	}
	amount, parsed := parseDetailAmount(cell(detailColAmount))
	txnOn := normalizeDetailDate(rawDate)
	if txnOn == "" && !parsed && strings.TrimSpace(strings.Join(cells, "")) == "" {
		return statementDetailLine{}, detailRowSkipped
	}
	line := statementDetailLine{
		TxnOn: txnOn, PostOn: normalizeDetailDate(cell(detailColPostOn)), Desc: desc,
		Tail: normalizeCardTail(cell(detailColTail)), Currency: normalizeDetailCurrency(cell(detailColCurrency)),
		AmountMinor: amount,
	}
	line.Kind, line.Reason = classifyDetailLine(line, wantTail, parsed)
	line.RowKey = statementDetailRowKey(line, rawDate, cell(detailColAmount), parsed)
	return line, detailRowData
}

// isDetailHeader recognises the header line the browser's sheet reader hands over
// together with the data.
func isDetailHeader(rawDate, desc string) bool {
	return strings.Contains(rawDate, "日期") || strings.Contains(desc, "摘要")
}

func classifyDetailLine(line statementDetailLine, wantTail string, parsed bool) (string, string) {
	switch {
	case wantTail != "" && line.Tail != "" && line.Tail != wantTail:
		return detailKindReview, "卡号末四位与本卡不符，未入账"
	case !parsed:
		return detailKindReview, "交易金额无法识别，未入账"
	case line.Currency != "CNY":
		return detailKindReview, "非人民币交易需人工确认汇率，未入账"
	case containsAny(line.Desc, detailRefundWords):
		return detailKindRefund, "退款不自动入账，请单独确认处理方式"
	case containsAny(line.Desc, detailPaymentWords):
		return detailKindPayment, "还款或转入不是消费，不计入支出，请按转账登记"
	case containsAny(line.Desc, detailFeeWords):
		return detailKindFee, "费用与利息不自动入账，请人工确认后单独记账"
	case line.AmountMinor < 0:
		return detailKindReview, "金额为负但摘要未标明退款，需人工核对"
	case line.TxnOn == "":
		return detailKindReview, "交易日期无法识别，未入账"
	default:
		return detailKindCharge, ""
	}
}

func containsAny(text string, words []string) bool {
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

// statementDetailRowKey is the per-line idempotency key. seq then distinguishes
// two genuinely identical charges on one day instead of collapsing them into one
// row, so neither is silently dropped.
func statementDetailRowKey(line statementDetailLine, rawDate, rawAmount string, parsed bool) string {
	date := line.TxnOn
	if date == "" {
		date = "raw:" + rawDate
	}
	amount := strconv.FormatInt(line.AmountMinor, 10)
	if !parsed {
		amount = "raw:" + rawAmount
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{date, amount, line.Tail, line.Currency, line.Desc}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// statementDetailDigest names one uploaded report for traceability. It covers the
// card, the tail the operator confirmed and every cell, so re-uploading the same
// file is recognised while a corrected export is recorded as a new source.
func statementDetailDigest(accountID string, in statementDetailRequest, wantTail string) string {
	parts := make([]string, 0, len(in.Rows)+2)
	parts = append(parts, "card="+accountID, "tail="+wantTail)
	for _, cells := range in.Rows {
		parts = append(parts, strings.Join(cells, "\x1e"))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func normalizeCardTail(raw string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	if len(digits) > 4 {
		digits = digits[len(digits)-4:]
	}
	return digits
}

// normalizeDetailDate accepts the forms bank reports print: 20261020,
// 2026-10-20, 2026/10/20 and 2026.10.20. Anything else is not a date.
func normalizeDetailDate(raw string) string {
	text := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.TrimSpace(raw))
	if len(text) != 8 {
		return ""
	}
	year, err := strconv.Atoi(text[:4])
	if err != nil || year < 1900 || year > 2999 {
		return ""
	}
	month, err := strconv.Atoi(text[4:6])
	if err != nil || month < 1 || month > 12 {
		return ""
	}
	day, err := strconv.Atoi(text[6:])
	if err != nil || day < 1 || day > 31 {
		return ""
	}
	out := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	if !validDate(out) {
		return ""
	}
	return out
}

// parseDetailAmount reads the bank's printed decimal into integer cents without
// floating point. Thousands separators, a currency sign, an accounting-style
// trailing minus and parentheses are accepted; an empty cell is not a number.
func parseDetailAmount(raw string) (int64, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	for _, drop := range []string{",", "，", " ", "　", "￥", "¥"} {
		text = strings.ReplaceAll(text, drop, "")
	}
	// Accounting style: a trailing minus or parentheses both mean debit-side
	// negative, while a leading sign is the bank's own and ParseYuan reads it.
	negative := strings.HasSuffix(text, "-") || (strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")"))
	text = strings.TrimSuffix(text, "-")
	text = strings.Trim(text, "()")
	value, err := money.ParseYuan(text)
	if err != nil {
		return 0, false
	}
	if negative {
		return -int64(value), true
	}
	return int64(value), true
}

func normalizeDetailCurrency(raw string) string {
	switch strings.TrimSpace(raw) {
	case "", "人民币", "人民币元", "CNY", "cny", "RMB", "rmb", "元", "￥", "¥":
		return "CNY"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}

// response renders the body preview and import share. created, transactions and
// amount_minor only appear on the write path, where they say what this request
// actually booked.
func (b *statementDetailBatch) response(written bool, booked []string) map[string]any {
	counts := map[string]int64{"rows": int64(len(b.lines)), "held": 0, "matched": 0, "missing": 0, "skipped": 0}
	items := make([]map[string]any, 0, len(b.lines))
	for _, line := range b.lines {
		counts[detailKindCountKey(line.Kind)]++
		switch line.State {
		case detailStateMatched:
			counts["matched"]++
		case detailStateMissing:
			counts["missing"]++
		case detailStateHeld:
			counts["held"]++
		}
		if line.Stored {
			counts["skipped"]++
		}
		items = append(items, line.json())
	}
	out := map[string]any{
		"counts": counts, "items": items, "account_id": b.accountID,
		"statement_id": nullableText(b.window.statementID),
		"charge_minor": b.chargeMinor, "missing_minor": b.missingMinor,
	}
	if written {
		out["created"] = int64(len(booked))
		out["matched"] = counts["matched"]
		out["held"] = counts["held"]
		out["skipped"] = counts["skipped"]
		out["amount_minor"] = b.bookedMinor
		out["transactions"] = booked
		out["replayed"] = len(booked) == 0 && counts["skipped"] > 0
	}
	if b.hasPlaceholder {
		out["placeholder"] = map[string]any{
			"transaction_id": b.placeholder.transactionID, "statement_id": b.placeholder.statementID,
			"amount_minor": b.placeholder.amountMinor, "due_on": nullableText(b.placeholder.dueOn),
			"period_end_on": b.placeholder.endOn,
		}
	}
	return out
}

func detailKindCountKey(kind string) string {
	switch kind {
	case detailKindCharge:
		return "charges"
	case detailKindRefund:
		return "refunds"
	case detailKindPayment:
		return "payments"
	case detailKindFee:
		return "fees"
	default:
		return "review"
	}
}

func (l statementDetailLine) json() map[string]any {
	return map[string]any{
		"row_no": l.RowNo, "seq": l.Seq, "txn_on": nullableText(l.TxnOn),
		"post_on": nullableText(l.PostOn), "description": l.Desc, "card_tail": l.Tail,
		"currency": l.Currency, "amount_minor": l.AmountMinor, "kind": l.Kind,
		"state": l.State, "reason": l.Reason, "already_imported": l.Stored,
		"transaction_id": nullableText(l.LedgerTransaction),
	}
}

// writeStatementDetails books the missing charges and stores every parsed line in
// ONE database transaction, so a failure anywhere leaves no half-imported bill.
// The analysis is repeated under that transaction: a placeholder or a concurrent
// upload that appeared since the preview is caught before it can double-count.
func (a *API) writeStatementDetails(r *http.Request, p Principal, account accountInfo,
	in statementDetailRequest) (map[string]any, error) {
	var summary map[string]any
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		window, err := a.loadDetailWindow(r.Context(), tx, p.HouseholdID, account.ID, strings.TrimSpace(in.StatementID))
		if err != nil {
			return err
		}
		batch, err := a.analyseStatementDetails(r.Context(), tx, p.HouseholdID, account, in, window)
		if err != nil {
			return err
		}
		if batch.hasPlaceholder && batch.bookedMinor > 0 {
			return errDetailPlaceholderLive
		}
		categoryID, err := a.statementDetailCategory(r.Context(), tx, p.HouseholdID)
		if err != nil {
			return err
		}
		booked := []string{}
		var postedMinor int64
		stored := 0
		for i := range batch.lines {
			line := batch.lines[i]
			if line.Stored {
				continue
			}
			if line.State == detailStateMissing {
				transactionID, err := a.bookStatementDetail(r, tx, p, account, line, categoryID)
				if err != nil {
					return err
				}
				line.State = detailStateImport
				line.LedgerTransaction = transactionID
				batch.lines[i] = line
				booked = append(booked, transactionID)
				postedMinor += line.AmountMinor
			}
			if err := insertStatementDetail(r.Context(), tx, p.HouseholdID, account.ID, line, batch, a.nowUTC()); err != nil {
				return err
			}
			stored++
		}
		// Only what this request posted may be reported as 已入账: the file also
		// carries lines booked by an earlier upload, and counting those would
		// claim money that did not move now.
		batch.bookedMinor = postedMinor
		reported, err := a.recordStatementDetailSource(r.Context(), tx, p.HouseholdID, batch)
		if err != nil {
			return err
		}
		if stored == 0 && !reported {
			summary = batch.response(true, booked)
			return nil
		}
		if err := audit(r.Context(), tx, p, detailAuditAction, "account", account.ID,
			fmt.Sprintf(`{"rows":%d,"created":%d,"stored":%d,"amount_minor":%d,"statement_id":%q}`,
				len(batch.lines), len(booked), stored, batch.bookedMinor, batch.window.statementID)); err != nil {
			return err
		}
		summary = batch.response(true, booked)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return summary, nil
}

// bookStatementDetail writes one imported charge. The transaction row, its two
// journal lines and the audit trail all ride on the caller's transaction, so the
// ledger never sees an expense without its entry.
func (a *API) bookStatementDetail(r *http.Request, tx *sql.Tx, p Principal, account accountInfo,
	line statementDetailLine, categoryID string) (string, error) {
	accountID := account.ID
	values := transactionValues{
		Type:       "expense",
		Amount:     money.Minor(line.AmountMinor),
		OccurredOn: line.TxnOn,
		CategoryID: &categoryID,
		AccountID:  &accountID,
	}
	transactionID, entryID := newID("tx"), newID("je")
	key := detailKeyPrefix + line.RowKey
	if line.Seq > 0 {
		key += "#" + strconv.Itoa(line.Seq)
	}
	entry, err := a.buildTransactionEntry(r.Context(), tx, p.HouseholdID, values, transactionID, entryID)
	if err != nil {
		return "", err
	}
	if err := db.SaveEntry(r.Context(), tx, entry, p.HouseholdID, key, p.Username); err != nil {
		return "", err
	}
	if err := insertTransactionRow(r.Context(), tx, p.HouseholdID, values, transactionID, entryID, key, p.Username); err != nil {
		return "", err
	}
	return transactionID, nil
}

func insertStatementDetail(ctx context.Context, tx *sql.Tx, householdID, accountID string,
	line statementDetailLine, batch *statementDetailBatch, now string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO statement_details
		(id,household_id,account_id,statement_id,transaction_id,row_key,row_no,seq,card_tail,kind,state,
		 txn_on,post_on,description,amount_minor,currency,review_reason,source_digest,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		newID("sd"), householdID, accountID, nullableText(batch.window.statementID),
		nullableText(line.LedgerTransaction), line.RowKey, line.RowNo, line.Seq, line.Tail,
		line.Kind, line.State, line.TxnOn, nullableText(line.PostOn), line.Desc, line.AmountMinor,
		line.Currency, line.Reason, batch.digest, now)
	return err
}

// recordStatementDetailSource keeps one traceability row per uploaded report, so
// the bill shows that details arrived by file rather than by mailbox. The digest
// is unique per household, which recognises a re-upload instead of counting the
// same file twice.
func (a *API) recordStatementDetailSource(ctx context.Context, tx *sql.Tx, householdID string,
	batch *statementDetailBatch) (bool, error) {
	if batch.window.statementID == "" {
		return false, nil
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM card_statement_sources
		WHERE household_id=? AND digest=?`, householdID, batch.digest).Scan(&existing); err != nil {
		return false, err
	}
	if existing > 0 {
		return false, nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO card_statement_sources
		(id,household_id,statement_id,kind,digest,created_at) VALUES(?,?,?,?,?,?)`,
		newID("css"), householdID, batch.window.statementID, "xls", batch.digest, a.nowUTC())
	return err == nil, err
}

// statementDetailCategory returns the expense category imported charges are
// filed under. 账单补录 exists on first use, like 待补明细: no household gets a
// silent default category, and a category the user deleted earlier is revived
// because the import cannot be booked without it. The category is not a request
// parameter — the household re-files a line afterwards with the normal
// transaction PATCH, so an upload can never book into an arbitrary category.
func (a *API) statementDetailCategory(ctx context.Context, tx *sql.Tx, householdID string) (string, error) {
	id := "cat_" + householdID + "_" + detailCategoryCode
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
		VALUES(?,?,?,?,?,?,?)`, id, householdID, "expense", detailCategoryName,
		sort, detailCategoryEmoji, detailCategoryColor)
	return id, err
}
