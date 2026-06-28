package usecase

import (
	"sort"

	"github.com/serverkraken/flow/internal/domain"
)

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
