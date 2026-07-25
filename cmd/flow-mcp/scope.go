package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/noderef"
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

// resolveWriteScope is deliberately stricter than read scoping: an unresolved
// cwd must never turn an omitted project into an unassigned/global write.
// Unassigned writes require the explicit sentinel "none".
func (h *handlers) resolveWriteScope(ctx context.Context, project string) (scope, error) {
	switch strings.TrimSpace(project) {
	case "":
		if _, matched := h.resolved(); !matched {
			return scope{}, errGuard{fmt.Errorf("no project is bound to this directory; use flow_bind_project or pass project=\"none\" explicitly")}
		}
	case "global":
		return scope{}, errGuard{fmt.Errorf("project=\"global\" is read-only; pass project=\"none\" explicitly for an unassigned write")}
	}
	return h.resolveScope(ctx, project)
}

// lookupNode finds a project by id, slug, or name (case-insensitive for slug
// and name). On a miss it refreshes the cache once — to catch a just-created
// project — then returns an actionable error listing the known slugs.
func (h *handlers) lookupNode(ctx context.Context, ref string) (domain.Node, error) {
	ps, err := h.nodeList(ctx, false)
	if err != nil {
		return domain.Node{}, fmt.Errorf("flow server error listing projects: %w", err)
	}
	if p, err := matchNodeRef(ps, ref); err == nil {
		return p, nil
	} else if !errors.Is(err, noderef.ErrNotFound) {
		return domain.Node{}, errGuard{err}
	}
	ps, err = h.nodeList(ctx, true) // refresh once, then retry
	if err != nil {
		return domain.Node{}, fmt.Errorf("flow server error listing projects: %w", err)
	}
	if p, err := matchNodeRef(ps, ref); err == nil {
		return p, nil
	} else if !errors.Is(err, noderef.ErrNotFound) {
		return domain.Node{}, errGuard{err}
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
	node, err := matchNodeRef(ps, ref)
	return node, err == nil
}

func matchNodeRef(ps []domain.Node, ref string) (domain.Node, error) {
	ref = strings.TrimSpace(ref)
	if node, err := noderef.Resolve(ps, ref); err == nil {
		return node, nil
	} else if !errors.Is(err, noderef.ErrNotFound) {
		return domain.Node{}, err
	}

	var nameMatch domain.Node
	nameMatches := 0
	for _, p := range ps {
		if strings.EqualFold(p.Name, ref) {
			nameMatch = p
			nameMatches++
		}
	}
	if nameMatches == 1 {
		return nameMatch, nil
	}
	if nameMatches > 1 {
		return domain.Node{}, fmt.Errorf("%w: name %q matches multiple nodes; use an id or qualified slug path", noderef.ErrAmbiguous, ref)
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
		return match, nil
	}
	if matches > 1 {
		return domain.Node{}, fmt.Errorf("%w: remote %q matches multiple nodes; use an id or qualified slug path", noderef.ErrAmbiguous, ref)
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
		return match, nil
	}
	if matches > 1 {
		return domain.Node{}, fmt.Errorf("%w: remote %q matches multiple nodes; use an id or qualified slug path", noderef.ErrAmbiguous, ref)
	}
	return domain.Node{}, fmt.Errorf("%w: %q", noderef.ErrNotFound, ref)
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

// nodeTarget resolves a tool's optional `node` argument to the node it names,
// or — when omitted — to the node bound to this directory. Unlike resolveScope it
// never yields the "all projects" or "unassigned" scopes: every tool in the node
// family acts on exactly one node. The miss message names both binding tools
// because either one fixes it (Spec §3 flow_get_node).
//
// Only Node.ID is guaranteed fresh. The omitted branch returns the auth-time
// resolved snapshot, so a node renamed since then still carries its old
// Name/Slug and has no LogoRef. A caller that PRINTS those fields must re-read
// the node with GetNode by ID (flow_get_node does exactly that); a caller that
// only needs the ID to address a mutation may use the result directly.
func (h *handlers) nodeTarget(ctx context.Context, ref string) (domain.Node, error) {
	if r := strings.TrimSpace(ref); r != "" {
		return h.lookupNode(ctx, r)
	}
	if node, matched := h.resolved(); matched {
		return node, nil
	}
	return domain.Node{}, errGuard{errors.New(`no node is bound to this directory: pass node=<slug/name/id>, or bind this directory with flow_node_binding (action="bind") or flow_bind_project`)}
}

// prefixGuard prefixes a guard error's message so the model learns WHICH
// argument was bad. A non-guard error (transport, auth, server) is returned
// untouched — wrapping it in errGuard would downgrade a server failure to
// invalid_request and tell the model to fix its arguments instead of retrying.
func prefixGuard(prefix string, err error) error {
	var g errGuard
	if errors.As(err, &g) {
		return errGuard{fmt.Errorf("%s: %s", prefix, g.Error())}
	}
	return err
}
