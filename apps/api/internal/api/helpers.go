// Package api exposes the V0.1 HTTP contract. All financial responses are no-store.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"tinynest/api/internal/auth"
	"tinynest/api/internal/db"
)

type contextKey int

const principalKey contextKey = 1

// Principal is the authenticated caller resolved from the session cookie.
type Principal struct {
	UserID      string
	HouseholdID string
	Username    string
	CSRFToken   string
}

type API struct{ DB *db.DB }

func New(database *db.DB) *API { return &API{DB: database} }

func principal(r *http.Request) (Principal, bool) {
	p, ok := r.Context().Value(principalKey).(Principal)
	return p, ok
}

func newID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic("api: secure random unavailable: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

func todayUTC() string { return time.Now().UTC().Format("2006-01-02") }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("api: encode response", "error", err)
	}
}

func writeErr(w http.ResponseWriter, status int, code, message string) {
	// Never expose SQL, file paths, stack traces or secrets to the client.
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func internalErr(w http.ResponseWriter, err error) {
	slog.Error("api: internal error", "error", err)
	writeErr(w, http.StatusInternalServerError, "internal_error", "操作失败，请稍后重试")
}

func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func (a *API) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("tn_session")
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "请先登录")
			return
		}
		var p Principal
		var expiresAt string
		err = a.DB.QueryRowContext(r.Context(), `
			SELECT s.csrf_token, s.expires_at, u.id, u.household_id, u.username
			FROM sessions s JOIN users u ON u.id = s.user_id
			WHERE s.token_hash = ? AND s.revoked_at IS NULL AND u.status = 'active'`,
			auth.HashToken(cookie.Value)).Scan(&p.CSRFToken, &expiresAt, &p.UserID, &p.HouseholdID, &p.Username)
		if err != nil || expiresAt < nowUTC() {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "会话无效或已过期")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if r.Header.Get("X-CSRF-Token") != p.CSRFToken {
				writeErr(w, http.StatusForbidden, "forbidden", "CSRF 校验失败")
				return
			}
		}
		next(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	}
}

// route registers an authenticated endpoint and injects the CSRF requirement for writes.
func (a *API) route(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.HandleFunc(pattern, a.requireAuth(h))
}

// audit returns an error so a failed audit write rolls the whole business transaction back.
func audit(ctx context.Context, q db.Queryer, p Principal, action, entity, entityID, payload string) error {
	if _, err := q.ExecContext(ctx,
		`INSERT INTO audit_logs(household_id, actor, action, entity, entity_id, payload_json) VALUES(?,?,?,?,?,?)`,
		p.HouseholdID, p.Username, action, entity, entityID, payload); err != nil {
		slog.Error("api: audit log write failed", "error", err)
		return err
	}
	return nil
}

var errConflict = errors.New("conflict")

// idempotencyKey extracts the optional Idempotency-Key request header.
func idempotencyKey(r *http.Request) string { return r.Header.Get("Idempotency-Key") }
