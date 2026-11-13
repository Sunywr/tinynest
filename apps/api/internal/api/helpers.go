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
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
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

// LoginLimiterConfig controls failed-login throttling. Zero values fall back to
// conservative defaults; tests inject small values plus a fixed clock.
type LoginLimiterConfig struct {
	MaxFailures int
	Window      time.Duration
	Lockout     time.Duration
}

// Options are explicit deployment-controlled knobs. There is deliberately NO
// implicit "guess from r.TLS" behaviour: security attributes must be declared.
// A zero value is a local-development default, never a production default.
type Options struct {
	// SetupToken is the one-time bootstrap token required by POST /api/v1/setup.
	// Empty means initialization is refused outright (fail closed).
	SetupToken string
	// CookieSecure forces the Secure attribute on tn_session.
	CookieSecure bool
	// TrustedProxies lists IPs/CIDRs whose X-Forwarded-* headers may be honored.
	TrustedProxies []string
	// Clock is injectable so business-date tests never depend on the host timezone.
	Clock func() time.Time
	// LoginLimiter configures the failed-login rate limiter.
	LoginLimiter LoginLimiterConfig
}

type API struct {
	DB      *db.DB
	opts    Options
	trusted []netip.Prefix
	limiter *loginLimiter
	setupMu sync.Mutex
}

// New keeps the historical single-argument form working while allowing the
// composition root (and tests) to inject explicit security configuration.
func New(database *db.DB, opts ...Options) *API {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	return &API{
		DB:      database,
		opts:    o,
		trusted: parseTrustedProxies(o.TrustedProxies),
		limiter: newLoginLimiter(o.LoginLimiter, o.Clock),
	}
}

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

// now is the single clock source. In production it is time.Now; tests inject a
// fixed or manually advanced clock so UTC-day boundaries are reproducible.
func (a *API) now() time.Time {
	if a.opts.Clock != nil {
		return a.opts.Clock()
	}
	return time.Now()
}

// nowUTC is for audit/session timestamps, which stay UTC (database convention).
func (a *API) nowUTC() string { return a.now().UTC().Format(time.RFC3339) }

// businessToday is the Asia/Shanghai calendar day expressed as a UTC-shifted
// time. A fixed +08:00 avoids a tzdata dependency and, crucially, uses the
// injectable clock instead of reading the wall clock directly.
func (a *API) businessToday() time.Time { return a.now().UTC().Add(8 * time.Hour) }

// businessDate returns the current Asia/Shanghai business day (YYYY-MM-DD).
func (a *API) businessDate() string { return a.businessToday().Format("2006-01-02") }

// parseTrustedProxies accepts explicit IPs or CIDRs. Invalid entries are ignored
// with a non-sensitive warning; an empty list means NO proxy header is trusted.
func parseTrustedProxies(raw []string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
			continue
		}
		if ip, err := netip.ParseAddr(s); err == nil {
			out = append(out, netip.PrefixFrom(ip.Unmap(), ip.Unmap().BitLen()))
			continue
		}
		slog.Warn("api: ignoring invalid trusted proxy entry", "entry", s)
	}
	return out
}

func (a *API) trustedContains(ip netip.Addr) bool {
	for _, p := range a.trusted {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// remoteIP parses the immediate peer from RemoteAddr; it never reads a
// client-supplied header (an attacker can forge X-Forwarded-* freely).
func remoteIP(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

// clientIP is used for rate-limit bucketing. X-Forwarded-For is honored ONLY
// when the immediate peer is an explicitly configured trusted proxy.
func (a *API) clientIP(r *http.Request) string {
	ip, ok := remoteIP(r)
	if !ok {
		return r.RemoteAddr
	}
	if a.trustedContains(ip) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if parsed, err := netip.ParseAddr(first); err == nil {
				return parsed.Unmap().String()
			}
		}
	}
	return ip.String()
}

// secureCookie is an explicit deployment decision. r.TLS is deliberately NOT
// consulted: Nginx terminates TLS and forwards plain HTTP to Go, so it is
// always nil in production and would silently downgrade the cookie.
func (a *API) secureCookie(r *http.Request) bool {
	if a.opts.CookieSecure {
		return true
	}
	ip, ok := remoteIP(r)
	if !ok || !a.trustedContains(ip) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

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
		if err != nil || expiresAt < a.nowUTC() {
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
