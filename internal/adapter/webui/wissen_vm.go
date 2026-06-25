package webui

import (
	"net/url"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// ProjectGroup groups project notes under one project header.
type ProjectGroup struct {
	ProjectID string
	Name      string
	Color     string
	Glyph     string
	Docs      []DocRow
}

// WissenVM is the view model for the AppShell Wissen list page.
type WissenVM struct {
	User       string
	AllTags    []TagChip
	ActiveTags []string
	SearchQ    string
	Query      string // encoded query preserved for the SSE fragment hx-get

	// Category sections; empty when the page is in search mode.
	Daily  []DocRow
	Notes  []ProjectGroup
	Free   []DocRow
	System []DocRow

	// Search mode.
	Results []SearchRow

	Page components.PageNav
}

// GroupDocsByCategory splits docs into the four Wissen list sections.
func GroupDocsByCategory(docs []domain.Document, projectNames, projectColors map[string]string) WissenVM {
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
			pid := projectIDString(d.ProjectID)
			g := groups[pid]
			if g == nil {
				kind := DocKindStyle(domain.DocProject)
				g = &ProjectGroup{
					ProjectID: pid,
					Name:      projectDisplayName(pid, projectNames),
					Color:     ColorHex(projectColors[pid]),
					Glyph:     kind.Glyph,
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
		pid := projectIDString(d.ProjectID)
		if seen[pid] {
			continue
		}
		seen[pid] = true
		vm.Notes = append(vm.Notes, *groups[pid])
	}
	return vm
}

func groupDocsByCategory(docs []domain.Document, projectNames, projectColors map[string]string) WissenVM {
	return GroupDocsByCategory(docs, projectNames, projectColors)
}

func docRowFromDocument(d domain.Document, projectColors map[string]string) DocRow {
	row := DocRow{ID: d.ID, Type: string(d.Type), Path: d.Path, Title: d.Title, Tags: d.Tags}
	if d.ProjectID != nil {
		row.ProjectID = *d.ProjectID
		row.ProjectColor = ColorHex(projectColors[*d.ProjectID])
	}
	return row
}

func projectIDString(projectID *string) string {
	if projectID == nil {
		return ""
	}
	return *projectID
}

func projectDisplayName(projectID string, projectNames map[string]string) string {
	if name := projectNames[projectID]; name != "" {
		return name
	}
	return projectID
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
