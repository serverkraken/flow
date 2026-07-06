package httpserver

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

const wissenPageSize = 50

// wissenOverviewData builds the /wissen library page's data: search-hit rows
// when a query is present, otherwise the type-shelf counts + capped "Zuletzt
// aktualisiert" list built from the (tag-filtered) owner document set —
// owner-scoped throughout via s.ListDocuments/s.SearchDocuments (both take
// u.ID).
func (s *Server) wissenOverviewData(r *http.Request, u domain.User) (webui.WissenOverviewVM, error) {
	base, active, q, err := s.wissenBaseVM(r, u, "/wissen", "")
	if err != nil {
		return webui.WissenOverviewVM{}, err
	}

	if q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, nil, active)
		if err != nil {
			return webui.WissenOverviewVM{}, err
		}
		for _, h := range hits {
			base.Results = append(base.Results, wissenSearchRow(h))
		}
		return webui.WissenOverviewVM{WissenVM: base}, nil
	}

	docs, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, active)
	if err != nil {
		return webui.WissenOverviewVM{}, err
	}
	recentAll := r.URL.Query().Get("recent") == "all"
	vm := webui.BuildWissenOverview(docs, s.Clock.Now(), recentAll)
	vm.WissenVM = base
	return vm, nil
}

// wissenTypeData builds one /wissen/typ?type= shelf page's data: search hits
// scoped to the shelf's types when a query is present, otherwise a
// paginated, flat Lesesaal-row listing of the shelf's documents. The shelf
// route is a query param rather than a path segment — see WissenVM.TypeParam
// for why "/wissen/typ/{type}" can't coexist with the established
// "/wissen/{id}/bearbeiten" action route in Go's http.ServeMux.
func (s *Server) wissenTypeData(r *http.Request, u domain.User, shelf webui.WissenShelf) (webui.WissenTypeVM, error) {
	const basePath = "/wissen/typ"
	base, active, q, err := s.wissenBaseVM(r, u, basePath, shelf.TypeKey)
	if err != nil {
		return webui.WissenTypeVM{}, err
	}

	if q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, nil, active)
		if err != nil {
			return webui.WissenTypeVM{}, err
		}
		vm := webui.WissenTypeVM{WissenVM: base, Shelf: shelf}
		for _, h := range hits {
			if !webui.DocumentInShelf(h.Document, shelf) {
				continue
			}
			vm.Results = append(vm.Results, wissenSearchRow(h))
		}
		vm.Total = len(vm.Results)
		return vm, nil
	}

	docs, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, active)
	if err != nil {
		return webui.WissenTypeVM{}, err
	}
	filtered := filterWissenShelfDocs(docs, shelf)
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	offset := (page - 1) * wissenPageSize
	pageDocs := paginateDocuments(filtered, wissenPageSize, offset)

	vm := webui.BuildWissenType(shelf, pageDocs, s.Clock.Now())
	vm.WissenVM = base
	vm.Total = len(filtered)
	vm.Page = components.PageNav{
		Page:     page,
		Total:    len(filtered),
		PageSize: wissenPageSize,
		BaseHref: basePath + wissenQueryString(shelf.TypeKey, active, q),
	}
	return vm, nil
}

func wissenSearchRow(h domain.SearchHit) webui.SearchRow {
	return webui.SearchRow{
		ID:        h.ID,
		Title:     h.Title,
		Path:      h.Path,
		ChipClass: webui.DocTypeChipClass(h.Type),
		ChipLabel: webui.DocTypeLabel(h.Type),
		Snippet:   renderSnippet(h.Snippet),
	}
}

// wissenBaseVM builds the bigsearch/tag-chip machinery shared by the
// overview and type-shelf pages. typeKey is "" on the overview page and the
// shelf's TypeKey on /wissen/typ — threaded through every href/query built
// here (and via wissenBigsearch's hidden "type" input, since a GET form
// drops its action URL's existing query string on submit) so the shelf
// filter survives search/tag/pagination round-trips.
func (s *Server) wissenBaseVM(r *http.Request, u domain.User, basePath, typeKey string) (webui.WissenVM, []string, string, error) {
	active := r.URL.Query()["tag"]
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	docType := domain.TaggableDocument
	allTags, err := s.ListTags.Execute(r.Context(), u.ID, domain.TagScope{Type: &docType})
	if err != nil {
		return webui.WissenVM{}, nil, "", err
	}
	activeSet := map[string]bool{}
	for _, t := range active {
		activeSet[t] = true
	}
	chips := make([]webui.TagChip, 0, len(allTags))
	for _, tc := range allTags {
		chips = append(chips, webui.TagChip{
			Tag:    tc.Tag,
			Count:  tc.Count,
			Active: activeSet[tc.Tag],
			Href:   basePath + wissenQueryString(typeKey, toggledTags(active, tc.Tag), ""),
		})
	}
	return webui.WissenVM{
		User:         u.Username,
		AllTags:      chips,
		ActiveTags:   active,
		SearchQ:      q,
		Query:        wissenQueryString(typeKey, active, q),
		SearchAction: basePath,
		ResetHref:    basePath + wissenQueryString(typeKey, nil, ""),
		TypeParam:    typeKey,
	}, active, q, nil
}

func (s *Server) handleWebWissenHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.wissenOverviewData(r, u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebWissenList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.wissenOverviewData(r, u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenFragment(vm).Render(r.Context(), w)
}

func (s *Server) handleWebWissenType(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	shelf, ok := webui.WissenShelfFromTypeKey(r.URL.Query().Get("type"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	vm, err := s.wissenTypeData(r, u, shelf)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenTypePage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebWissenTypeList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	shelf, ok := webui.WissenShelfFromTypeKey(r.URL.Query().Get("type"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	vm, err := s.wissenTypeData(r, u, shelf)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenTypeFragment(vm).Render(r.Context(), w)
}

// handleWebWissenRedirect 302s a retired Wissen category slug to its
// type-shelf successor (Offene Entsch. #7 — no dead links). /wissen/system
// has no 1:1 target (Codex #17: its five legacy types now spread across
// plan/memory/context/spec) and redirects to the /wissen overview instead —
// the caller wires that asymmetry explicitly in server.go.
func (s *Server) handleWebWissenRedirect(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}
}

func filterWissenShelfDocs(docs []domain.Document, shelf webui.WissenShelf) []domain.Document {
	out := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if webui.DocumentInShelf(d, shelf) {
			out = append(out, d)
		}
	}
	return out
}

func paginateDocuments(docs []domain.Document, limit, offset int) []domain.Document {
	total := len(docs)
	if limit <= 0 {
		limit = total
	}
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return docs[offset:end]
}

// nodeMaps builds id→name, id→color, id→kind maps from ListNodes for use by
// document/cockpit/home view models.
func (s *Server) nodeMaps(ctx context.Context, ownerID string) (map[string]string, map[string]string, map[string]domain.NodeKind, error) {
	names := map[string]string{}
	colors := map[string]string{}
	kinds := map[string]domain.NodeKind{}
	if s.ListNodes.Nodes == nil {
		return names, colors, kinds, nil
	}
	nodes, err := s.ListNodes.Execute(ctx, ownerID)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, n := range nodes {
		names[n.ID] = n.Name
		colors[n.ID] = n.Color
		kinds[n.ID] = n.Kind
	}
	return names, colors, kinds, nil
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// wissenQueryString builds the query string shared by every Wissen href and
// the SSE fragment's hx-get: an optional fixed "type" (the /wissen/typ shelf
// filter, empty on the overview page), the active tag filters, and the
// free-text query.
func wissenQueryString(typeKey string, tags []string, q string) string {
	v := url.Values{}
	if typeKey != "" {
		v.Set("type", typeKey)
	}
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

// toggledTags returns active with tag removed if present, or appended if not
// — the "click a tag chip to add/remove it from the filter" behavior.
func toggledTags(active []string, tag string) []string {
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
	return next
}
