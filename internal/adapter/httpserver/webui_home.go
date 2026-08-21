package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// homeContinueCap / homeWissenCap / homePulseCap bound the Schreibtisch's
// three list sections (Mockup Z.371–435: 3/5/3 illustrative rows — the
// brief settles on 5/5/8 so a light user still sees a full-feeling page).
const (
	homeContinueCap = 5
	homeWissenCap   = 5
	homePulseCap    = 8
)

// handleHomeHome renders the Home landing page at GET /.
func (s *Server) handleHomeHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.homeDataFor(r.Context(), u, "")
	if err != nil {
		s.webServerError(w, r, err)
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
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HomeFragment(vm).Render(r.Context(), w)
}

// homeDataFor builds the Schreibtisch view model (L4 Task 2): the ONE
// running timer's display state (Jetzt), MRU bookable nodes
// (Weiterarbeiten), the newest documents (Zuletzt im Wissen), and the
// account-wide activity feed (Puls). Every usecase call is guarded so the
// minimal test server (without worktime usecases wired) still serves a
// valid idle page — owner-scoped throughout (every call carries u.ID).
func (s *Server) homeDataFor(ctx context.Context, u domain.User, errMsg string) (webui.HomeVM, error) {
	now := s.Clock.Now()
	vm := webui.HomeVM{Today: webui.FmtDeskDate(now), Err: errMsg}

	// Jetzt: the one running timer's display state.
	if s.GetRunningSession.Sessions != nil {
		if rs, ok, rerr := s.GetRunningSession.Execute(ctx, u.ID); rerr == nil && ok {
			vm.Now = s.homeRunningNowVM(ctx, u, rs, now)
		}
	}

	// Today's total logged time — feeds both the running and idle Jetzt-row.
	if s.Stats.Sessions != nil {
		today, _ := s.Stats.Today(ctx, u.ID)
		vm.TodayLogged = webui.FmtVerbose(today.Logged)
	}

	// Weiterarbeiten: MRU bookable nodes derived from the last 30 days of
	// sessions — the exact ListSessions signature/window the ⌘K-Palette
	// already uses (webui_palette.go), not ListSessionsRange.
	if s.ListSessions.Sessions != nil && s.ListNodes.Nodes != nil {
		sessions, serr := s.ListSessions.Execute(ctx, u.ID, now.AddDate(0, 0, -30))
		if serr != nil {
			slog.WarnContext(ctx, "home: list sessions failed", "err", serr)
		}
		nodes, nerr := s.ListNodes.Execute(ctx, u.ID)
		if nerr != nil {
			slog.WarnContext(ctx, "home: list nodes failed", "err", nerr)
		}
		vm.Continue = webui.BuildRecentNodes(sessions, nodes, now, homeContinueCap)
	}

	// Zuletzt im Wissen: newest documents, reusing SortedDocuments +
	// WissenRowFromDocument (the /wissen "Zuletzt aktualisiert" builders).
	if s.ListDocuments.Docs != nil {
		docs, derr := s.ListDocuments.Execute(ctx, u.ID, nil, nil)
		if derr != nil {
			slog.WarnContext(ctx, "home: list documents failed", "err", derr)
		}
		sorted := webui.SortedDocuments(docs)
		if len(sorted) > homeWissenCap {
			sorted = sorted[:homeWissenCap]
		}
		for _, d := range sorted {
			vm.RecentWissen = append(vm.RecentWissen, webui.WissenRowFromDocument(d, now))
		}
	}

	// Puls: account-wide activity feed.
	if s.ListActivity.Activities != nil {
		entries, _, aerr := s.ListActivity.Execute(ctx, u.ID, nil, nil, homePulseCap, 0)
		if aerr != nil {
			slog.WarnContext(ctx, "home: list activity failed", "err", aerr)
		}
		names, _, kinds, merr := s.nodeMaps(ctx, u.ID)
		if merr != nil {
			slog.WarnContext(ctx, "home: nodeMaps for activity failed", "err", merr)
		}
		vm.Puls = webui.BuildActivityRows(entries, names, kinds, now)
	}

	return vm, nil
}

// homeRunningNowVM resolves the running session's node (when bound) into
// the Jetzt row's display fields, including the Work/Privat flag via the
// node's ancestor chain (domain.ResolveCountsTowardTarget) — degrading to
// an unbound-looking row (NodeID == "", no Stop button, Spec §10) on any
// resolution error rather than failing the whole page.
func (s *Server) homeRunningNowVM(ctx context.Context, u domain.User, rs domain.WorkSession, now time.Time) *webui.RunningNowVM {
	nowVM := &webui.RunningNowVM{
		BaseSeconds: int64(rs.Elapsed(now) / time.Second),
		SinceEpoch:  now.Unix() - int64(rs.Elapsed(now)/time.Second),
		SinceStr:    rs.Start.Format("15:04"),
	}
	if rs.NodeID == nil || s.GetNode.Nodes == nil {
		return nowVM
	}
	n, err := s.GetNode.Execute(ctx, u.ID, *rs.NodeID)
	if err != nil {
		slog.WarnContext(ctx, "home: running session node lookup failed", "err", err)
		return nowVM
	}
	nowVM.NodeID = n.ID
	nowVM.NodeName = webui.ShortName(n.Name)
	nowVM.NodeHref = "/nodes/" + n.ID
	nowVM.Initials = webui.Initials(n.Name)
	nowVM.Tone = webui.AvatarTone(n.Name)
	if s.NodeAncestors.Nodes != nil {
		if chain, aerr := s.NodeAncestors.Execute(ctx, u.ID, n.ID); aerr == nil {
			full := chain
			if len(full) == 0 || full[0].ID != n.ID {
				full = append([]domain.Node{n}, full...)
			}
			nowVM.CountsWork = domain.ResolveCountsTowardTarget(full)
		}
	}
	return nowVM
}
