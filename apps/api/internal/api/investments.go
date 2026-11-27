package api

import (
	"context"
	"database/sql"
	"sort"
)

// Investment accounts are a read-only valuation snapshot imported from the
// legacy pfm app. They are intentionally NOT part of the double-entry ledger:
// no journal lines are written for market values, and the Assets page labels
// the resulting net-worth figure accordingly.

type annualReturnPoint struct {
	On string `json:"on"`
	BP int64  `json:"bp"`
}

type investmentHoldingDTO struct {
	AnnualReturns      []annualReturnPoint `json:"annual_returns,omitempty"`
	ID                 string              `json:"id"`
	Symbol             string              `json:"symbol"`
	Name               string              `json:"name"`
	AssetClass         string              `json:"asset_class"`
	AssetLabel         string              `json:"asset_label"`
	QuantityMicros     int64               `json:"quantity_micros"`
	PriceMicros        int64               `json:"price_micros"`
	CostMinor          int64               `json:"cost_minor"`
	MarketValueMinor   int64               `json:"market_value_minor"`
	UnrealizedPNLMinor int64               `json:"unrealized_pnl_minor"`
	ReturnBP           int64               `json:"return_bp"`
	DividendsMinor     int64               `json:"dividends_minor"`
	TotalReturnMinor   int64               `json:"total_return_minor"`
	TotalReturnBP      int64               `json:"total_return_bp"`
	DividendYieldBP    int64               `json:"dividend_yield_bp"`
	LastTradeOn        *string             `json:"last_trade_on"`
	LastTradePNLMinor  int64               `json:"last_trade_pnl_minor"`
	LastTradeReturnBP  int64               `json:"last_trade_return_bp"`
	WeightBP           int64               `json:"weight_bp"`
}

type investmentAccountDTO struct {
	ID                 string                 `json:"id"`
	Kind               string                 `json:"kind"`
	Name               string                 `json:"name"`
	Institution        string                 `json:"institution"`
	OwnerMemberID      string                 `json:"owner_member_id"`
	OwnerName          string                 `json:"owner_name"`
	CashMinor          int64                  `json:"cash_minor"`
	HoldingsValueMinor int64                  `json:"holdings_value_minor"`
	MarketValueMinor   int64                  `json:"market_value_minor"`
	CostMinor          int64                  `json:"cost_minor"`
	UnrealizedPNLMinor int64                  `json:"unrealized_pnl_minor"`
	DividendsMinor     int64                  `json:"dividends_minor"`
	ReturnMinor        int64                  `json:"return_minor"`
	ReturnBP           int64                  `json:"return_bp"`
	DividendYieldBP    int64                  `json:"dividend_yield_bp"`
	HoldingCount       int64                  `json:"holding_count"`
	Holdings           []investmentHoldingDTO `json:"holdings"`
}

type investmentOwnerDTO struct {
	OwnerMemberID    string `json:"owner_member_id"`
	OwnerName        string `json:"owner_name"`
	MarketValueMinor int64  `json:"market_value_minor"`
	CostMinor        int64  `json:"cost_minor"`
	ReturnMinor      int64  `json:"return_minor"`
	ReturnBP         int64  `json:"return_bp"`
	DividendsMinor   int64  `json:"dividends_minor"`
	DividendYieldBP  int64  `json:"dividend_yield_bp"`
	HoldingCount     int64  `json:"holding_count"`
}

type investmentKindDTO struct {
	Kind             string `json:"kind"`
	Label            string `json:"label"`
	MarketValueMinor int64  `json:"market_value_minor"`
	CostMinor        int64  `json:"cost_minor"`
	ReturnMinor      int64  `json:"return_minor"`
	ReturnBP         int64  `json:"return_bp"`
	DividendsMinor   int64  `json:"dividends_minor"`
	DividendYieldBP  int64  `json:"dividend_yield_bp"`
	HoldingCount     int64  `json:"holding_count"`
}

type investmentAssetClassDTO struct {
	AssetClass       string `json:"asset_class"`
	Label            string `json:"label"`
	MarketValueMinor int64  `json:"market_value_minor"`
	CostMinor        int64  `json:"cost_minor"`
	ReturnMinor      int64  `json:"return_minor"`
	ReturnBP         int64  `json:"return_bp"`
	DividendsMinor   int64  `json:"dividends_minor"`
	DividendYieldBP  int64  `json:"dividend_yield_bp"`
	WeightBP         int64  `json:"weight_bp"`
	HoldingCount     int64  `json:"holding_count"`
}

type investmentTopHoldingDTO struct {
	ID               string `json:"id"`
	Symbol           string `json:"symbol"`
	Name             string `json:"name"`
	AccountID        string `json:"account_id"`
	AccountName      string `json:"account_name"`
	OwnerMemberID    string `json:"owner_member_id"`
	OwnerName        string `json:"owner_name"`
	MarketValueMinor int64  `json:"market_value_minor"`
	TotalReturnMinor int64  `json:"total_return_minor"`
	TotalReturnBP    int64  `json:"total_return_bp"`
	DividendYieldBP  int64  `json:"dividend_yield_bp"`
}

type investmentOverviewDTO struct {
	TotalCostMinor          int64                     `json:"total_cost_minor"`
	TotalHoldingsValueMinor int64                     `json:"total_holdings_value_minor"`
	TotalCashMinor          int64                     `json:"total_cash_minor"`
	TotalMarketValueMinor   int64                     `json:"total_market_value_minor"`
	TotalUnrealizedPNLMinor int64                     `json:"total_unrealized_pnl_minor"`
	TotalDividendsMinor     int64                     `json:"total_dividends_minor"`
	TotalReturnMinor        int64                     `json:"total_return_minor"`
	TotalReturnBP           int64                     `json:"total_return_bp"`
	TotalDividendYieldBP    int64                     `json:"total_dividend_yield_bp"`
	HoldingCount            int64                     `json:"holding_count"`
	AccountCount            int64                     `json:"account_count"`
	Accounts                []investmentAccountDTO    `json:"accounts"`
	ByOwner                 []investmentOwnerDTO      `json:"by_owner"`
	ByKind                  []investmentKindDTO       `json:"by_kind"`
	ByAssetClass            []investmentAssetClassDTO `json:"by_asset_class"`
	TopHoldings             []investmentTopHoldingDTO `json:"top_holdings"`
}

func invBasisPoints(part, total int64) int64 {
	if total == 0 {
		return 0
	}
	return part * 10000 / total
}

func investmentKindLabel(kind string) string {
	if kind == "security" {
		return "证券"
	}
	if kind == "wealth" {
		return "理财"
	}
	return kind
}

func (a *API) investmentOverview(ctx context.Context, householdID string) (*investmentOverviewDTO, error) {
	accounts := []investmentAccountDTO{}
	accountIndex := map[string]int{}
	rows, err := a.DB.QueryContext(ctx, `
		SELECT ia.id, ia.kind, ia.name, COALESCE(ia.institution,''),
		       COALESCE(ia.owner_member_id,''), COALESCE(hm.name,''),
		       ia.cash_minor, ia.dividends_minor
		FROM investment_accounts ia
		LEFT JOIN household_members hm ON hm.id=ia.owner_member_id
		WHERE ia.household_id=? AND ia.status<>'deleted'
		ORDER BY CASE ia.kind WHEN 'security' THEN 0 ELSE 1 END, ia.name, ia.id`,
		householdID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v investmentAccountDTO
		if err := rows.Scan(&v.ID, &v.Kind, &v.Name, &v.Institution, &v.OwnerMemberID, &v.OwnerName,
			&v.CashMinor, &v.DividendsMinor); err != nil {
			_ = rows.Close()
			return nil, err
		}
		v.Holdings = []investmentHoldingDTO{}
		accountIndex[v.ID] = len(accounts)
		accounts = append(accounts, v)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	holdings := []investmentHoldingDTO{}
	holder := []string{}
	hrows, err := a.DB.QueryContext(ctx, `
		SELECT h.id, h.account_id, h.symbol, h.name, h.asset_class, h.asset_label,
		       h.quantity_micros, h.price_micros, h.cost_minor, h.market_value_minor,
		       h.unrealized_pnl_minor, h.return_bp, h.dividends_minor, h.dividend_yield_bp,
		       h.last_trade_on, h.last_trade_pnl_minor, h.last_trade_return_bp
		FROM investment_holdings h
		JOIN investment_accounts ia ON ia.id=h.account_id
		WHERE h.household_id=? AND h.status<>'deleted' AND ia.status<>'deleted'
		ORDER BY h.market_value_minor DESC, h.symbol, h.id`,
		householdID)
	if err != nil {
		return nil, err
	}
	for hrows.Next() {
		var v investmentHoldingDTO
		var accountID string
		var lastTrade sql.NullString
		if err := hrows.Scan(&v.ID, &accountID, &v.Symbol, &v.Name, &v.AssetClass, &v.AssetLabel,
			&v.QuantityMicros, &v.PriceMicros, &v.CostMinor, &v.MarketValueMinor,
			&v.UnrealizedPNLMinor, &v.ReturnBP, &v.DividendsMinor, &v.DividendYieldBP,
			&lastTrade, &v.LastTradePNLMinor, &v.LastTradeReturnBP); err != nil {
			_ = hrows.Close()
			return nil, err
		}
		if lastTrade.Valid {
			s := lastTrade.String
			v.LastTradeOn = &s
		}
		v.ReturnBP = invBasisPoints(v.UnrealizedPNLMinor, v.CostMinor)
		v.DividendYieldBP = invBasisPoints(v.DividendsMinor, v.CostMinor)
		v.TotalReturnMinor = v.UnrealizedPNLMinor + v.DividendsMinor
		v.TotalReturnBP = invBasisPoints(v.TotalReturnMinor, v.CostMinor)
		if v.AssetLabel == "" {
			v.AssetLabel = v.AssetClass
		}
		if v.AssetLabel == "" {
			v.AssetLabel = "其他"
		}
		holdings = append(holdings, v)
		holder = append(holder, accountID)
	}
	if err := hrows.Close(); err != nil {
		return nil, err
	}

	annualRows, err := a.DB.QueryContext(ctx, `
		SELECT r.holding_id, r.observed_on, r.annual_return_bp
		FROM investment_annual_returns r
		JOIN investment_holdings h ON h.id=r.holding_id
		JOIN investment_accounts ia ON ia.id=h.account_id AND ia.household_id=h.household_id
		WHERE h.household_id=? AND h.status<>'deleted' AND ia.status<>'deleted' AND ia.kind='wealth'
		  AND r.observed_on>=?
		ORDER BY r.holding_id, r.observed_on`, householdID,
		a.businessToday().AddDate(0, 0, -89).Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer annualRows.Close()
	annualByHolding := make(map[string][]annualReturnPoint)
	for annualRows.Next() {
		var id string
		var point annualReturnPoint
		if err := annualRows.Scan(&id, &point.On, &point.BP); err != nil {
			return nil, err
		}
		annualByHolding[id] = append(annualByHolding[id], point)
	}
	if err := annualRows.Err(); err != nil {
		return nil, err
	}

	// Attach holdings and compute account-level aggregates.
	for i := range holdings {
		idx, ok := accountIndex[holder[i]]
		if !ok {
			continue
		}
		acc := &accounts[idx]
		h := holdings[i]
		h.AnnualReturns = annualByHolding[h.ID]
		if h.MarketValueMinor > 0 {
			// Weight is filled after the account total is known below.
			h.WeightBP = 0
		}
		acc.Holdings = append(acc.Holdings, h)
		acc.HoldingsValueMinor += h.MarketValueMinor
		acc.CostMinor += h.CostMinor
		acc.UnrealizedPNLMinor += h.UnrealizedPNLMinor
	}
	for i := range accounts {
		acc := &accounts[i]
		acc.HoldingCount = int64(len(acc.Holdings))
		acc.MarketValueMinor = acc.CashMinor + acc.HoldingsValueMinor
		acc.ReturnMinor = acc.UnrealizedPNLMinor + acc.DividendsMinor
		acc.ReturnBP = invBasisPoints(acc.ReturnMinor, acc.CostMinor)
		acc.DividendYieldBP = invBasisPoints(acc.DividendsMinor, acc.CostMinor)
		for j := range acc.Holdings {
			acc.Holdings[j].WeightBP = invBasisPoints(acc.Holdings[j].MarketValueMinor, acc.HoldingsValueMinor)
		}
	}

	overview := &investmentOverviewDTO{
		Accounts:     accounts,
		ByOwner:      []investmentOwnerDTO{},
		ByKind:       []investmentKindDTO{},
		ByAssetClass: []investmentAssetClassDTO{},
		TopHoldings:  []investmentTopHoldingDTO{},
	}
	ownerIndex := map[string]int{}
	kindIndex := map[string]int{}
	classIndex := map[string]int{}

	for _, acc := range accounts {
		overview.AccountCount++
		overview.HoldingCount += acc.HoldingCount
		overview.TotalCostMinor += acc.CostMinor
		overview.TotalHoldingsValueMinor += acc.HoldingsValueMinor
		overview.TotalCashMinor += acc.CashMinor
		overview.TotalUnrealizedPNLMinor += acc.UnrealizedPNLMinor
		overview.TotalDividendsMinor += acc.DividendsMinor

		ownerID := acc.OwnerMemberID
		ownerName := acc.OwnerName
		if ownerID == "" {
			ownerName = "未分配"
		}
		oi, ok := ownerIndex[ownerID]
		if !ok {
			ownerIndex[ownerID] = len(overview.ByOwner)
			overview.ByOwner = append(overview.ByOwner, investmentOwnerDTO{OwnerMemberID: ownerID, OwnerName: ownerName})
			oi = ownerIndex[ownerID]
		}
		ov := &overview.ByOwner[oi]
		ov.MarketValueMinor += acc.MarketValueMinor
		ov.CostMinor += acc.CostMinor
		ov.ReturnMinor += acc.ReturnMinor
		ov.DividendsMinor += acc.DividendsMinor
		ov.HoldingCount += acc.HoldingCount

		ki, ok := kindIndex[acc.Kind]
		if !ok {
			kindIndex[acc.Kind] = len(overview.ByKind)
			overview.ByKind = append(overview.ByKind, investmentKindDTO{Kind: acc.Kind, Label: investmentKindLabel(acc.Kind)})
			ki = kindIndex[acc.Kind]
		}
		kv := &overview.ByKind[ki]
		kv.MarketValueMinor += acc.MarketValueMinor
		kv.CostMinor += acc.CostMinor
		kv.ReturnMinor += acc.ReturnMinor
		kv.DividendsMinor += acc.DividendsMinor
		kv.HoldingCount += acc.HoldingCount

		for _, h := range acc.Holdings {
			key := h.AssetClass
			if key == "" {
				key = h.AssetLabel
			}
			ci, ok := classIndex[key]
			if !ok {
				classIndex[key] = len(overview.ByAssetClass)
				overview.ByAssetClass = append(overview.ByAssetClass, investmentAssetClassDTO{
					AssetClass: key, Label: h.AssetLabel,
				})
				ci = classIndex[key]
			}
			cv := &overview.ByAssetClass[ci]
			cv.MarketValueMinor += h.MarketValueMinor
			cv.CostMinor += h.CostMinor
			cv.ReturnMinor += h.TotalReturnMinor
			cv.DividendsMinor += h.DividendsMinor
			cv.HoldingCount++
		}
	}

	overview.TotalMarketValueMinor = overview.TotalHoldingsValueMinor + overview.TotalCashMinor
	overview.TotalReturnMinor = overview.TotalUnrealizedPNLMinor + overview.TotalDividendsMinor
	overview.TotalReturnBP = invBasisPoints(overview.TotalReturnMinor, overview.TotalCostMinor)
	overview.TotalDividendYieldBP = invBasisPoints(overview.TotalDividendsMinor, overview.TotalCostMinor)

	for i := range overview.ByOwner {
		v := &overview.ByOwner[i]
		v.ReturnBP = invBasisPoints(v.ReturnMinor, v.CostMinor)
		v.DividendYieldBP = invBasisPoints(v.DividendsMinor, v.CostMinor)
	}
	for i := range overview.ByKind {
		v := &overview.ByKind[i]
		v.ReturnBP = invBasisPoints(v.ReturnMinor, v.CostMinor)
		v.DividendYieldBP = invBasisPoints(v.DividendsMinor, v.CostMinor)
	}
	for i := range overview.ByAssetClass {
		v := &overview.ByAssetClass[i]
		v.ReturnBP = invBasisPoints(v.ReturnMinor, v.CostMinor)
		v.DividendYieldBP = invBasisPoints(v.DividendsMinor, v.CostMinor)
		v.WeightBP = invBasisPoints(v.MarketValueMinor, overview.TotalHoldingsValueMinor)
	}

	for _, acc := range accounts {
		for _, h := range acc.Holdings {
			overview.TopHoldings = append(overview.TopHoldings, investmentTopHoldingDTO{
				ID: h.ID, Symbol: h.Symbol, Name: h.Name, AccountID: acc.ID, AccountName: acc.Name,
				OwnerMemberID: acc.OwnerMemberID, OwnerName: acc.OwnerName,
				MarketValueMinor: h.MarketValueMinor, TotalReturnMinor: h.TotalReturnMinor,
				TotalReturnBP: h.TotalReturnBP, DividendYieldBP: h.DividendYieldBP,
			})
		}
	}
	sort.SliceStable(overview.TopHoldings, func(i, j int) bool {
		if overview.TopHoldings[i].TotalReturnMinor == overview.TopHoldings[j].TotalReturnMinor {
			return overview.TopHoldings[i].MarketValueMinor > overview.TopHoldings[j].MarketValueMinor
		}
		return overview.TopHoldings[i].TotalReturnMinor > overview.TopHoldings[j].TotalReturnMinor
	})
	if len(overview.TopHoldings) > 8 {
		overview.TopHoldings = overview.TopHoldings[:8]
	}

	return overview, nil
}
