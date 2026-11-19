package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

type savedCreditPreview struct {
	Generation     int64                        `json:"-"`
	SynchronizedAt string                       `json:"synchronized_at"`
	Statements     []everbrightPreviewStatement `json:"statements"`
	Choices        map[string]string            `json:"choices"`
}

type creditPreviewQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readSavedCreditPreview(ctx context.Context, q creditPreviewQuery, household, card string) (savedCreditPreview, error) {
	result := savedCreditPreview{Choices: map[string]string{}}
	var statements, choices string
	err := q.QueryRowContext(ctx, `SELECT generation,synchronized_at,statements_json,choices_json FROM credit_mail_previews
  WHERE household_id=? AND instrument_id=?`, household, card).Scan(&result.Generation, &result.SynchronizedAt, &statements, &choices)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal([]byte(statements), &result.Statements); err != nil {
		return result, err
	}
	err = json.Unmarshal([]byte(choices), &result.Choices)
	return result, err
}

func (a *API) writeSavedCreditPreview(ctx context.Context, tx *sql.Tx, household, card string, state savedCreditPreview) error {
	statements, err := json.Marshal(state.Statements)
	if err != nil {
		return err
	}
	choices, err := json.Marshal(state.Choices)
	if err != nil {
		return err
	}
	if state.Generation == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO credit_mail_previews(household_id,instrument_id,generation,synchronized_at,statements_json,choices_json)
   VALUES(?,?,1,?,?,?)`, household, card, state.SynchronizedAt, string(statements), string(choices))
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE credit_mail_previews SET generation=generation+1,synchronized_at=?,statements_json=?,choices_json=?
  WHERE household_id=? AND instrument_id=? AND generation=?`, state.SynchronizedAt, string(statements), string(choices), household, card, state.Generation)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errConflict
	}
	return nil
}

func (a *API) savedCreditPreview(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	card := r.PathValue("id")
	ctx := r.Context()
	var exists int
	err := a.DB.QueryRowContext(ctx, `SELECT 1 FROM credit_instruments WHERE household_id=? AND id=? AND kind='credit_card' AND status='active'`, p.HouseholdID, card).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "missing_card", "信用卡不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	if r.Method == http.MethodGet {
		state, err := readSavedCreditPreview(ctx, a.DB, p.HouseholdID, card)
		if err != nil {
			internalErr(w, err)
			return
		}
		// Ledger state can change through other pages or devices.
		for i := range state.Statements {
			state.Statements[i].Review = nil
		}
		if err := a.reviewEverbrightStatements(ctx, p.HouseholdID, card, state.Statements); err != nil {
			internalErr(w, err)
			return
		}
		writeJSON(w, 200, state)
		return
	}
	var choice struct {
		Key      string `json:"key"`
		Category string `json:"category_id"`
	}
	if r.Method == http.MethodPatch && decode(r, &choice) != nil {
		writeErr(w, 400, "invalid_request", "请选择有效分类")
		return
	}
	err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		state, err := readSavedCreditPreview(ctx, tx, p.HouseholdID, card)
		if err != nil {
			return err
		}
		if r.Method == http.MethodDelete {
			state.Statements = nil
			state.Choices = map[string]string{}
			state.SynchronizedAt = ""
		} else {
			valid := false
			for _, stmt := range state.Statements {
				for i, row := range stmt.Review {
					if row.Key == choice.Key && i < len(stmt.Transactions) && stmt.Transactions[i].Amount > 0 {
						valid = true
					}
				}
			}
			if !valid {
				return errConflict
			}
			if choice.Category != "" {
				if err := a.validateCategory(tx, r, p.HouseholdID, choice.Category, "expense"); err != nil {
					return errInvalidPreviewCategory
				}
			}
			state.Choices[choice.Key] = choice.Category
		}
		return a.writeSavedCreditPreview(ctx, tx, p.HouseholdID, card, state)
	})
	switch {
	case errors.Is(err, errConflict):
		writeErr(w, 409, "preview_changed", "预览已变更，请重新打开")
	case errors.Is(err, errInvalidPreviewCategory):
		writeErr(w, 400, "invalid_category", "请选择有效的支出分类")
	case err != nil:
		internalErr(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

var errInvalidPreviewCategory = errors.New("invalid preview category")
