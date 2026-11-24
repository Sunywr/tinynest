package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

type projectBudget struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AmountMinor int64  `json:"amount_minor"`
	SpentMinor  int64  `json:"spent_minor"`
}

func (a *API) handleListProjectBudgets(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `
        SELECT b.id,b.name,b.amount_minor,COALESCE(SUM(l.debit_minor-l.credit_minor),0)
        FROM project_budgets b
        LEFT JOIN transaction_budget_links x ON x.budget_id=b.id
        LEFT JOIN transactions t ON t.id=x.transaction_id AND t.household_id=b.household_id
            AND t.type='expense' AND t.status='posted'
        LEFT JOIN journal_entries e ON e.id=t.entry_id AND e.household_id=b.household_id
        LEFT JOIN journal_lines l ON l.entry_id=e.id AND l.account_code LIKE '5010:%'
        WHERE b.household_id=? GROUP BY b.id,b.name,b.amount_minor ORDER BY b.name,b.id`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []projectBudget{}
	for rows.Next() {
		var b projectBudget
		if err := rows.Scan(&b.ID, &b.Name, &b.AmountMinor, &b.SpentMinor); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *API) handleSaveProjectBudget(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var b projectBudget
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || utf8.RuneCountInString(b.Name) > 80 || b.AmountMinor <= 0 {
		writeErr(w, 400, "invalid_request", "请输入项目名称（最多 80 字）和正数预算")
		return
	}
	b.ID = r.PathValue("id")
	create := b.ID == ""
	if create {
		b.ID = newID("budget")
	}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if create {
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO project_budgets(id,household_id,name,amount_minor) VALUES(?,?,?,?)`, b.ID, p.HouseholdID, b.Name, b.AmountMinor); err != nil {
				return err
			}
		} else {
			var id string
			if err := tx.QueryRowContext(r.Context(), `SELECT id FROM project_budgets WHERE household_id=? AND id=?`, p.HouseholdID, b.ID).Scan(&id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), `UPDATE project_budgets SET name=?,amount_minor=? WHERE household_id=? AND id=?`, b.Name, b.AmountMinor, p.HouseholdID, b.ID); err != nil {
				return err
			}
		}
		return audit(r.Context(), tx, p, "set_project_budget", "project_budget", b.ID, "{}")
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "预算不存在")
		return
	}
	if err != nil {
		writeErr(w, 400, "invalid_request", "无法保存项目预算，请检查项目名称是否重复")
		return
	}
	writeJSON(w, 200, map[string]string{"id": b.ID})
}
