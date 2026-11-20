package credit

import (
	"regexp"
	"strings"
	"time"

	"tinynest/api/internal/money"
)

type StatementAmounts struct {
	NewCharges    *money.Minor `json:"new_charges_minor,omitempty"`
	AmountDue     *money.Minor `json:"amount_due_minor,omitempty"`
	StatementDate string       `json:"statement_date,omitempty"`
}

var statementAmountPair = regexp.MustCompile(`<(?:td|th)>\s*(本期新增消费及费用|新签金额\s*[/／]\s*其他费用|本期应还总额|本期应还款总额)[：:]?\s*</(?:td|th)>\s*<(?:td|th)>\s*([^<>]+?)\s*</(?:td|th)>`)
var statementClosingDate = regexp.MustCompile(`<(?:td|th)>\s*(?:账单日|账单日期)[：:]?\s*</(?:td|th)>\s*<(?:td|th)>\s*([0-9]{4}[-/][0-9]{2}[-/][0-9]{2})\s*</(?:td|th)>`)
var summaryCardTail = regexp.MustCompile(`(?:卡尾号|卡号后四位|卡号后4位)[：:]?\s*(?:</(?:td|th)>\s*<(?:td|th)>\s*)?([0-9]{4})(?:[^0-9]|$)`)

func StatementSummaryMatchesCard(body, tail string) bool {
	if len(body) > 2<<20 || len(tail) != 4 {
		return false
	}
	matches := summaryCardTail.FindAllStringSubmatch(statementTableMarkup(body), -1)
	if len(matches) == 0 {
		return false
	}
	for _, match := range matches {
		if match[1] != tail {
			return false
		}
	}
	return true
}

// ParseStatementAmounts accepts explicit summary labels followed by RMB values.
// Unlabeled, contradictory, and unsupported layouts remain unconfirmed.
func ParseStatementAmounts(body string) StatementAmounts {
	var result StatementAmounts
	if len(body) > 2<<20 {
		return result
	}
	amounts := make(map[string]money.Minor)
	conflicting := make(map[string]bool)
	markup := statementTableMarkup(body)
	for _, match := range statementClosingDate.FindAllStringSubmatch(markup, -1) {
		day, err := time.Parse(time.DateOnly, strings.ReplaceAll(match[1], "/", "-"))
		if err != nil || (result.StatementDate != "" && result.StatementDate != day.Format(time.DateOnly)) {
			result.StatementDate = ""
			break
		}
		result.StatementDate = day.Format(time.DateOnly)
	}
	for _, pair := range statementAmountPair.FindAllStringSubmatch(markup, -1) {
		label := strings.TrimRight(strings.Join(strings.Fields(pair[1]), ""), "：:")
		var key string
		switch label {
		case "本期新增消费及费用", "新签金额/其他费用", "新签金额／其他费用":
			key = "new"
		case "本期应还总额", "本期应还款总额":
			key = "due"
		default:
			continue
		}
		value, err := parseEverbrightAmount(strings.TrimPrefix(strings.TrimSpace(pair[2]), "人民币"))
		if err != nil || value < 0 {
			conflicting[key] = true
			continue
		}
		if previous, ok := amounts[key]; ok && previous != value {
			conflicting[key] = true
		}
		amounts[key] = value
	}
	if value, ok := amounts["new"]; ok && !conflicting["new"] {
		result.NewCharges = &value
	}
	if value, ok := amounts["due"]; ok && !conflicting["due"] {
		result.AmountDue = &value
	}
	return result
}
