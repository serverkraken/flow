// flow-server is the multi-device sync HTTP server for flow. See
// docs/superpowers/specs/2026-06-02-flow-client-server-phase1-design.md for
// the M1 design.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/adapter/systemclock"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui"
	"github.com/serverkraken/flow/internal/webui/handlers"
	webuimarkdown "github.com/serverkraken/flow/internal/webui/markdown"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// version is stamped via -ldflags "-X main.version=…" (Makefile,
// Dockerfile); "dev" für ungestempelte Builds.
var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// signal.NotifyContext makes ctx-cancel the single trigger for shutdown
	// — SIGINT (Ctrl-C during local dev) and SIGTERM (K8s pod termination)
	// both arrive here, no separate channel-fan-in required.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error("flow-server exit", slog.Any("err", err))
		os.Exit(1)
	}
}

// run wires every adapter and starts the HTTP server, blocking until ctx is
// cancelled (SIGTERM/SIGINT) or the listener fails. Extracted from main()
// so tests can drive the full lifecycle without forking a process.
//
// Returns nil on a clean drain, non-nil if the listener crashed or the
// drain timeout fired — main() maps a non-nil return to a non-zero exit so
// the container orchestrator (K8s, docker) sees the failure.
func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := httpserver.LoadConfig()
	if err != nil {
		return errors.New("config load failed: " + err.Error())
	}
	if err := requireConfig(cfg); err != nil {
		return errors.New("config validation failed: " + err.Error())
	}

	// --- Postgres store (R1: die einzige Wahrheit) ---------------------------

	pg, err := pgstore.Open(ctx, cfg.PgDSN)
	if err != nil {
		return errors.New("open postgres: " + err.Error())
	}
	defer pg.Close()

	users := pgstore.NewUsers(pg)
	projects := pgstore.NewProjects(pg)
	sessions := pgstore.NewSessions(pg)
	settings := pgstore.NewSettings(pg)
	activeStore := pgstore.NewActiveSessions(pg, sessions, settings)
	documents := pgstore.NewDocuments(pg)
	dayOffs := pgstore.NewDayOffs(pg)

	// --- OIDC + session cookie -----------------------------------------------

	provider, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		// Two Authentik providers (per_provider issuer mode) mint tokens for
		// flow-server: the browser confidential auth-code client and the public
		// CLI device-flow client. They carry DISTINCT `iss` claims and are
		// signed against distinct JWKS, so flow-server trusts both issuers (one
		// verifier each) AND both audiences — each JWT's `aud` is the issuing
		// client's id. For single-issuer IdPs the two issuer values are
		// identical and dedupe to a single verifier.
		Issuers:           []string{cfg.OIDCIssuer, cfg.OIDCCLIIssuer},
		AcceptedClientIDs: []string{cfg.OIDCClientID, cfg.OIDCCLIClientID},
	})
	if err != nil {
		return errors.New("oidc provider init failed: " + err.Error())
	}

	access := oidcserver.NewSubAllowlist(cfg.AllowedSubs)

	session, err := httpserver.NewSessionFromHex(cfg.CookieHashKey, cfg.CookieBlockKey)
	if err != nil {
		return errors.New("session keys invalid: " + err.Error())
	}

	_, tokenURL := provider.Endpoint()
	oidcCfg := httpserver.OIDCConfigResponse{
		Issuer:                 cfg.OIDCIssuer,
		DeviceAuthorizationURL: provider.DeviceAuthorizationURL(),
		TokenURL:               tokenURL,
		ClientID:               cfg.OIDCCLIClientID,
	}

	secure := strings.HasPrefix(cfg.BaseURL, "https://")

	// --- SSE broadcaster + 1Hz ticker (Plan E · Task 14) -------------------
	//
	// One broadcaster, shared by the mutating WebUI handlers (publish
	// session.* / project.* / note.*) and the SSE handler (subscribe).
	// The ticker goroutine fans out a per-second "tick" event so open
	// dashboards refresh their "läuft seit MM:SS" counters without
	// polling. Cancelled by runServer before srv.Shutdown returns, so the
	// ticker stops publishing into the broadcaster while in-flight SSE
	// requests drain. The defer here is the belt-and-suspenders cleanup
	// for early-return paths above.
	broadcaster := sse.New()
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	go runTicker(runCtx, broadcaster)

	// --- WebUI handlers (Plan E · Task 10) ---------------------------------
	webuiHandlers := buildWebUIHandlers(
		logger,
		cfg,
		sessions,
		activeStore,
		projects,
		documents,
		broadcaster,
	)

	srv := httpserver.NewWithAuth(httpserver.AuthDeps{
		Provider: provider,
		Access:   access,
		Session:  session,
		Users:    users,
		WorktimeAPI: &httpserver.WorktimeAPIDeps{
			Sessions: sessions, Active: activeStore, Settings: settings, Bus: broadcaster,
		},
		ProjectsAPI:  &httpserver.ProjectsAPIDeps{Projects: projects, Bus: broadcaster},
		DocumentsAPI: &httpserver.DocumentsAPIDeps{Store: documents, Bus: broadcaster},
		MiscAPI: &httpserver.DayOffsSettingsAPIDeps{
			DayOffs: dayOffs, Settings: settings, Bus: broadcaster,
		},
		Meta:         httpserver.MetaResponse{ServerVersion: version, MinClientVersion: "0.0.0"},
		WebUI:        webuiHandlers,
		Logger:       logger,
		BaseURL:      cfg.BaseURL,
		OIDCClientID: cfg.OIDCClientID,
		OIDCSecret:   cfg.OIDCClientSecret,
		Cookie:       httpserver.CookieConfig{Name: "flow_session", Secure: secure},
		Ready:        func() error { return pg.Ping(context.Background()) },
		OIDCConfig:   oidcCfg,
	})

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info(
		"flow-server starting",
		slog.String("addr", cfg.Addr),
		slog.String("base_url", cfg.BaseURL),
		slog.String("issuer", cfg.OIDCIssuer),
		slog.Int("allowed_subs", len(cfg.AllowedSubs)),
		slog.Duration("shutdown_timeout", cfg.ShutdownTimeout),
	)

	// runServer blocks until ctx is cancelled (SIGTERM/SIGINT) or the
	// listener crashes. runCancel stops the 1Hz SSE ticker BEFORE
	// srv.Shutdown returns so the ticker goroutine doesn't outlive its
	// broadcaster's request channels.
	return runServer(ctx, httpSrv, cfg.ShutdownTimeout, logger, runCancel)
}

// runServer starts srv.ListenAndServe in a goroutine, waits for ctx
// cancellation (SIGTERM / SIGINT), then drains in-flight requests with
// srv.Shutdown bounded by drainTimeout. beforeShutdown runs synchronously
// once shutdown is initiated and BEFORE srv.Shutdown is called — that's
// where the SSE ticker (and any other background goroutine sharing the
// http handler's lifetime) gets cancelled, so the broadcaster doesn't
// publish into draining response writers.
//
// Returns nil when the drain completes cleanly within the timeout.
// Returns the underlying error on:
//   - listener crashes (anything other than http.ErrServerClosed),
//   - drain exceeds drainTimeout (context.DeadlineExceeded — main maps
//     this to a non-zero exit so K8s sees the failure; we deliberately
//     do NOT call srv.Close() to force-kill connections, the process
//     dies and the orchestrator handles re-scheduling).
func runServer(
	ctx context.Context,
	srv *http.Server,
	drainTimeout time.Duration,
	logger *slog.Logger,
	beforeShutdown ...func(),
) error {
	listenErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErr <- err
			return
		}
		listenErr <- nil
	}()

	select {
	case <-ctx.Done():
		// Normal SIGTERM/SIGINT path — drain.
	case err := <-listenErr:
		// Listener died before signal arrived (port conflict, etc.).
		if err != nil {
			logger.Error("listener crashed", slog.Any("err", err))
			return err
		}
		// Server stopped cleanly without external trigger — shouldn't
		// happen under normal operation, but treat as graceful.
		return nil
	}

	start := time.Now()
	logger.Info(
		"shutdown initiated, draining",
		slog.Duration("timeout", drainTimeout),
	)

	for _, fn := range beforeShutdown {
		if fn != nil {
			fn()
		}
	}

	shutCtx, shutCancel := context.WithTimeout(context.Background(), drainTimeout)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error(
			"drain failed",
			slog.Duration("elapsed", time.Since(start)),
			slog.Any("err", err),
		)
		return err
	}

	// Wait for the listener goroutine to return ErrServerClosed (or nil)
	// so we don't race the channel send/receive on exit.
	<-listenErr

	logger.Info(
		"shutdown complete",
		slog.Duration("elapsed", time.Since(start)),
	)
	return nil
}

// buildWebUIHandlers assembles the WebUIHandlers bag handed to
// httpserver.NewWithAuth. Each per-route handler carries its own *Deps
// (per-handler-Deps convention — see internal/webui/handlers/dashboard.go);
// this function is the one place where the full set of concrete adapters
// + the markdown renderer + the clock get bound to the right routes.
func buildWebUIHandlers(
	logger *slog.Logger,
	cfg httpserver.Config,
	sessions *pgstore.Sessions,
	activeStore *pgstore.ActiveSessions,
	projects *pgstore.Projects,
	documents *pgstore.Documents,
	broadcaster *sse.Broadcaster,
) *httpserver.WebUIHandlers {
	_ = logger // no notebook warnings needed — documents is always wired
	clock := systemclock.New()
	mdRenderer := webuimarkdown.New()

	// One shared ServerWorktimeView is used by both Dashboard and
	// Worktime — the underlying SQL aggregations are cheap, but a single
	// view simplifies test seam-points (one Clock, one DefaultTarget).
	worktimeView := &usecase.ServerWorktimeView{
		Sessions:      sessions,
		Active:        activeStore,
		Clock:         clock,
		DefaultTarget: 8 * time.Hour,
	}

	startTime := time.Now()

	docDeps := handlers.DocumentsDeps{Store: documents, Markdown: mdRenderer, Clock: clock}
	docActionsDeps := handlers.DocumentActionsDeps{Store: documents, Bus: broadcaster}
	reposDeps := handlers.ReposDeps{Documents: documents, Markdown: mdRenderer, Clock: clock}
	noteActionsDeps := handlers.NoteActionsDeps{Documents: documents, Clock: clock, Bus: broadcaster}

	// All session-action handlers share the same Deps bag.
	sessionActionsDeps := handlers.SessionActionsDeps{
		Sessions:    sessions,
		Active:      activeStore,
		Projects:    projects,
		View:        worktimeView,
		Clock:       clock,
		DeviceLabel: "web",
		Bus:         broadcaster,
		PauseResume: activeStore,
	}

	// Project create/rename/archive.
	projectActionsDeps := handlers.ProjectActionsDeps{
		Projects: projects,
		Clock:    clock,
		Bus:      broadcaster,
	}

	return &httpserver.WebUIHandlers{
		Dashboard: handlers.NewDashboard(handlers.DashboardDeps{
			View:        worktimeView,
			Active:      activeStore,
			Sessions:    sessions,
			Projects:    projects,
			Clock:       clock,
			ActivityMax: 7,
		}),
		Worktime: handlers.NewWorktime(handlers.WorktimeDeps{
			View:     worktimeView,
			Active:   activeStore,
			Sessions: sessions,
			Projects: projects,
			Clock:    clock,
		}),
		SessionEdit:   handlers.NewSessionEdit(sessionActionsDeps),
		SessionPut:    handlers.NewSessionPut(sessionActionsDeps),
		SessionDelete: handlers.NewSessionDelete(sessionActionsDeps),
		ActiveStart:   handlers.NewActiveStart(sessionActionsDeps),
		ActiveStop:    handlers.NewActiveStop(sessionActionsDeps),

		// R1 — Pause-Statemachine.
		ActivePause:  handlers.NewActivePause(sessionActionsDeps),
		ActiveResume: handlers.NewActiveResume(sessionActionsDeps),

		// R1 — documents-backed notes.
		DocumentsIndex: handlers.NewDocumentsIndex(docDeps),
		DocumentView:   handlers.NewDocumentView(docDeps),
		DocumentEdit:   handlers.NewDocumentEdit(docDeps),
		DocumentPut:    handlers.NewDocumentPut(docActionsDeps),

		ReposIndex:   handlers.NewReposIndex(reposDeps),
		RepoNote:     handlers.NewRepoNote(reposDeps),
		RepoNoteEdit: handlers.NewRepoNoteEdit(noteActionsDeps),
		RepoNotePut:  handlers.NewRepoNotePut(noteActionsDeps),
		Projects: handlers.NewProjects(handlers.ProjectsDeps{
			Projects: projects,
			Sessions: sessions,
			Active:   activeStore,
			Clock:    clock,
		}),

		// Project create/rename/archive.
		ProjectNewForm:   handlers.NewProjectNewForm(projectActionsDeps),
		ProjectNewCancel: handlers.NewProjectNewCancel(projectActionsDeps),
		ProjectCreate:    handlers.NewProjectCreate(projectActionsDeps),
		ProjectEdit:      handlers.NewProjectEdit(projectActionsDeps),
		ProjectPut:       handlers.NewProjectPut(projectActionsDeps),
		ProjectArchive:   handlers.NewProjectArchive(projectActionsDeps),

		// SSE live updates.
		Events: handlers.NewEvents(broadcaster),
		Settings: handlers.NewSettings(handlers.SettingsDeps{
			ServerBaseURL: cfg.BaseURL,
			OIDCIssuer:    cfg.OIDCIssuer,
			ServerDBPath:  "PostgreSQL (FLOW_PG_DSN)",
			StartTime:     startTime,
			Clock:         clock,
		}),
		AuthLanding: handlers.NewLanding(handlers.AuthDeps{
			IssuerLabel: cfg.OIDCIssuer,
		}),
		StaticFS: webui.StaticFS(),
	}
}

// runTicker fans a "tick" event out to every subscribed dashboard once a
// second. Lets clients refresh "läuft seit MM:SS" without polling — the
// frontend JS handler in dashboard/index.templ + worktime/today.templ
// reads the tick and recomputes the elapsed label client-side.
//
// The 1Hz rate is intentional: granular enough that the counter doesn't
// look frozen, coarse enough that a few dozen open tabs don't pummel
// the goroutine. Drop policy in the broadcaster ensures a stuck tab
// can't slow this loop down.
func runTicker(ctx context.Context, b *sse.Broadcaster) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			b.PublishAll(sse.Event{Type: "tick", Data: now.Unix()})
		}
	}
}

// requireConfig fails fast if the operator forgot a required env var. Better
// to crash at boot than to serve 500s on every /login.
func requireConfig(c httpserver.Config) error {
	var missing []string
	if c.OIDCIssuer == "" {
		missing = append(missing, "FLOW_OIDC_ISSUER")
	}
	if c.OIDCClientID == "" {
		missing = append(missing, "FLOW_OIDC_CLIENT_ID")
	}
	if c.OIDCClientSecret == "" {
		missing = append(missing, "FLOW_OIDC_CLIENT_SECRET")
	}
	if c.CookieHashKey == "" {
		missing = append(missing, "FLOW_COOKIE_HASH_KEY")
	}
	if c.CookieBlockKey == "" {
		missing = append(missing, "FLOW_COOKIE_BLOCK_KEY")
	}
	if len(c.AllowedSubs) == 0 {
		missing = append(missing, "FLOW_ALLOWED_SUBS")
	}
	if c.PgDSN == "" {
		missing = append(missing, "FLOW_PG_DSN")
	}
	if len(missing) > 0 {
		return errors.New("missing required env vars: " + strings.Join(missing, ", "))
	}
	return nil
}
