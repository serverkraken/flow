package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

const maxContextReorderDocuments = 200
const defaultArchivedDocumentsLimit = 50

type contextInventoryIn struct {
	Repo string `json:"repo,omitempty" jsonschema:"explicit repo id, slug, name, origin slug, or upstream Git remote; default = current directory's resolved repo"`
	Cap  int    `json:"cap,omitempty" jsonschema:"hard token budget used to calculate included and dropped standings; default server budget"`
}

type contextInventoryItem struct {
	ID          string              `json:"id"`
	NodeID      *string             `json:"projectId,omitempty"`
	Scope       string              `json:"scope"`
	Type        domain.DocumentType `json:"type"`
	Path        string              `json:"path"`
	Title       string              `json:"title"`
	Tags        []string            `json:"tags,omitempty"`
	Pinned      bool                `json:"pinned"`
	Priority    int                 `json:"priority"`
	ContextMode domain.ContextMode  `json:"contextMode"`
	EstTokens   int                 `json:"estTokens"`
	State       string              `json:"state"`
	Rank        int                 `json:"rank,omitempty"`
	Total       int                 `json:"total,omitempty"`
	Reason      string              `json:"reason,omitempty"`
}

type contextInventoryResult struct {
	Repo   string                 `json:"repo"`
	Budget usecase.ContextBudget  `json:"budget"`
	Items  []contextInventoryItem `json:"items"`
}

func buildContextInventory(cc usecase.ComposedContext) contextInventoryResult {
	out := contextInventoryResult{Budget: cc.Budget, Items: make([]contextInventoryItem, 0, len(cc.Candidates))}
	if cc.Resolution.Repo != nil {
		out.Repo = cc.Resolution.Repo.Slug
	}
	for _, candidate := range cc.Candidates {
		standing := usecase.StandingOf(cc, candidate.ID)
		state, reason := standing.State, ""
		if candidate.ContextMode.OrAuto() == domain.ContextModeNie {
			state, reason = "hidden", "context_mode_nie"
		} else if state == "absent" {
			switch {
			case candidate.ContextMode.OrAuto() == domain.ContextModeImmer:
				state, reason = "dropped", "hard_budget"
			case candidate.Type == domain.DocInstruction:
				reason = "deduplicated_or_hard_budget"
			case candidate.Type == domain.DocActiveContext:
				reason = "inactive_or_hard_budget"
			case candidate.NodeID == nil:
				reason = "tag_gate"
			default:
				reason = "inactive"
			}
		}
		out.Items = append(out.Items, contextInventoryItem{
			ID: candidate.ID, NodeID: candidate.NodeID, Scope: candidate.ScopeLabel,
			Type: candidate.Type, Path: candidate.Path, Title: candidate.Title, Tags: candidate.Tags,
			Pinned: candidate.Pinned, Priority: candidate.Priority, ContextMode: candidate.ContextMode.OrAuto(),
			EstTokens: candidate.EstTokens, State: state, Rank: standing.Rank, Total: standing.Total, Reason: reason,
		})
	}
	stateRank := map[string]int{"always": 0, "included": 1, "dropped": 2, "hidden": 3, "absent": 4}
	sort.SliceStable(out.Items, func(i, j int) bool {
		if stateRank[out.Items[i].State] != stateRank[out.Items[j].State] {
			return stateRank[out.Items[i].State] < stateRank[out.Items[j].State]
		}
		if out.Items[i].Rank != out.Items[j].Rank {
			return out.Items[i].Rank < out.Items[j].Rank
		}
		if out.Items[i].Scope != out.Items[j].Scope {
			return out.Items[i].Scope < out.Items[j].Scope
		}
		return out.Items[i].Title < out.Items[j].Title
	})
	return out
}

func validateCompleteContextOrder(current, requested []string) error {
	if len(requested) != len(current) {
		return fmt.Errorf("order must contain all %d ranked context document ids exactly once", len(current))
	}
	want := make(map[string]struct{}, len(current))
	for _, id := range current {
		want[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		if _, ok := want[id]; !ok {
			return fmt.Errorf("document %q is not in the current ranked context set", id)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("document %q appears more than once", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (h *handlers) contextQuery(ctx context.Context, repo string, cap int, profile string) (apiclient.ContextQuery, error) {
	q := apiclient.ContextQuery{Cap: cap, Profile: profile}
	if strings.TrimSpace(repo) != "" {
		node, err := h.lookupNode(ctx, repo)
		if err != nil {
			return apiclient.ContextQuery{}, err
		}
		q.Node = node.ID
		return q, nil
	}
	if proj, matched := h.resolved(); matched {
		q.Node = proj.ID
		return q, nil
	}
	return apiclient.ContextQuery{}, errGuard{err: fmt.Errorf("no project is bound to this directory; use flow_bind_project or pass repo explicitly")}
}

func (h *handlers) loadContextInventory(ctx context.Context, c *apiclient.Client, repo string, cap int, client string) (usecase.ComposedContext, contextInventoryResult, error) {
	q, err := h.contextQuery(ctx, repo, cap, string(usecase.ContextProfileFull))
	if err != nil {
		return usecase.ComposedContext{}, contextInventoryResult{}, err
	}
	q.Client = client
	cc, err := c.ComposeContext(ctx, q)
	if err != nil {
		return usecase.ComposedContext{}, contextInventoryResult{}, err
	}
	return cc, buildContextInventory(cc), nil
}

func (h *handlers) contextInventory(ctx context.Context, req *mcp.CallToolRequest, in contextInventoryIn) (*mcp.CallToolResult, any, error) {
	var out contextInventoryResult
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		_, inventory, err := h.loadContextInventory(ctx, c, in.Repo, in.Cap, clientName(req))
		out = inventory
		return err
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return structuredResult(out), nil, nil
}

func rankedContextIDs(cc usecase.ComposedContext) []string {
	ids := make([]string, len(cc.Ranked))
	for i, ranked := range cc.Ranked {
		ids[i] = ranked.Item.ID
	}
	return ids
}

type curateContextIn struct {
	ID       string  `json:"id" jsonschema:"the context document id to curate"`
	Repo     string  `json:"repo,omitempty" jsonschema:"explicit repo id, slug, name, origin slug, or upstream Git remote; default = current directory's resolved repo"`
	Cap      int     `json:"cap,omitempty" jsonschema:"hard token budget used to report the resulting standing; default server budget"`
	Mode     *string `json:"mode,omitempty" jsonschema:"set context membership to auto, immer, or nie"`
	Pinned   *bool   `json:"pinned,omitempty" jsonschema:"pin or unpin this context document"`
	Archived *bool   `json:"archived,omitempty" jsonschema:"archive or un-archive this context document"`
	Confirm  bool    `json:"confirm,omitempty" jsonschema:"required (true) when archiving or un-archiving a human-owned note"`
}

type curateAction string

const (
	curateMode    curateAction = "set_context_mode"
	curatePin     curateAction = "set_pinned"
	curateArchive curateAction = "set_archived"
)

func validateCurateContextInput(in curateContextIn) (curateAction, error) {
	if strings.TrimSpace(in.ID) == "" {
		return "", fmt.Errorf("id is required")
	}
	actions := 0
	var action curateAction
	if in.Mode != nil {
		actions++
		action = curateMode
		mode := domain.ContextMode(strings.TrimSpace(*in.Mode))
		if !mode.Valid() {
			return "", fmt.Errorf("invalid mode %q; use auto, immer, or nie", *in.Mode)
		}
	}
	if in.Pinned != nil {
		actions++
		action = curatePin
	}
	if in.Archived != nil {
		actions++
		action = curateArchive
	}
	if actions != 1 {
		return "", fmt.Errorf("pass exactly one action: mode, pinned, or archived")
	}
	return action, nil
}

type curatedDocumentMetadata struct {
	ID          string              `json:"id"`
	NodeID      *string             `json:"projectId,omitempty"`
	Type        domain.DocumentType `json:"type"`
	Path        string              `json:"path"`
	Title       string              `json:"title"`
	Tags        []string            `json:"tags,omitempty"`
	Pinned      bool                `json:"pinned"`
	Priority    int                 `json:"priority"`
	ContextMode domain.ContextMode  `json:"contextMode"`
	Archived    bool                `json:"archived"`
	ArchivedAt  *time.Time          `json:"archivedAt,omitempty"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

func archivedMetadataOf(d domain.Document) curatedDocumentMetadata {
	return curatedDocumentMetadata{
		ID: d.ID, NodeID: d.NodeID, Type: d.Type, Path: d.Path, Title: d.Title, Tags: d.Tags,
		Pinned: d.Pinned, Priority: d.Priority, ContextMode: d.ContextMode.OrAuto(), Archived: d.Archived,
		ArchivedAt: d.ArchivedAt, UpdatedAt: d.UpdatedAt,
	}
}

type curateContextResult struct {
	Action   curateAction            `json:"action"`
	Document curatedDocumentMetadata `json:"document"`
	Standing *contextInventoryItem   `json:"standing,omitempty"`
	Budget   *usecase.ContextBudget  `json:"budget,omitempty"`
}

func contextInventoryItemByID(inventory contextInventoryResult, id string) (*contextInventoryItem, bool) {
	for i := range inventory.Items {
		if inventory.Items[i].ID == id {
			return &inventory.Items[i], true
		}
	}
	return nil, false
}

func contextDocumentType(typ domain.DocumentType) bool {
	switch typ {
	case domain.DocMemory, domain.DocInstruction, domain.DocActiveContext:
		return true
	default:
		return false
	}
}

func (h *handlers) curateContext(ctx context.Context, req *mcp.CallToolRequest, in curateContextIn) (*mcp.CallToolResult, any, error) {
	action, err := validateCurateContextInput(in)
	if err != nil {
		return h.resultErr(errGuard{err}), nil, nil
	}
	var out curateContextResult
	err = h.do(ctx, req, func(c *apiclient.Client) error {
		cc, inventory, err := h.loadContextInventory(ctx, c, in.Repo, in.Cap, clientName(req))
		if err != nil {
			return err
		}
		cur, err := c.GetDocument(ctx, in.ID)
		if err != nil {
			return err
		}
		standing, selected := contextInventoryItemByID(inventory, cur.ID)
		if action != curateArchive && (!contextDocumentType(cur.Type) || !selected) {
			return errGuard{fmt.Errorf("document %q is not a context candidate for repo %q", cur.ID, inventory.Repo)}
		}
		if action == curateArchive {
			if err := guardMutation(cur, in.Confirm); err != nil {
				return errGuard{err}
			}
			if !cur.Archived && !selected {
				return errGuard{fmt.Errorf("document %q is not a context candidate for repo %q", cur.ID, inventory.Repo)}
			}
			if cur.Archived && !documentBelongsToContext(cc, cur) {
				return errGuard{fmt.Errorf("archived document %q does not belong to repo %q or its context chain", cur.ID, inventory.Repo)}
			}
		}

		switch action {
		case curateMode:
			err = c.SetContextMode(ctx, cur.ID, domain.ContextMode(strings.TrimSpace(*in.Mode)))
		case curatePin:
			err = c.SetPinned(ctx, cur.ID, *in.Pinned)
		case curateArchive:
			err = c.SetArchived(ctx, cur.ID, *in.Archived)
		}
		if err != nil {
			return err
		}
		updated, err := c.GetDocument(ctx, cur.ID)
		if err != nil {
			return err
		}
		if updated.Archived {
			h.removeResource(updated.ID)
		} else if action == curateArchive {
			h.addResource(ctx, updated)
		}
		_, after, err := h.loadContextInventory(ctx, c, in.Repo, in.Cap, clientName(req))
		if err != nil {
			return err
		}
		out = curateContextResult{Action: action, Document: archivedMetadataOf(updated), Budget: &after.Budget}
		if item, ok := contextInventoryItemByID(after, updated.ID); ok {
			out.Standing = item
		} else if updated.Archived {
			out.Standing = &contextInventoryItem{ID: updated.ID, NodeID: updated.NodeID, Type: updated.Type, Path: updated.Path, Title: updated.Title, State: "archived", Reason: "archived"}
		} else if standing != nil {
			out.Standing = standing
		}
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return structuredResult(out), nil, nil
}

func documentBelongsToContext(cc usecase.ComposedContext, d domain.Document) bool {
	if d.NodeID == nil {
		return true
	}
	for _, node := range cc.Resolution.Chain {
		if node.ID == *d.NodeID {
			return true
		}
	}
	return cc.Resolution.Repo != nil && cc.Resolution.Repo.ID == *d.NodeID
}

type reorderContextIn struct {
	Repo string   `json:"repo,omitempty" jsonschema:"explicit repo id, slug, name, origin slug, or upstream Git remote; default = current directory's resolved repo"`
	Cap  int      `json:"cap,omitempty" jsonschema:"hard token budget used to report the resulting standings; default server budget"`
	IDs  []string `json:"ids" jsonschema:"every ranked context document id exactly once, in desired order"`
}

type reorderContextResult struct {
	Repo   string                 `json:"repo"`
	Count  int                    `json:"count"`
	Budget usecase.ContextBudget  `json:"budget"`
	Items  []contextInventoryItem `json:"items"`
}

func (h *handlers) reorderContext(ctx context.Context, req *mcp.CallToolRequest, in reorderContextIn) (*mcp.CallToolResult, any, error) {
	if len(in.IDs) > maxContextReorderDocuments {
		return h.resultErr(errGuard{fmt.Errorf("at most %d context documents can be reordered", maxContextReorderDocuments)}), nil, nil
	}
	var out reorderContextResult
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		cc, inventory, err := h.loadContextInventory(ctx, c, in.Repo, in.Cap, clientName(req))
		if err != nil {
			return err
		}
		if err := validateCompleteContextOrder(rankedContextIDs(cc), in.IDs); err != nil {
			return errGuard{err}
		}
		if err := c.ReorderContext(ctx, in.IDs); err != nil {
			return err
		}
		_, after, err := h.loadContextInventory(ctx, c, in.Repo, in.Cap, clientName(req))
		if err != nil {
			return err
		}
		out = reorderContextResult{Repo: inventory.Repo, Count: len(in.IDs), Budget: after.Budget, Items: after.Items}
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return structuredResult(out), nil, nil
}

type listArchivedDocsIn struct {
	Project string `json:"project,omitempty" jsonschema:"project slug, name, or id to scope to; 'global' for all projects, 'none' for unassigned; omit to use the current directory's project"`
	Type    string `json:"type,omitempty" jsonschema:"only this document type"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum number of results (default 50, max 200)"`
}

type listArchivedDocsResult struct {
	Scope string                    `json:"scope"`
	Items []curatedDocumentMetadata `json:"items"`
}

func filterArchivedDocuments(docs []domain.Document, nodeID *string, typ domain.DocumentType, limit int) []domain.Document {
	out := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if nodeID != nil {
			if *nodeID == "none" {
				if d.NodeID != nil {
					continue
				}
			} else if d.NodeID == nil || *d.NodeID != *nodeID {
				continue
			}
		}
		if typ != "" && d.Type != typ {
			continue
		}
		out = append(out, d)
		if len(out) == limit {
			break
		}
	}
	return out
}

func (h *handlers) listArchivedDocs(ctx context.Context, req *mcp.CallToolRequest, in listArchivedDocsIn) (*mcp.CallToolResult, any, error) {
	typ, err := checkType(in.Type)
	if err != nil {
		return h.resultErr(errGuard{err}), nil, nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultArchivedDocumentsLimit
	}
	if limit > maxContextReorderDocuments {
		return h.resultErr(errGuard{fmt.Errorf("limit must not exceed %d", maxContextReorderDocuments)}), nil, nil
	}
	var out listArchivedDocsResult
	err = h.do(ctx, req, func(c *apiclient.Client) error {
		sc, err := h.resolveScope(ctx, in.Project)
		if err != nil {
			return err
		}
		docs, err := c.ListArchived(ctx)
		if err != nil {
			return err
		}
		filtered := filterArchivedDocuments(docs, sc.nodeID, typ, limit)
		out = listArchivedDocsResult{Scope: sc.label, Items: make([]curatedDocumentMetadata, len(filtered))}
		for i, d := range filtered {
			out.Items[i] = archivedMetadataOf(d)
		}
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return structuredResult(out), nil, nil
}
