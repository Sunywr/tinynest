package api

import (
	"net/http"
	"strconv"
)

func (a *API) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeErr(w, http.StatusBadRequest, "invalid_request", "limit 必须介于 1 和 100")
			return
		}
		limit = n
	}

	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id,actor,action,entity,entity_id,created_at FROM audit_logs
		WHERE household_id=? ORDER BY id DESC LIMIT ?`, p.HouseholdID, limit)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	type auditEvent struct {
		ID        int64  `json:"id"`
		Actor     string `json:"actor"`
		Action    string `json:"action"`
		Entity    string `json:"entity"`
		EntityID  string `json:"entity_id"`
		CreatedAt string `json:"created_at"`
	}
	items := []auditEvent{}
	for rows.Next() {
		var event auditEvent
		if err := rows.Scan(&event.ID, &event.Actor, &event.Action, &event.Entity, &event.EntityID, &event.CreatedAt); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
