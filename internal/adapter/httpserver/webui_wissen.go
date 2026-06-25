package httpserver

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func (s *Server) wissenOverviewData(r *http.Request, u domain.User) (webui.WissenOverviewVM, error) {
	active := r.URL.Query()["tag"]
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	allTags, err := s.ListTags.Execute(r.Context(), u.ID)
	if err != nil {
		return webui.WissenOverviewVM{}, err
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

	base := webui.WissenVM{
		User:       u.Username,
		AllTags:    chips,
		ActiveTags: active,
		SearchQ:    q,
		Query:      wissenEncodeListQuery(active, q),
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
	names, colors, err := s.projectNameColorMaps(r.Context(), u.ID)
	if err != nil {
		return webui.WissenOverviewVM{}, err
	}
	vm := webui.BuildWissenOverview(docs, names, colors)
	vm.WissenVM = base
	return vm, nil
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
