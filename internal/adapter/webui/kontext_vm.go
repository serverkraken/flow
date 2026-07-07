package webui

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// KontextRowVM is one row of the Kuratieren rang list — one usecase.RankedItem
// reduced to display strings, in Compose's global fill order (no re-sort).
type KontextRowVM struct {
	DocID        string
	Num          int    // Included-Rank (usecase.RankedItem.Rank, 1-based, counts only included docs) — same source as usecase.StandingOf; 0 for dropped rows (rendered as "—", no number)
	Title        string
	ChipClass    string // DocTypeChipClass(Item.Type)
	TypeLabel    string // DocTypeLabel(Item.Type)
	ScopeLabel   string // "kind:name" (Item.ScopeLabel)
	TokensStr    string // fmtThousandsDE(Item.EstTokens)
	Pinned       bool
	Included     bool
	FirstDropped bool // true on the first row where Included flips true→false — the .cutline position
	IsFirst      bool // true on Rows[0] — disables the "Höher" button
	IsLast       bool // true on the last row — disables the "Tiefer" button
}

// KontextAlwaysVM is one row of the Always-Tier section (Instructions +
// ActiveContext) — not curatable, so it carries no rank/pin/reorder fields,
// only the same title/chip/scope/tokens display fields as KontextRowVM.
type KontextAlwaysVM struct {
	DocID      string
	Title      string
	ChipClass  string // DocTypeChipClass(Item.Type)
	TypeLabel  string // DocTypeLabel(Item.Type)
	ScopeLabel string // Item.ScopeLabel
	TokensStr  string // fmtThousandsDE(Item.EstTokens)
}

// KontextVM is the Kuratieren page's view model: the pagehead counters, the
// same budget-meter instrument as the cockpit rail (Pct/Full/UsedStr/CapStr —
// reuses BuildCockpitContext so the meter math + fmtThousandsDE formatting
// live in exactly one place), the Always-Tier section (Instructions +
// ActiveContext — Mini-Task 6b), and the flat rang list.
type KontextVM struct {
	NodeID    string
	Title     string // ShortName(n.Name)
	IncludedN int
	DroppedN  int
	UsedStr   string
	CapStr    string
	Pct       int
	Full      bool
	Always    []KontextAlwaysVM // Instructions + ActiveContext, not curatable — empty section must not render
	Rows      []KontextRowVM
	Err       string // set by the handler (not BuildKontextVM) on a Compose failure — .alert-err line
}

// BuildKontextVM reduces a composed context (usecase.Compose's output for
// n's chain) to the Kuratieren page's display VM. Pure: domain-/store-free.
func BuildKontextVM(n domain.Node, cc usecase.ComposedContext) KontextVM {
	meter := BuildCockpitContext(cc, n.ID)
	vm := KontextVM{
		NodeID:    n.ID,
		Title:     ShortName(n.Name),
		IncludedN: meter.IncludedN,
		DroppedN:  meter.DroppedN,
		UsedStr:   meter.UsedStr,
		CapStr:    meter.CapStr,
		Pct:       meter.Pct,
		Full:      meter.Full,
	}

	for _, it := range cc.Instructions {
		vm.Always = append(vm.Always, kontextAlwaysOf(it))
	}
	if cc.ActiveContext != nil {
		vm.Always = append(vm.Always, kontextAlwaysOf(*cc.ActiveContext))
	}

	n2 := len(cc.Ranked)
	firstDroppedSet := false
	for i, r := range cc.Ranked {
		row := KontextRowVM{
			DocID:      r.Item.ID,
			Num:        r.Rank,
			Title:      r.Item.Title,
			ChipClass:  DocTypeChipClass(r.Item.Type),
			TypeLabel:  DocTypeLabel(r.Item.Type),
			ScopeLabel: r.Item.ScopeLabel,
			TokensStr:  fmtThousandsDE(r.Item.EstTokens),
			Pinned:     r.Item.Pinned,
			Included:   r.Included,
			IsFirst:    i == 0,
			IsLast:     i == n2-1,
		}
		if !r.Included && !firstDroppedSet {
			row.FirstDropped = true
			firstDroppedSet = true
		}
		vm.Rows = append(vm.Rows, row)
	}
	return vm
}

// kontextAlwaysOf reduces one Always-Tier ContextItem (Instruction or
// ActiveContext) to its display VM.
func kontextAlwaysOf(item usecase.ContextItem) KontextAlwaysVM {
	return KontextAlwaysVM{
		DocID:      item.ID,
		Title:      item.Title,
		ChipClass:  DocTypeChipClass(item.Type),
		TypeLabel:  DocTypeLabel(item.Type),
		ScopeLabel: item.ScopeLabel,
		TokensStr:  fmtThousandsDE(item.EstTokens),
	}
}
