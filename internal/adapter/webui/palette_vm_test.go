package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBuildPaletteVM_FuzzyAndOrder(t *testing.T) {
	nodes := []domain.Node{
		{ID: "n1", Name: "gitlab.com/dataalliance/products/foolu/product/backstage", Kind: domain.KindRepo},
		{ID: "n2", Name: "RTL Extern", Kind: domain.KindEngagement},
		{ID: "n3", Name: "github.com/serverkraken/flow", Kind: domain.KindRepo},
	}
	docs := []domain.Document{
		{ID: "d1", Title: "Kompendium-Integration in flow", Path: "notes/kompendium"},
		{ID: "d2", Title: "Backstage Probleme", Path: "docs/group-processor-token-reuse"},
	}
	// fuzzy: "kompend" findet das Kompendium-Doc (Soenne-Gesetz)
	vm := BuildPaletteVM(nodes, nil, docs, "kompend")
	if len(vm.Docs) != 1 || vm.Docs[0].Title != "Kompendium-Integration in flow" {
		t.Fatalf("fuzzy recall failed: %+v", vm.Docs)
	}
	// leere Query: MRU-Knoten zuerst, dann Rest; Docs in gegebener (updated-desc) Reihenfolge
	vm = BuildPaletteVM(nodes, []string{"n3"}, docs, "")
	if vm.Nodes[0].ID != "n3" {
		t.Fatalf("MRU node not first: %+v", vm.Nodes)
	}
	// Kurzname + voller Pfad getrennt
	var bs PaletteNodeVM
	for _, n := range vm.Nodes {
		if n.ID == "n1" {
			bs = n
		}
	}
	if bs.Short != "backstage" || bs.Full != "gitlab.com/dataalliance/products/foolu/product/backstage" {
		t.Fatalf("short/full wrong: %+v", bs)
	}
}
