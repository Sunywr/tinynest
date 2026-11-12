package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tinynest/api/internal/server"
)

func main() {
	listen := os.Getenv("TINYNEST_LISTEN_ADDR")
	if listen == "" {
		listen = "127.0.0.1:8787"
	}
	dataDir := os.Getenv("TINYNEST_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	srv := server.HTTPServer(listen, dataDir)
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
	slog.Info("TinyNest Phase 0 API started", "listen", listen, "phase", "0")
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("API failed", "error", err)
		os.Exit(1)
	}
}
