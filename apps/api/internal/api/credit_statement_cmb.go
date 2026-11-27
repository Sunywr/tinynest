package api

import (
	"html"
	"regexp"
	"strings"
	"time"
)

var cmbSummaryField = regexp.MustCompile(`(?is)<(?:div|span)\s+(?:[^>]*\s)?id\s*=\s*["'](statementCycle|paymentDueDate|L1rmbLcurrBal)["'][^>]*>(.*?)</(?:div|span)\s*>`)

func parseCMBHTMLSummary(fields map[string]string, out statementMailPreview) statementMailPreview {
	out.NeedsReview = "招行邮件提供的是账户汇总，无法确认应还金额只属于当前信用卡"
	startText, endText, ok := strings.Cut(fields["statementcycle"], "-")
	if ok {
		start, startErr := time.Parse("2006/1/2", strings.TrimSpace(startText))
		end, endErr := time.Parse("2006/1/2", strings.TrimSpace(endText))
		if startErr == nil && endErr == nil && !end.Before(start) {
			out.PeriodStart = start.Format(time.DateOnly)
			out.StatementOn = end.Format(time.DateOnly)
		}
	}
	if due, err := time.Parse("2006/1/2", fields["paymentduedate"]); err == nil {
		out.DueOn = due.Format(time.DateOnly)
	}
	amount := fields["l1rmblcurrbal"]
	if strings.HasPrefix(amount, "¥") || strings.HasPrefix(amount, "￥") {
		out.AmountDue = statementMinor(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(amount, "¥"), "￥")))
	}
	if out.AmountDue == nil || out.StatementOn == "" || out.DueOn == "" || out.DueOn < out.StatementOn {
		out.NeedsReview = "请核对招行邮件中的账单周期、到期还款日和人民币应还金额"
	}
	return out
}

func collectCMBSummaryFields(fields map[string]string, source string) {
	for _, match := range cmbSummaryField.FindAllStringSubmatch(source, -1) {
		key := strings.ToLower(match[1])
		value := strings.TrimSpace(html.UnescapeString(statementHTMLTag.ReplaceAllString(match[2], "")))
		if previous, exists := fields[key]; exists && previous != value {
			fields[key] = ""
		} else {
			fields[key] = value
		}
	}
}
