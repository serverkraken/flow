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
	"path/filepath"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	kompfsstore "github.com/serverkraken/flow/internal/kompendium/adapter/fsstore"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	kompusecase "github.com/serverkraken/flow/internal/kompendium/usecase"
	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/adapter/systemclock"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui"
	"github.com/serverkraken/flow/internal/webui/handlers"
	webuimarkdown "github.com/serverkraken/flow/internal/webui/markdown"
)

// cliClientID is the OIDC client used by the CLI/MCP device-flow. Separate
// from the server's confidential client (FLOW_OIDC_CLIENT_ID) because the
// CLI is a public client without a client secret.
//
// Phase-1 keeps this hardcoded; Phase 2 will make it configurable via
// FLOW_OIDC_CLI_CLIENT_ID once we support multiple CLI installs.
const cliClientID = "flow-cli"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := httpserver.LoadConfig()
	if err != nil {
		logger.Error("config load failed", slog.Any("err", err))
		os.Exit(1)
	}
	if err := requireConfig(cfg); err != nil {
		logger.Error("config validation failed", slog.Any("err", err))
		os.Exit(1)
	}

	// --- SQLite server store -------------------------------------------------

	if err := os.MkdirAll(filepath.Dir(cfg.ServerDBPath), 0o755); err != nil {
		logger.Error("server db dir", slog.Any("err", err))
		os.Exit(1)
	}
	serverDB, err := sqliteserver.Open(cfg.ServerDBPath)
	if err != nil {
		logger.Error("open server db", slog.Any("err", err))
		os.Exit(1)
	}
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

	ctx := context.Background()
	provider, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuer:   cfg.OIDCIssuer,
		ClientID: cfg.OIDCClientID,
	})
	if err != nil {
		logger.Error("oidc provider init failed", slog.Any("err", err))
		os.Exit(1)
	}

	access := oidcserver.NewSubAllowlist(cfg.AllowedSubs)

	session, err := httpserver.NewSessionFromHex(cfg.CookieHashKey, cfg.CookieBlockKey)
	if err != nil {
		logger.Error("session keys invalid", slog.Any("err", err))
		os.Exit(1)
	}

	_, tokenURL := provider.Endpoint()
	oidcCfg := httpserver.OIDCConfigResponse{
		Issuer:                 cfg.OIDCIssuer,
		DeviceAuthorizationURL: provider.DeviceAuthorizationURL(),
		TokenURL:               tokenURL,
		ClientID:               cliClientID,
	}

	secure := strings.HasPrefix(cfg.BaseURL, "https://")

	// --- WebUI handlers (Plan E · Task 10) ---------------------------------
	//
	// Constructing the per-route handlers once here and passing them as a
	// single WebUIHandlers bag keeps server.go a pure router-wiring file.
	// Every handler depends on the same Clock + sqliteserver adapters
	// that already exist above, so the new wiring is additive — no
	// existing dependency needs to change shape.
	webuiHandlers := buildWebUIHandlers(
		logger,
		cfg,
		sessions,
		activeStore,
		projects,
		repos,
		repoNotes,
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
	)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server crashed", slog.Any("err", err))
		os.Exit(1)
	}
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
	}

	// M7 / Task 12 — note + repo-note editing handlers share their
	// own Deps bag. NoteStore stays nil-tolerant: when the notebook is
	// unconfigured the kompendium handlers return 404 rather than 500.
	noteActionsDeps := handlers.NoteActionsDeps{
		NoteStore: notesStore,
		Repos:     repos,
		RepoNotes: repoNotes,
		Clock:     clock,
	}

	// M7 / Task 13 — project create/rename/archive. Smaller deps bag —
	// projects don't need Sessions/Active/View like session-actions do.
	projectActionsDeps := handlers.ProjectActionsDeps{
		Projects: projects,
		Clock:    clock,
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
		NotesIndex: handlers.NewNotesIndex(notesDeps),
		NotesView:  handlers.NewNotesView(notesDeps),
		ReposIndex: handlers.NewReposIndex(reposDeps),
		RepoNote:   handlers.NewRepoNote(reposDeps),

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
