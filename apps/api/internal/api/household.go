package api

import (
	"database/sql"
	"errors"
	"net/http"
)

func (a *API) handleCurrentHousehold(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var name, currency, timezone string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT name,currency,timezone FROM households WHERE id=?`, p.HouseholdID).Scan(&name, &currency, &timezone)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "家庭不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"id": p.HouseholdID, "name": name, "currency": currency, "timezone": timezone})
}
