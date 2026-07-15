package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// scope is a tool call's resolved project filter: the apiclient nodeID pointer
// (nil → all projects, "none" → unassigned, &id → one project) plus a human label
// for result text.
type scope struct {
	nodeID *string
	label  string
}

// resolveScope maps a tool's optional `project` argument to a scope. Accepts:
// "" (use the cwd-resolved project, or global if none is bound), "global" (all
// projects), "none" (unassigned documents), or a project id / slug / name (looked
// up against the cached project list, refreshed once on a miss). An unknown
// reference returns an error whose message is shown to the model — never a silent
// empty result.
func (h *handlers) resolveScope(ctx context.Context, project string) (scope, error) {
	switch p := strings.TrimSpace(project); p {
	case "":
		if proj, matched := h.resolved(); matched {
			id := proj.ID
			return scope{nodeID: &id, label: "in project " + proj.Name}, nil
		}
		return scope{nodeID: nil, label: "across all projects (no project is bound to this directory — use flow_bind_project)"}, nil
	case "global":
		return scope{nodeID: nil, label: "across all projects"}, nil
	case "none":
		none := "none"
		return scope{nodeID: &none, label: "among unassigned documents"}, nil
	default:
		proj, err := h.lookupNode(ctx, p)
		if err != nil {
			return scope{}, err
		}
		id := proj.ID
		return scope{nodeID: &id, label: "in project " + proj.Name}, nil
	}
}

// lookupNode finds a project by id, slug, or name (case-insensitive for slug
// and name). On a miss it refreshes the cache once — to catch a just-created
// project — then returns an actionable error listing the known slugs.
func (h *handlers) lookupNode(ctx context.Context, ref string) (domain.Node, error) {
	ps, err := h.nodeList(ctx, false)
	if err != nil {
		return domain.Node{}, fmt.Errorf("flow server error listing projects: %w", err)
	}
	if p, ok := matchNode(ps, ref); ok {
		return p, nil
	}
	ps, err = h.nodeList(ctx, true) // refresh once, then retry
	if err != nil {
		return domain.Node{}, fmt.Errorf("flow server error listing projects: %w", err)
	}
	if p, ok := matchNode(ps, ref); ok {
		return p, nil
	}
	return domain.Node{}, errGuard{fmt.Errorf("unknown project %q. Use 'global' (all projects), 'none' (unassigned), or a known slug: %s", ref, slugList(ps))}
}

// nodeList returns the cached project list, fetching it once via the seam.
// refresh=true forces a re-fetch.
func (h *handlers) nodeList(ctx context.Context, refresh bool) ([]domain.Node, error) {
	h.projMu.Lock()
	defer h.projMu.Unlock()
	if h.projFetched && !refresh {
		return h.projects, nil
	}
	ps, err := h.listProjects(ctx)
	if err != nil {
		return nil, err
	}
	h.projects = ps
	h.projFetched = true
	return ps, nil
}

// projectName best-effort resolves a project id to its name via the cache;
// returns "" when id is nil or unknown.
func (h *handlers) projectName(ctx context.Context, id *string) string {
	if id == nil {
		return ""
	}
	ps, err := h.nodeList(ctx, false)
	if err != nil {
		return ""
	}
	for _, p := range ps {
		if p.ID == *id {
			return p.Name
		}
	}
	return ""
}

func matchNode(ps []domain.Node, ref string) (domain.Node, bool) {
	ref = strings.TrimSpace(ref)
	for _, p := range ps {
		if p.ID == ref || strings.EqualFold(p.Slug, ref) || strings.EqualFold(p.Name, ref) {
			return p, true
		}
	}

	remoteSlug := strings.ToLower(ref)
	if normalized, ok := domain.NormalizeRemoteSlug(ref); ok {
		remoteSlug = normalized
	}

	var match domain.Node
	matches := 0
	for _, p := range ps {
		if p.Kind != domain.KindRepo || !strings.EqualFold(p.OriginSlug, remoteSlug) {
			continue
		}
		match = p
		matches++
	}
	if matches == 1 {
		return match, true
	}
	if matches > 1 {
		return domain.Node{}, false
	}

	for _, p := range ps {
		if p.Kind != domain.KindRepo {
			continue
		}
		slug, ok := domain.NormalizeRemoteSlug(p.UpstreamGit)
		if !ok || !strings.EqualFold(slug, remoteSlug) {
			continue
		}
		match = p
		matches++
	}
	if matches == 1 {
		return match, true
	}
	return domain.Node{}, false
}

func slugList(ps []domain.Node) string {
	if len(ps) == 0 {
		return "(none)"
	}
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.Slug
	}
	return strings.Join(s, ", ")
}

// checkType validates an optional `type` filter argument against the canonical
// document-type set. "" → no filter (returns ""). An invalid value is an error
// listing the valid types (not a silent empty result).
func checkType(typ string) (domain.DocumentType, error) {
	t := strings.TrimSpace(typ)
	if t == "" {
		return "", nil
	}
	for _, v := range domain.DocumentTypes() {
		if domain.DocumentType(t) == v {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid type %q. Valid types: %s", t, typeList())
}

func typeList() string {
	ts := domain.DocumentTypes()
	s := make([]string, len(ts))
	for i, t := range ts {
		s[i] = string(t)
	}
	return strings.Join(s, ", ")
}
