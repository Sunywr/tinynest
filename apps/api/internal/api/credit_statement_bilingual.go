package api

import (
	"regexp"
	"strings"
	"time"

	"tinynest/api/internal/money"
)

const bankSlashDate = `[0-9]{4}/[0-9]{1,2}/[0-9]{1,2}`
const bankDecimal = `[+-]?[0-9][0-9,]*(?:\.[0-9]{1,2})?`
const gdbSummaryHeader = "卡号末四位 本期账单金额 最低还款额 最后还款日 入账货币 存款 卡片消费额度"
const cebSummaryHeader = "账号 Account Number 账户名称 Card Name 本期余额 Total Balance 本期应还款额 Statement Balance 本期最低还款额 Minimum Payment Due"

var (
	bankCycle        = regexp.MustCompile(`账单周期\s*[:：]?\s*(?:Statement Cycle\s*)?(` + bankSlashDate + `)\s*-\s*(` + bankSlashDate + `)(?:\s|$)`)
	gdbSummaryRow    = regexp.MustCompile(`^([0-9]{4})\s+(无欠款|` + bankDecimal + `)\s+(?:无欠款|` + bankDecimal + `)\s+(` + bankSlashDate + `)\s+(\S+)\s+` + bankDecimal + `\s+` + bankDecimal + `(?:\s|$)`)
	cebStatementDate = regexp.MustCompile(`账单日\s+Statement Date\s+` + statementDateLayout)
	cebDueDate       = regexp.MustCompile(`到期还款日\s+Payment Due Date\s+` + statementDateLayout)
	cebRMBAmount     = regexp.MustCompile(`人民币本期应还款额\s+RMB Statement Balance\s+[¥￥]\s*(` + bankDecimal + `)(?:\s|$)`)
	cebMaskedCard    = regexp.MustCompile(`\b[0-9]{8}[*＊]{4}([0-9]{4})\b`)
	cebAccountRow    = regexp.MustCompile(`^[0-9]{8}[*＊]{4}([0-9]{4})\s+[^0-9]{1,100}?\s+` + bankDecimal + `\s+(` + bankDecimal + `)\s+` + bankDecimal + `(?:\s|$)`)
)

func bankSlashDay(value string) string {
	day, err := time.Parse("2006/1/2", value)
	if err != nil {
		return ""
	}
	return day.Format(time.DateOnly)
}

func statementExplicitStart(body, close string) string {
	cycles := bankCycle.FindAllStringSubmatch(body, -1)
	if len(cycles) != 1 || close == "" || bankSlashDay(cycles[0][2]) != close {
		return ""
	}
	start := bankSlashDay(cycles[0][1])
	if start > close {
		return ""
	}
	return start
}

func parseGDBSummary(out statementMailPreview, body, rows, tail string) statementMailPreview {
	out.NeedsReview = "请核对广发卡尾、币种、账单周期和最后还款日"
	out.StatementOn = statementFindDate(statementClosingLabel, body)
	out.PeriodStart = statementExplicitStart(body, out.StatementOn)

	matches := 0
	for {
		row := gdbSummaryRow.FindStringSubmatch(strings.TrimSpace(rows))
		if row == nil {
			break
		}
		rows = strings.TrimSpace(rows)[len(row[0]):]
		if row[1] != tail || row[4] != "人民币" {
			continue
		}
		matches++
		out.DueOn = bankSlashDay(row[3])
		if row[2] == "无欠款" {
			var zero money.Minor
			out.AmountDue = &zero
		} else {
			out.AmountDue = statementMinor(row[2])
		}
	}
	if matches == 1 && out.AmountDue != nil && out.PeriodStart != "" && out.DueOn >= out.StatementOn {
		out.NeedsReview = ""
	} else {
		out.AmountDue = nil
	}
	return out
}

func parseCEBSummary(out statementMailPreview, body, rows, tail string) statementMailPreview {
	out.NeedsReview = "请核对光大卡尾、账单周期和人民币应还金额"
	out.StatementOn = statementFindDate(cebStatementDate, body)
	out.PeriodStart = statementExplicitStart(body, out.StatementOn)
	out.DueOn = statementFindDate(cebDueDate, body)

	amounts := cebRMBAmount.FindAllStringSubmatch(body, -1)
	row := cebAccountRow.FindStringSubmatch(strings.TrimSpace(rows))
	if len(amounts) != 1 || row == nil || row[1] != tail {
		return out
	}
	total := statementMinor(amounts[0][1])
	account := statementMinor(row[2])
	if total == nil || account == nil || *total != *account {
		return out
	}
	for _, mask := range cebMaskedCard.FindAllStringSubmatch(body, -1) {
		if mask[1] != tail {
			return out
		}
	}
	out.AmountDue = account
	if out.PeriodStart != "" && out.DueOn >= out.StatementOn {
		out.NeedsReview = ""
	}
	return out
}
