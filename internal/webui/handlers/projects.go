// Package handlers — see dashboard.go for the per-handler-Deps
// convention. The projects handler is mounted at /projects and is
// read-only for M6; create/rename/archive land in M7 (Task 13).
package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	projectstmpl "github.com/serverkraken/flow/internal/webui/templates/projects"
)

// ProjectsDeps bundles exactly the data sources the /projects handler
// needs. Follows the per-handler-Deps convention established by
// DashboardDeps — see its doc comment for the rationale.
//
// All three data fields are concrete sqliteserver adapters (their
// server Upsert signatures carry expectedVersion and so don't satisfy
// the client-side ports). Clock is exposed so tests can pin "now" for
// relative-time + week aggregations.
type ProjectsDeps struct {
	Projects *sqliteserver.Projects
	Sessions *sqliteserver.Sessions
	Active   *sqliteserver.ActiveSessions
	Clock    ports.Clock
}

// projectsSubTab identifies the active sub-tab on /projects. The
// "quellen" tab is rendered as a placeholder for M6 since
// Quellverzeichnisse are TUI-only currently (see memory
// `project_tmux_flow_migration`).
type projectsSubTab string

const (
	projectsTabWorktime projectsSubTab = "worktime"
	projectsTabQuellen  projectsSubTab = "quellen"
)

// parseProjectsSubTab maps a raw `?tab=` query value to a sub-tab; an
// empty or unknown value falls through to "worktime" so a typo never
// 400s.
func parseProjectsSubTab(raw string) projectsSubTab {
	if projectsSubTab(raw) == projectsTabQuellen {
		return projectsTabQuellen
	}
	return projectsTabWorktime
}

// NewProjects returns the http.Handler mounted at /projects. The
// BrowserAuthMiddleware guarantees a domain.User in context; the
// handler fails closed with 401 if it's absent.
//
// Dispatch is by `?tab=` (worktime / quellen). Default + invalid values
// fall through to worktime.
func NewProjects(d ProjectsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Defensive 404 for any subpath under /projects — Phase 2 will
		// add /projects/:id when create/rename/archive land in M7.
		tail := strings.TrimPrefix(r.URL.Path, "/projects")
		tail = strings.TrimPrefix(tail, "/")
		if tail != "" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		tab := parseProjectsSubTab(r.URL.Query().Get("tab"))
		now := d.Clock.Now()

		vm, err := buildProjectsIndexVM(d, u.ID, tab, now)
		if err != nil {
			slog.Error("projects: build view-model failed",
				slog.String("user_id", u.ID),
				slog.String("error", err.Error()),
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		meta := layout.PageMeta{
			Title:       "Projekte",
			CurrentPath: "/projects",
			UserLabel:   userLabelFromContext(r.Context()),
			Spine:       layout.SpineState{SyncState: "ok"},
		}
		if err := layout.Base(meta, projectstmpl.Index(vm)).Render(r.Context(), w); err != nil {
			slog.Error("projects: render failed",
				slog.String("user_id", u.ID),
				slog.String("error", err.Error()),
			)
		}
	})
}
