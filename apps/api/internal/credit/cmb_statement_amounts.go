package credit

import (
	"html"
	"regexp"
	"strings"
	"time"
)

var cmbCycleEnd = regexp.MustCompile(`账单周期\s*(?:Statement Cycle)?\s*</(?:td|th)>\s*<td>\s*[0-9]{4}/[0-9]{2}/[0-9]{2}\s*[-~至－]\s*([0-9]{4}/[0-9]{2}/[0-9]{2})\s*</td>`)
var cmbSummaryRows = regexp.MustCompile(`(?s)<tr>((?:\s*<(?:td|th)>[^<>]*</(?:td|th)>)+)\s*</tr>`)
var cmbSummaryCells = regexp.MustCompile(`<(?:td|th)>([^<>]*)</(?:td|th)>`)

// ParseCMBStatementAmounts reads the account reconciliation table independently
// of transaction row validation.
func ParseCMBStatementAmounts(body string) StatementAmounts {
	var result StatementAmounts
	if len(body) > 2<<20 {
		return result
	}
	markup := strings.ReplaceAll(html.UnescapeString(statementTableMarkup(body)), "\u00a0", " ")
	days := cmbCycleEnd.FindAllStringSubmatch(markup, -1)
	if len(days) == 0 {
		return result
	}
	day, err := time.Parse("2006/01/02", days[0][1])
	if err != nil {
		return result
	}
	for _, match := range days[1:] {
		if match[1] != days[0][1] {
			return result
		}
	}
	result.StatementDate = day.Format(time.DateOnly)
	rows := cmbSummaryRows.FindAllStringSubmatch(markup, -1)
	for i := 0; i+1 < len(rows); i++ {
		header := cmbSummaryCells.FindAllStringSubmatch(rows[i][1], -1)
		values := cmbSummaryCells.FindAllStringSubmatch(rows[i+1][1], -1)
		if len(header) < 6 {
			continue
		}
		if !strings.Contains(strings.Join(strings.Fields(header[0][1]), " "), "New Balance") ||
			!strings.Contains(strings.Join(strings.Fields(rows[i][1]), " "), "New Charges") {
			continue
		}
		var rmb []string
		for _, cell := range values {
			if value := strings.TrimSpace(cell[1]); cmbRMBValue(value) {
				rmb = append(rmb, value)
			}
		}
		if len(rmb) != 6 {
			continue
		}
		dueText, chargeText := rmb[0], rmb[3]
		due, dueErr := parseEverbrightAmount(dueText)
		charges, chargeErr := parseEverbrightAmount(chargeText)
		if dueErr != nil || chargeErr != nil || charges < 0 {
			continue
		}
		if result.NewCharges != nil && (*result.NewCharges != charges || *result.AmountDue != due) {
			return StatementAmounts{StatementDate: result.StatementDate}
		}
		result.NewCharges = &charges
		result.AmountDue = &due
	}
	return result
}

func cmbRMBValue(value string) bool {
	return strings.HasPrefix(value, "¥") || strings.HasPrefix(value, "￥")
}
