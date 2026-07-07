package webui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/usecase"
)

// ContextPinVM is one numbered pin row in the cockpit's "Kontext für Agenten"
// panel (Mockup Z.656–659): "01", Doc-Titel.
type ContextPinVM struct {
	Num, Title string
}

// CockpitContextVM is the cockpit rail's context-instrument panel: the
// composed agent-context budget for one node's chain, reduced to display
// strings/ints so CockpitRailBlocks stays logic-free.
type CockpitContextVM struct {
	NodeID  string // Kuratieren-Link-Ziel
	UsedStr string // "11.891" (Tausender-Punkt, de)
	CapStr  string // "12.000"
	Pct     int    // 0..100 (Meter-Breite), clamped
	Full    bool   // Pct >= 95 → Klasse "meter full" + warn-Notiz

	IncludedN int            // included Memories (len Ranked.Included) — Mockup "24 Docs"
	DroppedN  int            // Σ Budget.Dropped (Leaf+Vorhaben+Engagement+Global)
	PinnedN   int            // context-scoped: Ranked mit Item.Pinned (Offene Entscheidung #5)
	AlwaysN   int            // len(Instructions) + (1 wenn ActiveContext gesetzt) — zählt in Budget.Used, erscheint in keiner Ranked-Zeile (Mini-Task 6b)
	TopPins   []ContextPinVM // included && Pinned, top 3, nummeriert
}

// BuildCockpitContext reduces a ComposedContext (usecase.Compose's output for
// nodeID's chain) to the rail panel's display VM. Pure: domain-/store-free.
//
// PinnedN is context-scoped — every Ranked item (included OR dropped) whose
// Item.Pinned is true, counted within THIS node's composed chain. The mockup's
// "12" was the global corpus count; this node-scoped panel deliberately shows
// the chain's own pins instead (documented deviation, Offene Entscheidung #5).
//
// DroppedN sums the four tier buckets only (Leaf+Vorhaben+Engagement+Global);
// Budget.Dropped.Pinned is NOT added — it already double-counts an item that
// is also bucketed under its tier, so summing it in would inflate the total.
func BuildCockpitContext(cc usecase.ComposedContext, nodeID string) *CockpitContextVM {
	vm := &CockpitContextVM{
		NodeID:  nodeID,
		UsedStr: fmtThousandsDE(cc.Budget.Used),
		CapStr:  fmtThousandsDE(cc.Budget.Cap),
	}
	if cc.Budget.Cap > 0 {
		vm.Pct = cc.Budget.Used * 100 / cc.Budget.Cap
	}
	if vm.Pct > 100 {
		vm.Pct = 100
	}
	if vm.Pct < 0 {
		vm.Pct = 0
	}
	vm.Full = vm.Pct >= 95

	vm.DroppedN = cc.Budget.Dropped.Leaf + cc.Budget.Dropped.Vorhaben + cc.Budget.Dropped.Engagement + cc.Budget.Dropped.Global

	vm.AlwaysN = len(cc.Instructions)
	if cc.ActiveContext != nil {
		vm.AlwaysN++
	}

	for _, r := range cc.Ranked {
		if r.Included {
			vm.IncludedN++
		}
		if r.Item.Pinned {
			vm.PinnedN++
		}
		if r.Included && r.Item.Pinned && len(vm.TopPins) < 3 {
			vm.TopPins = append(vm.TopPins, ContextPinVM{
				Num:   fmt.Sprintf("%02d", len(vm.TopPins)+1),
				Title: r.Item.Title,
			})
		}
	}
	return vm
}

// fmtThousandsDE renders a non-negative int with "." as the thousands
// separator (de-DE convention, e.g. 11891 → "11.891"). No such helper exists
// yet in this package (verified: `rg -n "func Fmt.*int|thousand|de-DE" internal/adapter/webui`).
func fmtThousandsDE(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	return out
}
