package main

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestRepoParentItems — only engagements and vorhaben are valid parents for a new
// repo; each is path-labeled (so duplicate names stay distinguishable) and sorted.
func TestRepoParentItems(t *testing.T) {
	nodes := []domain.Node{
		{ID: "eng2", Slug: "rtl", Kind: domain.KindEngagement},
		{ID: "eng1", Slug: "privat", Kind: domain.KindEngagement},
		{ID: "vor1", Slug: "sf", Kind: domain.KindVorhaben, ParentID: p("eng1")},
		{ID: "repo1", Slug: "sf", Kind: domain.KindRepo, ParentID: p("vor1")}, // excluded
	}
	items := repoParentItems(nodes)

	if len(items) != 3 {
		t.Fatalf("got %d parent items, want 3 (2 engagements + 1 vorhaben, no repo)", len(items))
	}
	gotLabels := []string{items[0].Label, items[1].Label, items[2].Label}
	wantLabels := []string{"privat", "privat/sf", "rtl"} // sorted by path
	for i := range wantLabels {
		if gotLabels[i] != wantLabels[i] {
			t.Errorf("item[%d].Label = %q, want %q", i, gotLabels[i], wantLabels[i])
		}
	}
	// The vorhaben item must carry its node ID for the create-under call.
	byLabel := map[string]string{}
	for _, it := range items {
		byLabel[it.Label] = it.ID
	}
	if byLabel["privat/sf"] != "vor1" {
		t.Errorf("vorhaben item ID = %q, want vor1", byLabel["privat/sf"])
	}
}
