package usecase

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ErrContextUnresolved means the cwd/remote did not resolve to a bound node, so there
// is nowhere to write the activeContext (the human must `flow node bind` first).
var ErrContextUnresolved = errors.New("usecase: context not resolved (bind the repo first)")

// SetActiveContext writes (or upserts) an active-context memory doc at the
// resolved leaf node. It mirrors how ComposeContext resolves a leaf, then calls
// Docs.UpsertByPath at the fixed ActiveContextPath.
type SetActiveContext struct {
	Resolve ResolveNode
	Nodes   ports.NodeStore
	Docs    ports.DocumentStore
	Tags    ports.TagStore
}

// Execute resolves the leaf (NodeOverride slug lookup OR Resolve.Execute),
// returns ErrContextUnresolved when no bound node is found, then upserts the
// active-context memory doc. Tag writes happen after the upsert; a tag error
// is swallowed to avoid orphaning the successful upsert.
func (uc SetActiveContext) Execute(ctx context.Context, ownerID string, in ContextResolveInput, title, body string, tags []string) (string, time.Time, error) {
	var leaf domain.Node
	var ok bool
	var err error
	if in.NodeOverride != "" {
		all, e := uc.Nodes.List(ctx, ownerID)
		if e != nil {
			return "", time.Time{}, e
		}
		for _, n := range all {
			if n.Slug == in.NodeOverride {
				leaf, ok = n, true
				break
			}
		}
	} else {
		leaf, ok, err = uc.Resolve.Execute(ctx, ownerID, in.RemoteSlug, in.MachineID, in.Cwd)
	}
	if err != nil {
		return "", time.Time{}, err
	}
	if !ok {
		return "", time.Time{}, ErrContextUnresolved
	}
	if strings.TrimSpace(title) == "" {
		title = "Active Context"
	}
	a := actor.FromContext(ctx)
	id, updated, err := uc.Docs.UpsertByPath(ctx, ownerID, &leaf.ID, domain.DocActiveContext, ActiveContextPath, title, body, false, false, string(a.Kind), a.Ref)
	if err != nil {
		return "", time.Time{}, err
	}
	if tags != nil {
		// Tag write after the entity write; a tag failure must not orphan the upsert.
		if _, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, id, tags); err != nil {
			slog.WarnContext(ctx, "set active-context tags", "id", id, "err", err)
		}
	}
	return id, updated, nil
}

// ActiveContextPath is the fixed path of the per-leaf activeContext memory doc.
const ActiveContextPath = "active-context"

type ContextItem struct {
	ID         string              `json:"id"`
	NodeID     *string             `json:"nodeId"`
	ScopeLabel string              `json:"scope"`
	Type       domain.DocumentType `json:"type"`
	Tags       []string            `json:"tags,omitempty"`
	UpdatedAt  string              `json:"updatedAt"`
	Pinned     bool                `json:"pinned"`
	EstTokens  int                 `json:"estTokens"`
	Body       string              `json:"body"`
}

type DroppedCount struct {
	Leaf       int `json:"leaf"`
	Vorhaben   int `json:"vorhaben"`
	Engagement int `json:"engagement"`
	Global     int `json:"global"`
	Pinned     int `json:"pinned"`
}

type ContextResolution struct {
	Repo       *domain.Node  `json:"repo"`
	Chain      []domain.Node `json:"chain"`
	Unresolved bool          `json:"unresolved"`
}

type ContextBudget struct {
	Used    int          `json:"used"`
	Cap     int          `json:"cap"`
	Dropped DroppedCount `json:"dropped"`
}

type ComposedContext struct {
	Resolution    ContextResolution        `json:"resolution"`
	Instructions  []ContextItem            `json:"instructions"`
	ActiveContext *ContextItem             `json:"activeContext"`
	Memories      map[string][]ContextItem `json:"memories"`
	Budget        ContextBudget            `json:"budget"`
}

func estTokens(body string) int { return (len(body) + 3) / 4 }

func itemOf(d domain.Document, label string) ContextItem {
	return ContextItem{
		ID: d.ID, NodeID: d.NodeID, ScopeLabel: label, Type: d.Type, Tags: d.Tags,
		UpdatedAt: d.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"), Pinned: d.Pinned,
		EstTokens: estTokens(d.Body), Body: d.Body,
	}
}

// Compose classifies docs, ranks all memories into one pool
// (pinned desc, tierRank asc, updatedAt desc), and fills until the token cap.
// instructions + activeContext are always-tier (counted, never dropped). A pinned
// global memory bypasses the D7 tag-gate. Pure: no I/O.
func Compose(chain []domain.Node, docs []domain.Document, globalAllowed map[string]bool, cap int) ComposedContext {
	out := ComposedContext{Memories: map[string][]ContextItem{}}
	out.Budget.Cap = cap
	if len(chain) > 0 {
		repo := chain[0]
		out.Resolution.Repo = &repo
		out.Resolution.Chain = chain
	} else {
		out.Resolution.Unresolved = true
	}

	// node-id → scope label + tier classification from the chain.
	label := map[string]string{}
	tier := map[string]string{} // "leaf" | "vorhaben" | "engagement"
	for i, n := range chain {
		label[n.ID] = string(n.Kind) + ":" + n.Name
		switch {
		case i == 0:
			tier[n.ID] = "leaf"
		case i == len(chain)-1 && n.Kind == domain.KindEngagement:
			tier[n.ID] = "engagement"
		default:
			tier[n.ID] = "vorhaben"
		}
	}

	// tierRank: lower fills first among equally-pinned items.
	rankOf := map[string]int{"global": 0, "engagement": 1, "vorhaben": 2, "leaf": 3}

	type ranked struct {
		item   ContextItem
		group  string
		pinned bool
		rank   int
		upd    string
	}
	var pool []ranked

	for _, d := range docs {
		switch d.Type {
		case domain.DocInstruction:
			lbl := "global"
			if d.NodeID != nil {
				lbl = label[*d.NodeID]
			}
			out.Instructions = append(out.Instructions, itemOf(d, lbl))
		case domain.DocActiveContext:
			if d.NodeID != nil && tier[*d.NodeID] == "leaf" {
				it := itemOf(d, label[*d.NodeID])
				out.ActiveContext = &it
			}
		case domain.DocMemory:
			if d.NodeID == nil {
				if globalAllowed[d.ID] || d.Pinned { // pinned bypasses the D7 tag-gate
					it := itemOf(d, "global")
					pool = append(pool, ranked{it, "global", d.Pinned, rankOf["global"], it.UpdatedAt})
				}
				continue
			}
			g := tier[*d.NodeID]
			if g == "" {
				continue // node not in chain (defensive)
			}
			it := itemOf(d, label[*d.NodeID])
			pool = append(pool, ranked{it, g, d.Pinned, rankOf[g], it.UpdatedAt})
		}
	}

	// Always-tier (uncapped): instructions + activeContext into Used.
	for _, it := range out.Instructions {
		out.Budget.Used += it.EstTokens
	}
	if out.ActiveContext != nil {
		out.Budget.Used += out.ActiveContext.EstTokens
	}

	// Rank: pinned first, then tierRank (global→engagement→vorhaben→leaf), then newest.
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].pinned != pool[j].pinned {
			return pool[i].pinned
		}
		if pool[i].rank != pool[j].rank {
			return pool[i].rank < pool[j].rank
		}
		return pool[i].upd > pool[j].upd
	})
	for _, r := range pool {
		if out.Budget.Used+r.item.EstTokens <= cap {
			out.Budget.Used += r.item.EstTokens
			out.Memories[r.group] = append(out.Memories[r.group], r.item)
			continue
		}
		switch r.group {
		case "leaf":
			out.Budget.Dropped.Leaf++
		case "vorhaben":
			out.Budget.Dropped.Vorhaben++
		case "engagement":
			out.Budget.Dropped.Engagement++
		case "global":
			out.Budget.Dropped.Global++
		}
		if r.pinned {
			out.Budget.Dropped.Pinned++
		}
	}
	return out
}

// ContextResolveInput carries the resolution hints from the client.
type ContextResolveInput struct {
	RemoteSlug   string
	MachineID    string
	Cwd          string
	NodeOverride string // explicit node slug; bypasses binding resolution
}

// ComposeContext orchestrates resolution, doc gathering, tag-gating, and Compose.
type ComposeContext struct {
	Resolve ResolveNode
	Nodes   ports.NodeStore
	Docs    ports.DocumentStore
	Tags    ports.TagStore
}

var bootstrapTypes = []domain.DocumentType{domain.DocInstruction, domain.DocMemory, domain.DocActiveContext}

// Execute resolves the leaf node, walks its ancestor chain, gathers docs, applies
// the D7 tag-gate for global memories, and calls the pure Compose function.
func (uc ComposeContext) Execute(ctx context.Context, ownerID string, in ContextResolveInput, cap int) (ComposedContext, error) {
	leaf, ok, err := uc.resolveLeaf(ctx, ownerID, in)
	if err != nil {
		return ComposedContext{}, err
	}
	if !ok {
		// Unresolved: serve global docs only; no active-node tags so no global memories cross.
		docs, err := uc.Docs.ListForContext(ctx, ownerID, nil, true, bootstrapTypes)
		if err != nil {
			return ComposedContext{}, err
		}
		return Compose(nil, docs, map[string]bool{}, cap), nil
	}

	chain, err := uc.Nodes.Ancestors(ctx, ownerID, leaf.ID)
	if err != nil {
		return ComposedContext{}, err
	}
	chainIDs := make([]string, len(chain))
	for i, n := range chain {
		chainIDs[i] = n.ID
	}
	docs, err := uc.Docs.ListForContext(ctx, ownerID, chainIDs, true, bootstrapTypes)
	if err != nil {
		return ComposedContext{}, err
	}

	// D7 tag-gate: global memories cross only if they carry one of the chain's node tags.
	allowed, err := uc.globalAllowed(ctx, ownerID, chainIDs)
	if err != nil {
		return ComposedContext{}, err
	}
	return Compose(chain, docs, allowed, cap), nil
}

// resolveLeaf returns the leaf node using either an explicit slug override or the
// binding registry (remote slug → machine ID → cwd longest-prefix).
func (uc ComposeContext) resolveLeaf(ctx context.Context, ownerID string, in ContextResolveInput) (domain.Node, bool, error) {
	if in.NodeOverride != "" {
		all, err := uc.Nodes.List(ctx, ownerID)
		if err != nil {
			return domain.Node{}, false, err
		}
		for _, n := range all {
			if n.Slug == in.NodeOverride {
				return n, true, nil
			}
		}
		return domain.Node{}, false, nil
	}
	return uc.Resolve.Execute(ctx, ownerID, in.RemoteSlug, in.MachineID, in.Cwd)
}

// globalAllowed computes the set of global-document IDs permitted to cross into
// the composed context based on the union of the chain's node tags (D7 gate).
func (uc ComposeContext) globalAllowed(ctx context.Context, ownerID string, chainIDs []string) (map[string]bool, error) {
	tagsByNode, err := uc.Tags.TagsForMany(ctx, ownerID, domain.TaggableNode, chainIDs)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var slugs []string
	for _, ts := range tagsByNode {
		for _, t := range ts {
			if !seen[t.Slug] {
				seen[t.Slug] = true
				slugs = append(slugs, t.Slug)
			}
		}
	}
	allowed := map[string]bool{}
	if len(slugs) == 0 {
		return allowed, nil
	}
	ids, err := uc.Tags.FilterIDs(ctx, ownerID, domain.TaggableDocument, slugs, domain.TagMatchAny)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		allowed[id] = true
	}
	return allowed, nil
}
