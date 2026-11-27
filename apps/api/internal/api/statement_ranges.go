package api

import (
	"net/http"
	"strings"
	"time"
)

type statementDateRange struct {
	start, end time.Time
}

func (period statementDateRange) includes(date string) bool {
	return date >= period.start.Format(time.DateOnly) && date <= period.end.Format(time.DateOnly)
}

const statementDeliveryGraceDays = 7

func (period statementDateRange) matchesMail(item statementMailPreview) bool {
	if item.StatementOn != "" {
		return period.includes(item.StatementOn)
	}
	return item.ReceivedOn >= period.start.Format(time.DateOnly) &&
		item.ReceivedOn <= period.end.AddDate(0, 0, statementDeliveryGraceDays).Format(time.DateOnly)
}

func requestedStatementRanges(w http.ResponseWriter, r *http.Request) ([]statementDateRange, bool) {
	values := r.URL.Query()["range"]
	if len(values) <= 12 {
		ranges := make([]statementDateRange, 0, len(values))
		for _, value := range values {
			startText, endText, separated := strings.Cut(value, ",")
			start, startErr := time.Parse(time.DateOnly, startText)
			end, endErr := time.Parse(time.DateOnly, endText)
			if !separated || startErr != nil || endErr != nil || end.Before(start) || end.After(start.AddDate(0, 0, 62)) {
				break
			}
			ranges = append(ranges, statementDateRange{start: start, end: end})
		}
		if len(ranges) == len(values) {
			return ranges, true
		}
	}
	writeErr(w, 400, "invalid_request", "最多选择 12 个账单日期范围，每段不超过 63 天")
	return nil, false
}
