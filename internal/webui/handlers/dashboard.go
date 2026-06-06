package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui/format"
	"github.com/serverkraken/flow/internal/webui/templates/dashboard"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
)

// DashboardDeps bundles exactly the data sources this handler needs.
//
// The webui handlers each carry their own *Deps struct rather than
// sharing a single fat webui.ServerDeps bag. Rationale: each handler
// then expresses its data dependencies in its type signature
// (compile-time documentation), and a handler can be tested with stubs
// that satisfy only its own interfaces. The wiring in
// cmd/flow-server/main.go is the one place where the full set of
// sqliteserver adapters and usecases is assembled — which keeps it as
// the composition root.
//
// If a future handler needs more than ~5 fields here, that is a smell —
// either the page is doing too much or there is a missing aggregation
// usecase that should fold several store calls into one.
//
// All four data fields are concrete sqliteserver / usecase types — the
// sqliteserver adapters intentionally don't satisfy ports.SessionStore /
// ProjectStore because their signatures carry expectedVersion
// (server-side optimistic-concurrency) which the client-side ports
// don't have.
//
// Clock is exposed so tests can pin "now" and exercise hour-mask / week
// boundaries deterministically; in production wire a real clock.
//
// Phase 2: re-add Devices + Sync-Latenz when we have telemetry.
type DashboardDeps struct {
	View        *usecase.ServerWorktimeView
	Active      *sqliteserver.ActiveSessions
	Sessions    *sqliteserver.Sessions
	Projects    *sqliteserver.Projects
	Clock       ports.Clock
	ActivityMax int // 0 → defaults to 7 (mockup B count)
}

// NewDashboard returns the http.Handler that renders `/`. The
// BrowserAuthMiddleware in front of this route guarantees the request
// context carries a resolved domain.User — we extract it via the helper
// in adapter/httpserver. On a missing user we fail closed with a 401
// rather than rendering an empty page.
func NewDashboard(d DashboardDeps) http.Handler {
	if d.ActivityMax <= 0 {
		d.ActivityMax = 7
	}
	clock := d.Clock
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			// Defensive: middleware should always inject a user. Treat as 401
			// rather than silently rendering an empty dashboard.
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		now := clock.Now()

		vm, err := buildDashboardVM(d, u, now)
		if err != nil {
			slog.Error("dashboard: build view-model failed",
				slog.String("user_id", u.ID),
				slog.String("error", err.Error()),
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		meta := layout.PageMeta{
			Title:       "Dashboard",
			CurrentPath: "/",
			UserLabel:   userLabel(u),
			Spine: layout.SpineState{
				AnyActive: vm.HasActive,
				HourMask:  vm.DayBar,
				NowHour:   vm.NowHour,
				SyncState: "ok",
			},
		}
		if err := layout.Base(meta, dashboard.Index(vm)).Render(r.Context(), w); err != nil {
			slog.Error("dashboard: render failed",
				slog.String("user_id", u.ID),
				slog.String("error", err.Error()),
			)
		}
	})
}

// buildDashboardVM is split out so the test can exercise the data-shape
// glue without going through http. All DB reads happen here; the templ
// renderer only formats.
//
// No context.Context parameter: the sqliteserver adapters this function
// calls (Active, Sessions, Projects, View) don't accept ctx today, so a
// dead `_ context.Context` would only obscure that limitation. Thread
// ctx through here once those adapters grow ctx-aware methods.
func buildDashboardVM(d DashboardDeps, u domain.User, now time.Time) (dashboard.ViewModel, error) {
	vm := dashboard.ViewModel{
		Now:         now,
		HeaderLine:  format.FormatGermanDateHeader(now),
		HeaderTitle: "Heute · Übersicht",
		NowHour:     now.Hour(),
	}

	today, err := d.View.Today(u.ID)
	if err != nil {
		return vm, fmt.Errorf("today: %w", err)
	}
	todayTotal := today.Total(now)
	saldoToday := todayTotal - today.Target
	vm.TodayTotal = format.FormatHHMM(todayTotal)
	vm.TodayTarget = format.FormatHHMM(today.Target)
	vm.TodaySaldo = format.FormatSignedHHMM(saldoToday)
	vm.TodaySaldoPos = saldoToday >= 0

	// Week saldo aggregates the WeekDay rows.
	week, err := d.View.Week(u.ID)
	if err != nil {
		return vm, fmt.Errorf("week: %w", err)
	}
	weekLogged, weekTarget := dashboard.WeekTotals(week, now)
	saldoWeek := weekLogged - weekTarget
	vm.WeekSaldo = format.FormatSignedHHMM(saldoWeek)
	vm.WeekSaldoPos = saldoWeek >= 0
	vm.WeekLogged = format.FormatHHMM(weekLogged)
	vm.WeekTarget = format.FormatHHMM(weekTarget)

	// Active session lookup. Server enforces ≤1 per user; pick first.
	activeRows, err := d.Active.ListByUser(u.ID)
	if err != nil {
		return vm, fmt.Errorf("active list: %w", err)
	}
	var active *domain.ActiveSession
	if len(activeRows) > 0 {
		ar := activeRows[0]
		active = &ar
		vm.HasActive = true
		vm.ActiveTag = ar.Tag
		vm.ActiveStartedAt = ar.StartedAt.In(now.Location()).Format("15:04")
		vm.ActiveElapsed = formatElapsedHumane(now.Sub(ar.StartedAt))
		// Project name resolution — failures fall back to project id so
		// the banner still renders something sensible.
		if proj, err := d.Projects.GetByID(u.ID, ar.ProjectID); err == nil {
			vm.ActiveProject = proj.Name
		} else {
			vm.ActiveProject = ar.ProjectID
		}
	}

	// Day-bar mask from today's sessions + active tail.
	vm.DayBar = format.HourMask(today.Sessions, today.Active, now)

	// Activity feed — server has no EventLog yet, so M6 only emits
	// session-stopped + session-started rows. Fetch this week's sessions
	// for the feed (newest first).
	monday := format.MondayOf(now)
	sunday := monday.AddDate(0, 0, 6)
	weekSessions, err := d.Sessions.ListByUserDateRange(u.ID, monday, sunday)
	if err != nil {
		return vm, fmt.Errorf("week sessions: %w", err)
	}

	// Project name cache for the activity feed + top-project lookup.
	projectName := newProjectNameResolver(d.Projects, u.ID)
	vm.Activity = dashboard.BuildActivityStream(weekSessions, active, projectName, d.ActivityMax, now)

	// Mini-card: top project.
	topProjID, topProjTotal := dashboard.TopProjectOfWeek(weekSessions)
	if topProjID != "" {
		vm.TopProjectName = projectName(topProjID)
		vm.TopProjectTotal = formatElapsedHumane(topProjTotal)
		if weekLogged > 0 {
			share := int(topProjTotal * 100 / weekLogged)
			vm.TopProjectShare = fmt.Sprintf("%d%% der Woche", share)
		}
	}

	// Mini-card: session count.
	vm.WeekSessionCount = len(weekSessions)

	// Mini-card: last activity. Newest event from the unified feed.
	if len(vm.Activity) > 0 {
		ev := vm.Activity[0]
		label := "session beendet"
		if ev.Kind == dashboard.ActivitySessionStarted {
			label = "session gestartet"
		}
		vm.LastActivityText = fmt.Sprintf("%s · %s", format.HumanRelativeTime(ev.When, now), label)
	}

	return vm, nil
}

// newProjectNameResolver returns a closure that caches project-name
// lookups per request. The activity stream + top-project card both
// resolve names; a 2-3 row cache avoids hammering the DB.
func newProjectNameResolver(projects *sqliteserver.Projects, userID string) func(projectID string) string {
	cache := map[string]string{}
	return func(projectID string) string {
		if name, ok := cache[projectID]; ok {
			return name
		}
		p, err := projects.GetByID(userID, projectID)
		if err != nil {
			cache[projectID] = projectID
			return projectID
		}
		cache[projectID] = p.Name
		return p.Name
	}
}

// formatElapsedHumane renders a duration as "2h 14m" / "42m" / "8s".
// Used by the live-banner readout and the top-project mini-card so the
// surfaces match the mockup's friendly tone instead of the strict
// FormatHHMM used in the saldo row.
func formatElapsedHumane(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%dh %02dm", h, m)
}

// userLabel returns the best display string for the nav header — name
// first, email fallback. Empty when neither is set (login row).
func userLabel(u domain.User) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Email
}
