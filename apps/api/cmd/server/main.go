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
	"syscall"
	"time"

	"tinynest/api/internal/api"
	"tinynest/api/internal/db"
	"tinynest/api/internal/server"
)

const latestMigration = 7

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
		if len(versions) < latestMigration {
			return fmt.Errorf("migrations incomplete: %v", versions)
		}
		return nil
	}

	businessAPI := api.New(database)
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
