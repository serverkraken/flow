package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// handleHomeHome renders the Home landing page at GET /.
func (s *Server) handleHomeHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.homeDataFor(r.Context(), u, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HomePage(vm).Render(r.Context(), w)
}

// handleHomeFragment renders the inner Home content fragment at GET /ui/home
// (the SSE-swap target).
func (s *Server) handleHomeFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderHomeFragment(w, r, u, "")
}

// renderHomeFragment re-renders the Home fragment, optionally with an inline
// error banner. GET handlers funnel through here to render with an optional error.
func (s *Server) renderHomeFragment(w http.ResponseWriter, r *http.Request, u domain.User, errMsg string) {
	vm, err := s.homeDataFor(r.Context(), u, errMsg)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HomeFragment(vm).Render(r.Context(), w)
}

// homeDataFor builds the Home view model from today's daily stats, saldo
// tiles, burndown banner, newest documents, and activity logstream. Guards
// all usecase calls so the minimal test server (without worktime usecases)
// still serves a valid idle page. The running-session/timer-hero fields were
// retired in K3 Task 6 — the K1 shell timer widget owns that surface now.
func (s *Server) homeDataFor(ctx context.Context, u domain.User, errMsg string) (webui.HomeVM, error) {
	now := s.Clock.Now()

	vm := webui.HomeVM{
		Err: errMsg,
	}

	// Daily target + balance + saldo tiles + burndown banner
	// (degrade to zero when Stats is not wired, as with the minimal test server).
	if s.Stats.Sessions != nil {
		today, _ := s.Stats.Today(ctx, u.ID)

		// Saldo tile for Heute.
		vm.TodaySaldo = webui.FmtSaldoVerbose(today.Saldo)
		vm.TodayPos = today.Saldo >= 0
		vm.TodaySub = webui.FmtVerbose(today.Logged) + " / " + webui.FmtVerbose(today.Target)

		// Woche saldo: Mon–Fri only (exclude Sat/Sun per the recovered pattern).
		burndown, _ := s.Stats.Burndown(ctx, u.ID)
		weekDays, _ := s.Stats.Week(ctx, u.ID, time.Time{})
		var weekLogged, weekTarget time.Duration
		for _, wd := range weekDays {
			if wd.Date.Weekday() == time.Saturday || wd.Date.Weekday() == time.Sunday {
				continue
			}
			weekLogged += wd.Total(now)
			weekTarget += wd.Target
		}
		weekSaldo := weekLogged - weekTarget
		vm.WeekSaldo = webui.FmtSaldoVerbose(weekSaldo)
		vm.WeekPos = weekSaldo >= 0
		vm.WeekSub = webui.FmtVerbose(weekLogged) + " / " + webui.FmtVerbose(weekTarget)

		// Monat saldo + burndown banner.
		vm.MonthSaldo = webui.FmtSaldoVerbose(burndown.Saldo)
		vm.MonthPos = burndown.Saldo >= 0
		vm.MonthSub = webui.FmtVerbose(burndown.Total) + " / " + webui.FmtVerbose(burndown.Target)
		vm.Burndown = burndownBannerVM(burndown)
	}

	// Newest knowledge articles for the "Zuletzt im Wissen" section.
	// Guard: skip gracefully when ListDocuments is not wired (minimal test server).
	var names, colors map[string]string
	var kinds map[string]domain.NodeKind
	if s.ListDocuments.Docs != nil {
		docs, err := s.ListDocuments.Execute(ctx, u.ID, nil, nil)
		if err != nil {
			slog.WarnContext(ctx, "home: list documents failed", "err", err)
		}
		names, colors, kinds, err = s.nodeMaps(ctx, u.ID)
		if err != nil {
			slog.WarnContext(ctx, "home: nodeMaps failed", "err", err)
		}
		vm.NewestDocs = webui.BuildHomeNewest(docs, colors, 5)
	}

	// Activity logstream — guard: skip when ListActivity is not wired.
	if s.ListActivity.Activities != nil {
		entries, _, _ := s.ListActivity.Execute(ctx, u.ID, nil, nil, 15, 0)
		// If nodeMaps wasn't already loaded for docs, load it now for activity
		if names == nil {
			var nerr error
			if names, _, kinds, nerr = s.nodeMaps(ctx, u.ID); nerr != nil {
				slog.WarnContext(ctx, "home: nodeMaps for activity failed", "err", nerr)
			}
		}
		vm.LogEntries = webui.BuildActivityRows(entries, names, kinds, now)
		actors, _ := s.ListActivity.Actors(ctx, u.ID)
		vm.LogActors = actors
	}

	return vm, nil
}

// classToPrefix maps the WebUI chip class name to the kind-prefix used by ListActivity.
// Empty / unknown → nil (no filter).
func classToPrefix(class string) []string {
	switch class {
	case "zeit":
		return []string{"session"}
	case "wissen":
		return []string{"document"}
	case "struktur":
		return []string{"node"}
	case "frei":
		return []string{"dayoff"}
	default:
		return nil
	}
}

// handleHomeLogstream renders the logstream section fragment at
// GET /ui/home/logstream with optional class and actor filters.
func (s *Server) handleHomeLogstream(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	now := s.Clock.Now()

	class := r.URL.Query().Get("class")
	actor := r.URL.Query().Get("actor")

	var actorPtr *string
	if actor != "" {
		actorPtr = &actor
	}

	classes := classToPrefix(class)

	var entries []domain.ActivityEntry
	var logActors []string
	if s.ListActivity.Activities != nil {
		entries, _, _ = s.ListActivity.Execute(r.Context(), u.ID, classes, actorPtr, 15, 0)
		logActors, _ = s.ListActivity.Actors(r.Context(), u.ID)
	}

	names, _, kinds, nerr := s.nodeMaps(r.Context(), u.ID)
	if nerr != nil {
		slog.WarnContext(r.Context(), "home: nodeMaps for activity failed", "err", nerr)
	}

	vm := webui.HomeVM{
		LogEntries: webui.BuildActivityRows(entries, names, kinds, now),
		LogActors:  logActors,
		LogClass:   class,
		LogActor:   actor,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HomeLogstream(vm).Render(r.Context(), w)
}
