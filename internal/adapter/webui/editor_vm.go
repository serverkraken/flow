package webui

import (
	"context"
	"html/template"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
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
	Title          string
	TagsCSV        string
	Body           string
	Err            string
	PreviewHTML    template.HTML
	TypeOptions    []EditorOption
	ProjectOptions []EditorOption
}

func (vm EditorVM) Editing() bool { return vm.ID != "" }

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
func editorCrumbs(ctx context.Context, vm EditorVM) []components.Crumb {
	return []components.Crumb{
		{Href: "/wissen", Label: components.T(ctx, "wissen.title")},
		{Label: components.T(ctx, vm.HeadingKey())},
	}
}

func DocumentTypeOptions(_ string) []EditorOption {
	values := []string{"free", "project", "daily", "agent", "memory", "instruction", "skill", "plan"}
	opts := make([]EditorOption, 0, len(values))
	for _, v := range values {
		opts = append(opts, EditorOption{Value: v, Label: v})
	}
	return opts
}
