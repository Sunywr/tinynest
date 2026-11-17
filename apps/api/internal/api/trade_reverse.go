package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"tinynest/api/internal/db"
	"tinynest/api/internal/invest"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

var (
	errTradeOrder  = errors.New("同一资产存在入账顺序相同的成交，无法安全重算")
	errTradeReplay = errors.New("撤销后其后成交将超出持仓，请先撤销靠后的成交")
)

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
			var entryID, status string
			if err := tx.QueryRowContext(ctx, `
				SELECT t.entry_id, e.status FROM investment_trades t
				JOIN journal_entries e ON e.id=t.entry_id AND e.household_id=t.household_id
				WHERE t.household_id=? AND t.asset_id=? AND t.id=?`,
				p.HouseholdID, assetID, id).Scan(&entryID, &status); err != nil {
				return err
			}
			if status == ledger.StatusReversed {
				// Idempotent replay: report the existing reversal without re-writing.
				return tx.QueryRowContext(ctx, `
					SELECT id FROM journal_entries WHERE household_id=? AND reversal_of=?`,
					p.HouseholdID, entryID).Scan(&reversalID)
			}
			original, err := loadEntry(tx, r, entryID)
			if err != nil {
				return err
			}

			// Reversing ANY posted trade — the last one or an early one — replays the
			// remaining posted trades in posting order. The replay restates each later
			// sell's moving weighted cost and realised P&L; a balancing correction
			// entry per restated sell keeps the journal agreeing with the rebuilt
			// position, because later trades' cash movements stay historical fact.
			rows, err := tx.QueryContext(ctx, `
				SELECT t.id,t.side,t.trade_on,t.created_at,t.quantity_scaled,t.net_minor,
				       t.position_cost_before_minor-t.position_cost_after_minor
				FROM investment_trades t
				JOIN journal_entries e ON e.id=t.entry_id AND e.household_id=t.household_id
				WHERE t.household_id=? AND t.asset_id=? AND e.status='posted'
				ORDER BY t.created_at ASC, t.id ASC`, p.HouseholdID, assetID)
			if err != nil {
				return err
			}
			type tradeSnap struct {
				id        string
				side      string
				tradeOn   string
				createdAt string
				qty       int64
				net       int64
				carryOut  int64
			}
			var trades []tradeSnap
			for rows.Next() {
				var tr tradeSnap
				if err := rows.Scan(&tr.id, &tr.side, &tr.tradeOn, &tr.createdAt, &tr.qty, &tr.net, &tr.carryOut); err != nil {
					rows.Close()
					return err
				}
				trades = append(trades, tr)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			for i := 1; i < len(trades); i++ {
				if trades[i].createdAt == trades[i-1].createdAt {
					return errTradeOrder
				}
			}
			pos := invest.NewPosition(money.QuantityScale)
			type restate struct {
				tradeID     string
				tradeOn     string
				side        string
				net         int64
				oldCarryOut int64
				newCarryOut int64
				costBefore  int64
				costAfter   int64
				realized    int64
			}
			var restated []restate
			for _, tr := range trades {
				if tr.id == id {
					continue // the trade being reversed drops out of the replay
				}
				before := int64(pos.TotalCostMinor)
				if tr.side == "buy" {
					// For a buy, net_minor = gross + fee = capitalised cost.
					if err := pos.Buy(tr.qty, money.Minor(tr.net)); err != nil {
						return err
					}
				} else {
					if _, _, err := pos.Sell(tr.qty, money.Minor(tr.net)); err != nil {
						if errors.Is(err, invest.ErrInsufficientQty) {
							return errTradeReplay
						}
						return err
					}
				}
				restated = append(restated, restate{
					tradeID: tr.id, tradeOn: tr.tradeOn, side: tr.side, net: tr.net,
					oldCarryOut: tr.carryOut, newCarryOut: before - int64(pos.TotalCostMinor),
					costBefore: before, costAfter: int64(pos.TotalCostMinor),
					realized:   int64(pos.RealizedPnLMinor),
				})
			}

			// Reverse the target entry, backdated to its original business date.
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
			var holdSubject string
			if err := tx.QueryRowContext(ctx,
				`SELECT ledger_code FROM investment_assets WHERE household_id=? AND id=?`,
				p.HouseholdID, assetID).Scan(&holdSubject); err != nil {
				return err
			}

			// Correction chain: one balancing entry per later sell whose carried cost
			// moved. Each correction is backdated to the sell it restates and carries
			// the corrected trade's id as biz_id, so the chain stays queryable.
			type correction struct {
				TradeID    string `json:"trade_id"`
				EntryID    string `json:"correction_entry_id"`
				DeltaMinor int64  `json:"delta_minor"`
			}
			var corrections []correction
			for _, rs := range restated {
				if rs.side != "sell" || rs.newCarryOut == rs.oldCarryOut {
					continue
				}
				delta := rs.newCarryOut - rs.oldCarryOut
				oldRealized := rs.net - rs.oldCarryOut
				newRealized := rs.net - rs.newCarryOut
				memo := fmt.Sprintf("撤销成交 %s 联动更正成本结转", id)
				lines := make([]ledger.Line, 0, 4)
				if delta > 0 {
					lines = append(lines, ledger.Line{Account: holdSubject, Credit: money.Minor(delta), Memo: memo})
				} else {
					lines = append(lines, ledger.Line{Account: holdSubject, Debit: money.Minor(-delta), Memo: memo})
				}
				if oldRealized > 0 {
					lines = append(lines, ledger.Line{Account: ledger.SubjRealizedGain, Debit: money.Minor(oldRealized), Memo: memo})
				} else if oldRealized < 0 {
					lines = append(lines, ledger.Line{Account: ledger.SubjRealizedLoss, Credit: money.Minor(-oldRealized), Memo: memo})
				}
				if newRealized > 0 {
					lines = append(lines, ledger.Line{Account: ledger.SubjRealizedGain, Credit: money.Minor(newRealized), Memo: memo})
				} else if newRealized < 0 {
					lines = append(lines, ledger.Line{Account: ledger.SubjRealizedLoss, Debit: money.Minor(-newRealized), Memo: memo})
				}
				corrID := newID("je")
				entry, err := ledger.New(corrID, "invest_cost_correction", rs.tradeID, rs.tradeOn, lines...)
				if err != nil {
					return err
				}
				if err := db.SaveEntry(ctx, tx, entry, p.HouseholdID, "", p.Username); err != nil {
					return err
				}
				corrections = append(corrections, correction{TradeID: rs.tradeID, EntryID: corrID, DeltaMinor: delta})
			}

			// Restate every replayed trade's recorded snapshots so the chain stays
			// contiguous: each trade's cost_before matches the previous cost_after.
			for _, rs := range restated {
				if _, err := tx.ExecContext(ctx, `
					UPDATE investment_trades SET position_cost_before_minor=?,position_cost_after_minor=?,realized_pnl_minor=?
					WHERE household_id=? AND id=?`,
					rs.costBefore, rs.costAfter, rs.realized, p.HouseholdID, rs.tradeID); err != nil {
					return err
				}
			}

			avg, err := pos.AvgCostMinorPerUnit()
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE investment_positions SET quantity_scaled=?,total_cost_minor=?,avg_cost_minor=?,realized_pnl_minor=?
				WHERE asset_id IN (SELECT id FROM investment_assets WHERE household_id=? AND id=?)`,
				pos.QtyScaled, int64(pos.TotalCostMinor), int64(avg), int64(pos.RealizedPnLMinor), p.HouseholdID, assetID); err != nil {
				return err
			}
			payload, err := json.Marshal(map[string]any{
				"reason": in.Reason, "reversal_entry_id": reversalID, "corrections": corrections,
			})
			if err != nil {
				return err
			}
			return audit(ctx, tx, p, "reverse_investment_trade", "investment_trade", id, string(payload))
		})
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "投资成交不存在")
	case errors.Is(err, errTradeOrder), errors.Is(err, errTradeReplay):
		writeErr(w, 409, "trade_not_reversible", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "reversed", "reversal_entry_id": reversalID})
	}
}
