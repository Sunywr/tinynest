package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tinynest/api/internal/api"
	"tinynest/api/internal/db"
	"tinynest/api/internal/server"
)

// envInt / envSeconds return 0 on absence or invalid input, which the API layer
// interprets as "use conservative built-in defaults". Values are non-secret.
func envInt(name string) int {
	v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return 0
	}
	return v
}

func envSeconds(name string) time.Duration {
	n := envInt(name)
	if n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

func main() {
	listen := os.Getenv("TINYNEST_LISTEN_ADDR")
	if listen == "" {
		listen = "127.0.0.1:8787"
	}
	dataDir := os.Getenv("TINYNEST_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	dbPath := os.Getenv("TINYNEST_DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "tinynest.sqlite3")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		slog.Error("cannot create data directory", "error", err)
		os.Exit(1)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		slog.Error("cannot open SQLite database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	applied, err := database.Migrate(context.Background())
	if err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("SQLite ready", "applied_migrations", applied)

	// Derived from the embedded migration files, never a hardcoded count (P2-07).
	expectedVersions, err := db.EmbeddedVersions()
	if err != nil {
		slog.Error("cannot enumerate embedded migrations", "error", err)
		os.Exit(1)
	}

	// First-boot protection: an empty database without an operator-supplied token
	// must fail closed. The hint is deliberately non-sensitive (never log the token).
	setupToken := os.Getenv("TINYNEST_SETUP_TOKEN")
	var households int
	if err := database.QueryRow(`SELECT COUNT(*) FROM households`).Scan(&households); err != nil {
		slog.Error("cannot inspect initialization state", "error", err)
		os.Exit(1)
	}
	if households == 0 && setupToken == "" {
		slog.Warn("initialization disabled: empty database has no TINYNEST_SETUP_TOKEN; POST /api/v1/setup will refuse every request (token value is never logged)")
	}

	// Session cookie security is an explicit deployment decision. There is no
	// implicit fallback: a missing/invalid value refuses to start (fail closed).
	var cookieSecure bool
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TINYNEST_COOKIE_SECURE"))) {
	case "true":
		cookieSecure = true
	case "false":
		cookieSecure = false
	default:
		slog.Error("TINYNEST_COOKIE_SECURE must be explicitly set to 'true' (production HTTPS) or 'false' (local plain HTTP dev); refusing to start")
		os.Exit(1)
	}
	var trustedProxies []string
	for _, s := range strings.Split(os.Getenv("TINYNEST_TRUSTED_PROXIES"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			trustedProxies = append(trustedProxies, s)
		}
	}
	if !cookieSecure && !strings.HasPrefix(listen, "127.0.0.1:") &&
		!strings.HasPrefix(listen, "localhost:") && !strings.HasPrefix(listen, "[::1]:") {
		slog.Warn("session cookie Secure is explicitly disabled while the listener is not loopback; acceptable for local development ONLY")
	}

	readyCheck := func() error {
		var result string
		if err := database.QueryRow("PRAGMA quick_check").Scan(&result); err != nil {
			return err
		}
		if result != "ok" {
			return fmt.Errorf("quick_check: %s", result)
		}
		versions, err := database.Versions(context.Background())
		if err != nil {
			return err
		}
		got := make(map[int]bool, len(versions))
		for _, v := range versions {
			got[v] = true
		}
		for _, want := range expectedVersions {
			if !got[want] {
				return fmt.Errorf("migrations incomplete: missing version %d, applied %v", want, versions)
			}
		}
		return nil
	}

	businessAPI := api.New(database, api.Options{
		SetupToken:     setupToken,
		CookieSecure:   cookieSecure,
		TrustedProxies: trustedProxies,
		LoginLimiter: api.LoginLimiterConfig{
			MaxFailures: envInt("TINYNEST_LOGIN_MAX_FAILURES"),
			Window:      envSeconds("TINYNEST_LOGIN_WINDOW_SECONDS"),
			Lockout:     envSeconds("TINYNEST_LOGIN_LOCKOUT_SECONDS"),
		},
	})
	srv := server.HTTPServer(listen, dataDir, server.Options{
		ReadyCheck: readyCheck,
		Register:   businessAPI.Register,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()
	slog.Info("TinyNest API started", "listen", listen, "phase", "v0.1")
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("API failed", "error", err)
		os.Exit(1)
	}
}
