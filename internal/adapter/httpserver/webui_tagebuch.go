package httpserver

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// dailyDocsFor returns every non-archived DocDaily document for the owner —
// the shared source for Screens 04 and 14 (ListDocuments already excludes
// archived rows; daily notes carry no NodeID so nodeID=nil lists all of them).
func (s *Server) dailyDocsFor(ctx context.Context, ownerID string) ([]domain.Document, error) {
	all, err := s.ListDocuments.Execute(ctx, ownerID, nil, nil)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Document, 0, len(all))
	for _, d := range all {
		if d.Type == domain.DocDaily {
			out = append(out, d)
		}
	}
	return out, nil
}

// handleWebTagebuch serves GET /tagebuch: Screen 04, "heutige Tagesnotiz".
func (s *Server) handleWebTagebuch(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	docs, err := s.dailyDocsFor(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	vm := webui.BuildTagebuchVM(r.Context(), docs, r.URL.Query().Get("selected"), s.Clock.Now())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.TagebuchPage(vm).Render(r.Context(), w)
}

// handleWebTagebuchToday serves GET /tagebuch/heute: opens today's note,
// creating it first if this is the first visit today ("Heute schreiben").
func (s *Server) handleWebTagebuchToday(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	now := s.Clock.Now()
	path := domain.DailyPath(now)
	docs, err := s.dailyDocsFor(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	for _, d := range docs {
		if d.Path == path {
			http.Redirect(w, r, "/tagebuch?selected="+d.ID, http.StatusSeeOther)
			return
		}
	}
	doc, err := s.CreateDocument.Execute(r.Context(), u.ID, usecase.CreateDocumentInput{
		Type: domain.DocDaily, Title: now.Format("2006-01-02"),
	})
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tagebuch?selected="+doc.ID, http.StatusSeeOther)
}

// handleWebTagebuchArchiv serves GET /tagebuch/archiv: Screen 14.
func (s *Server) handleWebTagebuchArchiv(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	docs, err := s.dailyDocsFor(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	month, _ := strconv.Atoi(r.URL.Query().Get("month"))
	vm := webui.BuildTagebuchArchivVM(r.Context(), docs, year, month, s.Clock.Now())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.TagebuchArchivPage(vm).Render(r.Context(), w)
}

// assignableTagebuchTargets returns the Vorhaben/Engagement nodes a highlight
// can be assigned to (Screen 27 excludes Repo — a highlight targets a piece
// of work, not a code checkout), sorted by name.
func assignableTagebuchTargets(all []domain.Node) []domain.Node {
	out := make([]domain.Node, 0, len(all))
	for _, n := range all {
		if n.Kind == domain.KindVorhaben || n.Kind == domain.KindEngagement {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// handleWebTagebuchMarkieren serves GET /tagebuch/{id}/markieren: Screen 27.
func (s *Server) handleWebTagebuchMarkieren(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	docID := r.PathValue("id")
	doc, err := s.GetDocument.Execute(r.Context(), u.ID, docID)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	highlights, err := s.ListDocumentHighlights.Execute(r.Context(), u.ID, docID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	allNodes, err := s.ListNodes.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	nodesByID := make(map[string]domain.Node, len(allNodes))
	for _, n := range allNodes {
		nodesByID[n.ID] = n
	}
	now := s.Clock.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthHighlights, err := s.ListRecentHighlights.Execute(r.Context(), u.ID, monthStart)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	vm := webui.BuildTagebuchMarkierenVM(r.Context(), doc, highlights, nodesByID, assignableTagebuchTargets(allNodes), monthHighlights)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.TagebuchMarkierenPage(vm).Render(r.Context(), w)
}

// handleWebTagebuchHighlightCreate serves POST /tagebuch/{id}/highlights
// (form: quote, nodeId) — the "Zuordnen" submit on Screen 27.
func (s *Server) handleWebTagebuchHighlightCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	docID := r.PathValue("id")
	_ = r.ParseForm()
	if _, err := s.AssignHighlight.Execute(r.Context(), u.ID, docID, r.FormValue("nodeId"), r.FormValue("quote")); err != nil {
		http.Redirect(w, r, "/tagebuch/"+docID+"/markieren?err=assign", http.StatusSeeOther)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventHighlightChanged, UserID: u.ID, Data: map[string]any{"id": docID}})
	http.Redirect(w, r, "/tagebuch/"+docID+"/markieren", http.StatusSeeOther)
}

// handleWebTagebuchHighlightDelete serves POST /tagebuch/{id}/highlights/{hid}/delete.
func (s *Server) handleWebTagebuchHighlightDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	docID := r.PathValue("id")
	if err := s.RemoveHighlight.Execute(r.Context(), u.ID, r.PathValue("hid")); err != nil {
		http.Redirect(w, r, "/tagebuch/"+docID+"/markieren?err=remove", http.StatusSeeOther)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventHighlightChanged, UserID: u.ID, Data: map[string]any{"id": docID}})
	http.Redirect(w, r, "/tagebuch/"+docID+"/markieren", http.StatusSeeOther)
}
