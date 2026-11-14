package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"tinynest/api/internal/money"
)

// V0.1.2 additions. These are read-only or calculation-only endpoints: they do
// not change any journal rule, do not add migrations, and stay compatible with
// both the SQLite default and the MySQL production path.
func (a *API) registerV012(mux *http.ServeMux) {
	a.route(mux, "GET /api/v1/investments/fee-rules", a.handleListFeeRules)
	a.route(mux, "POST /api/v1/investments/fee-rules/preview", a.handlePreviewFeeRule)
	a.route(mux, "GET /api/v1/dashboard/net-worth", a.handleNetWorth)
}

// feeBreakdown is the single source of truth for investment fees. Both the
// preview endpoint and handleCreateTrade call computeTradeFee, so a preview can
// never drift from what is actually posted.
type feeBreakdown struct {
	CommissionMinor money.Minor
	StampDutyMinor  money.Minor
	TransferMinor   money.Minor
	TotalMinor      money.Minor
}

// computeTradeFee: commission = max(round(gross*bps/10000), minimum);
// stamp duty applies to sells only; transfer fee applies to both sides.
// All arithmetic is integer minor units (no float).
func computeTradeFee(gross money.Minor, commissionBPS, minCommission, stampBPS, transferBPS int64, side string) (feeBreakdown, error) {
	var out feeBreakdown
	if gross <= 0 {
		return out, errors.New("成交金额必须为正")
	}
	commission, err := money.FeeWithMinimum(gross, commissionBPS, money.Minor(minCommission))
	if err != nil {
		return out, err
	}
	transfer, err := money.MulBPS(gross, transferBPS)
	if err != nil {
		return out, err
	}
	var stamp money.Minor
	if side == "sell" {
		stamp, err = money.MulBPS(gross, stampBPS)
		if err != nil {
			return out, err
		}
	}
	total, err := money.Add(commission, stamp)
	if err != nil {
		return out, err
	}
	total, err = money.Add(total, transfer)
	if err != nil {
		return out, err
	}
	return feeBreakdown{commission, stamp, transfer, total}, nil
}

// GET /investments/fee-rules — versioned, date-effective rules for this
// household only. Filtering is pushed into SQL via the accounts join.
func (a *API) handleListFeeRules(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	query := `
		SELECT f.id, f.account_id, a.name, a.kind, f.version, f.effective_from,
		       f.commission_bps, f.commission_min_minor, f.stamp_duty_bps, f.transfer_fee_bps
		FROM investment_fee_rules f
		JOIN accounts a ON a.id = f.account_id
		WHERE a.household_id = ?`
	args := []any{p.HouseholdID}
	if accountID := r.URL.Query().Get("account_id"); accountID != "" {
		query += " AND f.account_id = ?"
		args = append(args, accountID)
	}
	query += " ORDER BY f.effective_from DESC, f.version DESC, f.id DESC"
	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, accountID, accountName, accountKind, effectiveFrom string
		var version, commissionBPS, minCommission, stampBPS, transferBPS int64
		if err := rows.Scan(&id, &accountID, &accountName, &accountKind, &version, &effectiveFrom,
			&commissionBPS, &minCommission, &stampBPS, &transferBPS); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"id": id, "account_id": accountID, "account_name": accountName,
			"account_kind": accountKind, "version": version, "effective_from": effectiveFrom,
			"commission_bps": commissionBPS, "commission_min_minor": minCommission,
			"stamp_duty_bps": stampBPS, "transfer_fee_bps": transferBPS,
		})
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

// POST /investments/fee-rules/preview — pure calculation. It opens a read-only
// transaction (no writes, no audit rows) and never books anything.
func (a *API) handlePreviewFeeRule(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		AccountID  string `json:"account_id"`
		Side       string `json:"side"`
		TradeOn    string `json:"trade_on"`
		GrossMinor int64  `json:"gross_minor"`
	}
	if err := decode(r, &in); err != nil || in.AccountID == "" ||
		(in.Side != "buy" && in.Side != "sell") || !validDate(in.TradeOn) || in.GrossMinor <= 0 {
		writeErr(w, 400, "invalid_request", "账户、方向、交易日期或成交金额不合法")
		return
	}

	var out map[string]any
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var kind string
		if err := tx.QueryRowContext(r.Context(),
			`SELECT kind FROM accounts WHERE id=? AND household_id=? AND status='active'`,
			in.AccountID, p.HouseholdID).Scan(&kind); err != nil {
			return errors.New("投资账户不存在或不可配置费率")
		}
		if kind != "securities" && kind != "wealth" {
			return errors.New("投资账户不存在或不可配置费率")
		}
		ruleID, cBPS, minFee, stampBPS, transferBPS, snapshot, err :=
			feeSnapshot(tx, r, p.HouseholdID, in.AccountID, in.TradeOn)
		if err != nil {
			return err
		}
		var breakdown feeBreakdown
		if ruleID != "" {
			breakdown, err = computeTradeFee(money.Minor(in.GrossMinor), cBPS, minFee, stampBPS, transferBPS, in.Side)
			if err != nil {
				return err
			}
		}
		var version int64
		var effectiveFrom string
		if ruleID != "" {
			if err := tx.QueryRowContext(r.Context(), `
				SELECT f.version, f.effective_from FROM investment_fee_rules f
				JOIN accounts a ON a.id=f.account_id
				WHERE a.household_id=? AND f.id=?`, p.HouseholdID, ruleID).
				Scan(&version, &effectiveFrom); err != nil {
				return err
			}
		}
		out = map[string]any{
			"account_id": in.AccountID, "side": in.Side, "trade_on": in.TradeOn,
			"gross_minor": in.GrossMinor,
			"commission_bps": cBPS, "commission_min_minor": minFee,
			"stamp_duty_bps": stampBPS, "transfer_fee_bps": transferBPS,
			"commission_minor": int64(breakdown.CommissionMinor),
			"stamp_duty_minor": int64(breakdown.StampDutyMinor),
			"transfer_fee_minor": int64(breakdown.TransferMinor),
			"total_fee_minor":    int64(breakdown.TotalMinor),
			"fee_rule_snapshot":  snapshot,
			"note":               "仅为费用预览，不会入账；实际交易可单笔覆盖费用并保存费率快照",
		}
		if ruleID != "" {
			out["fee_rule_id"] = ruleID
			out["fee_rule_version"] = version
			out["effective_from"] = effectiveFrom
		} else {
			out["fee_rule_id"] = nil
			out["fee_rule_version"] = nil
			out["effective_from"] = nil
			out["note"] = "该账户在交易日期尚无生效的费率规则，预计费用为 0（未配置，不代表免税）"
		}
		return nil
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// ledgerGroup maps a concrete ledger code to an asset / liability bucket.
// Keeping this explicit means every reported number can be traced to a code.
func ledgerGroup(code string) string {
	switch {
	case strings.HasPrefix(code, "1010:"):
		return "cash"
	case strings.HasPrefix(code, "1020:"), strings.HasPrefix(code, "1030:"):
		return "broker_cash"
	case strings.HasPrefix(code, "1040:"), strings.HasPrefix(code, "1050:"), strings.HasPrefix(code, "1060:"):
		return "investment_cost"
	case strings.HasPrefix(code, "1"):
		return "other_asset"
	case strings.HasPrefix(code, "2010:"):
		return "credit"
	case strings.HasPrefix(code, "2020:"):
		return "installment"
	case strings.HasPrefix(code, "2030:"):
		return "loan"
	case strings.HasPrefix(code, "2"):
		return "other_liability"
	default:
		return ""
	}
}

// GET /dashboard/net-worth — household asset / liability composition rebuilt
// from journal lines. Uses the SAME reporting window as dashboard/summary so
// net_worth_minor reconciles exactly with the existing headline number.
func (a *API) handleNetWorth(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	_, to := a.period(r)
	ctx := r.Context()

	rows, err := a.DB.QueryContext(ctx, `
		SELECT l.account_code, COALESCE(SUM(l.debit_minor - l.credit_minor), 0)
		FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id
		WHERE e.household_id = ? AND e.occurred_on >= ? AND e.occurred_on <= ?
		GROUP BY l.account_code`, p.HouseholdID, "0000-01-01", to)
	if err != nil {
		internalErr(w, err)
		return
	}
	assetItems, liabilityItems := []map[string]any{}, []map[string]any{}
	var assetTotal, liabilitySigned int64
	var cash, brokerCash, investmentCost, otherAsset int64
	var credit, installment, loan, otherLiability int64
	for rows.Next() {
		var code string
		var balance int64
		if err := rows.Scan(&code, &balance); err != nil {
			_ = rows.Close()
			internalErr(w, err)
			return
		}
		group := ledgerGroup(code)
		if group == "" {
			continue // equity / income / expense are not balance-sheet buckets
		}
		row := map[string]any{
			"ledger_code": code, "group": group,
			"balance_minor": balance, "absolute_minor": absMinor(balance),
		}
		switch group {
		case "cash":
			cash += balance
		case "broker_cash":
			brokerCash += balance
		case "investment_cost":
			investmentCost += balance
		case "other_asset":
			otherAsset += balance
		case "credit":
			credit += absMinor(balance)
		case "installment":
			installment += absMinor(balance)
		case "loan":
			loan += absMinor(balance)
		case "other_liability":
			otherLiability += absMinor(balance)
		}
		if strings.HasPrefix(code, "1") {
			assetTotal += balance
			assetItems = append(assetItems, row)
		} else {
			liabilitySigned += balance
			liabilityItems = append(liabilityItems, row)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		internalErr(w, err)
		return
	}
	_ = rows.Close()

	liabilityTotal := credit + installment + loan + otherLiability
	netWorth := assetTotal + liabilitySigned // liabilities carry negative balances

	// Manual valuations only. Assets without a valuation keep their cost in the
	// net-worth-at-market figure instead of silently disappearing.
	vrows, err := a.DB.QueryContext(ctx, `
		SELECT p.asset_id, a.code, a.name, p.quantity_scaled, p.total_cost_minor,
		       v.market_value_minor, v.valued_on
		FROM investment_positions p
		JOIN investment_assets a ON a.id = p.asset_id AND a.household_id = ?
		LEFT JOIN investment_valuations v ON v.id = (
			SELECT v2.id FROM investment_valuations v2
			JOIN investment_assets a2 ON a2.id = v2.asset_id AND a2.household_id = ?
			WHERE v2.asset_id = p.asset_id
			ORDER BY v2.valued_on DESC, v2.id DESC LIMIT 1
		)
		WHERE a.household_id = ? AND p.quantity_scaled > 0`, p.HouseholdID, p.HouseholdID, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer vrows.Close()
	investmentItems := []map[string]any{}
	var positionCost, valuedCost, marketValue int64
	var latestValuedOn string
	missing := 0
	for vrows.Next() {
		var assetID, code, name string
		var qty, cost int64
		var market sql.NullInt64
		var valuedOn sql.NullString
		if err := vrows.Scan(&assetID, &code, &name, &qty, &cost, &market, &valuedOn); err != nil {
			internalErr(w, err)
			return
		}
		positionCost += cost
		item := map[string]any{
			"asset_id": assetID, "code": code, "name": name,
			"quantity_scaled": qty, "cost_minor": cost,
			"market_value_minor": nil, "valued_on": nil,
		}
		if market.Valid {
			marketValue += market.Int64
			valuedCost += cost
			item["market_value_minor"] = market.Int64
			item["valued_on"] = valuedOn.String
			if valuedOn.String > latestValuedOn {
				latestValuedOn = valuedOn.String
			}
		} else {
			missing++
			item["note"] = "尚未录入估值；继续按成本计入净资产"
		}
		investmentItems = append(investmentItems, item)
	}
	if err := vrows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	var valuedOn any
	if latestValuedOn != "" {
		valuedOn = latestValuedOn
	}

	writeJSON(w, 200, map[string]any{
		"period": map[string]string{"from": "0000-01-01", "to": to, "basis": "Asia/Shanghai"},
		"assets": map[string]any{
			"cash_minor": cash, "broker_cash_minor": brokerCash,
			"investment_cost_minor": investmentCost, "other_minor": otherAsset,
			"total_minor": assetTotal, "items": assetItems,
		},
		"liabilities": map[string]any{
			"credit_minor": credit, "installment_minor": installment,
			"loan_minor": loan, "other_minor": otherLiability,
			"total_minor": liabilityTotal, "signed_total_minor": liabilitySigned,
			"items": liabilityItems,
		},
		"investment": map[string]any{
			"cost_minor": positionCost, "valued_cost_minor": valuedCost,
			"market_value_minor": marketValue, "unrealized_minor": marketValue - valuedCost,
			"valued_on": valuedOn, "missing_valuations": missing,
			"note": "手工估值，非实时行情；未估值持仓继续按成本计入净资产，不计入市值",
			"items": investmentItems,
		},
		"net_worth_minor":           netWorth,
		"net_worth_at_market_minor": netWorth + (marketValue - valuedCost),
		"net_worth_note":            "资产 − 负债；投资按成本计价，另给出按手工估值的参考值",
		"drilldown": map[string]any{
			"hint": "每项 items 的 ledger_code 可在流水中按科目复核；投资明细见 investment.items",
			"endpoints": map[string]any{
				"transactions":      "/api/v1/transactions",
				"trades":            "/api/v1/investments/trades",
				"positions":         "/api/v1/investments/positions",
				"credit_statements": "/api/v1/credit-statements",
				"debts":             "/api/v1/debts",
				"installments":      "/api/v1/installments",
			},
		},
	})
}

func absMinor(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
