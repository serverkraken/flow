package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func (s *Server) handleWebDocumentView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	doc, err := s.GetDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	all, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, nil)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	resolve := func(target string) (string, string, bool) {
		if t, ok := domain.ResolveWikilink(doc, target, all); ok {
			return "/wissen/" + t.ID, t.Title, true
		}
		return "", "", false
	}
	rendered := webui.RenderDocument(doc.Body, resolve)

	refs, err := s.BacklinksDocument.Execute(r.Context(), u.ID, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	projectNames, projectColors, err := s.projectNameColorMaps(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	categoryHref, categoryLabelKey := wissenCategoryHrefAndLabel(doc)

	kind := webui.DocKindStyle(doc.Type)
	vm := webui.DocumentVM{
		User:             u.Username,
		ID:               doc.ID,
		Type:             string(doc.Type),
		KindLabel:        kind.Label,
		KindGlyph:        kind.Glyph,
		KindTone:         kind.Tone,
		Title:            doc.Title,
		HTML:             rendered,
		CategoryHref:     categoryHref,
		CategoryLabelKey: categoryLabelKey,
	}
	if doc.NodeID != nil {
		vm.NodeID = *doc.NodeID
		vm.NodeName = projectNames[*doc.NodeID]
		vm.ProjectColor = webui.ColorHex(projectColors[*doc.NodeID])
	}
	if doc.Date != nil {
		vm.DateStr = doc.Date.Format("02.01.2006")
	}
	for _, t := range doc.Tags {
		vm.Tags = append(vm.Tags, webui.TagLink{Tag: t, Href: webui.WissenSingleTagHref(t)})
	}
	for _, ref := range refs {
		vm.Backlinks = append(vm.Backlinks, components.Backlink{
			ID: ref.ID, Type: string(ref.Type), Path: ref.Path, Title: ref.Title,
		})
	}
	if s.GetEmbedStatus.Docs != nil {
		if st, serr := s.GetEmbedStatus.Execute(r.Context(), u.ID, id); serr == nil {
			vm.Embed = &webui.EmbedView{
				State:     string(st.State),
				LastError: truncateError(st.LastError),
				ShowRetry: st.State == domain.EmbedFailed,
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocumentPage(vm).Render(r.Context(), w)
}

func wissenCategoryHrefAndLabel(doc domain.Document) (string, string) {
	if cat, ok := webui.WissenCategoryForType(doc.Type); ok {
		return cat.Href, cat.LabelKey
	}
	return "/wissen", "wissen.title"
}

func (s *Server) handleWebDocReembed(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if err := s.RetryEmbedding.Execute(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, ports.ErrDocumentNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("HX-Request") == "" {
		http.Redirect(w, r, "/wissen/"+id, http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocumentEmbedBadge(id, webui.EmbedView{State: "pending"}).Render(r.Context(), w)
}

func truncateError(s string) string {
	const max = 80
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
