package api

import (
	"database/sql"
	"net/http"
	"sort"
)

// V0.1.3 preflight: dividend history + annual summary. Read-only, household
// scoped in SQL, no migration and no journal changes. Works on both SQLite and
// the MySQL production path (plain SQL, no dialect-specific functions).
func (a *API) registerV013(mux *http.ServeMux) {
	a.route(mux, "GET /api/v1/investments/dividends", a.handleListDividends)
}

type dividendYear struct {
	year         string
	gross        int64
	tax          int64
	net          int64
	unknownGross int64
	count        int64
	unknown      int64
}

func (a *API) handleListDividends(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	q := r.URL.Query()
	assetID, from, to := q.Get("asset_id"), q.Get("from"), q.Get("to")
	if (from != "" && !validDate(from)) || (to != "" && !validDate(to)) {
		writeErr(w, 400, "invalid_request", "日期格式不正确")
		return
	}
	if from != "" && to != "" && from > to {
		writeErr(w, 400, "invalid_request", "起始日期不能晚于截止日期")
		return
	}

	query := `
		SELECT d.id, d.asset_id, a.code, a.name, d.received_on, d.gross_minor,
		       d.tax_minor, d.net_minor, acc.name, d.to_account_id, d.entry_id
		FROM investment_dividends d
		JOIN investment_assets a ON a.id = d.asset_id AND a.household_id = ?
		LEFT JOIN accounts acc ON acc.id = d.to_account_id AND acc.household_id = ?
		WHERE d.household_id = ?`
	args := []any{p.HouseholdID, p.HouseholdID, p.HouseholdID}
	if assetID != "" {
		query += " AND d.asset_id = ?"
		args = append(args, assetID)
	}
	if from != "" {
		query += " AND d.received_on >= ?"
		args = append(args, from)
	}
	if to != "" {
		query += " AND d.received_on <= ?"
		args = append(args, to)
	}
	query += " ORDER BY d.received_on DESC, d.id DESC"

	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	var totalGross, totalTax, totalNet, unknownGross int64
	unknownCount := int64(0)
	years := map[string]*dividendYear{}
	for rows.Next() {
		var id, aid, code, name, receivedOn, toAccountID, entryID string
		var gross, net int64
		var tax sql.NullInt64
		var accountName sql.NullString
		if err := rows.Scan(&id, &aid, &code, &name, &receivedOn, &gross, &tax, &net,
			&accountName, &toAccountID, &entryID); err != nil {
			internalErr(w, err)
			return
		}
		var taxValue any
		var taxMinor int64
		if tax.Valid {
			taxValue, taxMinor = tax.Int64, tax.Int64
			totalTax += tax.Int64
		} else {
			taxValue = nil // unknown, never coerced to zero
			unknownCount++
			unknownGross += gross
		}
		totalGross += gross
		totalNet += net

		year := receivedOn
		if len(year) >= 4 {
			year = year[:4]
		}
		bucket := years[year]
		if bucket == nil {
			bucket = &dividendYear{year: year}
			years[year] = bucket
		}
		bucket.gross += gross
		bucket.tax += taxMinor
		bucket.net += net
		bucket.count++
		if !tax.Valid {
			bucket.unknown++
			bucket.unknownGross += gross
		}

		var accountValue any
		if accountName.Valid {
			accountValue = accountName.String
		}
		items = append(items, map[string]any{
			"id": id, "asset_id": aid, "code": code, "name": name,
			"received_on": receivedOn, "gross_minor": gross, "tax_minor": taxValue,
			"net_minor": net, "tax_unknown": !tax.Valid,
			"to_account_id": toAccountID, "to_account_name": accountValue,
			"entry_id": entryID,
		})
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}

	keys := make([]string, 0, len(years))
	for k := range years {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	byYear := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		b := years[k]
		byYear = append(byYear, map[string]any{
			"year": k, "gross_minor": b.gross, "tax_minor": b.tax,
			"net_minor": b.net, "count": b.count,
			"unknown_tax_count": b.unknown, "unknown_gross_minor": b.unknownGross,
		})
	}

	writeJSON(w, 200, map[string]any{
		"period": map[string]string{"from": from, "to": to, "basis": "Asia/Shanghai"},
		"items":  items, "total": len(items),
		"summary": map[string]any{
			"total_gross_minor": totalGross, "total_tax_minor": totalTax,
			"total_net_minor": totalNet, "unknown_tax_count": unknownCount,
			"unknown_gross_minor": unknownGross, "by_year": byYear,
		},
		"note": "税额为空表示未填写/未知，不按 0 处理；年度汇总按到账日归属，估值与收益均为手工口径，非实时行情",
	})
}
