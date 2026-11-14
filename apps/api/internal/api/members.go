package api

import (
	"database/sql"
	"net/http"
	"strings"
	"unicode/utf8"
)

type householdMember struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (a *API) handleListMembers(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id, name, status FROM household_members
		WHERE household_id=? ORDER BY name, id`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()

	items := []householdMember{}
	for rows.Next() {
		var member householdMember
		if err := rows.Scan(&member.ID, &member.Name, &member.Status); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, member)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) handleCreateMember(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		writeErr(w, 400, "invalid_request", "成员名称须为 1 至 100 个字符")
		return
	}

	member := householdMember{ID: newID("mem"), Name: name, Status: "active"}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO household_members(id,household_id,name) VALUES(?,?,?)`,
			member.ID, p.HouseholdID, member.Name); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "create", "household_member", member.ID, "{}")
	})
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, member)
}
