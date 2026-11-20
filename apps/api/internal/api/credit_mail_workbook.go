package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"tinynest/api/internal/credit"
	"tinynest/api/internal/money"
)

func (a *API) importCreditWorkbook(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	card := r.PathValue("id")
	var name, issuer, boundTail string
	err := a.DB.QueryRowContext(ctx, `SELECT i.name,COALESCE(i.issuer,''),COALESCE(b.card_tail,'')
 FROM credit_instruments i LEFT JOIN credit_mail_bindings b ON b.instrument_id=i.id AND b.household_id=i.household_id
 WHERE i.id=? AND i.household_id=? AND i.kind='credit_card' AND i.status='active'`, card, p.HouseholdID).Scan(&name, &issuer, &boundTail)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "missing_card", "信用卡不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	bank := statementBank(issuer, name)
	if bank != "spdb" && bank != "citic" && bank != "cmb" {
		writeErr(w, 400, "unsupported_bank", "请选择浦发、中信或招商信用卡")
		return
	}
	var in struct {
		Tail string     `json:"card_tail"`
		Rows [][]string `json:"rows"`
	}
	if decode(r, &in) != nil || !workbookTail(in.Tail) || (boundTail != "" && in.Tail != boundTail) {
		writeErr(w, 400, "invalid_workbook", "请填写与信用卡绑定一致的四位卡尾号")
		return
	}
	rows, err := parseCreditWorkbook(bank, in.Tail, in.Rows)
	if err != nil {
		writeErr(w, 400, "invalid_workbook", err.Error())
		return
	}
	saved, err := readSavedCreditPreview(ctx, a.DB, p.HouseholdID, card)
	if err != nil {
		internalErr(w, err)
		return
	}
	latest := ""
	for _, row := range rows {
		if row.PostingDate > latest {
			latest = row.PostingDate
		}
	}
	found := false
	for i := range saved.Statements {
		if slices.Equal(saved.Statements[i].Transactions, rows) {
			found = true
		}
		saved.Statements[i].Review = nil
	}
	if !found {
		var count int
		for _, stmt := range saved.Statements {
			count += len(stmt.Transactions)
		}
		if count+len(rows) > 10000 {
			writeErr(w, 400, "preview_limit", "预览明细已达上限，请先处理并清空预览")
			return
		}
		saved.Statements = append([]everbrightPreviewStatement{{ReceivedOn: latest, Transactions: rows, Workbook: true}}, saved.Statements...)
	}
	if err := a.reviewEverbrightStatements(ctx, p.HouseholdID, card, saved.Statements); err != nil {
		internalErr(w, err)
		return
	}
	saved.SynchronizedAt = a.nowUTC()
	err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return saveImportedCreditPreview(ctx, a, tx, p.HouseholdID, card, saved)
	})
	if errors.Is(err, errConflict) {
		writeErr(w, 409, "preview_changed", "预览已更新，请重新打开后导入")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, saved)
}

func saveImportedCreditPreview(ctx context.Context, a *API, tx *sql.Tx, household, card string, saved savedCreditPreview) error {
	current, err := readSavedCreditPreview(ctx, tx, household, card)
	if err != nil {
		return err
	}
	if current.Generation != saved.Generation {
		return errConflict
	}
	return a.writeSavedCreditPreview(ctx, tx, household, card, saved)
}

func workbookTail(s string) bool { return len(s) == 4 && strings.Trim(s, "0123456789") == "" }

func parseCreditWorkbook(bank, tail string, data [][]string) ([]credit.EverbrightTransaction, error) {
	if len(data) > 2000 {
		return nil, errors.New("单个账单文件最多支持 2000 行")
	}
	headers := []string{"交易日期", "记账日期", "交易摘要", "卡号末四位", "交易币种", "交易金额"}
	if bank == "citic" || bank == "cmb" {
		headers = []string{"交易日期", "入账日期", "交易描述", "卡末四位", "结算币种", "结算金额"}
	}
	var indexes []int
	var rows []credit.EverbrightTransaction
	for _, cells := range data {
		if len(cells) > 32 {
			return nil, errors.New("账单列数超出范围")
		}
		if len(indexes) == 0 {
			byName := map[string]int{}
			for i, cell := range cells {
				byName[strings.TrimSpace(cell)] = i
			}
			for _, name := range headers {
				i, ok := byName[name]
				if !ok {
					indexes = nil
					break
				}
				indexes = append(indexes, i)
			}
			continue
		}
		if len(cells) == 0 || strings.TrimSpace(cells[0]) == "" {
			continue
		}
		values := make([]string, len(indexes))
		for i, j := range indexes {
			if j >= len(cells) {
				return nil, errors.New("账单明细缺少列")
			}
			values[i] = strings.TrimSpace(cells[j])
		}
		if !workbookTail(values[3]) {
			return nil, errors.New("账单卡尾号格式不正确")
		}
		if values[3] != tail || (values[4] != "人民币" && values[4] != "CNY") {
			continue
		}
		txn, err := workbookDate(values[0])
		if err != nil {
			return nil, err
		}
		posted, err := workbookDate(values[1])
		if err != nil {
			return nil, err
		}
		amount, err := money.ParseYuan(strings.ReplaceAll(values[5], ",", ""))
		if err != nil || amount == 0 || values[2] == "" || len(values[2]) > 512 {
			return nil, errors.New("账单金额或交易摘要无效")
		}
		rows = append(rows, credit.EverbrightTransaction{TransactionDate: txn, PostingDate: posted, CardTail: tail, Description: values[2], Amount: amount})
	}
	if len(indexes) == 0 {
		return nil, errors.New("文件表头与所选银行账单不匹配")
	}
	if len(rows) == 0 {
		return nil, errors.New("文件中没有匹配卡尾号的人民币明细")
	}
	return rows, nil
}

func workbookDate(s string) (string, error) {
	for _, layout := range []string{time.DateOnly, "20060102"} {
		date, err := time.Parse(layout, s)
		if err == nil {
			return date.Format(time.DateOnly), nil
		}
	}
	return "", errors.New("账单日期格式错误")
}
