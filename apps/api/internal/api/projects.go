package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

type projectDetails struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	StartOn *string `json:"start_on"`
	EndOn   *string `json:"end_on"`
	Note    *string `json:"note"`
	Status  string  `json:"status"`
}

const selectProject = `SELECT id,name,start_on,end_on,note,status FROM projects WHERE id=? AND household_id=?`

func (p *projectDetails) scan(row *sql.Row) error {
	return row.Scan(&p.ID, &p.Name, &p.StartOn, &p.EndOn, &p.Note, &p.Status)
}

func (a *API) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var project projectDetails
	err := project.scan(a.DB.QueryRowContext(r.Context(), selectProject, r.PathValue("id"), p.HouseholdID))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "项目不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, project)
}

func (a *API) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Name    *string `json:"name"`
		StartOn *string `json:"start_on"`
		EndOn   *string `json:"end_on"`
		Note    *string `json:"note"`
		Status  *string `json:"status"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}

	var project projectDetails
	invalid := errors.New("项目名称、状态或日期范围无效")
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		query := selectProject
		if a.DB.IsMySQL() {
			query += " FOR UPDATE"
		}
		if err := project.scan(tx.QueryRowContext(r.Context(), query, r.PathValue("id"), p.HouseholdID)); err != nil {
			return err
		}
		if in.Name != nil {
			project.Name = strings.TrimSpace(*in.Name)
		}
		if in.Status != nil {
			project.Status = *in.Status
		}
		for _, field := range []struct {
			input   *string
			current **string
		}{
			{in.StartOn, &project.StartOn}, {in.EndOn, &project.EndOn}, {in.Note, &project.Note},
		} {
			if field.input == nil {
				continue
			}
			value := strings.TrimSpace(*field.input)
			*field.current = nil
			if value != "" {
				*field.current = &value
			}
		}
		if project.Name == "" || (project.Status != "active" && project.Status != "archived") ||
			(project.StartOn != nil && !validDate(*project.StartOn)) ||
			(project.EndOn != nil && !validDate(*project.EndOn)) ||
			(project.StartOn != nil && project.EndOn != nil && *project.StartOn > *project.EndOn) {
			return invalid
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE projects SET name=?,start_on=?,end_on=?,note=?,status=?
			WHERE id=? AND household_id=?`, project.Name, project.StartOn, project.EndOn, project.Note,
			project.Status, project.ID, p.HouseholdID); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "update_project", "project", project.ID, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "项目不存在")
	case errors.Is(err, invalid):
		writeErr(w, 400, "invalid_request", invalid.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, project)
	}
}
