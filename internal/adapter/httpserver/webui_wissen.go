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

func (s *Server) wissenOverviewData(r *http.Request, u domain.User) (webui.WissenOverviewVM, error) {
	base, active, q, err := s.wissenBaseVM(r, u, "/wissen")
	if err != nil {
		return webui.WissenOverviewVM{}, err
	}

	if q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, nil, active)
		if err != nil {
			return webui.WissenOverviewVM{}, err
		}
		for _, h := range hits {
			base.Results = append(base.Results, webui.SearchRow{
				DocRow: webui.DocRow{
					ID: h.ID, Type: string(h.Type), Path: h.Path, Title: h.Title, Tags: h.Tags,
				},
				Snippet: renderSnippet(h.Snippet),
			})
		}
		return webui.WissenOverviewVM{WissenVM: base}, nil
	}

	docs, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, active)
	if err != nil {
		return webui.WissenOverviewVM{}, err
	}
	_, colors, _, err := s.nodeMaps(r.Context(), u.ID)
	if err != nil {
		return webui.WissenOverviewVM{}, err
	}
	vm := webui.BuildWissenOverview(docs, colors)
	vm.WissenVM = base
	return vm, nil
}

func (s *Server) wissenCategoryData(r *http.Request, u domain.User, c webui.WissenCategory) (webui.WissenCategoryVM, error) {
	base, active, q, err := s.wissenBaseVM(r, u, c.Href)
	if err != nil {
		return webui.WissenCategoryVM{}, err
	}
	if q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, nil, active)
		if err != nil {
			return webui.WissenCategoryVM{}, err
		}
		vm := webui.WissenCategoryVM{WissenVM: base, Category: c}
		for _, h := range hits {
			if !webui.DocumentInWissenCategory(h.Document, c) {
				continue
			}
			vm.Results = append(vm.Results, webui.SearchRow{
				DocRow: webui.DocRow{
					ID: h.ID, Type: string(h.Type), Path: h.Path, Title: h.Title, Tags: h.Tags,
				},
				Snippet: renderSnippet(h.Snippet),
			})
		}
		vm.Total = len(vm.Results)
		return vm, nil
	}

	docs, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, active)
	if err != nil {
		return webui.WissenCategoryVM{}, err
	}
	filtered := filterWissenCategoryDocs(docs, c)
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	offset := (page - 1) * wissenPageSize
	pageDocs := paginateDocuments(filtered, wissenPageSize, offset)

	names, colors, kinds, err := s.nodeMaps(r.Context(), u.ID)
	if err != nil {
		return webui.WissenCategoryVM{}, err
	}
	vm := webui.BuildWissenCategory(c, pageDocs, names, colors, kinds)
	vm.WissenVM = base
	vm.Category = c
	vm.Total = len(filtered)
	vm.Page = components.PageNav{
		Page:     page,
		Total:    len(filtered),
		PageSize: wissenPageSize,
		BaseHref: c.Href + wissenEncodeListQuery(active, q),
	}
	return vm, nil
}

func (s *Server) wissenBaseVM(r *http.Request, u domain.User, basePath string) (webui.WissenVM, []string, string, error) {
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
			Href:   wissenToggleTagHref(basePath, active, tc.Tag),
		})
	}
	return webui.WissenVM{
		User:         u.Username,
		AllTags:      chips,
		ActiveTags:   active,
		SearchQ:      q,
		Query:        wissenEncodeListQuery(active, q),
		SearchAction: basePath,
		ResetHref:    basePath,
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

func (s *Server) handleWebWissenCategory(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	c, ok := webui.WissenCategoryFromSlug(wissenCategorySlug(r))
	if !ok {
		http.NotFound(w, r)
		return
	}
	vm, err := s.wissenCategoryData(r, u, c)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenCategoryPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebWissenCategoryList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	c, ok := webui.WissenCategoryFromSlug(wissenCategorySlug(r))
	if !ok {
		http.NotFound(w, r)
		return
	}
	vm, err := s.wissenCategoryData(r, u, c)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenCategoryFragment(vm).Render(r.Context(), w)
}

func wissenCategorySlug(r *http.Request) string {
	if slug := r.PathValue("category"); slug != "" {
		return slug
	}
	if slug := strings.TrimPrefix(r.URL.Path, "/wissen/"); slug != r.URL.Path {
		return slug
	}
	return strings.TrimPrefix(r.URL.Path, "/ui/wissen/list/")
}

func filterWissenCategoryDocs(docs []domain.Document, c webui.WissenCategory) []domain.Document {
	out := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if webui.DocumentInWissenCategory(d, c) {
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
// wissen view models and document views.
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

func wissenEncodeListQuery(tags []string, q string) string {
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

func wissenToggleTagHref(basePath string, active []string, tag string) string {
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
	return basePath + wissenEncodeTagQuery(next)
}

func wissenEncodeTagQuery(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	q := url.Values{}
	for _, t := range tags {
		q.Add("tag", t)
	}
	return "?" + q.Encode()
}
