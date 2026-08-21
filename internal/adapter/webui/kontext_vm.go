package webui

import (
	"html/template"
	"time"

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
	// Mode is r.Item.ContextMode — constructively always "auto" (only
	// auto-mode docs ever enter the ranked pool; immer/nie bypass it
	// entirely, Task 2 Compose semantics), but the mode switcher still reads
	// it to render its pressed segment and to promote/demote the doc.
	Mode domain.ContextMode
	// Screen 07: jede Zeile ist wählbar — die Lesespalte zeigt die gewählte
	// Karte; Selected markiert die Zeile.
	Selected bool
}

// KontextAlwaysVM is one row of the Always-Tier section (Instructions +
// ActiveContext + immer-mode memories, Task 4) — not curatable via pin/
// reorder, so it carries no rank/pin/reorder fields, only the same title/
// chip/scope/tokens display fields as KontextRowVM plus the mode switcher
// (so an Instruction/ActiveContext can still be demoted to "nie", and an
// immer-memory's pressed segment reflects its forced mode).
type KontextAlwaysVM struct {
	DocID      string
	Title      string
	ChipClass  string // DocTypeChipClass(Item.Type)
	TypeLabel  string // DocTypeLabel(Item.Type)
	ScopeLabel string // Item.ScopeLabel
	TokensStr  string // fmtThousandsDE(Item.EstTokens)
	Mode       domain.ContextMode
	Selected   bool
}

// KontextHiddenVM is one row of the new "Ausgeblendet (nie)" section (Task
// 4): a nie-mode doc, collected by Compose purely for this restore
// affordance (cc.Hidden, 0 extra queries) — never in Used/Ranked/Memories/
// Always. Carries no pin/reorder actions (not curatable there either), only
// the mode switcher (the restore path — flipping back to auto/immer moves it
// back into the composed context in-place) and an editor link.
type KontextHiddenVM struct {
	DocID      string
	Title      string
	ChipClass  string
	TypeLabel  string
	ScopeLabel string
	TokensStr  string
	Mode       domain.ContextMode
	Selected   bool
}

// KontextLeseVM ist die Lesespalte von Screen 07: die gewählte Karte mit
// ihrem Stand im Kontext, ihrer Herkunft und ihrem gerenderten Inhalt —
// lesen und bearbeiten rechts, kuratieren links.
type KontextLeseVM struct {
	DocID, Title, Path   string
	ChipClass, TypeLabel string
	StatusKey            string // document.context.{in,always,dropped,hidden}
	Rank, Total          int    // nur bei StatusKey = document.context.in
	Actor, When          string
	TokensStr            string
	SharePct             int // Anteil am Budget, gerundet
	ReadMinutes          int
	Mode                 domain.ContextMode
	HTML                 template.HTML
	EditHref, OpenHref   string
}

// KontextVM is the Kuratieren page's view model: the pagehead counters, the
// same budget-meter instrument as the cockpit rail (Pct/Full/UsedStr/CapStr —
// reuses BuildCockpitContext so the meter math + fmtThousandsDE formatting
// live in exactly one place), the Always-Tier section (Instructions +
// ActiveContext + immer memories — Mini-Task 6b, extended by Task 4), the
// flat rang list, and the new Hidden (nie) section.
type KontextVM struct {
	NodeID    string
	Title     string // ShortName(n.Name)
	IncludedN int
	DroppedN  int
	UsedStr   string
	CapStr    string
	FreeStr   string // Cap − Used, nie negativ
	Pct       int
	Full      bool
	Always    []KontextAlwaysVM // Instructions + ActiveContext + immer memories — empty section must not render
	Rows      []KontextRowVM
	Hidden    []KontextHiddenVM // nie-mode docs (cc.Hidden) — empty section must not render
	Err       string            // set by the handler (not BuildKontextVM) on a Compose failure — .alert-err line
	// Screen 07 — kuratieren links, lesen und bearbeiten rechts.
	EbeneColor string         // Ton des Registers, der 3px-Streifen
	Selected   string         // gewählte Karte (Select); "" = nichts wählbar
	SelQuery   string         // "?doc=<id>" für Seiten- und Fragment-Links, "" ohne Wahl
	ActQuery   string         // "?sel=<id>" für die Aktions-POSTs, damit die Wahl den Tausch überlebt
	Lese       *KontextLeseVM // die Lesespalte; nil = nichts gewählt
}

// BuildKontextVM reduces a composed context (usecase.Compose's output for
// n's chain) to the Kuratieren page's display VM. Pure: domain-/store-free.
func BuildKontextVM(n domain.Node, cc usecase.ComposedContext) KontextVM {
	meter := BuildCockpitContext(cc, n.ID)
	free := cc.Budget.Cap - cc.Budget.Used
	if free < 0 {
		free = 0
	}
	vm := KontextVM{
		NodeID:     n.ID,
		Title:      ShortName(n.Name),
		IncludedN:  meter.IncludedN,
		DroppedN:   meter.DroppedN,
		UsedStr:    meter.UsedStr,
		CapStr:     meter.CapStr,
		FreeStr:    fmtThousandsDE(free),
		Pct:        meter.Pct,
		Full:       meter.Full,
		EbeneColor: EbeneAccentColor(n.Kind),
	}

	for _, it := range cc.Instructions {
		vm.Always = append(vm.Always, kontextAlwaysOf(it))
	}
	if cc.ActiveContext != nil {
		vm.Always = append(vm.Always, kontextAlwaysOf(*cc.ActiveContext))
	}
	for _, it := range cc.AlwaysMemories {
		vm.Always = append(vm.Always, kontextAlwaysOf(it))
	}
	for _, it := range cc.Hidden {
		vm.Hidden = append(vm.Hidden, kontextHiddenOf(it))
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
			Mode:       r.Item.ContextMode,
		}
		if !r.Included && !firstDroppedSet {
			row.FirstDropped = true
			firstDroppedSet = true
		}
		vm.Rows = append(vm.Rows, row)
	}
	return vm
}

// kontextAlwaysOf reduces one Always-Tier ContextItem (Instruction,
// ActiveContext, or an immer-mode memory) to its display VM.
func kontextAlwaysOf(item usecase.ContextItem) KontextAlwaysVM {
	return KontextAlwaysVM{
		DocID:      item.ID,
		Title:      item.Title,
		ChipClass:  DocTypeChipClass(item.Type),
		TypeLabel:  DocTypeLabel(item.Type),
		ScopeLabel: item.ScopeLabel,
		TokensStr:  fmtThousandsDE(item.EstTokens),
		Mode:       item.ContextMode,
	}
}

// kontextHiddenOf reduces one cc.Hidden ContextItem (a nie-mode doc) to its
// display VM for the Ausgeblendet section.
func kontextHiddenOf(item usecase.ContextItem) KontextHiddenVM {
	return KontextHiddenVM{
		DocID:      item.ID,
		Title:      item.Title,
		ChipClass:  DocTypeChipClass(item.Type),
		TypeLabel:  DocTypeLabel(item.Type),
		ScopeLabel: item.ScopeLabel,
		TokensStr:  fmtThousandsDE(item.EstTokens),
		Mode:       item.ContextMode,
	}
}

// Select wählt die Karte der Lesespalte: docID, wenn sie in einer der drei
// Listen steht, sonst die erste — Immer-Tier vor Rang vor Ausgeblendet. Es
// markiert die Zeile und hängt die Wahl an Seiten- und Aktionslinks, damit
// Aktionen und SSE-Neuladen sie nicht verlieren.
func (vm *KontextVM) Select(docID string) {
	has := func(id string) bool {
		for _, a := range vm.Always {
			if a.DocID == id {
				return true
			}
		}
		for _, r := range vm.Rows {
			if r.DocID == id {
				return true
			}
		}
		for _, h := range vm.Hidden {
			if h.DocID == id {
				return true
			}
		}
		return false
	}
	if docID == "" || !has(docID) {
		docID = ""
		switch {
		case len(vm.Always) > 0:
			docID = vm.Always[0].DocID
		case len(vm.Rows) > 0:
			docID = vm.Rows[0].DocID
		case len(vm.Hidden) > 0:
			docID = vm.Hidden[0].DocID
		}
	}
	vm.Selected = docID
	vm.SelQuery, vm.ActQuery = "", ""
	if docID != "" {
		vm.SelQuery = "?doc=" + docID
		vm.ActQuery = "?sel=" + docID
	}
	for i := range vm.Always {
		vm.Always[i].Selected = vm.Always[i].DocID == docID
	}
	for i := range vm.Rows {
		vm.Rows[i].Selected = vm.Rows[i].DocID == docID
	}
	for i := range vm.Hidden {
		vm.Hidden[i].Selected = vm.Hidden[i].DocID == docID
	}
}

// BuildKontextLese baut die Lesespalte für doc aus dem komponierten Kontext:
// Stand (immer / Rang n von N / Budget erreicht / ausgeblendet), Anteil am
// Budget und die gerenderte Karte. html kommt vom Aufrufer (RenderDocument
// mit den Auflösern des Registers), damit der Builder rein bleibt.
func BuildKontextLese(cc usecase.ComposedContext, doc domain.Document, html template.HTML, now time.Time) *KontextLeseVM {
	l := &KontextLeseVM{
		DocID:       doc.ID,
		Title:       doc.Title,
		Path:        doc.Path,
		ChipClass:   DocTypeChipClass(doc.Type),
		TypeLabel:   DocTypeLabel(doc.Type),
		StatusKey:   "document.context.hidden",
		Actor:       doc.UpdatedByRef,
		When:        FmtRelTime(doc.UpdatedAt, now),
		ReadMinutes: ReadingTime(doc.Body),
		Mode:        doc.ContextMode.OrAuto(),
		HTML:        html,
		EditHref:    "/wissen/" + doc.ID + "/bearbeiten",
		OpenHref:    "/wissen/" + doc.ID,
	}
	tokens := 0
	found := false
	mark := func(it usecase.ContextItem, key string) bool {
		if it.ID != doc.ID {
			return false
		}
		tokens, l.StatusKey, found = it.EstTokens, key, true
		return true
	}
	for _, it := range cc.Instructions {
		if mark(it, "document.context.always") {
			break
		}
	}
	if !found && cc.ActiveContext != nil {
		mark(*cc.ActiveContext, "document.context.always")
	}
	if !found {
		for _, it := range cc.AlwaysMemories {
			if mark(it, "document.context.always") {
				break
			}
		}
	}
	if !found {
		total := 0
		for _, r := range cc.Ranked {
			if r.Included {
				total++
			}
		}
		for _, r := range cc.Ranked {
			if r.Item.ID != doc.ID {
				continue
			}
			if r.Included {
				mark(r.Item, "document.context.in")
				l.Rank, l.Total = r.Rank, total
			} else {
				mark(r.Item, "document.context.dropped")
			}
			break
		}
	}
	if !found {
		for _, it := range cc.Hidden {
			if mark(it, "document.context.hidden") {
				break
			}
		}
	}
	if !found {
		tokens = (len(doc.Body) + 3) / 4 // dieselbe Schätzung wie Compose: vier Bytes je Token
	}
	l.TokensStr = fmtThousandsDE(tokens)
	if cc.Budget.Cap > 0 {
		l.SharePct = (tokens*100 + cc.Budget.Cap/2) / cc.Budget.Cap
	}
	return l
}
