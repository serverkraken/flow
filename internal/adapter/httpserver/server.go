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
	r.Handle("/healthz", NewHealthzHandler())
	r.Handle("/readyz", NewReadyzHandler(d.Ready))

	r.Handle("/api/v1/oidc/config", NewOIDCConfigHandler(d.OIDCConfig))

	ab := newAuthBrowser(d)
	r.Get("/login", ab.handleLogin)
	r.Get("/auth/callback", ab.handleCallback)
	r.Get("/logout", ab.handleLogout)

	// /api/v1/me requires a valid session cookie.
	r.Group(func(rr chi.Router) {
		rr.Use(NewAuthMiddleware(d.Session, d.Cookie.Name))
		rr.Get("/api/v1/me", NewMeHandler().ServeHTTP)
	})

	// Bearer-protected variant for CLI/MCP. Same handler, different auth proof.
	r.Group(func(rr chi.Router) {
		rr.Use(NewBearerMiddleware(d.Provider, d.Access, d.Users))
		rr.Get("/api/v1/me-bearer", NewMeHandler().ServeHTTP)
		if d.ProjectsServer != nil {
			rr.Get("/api/v1/projects", NewProjectsPullHandler(d.ProjectsServer).ServeHTTP)
			rr.Put("/api/v1/projects/{id}", NewProjectsPushHandler(d.ProjectsServer).ServeHTTP)
		}
		if d.SessionsServer != nil {
			rr.Get("/api/v1/sessions", NewSessionsPullHandler(d.SessionsServer).ServeHTTP)
			rr.Put("/api/v1/sessions/{id}", NewSessionsPushHandler(d.SessionsServer).ServeHTTP)
		}
		if d.ActiveServer != nil {
			rr.Get("/api/v1/active", NewActiveListHandler(d.ActiveServer).ServeHTTP)
			rr.Post("/api/v1/active/{project_id}/start", NewActiveStartHandler(d.ActiveServer).ServeHTTP)
			rr.Delete("/api/v1/active/{project_id}", NewActiveStopHandler(d.ActiveServer).ServeHTTP)
		}
		if d.ReposServer != nil {
			rr.Get("/api/v1/repos", NewReposPullHandler(d.ReposServer).ServeHTTP)
			rr.Put("/api/v1/repos/{id}", NewReposPushHandler(d.ReposServer).ServeHTTP)
		}
		if d.RepoNotesServer != nil {
			rr.Get("/api/v1/repo-notes", NewRepoNotesPullHandler(d.RepoNotesServer).ServeHTTP)
			rr.Put("/api/v1/repos/{repo_id}/note", NewRepoNotePushHandler(d.RepoNotesServer).ServeHTTP)
		}
	})

	// WebUI routes — cookie session, browser auth. Each handler field is
	// nil-guarded so partial wiring (e.g. notebook root unset) doesn't
	// crash the rest of the WebUI.
	if d.WebUI != nil {
		w := d.WebUI

		// Static assets sit OUTSIDE the cookie group: the login landing
		// itself needs /static/styles.css to render usefully, and the
		// embedded fonts/JS bundles are public anyway.
		if w.StaticFS != nil {
			r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(w.StaticFS))))
		}

		r.Group(func(rr chi.Router) {
			rr.Use(NewBrowserAuthMiddleware(d.Session, d.Cookie.Name, d.Users))
			if w.Dashboard != nil {
				rr.Method(http.MethodGet, "/", w.Dashboard)
			}
			if w.Worktime != nil {
				rr.Method(http.MethodGet, "/worktime", w.Worktime)
			}
			if w.NotesIndex != nil {
				rr.Method(http.MethodGet, "/notes", w.NotesIndex)
			}
			if w.NotesView != nil {
				// Multi-segment IDs like projects/serverkraken/flow/foo
				// require chi's wildcard, captured via URLParam(r, "*").
				// M7 (Task 12) reuses the same wildcard for the edit
				// form (suffix `/edit`) — chi cannot disambiguate a
				// wildcard from a literal trailing segment, so we
				// dispatch at the handler level via notesGetDispatch.
				rr.Method(http.MethodGet, "/notes/*", notesGetDispatch(w.NotesView, w.NoteEdit))
			}
			if w.NotePut != nil {
				// PUT shares the same wildcard so multi-segment IDs
				// route to the right place. method-mux discriminates
				// GET vs PUT, so there's no conflict with NotesView.
				rr.Method(http.MethodPut, "/notes/*", w.NotePut)
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

			// — M7 session write surface ----------------------------
			// All five routes share the cookie-auth group; each
			// handler is nil-guarded so partial wiring degrades
			// gracefully.
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
		})

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

	return &Server{router: r, baseURL: d.BaseURL}
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
