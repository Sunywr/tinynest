package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// Options let the V0.1 composition root add business routes and a real database
// readiness probe without changing the Phase-0 health check semantics by default.
type Options struct {
	ReadyCheck func() error
	Register   func(*http.ServeMux)
}

func NewMux(dataDir string, opts ...Options) *http.ServeMux {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	mux := http.NewServeMux()
	phase := "0"
	if opt.Register != nil {
		phase = "v0.1"
	}
	mux.HandleFunc("/api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !getOnly(w, r) {
			return
		}
		jsonOK(w, http.StatusOK, map[string]string{"app": "tinynest-api", "status": "ok", "phase": phase})
	})
	mux.HandleFunc("/api/v1/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !getOnly(w, r) {
			return
		}
		info, err := os.Stat(dataDir)
		if err != nil || !info.IsDir() {
			jsonOK(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "reason": "data_dir_unavailable"})
			return
		}
		// Phase 0 default: directory check ONLY (never claimed as a DB check).
		// V0.1 supplies a real quick_check + migration-version probe.
		check := "data_dir_exists_only"
		if opt.ReadyCheck != nil {
			if err := opt.ReadyCheck(); err != nil {
				jsonOK(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "reason": "database_unavailable"})
				return
			}
			check = "sqlite_quick_check_and_migrations"
		}
		jsonOK(w, http.StatusOK, map[string]string{"status": "ready", "phase": phase, "check": check})
	})
	if opt.Register != nil {
		opt.Register(mux)
	}
	return mux
}

func getOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", "GET")
	jsonOK(w, http.StatusMethodNotAllowed, map[string]string{"status": "method_not_allowed"})
	return false
}
func jsonOK(w http.ResponseWriter, status int, payload map[string]string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func HTTPServer(addr string, dataDir string, opts ...Options) *http.Server {
	handler := NewMux(dataDir, opts...)
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelError)}
}
