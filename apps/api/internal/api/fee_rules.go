package api

import (
	"database/sql"
	"errors"
	"net/http"
)

func (a *API) handleCreateFeeRule(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		AccountID          string `json:"account_id"`
		EffectiveFrom      string `json:"effective_from"`
		CommissionBPS      int64  `json:"commission_bps"`
		CommissionMinMinor int64  `json:"commission_min_minor"`
		StampDutyBPS       int64  `json:"stamp_duty_bps"`
		TransferFeeBPS     int64  `json:"transfer_fee_bps"`
	}
	if err := decode(r, &in); err != nil || in.AccountID == "" || !validDate(in.EffectiveFrom) || in.CommissionMinMinor < 0 {
		writeErr(w, http.StatusBadRequest, "invalid_request", "账户、生效日期或最低佣金不合法")
		return
	}
	for _, bps := range []int64{in.CommissionBPS, in.StampDutyBPS, in.TransferFeeBPS} {
		if bps < 0 || bps > 10000 {
			writeErr(w, http.StatusBadRequest, "invalid_request", "费率必须在 0 到 10000 基点之间")
			return
		}
	}
	id := newID("fee")
	var version int64
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var kind string
		if err := tx.QueryRowContext(r.Context(), `
			SELECT kind FROM accounts WHERE id=? AND household_id=? AND status='active'`,
			in.AccountID, p.HouseholdID).Scan(&kind); err != nil {
			return err
		}
		if kind != "securities" && kind != "wealth" {
			return sql.ErrNoRows
		}
		if err := tx.QueryRowContext(r.Context(), `
			SELECT COALESCE(MAX(version),0)+1 FROM investment_fee_rules WHERE account_id=?`,
			in.AccountID).Scan(&version); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO investment_fee_rules(id,account_id,version,commission_bps,commission_min_minor,
				stamp_duty_bps,transfer_fee_bps,effective_from)
			VALUES(?,?,?,?,?,?,?,?)`, id, in.AccountID, version, in.CommissionBPS, in.CommissionMinMinor,
			in.StampDutyBPS, in.TransferFeeBPS, in.EffectiveFrom); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "create_fee_rule", "investment_fee_rule", id, in.EffectiveFrom)
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not_found", "投资账户不存在或不可配置费率")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "version": version, "effective_from": in.EffectiveFrom})
}
