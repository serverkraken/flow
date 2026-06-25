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

func (s *Server) wissenData(r *http.Request, u domain.User) (webui.WissenVM, error) {
	active := r.URL.Query()["tag"]
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	offset := (page - 1) * wissenPageSize

	allTags, err := s.ListTags.Execute(r.Context(), u.ID)
	if err != nil {
		return webui.WissenVM{}, err
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
			Href:   wissenToggleTagHref(active, tc.Tag),
		})
	}

	vm := webui.WissenVM{
		User:       u.Username,
		AllTags:    chips,
		ActiveTags: active,
		SearchQ:    q,
		Query:      wissenEncodeListQuery(active, q),
	}
	if q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, nil, active)
		if err != nil {
			return webui.WissenVM{}, err
		}
		for _, h := range hits {
			vm.Results = append(vm.Results, webui.SearchRow{
				DocRow: webui.DocRow{
					ID: h.ID, Type: string(h.Type), Path: h.Path, Title: h.Title, Tags: h.Tags,
				},
				Snippet: renderSnippet(h.Snippet),
			})
		}
		return vm, nil
	}

	docs, total, err := s.listDocumentsPage(r.Context(), u.ID, active, wissenPageSize, offset)
	if err != nil {
		return webui.WissenVM{}, err
	}
	names, colors, err := s.projectNameColorMaps(r.Context(), u.ID)
	if err != nil {
		return webui.WissenVM{}, err
	}
	grouped := webui.GroupDocsByCategory(docs, names, colors)
	grouped.User = vm.User
	grouped.AllTags = vm.AllTags
	grouped.ActiveTags = active
	grouped.SearchQ = q
	grouped.Query = vm.Query
	grouped.Page = components.PageNav{
		Page:     page,
		Total:    total,
		PageSize: wissenPageSize,
		BaseHref: "/wissen" + wissenEncodeListQuery(active, q),
	}
	return grouped, nil
}

func (s *Server) handleWebWissenHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.wissenData(r, u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebWissenList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.wissenData(r, u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenFragment(vm).Render(r.Context(), w)
}

func (s *Server) listDocumentsPage(ctx context.Context, ownerID string, tags []string, limit, offset int) ([]domain.Document, int, error) {
	if s.ListDocumentsPage != nil {
		return s.ListDocumentsPage.Execute(ctx, ownerID, nil, tags, limit, offset)
	}
	all, err := s.ListDocuments.Execute(ctx, ownerID, nil, tags)
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (s *Server) projectNameColorMaps(ctx context.Context, ownerID string) (map[string]string, map[string]string, error) {
	names := map[string]string{}
	colors := map[string]string{}
	if s.ListProjects.Projects == nil {
		return names, colors, nil
	}
	projects, err := s.ListProjects.Execute(ctx, ownerID)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range projects {
		names[p.ID] = p.Name
		colors[p.ID] = p.Color
	}
	return names, colors, nil
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

func wissenToggleTagHref(active []string, tag string) string {
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
	return "/wissen" + wissenEncodeTagQuery(next)
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
