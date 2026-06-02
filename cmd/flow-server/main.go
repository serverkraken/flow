package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	srv := httpserver.New()

	addr := os.Getenv("FLOW_SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("flow-server starting", slog.String("addr", addr))
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server crashed", slog.Any("err", err))
		os.Exit(1)
	}
}
