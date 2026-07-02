package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
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
// error banner. POST action handlers (start/stop) funnel through here.
func (s *Server) renderHomeFragment(w http.ResponseWriter, r *http.Request, u domain.User, errMsg string) {
	vm, err := s.homeDataFor(r.Context(), u, errMsg)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HomeFragment(vm).Render(r.Context(), w)
}

// handleHomeStart starts a session and re-renders the Home fragment at POST /ui/home/start.
// Mirrors handleWebStart (webui.go) but targets the Home fragment.
func (s *Server) handleHomeStart(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if sess, err := s.StartSession.Execute(r.Context(), u.ID, nil, nil, ""); err == nil {
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID,
			Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	}
	s.renderHomeFragment(w, r, u, "")
}

// handleHomeStop stops the running session and re-renders the Home fragment at
// POST /ui/home/stop. Mirrors handleWebStop (webui.go) but targets the Home fragment.
func (s *Server) handleHomeStop(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	sessionID := r.FormValue("sessionId")
	nodeID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if p, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{Name: name, Kind: domain.KindEngagement}); err == nil {
			nodeID = p.ID
			s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeCreated, UserID: u.ID, Data: map[string]any{"id": p.ID, "name": p.Name}})
		}
	}
	sess, err := s.StopSession.Execute(r.Context(), u.ID, sessionID, &nodeID)
	if err != nil {
		// Booking is mandatory: surface the reason instead of silently leaving the
		// timer running (otherwise Stop appears to "do nothing").
		msg := "Sitzung konnte nicht gestoppt werden."
		if errors.Is(err, domain.ErrProjectRequired) {
			msg = "Bitte ein Projekt wählen, um die Sitzung zu stoppen."
		}
		s.renderHomeFragment(w, r, u, msg)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	s.renderHomeFragment(w, r, u, "")
}

// homeDataFor builds the Home view model from today's sessions, the running
// session (if any), and daily stats. Mirrors heuteDataFor (webui_heute.go)
// for the timer-hero fields; guards all usecase calls so the minimal test
// server (without worktime usecases) still serves a valid idle page.
func (s *Server) homeDataFor(ctx context.Context, u domain.User, errMsg string) (webui.HomeVM, error) {
	now := s.Clock.Now()
	day := startOfDay(now)

	// Today's sessions (for LoggedDur computation via Stats).
	var sessions []domain.WorkSession
	if s.ListSessionsRange.Sessions != nil {
		var err error
		sessions, err = s.ListSessionsRange.Execute(ctx, u.ID, day, day.AddDate(0, 0, 1))
		if err != nil {
			return webui.HomeVM{}, err
		}
	}

	// Nodes for the engagement picker in the stop form.
	var projects []domain.Node
	if s.ListNodes.Nodes != nil {
		var err error
		projects, err = s.ListNodes.Execute(ctx, u.ID)
		if err != nil {
			return webui.HomeVM{}, err
		}
	}

	// Running session — use GetRunningSession so an overnight timer stays visible
	// and stoppable; fall back to scanning today's range for narrow test harnesses.
	var running *domain.WorkSession
	if s.GetRunningSession.Sessions != nil {
		if r, ok, rerr := s.GetRunningSession.Execute(ctx, u.ID); rerr == nil && ok {
			rs := r
			running = &rs
		}
	} else {
		for i := range sessions {
			if sessions[i].Running() {
				r := sessions[i]
				running = &r
			}
		}
	}

	vm := webui.HomeVM{
		Running: running,
		Err:     errMsg,
	}

	// Engagement picker — only KindEngagement nodes are bookable (Slice B).
	vm.Nodes = make([]components.NodePickerItem, 0, len(projects))
	for _, p := range projects {
		if p.Kind != domain.KindEngagement {
			continue
		}
		vm.Nodes = append(vm.Nodes, components.NodePickerItem{
			ID:    p.ID,
			Name:  p.Name,
			Hue:   p.Color,
			Glyph: glyphOr(p.Glyph),
			Rate:  rateLabel(p.Rate),
		})
	}
	vm.HasProj = len(vm.Nodes) > 0

	if running != nil {
		vm.RunningBase = heuteRunningBase(*running, now)
		vm.StartedAt = running.Start.Local().Format("15:04")
		vm.RunningTag = strings.Join(running.Tags, " ")
		name, hue := nodeIdentity(projects, running.NodeID)
		vm.RunningName = name
		vm.RunningHue = hue
	}

	// Daily target + balance + saldo tiles + burndown banner
	// (degrade to zero when Stats is not wired, as with the minimal test server).
	if s.Stats.Sessions != nil {
		today, _ := s.Stats.Today(ctx, u.ID)
		vm.LoggedDur = webui.FmtVerbose(today.Logged)
		vm.TargetDur = webui.FmtVerbose(today.Target)
		if today.Target > 0 {
			vm.TargetPct = webui.ClampPct(int(today.Logged * 100 / today.Target))
		}
		vm.TargetVar = heuteTargetVariant(today, running != nil)
		vm.Balance = webui.FmtSaldoVerbose(today.Saldo)
		vm.BalancePos = today.Saldo >= 0

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
	if s.ListDocuments.Docs != nil {
		docs, err := s.ListDocuments.Execute(ctx, u.ID, nil, nil)
		if err != nil {
			slog.WarnContext(ctx, "home: list documents failed", "err", err)
		}
		_, colors, _, err := s.nodeMaps(ctx, u.ID)
		if err != nil {
			slog.WarnContext(ctx, "home: nodeMaps failed", "err", err)
		}
		vm.NewestDocs = webui.BuildHomeNewest(docs, colors, 5)
	}

	// Activity logstream — guard: skip when ListActivity is not wired.
	if s.ListActivity.Activities != nil {
		entries, _, _ := s.ListActivity.Execute(ctx, u.ID, nil, nil, 15, 0)
		vm.LogEntries = webui.BuildActivityRows(entries, now)
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

	vm := webui.HomeVM{
		LogEntries: webui.BuildActivityRows(entries, now),
		LogActors:  logActors,
		LogClass:   class,
		LogActor:   actor,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HomeLogstream(vm).Render(r.Context(), w)
}
