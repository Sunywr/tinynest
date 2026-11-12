package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	mux := NewMux(t.TempDir())
	r := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Fatalf("body %s", w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("api must never cache")
	}
}
func TestReadyDirectory(t *testing.T) {
	dir := t.TempDir()
	mux := NewMux(dir)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/readyz", nil))
	if w.Code != 200 {
		t.Fatalf("ready got %d", w.Code)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/v1/readyz", nil))
	if w2.Code != 503 {
		t.Fatalf("unready got %d", w2.Code)
	}
}
func TestMethod(t *testing.T) {
	w := httptest.NewRecorder()
	NewMux(t.TempDir()).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/healthz", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", w.Code)
	}
}
