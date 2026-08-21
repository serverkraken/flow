package httpserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// homeStart füllt die Start-Blöcke (Screen 24) aus dem, was der Schreibtisch
// ohnehin lädt (Register, Buchungen der letzten 30 Tage, Karten), plus je
// Engagement den Monat und den Kontext. Alles owner-scoped, alles
// abgesichert — ein fehlender Store lässt den Block leer, nicht die Seite.
func (s *Server) homeStart(ctx context.Context, u domain.User, now time.Time, nodes []domain.Node, sessions []domain.WorkSession, docs []domain.Document, vm *webui.HomeVM) {
	// Angepinnt.
	for _, d := range docs {
		if d.Pinned && !d.Archived {
			vm.Pinned = append(vm.Pinned, webui.WissenRowFromDocument(d, now))
			if len(vm.Pinned) == homeWissenCap {
				break
			}
		}
	}
	vm.Yesterday = webui.FindYesterdayNote(ctx, docs, now)

	archived := 0
	if s.ListArchived.Docs != nil {
		if arch, err := s.ListArchived.Execute(ctx, u.ID); err == nil {
			archived = len(arch)
		}
	}
	vm.Bestand = webui.BuildBestand(nodes, docs, archived)

	// Engagements: Wurzel jedes Teilbaums — für Erträge, Stille und Kontext.
	parent := make(map[string]string, len(nodes))
	var engagements []domain.Node
	for _, n := range nodes {
		if n.ParentID != nil {
			parent[n.ID] = *n.ParentID
		}
		if n.Kind == domain.KindEngagement && n.Status != domain.NodeArchived {
			engagements = append(engagements, n)
		}
	}
	root := func(id string) string {
		for i := 0; i < 32; i++ {
			p, ok := parent[id]
			if !ok {
				return id
			}
			id = p
		}
		return id
	}
	last := make(map[string]time.Time, len(engagements))
	for _, ws := range sessions {
		if ws.NodeID == nil {
			continue
		}
		r := root(*ws.NodeID)
		if ws.Start.After(last[r]) {
			last[r] = ws.Start
		}
	}

	dropped := make(map[string]int, len(engagements))
	var months []webui.EngagementMonth
	for _, e := range engagements {
		if s.ComposeContext.Nodes != nil {
			budget := s.ContextBudget
			if budget <= 0 {
				budget = 12000
			}
			if cc, err := s.ComposeContext.ExecuteForNode(ctx, u.ID, e.ID, budget); err == nil {
				d := cc.Budget.Dropped
				dropped[e.ID] = d.Leaf + d.Vorhaben + d.Engagement + d.Global
			} else {
				slog.WarnContext(ctx, "home: compose context failed", "nodeID", e.ID, "err", err)
			}
		}
		em := webui.EngagementMonth{Node: e, Rate: domain.ResolveRate([]domain.Node{e})}
		if s.Stats.Nodes != nil && s.Stats.Sessions != nil {
			if roll, err := s.Stats.NodeStats(ctx, u.ID, e.ID); err == nil {
				em.Month = roll.Month
			} else {
				slog.WarnContext(ctx, "home: node stats failed", "nodeID", e.ID, "err", err)
			}
		}
		months = append(months, em)
	}
	vm.Attention = webui.BuildAttention(ctx, webui.AttentionInput{Engagements: engagements, ContextDropped: dropped, LastBooking: last, Docs: docs, Now: now})
	vm.Ertraege = webui.BuildErtraege(ctx, webui.MonthText(ctx, now.Month()), months)
}
