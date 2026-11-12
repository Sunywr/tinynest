package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// Handlers intentionally expose ONLY infrastructure health checks; no financial API exists in Phase 0.
func NewMux(dataDir string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !getOnly(w, r) {
			return
		}
		jsonOK(w, http.StatusOK, map[string]string{"app": "tinynest-api", "status": "ok", "phase": "0"})
	})
	mux.HandleFunc("/api/v1/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !getOnly(w, r) {
			return
		}
		// Infra-only readiness: checks data directory availability, NOT SQLite integrity or a business database connection.
		info, err := os.Stat(dataDir)
		if err != nil || !info.IsDir() {
			jsonOK(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "reason": "data_dir_unavailable"})
			return
		}
		jsonOK(w, http.StatusOK, map[string]string{"status": "ready", "phase": "0", "check": "data_dir_exists_only"})
	})
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

func HTTPServer(addr string, dataDir string) *http.Server {
	handler := NewMux(dataDir)
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelError)}
}
