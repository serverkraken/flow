package usecase

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

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
	Resolve   ResolveNode
	Nodes     ports.NodeStore
	Docs      ports.DocumentStore
	Aggregate ports.DocumentAggregateStore
	Tags      ports.TagStore
}

// Execute resolves the leaf (NodeOverride slug lookup OR Resolve.Execute),
// returns ErrContextUnresolved when no bound node is found, then atomically
// upserts the active-context memory doc and its links/tags when Aggregate is
// configured.
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
	if uc.Aggregate != nil {
		var tagChanges *[]string
		if tags != nil {
			tagValues := tags
			tagChanges = &tagValues
		}
		doc, err := uc.Aggregate.UpsertDocumentAggregate(ctx, ports.DocumentAggregateUpsert{
			OwnerID:       ownerID,
			NodeID:        &leaf.ID,
			Type:          domain.DocActiveContext,
			Path:          ActiveContextPath,
			Title:         title,
			Body:          body,
			UpdatedByKind: string(a.Kind),
			UpdatedByRef:  a.Ref,
			Changes: ports.DocumentAggregateChanges{
				Links: domain.WikilinkTargets(body),
				Tags:  tagChanges,
			},
		})
		if err != nil {
			return "", time.Time{}, err
		}
		return doc.ID, doc.UpdatedAt, nil
	}
	id, updated, err := uc.Docs.UpsertByPath(ctx, ownerID, &leaf.ID, domain.DocActiveContext, ActiveContextPath, title, body, false, false, string(a.Kind), a.Ref)
	if err != nil {
		return "", time.Time{}, err
	}
	if tags != nil {
		// Legacy fallback preserves the historical best-effort behavior.
		if _, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, id, tags); err != nil {
			slog.WarnContext(ctx, "set active-context tags", "id", id, "err", err)
		}
	}
	return id, updated, nil
}

// ActiveContextPath is the fixed path of the per-leaf activeContext memory doc.
const ActiveContextPath = "active-context"

type ContextItem struct {
	ID          string              `json:"id"`
	NodeID      *string             `json:"nodeId"`
	ScopeLabel  string              `json:"scope"`
	Type        domain.DocumentType `json:"type"`
	Path        string              `json:"path"`
	Title       string              `json:"title"`
	Tags        []string            `json:"tags,omitempty"`
	UpdatedAt   string              `json:"updatedAt"`
	Pinned      bool                `json:"pinned"`
	Priority    int                 `json:"priority,omitempty"`
	EstTokens   int                 `json:"estTokens"`
	Body        string              `json:"body"`
	Truncated   bool                `json:"truncated,omitempty"`
	ContextMode domain.ContextMode  `json:"contextMode,omitempty"`
}

// RankedItem is one entry of the flat, globally-ordered ranking pool that
// Compose exposes alongside the tiered Memories map. It is the single source
// for the curation list, the meter counters, and the per-document rank
// ("04/24") — additive to Memories/Dropped, never a replacement.
type RankedItem struct {
	Item     ContextItem `json:"item"`
	Group    string      `json:"group"`
	Included bool        `json:"included"`
	Rank     int         `json:"rank"`
}

// ContextStanding is the per-document answer to "where does this doc stand in
// the composed context" — used by the curation UI to render a document's
// rank/dropped/always/absent state without recomputing Compose.
type ContextStanding struct {
	State      string `json:"state"` // "included" | "dropped" | "always" | "absent"
	Rank       int    `json:"rank,omitempty"`
	Total      int    `json:"total,omitempty"`
	ScopeLabel string `json:"scope,omitempty"`
}

type DroppedCount struct {
	Leaf         int `json:"leaf"`
	Vorhaben     int `json:"vorhaben"`
	Engagement   int `json:"engagement"`
	Global       int `json:"global"`
	Pinned       int `json:"pinned"`
	Instructions int `json:"instructions,omitempty"`
	Always       int `json:"always,omitempty"`
}

type ContextResolution struct {
	Repo       *domain.Node  `json:"repo"`
	Chain      []domain.Node `json:"chain"`
	Unresolved bool          `json:"unresolved"`
}

type ContextBudget struct {
	Used         int          `json:"used"`
	Cap          int          `json:"cap"`
	Dropped      DroppedCount `json:"dropped"`
	Deduplicated int          `json:"deduplicated,omitempty"`
}

type ComposedContext struct {
	Resolution     ContextResolution        `json:"resolution"`
	Profile        ContextProfile           `json:"profile,omitempty"`
	Instructions   []ContextItem            `json:"instructions"`
	ActiveContext  *ContextItem             `json:"activeContext"`
	Memories       map[string][]ContextItem `json:"memories"`
	Ranked         []RankedItem             `json:"ranked,omitempty"`
	AlwaysMemories []ContextItem            `json:"alwaysMemories,omitempty"`
	Hidden         []ContextItem            `json:"hidden,omitempty"`
	Candidates     []ContextItem            `json:"candidates,omitempty"`
	Budget         ContextBudget            `json:"budget"`
}

type ContextProfile string

const (
	ContextProfileHandoff  ContextProfile = "handoff"
	ContextProfileStandard ContextProfile = "standard"
	ContextProfileFull     ContextProfile = "full"
)

var ErrInvalidContextProfile = errors.New("usecase: invalid context profile")

// ApplyContextProfile shapes one already hard-budgeted context for its caller.
// Handoff is intentionally small and deterministic; standard omits curation
// diagnostics; full retains the complete metadata view used for audits.
func ApplyContextProfile(cc ComposedContext, raw string) (ComposedContext, error) {
	profile := ContextProfile(strings.TrimSpace(raw))
	if profile == "" {
		profile = ContextProfileStandard
	}
	cc.Profile = profile
	switch profile {
	case ContextProfileHandoff:
		cc.AlwaysMemories = nil
		cc.Memories = map[string][]ContextItem{}
		cc.Ranked = nil
		cc.Hidden = nil
		cc.Candidates = nil
		cc.Budget.Used = 0
		if cc.ActiveContext != nil {
			cc.Budget.Used += cc.ActiveContext.EstTokens
		}
		for _, it := range cc.Instructions {
			cc.Budget.Used += it.EstTokens
		}
	case ContextProfileStandard:
		cc.Ranked = nil
		cc.Hidden = nil
		cc.Candidates = nil
	case ContextProfileFull:
		// Keep all metadata. Ranked and Hidden bodies were already stripped by
		// Compose, so full does not duplicate content outside the budget.
	default:
		return ComposedContext{}, ErrInvalidContextProfile
	}
	return cc, nil
}

func estTokens(body string) int { return (len(body) + 3) / 4 }

func itemOf(d domain.Document, label string) ContextItem {
	return ContextItem{
		ID: d.ID, NodeID: d.NodeID, ScopeLabel: label, Type: d.Type, Path: d.Path, Title: d.Title, Tags: d.Tags,
		UpdatedAt: d.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"), Pinned: d.Pinned,
		Priority: d.Priority, EstTokens: estTokens(d.Body), Body: d.Body,
		ContextMode: d.ContextMode.OrAuto(),
	}
}

// metadataItem removes body content from diagnostic-only copies such as Ranked
// and Hidden. The canonical included item keeps its body; diagnostics must not
// duplicate or leak content outside the token budget.
func metadataItem(it ContextItem) ContextItem {
	it.Body = ""
	return it
}

// fitContextItem returns an item that consumes at most remaining estimated
// tokens. Priority-tier items (active context, instructions and immer memories)
// may be truncated so the hard budget remains true even when one item alone is
// larger than the cap.
func fitContextItem(it ContextItem, remaining int) (ContextItem, bool) {
	if remaining <= 0 {
		return ContextItem{}, false
	}
	if it.EstTokens <= remaining {
		return it, true
	}
	maxBytes := remaining * 4
	if maxBytes > len(it.Body) {
		maxBytes = len(it.Body)
	}
	for maxBytes > 0 && !utf8.ValidString(it.Body[:maxBytes]) {
		maxBytes--
	}
	if maxBytes == 0 {
		return ContextItem{}, false
	}
	it.Body = it.Body[:maxBytes]
	it.EstTokens = estTokens(it.Body)
	it.Truncated = true
	return it, it.EstTokens > 0 && it.EstTokens <= remaining
}

func instructionRank(it ContextItem, tier map[string]string) int {
	if it.NodeID == nil {
		return 3
	}
	switch tier[*it.NodeID] {
	case "leaf":
		return 0
	case "vorhaben":
		return 1
	case "engagement":
		return 2
	default:
		return 3
	}
}

func instructionModeRank(mode domain.ContextMode) int {
	if mode.OrAuto() == domain.ContextModeImmer {
		return 0
	}
	return 1
}

// dedupeInstructions prefers the most specific scope when identical
// instruction bodies exist globally and on the repo chain.
func dedupeInstructions(items []ContextItem, tier map[string]string) ([]ContextItem, int) {
	sort.SliceStable(items, func(i, j int) bool {
		leftScope, rightScope := instructionRank(items[i], tier), instructionRank(items[j], tier)
		if leftScope != rightScope {
			return leftScope < rightScope
		}
		return instructionModeRank(items[i].ContextMode) < instructionModeRank(items[j].ContextMode)
	})
	seen := make(map[string]bool, len(items))
	out := make([]ContextItem, 0, len(items))
	deduped := 0
	for _, it := range items {
		key := strings.TrimSpace(it.Body)
		if seen[key] {
			deduped++
			continue
		}
		seen[key] = true
		out = append(out, it)
	}
	return out, deduped
}

func clientFamily(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	for _, family := range []string{"codex", "claude", "gemini"} {
		if strings.Contains(normalized, family) {
			return family
		}
	}
	return normalized
}

func instructionClient(path string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(path)), "/")
	if len(parts) == 0 {
		return ""
	}
	switch parts[0] {
	case "codex", "claude", "gemini":
		return parts[0]
	case "clients":
		if len(parts) > 1 {
			return parts[1]
		}
	}
	return ""
}

func instructionMatchesClient(d domain.Document, client string) bool {
	target := instructionClient(d.Path)
	if target == "" || strings.TrimSpace(client) == "" {
		return true
	}
	return target == clientFamily(client)
}

// Compose classifies docs, ranks all memories into one pool
// (pinned desc, priority desc, tierRank asc, updatedAt desc), and fills until
// the token cap. Priority is a manual curation override: it lifts a doc's
// fill order within/across tiers but does NOT bypass the cap (only pinned
// docs do that, via first-fill priority alone — a too-large pin still drops).
// Default priority 0 is behavior-neutral: with every doc at 0 the key
// degenerates to the pre-L5 (pinned, tierRank, updatedAt) order.
// Active context is filled first, followed by deduplicated instructions and
// immer memories; these priority tiers may be truncated but never exceed the
// hard cap. A pinned global memory bypasses the D7 tag-gate. Pure: no I/O.
func Compose(chain []domain.Node, docs []domain.Document, globalAllowed map[string]bool, cap int) ComposedContext {
	return ComposeForClient(chain, docs, globalAllowed, cap, "")
}

// ComposeForClient composes context for one agent client. Global instructions
// under a client-qualified path (for example codex/... or claude/...) are only
// included for that client; repo-scoped and generic global instructions remain
// shared. An empty client preserves the complete administrative/WebUI view.
func ComposeForClient(chain []domain.Node, docs []domain.Document, globalAllowed map[string]bool, cap int, client string) ComposedContext {
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
		prio   int // manual curation override (d.Priority); higher fills first
		rank   int
		upd    string
	}
	var pool []ranked

	for _, d := range docs {
		if d.Archived {
			continue
		}
		if d.Type == domain.DocInstruction && d.NodeID == nil && !instructionMatchesClient(d, client) {
			continue
		}
		switch d.Type {
		case domain.DocInstruction, domain.DocMemory, domain.DocActiveContext:
		default:
			continue
		}
		candidateLabel := "global"
		if d.NodeID != nil {
			var ok bool
			candidateLabel, ok = label[*d.NodeID]
			if !ok {
				continue
			}
		}
		out.Candidates = append(out.Candidates, metadataItem(itemOf(d, candidateLabel)))
		mode := d.ContextMode.OrAuto()
		if mode == domain.ContextModeNie {
			// Never composed. Collect for the Kuratieren restore affordance only
			// (node-in-chain OR global). Not in Used/Ranked/Memories/Always.
			lbl := "global"
			if d.NodeID != nil {
				if l, ok := label[*d.NodeID]; ok {
					lbl = l
				} else {
					continue // node not in chain — not this chain's concern
				}
			}
			out.Hidden = append(out.Hidden, metadataItem(itemOf(d, lbl)))
			continue
		}
		if mode == domain.ContextModeImmer {
			// Forced priority tier regardless of type/tag-gate/pin. It is budgeted
			// before pooled memories but remains subject to the hard cap.
			lbl := "global"
			if d.NodeID != nil {
				if l, ok := label[*d.NodeID]; ok {
					lbl = l
				} else {
					continue // node not in chain
				}
			}
			it := itemOf(d, lbl)
			switch d.Type {
			case domain.DocInstruction:
				out.Instructions = append(out.Instructions, it)
			case domain.DocActiveContext:
				if d.NodeID != nil && tier[*d.NodeID] == "leaf" {
					out.ActiveContext = &it
				} else {
					out.AlwaysMemories = append(out.AlwaysMemories, it)
				}
			default: // memory
				out.AlwaysMemories = append(out.AlwaysMemories, it)
			}
			continue
		}
		// mode == auto: exact Bestand logic, unverändert.
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
					pool = append(pool, ranked{it, "global", d.Pinned, it.Priority, rankOf["global"], it.UpdatedAt})
				}
				continue
			}
			g := tier[*d.NodeID]
			if g == "" {
				continue // node not in chain (defensive)
			}
			it := itemOf(d, label[*d.NodeID])
			pool = append(pool, ranked{it, g, d.Pinned, it.Priority, rankOf[g], it.UpdatedAt})
		}
	}

	// Hard-cap priority: active context first, then repo-specific instructions,
	// then immer memories. None of these tiers may make Used exceed Cap.
	active := out.ActiveContext
	instructions, deduplicated := dedupeInstructions(out.Instructions, tier)
	always := out.AlwaysMemories
	out.ActiveContext = nil
	out.Instructions = nil
	out.AlwaysMemories = nil
	out.Budget.Deduplicated = deduplicated
	if active != nil {
		if fitted, ok := fitContextItem(*active, cap-out.Budget.Used); ok {
			out.ActiveContext = &fitted
			out.Budget.Used += fitted.EstTokens
		}
	}
	for _, it := range instructions {
		fitted, ok := fitContextItem(it, cap-out.Budget.Used)
		if !ok {
			out.Budget.Dropped.Instructions++
			continue
		}
		out.Instructions = append(out.Instructions, fitted)
		out.Budget.Used += fitted.EstTokens
	}
	for _, it := range always {
		fitted, ok := fitContextItem(it, cap-out.Budget.Used)
		if !ok {
			out.Budget.Dropped.Always++
			continue
		}
		out.AlwaysMemories = append(out.AlwaysMemories, fitted)
		out.Budget.Used += fitted.EstTokens
	}

	// Rank: pinned first, then priority (manual override), then tierRank
	// (global→engagement→vorhaben→leaf), then newest.
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].pinned != pool[j].pinned {
			return pool[i].pinned
		}
		if pool[i].prio != pool[j].prio {
			return pool[i].prio > pool[j].prio // higher priority fills first
		}
		if pool[i].rank != pool[j].rank {
			return pool[i].rank < pool[j].rank
		}
		return pool[i].upd > pool[j].upd
	})
	incl := 0
	for _, r := range pool {
		if out.Budget.Used+r.item.EstTokens <= cap {
			out.Budget.Used += r.item.EstTokens
			out.Memories[r.group] = append(out.Memories[r.group], r.item)
			incl++
			out.Ranked = append(out.Ranked, RankedItem{Item: metadataItem(r.item), Group: r.group, Included: true, Rank: incl})
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
		out.Ranked = append(out.Ranked, RankedItem{Item: metadataItem(r.item), Group: r.group, Included: false, Rank: 0})
	}
	return out
}

// ContextResolveInput carries the resolution hints from the client.
type ContextResolveInput struct {
	RemoteSlug   string
	MachineID    string
	Cwd          string
	NodeOverride string // explicit node slug; bypasses binding resolution
	Client       string // agent client name used to select global instructions
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
		return ComposeForClient(nil, docs, map[string]bool{}, cap, in.Client), nil
	}

	chain, err := uc.Nodes.Ancestors(ctx, ownerID, leaf.ID)
	if err != nil {
		return ComposedContext{}, err
	}
	return uc.composeForChain(ctx, ownerID, chain, cap, in.Client)
}

// ExecuteForNode composes the context of a node addressed by ID (not slug —
// slugs are only sibling-unique). Mirrors the cockpit chain assembly.
func (uc ComposeContext) ExecuteForNode(ctx context.Context, ownerID, nodeID string, cap int) (ComposedContext, error) {
	leaf, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return ComposedContext{}, err
	}
	chain, err := uc.Nodes.Ancestors(ctx, ownerID, leaf.ID)
	if err != nil {
		return ComposedContext{}, err
	}
	if len(chain) == 0 || chain[0].ID != leaf.ID {
		chain = append([]domain.Node{leaf}, chain...)
	}
	return uc.composeForChain(ctx, ownerID, chain, cap, "")
}

// composeForChain is the post-resolve tail shared by Execute and
// ExecuteForNode: gather docs for the chain, apply the D7 tag-gate for
// global memories, and call the pure Compose function.
func (uc ComposeContext) composeForChain(ctx context.Context, ownerID string, chain []domain.Node, cap int, client string) (ComposedContext, error) {
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
	return ComposeForClient(chain, docs, allowed, cap, client), nil
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

// StandingOf is the single source for a document's standing within an already
// composed context: "always" for instructions/activeContext (never in the
// pool/Ranked), "included"/"dropped" for a pooled memory (Rank/Total only set
// for "included" — Total is the count of included memories), or "absent" if
// the doc does not appear in cc at all. Pure: reads only cc.
func StandingOf(cc ComposedContext, docID string) ContextStanding {
	for _, it := range cc.Instructions {
		if it.ID == docID {
			return ContextStanding{State: "always", ScopeLabel: it.ScopeLabel}
		}
	}
	if cc.ActiveContext != nil && cc.ActiveContext.ID == docID {
		return ContextStanding{State: "always", ScopeLabel: cc.ActiveContext.ScopeLabel}
	}
	for _, it := range cc.AlwaysMemories {
		if it.ID == docID {
			return ContextStanding{State: "always", ScopeLabel: it.ScopeLabel}
		}
	}
	total := 0
	for _, r := range cc.Ranked {
		if r.Included {
			total++
		}
	}
	for _, r := range cc.Ranked {
		if r.Item.ID == docID {
			if r.Included {
				return ContextStanding{State: "included", Rank: r.Rank, Total: total, ScopeLabel: r.Item.ScopeLabel}
			}
			return ContextStanding{State: "dropped", ScopeLabel: r.Item.ScopeLabel}
		}
	}
	return ContextStanding{State: "absent"}
}
