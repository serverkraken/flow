package webui

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// WissenCategory identifies one first-level Wissen page.
type WissenCategory struct {
	ID             string
	Slug           string
	LabelKey       string
	DescriptionKey string
	Href           string
	Types          []domain.DocumentType
}

// WissenCategoryCard is one card on the Wissen overview page.
type WissenCategoryCard struct {
	WissenCategory
	Count  int
	Latest []DocRow
}

// WissenOverviewVM is the view model for the /wissen overview page.
type WissenOverviewVM struct {
	WissenVM
	Categories []WissenCategoryCard
}

// WissenCategoryVM is the view model for one category subpage.
type WissenCategoryVM struct {
	WissenVM
	Category WissenCategory
	Rows     []DocRow
	Groups   []ProjectGroup
	Total    int
}

// ProjectGroup groups project notes under one project header.
type ProjectGroup struct {
	NodeID string
	Name   string
	Color  string
	Glyph  string
	Kind   domain.NodeKind // kind of the linked node (used by nodeKindBadge)
	Docs   []DocRow
}

// WissenVM is the view model for the AppShell Wissen list page.
type WissenVM struct {
	User         string
	AllTags      []TagChip
	ActiveTags   []string
	SearchQ      string
	Query        string // encoded query preserved for the SSE fragment hx-get
	SearchAction string
	ResetHref    string

	// Category sections; empty when the page is in search mode.
	Daily  []DocRow
	Notes  []ProjectGroup
	Free   []DocRow
	System []DocRow

	// Search mode.
	Results []SearchRow

	Page components.PageNav
}

var (
	wikiAliasPreviewRE  = regexp.MustCompile(`\[\[([^|\]]+)\|([^\]]+)\]\]`)
	wikiSimplePreviewRE = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	htmlPreviewRE       = regexp.MustCompile(`<[^>]*>`)
	markdownPreviewRE   = regexp.MustCompile(`[*_~` + "`" + `]+([^*_~` + "`" + `]+)[*_~` + "`" + `]+`)
	markdownLinkRE      = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
)

func WissenCategories() []WissenCategory {
	return []WissenCategory{
		{
			ID:             "daily",
			Slug:           "daily",
			LabelKey:       "wissen.daily",
			DescriptionKey: "wissen.daily.description",
			Href:           "/wissen/daily",
			Types:          []domain.DocumentType{domain.DocDaily},
		},
		{
			ID:             "projekte",
			Slug:           "projekte",
			LabelKey:       "wissen.notes",
			DescriptionKey: "wissen.notes.description",
			Href:           "/wissen/projekte",
			Types:          []domain.DocumentType{domain.DocProject},
		},
		{
			ID:             "frei",
			Slug:           "frei",
			LabelKey:       "wissen.free",
			DescriptionKey: "wissen.free.description",
			Href:           "/wissen/frei",
			Types:          []domain.DocumentType{domain.DocFree},
		},
		{
			ID:             "system",
			Slug:           "system",
			LabelKey:       "wissen.system",
			DescriptionKey: "wissen.system.description",
			Href:           "/wissen/system",
			Types: []domain.DocumentType{
				domain.DocAgent,
				domain.DocMemory,
				domain.DocInstruction,
				domain.DocSkill,
				domain.DocPlan,
			},
		},
	}
}

func WissenCategoryFromSlug(slug string) (WissenCategory, bool) {
	for _, c := range WissenCategories() {
		if c.Slug == slug {
			return c, true
		}
	}
	return WissenCategory{}, false
}

func DocumentInWissenCategory(d domain.Document, c WissenCategory) bool {
	for _, typ := range c.Types {
		if d.Type == typ {
			return true
		}
	}
	return false
}

func WissenCategoryForType(t domain.DocumentType) (WissenCategory, bool) {
	for _, c := range WissenCategories() {
		for _, typ := range c.Types {
			if t == typ {
				return c, true
			}
		}
	}
	return WissenCategory{}, false
}

func BuildWissenOverview(docs []domain.Document, projectColors map[string]string) WissenOverviewVM {
	sorted := sortedDocuments(docs)
	vm := WissenOverviewVM{}
	for _, c := range WissenCategories() {
		card := WissenCategoryCard{WissenCategory: c}
		for _, d := range sorted {
			if !DocumentInWissenCategory(d, c) {
				continue
			}
			card.Count++
			if len(card.Latest) < 3 {
				card.Latest = append(card.Latest, docRowFromDocument(d, projectColors))
			}
		}
		vm.Categories = append(vm.Categories, card)
	}
	return vm
}

func BuildWissenCategory(c WissenCategory, docs []domain.Document, projectNames, projectColors map[string]string, nodeKinds map[string]domain.NodeKind) WissenCategoryVM {
	filtered := make([]domain.Document, 0, len(docs))
	for _, d := range sortedDocuments(docs) {
		if DocumentInWissenCategory(d, c) {
			filtered = append(filtered, d)
		}
	}
	grouped := GroupDocsByCategory(filtered, projectNames, projectColors, nodeKinds)
	previews := map[string]string{}
	for _, d := range filtered {
		previews[d.ID] = DocPreviewText(d.Body, 5)
	}
	for gi := range grouped.Notes {
		for ri := range grouped.Notes[gi].Docs {
			grouped.Notes[gi].Docs[ri].Preview = previews[grouped.Notes[gi].Docs[ri].ID]
		}
	}
	vm := WissenCategoryVM{
		WissenVM: grouped,
		Category: c,
		Groups:   grouped.Notes,
		Total:    len(filtered),
	}
	for _, d := range filtered {
		row := docRowFromDocument(d, projectColors)
		row.Preview = DocPreviewText(d.Body, 5)
		vm.Rows = append(vm.Rows, row)
	}
	return vm
}

func DocPreviewText(body string, maxLines int) string {
	if maxLines <= 0 {
		maxLines = 5
	}
	body = stripPreviewFrontmatter(body)
	var out []string
	inFence := false
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || line == "" {
			continue
		}
		line = simplifyPreviewMarkdown(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) == maxLines {
			break
		}
	}
	return strings.Join(out, "\n")
}

func stripPreviewFrontmatter(body string) string {
	if !strings.HasPrefix(body, "---\n") {
		return body
	}
	if end := strings.Index(body[4:], "\n---\n"); end >= 0 {
		return body[end+9:]
	}
	return body
}

func simplifyPreviewMarkdown(line string) string {
	line = strings.TrimLeft(line, "#>-*+ \t")
	line = wikiAliasPreviewRE.ReplaceAllString(line, "$2")
	line = wikiSimplePreviewRE.ReplaceAllString(line, "$1")
	line = markdownLinkRE.ReplaceAllString(line, "$1")
	line = htmlPreviewRE.ReplaceAllString(line, "")
	line = markdownPreviewRE.ReplaceAllString(line, "$1")
	return strings.Join(strings.Fields(line), " ")
}

func sortedDocuments(docs []domain.Document) []domain.Document {
	out := append([]domain.Document(nil), docs...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[j].UpdatedAt.Before(out[i].UpdatedAt)
	})
	return out
}

// GroupDocsByCategory splits docs into the four Wissen list sections.
// nodeKinds maps node id → NodeKind and is used to populate ProjectGroup.Kind
// for the node-kind badge rendered in group headers.
func GroupDocsByCategory(docs []domain.Document, projectNames, projectColors map[string]string, nodeKinds map[string]domain.NodeKind) WissenVM {
	var vm WissenVM
	groups := map[string]*ProjectGroup{}

	for _, d := range docs {
		row := docRowFromDocument(d, projectColors)
		switch d.Type {
		case domain.DocDaily:
			vm.Daily = append(vm.Daily, row)
		case domain.DocFree:
			vm.Free = append(vm.Free, row)
		case domain.DocProject:
			pid := projectIDString(d.NodeID)
			g := groups[pid]
			if g == nil {
				docKind := DocKindStyle(domain.DocProject)
				var nk domain.NodeKind
				if nodeKinds != nil {
					nk = nodeKinds[pid]
				}
				g = &ProjectGroup{
					NodeID: pid,
					Name:   projectDisplayName(pid, projectNames),
					Color:  ColorHex(projectColors[pid]),
					Glyph:  docKind.Glyph,
					Kind:   nk,
				}
				groups[pid] = g
			}
			g.Docs = append(g.Docs, row)
		default:
			vm.System = append(vm.System, row)
		}
	}

	seen := map[string]bool{}
	for _, d := range docs {
		if d.Type != domain.DocProject {
			continue
		}
		pid := projectIDString(d.NodeID)
		if seen[pid] {
			continue
		}
		seen[pid] = true
		vm.Notes = append(vm.Notes, *groups[pid])
	}
	return vm
}

func groupDocsByCategory(docs []domain.Document, projectNames, projectColors map[string]string, nodeKinds map[string]domain.NodeKind) WissenVM {
	return GroupDocsByCategory(docs, projectNames, projectColors, nodeKinds)
}

func docRowFromDocument(d domain.Document, projectColors map[string]string) DocRow {
	row := DocRow{ID: d.ID, Type: string(d.Type), Path: d.Path, Title: d.Title, Tags: d.Tags}
	if d.NodeID != nil {
		row.NodeID = *d.NodeID
		row.ProjectColor = ColorHex(projectColors[*d.NodeID])
	}
	return row
}

func projectIDString(nodeID *string) string {
	if nodeID == nil {
		return ""
	}
	return *nodeID
}

func projectDisplayName(nodeID string, projectNames map[string]string) string {
	if name := projectNames[nodeID]; name != "" {
		return name
	}
	return nodeID
}

func WissenTabs(vm WissenVM) []components.CatTab {
	return []components.CatTab{
		{ID: "daily-sec", LabelKey: "wissen.daily", Count: len(vm.Daily)},
		{ID: "notes-sec", LabelKey: "wissen.notes", Count: projectDocCount(vm.Notes)},
		{ID: "free-sec", LabelKey: "wissen.free", Count: len(vm.Free)},
		{ID: "system-sec", LabelKey: "wissen.system", Count: len(vm.System)},
	}
}

func WissenEmpty(vm WissenVM) bool {
	return len(vm.Daily) == 0 && len(vm.Notes) == 0 && len(vm.Free) == 0 && len(vm.System) == 0
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

func wissenCategoryNavClass(active, slug string) string {
	base := "inline-flex items-center gap-2 rounded-xl px-3 py-2 text-[.84rem] font-medium transition"
	if active == slug {
		return base + " border border-blue/40 bg-blue/10 text-blue"
	}
	return base + " glass text-muted hover:border-blue/40 hover:text-blue"
}

func projectDocCount(groups []ProjectGroup) int {
	var n int
	for _, group := range groups {
		n += len(group.Docs)
	}
	return n
}

func rowKind(row DocRow) DocKind {
	return DocKindStyle(domain.DocumentType(row.Type))
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

func swatchStyle(color string) string {
	if color == "" {
		return ""
	}
	return "--swatch: " + color
}
