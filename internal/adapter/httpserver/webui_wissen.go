package httpserver

import (
	"context"
	"fmt"
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
	activeDocs, archivedDocs, err := s.wissenDocuments(r.Context(), u.ID, base.NodeParam, base.Scope, active)
	if err != nil {
		return webui.WissenOverviewVM{}, err
	}
	base.ActiveCount = len(activeDocs)
	base.ArchivedCount = len(archivedDocs)
	base = withWissenStatusHrefs(base, "")

	if q != "" {
		if base.Status != "archived" {
			hits, err := s.wissenSearch(r.Context(), u.ID, q, base.NodeParam, base.Scope, active)
			if err != nil {
				return webui.WissenOverviewVM{}, err
			}
			for _, h := range hits {
				base.Results = append(base.Results, wissenSearchRow(h))
			}
		}
		if base.Status != "active" {
			for _, d := range archivedDocs {
				if wissenDocumentMatches(d, q) {
					base.Results = append(base.Results, wissenArchivedSearchRow(d, q))
				}
			}
		}
		return webui.WissenOverviewVM{WissenVM: base, TotalCount: len(activeDocs) + len(archivedDocs)}, nil
	}

	docs := wissenDocumentsForStatus(base.Status, activeDocs, archivedDocs)
	recentAll := r.URL.Query().Get("recent") == "all"
	vm := webui.BuildWissenOverview(docs, s.Clock.Now(), recentAll)
	vm.WissenVM = base
	vm.TotalCount = len(activeDocs) + len(archivedDocs)
	for i := range vm.Shelves {
		vm.Shelves[i].Href = "/wissen/typ" + wissenQueryStringFull(vm.Shelves[i].TypeKey, active, "", base.Status, base.NodeParam, "scope", base.Scope)
	}
	vm.RecentAllHref = "/wissen" + wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope, "recent", "all")
	vm.RecentAllFragmentHref = "/ui/wissen/list" + wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope, "recent", "all")
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

	activeDocs, archivedDocs, err := s.wissenDocuments(r.Context(), u.ID, base.NodeParam, base.Scope, active)
	if err != nil {
		return webui.WissenTypeVM{}, err
	}
	activeDocs = filterWissenShelfDocs(activeDocs, shelf)
	archivedDocs = filterWissenShelfDocs(archivedDocs, shelf)
	base.ActiveCount = len(activeDocs)
	base.ArchivedCount = len(archivedDocs)
	base = withWissenStatusHrefs(base, shelf.TypeKey)
	overviewHref := "/wissen" + wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope)

	if q != "" {
		var hits []domain.SearchHit
		if base.Status != "archived" {
			hits, err = s.wissenSearch(r.Context(), u.ID, q, base.NodeParam, base.Scope, active)
			if err != nil {
				return webui.WissenTypeVM{}, err
			}
		}
		vm := webui.WissenTypeVM{WissenVM: base, Shelf: shelf, OverviewHref: overviewHref}
		for _, h := range hits {
			if !webui.DocumentInShelf(h.Document, shelf) {
				continue
			}
			vm.Results = append(vm.Results, wissenSearchRow(h))
		}
		if base.Status != "active" {
			for _, d := range archivedDocs {
				if webui.DocumentInShelf(d, shelf) && wissenDocumentMatches(d, q) {
					vm.Results = append(vm.Results, wissenArchivedSearchRow(d, q))
				}
			}
		}
		vm.Total = len(vm.Results)
		return vm, nil
	}

	filtered := wissenDocumentsForStatus(base.Status, activeDocs, archivedDocs)
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	offset := (page - 1) * wissenPageSize
	pageDocs := paginateDocuments(filtered, wissenPageSize, offset)

	vm := webui.BuildWissenType(shelf, pageDocs, s.Clock.Now())
	vm.WissenVM = base
	vm.Total = len(filtered)
	vm.OverviewHref = overviewHref
	vm.Page = components.PageNav{
		Page:     page,
		Total:    len(filtered),
		PageSize: wissenPageSize,
		BaseHref: basePath + wissenQueryStringFull(shelf.TypeKey, active, q, base.Status, base.NodeParam, "scope", base.Scope),
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

func wissenArchivedSearchRow(d domain.Document, q string) webui.SearchRow {
	return webui.SearchRow{
		ID: d.ID, Title: d.Title, Path: d.Path, Archived: true,
		ChipClass: webui.DocTypeChipClass(d.Type), ChipLabel: webui.DocTypeLabel(d.Type),
		Snippet: renderSnippet(wissenArchiveSnippet(d, q)),
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
	status := wissenStatus(r.URL.Query().Get("status"))
	nodeParam := strings.TrimSpace(r.URL.Query().Get("node"))
	scope := wissenScope(r.URL.Query().Get("scope"), nodeParam)

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
			Href:   basePath + wissenQueryStringFull(typeKey, toggledTags(active, tc.Tag), "", status, nodeParam, "scope", scope),
		})
	}
	var nodeOptions []webui.WissenNodeOption
	if s.ListNodes.Nodes != nil {
		nodes, nerr := s.ListNodes.Execute(r.Context(), u.ID)
		if nerr != nil {
			return webui.WissenVM{}, nil, "", nerr
		}
		for _, n := range nodes {
			nodeOptions = append(nodeOptions, webui.WissenNodeOption{ID: n.ID, Name: n.Name})
		}
	}
	return webui.WissenVM{
		User:         u.Username,
		AllTags:      chips,
		ActiveTags:   active,
		SearchQ:      q,
		Query:        wissenQueryStringFull(typeKey, active, q, status, nodeParam, "scope", scope),
		SearchAction: basePath,
		ResetHref:    basePath + wissenQueryStringFull(typeKey, nil, "", status, nodeParam, "scope", scope),
		TypeParam:    typeKey,
		Status:       status,
		NodeParam:    nodeParam,
		Scope:        scope,
		NodeOptions:  nodeOptions,
		FilterAction: basePath,
	}, active, q, nil
}

func (s *Server) wissenDocuments(ctx context.Context, ownerID, nodeParam, scope string, tags []string) ([]domain.Document, []domain.Document, error) {
	if scope == "subtree" {
		allowed, err := s.wissenSubtreeIDs(ctx, ownerID, nodeParam)
		if err != nil {
			return nil, nil, err
		}
		active, err := s.ListDocuments.Execute(ctx, ownerID, nil, tags)
		if err != nil {
			return nil, nil, err
		}
		active = filterWissenDocumentsByNodeIDs(active, allowed)
		if s.ListArchived.Docs == nil {
			return active, nil, nil
		}
		archived, err := s.ListArchived.Execute(ctx, ownerID)
		if err != nil {
			return nil, nil, err
		}
		archived = filterWissenDocumentsByNodeIDs(archived, allowed)
		return active, filterWissenDocuments(archived, "", tags), nil
	}
	active, err := s.ListDocuments.Execute(ctx, ownerID, wissenNodeFilter(nodeParam), tags)
	if err != nil {
		return nil, nil, err
	}
	if s.ListArchived.Docs == nil {
		return active, nil, nil
	}
	archived, err := s.ListArchived.Execute(ctx, ownerID)
	if err != nil {
		return nil, nil, err
	}
	return active, filterWissenDocuments(archived, nodeParam, tags), nil
}

func (s *Server) wissenSearch(ctx context.Context, ownerID, q, nodeParam, scope string, tags []string) ([]domain.SearchHit, error) {
	if scope != "subtree" {
		return s.SearchDocuments.Execute(ctx, ownerID, q, wissenNodeFilter(nodeParam), tags)
	}
	allowed, err := s.wissenSubtreeIDs(ctx, ownerID, nodeParam)
	if err != nil {
		return nil, err
	}
	hits, err := s.SearchDocuments.Execute(ctx, ownerID, q, nil, tags)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SearchHit, 0, len(hits))
	for _, hit := range hits {
		if hit.NodeID != nil && allowed[*hit.NodeID] {
			out = append(out, hit)
		}
	}
	return out, nil
}

func (s *Server) wissenSubtreeIDs(ctx context.Context, ownerID, nodeID string) (map[string]bool, error) {
	if s.ListNodes.Nodes == nil {
		return nil, fmt.Errorf("wissen subtree: node store unavailable")
	}
	nodes, err := s.ListNodes.Nodes.Subtree(ctx, ownerID, nodeID)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		ids[node.ID] = true
	}
	return ids, nil
}

func withWissenStatusHrefs(vm webui.WissenVM, typeKey string) webui.WissenVM {
	vm.StatusActiveHref = vm.FilterAction + wissenQueryStringFull(typeKey, vm.ActiveTags, vm.SearchQ, "active", vm.NodeParam, "scope", vm.Scope)
	vm.StatusArchivedHref = vm.FilterAction + wissenQueryStringFull(typeKey, vm.ActiveTags, vm.SearchQ, "archived", vm.NodeParam, "scope", vm.Scope)
	vm.StatusAllHref = vm.FilterAction + wissenQueryStringFull(typeKey, vm.ActiveTags, vm.SearchQ, "all", vm.NodeParam, "scope", vm.Scope)
	return vm
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
	return wissenQueryStringFull(typeKey, tags, q, "active", "")
}

func wissenQueryStringFull(typeKey string, tags []string, q, status, node string, extra ...string) string {
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
	if status != "" && status != "active" {
		v.Set("status", status)
	}
	if node != "" {
		v.Set("node", node)
	}
	for i := 0; i+1 < len(extra); i += 2 {
		if extra[i] != "" && extra[i+1] != "" {
			v.Set(extra[i], extra[i+1])
		}
	}
	enc := v.Encode()
	if enc == "" {
		return ""
	}
	return "?" + enc
}

func wissenStatus(v string) string {
	switch v {
	case "archived", "all":
		return v
	default:
		return "active"
	}
}

func wissenScope(v, node string) string {
	if v == "subtree" && node != "" && node != "none" {
		return "subtree"
	}
	return ""
}

func wissenNodeFilter(node string) *string {
	if node == "" {
		return nil
	}
	return &node
}

func wissenDocumentsForStatus(status string, active, archived []domain.Document) []domain.Document {
	switch status {
	case "archived":
		return archived
	case "all":
		return webui.SortedDocuments(append(append([]domain.Document(nil), active...), archived...))
	default:
		return active
	}
}

func filterWissenDocuments(docs []domain.Document, node string, tags []string) []domain.Document {
	wantTags := domain.NormalizeTags(tags)
	out := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if node == "none" && d.NodeID != nil {
			continue
		}
		if node != "" && node != "none" && (d.NodeID == nil || *d.NodeID != node) {
			continue
		}
		have := make(map[string]bool, len(d.Tags))
		for _, tag := range domain.NormalizeTags(d.Tags) {
			have[tag] = true
		}
		matches := true
		for _, tag := range wantTags {
			if !have[tag] {
				matches = false
				break
			}
		}
		if matches {
			out = append(out, d)
		}
	}
	return out
}

func filterWissenDocumentsByNodeIDs(docs []domain.Document, allowed map[string]bool) []domain.Document {
	out := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if d.NodeID != nil && allowed[*d.NodeID] {
			out = append(out, d)
		}
	}
	return out
}

func wissenDocumentMatches(d domain.Document, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	return q == "" || strings.Contains(strings.ToLower(d.Title+"\n"+d.Path+"\n"+d.Body), q)
}

func wissenArchiveSnippet(d domain.Document, q string) string {
	source := strings.TrimSpace(d.Body)
	if source == "" {
		source = d.Path
	}
	const max = 180
	runes := []rune(source)
	if len(runes) > max {
		source = string(runes[:max]) + "…"
	}
	lower, needle := strings.ToLower(source), strings.ToLower(strings.TrimSpace(q))
	if i := strings.Index(lower, needle); needle != "" && i >= 0 && i+len(needle) <= len(source) {
		return source[:i] + domain.HighlightStart + source[i:i+len(needle)] + domain.HighlightEnd + source[i+len(needle):]
	}
	return source
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
