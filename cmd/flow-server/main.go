// flow-server is the multi-device sync HTTP server for flow. See
// docs/superpowers/specs/2026-06-02-flow-client-server-phase1-design.md for
// the M1 design.
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := httpserver.LoadConfig()
	if err != nil {
		logger.Error("config load failed", slog.Any("err", err))
		os.Exit(1)
	}

	srv := httpserver.New(func() error { return nil })

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("flow-server starting", slog.String("addr", cfg.Addr))
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server crashed", slog.Any("err", err))
		os.Exit(1)
	}
}
