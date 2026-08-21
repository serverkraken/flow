package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestArtifactRefsAndFilter(t *testing.T) {
	docs := []domain.Document{
		{ID: "a", Title: "Plan", Body: "Siehe ![[wochenansicht.png]] und nochmal [[wochenansicht.png|Abb.]]"},
		{ID: "b", Title: "Spec", Body: "[[angebot.pdf]] und [[wochenansicht.png#x]]"},
		{ID: "c", Title: "Archiv", Body: "[[wochenansicht.png]]", Archived: true},
	}
	refs := ArtifactRefs(docs)
	if len(refs["wochenansicht.png"]) != 2 || refs["wochenansicht.png"][0].Title != "Plan" || len(refs["angebot.pdf"]) != 1 {
		t.Errorf("refs = %+v", refs)
	}
	cards := []ArtifactCardVM{
		{Slug: "wochenansicht.png", IsImage: true}, {Slug: "angebot.pdf", TypeLabel: "PDF"}, {Slug: "zeiten.csv", TypeLabel: "CSV"},
	}
	AttachArtifactRefs(cards, refs)
	if cards[0].Refs != 2 || cards[2].Refs != 0 {
		t.Errorf("attach: %+v", cards)
	}
	vm := FilterArtifactCards(WissenArtifactsVM{Cards: cards}, "pdf")
	if vm.Total != 3 || vm.Unreferenced != 1 || vm.CountBild != 1 || vm.CountPDF != 1 || vm.CountDaten != 1 || len(vm.Cards) != 1 || vm.Cards[0].Slug != "angebot.pdf" {
		t.Errorf("filter: %+v", vm)
	}
	if ArtifactTypeFilter("egal") != "" || ArtifactTypeFilter("bild") != "bild" {
		t.Errorf("ArtifactTypeFilter")
	}
}
