package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"tinynest/api/internal/db"
	"tinynest/api/internal/invest"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

// Cost follows posting order, even for backdated trades. Equal timestamps
// cannot reliably establish an order for historical rows.
const hasLaterInvestmentTrade = `EXISTS(
	SELECT 1 FROM investment_trades newer
	JOIN journal_entries j ON j.id=newer.entry_id AND j.household_id=newer.household_id
	WHERE newer.household_id=t.household_id AND newer.asset_id=t.asset_id
	AND newer.id<>t.id AND newer.created_at>=t.created_at AND j.status='posted'
)`

var errTradeOrder = errors.New("只能撤销同一资产最后入账且顺序明确的成交")
var errTradePosition = errors.New("成交快照与当前持仓不一致")

func (a *API) handleReverseTrade(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	ctx := r.Context()
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || utf8.RuneCountInString(in.Reason) > 200 {
		writeErr(w, 400, "invalid_request", "撤销原因须为 1 至 200 个字符")
		return
	}
	var reversalID string
	var assetID string
	err := a.DB.QueryRowContext(ctx, `SELECT asset_id FROM investment_trades WHERE household_id=? AND id=?`,
		p.HouseholdID, id).Scan(&assetID)
	if err == nil {
		err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
			if err := a.lockInvestmentAsset(r, tx, p.HouseholdID, assetID); err != nil {
				return err
			}
			var entryID, side, status string
			var tradeQty, net, costBefore, costAfter, realizedAfter int64
			var hasLater bool
			if err := tx.QueryRowContext(ctx, `
				SELECT t.entry_id,t.side,e.status,t.quantity_scaled,t.net_minor,
				       t.position_cost_before_minor,t.position_cost_after_minor,t.realized_pnl_minor,`+hasLaterInvestmentTrade+`
				FROM investment_trades t JOIN journal_entries e ON e.id=t.entry_id AND e.household_id=t.household_id
				WHERE t.household_id=? AND t.asset_id=? AND t.id=?`,
				p.HouseholdID, assetID, id).Scan(&entryID, &side, &status, &tradeQty, &net,
				&costBefore, &costAfter, &realizedAfter, &hasLater); err != nil {
				return err
			}
			if status == ledger.StatusReversed {
				return tx.QueryRowContext(ctx, `
					SELECT id FROM journal_entries WHERE household_id=? AND reversal_of=?`,
					p.HouseholdID, entryID).Scan(&reversalID)
			}
			if hasLater {
				return errTradeOrder
			}
			var qty, cost, realized int64
			var subject string
			if err := tx.QueryRowContext(ctx, `
				SELECT pos.quantity_scaled,pos.total_cost_minor,pos.realized_pnl_minor,a.ledger_code
				FROM investment_positions pos JOIN investment_assets a ON a.id=pos.asset_id
				WHERE a.household_id=? AND a.id=?`, p.HouseholdID, assetID).Scan(&qty, &cost, &realized, &subject); err != nil {
				return err
			}
			original, err := loadEntry(tx, r, entryID)
			if err != nil {
				return err
			}
			if original.BizID != id || cost != costAfter || realized != realizedAfter ||
				original.Delta(subject) != money.Minor(costAfter-costBefore) {
				return errTradePosition
			}
			pos := invest.NewPosition(money.QuantityScale)
			pos.TotalCostMinor, pos.RealizedPnLMinor = money.Minor(costBefore), money.Minor(realized)
			if side == "buy" {
				if qty < tradeQty || original.BizType != ledger.BizInvestBuy {
					return errTradePosition
				}
				pos.QtyScaled = qty - tradeQty
			} else {
				if original.BizType != ledger.BizInvestSell {
					return errTradePosition
				}
				restoredQty, err := money.Add(money.Minor(qty), money.Minor(tradeQty))
				if err != nil {
					return err
				}
				pos.QtyScaled = int64(restoredQty)
				pos.RealizedPnLMinor, err = money.Sub(pos.RealizedPnLMinor, money.Minor(net-(costBefore-costAfter)))
				if err != nil {
					return err
				}
			}
			avg, err := pos.AvgCostMinorPerUnit()
			if err != nil {
				return err
			}
			reversalID = newID("je")
			reversal, err := original.Reverse(reversalID, original.OccurredOn)
			if err != nil {
				return err
			}
			if err := db.SaveEntry(ctx, tx, reversal, p.HouseholdID, "", p.Username); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE journal_entries SET status='reversed' WHERE household_id=? AND id=?`,
				p.HouseholdID, entryID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE investment_positions SET quantity_scaled=?,total_cost_minor=?,avg_cost_minor=?,realized_pnl_minor=?
				WHERE asset_id IN (SELECT id FROM investment_assets WHERE household_id=? AND id=?)`,
				pos.QtyScaled, pos.TotalCostMinor, avg, pos.RealizedPnLMinor, p.HouseholdID, assetID); err != nil {
				return err
			}
			payload, err := json.Marshal(in)
			if err != nil {
				return err
			}
			return audit(ctx, tx, p, "reverse_investment_trade", "investment_trade", id, string(payload))
		})
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "投资成交不存在")
	case errors.Is(err, errTradeOrder), errors.Is(err, errTradePosition):
		writeErr(w, 409, "trade_not_reversible", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "reversed", "reversal_entry_id": reversalID})
	}
}
