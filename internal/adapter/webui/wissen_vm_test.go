package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestGroupDocsByCategory(t *testing.T) {
	docs := []domain.Document{
		{ID: "1", Type: domain.DocDaily, Title: "Mon"},
		{ID: "2", Type: domain.DocProject, Title: "Note", ProjectID: strptr("p1")},
		{ID: "3", Type: domain.DocFree, Title: "Urlaub"},
		{ID: "4", Type: domain.DocMemory, Title: "Mem"},
	}
	names := map[string]string{"p1": "Alpha"}
	colors := map[string]string{"p1": "blue"}

	vm := groupDocsByCategory(docs, names, colors)

	if len(vm.Daily) != 1 || len(vm.Free) != 1 || len(vm.System) != 1 {
		t.Fatalf("category split wrong: %+v", vm)
	}
	if len(vm.Notes) != 1 || vm.Notes[0].Name != "Alpha" || len(vm.Notes[0].Docs) != 1 {
		t.Fatalf("project grouping wrong: %+v", vm.Notes)
	}
}

func strptr(s string) *string { return &s }
