// Command flow-server is the flow API + SSE server (composition root).
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

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcverify"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/uuidgen"
	"github.com/serverkraken/flow/internal/config"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

func main() {
	if err := run(); err != nil {
		slog.Error("flow-server exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	pool, err := pgstore.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pgstore.Migrate(ctx, pool); err != nil {
		return err
	}
	verifier, err := oidcverify.New(ctx, cfg.OIDCIssuer, cfg.OIDCClientID)
	if err != nil {
		return err
	}

	srv := &httpserver.Server{
		Verifier: verifier,
		Ensure: usecase.EnsureUser{
			Users: pgstore.NewUserStore(pool),
			IDs:   uuidgen.Gen{},
			Allow: func(id ports.Identity) bool { return cfg.AllowedSubs[id.Username] || cfg.AllowedSubs[id.Subject] },
		},
		Bus: sse.NewBus(),
		Dev: cfg.Dev,
	}

	httpSrv := &http.Server{Addr: cfg.ListenAddr, Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr, "dev", cfg.Dev)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutCtx)
}
