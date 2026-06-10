// session_actions.go — Plan E · Task 11 (M7).
//
// Browser-side write handlers for the /worktime?tab=heute surface:
//
//   - GET   /worktime/sessions/{id}/edit         → inline edit form
//   - PUT   /worktime/sessions/{id}              → update (HTMX row swap)
//   - DELETE /worktime/sessions/{id}             → delete (HTMX row removal)
//   - POST  /worktime/active/start               → start active (HTMX banner swap)
//   - POST  /worktime/active/stop                → stop active (HTMX banner removal)
//
// All handlers return HTML fragments (templ partials), not JSON — HTMX
// performs the in-page swap. Auth happens upstream via BrowserAuthMiddleware;
// a missing user is treated as 401 defensively here so a misconfigured
// route never leaks data.
//
// CSRF: deferred to Phase 2 (single-user hobby surface, low priority).
// TODO once audit logging lands.
//
// Per-handler-Deps convention: SessionActionsDeps bundles the concrete
// adapter set the five handlers share. Constructed in
// cmd/flow-server/main.go alongside Dashboard + Worktime.

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
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui/format"
	"github.com/serverkraken/flow/internal/webui/sse"
	"github.com/serverkraken/flow/internal/webui/templates/shared"
	"github.com/serverkraken/flow/internal/webui/templates/worktime/partials"
)

// SessionActionsDeps bundles the concrete adapter set used by all M7
// session-action handlers. The handlers share Sessions / Active /
// Projects (read for name resolution) / View (re-render Today after
// big mutations — only used today by start/stop banner) / Clock.
type SessionActionsDeps struct {
	Sessions *sqliteserver.Sessions
	Active   *sqliteserver.ActiveSessions
	Projects *sqliteserver.Projects
	View     *usecase.ServerWorktimeView
	Clock    ports.Clock

	// DeviceLabel is forwarded into ActiveSessions.Start as
	// started_on_device so the conflict-overlay can show "running on
	// mac-soenne". Optional; empty falls back to "web".
	DeviceLabel string

	// Bus broadcasts session.* events to the SSE stream so other open
	// dashboards refresh without polling. Optional — when nil, the
	// publish calls are silent no-ops. Tests inject a real broadcaster
	// + a recording subscriber to assert the right events fire.
	Bus *sse.Broadcaster
}

// publish is a nil-safe wrapper around Bus.Publish. Keeps the call sites
// in the action handlers terse — `d.publish(u.ID, "session.updated", …)`
// instead of `if d.Bus != nil { d.Bus.Publish(…) }`.
func (d SessionActionsDeps) publish(userID, eventType string, data any) {
	if d.Bus == nil {
		return
	}
	d.Bus.Publish(userID, sse.Event{Type: eventType, Data: data})
}

// — GET /worktime/sessions/{id}/edit -----------------------------------------

// NewSessionEdit returns the handler for GET /worktime/sessions/{id}/edit.
// Responds with the inline edit form (partials.SessionForm). When the
// `cancel=1` query is set, returns the read-only row instead so the
// "Abbrechen" button re-renders the original row.
func NewSessionEdit(d SessionActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		s, err := d.Sessions.GetByID(u.ID, id)
		if errors.Is(err, ports.ErrSessionNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error("session edit: GetByID failed", slog.String("user_id", u.ID), slog.String("id", id), slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		// cancel=1 → render the read-only row (server-version wins flow).
		if r.URL.Query().Get("cancel") == "1" {
			row := buildSessionRowVM(d, u.ID, s)
			_ = partials.SessionRow(row).Render(r.Context(), w)
			return
		}

		loc := d.Clock.Now().Location()
		form := partials.SessionFormVM{
			ID:          s.ID,
			StartLabel:  s.Start.In(loc).Format("15:04"),
			StopLabel:   s.Stop.In(loc).Format("15:04"),
			DateLabel:   s.Date.In(loc).Format("2006-01-02"),
			ProjectName: projectNameFor(d.Projects, u.ID, s.ProjectID),
			Tag:         s.Tag,
			Note:        s.Note,
			Duration:    format.HHMM(s.Elapsed),
			Version:     s.Version,
		}
		if err := partials.SessionForm(form).Render(r.Context(), w); err != nil {
			slog.Error("session edit: render failed", slog.String("err", err.Error()))
		}
	})
}

// — PUT /worktime/sessions/{id} ----------------------------------------------

// NewSessionPut returns the handler for PUT /worktime/sessions/{id}.
// Reads the form-encoded body, computes Elapsed = Stop − Start, calls
// Sessions.Upsert with expectedVersion from the hidden input or If-Match
// header. Returns:
//
//   - 200 + SessionRow on success
//   - 409 + ConflictOverlay on version mismatch
//   - 404 on missing/cross-tenant ID
//   - 400 on bad form input (the inline form re-renders with current
//     server state — the user can retry without losing data)
func NewSessionPut(d SessionActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		existing, err := d.Sessions.GetByID(u.ID, id)
		if errors.Is(err, ports.ErrSessionNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error("session put: GetByID failed", slog.String("id", id), slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		expected := readVersion(r)
		loc := d.Clock.Now().Location()
		updated, badInput := mergeSessionUpdate(existing, r, loc)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		if badInput != "" {
			// Re-render the form pre-populated with submitted values so
			// Soenne can fix the bad field without retyping.
			w.WriteHeader(http.StatusBadRequest)
			renderEditFormFromUpdate(r.Context(), w, d, u.ID, updated, expected, loc)
			return
		}

		saved, err := d.Sessions.Upsert(updated, expected)
		if errors.Is(err, ports.ErrSessionVersionConflict) {
			httpserver.SyncConflicts.WithLabelValues("sessions").Inc()
			// Fetch current server state, render conflict overlay.
			current, gerr := d.Sessions.GetByID(u.ID, id)
			if gerr != nil {
				slog.Error("session put: conflict re-read failed", slog.String("err", gerr.Error()))
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusConflict)
			renderConflictOverlay(r.Context(), w, current, updated, "put", loc)
			return
		}
		if err != nil {
			slog.Error("session put: Upsert failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		d.publish(u.ID, "session.updated", map[string]any{
			"id":         saved.ID,
			"project_id": saved.ProjectID,
			"duration":   int64(saved.Elapsed.Seconds()),
		})

		row := buildSessionRowVM(d, u.ID, saved)
		_ = partials.SessionRow(row).Render(r.Context(), w)
	})
}

// — DELETE /worktime/sessions/{id} -------------------------------------------

// NewSessionDelete returns the handler for DELETE /worktime/sessions/{id}.
// Reads expectedVersion from the If-Match header (the SessionRow partial
// sets it via hx-headers). Returns:
//
//   - 200 + empty body on success (HTMX swaps the row to nothing)
//   - 409 + ConflictOverlay on version mismatch
//   - 404 on missing/cross-tenant ID
func NewSessionDelete(d SessionActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		expected := readVersion(r)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		err := d.Sessions.Delete(u.ID, id, expected)
		if errors.Is(err, ports.ErrSessionNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, ports.ErrSessionVersionConflict) {
			httpserver.SyncConflicts.WithLabelValues("sessions").Inc()
			current, gerr := d.Sessions.GetByID(u.ID, id)
			if gerr != nil {
				slog.Error("session delete: conflict re-read failed", slog.String("err", gerr.Error()))
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusConflict)
			renderConflictOverlay(r.Context(), w, current, current, "delete", d.Clock.Now().Location())
			return
		}
		if err != nil {
			slog.Error("session delete: Delete failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		d.publish(u.ID, "session.deleted", map[string]any{"id": id})

		// Empty body — HTMX outerHTML swap removes the row.
		w.WriteHeader(http.StatusOK)
	})
}

// — POST /worktime/active/start ----------------------------------------------

// NewActiveStart returns the handler for POST /worktime/active/start.
// Reads the project_id from form data (the Today form uses an inline
// <select name="project_id">; this also accepts a path-param fallback
// via chi.URLParam for callers that want the M6-style URL).
//
// Returns the LiveBannerContainer fragment so HTMX can swap the static
// container with the new banner.
func NewActiveStart(d SessionActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Two path shapes accepted: /worktime/active/{project_id}/start
		// (path-style, matches the API surface) OR /worktime/active/start
		// with project_id in the form body (the inline picker uses this).
		projectID := chi.URLParam(r, "project_id")
		if projectID == "" {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			projectID = strings.TrimSpace(r.PostForm.Get("project_id"))
		}
		if projectID == "" {
			http.Error(w, "missing project_id", http.StatusBadRequest)
			return
		}
		// Verify project ownership; cross-tenant returns 404 to avoid leaks.
		if _, err := d.Projects.GetByID(u.ID, projectID); err != nil {
			if errors.Is(err, ports.ErrProjectNotFound) {
				http.NotFound(w, r)
				return
			}
			slog.Error("active start: project lookup", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		_ = r.ParseForm() // already parsed above when project_id was in body; idempotent.
		tag := strings.TrimSpace(r.PostForm.Get("tag"))
		note := strings.TrimSpace(r.PostForm.Get("note"))

		device := d.DeviceLabel
		if device == "" {
			device = "web"
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		a, err := d.Active.Start(u.ID, projectID, time.Now().UTC(), device, 0, tag, note)
		if errors.Is(err, ports.ErrActiveSessionConflict) {
			httpserver.SyncConflicts.WithLabelValues("active").Inc()
			// Server enforces ≤1 active per user → conflict means
			// another session is running. Re-render the banner from the
			// stored row so the user sees current state.
			w.WriteHeader(http.StatusConflict)
			renderLiveBanner(r.Context(), w, d, u.ID, d.Clock.Now())
			return
		}
		if err != nil {
			slog.Error("active start: Start failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		d.publish(u.ID, "session.started", map[string]any{
			"project_id": a.ProjectID,
			"started_at": a.StartedAt.Unix(),
			"tag":        a.Tag,
		})

		vm := buildBannerContainerVM(d, u.ID, &a, d.Clock.Now())
		_ = partials.LiveBannerContainer(vm).Render(r.Context(), w)
	})
}

// — POST /worktime/active/stop -----------------------------------------------

// NewActiveStop returns the handler for POST /worktime/active/stop.
// Reads the user's current active row (server enforces ≤1) and atomically
// transitions it into a finished session via ActiveSessions.Stop.
// Returns the empty LiveBannerContainer so the banner area is cleared.
//
// Phase 2: extend with `project_id` selector when parallel-tracking
// across projects lands.
func NewActiveStop(d SessionActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		rows, err := d.Active.ListByUser(u.ID)
		if err != nil {
			slog.Error("active stop: list failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if len(rows) == 0 {
			// Already stopped — render the empty container idempotently.
			_ = partials.LiveBannerContainer(partials.LiveBannerContainerVM{}).Render(r.Context(), w)
			return
		}
		ar := rows[0]
		stopped, err := d.Active.Stop(u.ID, ar.ProjectID, ar.Version, "", "")
		if errors.Is(err, ports.ErrActiveSessionConflict) {
			// Rare — another device stopped it. Re-render the empty container.
			_ = partials.LiveBannerContainer(partials.LiveBannerContainerVM{}).Render(r.Context(), w)
			return
		}
		if err != nil {
			slog.Error("active stop: Stop failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}

		d.publish(u.ID, "session.stopped", map[string]any{
			"id":         stopped.ID,
			"project_id": stopped.ProjectID,
			"duration":   int64(stopped.Elapsed.Seconds()),
		})

		_ = partials.LiveBannerContainer(partials.LiveBannerContainerVM{}).Render(r.Context(), w)
	})
}

// — helpers ------------------------------------------------------------------

// readVersion returns the expected version from either the If-Match
// header (HTMX delete + edit-form re-PUT) or the form field "version"
// (PUT submission). Zero on missing/invalid input → Upsert/Delete will
// reject with conflict if the stored row has a non-zero version, which
// is the right behavior for a stale page.
func readVersion(r *http.Request) int64 {
	if v := r.Header.Get("If-Match"); v != "" {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	if v := r.PostForm.Get("version"); v != "" {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// mergeSessionUpdate folds form fields (date / start / stop / tag / note)
// into the existing Session. Returns a description of the first bad
// field ("" → all valid) so the caller can decide between Upsert and
// re-rendering the form.
//
// Date + time inputs use HTML5 defaults (yyyy-mm-dd / HH:MM). Start/Stop
// are anchored to the Date in the user's location so DST transitions
// resolve consistently.
func mergeSessionUpdate(existing domain.Session, r *http.Request, loc *time.Location) (domain.Session, string) {
	dateStr := strings.TrimSpace(r.PostForm.Get("date"))
	startStr := strings.TrimSpace(r.PostForm.Get("start"))
	stopStr := strings.TrimSpace(r.PostForm.Get("stop"))
	tag := strings.TrimSpace(r.PostForm.Get("tag"))
	note := strings.TrimSpace(r.PostForm.Get("note"))

	out := existing
	if dateStr != "" {
		d, err := time.ParseInLocation("2006-01-02", dateStr, loc)
		if err != nil {
			return existing, "date"
		}
		out.Date = d
	}
	if startStr != "" {
		t, err := time.ParseInLocation("15:04", startStr, loc)
		if err != nil {
			return existing, "start"
		}
		out.Start = combineDateTime(out.Date, t, loc)
	}
	if stopStr != "" {
		t, err := time.ParseInLocation("15:04", stopStr, loc)
		if err != nil {
			return existing, "stop"
		}
		out.Stop = combineDateTime(out.Date, t, loc)
	}
	// If only the date changed (start/stop weren't re-submitted), re-anchor
	// the preserved wall-clock times to the new date so the date column
	// agrees with the start column. Without this the row stays anchored
	// to the OLD date and looks inconsistent in the UI.
	if dateStr != "" {
		if startStr == "" {
			out.Start = combineDateTime(out.Date, out.Start, loc)
		}
		if stopStr == "" {
			out.Stop = combineDateTime(out.Date, out.Stop, loc)
		}
	}
	out.Tag = tag
	out.Note = note
	// Defensive: stop before start → swap or reject? Reject is safer so
	// the user sees the explicit error. We treat zero/negative as a
	// validation failure.
	out.Elapsed = out.Stop.Sub(out.Start)
	if out.Elapsed <= 0 {
		return existing, "stop"
	}
	return out, ""
}

// combineDateTime anchors a wall-clock time to a calendar date in loc.
func combineDateTime(date, clock time.Time, loc *time.Location) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
}

// buildSessionRowVM turns a finished session into the partial VM with
// the project name resolved for display. The function lives here (not
// in worktime_helpers) because it crosses the handler boundary —
// per-handler-Deps convention keeps the dependency at the caller.
func buildSessionRowVM(d SessionActionsDeps, userID string, s domain.Session) partials.SessionRowVM {
	loc := d.Clock.Now().Location()
	return partials.SessionRowVM{
		ID:          s.ID,
		TimeLabel:   fmt.Sprintf("%s — %s", s.Start.In(loc).Format("15:04"), s.Stop.In(loc).Format("15:04")),
		ProjectName: projectNameFor(d.Projects, userID, s.ProjectID),
		Tag:         s.Tag,
		Note:        s.Note,
		Duration:    format.HHMM(s.Elapsed),
		Version:     s.Version,
	}
}

// renderEditFormFromUpdate re-renders the edit form with the user's
// submitted (but rejected) values. Lets Soenne fix a bad field without
// retyping the rest. Caller has already written the status code.
func renderEditFormFromUpdate(ctx context.Context, w http.ResponseWriter, d SessionActionsDeps, userID string, s domain.Session, version int64, loc *time.Location) {
	form := partials.SessionFormVM{
		ID:          s.ID,
		StartLabel:  s.Start.In(loc).Format("15:04"),
		StopLabel:   s.Stop.In(loc).Format("15:04"),
		DateLabel:   s.Date.In(loc).Format("2006-01-02"),
		ProjectName: projectNameFor(d.Projects, userID, s.ProjectID),
		Tag:         s.Tag,
		Note:        s.Note,
		Duration:    format.HHMM(s.Elapsed),
		Version:     version,
	}
	_ = partials.SessionForm(form).Render(ctx, w)
}

// renderConflictOverlay emits the two-column diff partial. Caller has
// already written the status code (409 in production).
func renderConflictOverlay(ctx context.Context, w http.ResponseWriter, server, local domain.Session, mode string, loc *time.Location) {
	vm := partials.ConflictOverlayVM{
		ID:            server.ID,
		ServerVersion: server.Version,
		ProjectID:     server.ProjectID,
		Mode:          mode,
		ServerRow:     conflictSideOf(server, "Server", loc),
		LocalRow:      conflictSideOf(local, "Dein Stand", loc),
	}
	_ = partials.ConflictOverlay(vm).Render(ctx, w)
}

func conflictSideOf(s domain.Session, label string, loc *time.Location) partials.ConflictRowSide {
	return partials.ConflictRowSide{
		Label:      label,
		TimeLabel:  fmt.Sprintf("%s — %s", s.Start.In(loc).Format("15:04"), s.Stop.In(loc).Format("15:04")),
		DateLabel:  s.Date.In(loc).Format("2006-01-02"),
		Tag:        s.Tag,
		Note:       s.Note,
		DurationS:  format.HHMM(s.Elapsed),
		StartLabel: s.Start.In(loc).Format("15:04"),
		StopLabel:  s.Stop.In(loc).Format("15:04"),
	}
}

// renderLiveBanner reads the current active row (if any) and renders
// the LiveBannerContainer. Used by Start's conflict path so the user
// sees the row that's actually running.
func renderLiveBanner(ctx context.Context, w http.ResponseWriter, d SessionActionsDeps, userID string, now time.Time) {
	rows, _ := d.Active.ListByUser(userID)
	var active *domain.ActiveSession
	if len(rows) > 0 {
		ar := rows[0]
		active = &ar
	}
	vm := buildBannerContainerVM(d, userID, active, now)
	_ = partials.LiveBannerContainer(vm).Render(ctx, w)
}

// buildBannerContainerVM resolves the project name and computes the
// elapsed label for a banner refresh. Empty `active` → empty container.
func buildBannerContainerVM(d SessionActionsDeps, userID string, active *domain.ActiveSession, now time.Time) partials.LiveBannerContainerVM {
	if active == nil {
		return partials.LiveBannerContainerVM{}
	}
	label := active.ProjectID
	if p, err := d.Projects.GetByID(userID, active.ProjectID); err == nil {
		label = p.Name
	}
	return partials.LiveBannerContainerVM{
		HasActive: true,
		Banner: shared.LiveBanner{
			ProjectLabel: label,
			Tag:          active.Tag,
			ElapsedLabel: formatElapsedHumane(now.Sub(active.StartedAt)),
			StartedAt:    active.StartedAt.In(now.Location()).Format("15:04"),
			SinceLabel:   "→ läuft",
			StopHref:     "/worktime/active/stop",
			// SSE tick consumer in worktime/today reads this off the
			// rendered `.live-elapsed` data attribute to advance the
			// counter client-side.
			StartedUnix: active.StartedAt.Unix(),
		},
	}
}

// projectNameFor resolves a project name with a single DB read.
// Falls back to projectID on lookup error so the row still renders.
func projectNameFor(projects *sqliteserver.Projects, userID, projectID string) string {
	if projects == nil {
		return projectID
	}
	p, err := projects.GetByID(userID, projectID)
	if err != nil {
		return projectID
	}
	return p.Name
}
