// Command flow-server is the flow API + SSE server (composition root).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcauth"
	"github.com/serverkraken/flow/internal/adapter/oidcverify"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/systemclock"
	"github.com/serverkraken/flow/internal/adapter/uuidgen"
	"github.com/serverkraken/flow/internal/adapter/websession"
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
	verifier, err := oidcverify.New(ctx, cfg.OIDCIssuer, []string{cfg.OIDCClientID, cfg.OIDCCliClientID})
	if err != nil {
		return err
	}
	authn, err := oidcauth.New(ctx, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.RedirectURL())
	if err != nil {
		return err
	}

	userStore := pgstore.NewUserStore(pool)
	projectStore := pgstore.NewProjectStore(pool)
	sessionStore := pgstore.NewSessionStore(pool)
	dayOffStore := pgstore.NewDayOffStore(pool)
	settingsStore := pgstore.NewUserSettingsStore(pool)
	feedTokenStore := pgstore.NewFeedTokenStore(pool)
	clock := systemclock.Clock{}
	ids := uuidgen.Gen{}
	bus := sse.NewBus()

	srv := &httpserver.Server{
		Verifier: verifier,
		Ensure: usecase.EnsureUser{
			Users: userStore,
			IDs:   ids,
			Allow: func(id ports.Identity) bool { return cfg.AllowedSubs[id.Username] || cfg.AllowedSubs[id.Subject] },
		},
		Bus:           bus,
		Clock:         clock,
		Dev:           cfg.Dev,
		StartSession:  usecase.StartSession{Sessions: sessionStore, IDs: ids, Clock: clock},
		StopSession:   usecase.StopSession{Sessions: sessionStore, Projects: projectStore, Clock: clock},
		ListSessions:  usecase.ListSessions{Sessions: sessionStore, Clock: clock},
		CreateProject: usecase.CreateProject{Projects: projectStore, IDs: ids, Clock: clock},
		ListProjects:  usecase.ListProjects{Projects: projectStore},
		AddDayOffs:    usecase.AddDayOffs{Store: dayOffStore, Bus: bus},
		DeleteDayOff:  usecase.DeleteDayOff{Store: dayOffStore, Bus: bus},
		ListDayOffs:   usecase.ListDayOffs{Store: dayOffStore, Settings: settingsStore, Loc: time.Local},
		GetSettings:   usecase.GetSettings{Settings: settingsStore, Tokens: feedTokenStore},
		SetBundesland: usecase.SetBundesland{Settings: settingsStore},
		IcsFeed:       usecase.IcsFeed{Tokens: feedTokenStore, Store: dayOffStore, Clock: clock},
		RegenIcsToken: usecase.RegenerateIcsToken{Tokens: feedTokenStore, Clock: clock},
		Stats: usecase.StatsComputer{
			Sessions: sessionStore,
			Settings: settingsStore,
			DayOffs:  usecase.ListDayOffs{Store: dayOffStore, Settings: settingsStore, Loc: time.Local},
			Clock:    clock,
			Loc:      time.Local,
		},
		SetTarget: usecase.SetTargetConfig{Settings: settingsStore},
		Users:     userStore,
		OIDCAuth:  authn,
		Session:   websession.NewCodec(cfg.SessionSecret, 7*24*time.Hour),
	}

	httpSrv := &http.Server{Addr: cfg.ListenAddr, Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second}
	srvErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr, "dev", cfg.Dev)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()

	select {
	case err := <-srvErr:
		return fmt.Errorf("flow-server: listen: %w", err)
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutCtx)
}
