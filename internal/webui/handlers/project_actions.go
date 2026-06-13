// project_actions.go — Plan E · Task 13 (M7).
//
// Browser-side write handlers for the /projects surface:
//
//   - GET  /projects/new                 → returns the inline NewProjectForm
//   - GET  /projects/new/cancel          → returns the NewProjectButton
//   - POST /projects                     → create + return fresh button + OOB row
//   - GET  /projects/{id}/edit           → inline rename form (or cancel → row)
//   - PUT  /projects/{id}                → rename (returns row partial)
//   - POST /projects/{id}/archive        → soft-delete (returns archived row)
//
// All handlers return HTML fragments (templ partials), not JSON — HTMX
// performs the in-page swap. Auth happens upstream via
// BrowserAuthMiddleware; a missing user is treated as 401 defensively
// here so a misconfigured route never leaks data.
//
// Archive uses POST + explicit /archive path rather than DELETE because
// archive is a SOFT-delete (sets archived_at, doesn't drop the row).
// DELETE semantics imply "won't come back"; POST + /archive matches the
// existing TUI verb "Projekt archivieren".
//
// CSRF: deferred to Phase 2 (single-user hobby surface, low priority).
// Same TODO as session_actions.go.
//
// Per-handler-Deps convention: ProjectActionsDeps bundles the concrete
// adapters the six handlers share. Constructed in
// cmd/flow-server/main.go alongside the other M7 deps bags.

package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui/format"
	"github.com/serverkraken/flow/internal/webui/sse"
	projectspartials "github.com/serverkraken/flow/internal/webui/templates/projects/partials"
)

// ProjectActionsDeps bundles the concrete adapter set used by the six
// M7 project-action handlers. Smaller than SessionActionsDeps because
// projects don't need Sessions/Active/View — the per-row aggregates
// only live on the index page render path.
type ProjectActionsDeps struct {
	Projects ProjectsStore
	Clock    ports.Clock

	// Bus broadcasts project.* events to the SSE stream. Optional — nil
	// makes publish a silent no-op so handler tests that don't care
	// about the bus stay tight.
	Bus *sse.Broadcaster
}

// publish is the nil-safe wrapper, mirroring SessionActionsDeps.publish.
func (d ProjectActionsDeps) publish(userID, eventType string, data any) {
	if d.Bus == nil {
		return
	}
	d.Bus.Publish(userID, sse.Event{Type: eventType, Data: data})
}

// — GET /projects/new --------------------------------------------------------

// NewProjectNewForm returns the handler for GET /projects/new. Renders
// the inline create form which swap-replaces the "+ Neues Projekt"
// button. Stateless — no DB read needed.
func NewProjectNewForm(_ ProjectActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := httpserver.UserFromContext(r.Context()); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = projectspartials.NewProjectForm(projectspartials.NewProjectFormVM{}).Render(r.Context(), w)
	})
}

// — GET /projects/new/cancel -------------------------------------------------

// NewProjectNewCancel returns the handler for GET /projects/new/cancel.
// Renders the button so the user can re-open the form. Symmetric with
// NewProjectNewForm — same swap target, opposite direction.
func NewProjectNewCancel(_ ProjectActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := httpserver.UserFromContext(r.Context()); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = projectspartials.NewProjectButton().Render(r.Context(), w)
	})
}

// — POST /projects -----------------------------------------------------------

// NewProjectCreate returns the handler for POST /projects. Reads the
// `name` form field, derives a slug via usecase.SlugFromName, walks the
// collision suffix loop, then inserts via Projects.EnsureBySlug.
//
// Response shape:
//
//   - 400 + NewProjectForm with Error: empty name
//   - 200 + NewProjectButton (main swap restores the button) + OOB
//     prepend of the new ProjectRow to #projects-list
//   - 500 on DB errors
func NewProjectCreate(d ProjectActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.PostForm.Get("name"))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = projectspartials.NewProjectForm(projectspartials.NewProjectFormVM{
				Name:  "",
				Error: "Bitte einen Projektnamen eingeben.",
			}).Render(r.Context(), w)
			return
		}

		slug, err := uniqueSlugFor(d.Projects, u.ID, name)
		if err != nil {
			slog.Error(
				"project create: slug resolve failed",
				slog.String("user_id", u.ID),
				slog.String("name", name),
				slog.String("err", err.Error()),
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		created, err := d.Projects.EnsureBySlug(u.ID, name, slug)
		if err != nil && isUniqueConstraintErr(err) {
			// TOCTOU race: another concurrent request grabbed `slug`
			// between uniqueSlugFor's probe and EnsureBySlug's INSERT.
			// Walk the suffix loop once more (now seeing the taken slug)
			// and retry. Capped at 1 retry — a second collision under
			// the same race would indicate a much larger contention
			// problem that warrants a real adapter-level fix.
			slug2, slugErr := uniqueSlugFor(d.Projects, u.ID, name)
			if slugErr != nil {
				slog.Error(
					"project create: slug resolve failed on retry",
					slog.String("user_id", u.ID),
					slog.String("name", name),
					slog.String("err", slugErr.Error()),
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			slug = slug2
			created, err = d.Projects.EnsureBySlug(u.ID, name, slug)
		}
		if err != nil {
			slog.Error(
				"project create: EnsureBySlug failed",
				slog.String("user_id", u.ID),
				slog.String("slug", slug),
				slog.String("err", err.Error()),
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		d.publish(u.ID, "project.created", map[string]any{
			"id":   created.ID,
			"slug": created.Slug,
			"name": created.Name,
		})

		row := buildProjectRowVM(created, d.Clock.Now())
		// Main swap: restore the button so the user can immediately add another.
		// OOB swap: prepend the new row to the projects list.
		_ = projectspartials.NewProjectButton().Render(r.Context(), w)
		_ = projectspartials.NewProjectRowOOB(row).Render(r.Context(), w)
	})
}

// — GET /projects/{id}/edit --------------------------------------------------

// NewProjectEdit returns the handler for GET /projects/{id}/edit.
// Responds with the inline rename form (partials.ProjectForm). When the
// `cancel=1` query is set, returns the read-only row instead so the
// "Abbrechen" button re-renders the original row.
func NewProjectEdit(d ProjectActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		p, err := d.Projects.GetByID(u.ID, id)
		if errors.Is(err, ports.ErrProjectNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error(
				"project edit: GetByID failed",
				slog.String("user_id", u.ID),
				slog.String("id", id),
				slog.String("err", err.Error()),
			)
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		// cancel=1 → render the read-only row. Cancel works on archived
		// rows too — restoring the row view is harmless either way.
		if r.URL.Query().Get("cancel") == "1" {
			_ = projectspartials.ProjectRow(buildProjectRowVM(p, d.Clock.Now())).Render(r.Context(), w)
			return
		}

		// Block rename of archived projects. The UI already hides the
		// pencil for archived rows, but a direct GET to
		// /projects/{id}/edit must also be rejected so the server is the
		// single source of truth for the invariant.
		if p.ArchivedAt != nil {
			http.Error(w, "archivierte Projekte können nicht umbenannt werden", http.StatusBadRequest)
			return
		}

		_ = projectspartials.ProjectForm(projectspartials.ProjectFormVM{
			ID:      p.ID,
			Name:    p.Name,
			Slug:    p.Slug,
			Version: p.Version,
		}).Render(r.Context(), w)
	})
}

// — PUT /projects/{id} -------------------------------------------------------

// NewProjectPut returns the handler for PUT /projects/{id}. Reads the
// form-encoded `name` + `version` fields and calls Projects.Upsert
// with expectedVersion. On version conflict re-renders the form with
// the SERVER's current state so the user sees the latest values.
//
// The slug is NOT changed by a rename — it stays stable so existing
// `flow start <slug>` invocations from CLI/TUI keep resolving the same
// project. This matches usecase.Projects.Rename's semantics.
//
// Response shape:
//
//   - 400 + ProjectForm with Error: empty name
//   - 409 + ProjectForm pre-filled with SERVER values (conflict)
//   - 200 + ProjectRow (success)
//   - 404 missing / cross-tenant
func NewProjectPut(d ProjectActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		existing, err := d.Projects.GetByID(u.ID, id)
		if errors.Is(err, ports.ErrProjectNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error("project put: GetByID failed", slog.String("id", id), slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		// Server-side guard mirroring NewProjectEdit: archived projects
		// can't be renamed even if a client crafts a direct PUT.
		if existing.ArchivedAt != nil {
			http.Error(w, "archivierte Projekte können nicht umbenannt werden", http.StatusBadRequest)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.PostForm.Get("name"))
		expected := parseProjectVersion(r.PostForm.Get("version"))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = projectspartials.ProjectForm(projectspartials.ProjectFormVM{
				ID:      existing.ID,
				Name:    existing.Name,
				Slug:    existing.Slug,
				Version: expected,
				Error:   "Bitte einen Projektnamen eingeben.",
			}).Render(r.Context(), w)
			return
		}

		updated := existing
		updated.Name = name

		saved, err := d.Projects.Upsert(updated, expected)
		if errors.Is(err, ports.ErrProjectVersionConflict) {
			httpserver.SyncConflicts.WithLabelValues("projects").Inc()
			current, gerr := d.Projects.GetByID(u.ID, id)
			if gerr != nil {
				slog.Error("project put: conflict re-read failed", slog.String("err", gerr.Error()))
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusConflict)
			renderProjectConflictForm(r.Context(), w, current)
			return
		}
		if err != nil {
			slog.Error("project put: Upsert failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		d.publish(u.ID, "project.renamed", map[string]any{
			"id":   saved.ID,
			"slug": saved.Slug,
			"name": saved.Name,
		})

		_ = projectspartials.ProjectRow(buildProjectRowVM(saved, d.Clock.Now())).Render(r.Context(), w)
	})
}

// — POST /projects/{id}/archive ----------------------------------------------

// NewProjectArchive returns the handler for POST /projects/{id}/archive.
// Soft-deletes the project via Projects.Archive (sets archived_at).
// Idempotent — archiving an already-archived row returns the same
// archived-state row partial.
//
// Returns the row in its archived state rather than removing it. The
// row stays in the list (the filter pills are inert for M7) so the user
// sees the visual change. Reactivate is deferred to Phase 2.
func NewProjectArchive(d ProjectActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		// Cross-tenant guard: GetByID with the caller's user-id 404s if
		// the row belongs to someone else.
		if _, err := d.Projects.GetByID(u.ID, id); err != nil {
			if errors.Is(err, ports.ErrProjectNotFound) {
				http.NotFound(w, r)
				return
			}
			slog.Error(
				"project archive: GetByID failed",
				slog.String("user_id", u.ID),
				slog.String("id", id),
				slog.String("err", err.Error()),
			)
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		if err := d.Projects.Archive(u.ID, id); err != nil {
			slog.Error(
				"project archive: Archive failed",
				slog.String("user_id", u.ID),
				slog.String("id", id),
				slog.String("err", err.Error()),
			)
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		// Re-read so the row carries the freshly-set archived_at.
		updated, err := d.Projects.GetByID(u.ID, id)
		if err != nil {
			slog.Error("project archive: post-read failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		d.publish(u.ID, "project.archived", map[string]any{
			"id":   updated.ID,
			"slug": updated.Slug,
		})

		_ = projectspartials.ProjectRow(buildProjectRowVM(updated, d.Clock.Now())).Render(r.Context(), w)
	})
}

// — helpers ------------------------------------------------------------------

// uniqueSlugFor walks the collision-suffix loop in lockstep with
// usecase.Projects.Create: derive base from name, then probe
// GetBySlug(slug) and append "-2", "-3", … until a free slot. The
// usecase variant lives behind the ports.ProjectStore interface; this
// helper reuses the SlugFromName rule directly so the WebUI doesn't
// take a circular dependency on usecase.Projects.
//
// TOCTOU note: this probe is NOT transactional with the subsequent
// EnsureBySlug insert. Two concurrent POST /projects with the same
// name can both see the slug free here, then the second INSERT fails
// with the (user_id, slug) UNIQUE constraint. NewProjectCreate
// detects that error via isUniqueConstraintErr and retries the walk
// once before giving up. A proper fix would push the slug walk into
// the adapter and do it inside a single transaction, but that's a
// larger refactor — deferred.
func uniqueSlugFor(p ProjectsStore, userID, name string) (string, error) {
	base := usecase.SlugFromName(name)
	slug := base
	for i := 2; ; i++ {
		_, err := p.GetBySlug(userID, slug)
		if errors.Is(err, ports.ErrProjectNotFound) {
			return slug, nil
		}
		if err != nil {
			return "", fmt.Errorf("uniqueSlugFor lookup: %w", err)
		}
		slug = base + "-" + strconv.Itoa(i)
		// Defensive cap — never spin forever. 1000 collision attempts
		// would be pathological; the user can rename to escape.
		if i > 1000 {
			return "", fmt.Errorf("uniqueSlugFor: collision cap exceeded for %q", base)
		}
	}
}

// isUniqueConstraintErr detects sqlite UNIQUE-constraint violations
// surfaced from EnsureBySlug. modernc.org/sqlite returns errors like
// "constraint failed: UNIQUE constraint failed: projects.user_id,
// projects.slug (2067)"; either substring matches.
//
// String-matching is brittle but the alternative — depending on
// modernc.org/sqlite's error types directly from the handler package —
// would bake a driver choice into the WebUI layer. The narrow
// fallback target (UNIQUE on user_id+slug) keeps the surface small.
func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint") || strings.Contains(msg, "constraint failed")
}

// parseProjectVersion folds a form field into the int64 expectedVersion.
// Empty or invalid input → 0 (matches the "first save" branch in the
// session_actions helper).
func parseProjectVersion(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// buildProjectRowVM resolves the read-only-row VM from a stored
// project. The "Zuletzt" / "Diese Woche" cells are stubbed in the
// HTMX-swap path — those columns are only accurate on the initial
// page render where the handler aggregates sessions; for a swapped
// row we deliberately render a neutral "—" so the user sees the
// state change without a misleading per-row aggregate.
//
// Phase 2 will refresh the swapped row via SSE; for now the user gets
// the canonical state on next full reload.
func buildProjectRowVM(p domain.Project, now time.Time) projectspartials.ProjectRowVM {
	isArchived := p.ArchivedAt != nil
	row := projectspartials.ProjectRowVM{
		ID:              p.ID,
		Name:            p.Name,
		Slug:            p.Slug,
		Archived:        isArchived,
		Version:         p.Version,
		WeekDuration:    "—",
		WeekCount:       "0",
		LastLabel:       "—",
		WeekDurationDim: true,
		LastDim:         true,
	}
	if isArchived {
		row.LastLabel = "archiviert"
	} else if !p.LastUsedAt.IsZero() {
		row.LastLabel = format.HumanRelativeTime(p.LastUsedAt, now)
		row.LastDim = false
	}
	return row
}

// renderProjectConflictForm re-renders the rename form pre-filled with
// the SERVER's current state on 409. Mirrors the session-conflict
// pattern but inlines the form (a project rename is a single-field
// change — a full two-column diff overlay would be overkill).
func renderProjectConflictForm(ctx context.Context, w http.ResponseWriter, current domain.Project) {
	_ = projectspartials.ProjectForm(projectspartials.ProjectFormVM{
		ID:      current.ID,
		Name:    current.Name,
		Slug:    current.Slug,
		Version: current.Version,
		Error:   "Versionskonflikt — der Server-Stand wurde inzwischen geändert. Aktueller Name übernommen.",
	}).Render(ctx, w)
}
