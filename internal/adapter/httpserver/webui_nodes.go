package httpserver

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
		Glyph:        r.FormValue("glyph"),
		Icon:         r.FormValue("icon"),
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
// the indented tree.  "" → active+paused; "archived" → archived only; "all".
func (s *Server) nodesListData(r *http.Request, u domain.User) webui.NodesPageData {
	status := r.URL.Query().Get("status")
	all, _ := s.ListNodes.Execute(r.Context(), u.ID)
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
	return webui.NodesPageData{
		User:   u.Username,
		Status: status,
		Rows:   webui.BuildTree(filtered),
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
	logoData, lerr := readLogoUpload(r)
	if lerr != nil {
		reRender(i18nT(r, "node.err.logo"))
		return
	}
	if len(logoData) > 0 {
		if _, verr := usecase.ValidateNodeLogo(logoData); verr != nil {
			reRender(logoErrMsg(r, verr))
			return
		}
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
	n, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{
		Name: vals.Name, Slug: vals.Slug, Kind: kind, ParentID: parent,
		Color: vals.Color, Glyph: vals.Glyph, Icon: vals.Icon,
		Description: vals.Description, UpstreamGit: vals.UpstreamGit,
		CountsTowardTarget: countsModeToPtr(vals.CountsMode),
	})
	if err != nil {
		reRender(i18nT(r, "node.err.create") + ": " + err.Error())
		return
	}
	if kind == domain.KindEngagement && rate != nil {
		_ = s.SetNodeRate.Execute(r.Context(), u.ID, n.ID, rate)
	}
	if s.SetTags.Tags != nil {
		if _, err := s.SetTags.Execute(r.Context(), u.ID, domain.TaggableNode, n.ID, strings.Fields(vals.TagsCSV)); err != nil {
			slog.WarnContext(r.Context(), "webui: set node tags failed", "nodeID", n.ID, "err", err)
		}
	}
	if len(logoData) > 0 {
		if _, err := s.UploadNodeLogo.Execute(r.Context(), u.ID, n.ID, logoData); err != nil {
			slog.WarnContext(r.Context(), "webui: upload node logo failed", "nodeID", n.ID, "err", err)
		}
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeCreated, UserID: u.ID, Data: map[string]any{"id": n.ID, "name": n.Name}})
	http.Redirect(w, r, "/nodes/"+n.ID, http.StatusSeeOther)
}

func (s *Server) handleWebNodeEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	n, err := s.GetNode.Execute(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
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
		Glyph:       n.Glyph,
		Icon:        n.Icon,
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeForm(webui.NodeFormData{User: u.Username, Vals: vals, Parents: s.nodeParents(r, u)}, &n).Render(r.Context(), w)
}

func (s *Server) handleWebNodeUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	vals := nodeFormValues(r)
	rate, rerr := parseRate(vals.RateAmount, vals.RateCurrency)
	cur, gerr := s.GetNode.Execute(r.Context(), u.ID, id)
	if gerr != nil {
		http.Error(w, "not found", http.StatusNotFound)
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
	logoData, lerr := readLogoUpload(r)
	if lerr != nil {
		reRender(i18nT(r, "node.err.logo"))
		return
	}
	if len(logoData) > 0 {
		if _, verr := usecase.ValidateNodeLogo(logoData); verr != nil {
			reRender(logoErrMsg(r, verr))
			return
		}
	}
	n, err := s.UpdateNode.Execute(r.Context(), u.ID, id, usecase.UpdateNodeInput{
		Name:        vals.Name,
		Slug:        vals.Slug,
		Color:       vals.Color,
		Glyph:       vals.Glyph,
		Icon:        vals.Icon,
		Description: vals.Description,
		UpstreamGit: vals.UpstreamGit,
		Status:      domain.NodeStatus(orStatus(vals.Status)),
	})
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, domain.ErrInvalidNode) || errors.Is(err, domain.ErrInvalidUpstream):
		reRender(err.Error())
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	// Rate applies only to engagements; nil clears any existing rate.
	if n.Kind == domain.KindEngagement {
		_ = s.SetNodeRate.Execute(r.Context(), u.ID, id, rate)
	}
	// Work/Privat tri-state: always-apply (UpdateNodeInput's nil means preserve,
	// which cannot express "set back to inherit" — SetCountsTowardTarget can).
	if _, err := s.SetCountsTowardTarget.Execute(r.Context(), u.ID, id, countsModeToPtr(vals.CountsMode)); err != nil {
		slog.WarnContext(r.Context(), "webui: set counts-toward-target failed", "nodeID", id, "err", err)
	}
	if s.SetTags.Tags != nil {
		if _, err := s.SetTags.Execute(r.Context(), u.ID, domain.TaggableNode, n.ID, strings.Fields(vals.TagsCSV)); err != nil {
			slog.WarnContext(r.Context(), "webui: set node tags failed", "nodeID", n.ID, "err", err)
		}
	}
	if r.FormValue("logoRemove") == "1" {
		if _, err := s.DeleteNodeLogo.Execute(r.Context(), u.ID, id); err != nil {
			slog.WarnContext(r.Context(), "webui: delete node logo failed", "nodeID", id, "err", err)
		}
	} else if len(logoData) > 0 {
		if _, err := s.UploadNodeLogo.Execute(r.Context(), u.ID, id, logoData); err != nil {
			slog.WarnContext(r.Context(), "webui: upload node logo failed", "nodeID", id, "err", err)
		}
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
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_, err = s.UpdateNode.Execute(r.Context(), u.ID, id, usecase.UpdateNodeInput{
		Name:        cur.Name,
		Slug:        cur.Slug,
		Color:       cur.Color,
		Glyph:       cur.Glyph,
		Icon:        cur.Icon,
		Description: cur.Description,
		UpstreamGit: cur.UpstreamGit,
		Status:      domain.NodeStatus(r.FormValue("status")),
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
		http.Error(w, "server error", http.StatusInternalServerError)
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
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeMoved, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/nodes/"+id, http.StatusSeeOther)
}

// logoErrMsg maps logo validation sentinels to i18n form errors.
func logoErrMsg(r *http.Request, err error) string {
	switch {
	case errors.Is(err, usecase.ErrLogoTooLarge):
		return i18nT(r, "node.err.logoSize")
	case errors.Is(err, usecase.ErrLogoBadType):
		return i18nT(r, "node.err.logoType")
	default:
		return i18nT(r, "node.err.logo")
	}
}
