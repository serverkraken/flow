package httpserver

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Server wires the HTTP handlers into a Chi router. Construction is
// deliberately small — every endpoint group lives in its own file so the
// router stays a wiring index, never a god-object.
type Server struct {
	router  chi.Router
	baseURL string
}

// New constructs the server with the supplied readiness check. The check is
// run on every /readyz request; pass `func() error { return nil }` when no
// dependencies have been wired up yet.
func New(readyCheck ReadinessCheck) *Server {
	r := chi.NewRouter()
	r.Handle("/healthz", NewHealthzHandler())
	r.Handle("/readyz", NewReadyzHandler(readyCheck))
	return &Server{router: r}
}

// Handler returns the underlying http.Handler for use with http.Server.
func (s *Server) Handler() http.Handler { return s.router }

// NewWithAuth assembles the Phase-1-M1 server: healthz + readyz + browser
// auth (login/callback/logout). Plain New() is kept for tests that don't
// need auth.
func NewWithAuth(d AuthDeps) *Server {
	r := chi.NewRouter()
	// Global metrics middleware — wraps EVERY handler below so
	// /healthz, /readyz, the auth surface and the WebUI all show up
	// in flow_http_requests_total. The middleware skips /metrics
	// internally to avoid self-observation.
	r.Use(NewMetricsMiddleware)
	// Structured JSON request log (Plan F · Task 7). Sits between
	// metrics and auth so its status field reflects the post-auth
	// outcome (401/403 verdicts show up in the log) while still
	// being observed by the metrics counter above.
	r.Use(NewLogMiddleware(d.Logger))

	// /metrics is intentionally UNAUTHENTICATED. Prometheus scrapes
	// anonymously; access control belongs at the network layer
	// (NetworkPolicy / ServiceMesh / ingress allowlist).
	r.Handle(metricsPath, NewMetricsHandler())

	r.Handle("/healthz", NewHealthzHandler())
	r.Handle("/readyz", NewReadyzHandler(d.Ready))

	r.Handle("/api/v1/oidc/config", NewOIDCConfigHandler(d.OIDCConfig))
	r.Handle("/api/v1/meta", NewMetaHandler(d.Meta))

	ab := newAuthBrowser(d)
	r.Get("/login", ab.handleLogin)
	r.Get("/auth/callback", ab.handleCallback)
	r.Get("/logout", ab.handleLogout)

	// /api/v1/me requires a valid session cookie.
	r.Group(func(rr chi.Router) {
		rr.Use(NewAuthMiddleware(d.Session, d.Cookie.Name))
		rr.Get("/api/v1/me", NewMeHandler().ServeHTTP)
	})

	// Bearer-protected API surface (Spec §7). me-bearer bleibt als
	// CLI-Identitäts-Probe erhalten.
	r.Group(func(rr chi.Router) {
		rr.Use(NewBearerMiddleware(d.Provider, d.Access, d.Users))
		rr.Get("/api/v1/me-bearer", NewMeHandler().ServeHTTP)
		rr.Route("/api/v1", func(api chi.Router) {
			if d.WorktimeAPI != nil {
				MountWorktimeAPI(api, *d.WorktimeAPI)
			}
			if d.ProjectsAPI != nil {
				MountProjectsAPI(api, *d.ProjectsAPI)
			}
			if d.DocumentsAPI != nil {
				MountDocumentsAPI(api, *d.DocumentsAPI)
			}
			if d.MiscAPI != nil {
				MountDayOffsSettingsAPI(api, *d.MiscAPI)
			}
		})
	})

	mountWebUI(r, d)

	return &Server{router: r, baseURL: d.BaseURL}
}

// mountWebUI wires the WebUI surface onto r. Split out of NewWithAuth to
// keep the composition root's cognitive complexity below the lint gate;
// the function is purely declarative (each `if` is a nil-guard around a
// single route registration). Each handler field is nil-guarded so
// partial wiring (e.g. in handler tests that mount a subset) doesn't
// crash the rest of the WebUI.
func mountWebUI(r chi.Router, d AuthDeps) {
	if d.WebUI == nil {
		return
	}
	w := d.WebUI

	// Static assets sit OUTSIDE the cookie group: the login landing
	// itself needs /static/styles.css to render usefully, and the
	// embedded fonts/JS bundles are public anyway.
	if w.StaticFS != nil {
		r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(w.StaticFS))))
	}

	r.Group(func(rr chi.Router) {
		rr.Use(NewBrowserAuthMiddleware(d.Session, d.Cookie.Name, d.Users))
		mountWebUIRead(rr, w)
		mountWebUIProjectWrites(rr, w)
		mountWebUISessionWrites(rr, w)
	})

	// /api/v1/events bedient Browser (Cookie) UND TUI/MCP (Bearer) über
	// dieselbe Route (Spec §5/§7).
	if w.Events != nil {
		r.Group(func(rr chi.Router) {
			rr.Use(NewBearerOrCookieMiddleware(
				NewBearerMiddleware(d.Provider, d.Access, d.Users),
				NewBrowserAuthMiddleware(d.Session, d.Cookie.Name, d.Users),
			))
			rr.Method(http.MethodGet, "/api/v1/events", w.Events)
		})
	}

	// Auth landing is mounted OUTSIDE the cookie group — the
	// middleware redirects unauthenticated traffic here, so it
	// MUST be reachable without a session cookie (chicken/egg).
	// The BrowserAuthMiddleware's internal LandingPath bypass is
	// defence in depth; this explicit out-of-group registration
	// is the real escape hatch.
	if w.AuthLanding != nil {
		r.Method(http.MethodGet, LandingPath, w.AuthLanding)
	}
}

// mountWebUIRead wires the read-only WebUI routes (dashboard, worktime,
// notes, repos, projects, settings).
func mountWebUIRead(rr chi.Router, w *WebUIHandlers) {
	if w.Dashboard != nil {
		rr.Method(http.MethodGet, "/", w.Dashboard)
	}
	if w.Worktime != nil {
		rr.Method(http.MethodGet, "/worktime", w.Worktime)
	}
	if w.DocumentsIndex != nil {
		rr.Method(http.MethodGet, "/notes", w.DocumentsIndex)
	}
	if w.DocumentView != nil {
		// Multi-segment paths like "daily/2026-06-11.md" require chi's
		// wildcard. The edit form suffix (/edit) is dispatched at handler
		// level via notesGetDispatch so chi's wildcard stays unambiguous.
		rr.Method(http.MethodGet, "/notes/*", notesGetDispatch(w.DocumentView, w.DocumentEdit))
	}
	if w.DocumentPut != nil {
		// PUT shares the same wildcard so multi-segment paths route correctly.
		rr.Method(http.MethodPut, "/notes/*", w.DocumentPut)
	}
	if w.ReposIndex != nil {
		rr.Method(http.MethodGet, "/repos", w.ReposIndex)
	}
	if w.RepoNote != nil {
		rr.Method(http.MethodGet, "/repos/{key}/note", w.RepoNote)
	}
	if w.RepoNoteEdit != nil {
		rr.Method(http.MethodGet, "/repos/{key}/note/edit", w.RepoNoteEdit)
	}
	if w.RepoNotePut != nil {
		rr.Method(http.MethodPut, "/repos/{key}/note", w.RepoNotePut)
	}
	if w.Projects != nil {
		rr.Method(http.MethodGet, "/projects", w.Projects)
	}
	if w.Settings != nil {
		rr.Method(http.MethodGet, "/settings", w.Settings)
	}
}

// mountWebUIProjectWrites wires the M7 project write surface. The
// /projects/new + /projects/new/cancel routes power the button↔form
// swap; /projects POST creates; /projects/{id}/edit + PUT /projects/{id}
// rename; POST /projects/{id}/archive soft-deletes.
func mountWebUIProjectWrites(rr chi.Router, w *WebUIHandlers) {
	if w.ProjectNewForm != nil {
		rr.Method(http.MethodGet, "/projects/new", w.ProjectNewForm)
	}
	if w.ProjectNewCancel != nil {
		rr.Method(http.MethodGet, "/projects/new/cancel", w.ProjectNewCancel)
	}
	if w.ProjectCreate != nil {
		rr.Method(http.MethodPost, "/projects", w.ProjectCreate)
	}
	if w.ProjectEdit != nil {
		rr.Method(http.MethodGet, "/projects/{id}/edit", w.ProjectEdit)
	}
	if w.ProjectPut != nil {
		rr.Method(http.MethodPut, "/projects/{id}", w.ProjectPut)
	}
	if w.ProjectArchive != nil {
		rr.Method(http.MethodPost, "/projects/{id}/archive", w.ProjectArchive)
	}
}

// mountWebUISessionWrites wires the M7 session write surface — five
// routes for session edit/put/delete and active start/stop.
func mountWebUISessionWrites(rr chi.Router, w *WebUIHandlers) {
	if w.SessionEdit != nil {
		rr.Method(http.MethodGet, "/worktime/sessions/{id}/edit", w.SessionEdit)
	}
	if w.SessionPut != nil {
		rr.Method(http.MethodPut, "/worktime/sessions/{id}", w.SessionPut)
	}
	if w.SessionDelete != nil {
		rr.Method(http.MethodDelete, "/worktime/sessions/{id}", w.SessionDelete)
	}
	if w.ActiveStart != nil {
		// Both path-style and body-style accepted; the inline
		// "Neue Session" picker uses /worktime/active/start
		// with project_id in the form, the API-shaped path
		// is kept for parity.
		rr.Method(http.MethodPost, "/worktime/active/start", w.ActiveStart)
		rr.Method(http.MethodPost, "/worktime/active/{project_id}/start", w.ActiveStart)
	}
	if w.ActiveStop != nil {
		rr.Method(http.MethodPost, "/worktime/active/stop", w.ActiveStop)
	}
	if w.ActivePause != nil {
		rr.Method(http.MethodPost, "/worktime/active/pause", w.ActivePause)
	}
	if w.ActiveResume != nil {
		rr.Method(http.MethodPost, "/worktime/active/resume", w.ActiveResume)
	}
}

// SetBaseURL allows tests to swap baseURL after the server is constructed
// (so RedirectURL matches httptest's random port).
func (s *Server) SetBaseURL(u string) { s.baseURL = u }

// notesGetDispatch routes GET /notes/* requests between the view
// handler and the edit handler based on the path suffix. Chi v5
// cannot mix a wildcard pattern with a literal trailing segment, so we
// disambiguate at the handler level. The wildcard captures multi-segment
// kompendium IDs (e.g. "projects/serverkraken/flow/foo") which may
// themselves end with `/edit` only when the user is asking for the
// edit form — Soenne owns the kompendium namespace, so the suffix
// hijack is safe.
//
// edit may be nil — degrade to the view handler (the edit form simply
// isn't mounted), which surfaces as a 404 on the kompendium-ID parse.
func notesGetDispatch(view, edit http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if edit != nil && strings.HasSuffix(r.URL.Path, "/edit") {
			edit.ServeHTTP(w, r)
			return
		}
		view.ServeHTTP(w, r)
	})
}
