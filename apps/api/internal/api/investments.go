package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"tinynest/api/internal/db"
	"tinynest/api/internal/invest"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

func (a *API) registerInvestments(mux *http.ServeMux) {
	a.route(mux, "POST /api/v1/investments/fee-rules", a.handleCreateFeeRule)
	a.route(mux, "GET /api/v1/investments/assets", a.handleListAssets)
	a.route(mux, "POST /api/v1/investments/assets", a.handleCreateAsset)
	a.route(mux, "GET /api/v1/investments/trades", a.handleListTrades)
	a.route(mux, "POST /api/v1/investments/trades", a.handleCreateTrade)
	a.route(mux, "GET /api/v1/investments/positions", a.handleListPositions)
	a.route(mux, "POST /api/v1/investments/dividends", a.handleCreateDividend)
	a.route(mux, "POST /api/v1/investments/valuations", a.handleCreateValuation)
}

func (a *API) handleCreateAsset(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Code      string  `json:"code"`
		Name      string  `json:"name"`
		Kind      string  `json:"kind"`
		Market    *string `json:"market"`
		AccountID string  `json:"account_id"`
	}
	if err := decode(r, &in); err != nil || in.Code == "" || in.Name == "" {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	switch in.Kind {
	case "stock", "etf", "wealth", "custom":
	default:
		writeErr(w, 400, "invalid_request", "不支持的资产类型")
		return
	}
	id := newID("ast")
	holdSubject := ledger.Subj(ledger.SubjHoldingStock, id)
	if in.Kind == "etf" {
		holdSubject = ledger.Subj(ledger.SubjHoldingETF, id)
	} else if in.Kind == "wealth" {
		holdSubject = ledger.Subj(ledger.SubjHoldingWealth, id)
	}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var cashSubject string
		if err := tx.QueryRowContext(r.Context(),
			`SELECT ledger_code FROM accounts WHERE id=? AND household_id=? AND kind IN ('securities','wealth')`,
			in.AccountID, p.HouseholdID).Scan(&cashSubject); err != nil {
			return errors.New("投资账户不存在或类型不符")
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO investment_assets(id,household_id,code,name,kind,market,account_id,ledger_code)
			VALUES(?,?,?,?,?,?,?,?)`,
			id, p.HouseholdID, in.Code, in.Name, in.Kind, in.Market, in.AccountID, holdSubject); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO investment_positions(asset_id,quantity_scaled,total_cost_minor) VALUES(?,0,0)`, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(r.Context(),
			`INSERT INTO ledger_accounts(id,household_id,code,name,category,ref_type,ref_id) VALUES(?,?,?,?,1,'investment_asset',?)`,
			newID("la"), p.HouseholdID, holdSubject, in.Name, id)
		return err
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "code": in.Code, "ledger_code": holdSubject})
}

func (a *API) handleListAssets(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,code,name,kind,account_id,ledger_code FROM investment_assets WHERE household_id=? ORDER BY created_at`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, code, name, kind, acc, lc string
		if err := rows.Scan(&id, &code, &name, &kind, &acc, &lc); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "code": code, "name": name, "kind": kind, "account_id": acc, "ledger_code": lc})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

// feeSnapshot loads the account's active fee rule and returns a versioned JSON snapshot.
func feeSnapshot(tx *sql.Tx, r *http.Request, householdID, accountID, tradeOn string) (string, int64, int64, int64, int64, string, error) {
	var id string
	var commissionBPS, minFee, stampBPS, transferBPS int64
	err := tx.QueryRowContext(r.Context(), `
		SELECT f.id,commission_bps,commission_min_minor,stamp_duty_bps,transfer_fee_bps
		FROM investment_fee_rules f JOIN accounts a ON a.id=f.account_id
		WHERE a.household_id=? AND f.account_id=? AND f.effective_from<=?
		ORDER BY f.effective_from DESC, f.version DESC LIMIT 1`, householdID, accountID, tradeOn).
		Scan(&id, &commissionBPS, &minFee, &stampBPS, &transferBPS)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, 0, 0, 0, "{}", nil
		}
		return "", 0, 0, 0, 0, "", err
	}
	b, _ := json.Marshal(map[string]any{
		"fee_rule_id": id, "commission_bps": commissionBPS, "commission_min_minor": minFee,
		"stamp_duty_bps": stampBPS, "transfer_fee_bps": transferBPS,
	})
	return id, commissionBPS, minFee, stampBPS, transferBPS, string(b), nil
}

func (a *API) handleCreateTrade(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		AssetID        string  `json:"asset_id"`
		Side           string  `json:"side"`
		TradeOn        string  `json:"trade_on"`
		QuantityScaled int64   `json:"quantity_scaled"`
		GrossMinor     int64   `json:"gross_minor"`
		FeeMinor       *int64  `json:"fee_minor"` // nil = auto-compute from the fee rule
		FromAccountID  string  `json:"from_account_id"`
		Note           *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || !validDate(in.TradeOn) {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	if in.Side != "buy" && in.Side != "sell" || in.QuantityScaled <= 0 || in.GrossMinor <= 0 {
		writeErr(w, 400, "invalid_amount", "方向/数量/成交金额不正确")
		return
	}
	tradeID, entryID := newID("trade"), newID("je")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var holdSubject, cashSubject, assetAccount string
		if err := tx.QueryRowContext(r.Context(),
			`SELECT ledger_code,account_id FROM investment_assets WHERE id=? AND household_id=?`,
			in.AssetID, p.HouseholdID).Scan(&holdSubject, &assetAccount); err != nil {
			return errors.New("投资资产不存在")
		}
		if err := tx.QueryRowContext(r.Context(),
			`SELECT ledger_code FROM accounts WHERE id=? AND household_id=?`, in.FromAccountID, p.HouseholdID).
			Scan(&cashSubject); err != nil {
			return errors.New("资金账户不存在")
		}
		ruleID, cBPS, minFee, stampBPS, transferBPS, snapshot, feeErr := feeSnapshot(tx, r, p.HouseholdID, assetAccount, in.TradeOn)
		if feeErr != nil {
			return feeErr
		}
		computed := money.Minor(0)
		if ruleID != "" {
			// Single source of truth shared with POST /fee-rules/preview.
			breakdown, calcErr := computeTradeFee(money.Minor(in.GrossMinor), cBPS, minFee, stampBPS, transferBPS, in.Side)
			if calcErr != nil {
				return calcErr
			}
			computed = breakdown.TotalMinor
		}
		fee := computed
		if in.FeeMinor != nil {
			fee = money.Minor(*in.FeeMinor)
		}
		if fee < 0 {
			return errors.New("费用不能为负")
		}
		gross := money.Minor(in.GrossMinor)

		var qty, cost, realized, netDiv int64
		if err := tx.QueryRowContext(r.Context(),
			`SELECT quantity_scaled,total_cost_minor,realized_pnl_minor,net_dividend_minor FROM investment_positions WHERE asset_id=?`,
			in.AssetID).Scan(&qty, &cost, &realized, &netDiv); err != nil {
			return errors.New("持仓记录不存在")
		}
		pos := invest.NewPosition(money.QuantityScale)
		pos.QtyScaled, pos.TotalCostMinor = qty, money.Minor(cost)
		pos.RealizedPnLMinor, pos.NetDividendMinor = money.Minor(realized), money.Minor(netDiv)

		c := ledger.Context{EntryID: entryID, BizID: tradeID, OccurredOn: in.TradeOn}
		var entry *ledger.Entry
		var err error
		costBefore, costAfter := pos.TotalCostMinor, pos.TotalCostMinor
		if in.Side == "buy" {
			totalCost, err := money.Add(gross, fee)
			if err != nil {
				return err
			}
			entry, err = ledger.InvestBuy(c, cashSubject, holdSubject, gross, fee)
			if err != nil {
				return err
			}
			if err := pos.Buy(in.QuantityScaled, totalCost); err != nil {
				return err
			}
			costAfter = pos.TotalCostMinor
		} else {
			net, err := money.Sub(gross, fee)
			if err != nil {
				return err
			}
			if err := money.MustPositive(net); err != nil {
				return errors.New("卖出净收入必须为正")
			}
			carry, pnl, err := pos.Sell(in.QuantityScaled, net)
			if err != nil {
				return err
			}
			entry, err = ledger.InvestSell(c, cashSubject, holdSubject, net, carry)
			if err != nil {
				return err
			}
			realized = int64(pos.RealizedPnLMinor)
			costAfter = pos.TotalCostMinor
			_ = pnl
		}
		if err := db.SaveEntry(r.Context(), tx, entry, p.HouseholdID, idempotencyKey(r), p.Username); err != nil {
			return err
		}
		netValue := in.GrossMinor - int64(fee)
		if in.Side == "buy" {
			netValue = in.GrossMinor + int64(fee)
		}
		// Derive a recorded per-unit price (minor units, fixed-point scaled) from the
		// manually entered gross amount so the trade row is self-describing.
		priceScaled, err := money.DivRound(in.GrossMinor*money.QuantityScale, in.QuantityScaled)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO investment_trades(id,household_id,asset_id,side,trade_on,quantity_scaled,price_scaled,
			  gross_minor,computed_fee_minor,fee_minor,fee_rule_id,fee_rule_snapshot_json,net_minor,
			  from_account_id,position_cost_before_minor,position_cost_after_minor,realized_pnl_minor,note,entry_id,idempotency_key)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			tradeID, p.HouseholdID, in.AssetID, in.Side, in.TradeOn, in.QuantityScaled, priceScaled, in.GrossMinor,
			int64(computed), int64(fee), nullableKey(ruleID), snapshot, netValue,
			in.FromAccountID, int64(costBefore), int64(costAfter), realized, in.Note, entryID,
			nullableKey(idempotencyKey(r))); err != nil {
			return err
		}
		avg, _ := pos.AvgCostMinorPerUnit()
		_, err = tx.ExecContext(r.Context(), `
			UPDATE investment_positions SET quantity_scaled=?,total_cost_minor=?,avg_cost_minor=?,realized_pnl_minor=?
			WHERE asset_id=?`,
			pos.QtyScaled, int64(pos.TotalCostMinor), int64(avg), int64(pos.RealizedPnLMinor), in.AssetID)
		return err
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": tradeID, "entry_id": entryID, "side": in.Side})
}

func (a *API) handleListTrades(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,asset_id,side,trade_on,quantity_scaled,gross_minor,fee_minor,net_minor,realized_pnl_minor
		 FROM investment_trades WHERE household_id=? ORDER BY trade_on DESC LIMIT 200`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, asset, side, on string
		var qty, gross, fee, net, realized int64
		if err := rows.Scan(&id, &asset, &side, &on, &qty, &gross, &fee, &net, &realized); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"id": id, "asset_id": asset, "side": side, "trade_on": on,
			"quantity_scaled": qty, "gross_minor": gross, "fee_minor": fee,
			"net_minor": net, "realized_pnl_minor": realized,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) handleListPositions(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT p.asset_id,a.code,a.name,p.quantity_scaled,p.total_cost_minor,p.avg_cost_minor,
		       p.realized_pnl_minor,p.net_dividend_minor,p.last_valued_on,
	       EXISTS(SELECT 1 FROM investment_dividends d
	              WHERE d.household_id=a.household_id AND d.asset_id=p.asset_id AND d.tax_minor IS NULL),
		       (SELECT market_value_minor FROM investment_valuations v WHERE v.asset_id=p.asset_id ORDER BY v.valued_on DESC LIMIT 1),
	       (SELECT COALESCE(SUM(t.gross_minor+t.fee_minor),0) FROM investment_trades t
	        WHERE t.asset_id=p.asset_id AND t.household_id=a.household_id AND t.side='buy')
		FROM investment_positions p JOIN investment_assets a ON a.id=p.asset_id
		WHERE a.household_id=? ORDER BY a.code`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, code, name string
		var qty, cost, avg, realized, netDiv int64
		var valued *string
		var market *int64
		var dividendTaxUnknown bool
		var cumulativeBuyCost int64
		if err := rows.Scan(&id, &code, &name, &qty, &cost, &avg, &realized, &netDiv, &valued, &dividendTaxUnknown, &market, &cumulativeBuyCost); err != nil {
			internalErr(w, err)
			return
		}
		m := map[string]any{
			"asset_id": id, "code": code, "name": name, "quantity_scaled": qty,
			"total_cost_minor": cost, "avg_cost_minor": avg,
			"realized_pnl_minor": realized, "net_dividend_minor": netDiv,
			"last_valued_on": valued, "dividend_tax_unknown": dividendTaxUnknown,
		}
		if qty == 0 {
			zero := int64(0)
			market = &zero
		}
		if market != nil {
			m["market_value_minor"] = *market
			m["unrealized_pnl_minor"] = *market - cost
			m["total_return_minor"] = realized + (*market - cost) + netDiv
			position := invest.Position{
				TotalCostMinor: money.Minor(cost), RealizedPnLMinor: money.Minor(realized), NetDividendMinor: money.Minor(netDiv),
			}
			if bps, err := position.ReferenceReturnBPS(money.Minor(*market), money.Minor(cumulativeBuyCost)); err == nil {
				m["reference_return_bps"] = bps
			}
		} else {
			m["unrealized_pnl_minor"] = nil
			m["note"] = "尚未录入估值"
		}
		items = append(items, m)
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) handleCreateDividend(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		AssetID     string `json:"asset_id"`
		ReceivedOn  string `json:"received_on"`
		GrossMinor  int64  `json:"gross_minor"`
		TaxMinor    *int64 `json:"tax_minor"` // null = unknown, never assumed zero
		ToAccountID string `json:"to_account_id"`
	}
	if err := decode(r, &in); err != nil || !validDate(in.ReceivedOn) || in.GrossMinor <= 0 {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	net := money.Minor(in.GrossMinor)
	var taxPtr *money.Minor
	if in.TaxMinor != nil {
		v := money.Minor(*in.TaxMinor)
		if v < 0 || v > net {
			writeErr(w, 400, "invalid_amount", "红利税金额不合法")
			return
		}
		net -= v
		taxPtr = &v
	} else {
		// Unknown tax: only the received amount is known, recorded as gross.
		net = money.Minor(in.GrossMinor)
	}
	divID, entryID := newID("div"), newID("je")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var toSubject string
		var n int
		if err := tx.QueryRowContext(r.Context(),
			`SELECT ledger_code FROM accounts WHERE id=? AND household_id=?`, in.ToAccountID, p.HouseholdID).
			Scan(&toSubject); err != nil {
			return errors.New("到账账户不存在")
		}
		if err := tx.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM investment_assets WHERE id=? AND household_id=?`, in.AssetID, p.HouseholdID).Scan(&n); err != nil || n == 0 {
			return errors.New("投资资产不存在")
		}
		var taxForLedger *money.Minor
		grossForLedger := net
		if taxPtr != nil {
			grossForLedger = money.Minor(in.GrossMinor)
			taxForLedger = taxPtr
		}
		e, err := ledger.Dividend(ledger.Context{EntryID: entryID, BizID: divID, OccurredOn: in.ReceivedOn},
			toSubject, grossForLedger, taxForLedger, net)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, idempotencyKey(r), p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO investment_dividends(id,household_id,asset_id,received_on,gross_minor,tax_minor,net_minor,
			  to_account_id,to_ledger_code,entry_id,idempotency_key)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			divID, p.HouseholdID, in.AssetID, in.ReceivedOn, in.GrossMinor, in.TaxMinor, int64(net),
			in.ToAccountID, toSubject, entryID, nullableKey(idempotencyKey(r))); err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(),
			`UPDATE investment_positions SET net_dividend_minor=net_dividend_minor+? WHERE asset_id=?`,
			int64(net), in.AssetID)
		return err
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": divID, "net_minor": net, "tax_unknown": in.TaxMinor == nil})
}

func (a *API) handleCreateValuation(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		AssetID          string  `json:"asset_id"`
		ValuedOn         string  `json:"valued_on"`
		MarketValueMinor int64   `json:"market_value_minor"`
		PriceScaled      int64   `json:"price_scaled"`
		Note             *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || !validDate(in.ValuedOn) || in.MarketValueMinor < 0 {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	var n int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM investment_assets WHERE id=? AND household_id=?`, in.AssetID, p.HouseholdID).Scan(&n); err != nil || n == 0 {
		writeErr(w, 404, "not_found", "投资资产不存在")
		return
	}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		upsert := "INSERT INTO investment_valuations(asset_id,valued_on,price_scaled,market_value_minor,note) VALUES(?,?,?,?,?) "
		if a.DB.IsMySQL() {
			upsert += "AS incoming ON DUPLICATE KEY UPDATE price_scaled=incoming.price_scaled, market_value_minor=incoming.market_value_minor,note=incoming.note"
		} else {
			upsert += "ON CONFLICT(asset_id,valued_on) DO UPDATE SET price_scaled=excluded.price_scaled,market_value_minor=excluded.market_value_minor,note=excluded.note"
		}
		if _, err := tx.ExecContext(r.Context(), upsert, in.AssetID, in.ValuedOn, in.PriceScaled, in.MarketValueMinor, in.Note); err != nil {
			return err
		}
		_, err := tx.ExecContext(r.Context(),
			`UPDATE investment_positions SET last_valued_on=? WHERE asset_id=?`, in.ValuedOn, in.AssetID)
		return err
	})
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"asset_id": in.AssetID, "valued_on": in.ValuedOn, "market_value_minor": in.MarketValueMinor})
}
