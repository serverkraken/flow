package webui

import (
	"context"
	"html/template"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

type EditorOption struct {
	Value string
	Label string
}

type EditorVM struct {
	User           string
	ID             string
	Type           string
	NodeID         string
	Path           string
	Date           string
	Title          string
	TagsCSV        string
	Body           string
	Err            string
	PreviewHTML    template.HTML
	TypeOptions    []EditorOption
	ProjectOptions []EditorOption
}

func (vm EditorVM) Editing() bool { return vm.ID != "" }

func (vm EditorVM) Daily() bool { return vm.Type == string(domain.DocDaily) }

func (vm EditorVM) ShowsProject() bool {
	switch domain.DocumentType(vm.Type) {
	case domain.DocProject, domain.DocAgent, domain.DocMemory, domain.DocInstruction,
		domain.DocSkill, domain.DocPlan, domain.DocSpec, domain.DocActiveContext:
		return true
	default:
		return false
	}
}

func (vm EditorVM) Action() string {
	if vm.Editing() {
		return "/wissen/" + vm.ID
	}
	return "/wissen"
}

func (vm EditorVM) HeadingKey() string {
	if vm.Editing() {
		return "wissen.edit.title"
	}
	return "wissen.new"
}

// editorCrumbs builds the Wissen -> heading breadcrumb trail for the editor
// page, reusing components.Breadcrumb (like nodeCrumbs does for the cockpit).
// editorCrumbs behält beim Schreiben die Spur der Karte: der Zurück-Chip
// führt auf die Karte selbst, nicht in die Bibliothek (Screen 16). Eine neue
// Karte hat noch nichts, wohin sie zurückführen könnte — dann trägt die Spur
// allein die Bibliothek.
func editorCrumbs(ctx context.Context, vm EditorVM) (*components.Crumb, []components.Crumb, string) {
	var back *components.Crumb
	if vm.Editing() {
		back = &components.Crumb{Href: "/wissen/" + vm.ID, Label: vm.Title}
	}
	return back, []components.Crumb{
		{Href: "/wissen", Label: components.T(ctx, "wissen.title")},
		{Label: components.T(ctx, vm.HeadingKey())},
	}, "karte"
}

func DocumentTypeOptions(ctx context.Context, _ string) []EditorOption {
	values := domain.DocumentTypes()
	opts := make([]EditorOption, 0, len(values))
	for _, v := range values {
		opts = append(opts, EditorOption{Value: string(v), Label: components.T(ctx, "wissen.type."+string(v))})
	}
	return opts
}
