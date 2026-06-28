package usecase

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

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
	id, updated, err := uc.Docs.UpsertByPath(ctx, ownerID, &leaf.ID, domain.DocMemory, ActiveContextPath, title, body, false)
	if err != nil {
		return "", time.Time{}, err
	}
	if tags != nil {
		// Tag write after the entity write; a tag failure must not orphan the upsert.
		_, _ = uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, id, tags)
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
	Engagement int `json:"engagement"`
	Global     int `json:"global"`
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

// Compose classifies docs into tiers, ranks the relevance tier (pinned → newest),
// and fills until the token cap, counting dropped relevance items. Pure: no I/O.
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

	type ranked struct {
		item   ContextItem
		group  string
		pinned bool
		upd    string
	}
	var relevance []ranked

	for _, d := range docs {
		switch d.Type {
		case domain.DocInstruction:
			lbl := "global"
			if d.NodeID != nil {
				lbl = label[*d.NodeID]
			}
			out.Instructions = append(out.Instructions, itemOf(d, lbl))
		case domain.DocMemory:
			if d.NodeID == nil {
				if globalAllowed[d.ID] {
					it := itemOf(d, "global")
					relevance = append(relevance, ranked{it, "global", d.Pinned, it.UpdatedAt})
				}
				continue
			}
			nid := *d.NodeID
			switch tier[nid] {
			case "leaf":
				if d.Path == ActiveContextPath {
					it := itemOf(d, label[nid])
					out.ActiveContext = &it
				} else {
					out.Memories["leaf"] = append(out.Memories["leaf"], itemOf(d, label[nid]))
				}
			case "vorhaben":
				out.Memories["vorhaben"] = append(out.Memories["vorhaben"], itemOf(d, label[nid]))
			case "engagement":
				it := itemOf(d, label[nid])
				relevance = append(relevance, ranked{it, "engagement", d.Pinned, it.UpdatedAt})
			}
		}
	}

	// Always-tier into Used.
	for _, it := range out.Instructions {
		out.Budget.Used += it.EstTokens
	}
	if out.ActiveContext != nil {
		out.Budget.Used += out.ActiveContext.EstTokens
	}
	for _, g := range []string{"leaf", "vorhaben"} {
		for _, it := range out.Memories[g] {
			out.Budget.Used += it.EstTokens
		}
	}

	// Rank relevance: pinned first, then newest (UpdatedAt RFC3339 sorts lexicographically).
	sort.SliceStable(relevance, func(i, j int) bool {
		if relevance[i].pinned != relevance[j].pinned {
			return relevance[i].pinned
		}
		return relevance[i].upd > relevance[j].upd
	})
	for _, r := range relevance {
		if out.Budget.Used+r.item.EstTokens <= cap {
			out.Budget.Used += r.item.EstTokens
			out.Memories[r.group] = append(out.Memories[r.group], r.item)
		} else if r.group == "engagement" {
			out.Budget.Dropped.Engagement++
		} else {
			out.Budget.Dropped.Global++
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

var bootstrapTypes = []domain.DocumentType{domain.DocInstruction, domain.DocMemory}

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
