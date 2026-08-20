package webui

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
)

// docKastenCap bounds the middle column. Screens 01/22 show the register's
// cards beside the text, not the register's archive — a column that unrolls
// hundreds of rows next to a Lesetext is a wall, so past the cap the column
// says how many more there are and where they live.
const docKastenCap = 30

// DocKastenVM is the middle column of Screens 01/22: the open card's register
// (or, for a card filed outside the tree, its Regal) as a browsable card list
// between Schiene and Lesespalte.
type DocKastenVM struct {
	Title     string
	KindKey   string // i18n key of the register kind badge; "" for a Regal
	KindClass string // Ebenen text color for the badge
	CardCount int
	NewHref   string
	Tabs      []DocKastenTab
	Rows      []DocKastenRow
	MoreHref  string // where the rest lives once the column caps
	MoreCount int    // total cards in scope (only set when capped)
}

// DocKastenTab is one Regal tab above the card list. Count always describes
// the whole scope — the tabs are a table of contents, not a search result.
type DocKastenTab struct {
	Label  string
	Regal  string // type value carried in ?regal=; "" is Alle
	Href   string
	Count  int
	Active bool
}

// DocKastenRow is one card row: type eyebrow, Datumsstaffel, title, excerpt.
type DocKastenRow struct {
	ID        string
	Href      string
	TypeLabel string
	TypeColor string
	When      string
	Title     string
	Excerpt   string
	Active    bool
}

// ENTSCHEIDUNG (2026-08-20): Beide Linien haben unabhängig voneinander eine
// Datumsstaffel gebaut. Diese Datei rief die des Entwurfs-Branches
// (staffelText); sie ruft jetzt FmtStaffel — die Fassung dieser Linie ist an
// sechs Stellen verdrahtet und hält timefmt sprachfrei, indem die Wörter von
// außen hereinkommen. Eine von beiden musste sterben.
//
// BuildDocKasten assembles the column for the card being read. Scope is the
// card's register when it has one, otherwise every card of the same Regal —
// never the whole box, that is what the Bibliothek is for. regal filters the
// rows by type; the tab counts stay untouched by it.
func BuildDocKasten(ctx context.Context, doc domain.Document, all []domain.Document, nodeNames map[string]string, nodeKinds map[string]domain.NodeKind, now time.Time, regal string) *DocKastenVM {
	vm := &DocKastenVM{}

	var scoped []domain.Document
	if doc.NodeID != nil && *doc.NodeID != "" {
		nodeID := *doc.NodeID
		vm.Title = nodeNames[nodeID]
		if vm.Title == "" {
			vm.Title = i18n.T(ctx, "wissen.unknownRegister")
		}
		badge := NodeKindStyle(nodeKinds[nodeID])
		vm.KindKey = badge.LabelKey
		vm.KindClass = docKastenKindClass(nodeKinds[nodeID])
		vm.NewHref = "/wissen/neu?node=" + url.QueryEscape(nodeID)
		vm.MoreHref = "/nodes/" + url.PathEscape(nodeID)
		for _, d := range all {
			if d.NodeID != nil && *d.NodeID == nodeID {
				scoped = append(scoped, d)
			}
		}
	} else {
		shelf, ok := WissenShelfForType(doc.Type)
		if !ok {
			return nil
		}
		vm.Title = i18n.T(ctx, shelf.LabelKey)
		vm.NewHref = "/wissen/neu"
		vm.MoreHref = "/wissen/typ/"+shelf.TypeKey
		for _, d := range all {
			if (d.NodeID == nil || *d.NodeID == "") && DocumentInShelf(d, shelf) {
				scoped = append(scoped, d)
			}
		}
	}

	sort.SliceStable(scoped, func(i, j int) bool { return scoped[i].UpdatedAt.After(scoped[j].UpdatedAt) })
	vm.CardCount = len(scoped)
	vm.Tabs = docKastenTabs(scoped, doc.ID, regal)

	for _, d := range scoped {
		if regal != "" && string(d.Type) != regal {
			continue
		}
		if len(vm.Rows) == docKastenCap {
			vm.MoreCount = len(scoped)
			break
		}
		vm.Rows = append(vm.Rows, DocKastenRow{
			ID:        d.ID,
			Href:      "/wissen/" + url.PathEscape(d.ID) + docKastenQuery(regal),
			TypeLabel: DocKindStyle(d.Type).Label,
			TypeColor: docRowTypeColor(d.Type),
			When:      FmtStaffel(ctx, d.UpdatedAt, now),
			Title:     d.Title,
			Excerpt:   docKastenExcerpt(d.Body),
			Active:    d.ID == doc.ID,
		})
	}
	if vm.MoreCount == 0 {
		vm.MoreHref = ""
	}
	return vm
}

// docKastenTabs derives the Regal tabs from the types actually present, in
// first-appearance order of the (date-sorted) scope. A tab for a type the
// register does not hold would only lead to an empty list.
func docKastenTabs(scoped []domain.Document, docID, regal string) []DocKastenTab {
	self := "/wissen/" + url.PathEscape(docID)
	tabs := []DocKastenTab{{Label: "", Regal: "", Href: self, Count: len(scoped), Active: regal == ""}}
	at := map[domain.DocumentType]int{}
	for _, d := range scoped {
		if i, seen := at[d.Type]; seen {
			tabs[i].Count++
			continue
		}
		at[d.Type] = len(tabs)
		tabs = append(tabs, DocKastenTab{
			Label:  DocKindStyle(d.Type).Label,
			Regal:  string(d.Type),
			Href:   self + "?regal=" + url.QueryEscape(string(d.Type)),
			Count:  1,
			Active: regal == string(d.Type),
		})
	}
	return tabs
}

// docKastenExcerpt clips a card body to one clean preview line: frontmatter,
// heading lines and fenced code blocks are skipped (the title already stands
// above, and code is not a summary), markdown syntax is simplified away —
// the excerpt is prose, not source.
func docKastenExcerpt(body string) string {
	inFence := false
	for _, line := range strings.Split(stripPreviewFrontmatter(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Pair up inline markers before simplify trims the line's leading
		// list bullet: its TrimLeft would otherwise eat the opening ** of a
		// bold-led bullet and strand the closing marker mid-sentence.
		line = markdownPreviewRE.ReplaceAllString(line, "$1")
		line = simplifyPreviewMarkdown(line)
		if line == "" {
			continue
		}
		const maxLen = 90
		if r := []rune(line); len(r) > maxLen {
			return string(r[:maxLen]) + "…"
		}
		return line
	}
	return ""
}

// docKastenQuery keeps an active Regal filter alive while hopping between
// cards of the same register.
func docKastenQuery(regal string) string {
	if regal == "" {
		return ""
	}
	return "?regal=" + url.QueryEscape(regal)
}

// docKastenKindClass colors the kind badge in the register's Ebenenfarbe
// over its wash (TOKENS.md: Engagement amber, Vorhaben violet, Repo steel).
func docKastenKindClass(k domain.NodeKind) string {
	switch k {
	case domain.KindEngagement:
		return "text-amber bg-amber-wash"
	case domain.KindVorhaben:
		return "text-violet bg-violet-wash"
	default:
		return "text-steel bg-steel-wash"
	}
}
