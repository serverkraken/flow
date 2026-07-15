package webui

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// wissenRecentCap bounds the Wissen overview's "Zuletzt aktualisiert" section
// to 8 rows (Mockup Z.832–838) — mirrors the cockpit Wissen section's
// identical cap (webui_cockpit.go wissenSectionCap). ?recent=all expands it
// in place (BuildWissenOverview's recentAll parameter).
const wissenRecentCap = 8

// WissenShelf is one type-based shelf on the Wissen library overview
// (Mockup Z.821–830): a group of domain.DocumentType shown as one row with a
// document count. Replaces the historic WissenCategory four-way split
// (daily/projekte/frei/system) — shelves group by document type instead,
// with no 1:1 successor for the old "system" bucket (Codex #17): its five
// legacy types now spread across plan/memory/context/spec.
type WissenShelf struct {
	Types    []domain.DocumentType
	TypeKey  string // mono key on the shelf row + the /wissen/typ/{TypeKey} route segment
	LabelKey string
	DescKey  string
	Count    int
}

func (s WissenShelf) NewDocumentHref() string {
	if len(s.Types) == 0 {
		return "/wissen/neu"
	}
	return "/wissen/neu?type=" + url.QueryEscape(string(s.Types[0]))
}

// WissenShelves returns the 7 fixed type-shelves in Mockup order (Z.821–
// 830). DocAgent (deprecated, B3d: split into spec/plan) folds into the spec
// shelf; DocActiveContext/DocInstruction/DocSkill fold into "context".
func WissenShelves() []WissenShelf {
	return []WissenShelf{
		{Types: []domain.DocumentType{domain.DocProject}, TypeKey: "project", LabelKey: "wissen.shelf.project", DescKey: "wissen.shelf.project.desc"},
		{Types: []domain.DocumentType{domain.DocPlan}, TypeKey: "plan", LabelKey: "wissen.shelf.plan", DescKey: "wissen.shelf.plan.desc"},
		{Types: []domain.DocumentType{domain.DocSpec, domain.DocAgent}, TypeKey: "spec", LabelKey: "wissen.shelf.spec", DescKey: "wissen.shelf.spec.desc"},
		{Types: []domain.DocumentType{domain.DocMemory}, TypeKey: "memory", LabelKey: "wissen.shelf.memory", DescKey: "wissen.shelf.memory.desc"},
		{Types: []domain.DocumentType{domain.DocDaily}, TypeKey: "daily", LabelKey: "wissen.shelf.daily", DescKey: "wissen.shelf.daily.desc"},
		{Types: []domain.DocumentType{domain.DocActiveContext, domain.DocInstruction, domain.DocSkill}, TypeKey: "context", LabelKey: "wissen.shelf.context", DescKey: "wissen.shelf.context.desc"},
		{Types: []domain.DocumentType{domain.DocFree}, TypeKey: "free", LabelKey: "wissen.shelf.free", DescKey: "wissen.shelf.free.desc"},
	}
}

// WissenShelfFromTypeKey resolves a /wissen/typ/{type} path segment to its shelf.
func WissenShelfFromTypeKey(key string) (WissenShelf, bool) {
	for _, s := range WissenShelves() {
		if s.TypeKey == key {
			return s, true
		}
	}
	return WissenShelf{}, false
}

// WissenShelfForType resolves a document's own type to the shelf it lands in
// — used by the editor's post-delete redirect (webui_document.go
// wissenShelfHrefForType), replacing the old WissenCategoryForType.
func WissenShelfForType(t domain.DocumentType) (WissenShelf, bool) {
	for _, s := range WissenShelves() {
		if DocumentInShelf(domain.Document{Type: t}, s) {
			return s, true
		}
	}
	return WissenShelf{}, false
}

// DocumentInShelf reports whether d's type belongs to shelf s.
func DocumentInShelf(d domain.Document, s WissenShelf) bool {
	for _, typ := range s.Types {
		if d.Type == typ {
			return true
		}
	}
	return false
}

// WissenRowVM is one Lesesaal row on a Wissen library surface — the
// "Zuletzt aktualisiert" section on /wissen, the /wissen/typ/{type} shelf
// listing, and (L4 Task 2) the Schreibtisch's "Zuletzt im Wissen" section.
type WissenRowVM struct {
	ID, Title, ChipClass, ChipLabel, Meta, TimeStr string
}

// WissenRowFromDocument maps a document to a WissenRowVM: ChipClass/
// ChipLabel from DocTypeChipClass/DocTypeLabel (Spec §7.1, L2 Bestand), Meta
// is "Pfad · Akteur" (Mockup Z.834) built from UpdatedByRef (Task 3
// provenance stamp) — degrading to just the path when the stamp is unset
// (pre-L3 documents carry no UpdatedByKind/Ref, NULL in storage). TimeStr
// reuses FmtRelTime, the app-wide relative-time convention already used for
// the cockpit/activity feed and the Wissen "zuletzt aktualisiert" caller
// noted in its own doc comment.
func WissenRowFromDocument(d domain.Document, now time.Time) WissenRowVM {
	meta := d.Path
	if d.UpdatedByRef != "" {
		meta = d.Path + " · " + d.UpdatedByRef
	}
	return WissenRowVM{
		ID:        d.ID,
		Title:     d.Title,
		ChipClass: DocTypeChipClass(d.Type),
		ChipLabel: DocTypeLabel(d.Type),
		Meta:      meta,
		TimeStr:   FmtRelTime(d.UpdatedAt, now),
	}
}

// WissenOverviewVM is the view model for the /wissen library overview page:
// the bigsearch/tag-chip machinery (WissenVM, shared with search mode), the
// summary counts, the 7 type shelves, and the capped "Zuletzt aktualisiert"
// list.
type WissenOverviewVM struct {
	WissenVM
	TotalCount  int
	PinnedCount int
	Shelves     []WissenShelf
	Recent      []WissenRowVM
	RecentTotal int
	RecentAll   bool
}

// WissenSummary renders "%d Dokumente · %d angepinnt" (mirrors the
// ProjectsSummary(ctx, vm) convention used by the Projekte pagehead).
func WissenSummary(ctx context.Context, vm WissenOverviewVM) string {
	return fmt.Sprintf(components.T(ctx, "wissen.summary"), vm.TotalCount, vm.PinnedCount)
}

// BuildWissenOverview builds the shelf counts and the capped "Zuletzt
// aktualisiert" list from the owner's full document set. recentAll disables
// the cap (the "Alle N ›" expand-in-place, mirrors the cockpit Wissen
// section's ?wissen=all).
func BuildWissenOverview(docs []domain.Document, now time.Time, recentAll bool) WissenOverviewVM {
	vm := WissenOverviewVM{TotalCount: len(docs)}
	for _, d := range docs {
		if d.Pinned {
			vm.PinnedCount++
		}
	}

	shelves := WissenShelves()
	for i := range shelves {
		for _, d := range docs {
			if DocumentInShelf(d, shelves[i]) {
				shelves[i].Count++
			}
		}
	}
	vm.Shelves = shelves

	sorted := SortedDocuments(docs)
	vm.RecentTotal = len(sorted)
	vm.RecentAll = recentAll
	if !recentAll && len(sorted) > wissenRecentCap {
		sorted = sorted[:wissenRecentCap]
	}
	for _, d := range sorted {
		vm.Recent = append(vm.Recent, WissenRowFromDocument(d, now))
	}
	return vm
}

// WissenTypeVM is the view model for the /wissen/typ/{type} shelf listing:
// one type's documents as flat Lesesaal rows plus pagination — replaces the
// old WissenCategoryVM (which additionally grouped "projekte" docs by
// project; the type-shelf page lists flat, matching the Mockup's other
// Wissen rows).
type WissenTypeVM struct {
	WissenVM
	Shelf WissenShelf
	Rows  []WissenRowVM
	Total int
}

// BuildWissenType maps one page of a shelf's documents to Lesesaal rows.
func BuildWissenType(shelf WissenShelf, pageDocs []domain.Document, now time.Time) WissenTypeVM {
	vm := WissenTypeVM{Shelf: shelf}
	for _, d := range pageDocs {
		vm.Rows = append(vm.Rows, WissenRowFromDocument(d, now))
	}
	return vm
}

// WissenVM is the view model for the Wissen bigsearch/tag-filter machinery,
// shared by the overview page and the type-shelf page.
type WissenVM struct {
	User         string
	AllTags      []TagChip
	ActiveTags   []string
	SearchQ      string
	Query        string // encoded query preserved for the SSE fragment hx-get
	SearchAction string
	ResetHref    string

	// TypeParam is the /wissen/typ?type= shelf filter, empty on the plain
	// overview page. A GET form drops its action URL's existing query string
	// on submit, so the bigsearch form re-submits it via a hidden input
	// (wissenBigsearch) — a query param rather than a path segment because
	// Go's http.ServeMux rejects "/wissen/typ/{type}" as ambiguous against
	// the already-established "/wissen/{id}/bearbeiten" action route (both
	// are 3-segment patterns with the wildcard/literal swapped, e.g.
	// "/wissen/typ/bearbeiten" would match either).
	TypeParam string

	// Search mode.
	Results []SearchRow

	Page components.PageNav
}

// SortedDocuments returns docs newest-first by UpdatedAt. Exported (L4 Task
// 2) so httpserver's homeDataFor can reuse the exact same sort as
// BuildWissenOverview's "Zuletzt aktualisiert" list for the Schreibtisch's
// "Zuletzt im Wissen" section, instead of re-sorting with a second
// convention.
func SortedDocuments(docs []domain.Document) []domain.Document {
	out := append([]domain.Document(nil), docs...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[j].UpdatedAt.Before(out[i].UpdatedAt)
	})
	return out
}

func WissenSingleTagHref(tag string) string {
	return "/wissen?tag=" + url.QueryEscape(tag)
}

func wissenSearchAction(vm WissenVM) string {
	if vm.SearchAction != "" {
		return vm.SearchAction
	}
	return "/wissen"
}

func wissenResetHref(vm WissenVM) string {
	if vm.ResetHref != "" {
		return vm.ResetHref
	}
	return "/wissen"
}

func kindToneClass(tone string) string {
	switch tone {
	case "accent":
		return "border-accent/30 bg-accent/10 text-accent"
	case "success":
		return "border-success/30 bg-success/10 text-success"
	case "highlight":
		return "border-highlight/30 bg-highlight/10 text-highlight"
	case "warning":
		return "border-warning/35 bg-warning/10 text-warning"
	case "danger":
		return "border-danger/35 bg-danger/10 text-danger"
	default:
		return "border-line bg-sunken text-muted"
	}
}
