package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"tinynest/api/internal/auth"
	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

// Register wires every V0.1 endpoint onto the existing Phase-0 mux.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/setup", a.handleSetup)
	mux.HandleFunc("POST /api/v1/auth/session", a.handleLogin)
	a.route(mux, "GET /api/v1/auth/me", a.handleMe)
	a.route(mux, "GET /api/v1/admin/audit-logs", a.handleAuditLogs)
	a.route(mux, "DELETE /api/v1/auth/session", a.handleLogout)

	a.route(mux, "GET /api/v1/categories", a.handleListCategories)
	a.route(mux, "POST /api/v1/categories", a.handleCreateCategory)
	a.route(mux, "GET /api/v1/projects", a.handleListProjects)
	a.route(mux, "POST /api/v1/projects", a.handleCreateProject)
	a.route(mux, "GET /api/v1/projects/{id}/summary", a.handleProjectSummary)
	a.route(mux, "GET /api/v1/accounts", a.handleListAccounts)
	a.route(mux, "POST /api/v1/accounts", a.handleCreateAccount)
	a.route(mux, "GET /api/v1/accounts/{id}/balance", a.handleAccountBalance)

	a.registerTransactions(mux)
	a.registerCredit(mux)
	a.registerInvestments(mux)
	a.registerDashboard(mux)
	a.registerV011(mux)
	a.registerV012(mux)
}

// --- bootstrap & identity ---------------------------------------------------

// setupTokenHeader carries the one-time bootstrap token supplied by the operator.
// It is intentionally NOT a cookie/CSRF flow: this endpoint runs before any user exists.
const setupTokenHeader = "X-Setup-Token"

// constantTimeTokenEqual compares fixed-length SHA-256 digests so neither the
// length nor the content of the configured token leaks through timing.
func constantTimeTokenEqual(got, want string) bool {
	g := sha256.Sum256([]byte(got))
	w := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

func (a *API) handleSetup(w http.ResponseWriter, r *http.Request) {
	// Fail closed: an empty database started without an operator-configured token
	// must never expose an open admin-creation endpoint. RemoteAddr is deliberately
	// NOT trusted here: Nginx makes every public request appear to come from loopback.
	if a.opts.SetupToken == "" {
		writeErr(w, http.StatusForbidden, "setup_disabled",
			"未配置一次性初始化令牌，初始化已禁用；请在后端主机设置 TINYNEST_SETUP_TOKEN")
		return
	}
	if !constantTimeTokenEqual(r.Header.Get(setupTokenHeader), a.opts.SetupToken) {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "初始化令牌无效")
		return
	}

	var in struct {
		HouseholdName string `json:"household_name"`
		Username      string `json:"username"`
		Password      string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	if len(in.Username) < 3 || len(in.Password) < 8 || in.HouseholdName == "" {
		writeErr(w, 400, "invalid_request", "家庭名称、用户名(>=3)与密码(>=8)均为必填")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		internalErr(w, err)
		return
	}

	// Serialize bootstrap in-process; the conditional INSERT below plus the
	// single-writer SQLite transaction guards the cross-request case.
	a.setupMu.Lock()
	defer a.setupMu.Unlock()

	hhID, userID := newID("hh"), newID("usr")
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM households`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return errConflict
		}
		// Conditional insert keeps the "only ever one household" invariant inside
		// the database transaction, not in an application-level pre-check alone.
		res, err := tx.ExecContext(r.Context(),
			`INSERT INTO households(id,name) SELECT ?,? WHERE NOT EXISTS (SELECT 1 FROM households)`,
			hhID, in.HouseholdName)
		if err != nil {
			return err
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			return errConflict
		}
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO users(id,household_id,username,password_hash,role) VALUES(?,?,?,?,'admin')`,
			userID, hhID, in.Username, hash); err != nil {
			return err
		}
		return a.DB.SeedForHousehold(r.Context(), tx, hhID)
	})
	if errors.Is(err, errConflict) {
		// Once initialized the bootstrap token can never create another admin.
		writeErr(w, 409, "conflict", "系统已初始化，不能重复创建家庭")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"household_id": hhID, "user_id": userID, "status": "initialized"})
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	// Throttle BEFORE touching the password hash: an unbounded stream of
	// Argon2id verifications is itself a denial-of-service vector.
	principal := strings.ToLower(strings.TrimSpace(in.Username))
	ip := a.clientIP(r)
	if allowed, retry := a.limiter.allow(principal, ip); !allowed {
		w.Header().Set("Retry-After", retryAfterSeconds(retry))
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "登录尝试过于频繁，请稍后重试")
		return
	}
	var userID, hhID, hash string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id, household_id, password_hash FROM users WHERE username=? AND status='active'`, in.Username).
		Scan(&userID, &hhID, &hash)
	if err != nil || !auth.VerifyPassword(hash, in.Password) {
		if blocked, retry := a.limiter.failure(principal, ip); blocked {
			w.Header().Set("Retry-After", retryAfterSeconds(retry))
			writeErr(w, http.StatusTooManyRequests, "rate_limited", "登录尝试过于频繁，请稍后重试")
			return
		}
		writeErr(w, 401, "unauthorized", "用户名或密码错误")
		return
	}
	// A successful authentication clears the per-principal failure counter, but
	// never the IP-wide bucket (see loginLimiter.reset).
	a.limiter.reset(principal, ip)
	raw, tokenHash, err := auth.NewToken()
	if err != nil {
		internalErr(w, err)
		return
	}
	csrfRaw, _, err := auth.NewToken()
	if err != nil {
		internalErr(w, err)
		return
	}
	expires := a.now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES(?,?,?,?,?)`,
		newID("sess"), userID, tokenHash, csrfRaw, expires); err != nil {
		internalErr(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "tn_session", Value: raw, Path: "/", HttpOnly: true,
		Secure: a.secureCookie(r), SameSite: http.SameSiteLaxMode,
		Expires: a.now().UTC().Add(7 * 24 * time.Hour),
	})
	writeJSON(w, 200, map[string]string{"username": in.Username, "csrf_token": csrfRaw, "status": "ok"})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	writeJSON(w, 200, map[string]string{
		"user_id": p.UserID, "household_id": p.HouseholdID,
		"username": p.Username, "csrf_token": p.CSRFToken,
	})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("tn_session")
	if err == nil {
		_, _ = a.DB.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=? WHERE token_hash=?`,
			a.nowUTC(), auth.HashToken(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "tn_session", Value: "", Path: "/", HttpOnly: true,
		Secure: a.secureCookie(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	writeJSON(w, 200, map[string]string{"status": "logged_out"})
}

// --- S1: categories, projects, accounts ------------------------------------

func (a *API) handleListCategories(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, parent_id, kind, name, level, sort, active FROM categories
		 WHERE household_id=? ORDER BY kind, sort, name`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	type cat struct {
		ID       string  `json:"id"`
		ParentID *string `json:"parent_id"`
		Kind     string  `json:"kind"`
		Name     string  `json:"name"`
		Level    int     `json:"level"`
		Sort     int     `json:"sort"`
		Active   int     `json:"active"`
	}
	out := []cat{}
	for rows.Next() {
		var c cat
		if err := rows.Scan(&c.ID, &c.ParentID, &c.Kind, &c.Name, &c.Level, &c.Sort, &c.Active); err != nil {
			internalErr(w, err)
			return
		}
		out = append(out, c)
	}
	writeJSON(w, 200, map[string]any{"items": out, "total": len(out)})
}

func (a *API) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		ParentID *string `json:"parent_id"`
		Kind     string  `json:"kind"`
		Name     string  `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	if (in.Kind != "income" && in.Kind != "expense") || in.Name == "" {
		writeErr(w, 400, "invalid_request", "kind 必须为 income/expense，名称不可为空")
		return
	}
	level := 1
	if in.ParentID != nil {
		var parentKind string
		var parentLevel int
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT kind, level FROM categories WHERE id=? AND household_id=?`, *in.ParentID, p.HouseholdID).
			Scan(&parentKind, &parentLevel); err != nil {
			writeErr(w, 400, "invalid_request", "父分类不存在")
			return
		}
		if parentKind != in.Kind || parentLevel != 1 {
			writeErr(w, 400, "invalid_request", "只允许在一级分类下建立二级分类")
			return
		}
		level = 2
	}
	id := newID("cat")
	_, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO categories(id,household_id,parent_id,kind,name,level) VALUES(?,?,?,?,?,?)`,
		id, p.HouseholdID, in.ParentID, in.Kind, in.Name, level)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "name": in.Name, "kind": in.Kind})
}

func (a *API) handleListProjects(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,name,start_on,end_on,status FROM projects WHERE household_id=? ORDER BY created_at DESC`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, status string
		var start, end *string
		if err := rows.Scan(&id, &name, &start, &end, &status); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "start_on": start, "end_on": end, "status": status})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Name    string  `json:"name"`
		StartOn *string `json:"start_on"`
		EndOn   *string `json:"end_on"`
		Note    *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || in.Name == "" {
		writeErr(w, 400, "invalid_request", "项目名称必填")
		return
	}
	id := newID("prj")
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO projects(id,household_id,name,start_on,end_on,note) VALUES(?,?,?,?,?,?)`,
		id, p.HouseholdID, in.Name, in.StartOn, in.EndOn, in.Note); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "name": in.Name})
}

func (a *API) handleProjectSummary(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var name string
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT name FROM projects WHERE id=? AND household_id=?`, id, p.HouseholdID).Scan(&name); err != nil {
		writeErr(w, 404, "not_found", "项目不存在")
		return
	}
	var expense, income int64
	_ = a.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE(SUM(CASE WHEN t.type='expense' THEN s.amount_minor ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN t.type='income'  THEN s.amount_minor ELSE 0 END),0)
		FROM transaction_splits s JOIN transactions t ON t.id=s.transaction_id
		WHERE s.project_id=? AND t.household_id=? AND t.status='posted'`, id, p.HouseholdID).Scan(&expense, &income)
	var refunded int64
	_ = a.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE(SUM(r.amount_minor),0) FROM refunds r
		JOIN transaction_splits s ON s.id=r.split_id
		WHERE s.project_id=? AND r.household_id=?`, id, p.HouseholdID).Scan(&refunded)
	writeJSON(w, 200, map[string]any{
		"project_id": id, "name": name,
		"expense_minor": expense, "income_minor": income,
		"refunded_minor": refunded, "net_expense_minor": expense - refunded,
	})
}

func ledgerCodeFor(kind, id string) string {
	switch kind {
	case "securities":
		return ledger.Subj(ledger.SubjSecuritiesCash, id)
	case "wealth":
		return ledger.Subj(ledger.SubjWealthCash, id)
	default:
		return ledger.CashSubject(id)
	}
}

func (a *API) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,kind,subtype,name,ledger_code,status FROM accounts WHERE household_id=? ORDER BY created_at`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	type accountRow struct {
		id, kind, name, code, status string
		subtype                      *string
	}
	var list []accountRow
	for rows.Next() {
		var v accountRow
		if err := rows.Scan(&v.id, &v.kind, &v.subtype, &v.name, &v.code, &v.status); err != nil {
			_ = rows.Close()
			internalErr(w, err)
			return
		}
		list = append(list, v)
	}
	// Close the rows BEFORE issuing per-account balance queries: the SQLite pool is
	// intentionally single-connection, so a nested query here would deadlock.
	if err := rows.Close(); err != nil {
		internalErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, v := range list {
		bal, err := db.Balance(r.Context(), a.DB, p.HouseholdID, v.code)
		if err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"id": v.id, "kind": v.kind, "subtype": v.subtype, "name": v.name,
			"ledger_code": v.code, "status": v.status, "balance_minor": bal,
		})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Kind                string  `json:"kind"`
		Subtype             *string `json:"subtype"`
		Name                string  `json:"name"`
		OwnerMemberID       *string `json:"owner_member_id"`
		OpeningBalanceMinor int64   `json:"opening_balance_minor"`
	}
	if err := decode(r, &in); err != nil || in.Name == "" {
		writeErr(w, 400, "invalid_request", "账户名称必填")
		return
	}
	if in.Kind != "cash" && in.Kind != "securities" && in.Kind != "wealth" {
		writeErr(w, 400, "invalid_request", "kind 必须为 cash/securities/wealth")
		return
	}
	opening := money.Minor(in.OpeningBalanceMinor)
	if opening < 0 {
		writeErr(w, 400, "invalid_amount", "期初余额不能为负")
		return
	}
	id := newID("acc")
	code := ledgerCodeFor(in.Kind, id)
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO accounts(id,household_id,kind,subtype,name,owner_member_id,ledger_code) VALUES(?,?,?,?,?,?,?)`,
			id, p.HouseholdID, in.Kind, in.Subtype, in.Name, in.OwnerMemberID, code); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO ledger_accounts(id,household_id,code,name,category,ref_type,ref_id) VALUES(?,?,?,?,1,'account',?)`,
			newID("la"), p.HouseholdID, code, in.Name, id); err != nil {
			return err
		}
		if opening > 0 {
			entryID := newID("je")
			e, err := ledger.OpeningBalance(ledger.Context{EntryID: entryID, BizID: id, OccurredOn: a.businessDate()}, code, opening)
			if err != nil {
				return err
			}
			if err := db.SaveEntry(r.Context(), tx, e, p.HouseholdID, "", p.Username); err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(),
				`INSERT INTO account_opening_balances(id,account_id,amount_minor,entry_id) VALUES(?,?,?,?)`,
				newID("ob"), id, int64(opening), entryID); err != nil {
				return err
			}
		}
		return audit(r.Context(), tx, p, "create_account", "account", id, "")
	})
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "name": in.Name, "ledger_code": code, "opening_balance_minor": opening})
}

func (a *API) handleAccountBalance(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var code, name string
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT ledger_code,name FROM accounts WHERE id=? AND household_id=?`, id, p.HouseholdID).Scan(&code, &name); err != nil {
		writeErr(w, 404, "not_found", "账户不存在")
		return
	}
	bal, err := db.Balance(r.Context(), a.DB, p.HouseholdID, code)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"account_id": id, "name": name, "balance_minor": bal, "source": "rebuilt_from_journal"})
}
