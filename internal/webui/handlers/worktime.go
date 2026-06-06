package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	"github.com/serverkraken/flow/internal/webui/templates/worktime"
)

// WorktimeDeps bundles exactly the data sources the /worktime handler
// needs. Follows the per-handler-deps convention established by
// DashboardDeps — see its doc comment for the rationale.
//
// Phase 2: when a DayOffStore lands server-side, add it here so the
// Frei tab can drop its placeholder.
type WorktimeDeps struct {
	View     *usecase.ServerWorktimeView
	Active   *sqliteserver.ActiveSessions
	Sessions *sqliteserver.Sessions
	Projects *sqliteserver.Projects
	Clock    ports.Clock

	// DeviceLabel is shown in the Today rail's Sync row. Optional; falls
	// back to "dieses gerät" when empty.
	DeviceLabel string
}

// NewWorktime returns the http.Handler that renders `/worktime`. The
// BrowserAuthMiddleware in front of this route injects the resolved
// domain.User; on a missing user we fail closed with 401 rather than
// rendering an empty page.
//
// Dispatch is by `?tab=` (German values: heute / woche / verlauf / frei).
// Default + invalid values fall through to heute so a typo never 400s
// the user — see TestWorktime_InvalidTab_FallsThroughToHeute.
func NewWorktime(d WorktimeDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		now := d.Clock.Now()
		tab := worktime.SubTab(r.URL.Query().Get("tab"))
		switch tab {
		case worktime.TabHeute, worktime.TabWoche, worktime.TabVerlauf, worktime.TabFrei:
			// recognised
		default:
			tab = worktime.TabHeute
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		switch tab {
		case worktime.TabWoche:
			renderTab(w, r, d, u, now, "Worktime · Woche", tab, renderWeek)
		case worktime.TabVerlauf:
			renderTab(w, r, d, u, now, "Worktime · Verlauf", tab, renderHistory)
		case worktime.TabFrei:
			renderTab(w, r, d, u, now, "Worktime · Frei", tab, renderFrei)
		default:
			renderTab(w, r, d, u, now, "Worktime · Heute", tab, renderToday)
		}
	})
}

// renderFn builds + renders one sub-tab. The full *http.Request is
// passed in so a sub-tab can read its own query params (Verlauf uses
// `?date=`) without going through context-key gymnastics.
type renderFn func(*http.Request, WorktimeDeps, domain.User, time.Time) (templ.Component, layout.SpineState, error)

func renderTab(
	w http.ResponseWriter,
	r *http.Request,
	d WorktimeDeps,
	u domain.User,
	now time.Time,
	title string,
	tab worktime.SubTab,
	build renderFn,
) {
	body, spine, err := build(r, d, u, now)
	if err != nil {
		slog.Error("worktime: build view-model failed",
			slog.String("user_id", u.ID),
			slog.String("tab", string(tab)),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	meta := layout.PageMeta{
		Title:       title,
		CurrentPath: "/worktime",
		UserLabel:   userLabel(u),
		Spine:       spine,
	}
	if err := layout.Base(meta, body).Render(r.Context(), w); err != nil {
		slog.Error("worktime: render failed",
			slog.String("user_id", u.ID),
			slog.String("tab", string(tab)),
			slog.String("error", err.Error()),
		)
	}
}

// — shared helpers used by every renderFn —

// firstActiveSession returns the first active row for userID (server
// enforces ≤1 per user; picking [0] is the correct shape) or nil if
// none. Errors are wrapped so the caller can propagate.
func firstActiveSession(d WorktimeDeps, userID string) (*domain.ActiveSession, error) {
	rows, err := d.Active.ListByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("active list: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	return &r, nil
}

// activeStartPtr returns a *time.Time to the active row's StartedAt or
// nil. Convenience so the call site doesn't reach into a possibly-nil
// pointer.
func activeStartPtr(a *domain.ActiveSession) *time.Time {
	if a == nil {
		return nil
	}
	t := a.StartedAt
	return &t
}

// todaySpine returns the spine state derived from today's sessions +
// the user's current active row. Every sub-tab (Heute / Woche /
// Verlauf / Frei) calls this so the left-edge status segment stays
// consistent regardless of which body is being shown.
func todaySpine(d WorktimeDeps, userID string, active *domain.ActiveSession, now time.Time) (layout.SpineState, error) {
	today, err := d.View.Today(userID)
	if err != nil {
		return layout.SpineState{}, fmt.Errorf("today (for spine): %w", err)
	}
	return layout.SpineState{
		AnyActive: active != nil,
		HourMask:  worktime.HourMask(today.Sessions, activeStartPtr(active), now),
		NowHour:   now.Hour(),
		SyncState: "ok",
	}, nil
}

// — pure helpers (no I/O, easy to test indirectly via the renderers) —

func weekTotals(week []domain.WeekDay, now time.Time) (logged, target time.Duration) {
	for _, wd := range week {
		logged += wd.Total(now)
		target += wd.Target
	}
	return logged, target
}

func countBookedDays(week []domain.WeekDay, now time.Time) int {
	n := 0
	for _, wd := range week {
		if wd.Total(now) > 0 {
			n++
		}
	}
	return n
}

// parseHistoryDate reads `?date=YYYY-MM-DD` and falls back to yesterday
// on missing/invalid input. Invalid input never produces a 400 — the
// user is shown yesterday's row instead.
//
// Future dates are also pulled back to yesterday so the verlauf surface
// stays consistent with the "past only" UX.
func parseHistoryDate(raw string, now time.Time) time.Time {
	yesterday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -1)
	if raw == "" {
		return yesterday
	}
	t, err := time.ParseInLocation(historyDateFormat, raw, now.Location())
	if err != nil {
		return yesterday
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if t.After(today) {
		return yesterday
	}
	return t
}

// defaultTargetFor mirrors ServerWorktimeView.targetFor — weekday returns
// the default target, weekend returns 0. Duplicated rather than exported
// from the usecase because the targetFor method is unexported and a
// public Target(date) accessor would over-grow the usecase API for this
// one caller.
func defaultTargetFor(date time.Time, defaultTarget time.Duration) time.Duration {
	wd := date.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return 0
	}
	return defaultTarget
}

// sessionLengthExtremes returns "<H:MM> · <weekday-short>" labels for
// the longest + shortest session in `sessions`. Empty strings when the
// slice is empty.
func sessionLengthExtremes(sessions []domain.Session, loc *time.Location) (longest, shortest string) {
	if len(sessions) == 0 {
		return "", ""
	}
	maxIdx, minIdx := 0, 0
	for i := 1; i < len(sessions); i++ {
		if sessions[i].Elapsed > sessions[maxIdx].Elapsed {
			maxIdx = i
		}
		if sessions[i].Elapsed < sessions[minIdx].Elapsed {
			minIdx = i
		}
	}
	longest = fmt.Sprintf("%s · %s", worktime.FormatHHMM(sessions[maxIdx].Elapsed), shortWeekdayLabel(sessions[maxIdx].Date.In(loc).Weekday()))
	shortest = fmt.Sprintf("%s · %s", worktime.FormatHHMM(sessions[minIdx].Elapsed), shortWeekdayLabel(sessions[minIdx].Date.In(loc).Weekday()))
	return longest, shortest
}

func shortWeekdayLabel(w time.Weekday) string {
	switch w {
	case time.Monday:
		return "Mo"
	case time.Tuesday:
		return "Di"
	case time.Wednesday:
		return "Mi"
	case time.Thursday:
		return "Do"
	case time.Friday:
		return "Fr"
	case time.Saturday:
		return "Sa"
	default:
		return "So"
	}
}
