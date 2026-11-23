package api

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"tinynest/api/internal/db"
	"tinynest/api/internal/server"
)

// The fixture route exists only in the Go test binary on loopback. Production
// builds never expose answers, even when run with the browser test variables.
func TestBrowserFixtureServer(t *testing.T) {
	dir := os.Getenv("TINYNEST_E2E_FIXTURE_DIR")
	if dir == "" {
		t.Skip("only used by the Playwright web server")
	}
	database, err := db.Open(filepath.Join(dir, "tinynest.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	mailKey, err := os.ReadFile(filepath.Join(dir, "mail.key"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(database, Options{SetupToken: os.Getenv("TINYNEST_SETUP_TOKEN"), MailKey: mailKey})
	mux := server.NewMux(dir, server.Options{Register: func(m *http.ServeMux) {
		a.Register(m)
		m.HandleFunc("GET /api/v1/test/captcha/{id}", func(w http.ResponseWriter, r *http.Request) {
			a.captchas.mu.Lock()
			challenge, ok := a.captchas.entries[r.PathValue("id")]
			a.captchas.mu.Unlock()
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, 200, map[string]string{"answer": challenge.answer})
		})
	}})
	listener, err := net.Listen("tcp", "127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal(http.Serve(listener, mux))
}
