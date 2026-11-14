package api

import "testing"

func TestProjectMaintenance(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	base := "/api/v1/projects"
	p := c.mustOK("POST", base, map[string]string{"name": "旧项目"})
	path := base + "/" + p["id"].(string)
	c.mustOK("PATCH", path, map[string]any{"name": "新项目", "status": "archived", "start_on": "2026-11-01"})
	project := c.mustOK("GET", path, nil)
	if project["name"] != "新项目" || project["status"] != "archived" || project["start_on"] != "2026-11-01" {
		t.Fatalf("unexpected project: %v", project)
	}
	if code, _ := c.do("PATCH", path, map[string]string{"end_on": "2026-10-01"}); code != 400 {
		t.Fatalf("invalid date range returned %d", code)
	}
	c.mustOK("GET", path+"/summary", nil)
}
