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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/adapter/systemclock"
	kompfsstore "github.com/serverkraken/flow/internal/kompendium/adapter/fsstore"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	kompusecase "github.com/serverkraken/flow/internal/kompendium/usecase"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui"
	"github.com/serverkraken/flow/internal/webui/handlers"
	webuimarkdown "github.com/serverkraken/flow/internal/webui/markdown"
	"github.com/serverkraken/flow/internal/webui/sse"
)

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

	// --- SQLite server store -------------------------------------------------

	if err := os.MkdirAll(filepath.Dir(cfg.ServerDBPath), 0o755); err != nil {
		return errors.New("server db dir: " + err.Error())
	}
	serverDB, err := sqliteserver.Open(cfg.ServerDBPath)
	if err != nil {
		return errors.New("open server db: " + err.Error())
	}
	// Deferred Close runs AFTER runServer's Shutdown completes, so no
	// writer goroutine is still touching the DB when we close it.
	defer func() {
		if err := serverDB.Close(); err != nil {
			logger.Error("server db close", slog.Any("err", err))
		}
	}()

	users := sqliteserver.NewUsers(serverDB)
	projects := sqliteserver.NewProjects(serverDB)
	sessions := sqliteserver.NewSessions(serverDB)
	activeStore := sqliteserver.NewActiveSessions(serverDB)
	repos := sqliteserver.NewRepos(serverDB)
	repoNotes := sqliteserver.NewRepoNotes(serverDB)

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
		repos,
		repoNotes,
		broadcaster,
	)

	srv := httpserver.NewWithAuth(httpserver.AuthDeps{
		Provider:        provider,
		Access:          access,
		Session:         session,
		Users:           users,
		ProjectsServer:  projects,
		SessionsServer:  sessions,
		ActiveServer:    activeStore,
		ReposServer:     repos,
		RepoNotesServer: repoNotes,
		WebUI:           webuiHandlers,
		Logger:          logger,
		BaseURL:         cfg.BaseURL,
		OIDCClientID:    cfg.OIDCClientID,
		OIDCSecret:      cfg.OIDCClientSecret,
		Cookie:          httpserver.CookieConfig{Name: "flow_session", Secure: secure},
		Ready:           func() error { return serverDB.DB().Ping() },
		OIDCConfig:      oidcCfg,
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
//
// Notebook root (FLOW_NOTEBOOK_ROOT) is optional. When unset, the Notes
// handler renders a "Notes nicht konfiguriert" placeholder rather than
// 500ing — we log a single warning at boot so the operator sees the gap
// without losing the rest of the WebUI.
func buildWebUIHandlers(
	logger *slog.Logger,
	cfg httpserver.Config,
	sessions *sqliteserver.Sessions,
	activeStore *sqliteserver.ActiveSessions,
	projects *sqliteserver.Projects,
	repos *sqliteserver.Repos,
	repoNotes *sqliteserver.RepoNotes,
	broadcaster *sse.Broadcaster,
) *httpserver.WebUIHandlers {
	clock := systemclock.New()
	mdRenderer := webuimarkdown.New()

	// Notebook is optional — Notes handler renders a placeholder when
	// Store + Lister are nil. Log once at boot so the gap is visible.
	var (
		notesStore  kompports.NoteStore
		notesLister *kompusecase.ListNotes
	)
	if cfg.NotebookRoot != "" {
		ns, err := kompfsstore.New(cfg.NotebookRoot)
		if err != nil {
			logger.Warn(
				"notes: fsstore init failed; /notes will render placeholder",
				slog.String("root", cfg.NotebookRoot),
				slog.Any("err", err),
			)
		} else {
			notesStore = ns
			notesLister = kompusecase.NewListNotes(ns)
		}
	} else {
		logger.Warn(
			"notes: FLOW_NOTEBOOK_ROOT unset; /notes will render placeholder",
		)
	}

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

	notesDeps := handlers.NotesDeps{
		Store:    notesStore,
		Lister:   notesLister,
		Markdown: mdRenderer,
		Clock:    clock,
	}
	reposDeps := handlers.ReposDeps{
		Repos:     repos,
		RepoNotes: repoNotes,
		Markdown:  mdRenderer,
		Clock:     clock,
	}

	// All five M7 session-action handlers share the same Deps bag.
	sessionActionsDeps := handlers.SessionActionsDeps{
		Sessions:    sessions,
		Active:      activeStore,
		Projects:    projects,
		View:        worktimeView,
		Clock:       clock,
		DeviceLabel: "web",
		Bus:         broadcaster,
	}

	// M7 / Task 12 — note + repo-note editing handlers share their
	// own Deps bag. NoteStore stays nil-tolerant: when the notebook is
	// unconfigured the kompendium handlers return 404 rather than 500.
	noteActionsDeps := handlers.NoteActionsDeps{
		NoteStore: notesStore,
		Repos:     repos,
		RepoNotes: repoNotes,
		Clock:     clock,
		Bus:       broadcaster,
	}

	// M7 / Task 13 — project create/rename/archive. Smaller deps bag —
	// projects don't need Sessions/Active/View like session-actions do.
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
		NotesIndex:    handlers.NewNotesIndex(notesDeps),
		NotesView:     handlers.NewNotesView(notesDeps),
		ReposIndex:    handlers.NewReposIndex(reposDeps),
		RepoNote:      handlers.NewRepoNote(reposDeps),

		// M7 / Task 12 — note + repo-note editing.
		NoteEdit:     handlers.NewNoteEdit(noteActionsDeps),
		NotePut:      handlers.NewNotePut(noteActionsDeps),
		RepoNoteEdit: handlers.NewRepoNoteEdit(noteActionsDeps),
		RepoNotePut:  handlers.NewRepoNotePut(noteActionsDeps),
		Projects: handlers.NewProjects(handlers.ProjectsDeps{
			Projects: projects,
			Sessions: sessions,
			Active:   activeStore,
			Clock:    clock,
		}),

		// M7 / Task 13 — project create/rename/archive.
		ProjectNewForm:   handlers.NewProjectNewForm(projectActionsDeps),
		ProjectNewCancel: handlers.NewProjectNewCancel(projectActionsDeps),
		ProjectCreate:    handlers.NewProjectCreate(projectActionsDeps),
		ProjectEdit:      handlers.NewProjectEdit(projectActionsDeps),
		ProjectPut:       handlers.NewProjectPut(projectActionsDeps),
		ProjectArchive:   handlers.NewProjectArchive(projectActionsDeps),

		// M7 / Task 14 — SSE live updates.
		Events: handlers.NewEvents(broadcaster),
		Settings: handlers.NewSettings(handlers.SettingsDeps{
			ServerBaseURL: cfg.BaseURL,
			OIDCIssuer:    cfg.OIDCIssuer,
			ServerDBPath:  cfg.ServerDBPath,
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
	if len(missing) > 0 {
		return errors.New("missing required env vars: " + strings.Join(missing, ", "))
	}
	return nil
}
