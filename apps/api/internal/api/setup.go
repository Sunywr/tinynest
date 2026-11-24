package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"hash/fnv"
	"net/http"
	"strings"
	"time"

	"tinynest/api/internal/auth"
	"tinynest/api/internal/db"
)

// Register wires every V2 endpoint onto the HTTP mux.
//
// The route budget counts each method and path pair, including the credit
// statement mailbox connection test.
func (a *API) Register(mux *http.ServeMux) {
	a.routeCount = 0

	// --- setup & authentication (kept) ---
	a.register(mux, "POST /api/v1/setup", a.handleSetup)
	a.register(mux, "GET /api/v1/auth/captcha", a.handleCaptcha)
	a.register(mux, "POST /api/v1/auth/session", a.handleLogin)
	a.register(mux, "POST /api/v1/auth/register", a.handleRegister)
	a.register(mux, "POST /api/v1/auth/join", a.handleApplyInvite)
	a.register(mux, "DELETE /api/v1/auth/session", a.handleLogout)
	a.route(mux, "GET /api/v1/auth/me", a.handleMe)

	// --- household & invitations (kept) ---
	a.route(mux, "POST /api/v1/household/invitations", a.handleIssueInvite)
	a.adminRoute(mux, "GET /api/v1/admin/invitations", a.handleListInvitations)
	a.adminRoute(mux, "DELETE /api/v1/admin/invitations/{id}", a.handleRevokeInvitation)
	a.adminRoute(mux, "GET /api/v1/admin/join-applications", a.handleListApplications)
	a.adminRoute(mux, "POST /api/v1/admin/join-applications/{id}", a.handleDecideApplication)
	a.route(mux, "GET /api/v1/household/current", a.handleCurrentHousehold)
	a.adminRoute(mux, "PATCH /api/v1/household/current", a.handleUpdateHousehold)
	a.adminRoute(mux, "DELETE /api/v1/household/current", a.handleArchiveHousehold)
	a.register(mux, "POST /api/v1/household/current", a.requireAuth(a.handleCreateHousehold))
	a.route(mux, "GET /api/v1/household/members", a.handleListMembers)
	a.route(mux, "POST /api/v1/household/members", a.handleCreateMember)
	a.route(mux, "DELETE /api/v1/household/members/{id}", a.handleDeleteMember)
	a.route(mux, "POST /api/v1/household/members/current", a.handleEnsureCurrentMember)

	// --- admin users & audit (kept) ---
	a.adminRoute(mux, "GET /api/v1/admin/users", a.handleListUsers)
	a.adminRoute(mux, "PATCH /api/v1/admin/users/{id}/role", a.handleUpdateUserRole)
	a.adminRoute(mux, "POST /api/v1/admin/users/{id}/disable", a.handleDisableUser)
	a.adminRoute(mux, "GET /api/v1/admin/audit-logs", a.handleAuditLogs)
	a.registerV014(mux)

	// --- system mail: SMTP config, identity, challenges, queue (kept) ---
	a.registerMail(mux)
	a.route(mux, "GET /api/v1/credit-statement-mailbox", a.handleGetStatementMailbox)
	a.route(mux, "PUT /api/v1/credit-statement-mailbox", a.handlePutStatementMailbox)
	a.route(mux, "DELETE /api/v1/credit-statement-mailbox", a.handleDeleteStatementMailbox)
	a.route(mux, "POST /api/v1/credit-statement-mailbox/test", a.handleTestStatementMailbox)

	// --- bookkeeping: categories -------------------------------------------------
	a.registerBookkeeping(mux)
}

// registerBookkeeping wires the 22 V2 bookkeeping routes.
func (a *API) registerBookkeeping(mux *http.ServeMux) {
	// Categories
	a.route(mux, "GET /api/v1/categories", a.handleListCategories)
	a.route(mux, "POST /api/v1/categories", a.handleCreateCategory)
	a.route(mux, "PATCH /api/v1/categories/{id}", a.handleUpdateCategory)
	a.route(mux, "DELETE /api/v1/categories/{id}", a.handleDeleteCategory)

	// Accounts
	a.route(mux, "GET /api/v1/accounts", a.handleListAccounts)
	a.route(mux, "POST /api/v1/accounts", a.handleCreateAccount)
	a.route(mux, "GET /api/v1/accounts/{id}", a.handleGetAccount)
	a.route(mux, "PATCH /api/v1/accounts/{id}", a.handleUpdateAccount)
	a.route(mux, "DELETE /api/v1/accounts/{id}", a.handleDeleteAccount)
	a.route(mux, "GET /api/v1/accounts/{id}/journal", a.handleAccountJournal)
	a.route(mux, "POST /api/v1/accounts/transfers", a.handleAccountTransfer)

	// Transactions (public "流水" on the double-entry kernel)
	a.route(mux, "GET /api/v1/transactions", a.handleListTransactions)
	a.route(mux, "POST /api/v1/transactions", a.handleCreateTransaction)
	a.route(mux, "GET /api/v1/transactions/{id}", a.handleGetTransaction)
	a.route(mux, "PATCH /api/v1/transactions/{id}", a.handleCorrectTransaction)
	a.route(mux, "DELETE /api/v1/transactions/{id}", a.handleDeleteTransaction)

	// Stats (all derived from journal_lines, household-scoped in SQL)
	a.route(mux, "GET /api/v1/stats/summary", a.handleStatsSummary)
	a.route(mux, "GET /api/v1/stats/categories", a.handleStatsCategories)
	a.route(mux, "GET /api/v1/stats/daily", a.handleStatsDaily)
	a.route(mux, "GET /api/v1/stats/monthly", a.handleStatsMonthly)

	// Budgets
	a.route(mux, "GET /api/v1/project-budgets", a.handleListProjectBudgets)
	a.route(mux, "POST /api/v1/project-budgets", a.handleSaveProjectBudget)
	a.route(mux, "PUT /api/v1/project-budgets/{id}", a.handleSaveProjectBudget)
	a.route(mux, "GET /api/v1/budgets/{month}", a.handleGetBudget)
	a.route(mux, "PUT /api/v1/budgets/{month}", a.handlePutBudget)
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
	// Operator-controlled shutdown for already-initialized deployments. This is
	// checked before any token comparison or database read, so the public surface
	// is closed even if a valid token ever leaks.
	if a.opts.SetupDisabled {
		writeErr(w, http.StatusForbidden, "setup_disabled",
			"初始化入口已由运维显式关闭；本实例已完成首次部署")
		return
	}
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
	// single-writer transaction guards the cross-request case.
	a.setupMu.Lock()
	defer a.setupMu.Unlock()

	hhID, userID := newID("hh"), newID("usr")
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM households WHERE id<>?`, onboardingHousehold).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return errConflict
		}
		// Bootstrap applies only before any real household exists.
		result, err := tx.ExecContext(r.Context(),
			`INSERT INTO households(id,name) SELECT ?,? WHERE NOT EXISTS (SELECT 1 FROM households WHERE id<>?)`,
			hhID, in.HouseholdName, onboardingHousehold)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return errConflict
		}
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO users(id,household_id,username,password_hash,role) VALUES(?,?,?,?,'admin')`,
			userID, hhID, in.Username, hash); err != nil {
			return err
		}
		return db.SeedForHousehold(r.Context(), tx, hhID)
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

var loginWork = make(chan struct{}, 4)

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string
		Password string
		Captcha  captchaProof
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if err := decode(r, &in); err != nil || len(in.Username) > 191 || len(in.Password) > 256 {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	if !a.requireCaptcha(w, r, in.Captcha) {
		return
	}
	principalName := strings.ToLower(strings.TrimSpace(in.Username))
	ip := a.clientIP(r)
	key := fnv.New32a()
	_, _ = key.Write([]byte(principalName))
	shard := &a.loginShards[key.Sum32()%uint32(len(a.loginShards))]
	shard.Lock()
	defer shard.Unlock()

	var userID, hash string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id,password_hash FROM users WHERE username=? AND status='active'`, in.Username).Scan(&userID, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		internalErr(w, err)
		return
	}
	now := a.now()
	var state accountLoginState
	if userID != "" {
		state, err = a.loginState(r, userID, now)
		if err != nil {
			internalErr(w, err)
			return
		}
		if state.lockedUntil > now.Unix() {
			writeAccountLocked(w, state, now)
			return
		}
	}
	if allowed, retry := a.limiter.allow(principalName, ip); !allowed {
		w.Header().Set("Retry-After", retryAfterSeconds(retry))
		writeErr(w, 429, "rate_limited", "登录尝试过于频繁，请稍后重试")
		return
	}
	select {
	case loginWork <- struct{}{}:
		defer func() { <-loginWork }()
	default:
		writeErr(w, 429, "rate_limited", "登录请求过多，请稍后重试")
		return
	}
	if userID == "" || !auth.VerifyPassword(hash, in.Password) {
		a.limiter.failure(principalName, ip)
		if userID != "" {
			state, err = a.recordAccountFailure(r, userID, state, now)
			if err != nil {
				internalErr(w, err)
				return
			}
			if state.lockedUntil > now.Unix() {
				a.limiter.reset(principalName, ip)
				writeAccountLocked(w, state, now)
				return
			}
		}
		writeErr(w, 401, "unauthorized", "用户名或密码错误")
		return
	}
	if state.failures > 0 {
		if _, err := a.DB.ExecContext(r.Context(), `UPDATE auth_login_locks SET failures=0 WHERE user_id=?`, userID); err != nil {
			internalErr(w, err)
			return
		}
	}
	a.limiter.reset(principalName, ip)
	a.issueSession(w, r, http.StatusOK, in.Username, userID, hash)
}

func (a *API) issueSession(w http.ResponseWriter, r *http.Request, status int, username, userID, hash string) {
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
	// Recheck credentials at insertion so a reset or deletion during password
	// verification cannot leave a new session authenticated by the old password.
	result, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at)
		 SELECT ?,id,?,?,? FROM users WHERE id=? AND password_hash=? AND status='active'`,
		newID("sess"), tokenHash, csrfRaw, expires, userID, hash)
	if err != nil {
		internalErr(w, err)
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		internalErr(w, err)
		return
	}
	if count == 0 {
		writeErr(w, 401, "unauthorized", "用户名或密码错误")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "tn_session", Value: raw, Path: "/", HttpOnly: true,
		Secure: a.secureCookie(r), SameSite: http.SameSiteLaxMode,
		MaxAge:  7 * 24 * 60 * 60,
		Expires: a.now().UTC().Add(7 * 24 * time.Hour),
	})
	writeJSON(w, status, map[string]string{"username": username, "csrf_token": csrfRaw, "status": "ok"})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var memberID, memberName string
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT m.id,m.name FROM household_user_members b
		JOIN household_members m ON m.id=b.member_id
		WHERE b.user_id=? AND m.household_id=? AND m.status='active'`, p.UserID, p.HouseholdID).
		Scan(&memberID, &memberName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		internalErr(w, err)
		return
	}
	var email, avatar, birthday sql.NullString
	var payday sql.NullInt64
	if err := a.DB.QueryRowContext(r.Context(), `SELECT email,avatar,birthday,payday FROM users WHERE id=? AND household_id=?`,
		p.UserID, p.HouseholdID).Scan(&email, &avatar, &birthday, &payday); err != nil {
		internalErr(w, err)
		return
	}
	householdID := p.HouseholdID
	if householdID == onboardingHousehold {
		householdID = ""
	}
	out := map[string]any{
		"member_id": memberID, "member_name": memberName,
		"user_id": p.UserID, "household_id": householdID,
		"username": p.Username, "csrf_token": p.CSRFToken, "role": p.Role,
	}
	var verified int
	if e := a.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM email_identities WHERE owner=?`, p.UserID).Scan(&verified); e != nil {
		internalErr(w, e)
		return
	}
	out["email_verified"] = verified == 1
	if householdID == "" && verified == 1 {
		var pending bool
		if err := a.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM household_invites
			WHERE applicant=? AND decided_at IS NULL AND expires_at>?)`, p.Username, a.nowUTC()).Scan(&pending); err != nil {
			internalErr(w, err)
			return
		}
		out["join_pending"] = pending
	}
	if email.Valid {
		out["email"] = email.String
	}
	if avatar.Valid {
		out["avatar"] = avatar.String
	}
	if birthday.Valid {
		out["birthday"] = birthday.String
	}
	if payday.Valid {
		out["payday"] = payday.Int64
	}
	writeJSON(w, 200, out)
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
