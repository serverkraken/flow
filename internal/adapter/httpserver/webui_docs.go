package httpserver

import (
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// renderSnippet escapes the snippet then replaces the highlight sentinels with
// <mark> tags — escape-first so no document content can inject HTML. Returns a
// safe HTML string (rendered raw in the template via @templ.Raw). Stray
// (unmatched) sentinels are removed before html-escaping so they can never
// become an unclosed <mark> that corrupts the page layout.
func renderSnippet(s string) string {
	// Belt-and-suspenders: strip stray sentinels before escaping so they can
	// never produce an unclosed <mark>. html.EscapeString passes \x02/\x03
	// through unchanged, so we handle them before they become HTML tokens.
	s = stripStraySentinels(s)
	esc := html.EscapeString(s)
	esc = strings.ReplaceAll(esc, domain.HighlightStart, "<mark>")
	esc = strings.ReplaceAll(esc, domain.HighlightEnd, "</mark>")
	return esc
}

// stripStraySentinels removes HighlightStart/HighlightEnd bytes that are not
// balanced (each Start must be followed by an End with no nested Start in
// between). Matched pairs are left intact so ts_headline output is honoured.
func stripStraySentinels(s string) string {
	starts := strings.Count(s, domain.HighlightStart)
	ends := strings.Count(s, domain.HighlightEnd)
	if starts == 0 && ends == 0 {
		return s
	}
	if starts != ends {
		// Unbalanced: strip all sentinels.
		return domain.StripHighlightSentinels(s)
	}
	// Equal counts: verify ordering — each Start must precede its matching End.
	// Walk through and remove any Start that isn't followed by an End before the
	// next Start, and vice versa.
	var out strings.Builder
	for {
		i := strings.Index(s, domain.HighlightStart)
		if i < 0 {
			out.WriteString(s)
			break
		}
		j := strings.Index(s[i:], domain.HighlightEnd)
		if j < 0 {
			// No End after this Start: strip all remaining sentinels.
			out.WriteString(strings.ReplaceAll(strings.ReplaceAll(s, domain.HighlightStart, ""), domain.HighlightEnd, ""))
			break
		}
		j += i // absolute index
		// Check there is no nested Start between i and j.
		between := s[i+len(domain.HighlightStart) : j]
		if strings.Contains(between, domain.HighlightStart) {
			// Nested/malformed: strip and continue.
			out.WriteString(s[:i])
			s = s[i+len(domain.HighlightStart):]
			continue
		}
		out.WriteString(s[:i])
		out.WriteString(domain.HighlightStart)
		out.WriteString(between)
		out.WriteString(domain.HighlightEnd)
		s = s[j+len(domain.HighlightEnd):]
	}
	return out.String()
}

// encodeListQuery encodes the active tags plus an optional q into a "?..." string
// (empty when nothing is set) for the SSE-refresh hx-get on the list container.
func encodeListQuery(tags []string, q string) string {
	v := url.Values{}
	for _, t := range tags {
		v.Add("tag", t)
	}
	if q != "" {
		v.Set("q", q)
	}
	enc := v.Encode()
	if enc == "" {
		return ""
	}
	return "?" + enc
}

func (s *Server) docsListData(r *http.Request, u domain.User) (webui.DocsPageData, error) {
	active := r.URL.Query()["tag"]
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	allTags, err := s.ListTags.Execute(r.Context(), u.ID)
	if err != nil {
		return webui.DocsPageData{}, err
	}
	activeSet := map[string]bool{}
	for _, t := range active {
		activeSet[t] = true
	}
	chips := make([]webui.TagChip, 0, len(allTags))
	for _, tc := range allTags {
		chips = append(chips, webui.TagChip{
			Tag: tc.Tag, Count: tc.Count, Active: activeSet[tc.Tag], Href: toggleTagHref(active, tc.Tag),
		})
	}
	data := webui.DocsPageData{
		User: u.Username, AllTags: chips, ActiveTags: active,
		SearchQ: q, Query: encodeListQuery(active, q),
	}
	if q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, nil, active)
		if err != nil {
			return webui.DocsPageData{}, err
		}
		results := make([]webui.SearchRow, 0, len(hits))
		for _, h := range hits {
			results = append(results, webui.SearchRow{
				DocRow:  webui.DocRow{ID: h.ID, Type: string(h.Type), Path: h.Path, Title: h.Title, Tags: h.Tags},
				Snippet: renderSnippet(h.Snippet),
			})
		}
		data.Results = results
		return data, nil
	}
	list, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, active)
	if err != nil {
		return webui.DocsPageData{}, err
	}
	rows := make([]webui.DocRow, 0, len(list))
	for _, d := range list {
		rows = append(rows, webui.DocRow{
			ID: d.ID, Type: string(d.Type), Path: d.Path, Title: d.Title, Tags: d.Tags,
		})
	}
	data.Docs = rows
	return data, nil
}

// toggleTagHref returns the /docs URL with tag added to (or removed from) the current filter set.
func toggleTagHref(active []string, tag string) string {
	var next []string
	removed := false
	for _, t := range active {
		if t == tag {
			removed = true
			continue
		}
		next = append(next, t)
	}
	if !removed {
		next = append(next, tag)
	}
	return "/docs" + encodeTagQuery(next)
}

// singleTagHref returns the /docs URL filtered to exactly one tag.
func singleTagHref(tag string) string {
	return "/docs" + encodeTagQuery([]string{tag})
}

func encodeTagQuery(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	q := url.Values{}
	for _, t := range tags {
		q.Add("tag", t)
	}
	return "?" + q.Encode()
}

func (s *Server) handleWebDocsHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.docsListData(r, u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocsPage(d).Render(r.Context(), w)
}

// handleWebDocsList renders just the list fragment (innerHTML swap for SSE).
func (s *Server) handleWebDocsList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.docsListData(r, u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocsFragment(d).Render(r.Context(), w)
}

func (s *Server) handleWebDocView(w http.ResponseWriter, r *http.Request) {
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
			return "/docs/" + t.ID, t.Title, true
		}
		return "", "", false
	}
	rendered := webui.RenderDocument(doc.Body, resolve)

	refs, err := s.BacklinksDocument.Execute(r.Context(), u.ID, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	blRows := make([]webui.DocRow, 0, len(refs))
	for _, ref := range refs {
		blRows = append(blRows, webui.DocRow{ID: ref.ID, Type: string(ref.Type), Path: ref.Path, Title: ref.Title})
	}

	tagLinks := make([]webui.TagLink, 0, len(doc.Tags))
	for _, t := range doc.Tags {
		tagLinks = append(tagLinks, webui.TagLink{Tag: t, Href: singleTagHref(t)})
	}
	d := webui.DocsPageData{
		User: u.Username,
		Current: &webui.DocDetail{
			ID: doc.ID, Type: string(doc.Type), Path: doc.Path, Title: doc.Title,
			HTML: rendered, Body: doc.Body, Backlinks: blRows, Tags: tagLinks,
		},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocView(d).Render(r.Context(), w)
}

func (s *Server) handleWebDocNew(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d := webui.DocsPageData{User: u.Username}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocForm(d, nil).Render(r.Context(), w)
}

func (s *Server) handleWebDocEdit(w http.ResponseWriter, r *http.Request) {
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
	d := webui.DocsPageData{User: u.Username}
	editing := &webui.DocDetail{
		ID: doc.ID, Type: string(doc.Type), Path: doc.Path, Title: doc.Title, Body: doc.Body,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocForm(d, editing).Render(r.Context(), w)
}

func (s *Server) handleWebDocCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()

	var projID *string
	if v := r.FormValue("projectId"); v != "" {
		projID = &v
	}

	// Capture submitted values for form re-population on error.
	submitted := &webui.DocFormValues{
		Type:      r.FormValue("type"),
		ProjectID: r.FormValue("projectId"),
		Path:      r.FormValue("path"),
		Title:     r.FormValue("title"),
		Body:      r.FormValue("body"),
	}

	doc, err := s.CreateDocument.Execute(r.Context(), u.ID, usecase.CreateDocumentInput{
		Type:      domain.DocumentType(submitted.Type),
		ProjectID: projID,
		Path:      submitted.Path,
		Title:     submitted.Title,
		Body:      submitted.Body,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidDocument):
		d := webui.DocsPageData{User: u.Username, Error: err.Error(), Form: submitted}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.DocForm(d, nil).Render(r.Context(), w)
	case errors.Is(err, ports.ErrDocumentExists):
		d := webui.DocsPageData{User: u.Username, Error: "a document with that path already exists", Form: submitted}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_ = webui.DocForm(d, nil).Render(r.Context(), w)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Bus.Publish(domain.Event{Type: domain.EventDocumentCreated, UserID: u.ID, Data: map[string]any{"id": doc.ID}})
		http.Redirect(w, r, "/docs/"+doc.ID, http.StatusSeeOther)
	}
}

func (s *Server) handleWebDocUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()

	_, err := s.UpdateDocument.Execute(r.Context(), u.ID, id, usecase.UpdateDocumentInput{
		Title: r.FormValue("title"),
		Body:  r.FormValue("body"),
	})
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/docs/"+id, http.StatusSeeOther)
}

func (s *Server) handleWebDocDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	err := s.DeleteDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventDocumentDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/docs", http.StatusSeeOther)
}
