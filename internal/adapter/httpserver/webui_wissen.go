package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

const (
	wissenPageSize            = 50
	wissenOverviewRecentLimit = 8
)

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
		pageNumber := atoiDefault(r.URL.Query().Get("page"), 1)
		query, err := s.wissenLibraryQuery(r.Context(), u.ID, base, nil, active, wissenPageSize, (pageNumber-1)*wissenPageSize)
		if err != nil {
			return webui.WissenOverviewVM{}, err
		}
		library, err := s.SearchDocumentLibrary.Execute(r.Context(), u.ID, q, query)
		if err != nil {
			return webui.WissenOverviewVM{}, err
		}
		base.ActiveCount = library.ActiveTotal
		base.ArchivedCount = library.ArchivedTotal
		base = withWissenStatusHrefs(base, "")
		base = withWissenTagTotals(base, library.TagTotals, "/wissen", "")
		for _, hit := range library.Results {
			base.Results = append(base.Results, wissenSearchRow(hit))
		}
		base.Page = components.PageNav{
			Page: pageNumber, Total: library.Total, PageSize: wissenPageSize,
			BaseHref: "/wissen" + wissenQueryStringFull("", active, q, base.Status, base.NodeParam, "scope", base.Scope),
		}
		base.Query = wissenQueryStringFull("", active, q, base.Status, base.NodeParam, "scope", base.Scope, "page", strconv.Itoa(pageNumber))
		return webui.WissenOverviewVM{WissenVM: base, TotalCount: library.ActiveTotal + library.ArchivedTotal}, nil
	}
	mode := webui.NormalizeDocSort(r.URL.Query().Get("sort"))
	base.LibrarySort = webui.LibrarySort(mode)
	if s.ListDocumentLibrary.Docs != nil {
		recentAll := r.URL.Query().Get("recent") == "all"
		pageNumber, limit := 1, wissenOverviewRecentLimit
		if recentAll {
			pageNumber = atoiDefault(r.URL.Query().Get("page"), 1)
			limit = wissenPageSize
		}
		query, err := s.wissenLibraryQuery(r.Context(), u.ID, base, nil, active, limit, (pageNumber-1)*limit)
		if err != nil {
			return webui.WissenOverviewVM{}, err
		}
		library, err := s.ListDocumentLibrary.Execute(r.Context(), u.ID, query)
		if err != nil {
			return webui.WissenOverviewVM{}, err
		}
		base.ActiveCount = library.ActiveTotal
		base.ArchivedCount = library.ArchivedTotal
		base = withWissenStatusHrefs(base, "")
		base = withWissenTagTotals(base, library.TagTotals, "/wissen", "")
		if recentAll {
			base.Query = wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope, "recent", "all", "page", strconv.Itoa(pageNumber))
		}
		vm := webui.BuildWissenOverviewPage(library.Documents, library.Total, library.TypeTotals, s.Clock.Now(), recentAll)
		// Auf diesem Pfad sortiert die ABFRAGE, nicht der Speicher — der
		// Schalter darf also erscheinen, weil er die ganze Liste ordnet und
		// nicht nur die sichtbare Seite.
		vm.Sort = mode
		vm.SortSupported = true
		vm.SortHead = webui.BuildSortHead(mode, "/wissen"+wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope))
		vm.WissenVM = base
		vm.TotalCount = library.ActiveTotal + library.ArchivedTotal
		for i := range vm.Shelves {
			vm.Shelves[i].Href = "/wissen/typ" + wissenQueryStringFull(vm.Shelves[i].TypeKey, active, "", base.Status, base.NodeParam, "scope", base.Scope)
		}
		vm.RecentAllHref = "/wissen" + wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope, "recent", "all")
		vm.RecentAllFragmentHref = "/ui/wissen/list" + wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope, "recent", "all")
		if recentAll {
			vm.Page = components.PageNav{
				Page: pageNumber, Total: library.Total, PageSize: wissenPageSize,
				BaseHref: "/wissen" + wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope, "recent", "all"),
			}
		}
		return vm, nil
	}

	activeDocs, archivedDocs, err := s.wissenDocuments(r.Context(), u.ID, base.NodeParam, base.Scope, active)
	if err != nil {
		return webui.WissenOverviewVM{}, err
	}
	base.ActiveCount = len(activeDocs)
	base.ArchivedCount = len(archivedDocs)
	base = withWissenStatusHrefs(base, "")
	docs := wissenDocumentsForStatus(base.Status, activeDocs, archivedDocs)
	recentAll := r.URL.Query().Get("recent") == "all"
	vm := webui.BuildWissenOverviewSorted(docs, s.Clock.Now(), recentAll, mode)
	vm.SortHead = webui.BuildSortHead(mode, "/wissen"+wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope))
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
	overviewHref := "/wissen" + wissenQueryStringFull("", active, "", base.Status, base.NodeParam, "scope", base.Scope)
	if q != "" {
		pageNumber := atoiDefault(r.URL.Query().Get("page"), 1)
		query, err := s.wissenLibraryQuery(r.Context(), u.ID, base, shelf.Types, active, wissenPageSize, (pageNumber-1)*wissenPageSize)
		if err != nil {
			return webui.WissenTypeVM{}, err
		}
		library, err := s.SearchDocumentLibrary.Execute(r.Context(), u.ID, q, query)
		if err != nil {
			return webui.WissenTypeVM{}, err
		}
		base.ActiveCount = library.ActiveTotal
		base.ArchivedCount = library.ArchivedTotal
		base = withWissenStatusHrefs(base, shelf.TypeKey)
		base = withWissenTagTotals(base, library.TagTotals, basePath, shelf.TypeKey)
		for _, hit := range library.Results {
			base.Results = append(base.Results, wissenSearchRow(hit))
		}
		base.Page = components.PageNav{
			Page: pageNumber, Total: library.Total, PageSize: wissenPageSize,
			BaseHref: basePath + wissenQueryStringFull(shelf.TypeKey, active, q, base.Status, base.NodeParam, "scope", base.Scope),
		}
		base.Query = wissenQueryStringFull(shelf.TypeKey, active, q, base.Status, base.NodeParam, "scope", base.Scope, "page", strconv.Itoa(pageNumber))
		return webui.WissenTypeVM{WissenVM: base, Shelf: shelf, Total: library.Total, OverviewHref: overviewHref}, nil
	}

	if q == "" && s.ListDocumentLibrary.Docs != nil {
		pageNumber := atoiDefault(r.URL.Query().Get("page"), 1)
		query, err := s.wissenLibraryQuery(r.Context(), u.ID, base, shelf.Types, active, wissenPageSize, (pageNumber-1)*wissenPageSize)
		if err != nil {
			return webui.WissenTypeVM{}, err
		}
		library, err := s.ListDocumentLibrary.Execute(r.Context(), u.ID, query)
		if err != nil {
			return webui.WissenTypeVM{}, err
		}
		base.ActiveCount = library.ActiveTotal
		base.ArchivedCount = library.ArchivedTotal
		base = withWissenStatusHrefs(base, shelf.TypeKey)
		base = withWissenTagTotals(base, library.TagTotals, basePath, shelf.TypeKey)
		vm := webui.BuildWissenType(shelf, library.Documents, s.Clock.Now())
		vm.WissenVM = base
		vm.Total = library.Total
		vm.OverviewHref = overviewHref
		vm.Page = components.PageNav{
			Page:     pageNumber,
			Total:    library.Total,
			PageSize: wissenPageSize,
			BaseHref: basePath + wissenQueryStringFull(shelf.TypeKey, active, q, base.Status, base.NodeParam, "scope", base.Scope),
		}
		return vm, nil
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

func (s *Server) wissenLibraryQuery(ctx context.Context, ownerID string, vm webui.WissenVM, types []domain.DocumentType, tags []string, limit, offset int) (ports.DocumentLibraryQuery, error) {
	query := ports.DocumentLibraryQuery{
		Types:  types,
		Tags:   tags,
		Sort:   vm.LibrarySort,
		Limit:  limit,
		Offset: offset,
	}
	switch vm.Status {
	case "archived":
		query.Status = ports.DocumentLibraryArchived
	case "all":
		query.Status = ports.DocumentLibraryAll
	default:
		query.Status = ports.DocumentLibraryActive
	}
	if vm.NodeParam == "none" {
		query.UnassignedOnly = true
		return query, nil
	}
	if vm.NodeParam == "" {
		return query, nil
	}
	if vm.Scope != "subtree" {
		query.NodeIDs = []string{vm.NodeParam}
		query.FilterNodeIDs = true
		return query, nil
	}
	allowed, err := s.wissenSubtreeIDs(ctx, ownerID, vm.NodeParam)
	if err != nil {
		return ports.DocumentLibraryQuery{}, err
	}
	query.NodeIDs = make([]string, 0, len(allowed))
	query.FilterNodeIDs = true
	for id := range allowed {
		query.NodeIDs = append(query.NodeIDs, id)
	}
	return query, nil
}

func wissenSearchRow(h domain.SearchHit) webui.SearchRow {
	return webui.SearchRow{
		ID:              h.ID,
		Title:           h.Title,
		Path:            h.Path,
		Archived:        h.Archived,
		ContextEligible: h.Type.ContextEligible(),
		ChipClass:       webui.DocTypeChipClass(h.Type),
		ChipLabel:       webui.DocTypeLabel(h.Type),
		Snippet:         renderSnippet(h.Snippet),
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

	var allTags []domain.TagCount
	if s.ListDocumentLibrary.Docs == nil {
		docType := domain.TaggableDocument
		var err error
		allTags, err = s.ListTags.Execute(r.Context(), u.ID, domain.TagScope{Type: &docType})
		if err != nil {
			return webui.WissenVM{}, nil, "", err
		}
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
		nodeOptions = wissenNodeOptions(nodes)
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

func withWissenTagTotals(vm webui.WissenVM, totals []domain.TagCount, basePath, typeKey string) webui.WissenVM {
	activeSet := make(map[string]bool, len(vm.ActiveTags))
	counts := make(map[string]int, len(totals)+len(vm.ActiveTags))
	for _, tag := range vm.ActiveTags {
		activeSet[tag] = true
		counts[tag] = 0
	}
	for _, total := range totals {
		counts[total.Tag] = total.Count
	}
	totals = totals[:0]
	for tag, count := range counts {
		totals = append(totals, domain.TagCount{Tag: tag, Count: count})
	}
	sort.Slice(totals, func(i, j int) bool {
		if totals[i].Count != totals[j].Count {
			return totals[i].Count > totals[j].Count
		}
		return totals[i].Tag < totals[j].Tag
	})
	vm.AllTags = make([]webui.TagChip, 0, len(totals))
	for _, total := range totals {
		vm.AllTags = append(vm.AllTags, webui.TagChip{
			Tag: total.Tag, Count: total.Count, Active: activeSet[total.Tag],
			Href: basePath + wissenQueryStringFull(typeKey, toggledTags(vm.ActiveTags, total.Tag), vm.SearchQ, vm.Status, vm.NodeParam, "scope", vm.Scope),
		})
	}
	return vm
}

func wissenNodeOptions(nodes []domain.Node) []webui.WissenNodeOption {
	byID := make(map[string]domain.Node, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	memo := make(map[string]string, len(nodes))
	var label func(string, map[string]bool) string
	label = func(id string, visiting map[string]bool) string {
		if cached, ok := memo[id]; ok {
			return cached
		}
		node, ok := byID[id]
		if !ok {
			return ""
		}
		if visiting[id] {
			return node.Name
		}
		visiting[id] = true
		name := node.Name
		if node.ParentID != nil {
			if parent := label(*node.ParentID, visiting); parent != "" {
				name = parent + " / " + name
			}
		}
		delete(visiting, id)
		memo[id] = name
		return name
	}
	options := make([]webui.WissenNodeOption, 0, len(nodes))
	for _, node := range nodes {
		options = append(options, webui.WissenNodeOption{ID: node.ID, Name: label(node.ID, map[string]bool{})})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Name != options[j].Name {
			return options[i].Name < options[j].Name
		}
		return options[i].ID < options[j].ID
	})
	return options
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

func (s *Server) handleWebWissenBulk(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ids := splitWissenDocumentIDs(r.Form["ids"])
	action := r.FormValue("action")
	input := usecase.BulkCurateDocumentsInput{IDs: ids}
	eventAction := action
	switch action {
	case "archive":
		value := true
		input.Archived = &value
	case "restore":
		value := false
		input.Archived = &value
	case "mode":
		mode := domain.ContextMode(r.FormValue("mode"))
		input.ContextMode = &mode
		eventAction = "context." + string(mode)
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	changed, err := s.BulkCurateDocuments.Execute(r.Context(), u.ID, input)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, domain.ErrInvalidDocument) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	for _, doc := range changed {
		data := map[string]any{"id": doc.ID, "title": doc.Title, "action": eventAction}
		if doc.NodeID != nil {
			data["node"] = *doc.NodeID
		}
		s.emitEvent(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: data})
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Trigger", "wissenBulkDone")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/wissen", http.StatusSeeOther)
}

func splitWissenDocumentIDs(values []string) []string {
	var ids []string
	for _, value := range values {
		ids = append(ids, strings.FieldsFunc(value, func(r rune) bool { return r == ',' })...)
	}
	return ids
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
	if node == "" || node == "none" {
		return ""
	}
	// The Wissen node picker means the complete containment subtree. Only a
	// Cockpit link from its explicit "this node only" view may opt into self.
	if v == "self" {
		return "self"
	}
	return "subtree"
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
