package httpserver

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// nodeFormValues reads all node form fields from the request.
func nodeFormValues(r *http.Request) webui.NodeFormValues {
	return webui.NodeFormValues{
		Name:         r.FormValue("name"),
		Slug:         r.FormValue("slug"),
		Kind:         r.FormValue("kind"),
		ParentID:     r.FormValue("parentId"),
		Description:  r.FormValue("description"),
		UpstreamGit:  r.FormValue("upstreamGit"),
		Status:       r.FormValue("status"),
		Color:        r.FormValue("color"),
		RateAmount:   r.FormValue("rateAmount"),
		RateCurrency: r.FormValue("rateCurrency"),
		TagsCSV:      r.FormValue("tags"),
		CountsMode:   r.FormValue("countsMode"),
	}
}

// countsModeToPtr maps the tri-state form value to the *bool override
// (nil = inherit, *true = Work, *false = Privat).
func countsModeToPtr(mode string) *bool {
	switch mode {
	case "work":
		t := true
		return &t
	case "privat":
		f := false
		return &f
	default: // "inherit" / ""
		return nil
	}
}

// countsModeOf is the inverse: renders a node's override as the form value.
func countsModeOf(v *bool) string {
	switch {
	case v == nil:
		return "inherit"
	case *v:
		return "work"
	default:
		return "privat"
	}
}

// nodesListData loads the owner's nodes, applies the status filter and builds
// the Projekte page's tree-as-content view model. "" → active+paused;
// "archived" → archived only; "all". Side-sources (sessions/docs/running) each
// degrade silently on failure (brief §Zustände "Request-Fehler") — the page
// still renders, just with "—"/quiet notes instead of hours/doc-counts/timer.
func (s *Server) nodesListData(r *http.Request, u domain.User) webui.NodesPageData {
	ctx := r.Context()
	status := r.URL.Query().Get("status")
	all, err := s.ListNodes.Execute(ctx, u.ID)
	if err != nil {
		return webui.NodesPageData{User: u.Username, Status: status}
	}
	filtered := make([]domain.Node, 0, len(all))
	for _, n := range all {
		switch status {
		case "all":
			filtered = append(filtered, n)
		case "archived":
			if n.Status == domain.NodeArchived {
				filtered = append(filtered, n)
			}
		default: // active + paused
			if n.Status == domain.NodeActive || n.Status == domain.NodePaused {
				filtered = append(filtered, n)
			}
		}
	}

	now := s.Clock.Now()
	var sessions []domain.WorkSession
	if s.ListSessionsRange.Sessions != nil {
		since := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		if sess, serr := s.ListSessionsRange.Execute(ctx, u.ID, since, now.AddDate(0, 0, 1)); serr == nil {
			sessions = sess
		} else {
			slog.WarnContext(ctx, "projekte: list sessions failed", "err", serr)
		}
	}

	docCounts := map[string]int{}
	if s.ListDocuments.Docs != nil {
		if docs, derr := s.ListDocuments.Execute(ctx, u.ID, nil, nil); derr == nil {
			for _, d := range docs {
				if d.NodeID != nil {
					docCounts[*d.NodeID]++
				}
			}
		} else {
			slog.WarnContext(ctx, "projekte: list documents failed", "err", derr)
		}
	}

	var running *domain.WorkSession
	if s.GetRunningSession.Sessions != nil {
		if rs, ok, rerr := s.GetRunningSession.Execute(ctx, u.ID); rerr == nil && ok {
			r2 := rs
			running = &r2
		} else if rerr != nil {
			slog.WarnContext(ctx, "projekte: get running session failed", "err", rerr)
		}
	}

	return webui.NodesPageData{
		User:   u.Username,
		Status: status,
		VM:     webui.BuildProjectsVM(filtered, sessions, docCounts, running, now),
	}
}

func (s *Server) handleWebNodesHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodesPage(s.nodesListData(r, u)).Render(r.Context(), w)
}

func (s *Server) handleWebNodesList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodesFragment(s.nodesListData(r, u)).Render(r.Context(), w)
}

// ---------------------------------------------------------------------------
// Node form helpers
// ---------------------------------------------------------------------------

// parseRate reads the optional rate form fields. Blank amount → (nil, nil) =
// "clear the rate". A malformed amount → error (re-rendered as a form error).
func parseRate(amount, currency string) (*domain.Money, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil || f < 0 {
		return nil, fmt.Errorf("ungültiger Satz %q", amount)
	}
	cur := strings.TrimSpace(currency)
	if cur == "" {
		cur = "EUR"
	}
	return &domain.Money{Amount: int64(f*100 + 0.5), Currency: cur}, nil
}

// orStatus defaults an empty status form value to "active".
func orStatus(s string) string {
	if s == "" {
		return "active"
	}
	return s
}

// parentPtr converts an empty-or-ID string to *string.
func parentPtr(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

// nodeParents returns candidate parents (engagements + vorhaben) for the form.
func (s *Server) nodeParents(r *http.Request, u domain.User) []domain.Node {
	all, _ := s.ListNodes.Execute(r.Context(), u.ID)
	var out []domain.Node
	for _, n := range all {
		if n.Kind == domain.KindEngagement || n.Kind == domain.KindVorhaben {
			out = append(out, n)
		}
	}
	return out
}

// i18nT resolves an i18n key in the request's language for handler-side messages.
func i18nT(r *http.Request, key string) string { return i18n.T(r.Context(), key) }

// ---------------------------------------------------------------------------
// Node form handlers
// ---------------------------------------------------------------------------

func (s *Server) handleWebNodeNew(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "engagement"
	}
	vals := webui.NodeFormValues{Kind: kind, Status: "active"}
	if parentID := r.URL.Query().Get("parent"); parentID != "" {
		vals.ParentID = parentID
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeForm(webui.NodeFormData{
		User:    u.Username,
		Vals:    vals,
		Parents: s.nodeParents(r, u),
	}, nil).Render(r.Context(), w)
}

func (s *Server) handleWebNodeCreate(w http.ResponseWriter, r *http.Request) {
	// Cap the whole multipart body: ParseMultipartForm would otherwise buffer
	// an unbounded body (32 MiB RAM + unlimited temp files) before the logo
	// LimitReader ever runs. Headroom covers the non-file form fields.
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxNodeBannerBytes+64*1024)
	u, _ := userFrom(r.Context())
	vals := nodeFormValues(r)
	rate, rerr := parseRate(vals.RateAmount, vals.RateCurrency)
	reRender := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.NodeForm(webui.NodeFormData{User: u.Username, Error: msg, Vals: vals, Parents: s.nodeParents(r, u)}, nil).Render(r.Context(), w)
	}
	if vals.Name == "" {
		reRender(i18nT(r, "node.err.nameRequired"))
		return
	}
	if rerr != nil {
		reRender(rerr.Error())
		return
	}
	bannerData, errMsg, ok := readValidatedBanner(r)
	if !ok {
		reRender(errMsg)
		return
	}
	// Reject a bad upstream up front so we never create a half-configured project.
	if vals.UpstreamGit != "" {
		if _, ok := domain.NormalizeRemoteSlug(vals.UpstreamGit); !ok {
			reRender(i18nT(r, "node.err.badUpstream"))
			return
		}
	}
	kind := domain.NodeKind(vals.Kind)
	if kind == "" {
		kind = domain.KindEngagement
	}
	parent := parentPtr(vals.ParentID)
	if kind == domain.KindEngagement {
		parent = nil // engagements are always roots
	}
	tags := strings.Fields(vals.TagsCSV)
	var nodeRate *domain.Money
	if kind == domain.KindEngagement {
		nodeRate = rate
	}
	n, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{
		Name: vals.Name, Slug: vals.Slug, Kind: kind, ParentID: parent,
		Color:       vals.Color,
		Description: vals.Description, UpstreamGit: vals.UpstreamGit,
		CountsTowardTarget: countsModeToPtr(vals.CountsMode),
		Rate:               nodeRate, Tags: &tags, BannerData: bannerData,
	})
	if err != nil {
		reRender(i18nT(r, "node.err.create") + ": " + err.Error())
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeCreated, UserID: u.ID, Data: map[string]any{"id": n.ID, "name": n.Name}})
	http.Redirect(w, r, "/nodes/"+n.ID, http.StatusSeeOther)
}

func (s *Server) handleWebNodeEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	n, err := s.GetNode.Execute(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.webNotFound(w, r)
		return
	}
	vals := webui.NodeFormValues{
		Name:        n.Name,
		Slug:        n.Slug,
		Kind:        string(n.Kind),
		Description: n.Description,
		UpstreamGit: n.UpstreamGit,
		Status:      string(n.Status),
		Color:       n.Color,
		CountsMode:  countsModeOf(n.CountsTowardTarget),
	}
	if n.ParentID != nil {
		vals.ParentID = *n.ParentID
	}
	if n.Rate != nil {
		vals.RateAmount = fmt.Sprintf("%d.%02d", n.Rate.Amount/100, n.Rate.Amount%100)
		vals.RateCurrency = n.Rate.Currency
	}
	if s.GetTags.Tags != nil {
		if tags, terr := s.GetTags.Execute(r.Context(), u.ID, domain.TaggableNode, n.ID); terr == nil {
			slugs := make([]string, len(tags))
			for i, tg := range tags {
				slugs[i] = tg.Slug
			}
			vals.TagsCSV = strings.Join(slugs, " ")
		}
	}
	all, _ := s.ListNodes.Execute(r.Context(), u.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeForm(webui.NodeFormData{
		User: u.Username, Vals: vals, Parents: s.nodeParents(r, u),
		MoveTargets: webui.MoveTargetsFor(all, n),
	}, &n).Render(r.Context(), w)
}

func (s *Server) handleWebNodeUpdate(w http.ResponseWriter, r *http.Request) {
	// Cap the whole multipart body: ParseMultipartForm would otherwise buffer
	// an unbounded body (32 MiB RAM + unlimited temp files) before the logo
	// LimitReader ever runs. Headroom covers the non-file form fields.
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxNodeBannerBytes+64*1024)
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	vals := nodeFormValues(r)
	rate, rerr := parseRate(vals.RateAmount, vals.RateCurrency)
	cur, gerr := s.GetNode.Execute(r.Context(), u.ID, id)
	if gerr != nil {
		s.webNotFound(w, r)
		return
	}
	reRender := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.NodeForm(webui.NodeFormData{User: u.Username, Error: msg, Vals: vals, Parents: s.nodeParents(r, u)}, &cur).Render(r.Context(), w)
	}
	if rerr != nil {
		reRender(rerr.Error())
		return
	}
	bannerData, errMsg, ok := readValidatedBanner(r)
	if !ok {
		reRender(errMsg)
		return
	}
	tags := strings.Fields(vals.TagsCSV)
	removeBanner := r.FormValue("bannerRemove") == "1"
	if removeBanner {
		bannerData = nil
	}
	n, err := s.UpdateNode.Execute(r.Context(), u.ID, id, usecase.UpdateNodeInput{
		Name:        sp(vals.Name),
		Slug:        sp(vals.Slug),
		Color:       sp(vals.Color),
		Description: sp(vals.Description),
		UpstreamGit: sp(vals.UpstreamGit),
		Status:      nsp(domain.NodeStatus(orStatus(vals.Status))),
		ApplyRate:   cur.Kind == domain.KindEngagement, Rate: rate,
		ApplyCountsTowardTarget: true, CountsTowardTarget: countsModeToPtr(vals.CountsMode),
		Tags: &tags, BannerData: bannerData, DeleteBanner: removeBanner,
	})
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		s.webNotFound(w, r)
		return
	case errors.Is(err, domain.ErrInvalidNode) || errors.Is(err, domain.ErrInvalidUpstream):
		reRender(err.Error())
		return
	case err != nil:
		s.webServerError(w, r, err)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": n.ID, "name": n.Name}})
	http.Redirect(w, r, "/nodes/"+id, http.StatusSeeOther)
}

// handleWebNodeStatus applies a single status transition (full-replace
// UpdateNode preserving current fields).
func (s *Server) handleWebNodeStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	cur, err := s.GetNode.Execute(r.Context(), u.ID, id)
	if err != nil {
		s.webNotFound(w, r)
		return
	}
	_, err = s.UpdateNode.Execute(r.Context(), u.ID, id, usecase.UpdateNodeInput{
		Name:        sp(cur.Name),
		Slug:        sp(cur.Slug),
		Color:       sp(cur.Color),
		Glyph:       sp(cur.Glyph),
		Icon:        sp(cur.Icon),
		Description: sp(cur.Description),
		UpstreamGit: sp(cur.UpstreamGit),
		Status:      nsp(domain.NodeStatus(r.FormValue("status"))),
	})
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id, "name": cur.Name}})
	http.Redirect(w, r, "/nodes/"+id, http.StatusSeeOther)
}

func (s *Server) handleWebNodeDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if err := s.DeleteNode.Execute(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, ports.ErrNodeHasChildren) {
			http.Redirect(w, r, "/nodes/"+id+"?err=children", http.StatusSeeOther)
			return
		}
		if errors.Is(err, ports.ErrNodeHasProjectDocuments) {
			http.Redirect(w, r, "/nodes/"+id+"?err=documents", http.StatusSeeOther)
			return
		}
		s.webServerError(w, r, err)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/nodes", http.StatusSeeOther)
}

// handleWebNodeMove handles POST /nodes/{id}/move — reads parentId from form
// ("" = root), calls MoveNode, and redirects to the cockpit on success. On a
// cycle or invalid-kind error it redirects back with an error query param.
func (s *Server) handleWebNodeMove(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	parent := parentPtr(r.FormValue("parentId"))
	_, err := s.MoveNode.Execute(r.Context(), u.ID, id, parent)
	switch {
	case errors.Is(err, usecase.ErrNodeCycle):
		http.Redirect(w, r, "/nodes/"+id+"?err=cycle", http.StatusSeeOther)
		return
	case errors.Is(err, domain.ErrInvalidNode):
		http.Redirect(w, r, "/nodes/"+id+"?err=move", http.StatusSeeOther)
		return
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Redirect(w, r, "/nodes/"+id+"?err=move", http.StatusSeeOther)
		return
	case err != nil:
		s.webServerError(w, r, err)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeMoved, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/nodes/"+id, http.StatusSeeOther)
}
