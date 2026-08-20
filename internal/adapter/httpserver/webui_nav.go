package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// handleNavTreeFragment bedient GET /ui/nav/tree?active= — die beiden
// K3Rail-Sektionen: der Registerbaum mit Teilbaum-Kartenzählern und laufender
// Uhr, darunter die ALLGEMEIN-Zeilen mit ihren Werten. Die Schiene lädt das
// Fragment nach, weil components.AppShell das webui-Paket nicht importieren
// darf (Zyklus) — dieselbe Trennung wie beim Timer-Widget.
func (s *Server) handleNavTreeFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm := webui.RailNavVM{Active: r.URL.Query().Get("active")}

	var visible []domain.Node
	if s.ListNodes.Nodes != nil {
		all, err := s.ListNodes.Execute(r.Context(), u.ID)
		if err != nil {
			slog.WarnContext(r.Context(), "rail nav: list nodes failed", "err", err)
		}
		for _, n := range all {
			if n.Status == domain.NodeActive || n.Status == domain.NodePaused {
				visible = append(visible, n)
			}
		}
	}

	var cards map[string]int
	if s.ListDocuments.Docs != nil {
		docs, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, nil)
		if err != nil {
			slog.WarnContext(r.Context(), "rail nav: list documents failed", "err", err)
		}
		cards = webui.SubtreeDocTotals(visible, docs)
		vm.BibCount = len(docs)
	}

	runningNode, runningDur := "", ""
	if s.GetRunningSession.Sessions != nil {
		if rs, ok, err := s.GetRunningSession.Execute(r.Context(), u.ID); err == nil && ok && rs.NodeID != nil {
			runningNode = *rs.NodeID
			runningDur = webui.FmtDurHMExport(rs.Elapsed(s.Clock.Now()))
		}
	}

	// Wer Kinder hat, bekommt einen Pfeil. TreeRow trägt das nicht, und die
	// geteilte Struktur dafür zu erweitern wäre teurer als die eine Zeile hier.
	hasKids := make(map[string]bool, len(visible))
	for _, n := range visible {
		if n.ParentID != nil {
			hasKids[*n.ParentID] = true
		}
	}

	for _, row := range webui.BuildTree(visible) {
		nr := webui.RailNavRow{
			ID: row.Node.ID, Name: row.Node.Name, ParentID: navParentID(row.Node),
			Kind: row.Node.Kind, Level: row.Level,
			Cards: cards[row.Node.ID], HasChildren: hasKids[row.Node.ID],
		}
		if row.Node.ID == runningNode {
			nr.RunningDur = runningDur
		}
		vm.Rows = append(vm.Rows, nr)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.RailNav(vm).Render(r.Context(), w)
}

func navParentID(n domain.Node) string {
	if n.ParentID == nil {
		return ""
	}
	return *n.ParentID
}

// handleRailMonograms bedient GET /ui/rail/monograms — die Kacheln des
// 76px-Streifens im Tablet-Band. Der Streifen ist ab 1200px display:none,
// der Desktop fragt das Fragment also nie an (htmx intersect once).
func (s *Server) handleRailMonograms(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	all, err := s.ListNodes.Execute(r.Context(), u.ID)
	if err != nil {
		slog.WarnContext(r.Context(), "rail monograms: list nodes failed", "err", err)
		all = nil
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.RailMonograms(webui.RailMonogramNodes(all)).Render(r.Context(), w)
}
